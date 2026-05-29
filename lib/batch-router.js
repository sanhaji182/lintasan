// Batch API Routing — queue non-urgent requests for cheaper batch processing
// Requests marked as non-urgent (via header or param) are queued and processed
// in batches at lower cost (typically 50% cheaper via provider batch APIs)
import { getDb, getSetting } from "./db/index.js";

function initBatchTables() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS batch_queue (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      request_id TEXT UNIQUE NOT NULL,
      model TEXT NOT NULL,
      provider TEXT NOT NULL,
      body TEXT NOT NULL,
      status TEXT DEFAULT 'queued',
      response TEXT,
      error TEXT,
      priority INTEGER DEFAULT 0,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      processed_at DATETIME,
      expires_at DATETIME NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_batch_status ON batch_queue(status);
    CREATE INDEX IF NOT EXISTS idx_batch_request ON batch_queue(request_id);
  `);
}

let batchInit = false;
function ensureInit() {
  if (!batchInit) {
    initBatchTables();
    batchInit = true;
  }
}

export function isBatchEnabled() {
  return getSetting("batch_routing_enabled", "true") === "true";
}

// Check if a request should be batched
export function shouldBatch(request, body) {
  if (!isBatchEnabled()) return false;

  // Check explicit header
  const batchHeader = request.headers.get("x-batch-mode");
  if (batchHeader === "true" || batchHeader === "1") return true;

  // Check body param
  if (body._batch === true || body.batch === true) return true;

  // Check priority header (low priority = batch eligible)
  const priority = request.headers.get("x-priority");
  if (priority === "low" || priority === "batch") return true;

  return false;
}

// Queue a request for batch processing
export function queueBatchRequest(requestId, model, provider, body) {
  ensureInit();
  const db = getDb();
  const ttl = parseInt(getSetting("batch_ttl_seconds", "3600"));
  const expiresAt = new Date(Date.now() + ttl * 1000).toISOString();

  db.prepare(`
    INSERT OR REPLACE INTO batch_queue (request_id, model, provider, body, status, expires_at)
    VALUES (?, ?, ?, ?, 'queued', ?)
  `).run(requestId, model, provider, JSON.stringify(body), expiresAt);

  return { requestId, status: "queued", expiresAt };
}

// Get batch request status
export function getBatchStatus(requestId) {
  ensureInit();
  const db = getDb();
  const row = db.prepare("SELECT status, response, error, processed_at FROM batch_queue WHERE request_id = ?").get(requestId);
  if (!row) return null;

  if (row.status === "completed" && row.response) {
    try {
      return { status: "completed", response: JSON.parse(row.response), processedAt: row.processed_at };
    } catch {
      return { status: "error", error: "Invalid response data" };
    }
  }

  return { status: row.status, error: row.error, processedAt: row.processed_at };
}

// Get pending batch items (for batch processor)
export function getPendingBatch(limit = 10) {
  ensureInit();
  const db = getDb();
  return db.prepare(`
    SELECT id, request_id, model, provider, body
    FROM batch_queue
    WHERE status = 'queued' AND expires_at > datetime('now')
    ORDER BY priority DESC, created_at ASC
    LIMIT ?
  `).all(limit);
}

// Mark batch item as processing
export function markProcessing(ids) {
  ensureInit();
  const db = getDb();
  const placeholders = ids.map(() => "?").join(",");
  db.prepare(`UPDATE batch_queue SET status = 'processing' WHERE id IN (${placeholders})`).run(...ids);
}

// Store batch result
export function storeBatchResult(requestId, response, error = null) {
  ensureInit();
  const db = getDb();
  if (error) {
    db.prepare("UPDATE batch_queue SET status = 'error', error = ?, processed_at = datetime('now') WHERE request_id = ?")
      .run(error, requestId);
  } else {
    db.prepare("UPDATE batch_queue SET status = 'completed', response = ?, processed_at = datetime('now') WHERE request_id = ?")
      .run(JSON.stringify(response), requestId);
  }
}

// Process pending batch (called periodically or on-demand)
export async function processBatch(executeFunc) {
  ensureInit();
  const items = getPendingBatch(10);
  if (items.length === 0) return { processed: 0 };

  const ids = items.map((i) => i.id);
  markProcessing(ids);

  let processed = 0;
  let errors = 0;

  for (const item of items) {
    try {
      const body = JSON.parse(item.body);
      const result = await executeFunc({
        providerId: item.provider,
        model: item.model,
        body,
        stream: false,
      });

      if (result.ok) {
        storeBatchResult(item.request_id, result.data);
        processed++;
      } else {
        storeBatchResult(item.request_id, null, result.error || "Provider error");
        errors++;
      }
    } catch (err) {
      storeBatchResult(item.request_id, null, err.message);
      errors++;
    }
  }

  return { processed, errors, total: items.length };
}

// Cleanup expired/old batch entries
export function cleanupBatch() {
  ensureInit();
  const db = getDb();
  db.prepare("DELETE FROM batch_queue WHERE expires_at < datetime('now') OR (status = 'completed' AND processed_at < datetime('now', '-1 day'))").run();
}

// Get batch queue stats
export function getBatchStats() {
  ensureInit();
  const db = getDb();
  const stats = db.prepare(`
    SELECT status, COUNT(*) as count
    FROM batch_queue
    WHERE expires_at > datetime('now')
    GROUP BY status
  `).all();
  return stats.reduce((acc, row) => { acc[row.status] = row.count; return acc; }, {});
}

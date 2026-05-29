// Streaming Response Cache — cache SSE streaming responses
// Captures streaming chunks, stores complete response, replays as SSE on cache hit
import { getDb, getSetting } from "./db/index.js";
import { getCacheKey } from "./cache.js";

function initStreamCache() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS stream_response_cache (
      hash TEXT PRIMARY KEY,
      model TEXT NOT NULL,
      chunks TEXT NOT NULL,
      total_tokens INTEGER DEFAULT 0,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      expires_at DATETIME NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_stream_cache_expires ON stream_response_cache(expires_at);
  `);
}

let streamCacheInit = false;
function ensureInit() {
  if (!streamCacheInit) {
    initStreamCache();
    streamCacheInit = true;
  }
}

export function isStreamCacheEnabled() {
  return getSetting("stream_cache_enabled", "true") === "true";
}

// Get cached stream response
export function getCachedStream(model, messages, params = {}) {
  ensureInit();
  if (!isStreamCacheEnabled()) return null;

  const db = getDb();
  const hash = getCacheKey(model, messages, params);

  const row = db.prepare(
    "SELECT chunks, total_tokens FROM stream_response_cache WHERE hash = ? AND expires_at > datetime('now')"
  ).get(hash);

  if (!row) return null;

  try {
    return { chunks: JSON.parse(row.chunks), totalTokens: row.total_tokens, hash };
  } catch {
    return null;
  }
}

// Store streaming response chunks
export function setCachedStream(model, messages, params, chunks, totalTokens) {
  ensureInit();
  if (!isStreamCacheEnabled()) return;

  const db = getDb();
  const hash = getCacheKey(model, messages, params);
  const ttl = parseInt(getSetting("stream_cache_ttl", "3600"));
  const expiresAt = new Date(Date.now() + ttl * 1000).toISOString();

  db.prepare(`
    INSERT OR REPLACE INTO stream_response_cache (hash, model, chunks, total_tokens, expires_at)
    VALUES (?, ?, ?, ?, ?)
  `).run(hash, model, JSON.stringify(chunks), totalTokens || 0, expiresAt);
}

// Replay cached chunks as SSE stream
export function replayCachedStream(cachedData, model) {
  const encoder = new TextEncoder();
  const chunks = cachedData.chunks;

  const stream = new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(`data: ${JSON.stringify(chunk)}\n\n`));
      }
      controller.enqueue(encoder.encode("data: [DONE]\n\n"));
      controller.close();
    },
  });

  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Cache": "STREAM-HIT",
    },
  });
}

// Create a capturing stream that stores chunks while forwarding them
export function createCapturingStream(upstreamBody, model, messages, params) {
  const reader = upstreamBody.getReader();
  const decoder = new TextDecoder();
  const encoder = new TextEncoder();
  const capturedChunks = [];
  let buffer = "";

  const stream = new ReadableStream({
    async pull(controller) {
      try {
        const { done, value } = await reader.read();
        if (done) {
          // Process remaining buffer
          if (buffer.trim()) {
            controller.enqueue(encoder.encode(buffer));
          }
          // Store captured chunks
          if (capturedChunks.length > 0) {
            setCachedStream(model, messages, params, capturedChunks, 0);
          }
          controller.close();
          return;
        }

        const text = decoder.decode(value, { stream: true });
        controller.enqueue(value);

        // Capture SSE chunks for caching
        buffer += text;
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          if (line.startsWith("data: ") && line !== "data: [DONE]") {
            try {
              const parsed = JSON.parse(line.slice(6));
              capturedChunks.push(parsed);
            } catch {
              // skip unparseable
            }
          }
        }
      } catch (error) {
        controller.error(error);
      }
    },
    cancel() {
      reader.cancel();
    },
  });

  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Cache": "MISS",
    },
  });
}

// Cleanup expired entries
export function cleanupStreamCache() {
  ensureInit();
  const db = getDb();
  db.prepare("DELETE FROM stream_response_cache WHERE expires_at < datetime('now')").run();
}

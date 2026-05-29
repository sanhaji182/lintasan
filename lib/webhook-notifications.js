// Advanced Webhook Notification System for Lintasan LLM Proxy
import { getDb } from "./db/index.js";
import { randomUUID, createHmac } from "node:crypto";

// ─── Event Types ────────────────────────────────────────────────────────────
export const EVENT_TYPES = [
  "budget_warning",       // 80% of token budget used
  "budget_exhausted",     // Token budget fully consumed
  "provider_down",        // Circuit breaker opened
  "provider_recovered",   // Circuit breaker closed / provider back
  "anomaly_detected",     // Sudden spike in errors or latency
  "high_latency",         // p95 latency exceeds threshold
  "cache_hit_rate_low",   // Cache hit rate below threshold
];

// ─── In-Memory State ────────────────────────────────────────────────────────
const notificationHistory = []; // last 100 notifications
const MAX_HISTORY = 100;

// Rolling stats for anomaly detection
const rollingStats = {
  errors: [],        // { ts, count } per minute
  latencies: [],     // { ts, p95 } per minute
  windowMs: 600_000, // 10 minute rolling window
  errorBaseline: 0,
  latencyBaseline: 0,
};

// ─── Database Initialization ────────────────────────────────────────────────
let tableInitialized = false;

function ensureTable() {
  if (tableInitialized) return;
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS webhooks (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      url TEXT NOT NULL,
      secret TEXT NOT NULL,
      events TEXT NOT NULL DEFAULT '[]',
      is_active INTEGER DEFAULT 1,
      created_at TEXT DEFAULT (datetime('now'))
    )
  `);
  tableInitialized = true;
}

// ─── Webhook Registry ───────────────────────────────────────────────────────

/**
 * Register a new webhook endpoint.
 * @param {{ name: string, url: string, secret?: string, events?: string[] }} opts
 * @returns {object} The created webhook record
 */
export function registerWebhook({ name, url, secret, events }) {
  ensureTable();
  const db = getDb();
  const id = randomUUID();
  const webhookSecret = secret || randomUUID().replace(/-/g, "");
  const eventList = Array.isArray(events) ? events.filter(e => EVENT_TYPES.includes(e)) : [...EVENT_TYPES];

  db.prepare(
    "INSERT INTO webhooks (id, name, url, secret, events, is_active) VALUES (?, ?, ?, ?, ?, 1)"
  ).run(id, name, url, webhookSecret, JSON.stringify(eventList));

  return { id, name, url, secret: webhookSecret, events: eventList, is_active: 1 };
}

/**
 * Remove a webhook by ID.
 * @param {string} id
 * @returns {boolean}
 */
export function removeWebhook(id) {
  ensureTable();
  const db = getDb();
  const result = db.prepare("DELETE FROM webhooks WHERE id = ?").run(id);
  return result.changes > 0;
}

/**
 * Update an existing webhook.
 * @param {string} id
 * @param {object} updates - { name?, url?, secret?, events?, is_active? }
 * @returns {object|null}
 */
export function updateWebhook(id, updates) {
  ensureTable();
  const db = getDb();
  const existing = db.prepare("SELECT * FROM webhooks WHERE id = ?").get(id);
  if (!existing) return null;

  const fields = [];
  const values = [];
  if (updates.name !== undefined) { fields.push("name = ?"); values.push(updates.name); }
  if (updates.url !== undefined) { fields.push("url = ?"); values.push(updates.url); }
  if (updates.secret !== undefined) { fields.push("secret = ?"); values.push(updates.secret); }
  if (updates.events !== undefined) {
    const filtered = Array.isArray(updates.events) ? updates.events.filter(e => EVENT_TYPES.includes(e)) : [];
    fields.push("events = ?");
    values.push(JSON.stringify(filtered));
  }
  if (updates.is_active !== undefined) { fields.push("is_active = ?"); values.push(updates.is_active ? 1 : 0); }

  if (fields.length === 0) return getWebhookById(id);

  values.push(id);
  db.prepare(`UPDATE webhooks SET ${fields.join(", ")} WHERE id = ?`).run(...values);
  return getWebhookById(id);
}

/**
 * Get a single webhook by ID.
 */
export function getWebhookById(id) {
  ensureTable();
  const db = getDb();
  const row = db.prepare("SELECT * FROM webhooks WHERE id = ?").get(id);
  if (!row) return null;
  return { ...row, events: JSON.parse(row.events || "[]") };
}

/**
 * List all registered webhooks.
 * @returns {object[]}
 */
export function listWebhooks() {
  ensureTable();
  const db = getDb();
  const rows = db.prepare("SELECT id, name, url, events, is_active, created_at FROM webhooks ORDER BY created_at DESC").all();
  return rows.map(r => ({ ...r, events: JSON.parse(r.events || "[]") }));
}

// ─── Notification Delivery ──────────────────────────────────────────────────

/**
 * Sign a payload body with HMAC-SHA256.
 */
function signPayload(body, secret) {
  return createHmac("sha256", secret).update(body).digest("hex");
}

/**
 * Deliver a notification to a single webhook with retry logic.
 * 3 attempts, exponential backoff (1s, 2s, 4s).
 */
async function deliverNotification(webhook, payload) {
  const body = JSON.stringify(payload);
  const signature = signPayload(body, webhook.secret);

  const record = {
    id: randomUUID(),
    webhook_id: webhook.id,
    webhook_name: webhook.name,
    event: payload.event,
    timestamp: payload.timestamp,
    attempts: 0,
    status: "pending",
    response_code: null,
    error: null,
    delivered_at: null,
  };

  const delays = [1000, 2000, 4000]; // exponential backoff

  for (let attempt = 0; attempt < 3; attempt++) {
    record.attempts = attempt + 1;
    try {
      const response = await fetch(webhook.url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Webhook-Signature": signature,
          "X-Webhook-Event": payload.event,
          "X-Webhook-Id": record.id,
        },
        body,
        signal: AbortSignal.timeout(10000),
      });

      record.response_code = response.status;

      if (response.ok) {
        record.status = "delivered";
        record.delivered_at = new Date().toISOString();
        break;
      } else {
        record.status = "failed";
        record.error = `HTTP ${response.status}`;
      }
    } catch (err) {
      record.status = "failed";
      record.error = err.message || "Network error";
    }

    // Wait before retry (skip wait on last attempt)
    if (attempt < 2) {
      await new Promise(resolve => setTimeout(resolve, delays[attempt]));
    }
  }

  // Store in history
  addToHistory(record);
  return record;
}

/**
 * Add a notification record to in-memory history (capped at MAX_HISTORY).
 */
function addToHistory(record) {
  notificationHistory.unshift(record);
  if (notificationHistory.length > MAX_HISTORY) {
    notificationHistory.length = MAX_HISTORY;
  }
}

// ─── Event Triggering ───────────────────────────────────────────────────────

/**
 * Trigger an event and deliver to all subscribed active webhooks.
 * @param {string} type - One of EVENT_TYPES
 * @param {object} data - Event-specific data
 * @returns {Promise<object[]>} Delivery results
 */
export async function triggerEvent(type, data = {}) {
  if (!EVENT_TYPES.includes(type)) {
    return [];
  }

  ensureTable();
  const db = getDb();
  const webhooks = db.prepare("SELECT * FROM webhooks WHERE is_active = 1").all();

  const payload = {
    event: type,
    timestamp: new Date().toISOString(),
    data,
  };

  const results = [];

  for (const row of webhooks) {
    const events = JSON.parse(row.events || "[]");
    if (!events.includes(type)) continue;

    // Deliver asynchronously but collect results
    const result = await deliverNotification(row, { ...payload, signature: undefined });
    results.push(result);
  }

  return results;
}

/**
 * Send a test notification to a specific webhook.
 * @param {string} id - Webhook ID
 * @returns {Promise<object|null>}
 */
export async function testWebhook(id) {
  ensureTable();
  const db = getDb();
  const row = db.prepare("SELECT * FROM webhooks WHERE id = ?").get(id);
  if (!row) return null;

  const payload = {
    event: "test",
    timestamp: new Date().toISOString(),
    data: {
      message: "This is a test notification from Lintasan LLM Proxy",
      webhook_id: id,
      webhook_name: row.name,
    },
  };

  return await deliverNotification(row, payload);
}

// ─── Notification History ───────────────────────────────────────────────────

/**
 * Get notification history (last 100 entries).
 * @param {number} limit
 * @returns {object[]}
 */
export function getNotificationHistory(limit = 50) {
  return notificationHistory.slice(0, limit);
}

// ─── Anomaly Detection ──────────────────────────────────────────────────────

/**
 * Record an error data point for anomaly detection.
 * Call this on each request error.
 */
export function recordError() {
  const now = Date.now();
  rollingStats.errors.push({ ts: now, count: 1 });
  pruneOldEntries();
  checkErrorAnomaly();
}

/**
 * Record a latency data point for anomaly detection.
 * @param {number} latencyMs
 */
export function recordLatency(latencyMs) {
  const now = Date.now();
  rollingStats.latencies.push({ ts: now, value: latencyMs });
  pruneOldEntries();
  checkLatencyAnomaly();
}

function pruneOldEntries() {
  const cutoff = Date.now() - rollingStats.windowMs;
  rollingStats.errors = rollingStats.errors.filter(e => e.ts > cutoff);
  rollingStats.latencies = rollingStats.latencies.filter(e => e.ts > cutoff);
}

// Cooldown to avoid spamming anomaly alerts
let lastAnomalyAlert = 0;
let lastLatencyAlert = 0;
const ANOMALY_COOLDOWN_MS = 300_000; // 5 minutes

function checkErrorAnomaly() {
  const now = Date.now();
  if (now - lastAnomalyAlert < ANOMALY_COOLDOWN_MS) return;

  const recentWindow = 60_000; // last 1 minute
  const recentErrors = rollingStats.errors.filter(e => e.ts > now - recentWindow).length;
  const totalErrors = rollingStats.errors.length;
  const windowMinutes = rollingStats.windowMs / 60_000;
  const avgPerMinute = totalErrors / windowMinutes;

  // Alert if recent error rate > 2x the rolling average
  if (avgPerMinute > 0 && recentErrors > avgPerMinute * 2 && recentErrors >= 3) {
    lastAnomalyAlert = now;
    triggerEvent("anomaly_detected", {
      type: "error_spike",
      recent_errors_per_minute: recentErrors,
      average_errors_per_minute: Math.round(avgPerMinute * 100) / 100,
      multiplier: Math.round((recentErrors / avgPerMinute) * 100) / 100,
    });
  }
}

function checkLatencyAnomaly() {
  const now = Date.now();
  if (now - lastLatencyAlert < ANOMALY_COOLDOWN_MS) return;

  const recentWindow = 60_000;
  const recentLatencies = rollingStats.latencies.filter(e => e.ts > now - recentWindow).map(e => e.value);
  const allLatencies = rollingStats.latencies.map(e => e.value);

  if (recentLatencies.length < 3 || allLatencies.length < 5) return;

  const recentP95 = percentile(recentLatencies, 95);
  const overallAvg = allLatencies.reduce((a, b) => a + b, 0) / allLatencies.length;

  // Alert if recent p95 > 3x the rolling average
  if (overallAvg > 0 && recentP95 > overallAvg * 3) {
    lastLatencyAlert = now;
    triggerEvent("high_latency", {
      p95_ms: Math.round(recentP95),
      average_ms: Math.round(overallAvg),
      multiplier: Math.round((recentP95 / overallAvg) * 100) / 100,
    });
  }
}

/**
 * Calculate percentile from an array of numbers.
 */
function percentile(arr, p) {
  const sorted = [...arr].sort((a, b) => a - b);
  const idx = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, idx)];
}

// ─── Convenience Triggers ───────────────────────────────────────────────────

/**
 * Trigger budget warning (80% used).
 */
export function triggerBudgetWarning(data) {
  return triggerEvent("budget_warning", data);
}

/**
 * Trigger budget exhausted.
 */
export function triggerBudgetExhausted(data) {
  return triggerEvent("budget_exhausted", data);
}

/**
 * Trigger provider down (circuit breaker open).
 */
export function triggerProviderDown(data) {
  return triggerEvent("provider_down", data);
}

/**
 * Trigger provider recovered.
 */
export function triggerProviderRecovered(data) {
  return triggerEvent("provider_recovered", data);
}

/**
 * Trigger cache hit rate low.
 */
export function triggerCacheHitRateLow(data) {
  return triggerEvent("cache_hit_rate_low", data);
}

// ─── Legacy Compatibility ───────────────────────────────────────────────────
// Re-export trackError from original webhooks.js behavior, enhanced with anomaly detection

const errorCounts = new Map();
const ALERT_THRESHOLD = 5;
const ALERT_WINDOW_MS = 300_000;
const ALERT_COOLDOWN_MS_LEGACY = 600_000;

export function trackError(providerId, error, model) {
  const now = Date.now();
  let entry = errorCounts.get(providerId);

  if (!entry || now - entry.windowStart > ALERT_WINDOW_MS) {
    entry = { count: 0, windowStart: now, lastAlert: entry?.lastAlert || 0 };
    errorCounts.set(providerId, entry);
  }

  entry.count++;

  // Feed into anomaly detection
  recordError();

  // Legacy threshold alert via new system
  if (entry.count >= ALERT_THRESHOLD && now - entry.lastAlert > ALERT_COOLDOWN_MS_LEGACY) {
    entry.lastAlert = now;
    triggerEvent("provider_down", {
      provider: providerId,
      error_count: entry.count,
      last_error: typeof error === "string" ? error.slice(0, 200) : "Unknown",
      model: model || "unknown",
    });
  }
}

export function getErrorStats() {
  const stats = {};
  const now = Date.now();
  for (const [provider, entry] of errorCounts) {
    if (now - entry.windowStart < ALERT_WINDOW_MS) {
      stats[provider] = { errors: entry.count, windowStart: new Date(entry.windowStart).toISOString() };
    }
  }
  return stats;
}

// Webhook notifications - alert on provider errors
import { getSetting, getDb } from "./db/index.js";

// In-memory error counter for alerting
const errorCounts = new Map(); // provider -> { count, lastAlert }
const ALERT_THRESHOLD = 5; // Alert after 5 errors in window
const ALERT_WINDOW_MS = 300_000; // 5 minute window
const ALERT_COOLDOWN_MS = 600_000; // Don't re-alert for 10 min

export function getWebhookUrl() {
  return getSetting("webhook_url", "");
}

export function setWebhookUrl(url) {
  const db = getDb();
  db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("webhook_url", url);
}

export function trackError(providerId, error, model) {
  const now = Date.now();
  let entry = errorCounts.get(providerId);

  if (!entry || now - entry.windowStart > ALERT_WINDOW_MS) {
    entry = { count: 0, windowStart: now, lastAlert: entry?.lastAlert || 0 };
    errorCounts.set(providerId, entry);
  }

  entry.count++;

  // Check if we should alert
  if (entry.count >= ALERT_THRESHOLD && now - entry.lastAlert > ALERT_COOLDOWN_MS) {
    entry.lastAlert = now;
    sendAlert(providerId, entry.count, error, model);
  }
}

async function sendAlert(providerId, errorCount, lastError, model) {
  const webhookUrl = getWebhookUrl();
  if (!webhookUrl) return;

  const payload = {
    event: "provider_error_threshold",
    provider: providerId,
    error_count: errorCount,
    window_minutes: ALERT_WINDOW_MS / 60000,
    last_error: typeof lastError === "string" ? lastError.slice(0, 200) : "Unknown",
    model: model || "unknown",
    timestamp: new Date().toISOString(),
    source: "lintasan",
  };

  try {
    await fetch(webhookUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(5000),
    });
  } catch (e) {
    // Webhook delivery failure is non-fatal
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

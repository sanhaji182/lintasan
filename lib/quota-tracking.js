// Quota Tracking — real-time token counts per connection with reset countdowns
// Tracks daily/monthly usage per connection and per key
// Alerts when approaching limits, auto-triggers fallback when exhausted

import { getDb, getSetting } from "./db/index.js";

// In-memory counters (fast, persisted periodically)
const usageCounters = new Map(); // connection_id -> { tokens_today, tokens_month, requests_today, last_reset_day, last_reset_month }

// Initialize or get counter for a connection
function getCounter(connectionId) {
  if (!usageCounters.has(connectionId)) {
    // Try to load from DB
    const db = getDb();
    try {
      db.exec(`CREATE TABLE IF NOT EXISTS quota_usage (
        connection_id TEXT PRIMARY KEY,
        tokens_today INTEGER DEFAULT 0,
        tokens_month INTEGER DEFAULT 0,
        requests_today INTEGER DEFAULT 0,
        requests_month INTEGER DEFAULT 0,
        last_reset_day TEXT,
        last_reset_month TEXT,
        updated_at TEXT DEFAULT (datetime('now'))
      )`);
      const row = db.prepare("SELECT * FROM quota_usage WHERE connection_id = ?").get(connectionId);
      if (row) {
        usageCounters.set(connectionId, {
          tokens_today: row.tokens_today,
          tokens_month: row.tokens_month,
          requests_today: row.requests_today,
          requests_month: row.requests_month,
          last_reset_day: row.last_reset_day,
          last_reset_month: row.last_reset_month,
        });
      }
    } catch {}
  }

  if (!usageCounters.has(connectionId)) {
    usageCounters.set(connectionId, {
      tokens_today: 0,
      tokens_month: 0,
      requests_today: 0,
      requests_month: 0,
      last_reset_day: new Date().toISOString().slice(0, 10),
      last_reset_month: new Date().toISOString().slice(0, 7),
    });
  }

  const counter = usageCounters.get(connectionId);

  // Auto-reset on new day/month
  const today = new Date().toISOString().slice(0, 10);
  const thisMonth = new Date().toISOString().slice(0, 7);

  if (counter.last_reset_day !== today) {
    counter.tokens_today = 0;
    counter.requests_today = 0;
    counter.last_reset_day = today;
  }
  if (counter.last_reset_month !== thisMonth) {
    counter.tokens_month = 0;
    counter.requests_month = 0;
    counter.last_reset_month = thisMonth;
  }

  return counter;
}

// Record usage after a request
export function recordQuotaUsage(connectionId, inputTokens = 0, outputTokens = 0) {
  const counter = getCounter(connectionId);
  const totalTokens = inputTokens + outputTokens;

  counter.tokens_today += totalTokens;
  counter.tokens_month += totalTokens;
  counter.requests_today += 1;
  counter.requests_month += 1;

  // Persist to DB periodically (every 10 requests)
  if (counter.requests_today % 10 === 0) {
    persistCounter(connectionId, counter);
  }
}

// Persist counter to DB
function persistCounter(connectionId, counter) {
  try {
    const db = getDb();
    db.prepare(`INSERT OR REPLACE INTO quota_usage 
      (connection_id, tokens_today, tokens_month, requests_today, requests_month, last_reset_day, last_reset_month, updated_at) 
      VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`)
      .run(connectionId, counter.tokens_today, counter.tokens_month, counter.requests_today, counter.requests_month, counter.last_reset_day, counter.last_reset_month);
  } catch {}
}

// Get quota status for a connection
export function getQuotaStatus(connectionId) {
  const counter = getCounter(connectionId);
  const limits = getQuotaLimits(connectionId);

  const dailyPercent = limits.daily_tokens > 0 ? (counter.tokens_today / limits.daily_tokens) * 100 : 0;
  const monthlyPercent = limits.monthly_tokens > 0 ? (counter.tokens_month / limits.monthly_tokens) * 100 : 0;

  return {
    connection_id: connectionId,
    tokens_today: counter.tokens_today,
    tokens_month: counter.tokens_month,
    requests_today: counter.requests_today,
    requests_month: counter.requests_month,
    limits,
    daily_percent: Math.round(dailyPercent * 10) / 10,
    monthly_percent: Math.round(monthlyPercent * 10) / 10,
    exhausted: (limits.daily_tokens > 0 && counter.tokens_today >= limits.daily_tokens) ||
               (limits.monthly_tokens > 0 && counter.tokens_month >= limits.monthly_tokens),
    warning: dailyPercent >= 80 || monthlyPercent >= 80,
    reset_daily: getNextDailyReset(),
    reset_monthly: getNextMonthlyReset(),
  };
}

// Get all connections quota status
export function getAllQuotaStatus() {
  const statuses = [];
  for (const [connId, counter] of usageCounters) {
    statuses.push(getQuotaStatus(connId));
  }
  return statuses;
}

// Check if a connection's quota is exhausted
export function isQuotaExhausted(connectionId) {
  const status = getQuotaStatus(connectionId);
  return status.exhausted;
}

// Get quota limits for a connection (from settings)
function getQuotaLimits(connectionId) {
  try {
    const limitsJson = getSetting("quota_limits", "{}");
    const limits = JSON.parse(limitsJson);
    return limits[connectionId] || { daily_tokens: 0, monthly_tokens: 0 }; // 0 = unlimited
  } catch {
    return { daily_tokens: 0, monthly_tokens: 0 };
  }
}

// Set quota limits for a connection
export function setQuotaLimits(connectionId, dailyTokens, monthlyTokens) {
  try {
    const db = getDb();
    const limitsJson = getSetting("quota_limits", "{}");
    const limits = JSON.parse(limitsJson);
    limits[connectionId] = { daily_tokens: dailyTokens || 0, monthly_tokens: monthlyTokens || 0 };
    db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("quota_limits", JSON.stringify(limits));
  } catch {}
}

// Helper: next daily reset time
function getNextDailyReset() {
  const now = new Date();
  const tomorrow = new Date(now);
  tomorrow.setUTCDate(tomorrow.getUTCDate() + 1);
  tomorrow.setUTCHours(0, 0, 0, 0);
  return tomorrow.toISOString();
}

// Helper: next monthly reset time
function getNextMonthlyReset() {
  const now = new Date();
  const nextMonth = new Date(now.getUTCFullYear(), now.getUTCMonth() + 1, 1);
  return nextMonth.toISOString();
}

// Flush all counters to DB (call on shutdown)
export function flushAllCounters() {
  for (const [connId, counter] of usageCounters) {
    persistCounter(connId, counter);
  }
}

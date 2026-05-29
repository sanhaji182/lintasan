// Multi-user API keys with per-key quota
import { getDb } from "./db/index.js";
import { v4 as uuidv4 } from "uuid";

// Ensure table exists
function initApiKeys() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS api_keys (
      id TEXT PRIMARY KEY,
      key TEXT UNIQUE NOT NULL,
      name TEXT NOT NULL,
      is_active INTEGER DEFAULT 1,
      quota_rpm INTEGER DEFAULT 60,
      quota_daily INTEGER DEFAULT 1000,
      used_today INTEGER DEFAULT 0,
      last_reset TEXT DEFAULT (date('now')),
      created_at TEXT DEFAULT (datetime('now'))
    );
  `);
}

export function createApiKey({ name, quotaRpm = 60, quotaDaily = 1000 }) {
  initApiKeys();
  const db = getDb();
  const id = uuidv4();
  const key = "sr-" + uuidv4().replace(/-/g, "").slice(0, 32);
  db.prepare(
    "INSERT INTO api_keys (id, key, name, quota_rpm, quota_daily) VALUES (?, ?, ?, ?, ?)"
  ).run(id, key, name, quotaRpm, quotaDaily);
  return { id, key, name, quota_rpm: quotaRpm, quota_daily: quotaDaily, is_active: 1 };
}

export function listApiKeys() {
  initApiKeys();
  const db = getDb();
  return db.prepare("SELECT id, key, name, is_active, quota_rpm, quota_daily, used_today, created_at FROM api_keys ORDER BY created_at DESC").all();
}

export function validateUserApiKey(key) {
  initApiKeys();
  const db = getDb();

  // Reset daily counters if new day
  db.prepare("UPDATE api_keys SET used_today = 0, last_reset = date('now') WHERE last_reset < date('now')").run();

  const row = db.prepare("SELECT * FROM api_keys WHERE key = ? AND is_active = 1").get(key);
  if (!row) return { valid: false };

  // Check daily quota
  if (row.used_today >= row.quota_daily) {
    return { valid: false, reason: "Daily quota exceeded (" + row.quota_daily + " requests)" };
  }

  // Increment usage
  db.prepare("UPDATE api_keys SET used_today = used_today + 1 WHERE id = ?").run(row.id);

  return { valid: true, keyId: row.id, name: row.name, quotaRpm: row.quota_rpm, remaining: row.quota_daily - row.used_today - 1 };
}

export function deleteApiKey(id) {
  initApiKeys();
  const db = getDb();
  db.prepare("DELETE FROM api_keys WHERE id = ?").run(id);
}

export function toggleApiKey(id, active) {
  initApiKeys();
  const db = getDb();
  db.prepare("UPDATE api_keys SET is_active = ? WHERE id = ?").run(active ? 1 : 0, id);
}

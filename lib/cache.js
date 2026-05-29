// Response Cache - SQLite based
import { getDb } from "./db/index.js";
import crypto from "crypto";

// Ensure cache table exists
function initCache() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS response_cache (
      hash TEXT PRIMARY KEY,
      provider TEXT NOT NULL,
      model TEXT NOT NULL,
      request_body TEXT NOT NULL,
      response_body TEXT NOT NULL,
      input_tokens INTEGER DEFAULT 0,
      output_tokens INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now')),
      expires_at TEXT NOT NULL,
      hit_count INTEGER DEFAULT 0
    );
    CREATE INDEX IF NOT EXISTS idx_cache_expires ON response_cache(expires_at);
    CREATE INDEX IF NOT EXISTS idx_cache_model ON response_cache(model);
  `);
}

// Generate cache key from request
export function getCacheKey(model, messages, params = {}) {
  const payload = JSON.stringify({
    model,
    messages,
    temperature: params.temperature,
    max_tokens: params.max_tokens,
    top_p: params.top_p,
  });
  return crypto.createHash("sha256").update(payload).digest("hex");
}

// Get cached response
export function getCachedResponse(hash) {
  try {
    initCache();
    const db = getDb();
    const now = new Date().toISOString();

    const row = db.prepare(
      "SELECT * FROM response_cache WHERE hash = ? AND expires_at > ?"
    ).get(hash, now);

    if (row) {
      // Increment hit count
      db.prepare("UPDATE response_cache SET hit_count = hit_count + 1 WHERE hash = ?").run(hash);
      return JSON.parse(row.response_body);
    }
    return null;
  } catch (e) {
    return null;
  }
}

// Store response in cache
export function setCachedResponse(hash, { provider, model, requestBody, responseBody, inputTokens, outputTokens, ttlSeconds }) {
  try {
    initCache();
    const db = getDb();
    const expiresAt = new Date(Date.now() + ttlSeconds * 1000).toISOString();

    db.prepare(
      "INSERT OR REPLACE INTO response_cache (hash, provider, model, request_body, response_body, input_tokens, output_tokens, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
    ).run(
      hash, provider, model,
      JSON.stringify(requestBody),
      JSON.stringify(responseBody),
      inputTokens || 0, outputTokens || 0, expiresAt
    );
  } catch (e) {
    // Cache write failure is non-fatal
  }
}

// Get cache stats
export function getCacheStats() {
  try {
    initCache();
    const db = getDb();
    const now = new Date().toISOString();

    const total = db.prepare("SELECT COUNT(*) as count FROM response_cache").get();
    const active = db.prepare("SELECT COUNT(*) as count FROM response_cache WHERE expires_at > ?").get(now);
    const hits = db.prepare("SELECT SUM(hit_count) as total FROM response_cache").get();
    const size = db.prepare("SELECT SUM(LENGTH(response_body)) as bytes FROM response_cache WHERE expires_at > ?").get(now);

    return {
      totalEntries: total.count,
      activeEntries: active.count,
      totalHits: hits.total || 0,
      cacheSizeBytes: size.bytes || 0,
      cacheSizeMB: ((size.bytes || 0) / 1024 / 1024).toFixed(2),
    };
  } catch (e) {
    return { totalEntries: 0, activeEntries: 0, totalHits: 0, cacheSizeBytes: 0, cacheSizeMB: "0" };
  }
}

// Clear expired entries
export function clearExpiredCache() {
  try {
    initCache();
    const db = getDb();
    const now = new Date().toISOString();
    const result = db.prepare("DELETE FROM response_cache WHERE expires_at <= ?").run(now);
    return result.changes;
  } catch (e) {
    return 0;
  }
}

// Clear all cache
export function clearAllCache() {
  try {
    initCache();
    const db = getDb();
    const result = db.prepare("DELETE FROM response_cache").run();
    return result.changes;
  } catch (e) {
    return 0;
  }
}

// Check if caching is enabled
export function isCacheEnabled() {
  const db = getDb();
  const row = db.prepare("SELECT value FROM settings WHERE key = 'cache_enabled'").get();
  return row ? row.value === "true" : true; // Enabled by default
}

// Get cache TTL in seconds
export function getCacheTTL() {
  const db = getDb();
  const row = db.prepare("SELECT value FROM settings WHERE key = 'cache_ttl'").get();
  return row ? parseInt(row.value) : 3600; // Default 1 hour
}

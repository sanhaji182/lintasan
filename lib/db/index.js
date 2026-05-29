import Database from "better-sqlite3";
import path from "path";
import { v4 as uuidv4 } from "uuid";

const DB_PATH = path.join(process.cwd(), "data", "lintasan.db");

let db;

export function getDb() {
  if (!db) {
    db = new Database(DB_PATH);
    db.pragma("journal_mode = WAL");
    db.pragma("foreign_keys = ON");
    initTables(db);
  }
  return db;
}

function initTables(db) {
  db.exec(`
    CREATE TABLE IF NOT EXISTS connections (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      base_url TEXT NOT NULL,
      api_key TEXT NOT NULL,
      format TEXT NOT NULL DEFAULT 'openai',
      chat_path TEXT NOT NULL DEFAULT '/v1/chat/completions',
      models_path TEXT DEFAULT '/v1/models',
      auth_header TEXT DEFAULT 'Authorization',
      auth_prefix TEXT DEFAULT 'Bearer ',
      extra_headers TEXT DEFAULT '{}',
      is_active INTEGER DEFAULT 1,
      priority INTEGER DEFAULT 0,
      last_sync TEXT,
      models_count INTEGER DEFAULT 0,
      created_at TEXT DEFAULT (datetime('now')),
      updated_at TEXT DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS discovered_models (
      id TEXT PRIMARY KEY,
      connection_id TEXT NOT NULL,
      model_id TEXT NOT NULL,
      model_name TEXT,
      owned_by TEXT,
      is_active INTEGER DEFAULT 1,
      discovered_at TEXT DEFAULT (datetime('now')),
      FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS request_logs (
      id TEXT PRIMARY KEY,
      connection_id TEXT,
      provider TEXT,
      model TEXT,
      combo_name TEXT,
      status INTEGER,
      input_tokens INTEGER DEFAULT 0,
      output_tokens INTEGER DEFAULT 0,
      latency_ms INTEGER DEFAULT 0,
      cached INTEGER DEFAULT 0,
      error TEXT,
      created_at TEXT DEFAULT (datetime('now'))
    );

    CREATE TABLE IF NOT EXISTS settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );

    CREATE INDEX IF NOT EXISTS idx_discovered_models_connection ON discovered_models(connection_id);
    CREATE INDEX IF NOT EXISTS idx_discovered_models_model ON discovered_models(model_id);
    CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at);
  `);

  // Migration: add is_active column to discovered_models if missing
  try {
    const cols = db.prepare("PRAGMA table_info(discovered_models)").all();
    if (!cols.find(c => c.name === "is_active")) {
      db.exec("ALTER TABLE discovered_models ADD COLUMN is_active INTEGER DEFAULT 1");
    }
  } catch (e) { /* ignore */ }

  // Migration: add combo_name and cached columns to request_logs
  try {
    const logCols = db.prepare("PRAGMA table_info(request_logs)").all();
    if (!logCols.find(c => c.name === "combo_name")) {
      db.exec("ALTER TABLE request_logs ADD COLUMN combo_name TEXT");
    }
    if (!logCols.find(c => c.name === "cached")) {
      db.exec("ALTER TABLE request_logs ADD COLUMN cached INTEGER DEFAULT 0");
    }
  } catch (e) { /* ignore */ }
}

// Connection CRUD
export function createConnection({ name, baseUrl, apiKey, format = "openai", chatPath = "/v1/chat/completions", modelsPath = "/v1/models", authHeader = "Authorization", authPrefix = "Bearer ", extraHeaders = "{}", priority = 0 }) {
  const db = getDb();
  const id = uuidv4();
  db.prepare(
    "INSERT INTO connections (id, name, base_url, api_key, format, chat_path, models_path, auth_header, auth_prefix, extra_headers, priority) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
  ).run(id, name, baseUrl, apiKey, format, chatPath, modelsPath, authHeader, authPrefix, extraHeaders, priority);
  return { id, name, base_url: baseUrl, format, is_active: 1, priority, models_count: 0 };
}

export function listConnections() {
  const db = getDb();
  return db.prepare("SELECT id, name, base_url, format, chat_path, models_path, is_active, priority, last_sync, models_count, created_at FROM connections ORDER BY priority DESC, name ASC").all();
}

export function getConnection(id) {
  const db = getDb();
  return db.prepare("SELECT * FROM connections WHERE id = ?").get(id);
}

export function getActiveConnections() {
  const db = getDb();
  return db.prepare("SELECT * FROM connections WHERE is_active = 1 ORDER BY priority DESC").all();
}

export function updateConnection(id, updates) {
  const db = getDb();
  const allowedCols = { name: "name", baseUrl: "base_url", base_url: "base_url", apiKey: "api_key", api_key: "api_key", format: "format", chatPath: "chat_path", chat_path: "chat_path", modelsPath: "models_path", models_path: "models_path", authHeader: "auth_header", auth_header: "auth_header", authPrefix: "auth_prefix", auth_prefix: "auth_prefix", extraHeaders: "extra_headers", extra_headers: "extra_headers", isActive: "is_active", is_active: "is_active", priority: "priority", last_sync: "last_sync", models_count: "models_count" };
  const fields = [];
  const values = [];
  for (const [key, val] of Object.entries(updates)) {
    if (key === "id") continue;
    const col = allowedCols[key];
    if (!col) continue;
    fields.push(col + " = ?");
    values.push(val);
  }
  if (fields.length === 0) return;
  fields.push("updated_at = datetime('now')");
  values.push(id);
  db.prepare("UPDATE connections SET " + fields.join(", ") + " WHERE id = ?").run(...values);
}

export function deleteConnection(id) {
  const db = getDb();
  db.prepare("DELETE FROM discovered_models WHERE connection_id = ?").run(id);
  db.prepare("DELETE FROM connections WHERE id = ?").run(id);
}

// Discovered Models
export function saveDiscoveredModels(connectionId, models) {
  const db = getDb();
  db.prepare("DELETE FROM discovered_models WHERE connection_id = ?").run(connectionId);
  const insert = db.prepare("INSERT INTO discovered_models (id, connection_id, model_id, model_name, owned_by) VALUES (?, ?, ?, ?, ?)");
  const insertMany = db.transaction((items) => {
    for (const m of items) {
      insert.run(uuidv4(), connectionId, m.id, m.name || m.id, m.owned_by || "");
    }
  });
  insertMany(models);
  // Update connection models_count and last_sync
  db.prepare("UPDATE connections SET models_count = ?, last_sync = datetime('now'), updated_at = datetime('now') WHERE id = ?").run(models.length, connectionId);
}

export function listDiscoveredModels(connectionId = null) {
  const db = getDb();
  if (connectionId) {
    return db.prepare("SELECT dm.*, c.name as connection_name, c.base_url FROM discovered_models dm JOIN connections c ON dm.connection_id = c.id WHERE dm.connection_id = ? ORDER BY dm.model_id").all(connectionId);
  }
  return db.prepare("SELECT dm.*, c.name as connection_name, c.base_url, c.format, c.chat_path, c.auth_header, c.auth_prefix FROM discovered_models dm JOIN connections c ON dm.connection_id = c.id WHERE c.is_active = 1 AND dm.is_active = 1 ORDER BY c.priority DESC, dm.model_id").all();
}

export function findModelConnections(modelId) {
  const db = getDb();
  return db.prepare("SELECT dm.*, c.* FROM discovered_models dm JOIN connections c ON dm.connection_id = c.id WHERE dm.model_id = ? AND dm.is_active = 1 AND c.is_active = 1 ORDER BY c.priority DESC").all(modelId);
}

// Toggle a model's active state
export function toggleModelActive(connectionId, modelId, active) {
  const db = getDb();
  db.prepare("UPDATE discovered_models SET is_active = ? WHERE connection_id = ? AND model_id = ?").run(active ? 1 : 0, connectionId, modelId);
}

// Add a single model to a connection manually
export function addModelToConnection(connectionId, modelId, modelName, ownedBy = "") {
  const db = getDb();
  // Check if already exists
  const existing = db.prepare("SELECT id FROM discovered_models WHERE connection_id = ? AND model_id = ?").get(connectionId, modelId);
  if (existing) return { added: false, reason: "already_exists" };
  db.prepare("INSERT INTO discovered_models (id, connection_id, model_id, model_name, owned_by) VALUES (?, ?, ?, ?, ?)").run(uuidv4(), connectionId, modelId, modelName || modelId, ownedBy);
  // Update count
  const count = db.prepare("SELECT COUNT(*) as cnt FROM discovered_models WHERE connection_id = ?").get(connectionId);
  db.prepare("UPDATE connections SET models_count = ?, updated_at = datetime('now') WHERE id = ?").run(count.cnt, connectionId);
  return { added: true };
}

// Remove a single model from a connection
export function removeModelFromConnection(connectionId, modelId) {
  const db = getDb();
  db.prepare("DELETE FROM discovered_models WHERE connection_id = ? AND model_id = ?").run(connectionId, modelId);
  const count = db.prepare("SELECT COUNT(*) as cnt FROM discovered_models WHERE connection_id = ?").get(connectionId);
  db.prepare("UPDATE connections SET models_count = ?, updated_at = datetime('now') WHERE id = ?").run(count.cnt, connectionId);
}

// Legacy compat - listCustomProviders returns empty (removed)
export function listCustomProviders() {
  return [];
}

// Logging
export function logRequest({ connectionId, provider, model, status, inputTokens, outputTokens, latencyMs, error, comboName, cached }) {
  const db = getDb();
  const id = uuidv4();
  db.prepare(
    "INSERT INTO request_logs (id, connection_id, provider, model, combo_name, status, input_tokens, output_tokens, latency_ms, cached, error) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
  ).run(id, connectionId, provider, model, comboName || null, status, inputTokens || 0, outputTokens || 0, latencyMs || 0, cached || 0, error || null);
}

export function getRecentLogs(limit = 50) {
  const db = getDb();
  return db.prepare("SELECT * FROM request_logs ORDER BY created_at DESC LIMIT ?").all(limit);
}

// Settings
// In-memory settings cache (avoids 30+ SQLite reads per request)
let settingsCache = null;
let settingsCacheTime = 0;
const SETTINGS_CACHE_TTL = 5000; // 5 seconds

function loadAllSettings() {
  const now = Date.now();
  if (settingsCache && (now - settingsCacheTime) < SETTINGS_CACHE_TTL) {
    return settingsCache;
  }
  const db = getDb();
  const rows = db.prepare("SELECT key, value FROM settings").all();
  settingsCache = Object.fromEntries(rows.map(r => [r.key, r.value]));
  settingsCacheTime = now;
  return settingsCache;
}

export function getSetting(key, defaultValue = null) {
  const cache = loadAllSettings();
  return cache[key] !== undefined ? cache[key] : defaultValue;
}

export function setSetting(key, value) {
  const db = getDb();
  db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run(key, value);
  // Invalidate cache immediately on write
  settingsCache = null;
}

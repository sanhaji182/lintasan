import Database from "better-sqlite3";
import path from "path";
import { fileURLToPath } from "url";
import { randomUUID } from "crypto";
import fs from "fs";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const DATA_DIR = path.join(__dirname, "..", "data");
const DB_PATH = path.join(DATA_DIR, "lintasan.db");

// Ensure data directory exists
if (!fs.existsSync(DATA_DIR)) {
  fs.mkdirSync(DATA_DIR, { recursive: true });
}

const db = new Database(DB_PATH);
db.pragma("journal_mode = WAL");
db.pragma("foreign_keys = ON");

// Create tables
db.exec(`
  CREATE TABLE IF NOT EXISTS connections (
    id TEXT PRIMARY KEY,
    provider TEXT,
    name TEXT NOT NULL,
    base_url TEXT,
    api_key TEXT,
    format TEXT DEFAULT 'openai',
    chat_path TEXT DEFAULT '/v1/chat/completions',
    models_path TEXT DEFAULT '/v1/models',
    auth_header TEXT DEFAULT 'Authorization',
    auth_prefix TEXT DEFAULT 'Bearer',
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
    discovered_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
  );

  CREATE TABLE IF NOT EXISTS request_logs (
    id TEXT PRIMARY KEY,
    connection_id TEXT,
    provider TEXT,
    model TEXT,
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

  CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    key TEXT NOT NULL UNIQUE,
    is_active INTEGER DEFAULT 1,
    daily_limit INTEGER DEFAULT 0,
    monthly_limit INTEGER DEFAULT 0,
    daily_used INTEGER DEFAULT 0,
    monthly_used INTEGER DEFAULT 0,
    last_reset TEXT DEFAULT (datetime('now')),
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT DEFAULT 'viewer',
    is_active INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS teams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS team_members (
    team_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role TEXT DEFAULT 'member',
    PRIMARY KEY (team_id, user_id),
    FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
  );

  CREATE TABLE IF NOT EXISTS webhooks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    events TEXT DEFAULT '[]',
    secret TEXT,
    is_active INTEGER DEFAULT 1,
    last_triggered TEXT,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS semantic_cache (
    id TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    response TEXT NOT NULL,
    model TEXT,
    tokens_saved INTEGER DEFAULT 0,
    hit_count INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS response_cache (
    id TEXT PRIMARY KEY,
    cache_key TEXT NOT NULL UNIQUE,
    response TEXT NOT NULL,
    model TEXT,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS stream_response_cache (
    id TEXT PRIMARY KEY,
    cache_key TEXT NOT NULL UNIQUE,
    chunks TEXT NOT NULL,
    model TEXT,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS embedding_cache (
    id TEXT PRIMARY KEY,
    embedding TEXT NOT NULL,
    response TEXT NOT NULL,
    model TEXT,
    tokens_saved INTEGER DEFAULT 0,
    hit_count INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
  );

  CREATE INDEX IF NOT EXISTS idx_discovered_models_connection ON discovered_models(connection_id);
  CREATE INDEX IF NOT EXISTS idx_discovered_models_model ON discovered_models(model_id);
  CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at);
  CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs(model);
  CREATE INDEX IF NOT EXISTS idx_semantic_cache_fingerprint ON semantic_cache(fingerprint);
`);

// Seed default settings
const defaults = {
  cache_enabled: "true",
  semantic_cache_enabled: "true",
  embedding_cache_enabled: "false",
  stream_cache_enabled: "true",
  circuit_breaker_enabled: "true",
  coalescing_enabled: "true",
  rtk_enabled: "true",
  caveman_mode: "off",
  smart_tokens_enabled: "true",
  smart_tokens_force_inject: "false",
  prompt_enhancer_enabled: "true",
  web_search_enabled: "false",
  load_balance_strategy: "priority",
  semantic_threshold: "0.92",
  embedding_similarity_threshold: "0.85",
  embedding_model: "text-embedding-3-small",
};

const insertSetting = db.prepare(
  "INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)"
);

for (const [key, value] of Object.entries(defaults)) {
  insertSetting.run(key, value);
}

// Generate master key if not set
const masterKey = db.prepare("SELECT value FROM settings WHERE key = 'master_key'").get();
if (!masterKey) {
  const key = "sk-" + randomUUID().replace(/-/g, "");
  db.prepare("INSERT INTO settings (key, value) VALUES (?, ?)").run("master_key", key);
  console.log("🔑 Generated master API key:", key);
  console.log("   Save this key — you'll need it for API access.");
}

db.close();
console.log("✅ Database initialized at:", DB_PATH);
console.log("🚀 Run 'npm run dev' or 'npm run build && npm start' to launch Lintasan");

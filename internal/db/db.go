package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) Conn() *sql.DB {
	return d.conn
}

func (d *DB) migrate() error {
	// Drop old broken semantic_cache table if it exists (never functional, safe to drop)
	d.conn.Exec(`DROP TABLE IF EXISTS semantic_cache`)
	
	// Compatible with Node.js Lintasan schema
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS connections (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL DEFAULT '',
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
		)`,
		`CREATE TABLE IF NOT EXISTS discovered_models (
			id TEXT PRIMARY KEY,
			connection_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			model_name TEXT,
			owned_by TEXT,
			discovered_at TEXT DEFAULT (datetime('now')),
			is_active INTEGER DEFAULT 1,
			FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			connection_id TEXT,
			provider TEXT,
			model TEXT,
			status INTEGER,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			latency_ms INTEGER DEFAULT 0,
			error TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			combo_name TEXT,
			cached INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS embedding_cache (
			key TEXT PRIMARY KEY,
			value BLOB NOT NULL,
			model TEXT DEFAULT '',
			hits INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now')),
			expires_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS semantic_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			messages_hash TEXT NOT NULL,
			response TEXT NOT NULL,
			hits INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now')),
			expires_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_semantic_cache_model_expires ON semantic_cache(model, expires_at)`,
		`CREATE TABLE IF NOT EXISTS response_cache (
			hash TEXT PRIMARY KEY,
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL,
			request_body TEXT NOT NULL,
			response_body TEXT NOT NULL,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			hit_count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS stream_response_cache (
			hash TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			request_body TEXT NOT NULL,
			chunks TEXT NOT NULL,
			total_tokens INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			hit_count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_sessions (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			access_token TEXT DEFAULT '',
			refresh_token TEXT DEFAULT '',
			expires_at TEXT,
			status TEXT DEFAULT 'pending',
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS plugins (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			enabled INTEGER DEFAULT 1,
			priority INTEGER DEFAULT 100,
			code TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			action TEXT NOT NULL,
			actor TEXT DEFAULT '',
			resource TEXT DEFAULT '',
			details TEXT DEFAULT '{}',
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id TEXT PRIMARY KEY,
			webhook_id TEXT,
			event TEXT,
			status INTEGER DEFAULT 0,
			response TEXT DEFAULT '',
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_discovered_models_connection ON discovered_models(connection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_discovered_models_model ON discovered_models(model_id)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at)`,
	}

	for _, m := range migrations {
		if _, err := d.conn.Exec(m); err != nil {
			// Ignore "already exists" errors for indexes
			continue
		}
	}

	return nil
}

// GetSetting retrieves a setting value by key
func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting upserts a setting
func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		key, value, value,
	)
	return err
}

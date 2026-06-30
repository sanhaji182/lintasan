package db

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"

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

	// Connection pool ceiling. Default 1 (legacy serialized behaviour).
	// Override with LINTASAN_DB_MAX_CONNS for benchmarking / tuning. WAL mode
	// supports concurrent readers + a single writer, so >1 helps read-heavy load.
	maxConns := 1
	if v := os.Getenv("LINTASAN_DB_MAX_CONNS"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			maxConns = n
		}
	}
	conn.SetMaxOpenConns(maxConns)

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
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			updated_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS provider_presets (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			domain TEXT NOT NULL,
			base_url TEXT NOT NULL,
			format TEXT NOT NULL DEFAULT 'openai',
			key_label TEXT NOT NULL DEFAULT 'API Key',
			category TEXT NOT NULL DEFAULT 'foundation',
			is_builtin INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			updated_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS preset_categories (
			key TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			icon TEXT NOT NULL DEFAULT '📦',
			color TEXT NOT NULL DEFAULT '#8b5cf6',
			sort_order INTEGER DEFAULT 0,
			is_builtin INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			updated_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS discovered_models (
			id TEXT PRIMARY KEY,
			connection_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			model_name TEXT,
			owned_by TEXT,
			discovered_at TEXT DEFAULT (datetime('now', 'localtime')),
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
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			combo_name TEXT,
			cached INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS embedding_cache (
			key TEXT PRIMARY KEY,
			value BLOB NOT NULL,
			model TEXT DEFAULT '',
			hits INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			expires_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS semantic_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			messages_hash TEXT NOT NULL,
			response TEXT NOT NULL,
			hits INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
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
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			updated_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_sessions (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			access_token TEXT DEFAULT '',
			refresh_token TEXT DEFAULT '',
			expires_at TEXT,
			status TEXT DEFAULT 'pending',
			created_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS plugins (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			enabled INTEGER DEFAULT 1,
			priority INTEGER DEFAULT 100,
			code TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			action TEXT NOT NULL,
			actor TEXT DEFAULT '',
			resource TEXT DEFAULT '',
			details TEXT DEFAULT '{}',
			created_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id TEXT PRIMARY KEY,
			webhook_id TEXT,
			event TEXT,
			status INTEGER DEFAULT 0,
			response TEXT DEFAULT '',
			created_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_discovered_models_connection ON discovered_models(connection_id)`,
		`CREATE INDEX IF NOT EXISTS idx_discovered_models_model ON discovered_models(model_id)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at)`,
		// Perf (H1 root cause): /api/stats fired full-table scans for cache-rate and
		// avg-latency aggregates. Benchmark on 789k rows: cached=1 540ms->0ms (covering),
		// status=200 AVG 380ms->110ms (composite covering, skips row lookups).
		`CREATE INDEX IF NOT EXISTS idx_request_logs_cached ON request_logs(cached)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_status_lat ON request_logs(status, latency_ms)`,
		// P0 security: column to force rotation of bootstrap/seeded admin credentials.
		// Idempotent — fails with "duplicate column" on re-run, which the loop ignores.
		`ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE oauth_sessions ADD COLUMN pkce_verifier TEXT DEFAULT ''`,
		`ALTER TABLE oauth_sessions ADD COLUMN device_code TEXT DEFAULT ''`,
		`ALTER TABLE oauth_sessions ADD COLUMN flow_meta TEXT DEFAULT ''`,
		`ALTER TABLE connections ADD COLUMN oauth_provider TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE connections ADD COLUMN pool_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_connections_pool_id ON connections(pool_id)`,
		// P1: Experimental Provider Registry Persistence — stores lifecycle state,
		// admission reports, validation evidence, and descriptor snapshots for the
		// Experimental provider ecosystem. Credentials are NEVER stored (Invariant 3).
		`CREATE TABLE IF NOT EXISTS experimental_providers (
			name TEXT PRIMARY KEY,
			track TEXT NOT NULL DEFAULT 'experimental',
			state TEXT NOT NULL DEFAULT 'proposed',
			descriptor_json TEXT NOT NULL DEFAULT '{}',
			admission_report_json TEXT DEFAULT NULL,
			admitted_at TEXT DEFAULT NULL,
			activated_at TEXT DEFAULT NULL,
			deactivated_at TEXT DEFAULT NULL,
			validation_evidence TEXT NOT NULL DEFAULT '',
			risk_badge TEXT NOT NULL DEFAULT 'experimental',
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			updated_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
		// Credential Management V1: encrypted credential storage for Experimental
		// providers. Dashboard-managed credentials override environment variables.
		// Secrets are AES-256-GCM encrypted (key derived from master_key).
		`CREATE TABLE IF NOT EXISTS experimental_credentials (
			provider_name TEXT PRIMARY KEY,
			encrypted_value TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'dashboard',
			created_at TEXT DEFAULT (datetime('now', 'localtime')),
			updated_at TEXT DEFAULT (datetime('now', 'localtime'))
		)`,
	}

	for _, m := range migrations {
		if _, err := d.conn.Exec(m); err != nil {
			// Ignore "already exists" errors for indexes
			continue
		}
	}

	// One-time backfill: force any admin that existed BEFORE this security
	// migration (notably the legacy admin/admin123 seed) to rotate its password.
	// Guarded by a settings marker so it never re-flags an admin that has
	// already rotated. New admins are flagged at seed/create time, not here.
	if done, _ := d.GetSetting("migration_admin_pwd_rotation_v1"); done != "1" {
		d.conn.Exec("UPDATE users SET must_change_password = 1 WHERE role = 'admin'")
		d.SetSetting("migration_admin_pwd_rotation_v1", "1")
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

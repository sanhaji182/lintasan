package server

import (
	"database/sql"
	"fmt"
	"time"
)

// System reset — return an install to a known baseline.
//
// Two modes, because "reset" means two very different things in practice:
//
//	soft    — wipe operational data (connections, discovered models, logs,
//	          caches, routing config) but KEEP the operator's ability to log
//	          in and keep existing /v1 clients working. This is the "clean the
//	          slate, I'm reconfiguring" button.
//	factory — everything soft does, PLUS users, credentials and the secrets
//	          (master_key, jwt_secret). The install comes back up like a fresh
//	          one: a freshly seeded admin with a random must-change password.
//
// The distinction matters because master_key and jwt_secret are load-bearing:
// clearing them invalidates every issued API key and every dashboard session.
// That is the correct behavior for "like a new install" and the wrong one for
// "clear my test data", so the caller has to say which it means.

// ResetMode selects how deep a reset goes.
type ResetMode string

const (
	ResetModeSoft    ResetMode = "soft"
	ResetModeFactory ResetMode = "factory"
)

// operationalTables are cleared by BOTH modes. These hold data an install
// accumulates while running: providers the operator configured, models
// discovered from them, request history, caches, and delivery state.
//
// Order matters only for readability — SQLite has no FK cascade configured
// here, and every statement runs inside one transaction.
var operationalTables = []string{
	"connections",
	"discovered_models",
	"request_logs",
	"quota_usage",
	"audit_events",
	"response_cache",
	"stream_response_cache",
	"semantic_cache",
	"embedding_cache",
	"webhooks",
	"webhook_deliveries",
	"plugins",
	"experimental_providers",
	"experimental_credentials",
}

// identityTables are cleared ONLY by factory reset. Wiping these logs the
// operator out and invalidates every credential.
var identityTables = []string{
	"users",
	"oauth_sessions",
}

// operationalSettings are configuration keys both modes reset. These are
// routing/behaviour choices, not identity — clearing them returns the router
// to defaults without breaking authentication.
var operationalSettings = []string{
	"aliases",
	"combos",
	"fallback_chains",
	"teams",
	"webhooks",
	"plugins",
	"lb_strategy",
	"load_balancer_strategy",
	"thinking_mode",
	"api_keys",
}

// secretSettings are cleared ONLY by factory reset. Removing master_key and
// jwt_secret is what makes a factory reset genuinely "like new": every
// previously issued API key stops working and every dashboard session dies.
// They are regenerated on next start, exactly as on a first install.
var secretSettings = []string{
	"master_key",
	"jwt_secret",
	"key",
}

// ResetReport describes what a reset actually did. Counts are read BEFORE
// deletion so the operator can see the size of what they destroyed — and so
// the audit trail records it, since the audit table itself is cleared.
type ResetReport struct {
	Mode          ResetMode      `json:"mode"`
	BackupPath    string         `json:"backup_path"`
	RowsDeleted   map[string]int `json:"rows_deleted"`
	SettingsReset []string       `json:"settings_reset"`
	PresetsSeeded int            `json:"presets_seeded"`
	AdminUsername string         `json:"admin_username,omitempty"`
	AdminPassword string         `json:"admin_password,omitempty"`
	CompletedAt   string         `json:"completed_at"`
}

// tablesFor returns the tables a given mode clears.
func tablesFor(mode ResetMode) []string {
	out := append([]string{}, operationalTables...)
	if mode == ResetModeFactory {
		out = append(out, identityTables...)
	}
	return out
}

// settingsFor returns the settings keys a given mode clears.
func settingsFor(mode ResetMode) []string {
	out := append([]string{}, operationalSettings...)
	if mode == ResetModeFactory {
		out = append(out, secretSettings...)
	}
	return out
}

// backupBeforeReset writes a consistent snapshot of the whole database next to
// nothing in particular — SQLite's VACUUM INTO produces a single-file copy
// without needing to know the source path or stopping the server. A reset that
// cannot be backed up does not proceed.
//
// The caller gets the path so it can be surfaced to the operator; recovery is
// a file copy, not a restore procedure.
func backupBeforeReset(conn *sql.DB, dir string) (string, error) {
	stamp := time.Now().UTC().Format("20060102-150405")
	path := fmt.Sprintf("%s/lintasan-pre-reset-%s.db", dir, stamp)
	// VACUUM INTO fails if the target exists, which is the behavior we want:
	// never silently overwrite an earlier safety copy.
	if _, err := conn.Exec("VACUUM INTO ?", path); err != nil {
		return "", fmt.Errorf("pre-reset backup failed: %w", err)
	}
	return path, nil
}

// countRows returns current row counts for the given tables. Counting happens
// before deletion; a table that does not exist is reported as absent rather
// than failing the whole reset (schema varies across versions).
func countRows(conn *sql.DB, tables []string) map[string]int {
	out := make(map[string]int, len(tables))
	for _, t := range tables {
		var n int
		// Table names come from the package-level allowlists above, never from
		// user input, so interpolation here cannot be influenced by a caller.
		if err := conn.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n); err != nil {
			continue
		}
		out[t] = n
	}
	return out
}

// performReset executes the reset inside a single transaction: either the
// install ends up fully reset, or entirely untouched. A partial reset would
// leave an install in a state neither the operator nor the code expects.
func performReset(conn *sql.DB, mode ResetMode) error {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin reset transaction: %w", err)
	}
	defer tx.Rollback() // no-op after a successful Commit

	for _, t := range tablesFor(mode) {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			// A missing table is not a failure: schemas differ between
			// versions and an absent table is already "empty".
			continue
		}
	}

	for _, k := range settingsFor(mode) {
		if _, err := tx.Exec("DELETE FROM settings WHERE key = ?", k); err != nil {
			return fmt.Errorf("reset setting %q: %w", k, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset: %w", err)
	}
	return nil
}

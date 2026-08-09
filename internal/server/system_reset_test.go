package server

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// system_reset_test.go — the reset feature is irreversible in effect, so the
// tests focus on the properties that keep an operator safe: soft must NOT
// touch identity, factory must, backup must exist before anything is deleted,
// and a failure must leave the install untouched.

// newResetDB creates a file-backed SQLite DB with the tables reset touches.
// A file (not :memory:) because VACUUM INTO — the backup mechanism — needs a
// real database to copy.
func newResetDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	schema := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE connections (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE discovered_models (connection_id TEXT, model_id TEXT)`,
		`CREATE TABLE request_logs (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT)`,
		`CREATE TABLE oauth_sessions (id TEXT PRIMARY KEY)`,
		`CREATE TABLE audit_events (id TEXT PRIMARY KEY)`,
	}
	for _, s := range schema {
		if _, err := conn.Exec(s); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}

	seed := []string{
		`INSERT INTO connections VALUES ('c1','Cerebras'), ('c2','OpenAI')`,
		`INSERT INTO discovered_models VALUES ('c1','gpt-oss-120b')`,
		`INSERT INTO request_logs (id) VALUES (1), (2), (3)`,
		`INSERT INTO users VALUES ('u1','admin')`,
		`INSERT INTO oauth_sessions VALUES ('s1')`,
		`INSERT INTO audit_events VALUES ('a1')`,
		`INSERT INTO settings VALUES
			('master_key','secret-key'),
			('jwt_secret','secret-jwt'),
			('combos','[]'),
			('aliases','{}'),
			('port','20180')`,
	}
	for _, s := range seed {
		if _, err := conn.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return conn, dir
}

func count(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func setting(t *testing.T, conn *sql.DB, key string) (string, bool) {
	t.Helper()
	var v string
	err := conn.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatalf("read setting %s: %v", key, err)
	}
	return v, true
}

// THE critical safety property: a soft reset must never log the operator out
// or invalidate their API keys. If this regresses, "clear my test data"
// silently becomes "lock me out of my own gateway".
func TestSoftResetPreservesIdentity(t *testing.T) {
	conn, _ := newResetDB(t)

	if err := performReset(conn, ResetModeSoft); err != nil {
		t.Fatalf("soft reset: %v", err)
	}

	// Operational data gone.
	if n := count(t, conn, "connections"); n != 0 {
		t.Errorf("connections = %d, want 0", n)
	}
	if n := count(t, conn, "request_logs"); n != 0 {
		t.Errorf("request_logs = %d, want 0", n)
	}

	// Identity intact — this is the whole point of the mode.
	if n := count(t, conn, "users"); n != 1 {
		t.Errorf("users = %d, want 1 (soft reset must not delete users)", n)
	}
	if v, ok := setting(t, conn, "master_key"); !ok || v != "secret-key" {
		t.Error("master_key must survive a soft reset — existing API clients depend on it")
	}
	if v, ok := setting(t, conn, "jwt_secret"); !ok || v != "secret-jwt" {
		t.Error("jwt_secret must survive a soft reset — dashboard sessions depend on it")
	}
}

// Factory reset is the "like a new install" mode: identity and secrets go too.
func TestFactoryResetClearsIdentityAndSecrets(t *testing.T) {
	conn, _ := newResetDB(t)

	if err := performReset(conn, ResetModeFactory); err != nil {
		t.Fatalf("factory reset: %v", err)
	}

	for _, table := range []string{"connections", "request_logs", "users", "oauth_sessions"} {
		if n := count(t, conn, table); n != 0 {
			t.Errorf("%s = %d, want 0 after factory reset", table, n)
		}
	}
	if _, ok := setting(t, conn, "master_key"); ok {
		t.Error("master_key must be cleared by a factory reset")
	}
	if _, ok := setting(t, conn, "jwt_secret"); ok {
		t.Error("jwt_secret must be cleared by a factory reset")
	}
}

// Settings that are neither operational nor secret (like the listen port) are
// infrastructure, not user data — wiping them could make the server come back
// on an unexpected port.
func TestResetPreservesUnrelatedSettings(t *testing.T) {
	conn, _ := newResetDB(t)

	if err := performReset(conn, ResetModeFactory); err != nil {
		t.Fatalf("factory reset: %v", err)
	}
	if v, ok := setting(t, conn, "port"); !ok || v != "20180" {
		t.Error("unrelated settings (port) must not be cleared by reset")
	}
}

func TestBackupCreatedBeforeReset(t *testing.T) {
	conn, dir := newResetDB(t)

	path, err := backupBeforeReset(conn, dir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if info.Size() == 0 {
		t.Error("backup file is empty")
	}

	// The backup must still hold the data after the reset wipes the original —
	// otherwise it is not a recovery point.
	if err := performReset(conn, ResetModeFactory); err != nil {
		t.Fatalf("reset: %v", err)
	}

	backup, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer backup.Close()

	var n int
	if err := backup.QueryRow("SELECT COUNT(*) FROM connections").Scan(&n); err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if n != 2 {
		t.Errorf("backup has %d connections, want 2 — backup must predate the wipe", n)
	}
}

// Never silently overwrite an earlier safety copy.
func TestBackupDoesNotOverwrite(t *testing.T) {
	conn, dir := newResetDB(t)

	path, err := backupBeforeReset(conn, dir)
	if err != nil {
		t.Fatalf("first backup: %v", err)
	}
	// VACUUM INTO refuses an existing target; same-second retry must error
	// rather than clobber.
	if _, err := conn.Exec("VACUUM INTO ?", path); err == nil {
		t.Error("backup overwrote an existing file; it must refuse")
	}
}

// A reset that fails partway must leave the install untouched, not half-wiped.
func TestResetIsAtomic(t *testing.T) {
	conn, _ := newResetDB(t)

	// Drop settings so the settings DELETE inside the transaction fails after
	// the table DELETEs have already run.
	if _, err := conn.Exec("DROP TABLE settings"); err != nil {
		t.Fatalf("drop settings: %v", err)
	}

	if err := performReset(conn, ResetModeSoft); err == nil {
		t.Fatal("expected reset to fail when settings table is missing")
	}

	// Rolled back: operational data must still be there.
	if n := count(t, conn, "connections"); n != 2 {
		t.Errorf("connections = %d, want 2 — a failed reset must roll back", n)
	}
	if n := count(t, conn, "request_logs"); n != 3 {
		t.Errorf("request_logs = %d, want 3 — a failed reset must roll back", n)
	}
}

// Each mode has its own phrase so a soft confirmation can't trigger a factory
// wipe (e.g. a UI that forgets to update the phrase when the mode changes).
func TestConfirmPhrasesDiffer(t *testing.T) {
	soft := confirmPhraseFor(ResetModeSoft)
	factory := confirmPhraseFor(ResetModeFactory)
	if soft == factory {
		t.Fatalf("soft and factory share the confirmation phrase %q", soft)
	}
	if soft == "" || factory == "" {
		t.Fatal("confirmation phrases must not be empty")
	}
}

// Mode membership is the contract the whole feature rests on.
func TestModeTableMembership(t *testing.T) {
	softTables := tablesFor(ResetModeSoft)
	for _, forbidden := range identityTables {
		for _, got := range softTables {
			if got == forbidden {
				t.Errorf("soft reset must not clear identity table %q", forbidden)
			}
		}
	}

	softSettings := settingsFor(ResetModeSoft)
	for _, forbidden := range secretSettings {
		for _, got := range softSettings {
			if got == forbidden {
				t.Errorf("soft reset must not clear secret setting %q", forbidden)
			}
		}
	}

	// Factory is a strict superset — anything soft clears, factory clears too.
	factoryTables := tablesFor(ResetModeFactory)
	for _, want := range softTables {
		found := false
		for _, got := range factoryTables {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("factory reset missing table %q that soft clears", want)
		}
	}
}

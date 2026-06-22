package plugin

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	_, err = db.Exec(`PRAGMA journal_mode=MEMORY`)
	if err != nil {
		t.Fatalf("failed to set journal mode: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE plugins (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			enabled INTEGER DEFAULT 1,
			priority INTEGER DEFAULT 100,
			code TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now', 'localtime'))
		)
	`)
	if err != nil {
		t.Fatalf("failed to create plugins table: %v", err)
	}
	return db
}

func TestNewManager_LoadsPlugins(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO plugins (id, name, description, code, enabled, priority) VALUES (?, ?, ?, ?, ?, ?)`,
		"test-1", "test plugin", "a test plugin", `
function beforeRequest(req) {
    req.modified = true;
    return req;
}
`, 1, 10)

	if err != nil {
		t.Fatalf("failed to insert plugin: %v", err)
	}

	m := NewManager(db)
	if len(m.plugins) != 1 {
		t.Fatalf("expected 1 plugin loaded, got %d", len(m.plugins))
	}

	if m.plugins[0].Name != "test plugin" {
		t.Errorf("expected plugin name 'test plugin', got '%s'", m.plugins[0].Name)
	}
	if m.plugins[0].Enabled != true {
		t.Errorf("expected plugin enabled=true")
	}
}

func TestNewManager_OnlyLoadsEnabled(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO plugins (id, name, description, code, enabled, priority) VALUES (?, ?, ?, ?, ?, ?)`,
		"disabled-1", "disabled", "desc", "function beforeRequest(r){return r}", 0, 10)
	_, _ = db.Exec(`INSERT INTO plugins (id, name, description, code, enabled, priority) VALUES (?, ?, ?, ?, ?, ?)`,
		"enabled-1", "enabled", "desc", "function beforeRequest(r){return r}", 1, 20)

	m := NewManager(db)
	if len(m.plugins) != 1 {
		t.Fatalf("expected 1 enabled plugin, got %d", len(m.plugins))
	}
	if m.plugins[0].Name != "enabled" {
		t.Errorf("expected 'enabled' plugin, got '%s'", m.plugins[0].Name)
	}
}

func TestPlugins_ReturnsAdapterSlice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO plugins (id, name, description, code, enabled, priority) VALUES (?, ?, ?, ?, ?, ?)`,
		"p1", "plugin1", "desc", "function beforeRequest(r){return r}", 1, 10)

	m := NewManager(db)
	adapters := m.Plugins()
	if len(adapters) != 1 {
		t.Fatalf("expected 1 adapter, got %d", len(adapters))
	}
	_, ok := adapters[0].(*PluginAdapter)
	if !ok {
		t.Errorf("expected PluginAdapter type, got %T", adapters[0])
	}
}

func TestExecuteRequestHook_NoPlugins(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db)
	body := []byte(`{"test": true}`)
	result, err := m.ExecuteRequestHook(nil, "conn1", "gpt-4", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(body) {
		t.Errorf("expected body unchanged with no plugins, got %s", result)
	}
}

func TestNewManager_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db)
	if len(m.plugins) != 0 {
		t.Errorf("expected 0 plugins in empty DB, got %d", len(m.plugins))
	}
}

func TestLoadPlugins_CompileError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO plugins (id, name, description, code, enabled) VALUES (?, ?, ?, ?, ?)`,
		"bad", "bad plugin", "desc", "this is not valid javascript {{ {", 1)

	m := NewManager(db)
	if len(m.plugins) != 0 {
		t.Errorf("expected 0 plugins loaded (compile error skipped), got %d", len(m.plugins))
	}
}

func TestPluginAdapter_OnRequest(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO plugins (id, name, description, code, enabled, priority) VALUES (?, ?, ?, ?, ?, ?)`,
		"p1", "test", "desc", "function beforeRequest(r){return r}", 1, 10)

	m := NewManager(db)
	if len(m.plugins) == 0 {
		t.Fatal("no plugins loaded")
	}
	adapter := &PluginAdapter{Plugin: m.plugins[0]}
	body := []byte(`{"test": true}`)
	result, err := adapter.OnRequest(nil, "conn1", "gpt-4", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(body) {
		t.Errorf("expected passthrough body, got %s", result)
	}
}

func TestPluginAdapter_OnResponse(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO plugins (id, name, description, code, enabled, priority) VALUES (?, ?, ?, ?, ?, ?)`,
		"p1", "test", "desc", "function afterResponse(r){return r}", 1, 10)

	m := NewManager(db)
	if len(m.plugins) == 0 {
		t.Fatal("plugin not loaded")
	}
	adapter := &PluginAdapter{Plugin: m.plugins[0]}
	body := []byte(`{"test": true}`)
	result, err := adapter.OnResponse(nil, "conn1", "gpt-4", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(body) {
		t.Errorf("expected passthrough body, got %s", result)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

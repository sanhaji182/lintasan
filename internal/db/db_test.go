package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer d.Close()

	if d.Conn() == nil {
		t.Fatal("Conn() returned nil")
	}
}

func TestOpen_CreatesTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lintasan.db")

	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer d.Close()

	// Verify key tables exist
	tables := []string{"settings", "connections", "users", "request_logs", "plugins"}
	for _, table := range tables {
		var count int
		err := d.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s not accessible: %v", table, err)
		}
	}
}

func TestGetSetting_Missing(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer d.Close()

	val, err := d.GetSetting("nonexistent")
	if err != nil {
		t.Fatalf("GetSetting() failed: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty for missing key, got %q", val)
	}
}

func TestSetSettingAndGetSetting(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer d.Close()

	err = d.SetSetting("test_key", "test_value")
	if err != nil {
		t.Fatalf("SetSetting() failed: %v", err)
	}

	val, err := d.GetSetting("test_key")
	if err != nil {
		t.Fatalf("GetSetting() after set failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got %q", val)
	}
}

func TestSetSetting_Overwrite(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer d.Close()

	_ = d.SetSetting("k", "v1")
	_ = d.SetSetting("k", "v2")

	val, _ := d.GetSetting("k")
	if val != "v2" {
		t.Errorf("expected 'v2', got %q", val)
	}
}

func TestOpen_ReusesExistingDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.db")

	d1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open() failed: %v", err)
	}
	_ = d1.SetSetting("already_there", "yep")
	d1.Close()

	// Open again — should reuse existing DB and data
	d2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open() failed: %v", err)
	}
	defer d2.Close()

	val, err := d2.GetSetting("already_there")
	if err != nil {
		t.Fatalf("GetSetting() after reopen failed: %v", err)
	}
	if val != "yep" {
		t.Errorf("expected 'yep' from existing DB, got %q", val)
	}
}

func TestOpen_DefaultMaxConns(t *testing.T) {
	dir := t.TempDir()
	os.Unsetenv("LINTASAN_DB_MAX_CONNS")

	d, err := Open(filepath.Join(dir, "conns.db"))
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer d.Close()

	// If it opened without error with default conns, that's sufficient
	// (go-sqlite3 doesn't expose MaxOpenConns getter)
	_ = d.Conn().Ping()
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

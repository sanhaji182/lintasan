package expprovider

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS experimental_providers (
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
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteStore_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	desc := map[string]string{"name": "codex", "auth_env_var": "OPENAI_API_KEY"}
	descJSON, _ := json.Marshal(desc)
	now := time.Now().UTC().Truncate(time.Second)

	rec := &ProviderRecord{
		Name:               "codex",
		Track:              "experimental",
		State:              "admitted",
		DescriptorJSON:     descJSON,
		AdmittedAt:         &now,
		ValidationEvidence: "wire-pass",
		RiskBadge:          "experimental",
	}

	if err := store.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get(ctx, "codex")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Name != "codex" || got.State != "admitted" || got.Track != "experimental" {
		t.Fatalf("unexpected record: %+v", got)
	}
	if got.ValidationEvidence != "wire-pass" {
		t.Fatalf("validation_evidence = %q, want wire-pass", got.ValidationEvidence)
	}
	if got.AdmittedAt == nil {
		t.Fatal("admitted_at is nil")
	}
}

func TestSQLiteStore_List(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	for _, name := range []string{"codex", "claude-code", "gemini-cli"} {
		rec := &ProviderRecord{
			Name:      name,
			Track:     "experimental",
			State:     "proposed",
			RiskBadge: "experimental",
		}
		if err := store.Save(ctx, rec); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d records, want 3", len(list))
	}
	// Ordered by name
	if list[0].Name != "claude-code" || list[1].Name != "codex" || list[2].Name != "gemini-cli" {
		t.Fatalf("unexpected order: %s, %s, %s", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestSQLiteStore_UpdateState(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	rec := &ProviderRecord{
		Name:      "codex",
		Track:     "experimental",
		State:     "proposed",
		RiskBadge: "experimental",
	}
	if err := store.Save(ctx, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateState(ctx, "codex", "admitted", now); err != nil {
		t.Fatalf("UpdateState admitted: %v", err)
	}

	got, _ := store.Get(ctx, "codex")
	if got.State != "admitted" {
		t.Fatalf("state = %q, want admitted", got.State)
	}
	if got.AdmittedAt == nil {
		t.Fatal("admitted_at not set")
	}

	// Activate
	later := now.Add(time.Hour)
	if err := store.UpdateState(ctx, "codex", "active", later); err != nil {
		t.Fatalf("UpdateState active: %v", err)
	}
	got, _ = store.Get(ctx, "codex")
	if got.State != "active" || got.ActivatedAt == nil {
		t.Fatalf("state=%q activated_at=%v", got.State, got.ActivatedAt)
	}

	// Deactivate
	if err := store.UpdateState(ctx, "codex", "deprecated", later.Add(time.Hour)); err != nil {
		t.Fatalf("UpdateState deprecated: %v", err)
	}
	got, _ = store.Get(ctx, "codex")
	if got.State != "deprecated" || got.DeactivatedAt == nil {
		t.Fatalf("state=%q deactivated_at=%v", got.State, got.DeactivatedAt)
	}
}

func TestSQLiteStore_SetValidationEvidence(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	rec := &ProviderRecord{
		Name:      "codex",
		Track:     "experimental",
		State:     "admitted",
		RiskBadge: "experimental",
	}
	store.Save(ctx, rec)

	if err := store.SetValidationEvidence(ctx, "codex", "live-pass: tool-loop closed"); err != nil {
		t.Fatalf("SetValidationEvidence: %v", err)
	}
	got, _ := store.Get(ctx, "codex")
	if got.ValidationEvidence != "live-pass: tool-loop closed" {
		t.Fatalf("evidence = %q", got.ValidationEvidence)
	}
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	got, err := store.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for nonexistent, got %+v", got)
	}
}

func TestSQLiteStore_Upsert(t *testing.T) {
	db := setupTestDB(t)
	store := NewSQLiteStore(db)
	ctx := context.Background()

	rec := &ProviderRecord{
		Name:      "codex",
		Track:     "experimental",
		State:     "proposed",
		RiskBadge: "experimental",
	}
	store.Save(ctx, rec)

	// Update via Save (upsert)
	rec.State = "admitted"
	rec.ValidationEvidence = "fixture-pass"
	store.Save(ctx, rec)

	got, _ := store.Get(ctx, "codex")
	if got.State != "admitted" || got.ValidationEvidence != "fixture-pass" {
		t.Fatalf("upsert failed: state=%q evidence=%q", got.State, got.ValidationEvidence)
	}

	// Only one record
	list, _ := store.List(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1 record after upsert, got %d", len(list))
	}
}

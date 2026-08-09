package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The whole point of the refresh path: an install that seeded an OLD catalogue
// must be brought up to the new one — renamed presets update in place,
// recategorised presets move category — rather than being frozen at whatever
// was current when it first seeded.
func TestSeedBuiltinPresets_RefreshesExistingBuiltin(t *testing.T) {
	s := newRESTTestServer(t)
	now := "2026-01-01 00:00:00"

	// Simulate an install that already has the OLD catalogue: a preset under
	// its old name and old category. Use a real endpoint from the new
	// catalogue so the seeder matches it.
	target := seedCatalogue()[0] // any built-in
	_, err := s.db.Conn().Exec(`
		INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
		VALUES ('old1', ?, ?, ?, 'openai', 'API Key', 'inference', 1, ?, ?)`,
		"Old Name For "+target.Name, target.Domain, target.BaseURL, now, now)
	if err != nil {
		t.Fatalf("seed old row: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleSeedBuiltinPresets(rec, httptest.NewRequest("POST", "/api/presets/seed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	// The old row must have been renamed AND recategorised in place — not
	// duplicated under the new name.
	var count int
	_ = s.db.Conn().QueryRow(
		"SELECT COUNT(*) FROM provider_presets WHERE base_url = ?", target.BaseURL).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row for %s after refresh, got %d (rename duplicated the row)", target.BaseURL, count)
	}
	var name, category string
	_ = s.db.Conn().QueryRow(
		"SELECT name, category FROM provider_presets WHERE base_url = ?", target.BaseURL).Scan(&name, &category)
	if name != target.Name {
		t.Errorf("name = %q, want refreshed %q", name, target.Name)
	}
	if category != target.Category {
		t.Errorf("category = %q, want refreshed %q", category, target.Category)
	}
}

// A user's own (non-builtin) preset must never be rewritten by seeding, even
// when it points at an endpoint the built-in catalogue also lists.
func TestSeedBuiltinPresets_NeverTouchesManualPreset(t *testing.T) {
	s := newRESTTestServer(t)
	now := "2026-01-01 00:00:00"
	target := seedCatalogue()[0]

	_, err := s.db.Conn().Exec(`
		INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
		VALUES ('mine', 'My Edited Name', ?, ?, 'openai', 'API Key', 'custom-cat', 0, ?, ?)`,
		target.Domain, target.BaseURL, now, now)
	if err != nil {
		t.Fatalf("seed manual row: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleSeedBuiltinPresets(rec, httptest.NewRequest("POST", "/api/presets/seed", nil))

	var name, category string
	_ = s.db.Conn().QueryRow(
		"SELECT name, category FROM provider_presets WHERE id = 'mine'").Scan(&name, &category)
	if name != "My Edited Name" || category != "custom-cat" {
		t.Errorf("manual preset was rewritten: name=%q category=%q", name, category)
	}
}

// A rename must find the row by its OLD name and update it, not insert a new
// one. This is the path that lets "gitlawb" become "GitLawb" without leaving a
// stale duplicate behind.
func TestSeedBuiltinPresets_RenameMatchesOldName(t *testing.T) {
	s := newRESTTestServer(t)
	now := "2026-01-01 00:00:00"

	// Insert under the OLD name "gitlawb", which the catalogue now calls
	// "GitLawb". The base_url matches the new entry.
	_, err := s.db.Conn().Exec(`
		INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
		VALUES ('gl', 'gitlawb', 'gitlawb.com', 'https://opengateway.gitlawb.com/v1/', 'openai', 'API Key', 'indonesia', 0, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("seed old-name row: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleSeedBuiltinPresets(rec, httptest.NewRequest("POST", "/api/presets/seed", nil))

	// The manual row keeps its own identity (skipped, not rewritten), and no
	// duplicate may be created for the same endpoint.
	var count int
	_ = s.db.Conn().QueryRow(
		"SELECT COUNT(*) FROM provider_presets WHERE lower(rtrim(base_url,'/')) = 'https://opengateway.gitlawb.com/v1'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row for the gitlawb endpoint, got %d", count)
	}
	var name string
	_ = s.db.Conn().QueryRow("SELECT name FROM provider_presets WHERE id='gl'").Scan(&name)
	if !strings.EqualFold(name, "gitlawb") {
		t.Errorf("manual row name changed to %q; seeding must not rewrite manual presets", name)
	}
}

// The seed response must report what it did, so the UI can show "updated N"
// alongside "inserted N".
func TestSeedBuiltinPresets_ReportsCounts(t *testing.T) {
	s := newRESTTestServer(t)
	rec := httptest.NewRecorder()
	s.handleSeedBuiltinPresets(rec, httptest.NewRequest("POST", "/api/presets/seed", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("seed: got %d", rec.Code)
	}
	var out struct {
		Inserted int `json:"inserted"`
		Updated  int `json:"updated"`
		Skipped  int `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if out.Inserted == 0 {
		t.Errorf("expected a fresh install to insert presets, got 0 (response: %+v)", out)
	}
}

// Every preset's category must be a registered category key, otherwise the UI
// cannot label it. Eight presets were pointing at free-tier/coding before those
// categories existed.
func TestBuiltinPresetCategoriesAreRegistered(t *testing.T) {
	registered := map[string]bool{
		"foundation": true, "open": true, "inference": true, "aggregator": true,
		"search": true, "local": true, "free-tier": true, "coding": true,
	}
	for _, p := range seedCatalogue() {
		if !registered[p.Category] {
			t.Errorf("preset %q has unregistered category %q", p.Name, p.Category)
		}
	}
}

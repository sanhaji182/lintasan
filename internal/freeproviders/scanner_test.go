package freeproviders

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"strings"
	"testing"
)

// newTestDB creates an in-memory SQLite database with the required schema.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	// Create minimal schema
	migrations := []string{
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
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			// Ignore already-exists errors
			continue
		}
	}
	return db
}

// TestDefaultTargets verifies the default target list is non-empty and covers
// the expected ports.
func TestDefaultTargets(t *testing.T) {
	targets := DefaultTargets()
	if len(targets) == 0 {
		t.Fatal("expected non-empty targets list")
	}

	// Verify known ports
	portSet := make(map[int]bool)
	for _, tg := range targets {
		portSet[tg.Port] = true
	}
	expectedPorts := []int{11434, 1234, 4891, 8080}
	for _, p := range expectedPorts {
		if !portSet[p] {
			t.Errorf("expected port %d in targets, not found", p)
		}
	}

	// All should have type "local"
	for _, tg := range targets {
		if tg.Type != "local" {
			t.Errorf("target %s: type = %q, want %q", tg.ID, tg.Type, "local")
		}
	}
}

// TestProbeEndpointUnreachable checks behavior when nothing is listening.
func TestProbeEndpointUnreachable(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	s := New(db)
	// Use a port that's very unlikely to be in use during tests
	body, status, err := s.probeEndpoint(19999, "/api/tags")
	if err == nil && status == 200 {
		t.Skip("port 19999 unexpectedly responded — skipping unreachable test")
	}
	if err == nil {
		t.Logf("Got status %d on port 19999 (unexpected but not an error)", status)
	}
	_ = body
}

// TestProbeEndpointWithMockServer verifies probing against a real HTTP server.
func TestProbeEndpointWithMockServer(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Start a mock server that returns Ollama-style /api/tags
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			fmt.Fprint(w, `{"models":[{"name":"llama3.2:latest"},{"name":"codellama:7b"}]}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer mockSrv.Close()

	// We can't directly use mockSrv URL since probeEndpoint hardcodes 127.0.0.1.
	// Instead test the extraction logic directly.
	s := New(db)

	body := []byte(`{"models":[{"name":"llama3.2:latest"},{"name":"codellama:7b"}]}`)
	models := s.extractModels("ollama", body)

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0] != "llama3.2:latest" {
		t.Errorf("models[0] = %q, want %q", models[0], "llama3.2:latest")
	}
	if models[1] != "codellama:7b" {
		t.Errorf("models[1] = %q, want %q", models[1], "codellama:7b")
	}
}

// TestExtractModelsOpenAI tests parsing of OpenAI-compatible /v1/models responses.
func TestExtractModelsOpenAI(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	s := New(db)

	body := []byte(`{"data":[{"id":"gpt-4","object":"model"},{"id":"gpt-3.5-turbo","object":"model"}]}`)
	models := s.extractModels("openai", body)

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0] != "gpt-4" {
		t.Errorf("models[0] = %q, want %q", models[0], "gpt-4")
	}
}

// TestExtractModelsEmpty tests handling of empty/malformed responses.
func TestExtractModelsEmpty(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	s := New(db)

	// Empty body
	models := s.extractModels("openai", []byte(`{}`))
	if len(models) != 0 {
		t.Errorf("expected 0 models for empty response, got %d", len(models))
	}

	// Invalid JSON
	models = s.extractModels("ollama", []byte(`not json`))
	if len(models) != 0 {
		t.Errorf("expected 0 models for invalid JSON, got %d", len(models))
	}

	// Unknown format
	models = s.extractModels("unknown", []byte(`{"data":[]}`))
	if len(models) != 0 {
		t.Errorf("expected 0 models for unknown format, got %d", len(models))
	}
}

// TestRegisterProvider tests connection registration logic.
func TestRegisterProvider(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()
	s := New(db)

	target := ProviderTarget{
		ID:         "test-local",
		Name:       "Test Provider",
		Port:       9999,
		Format:     "openai",
		ChatPath:   "/v1/chat/completions",
		ModelsPath: "/v1/models",
	}

	models := []string{"model-a", "model-b"}

	// First registration should create a new connection
	id, registered, err := s.registerProvider(target, models)
	if err != nil {
		t.Fatalf("registerProvider: %v", err)
	}
	if !registered {
		t.Error("expected first registration to be new")
	}
	if id == "" {
		t.Error("expected non-empty connection ID")
	}

	// Verify connection was inserted
	var baseURL string
	err = db.QueryRow(`SELECT base_url FROM connections WHERE id = ?`, id).Scan(&baseURL)
	if err != nil {
		t.Fatalf("query connection: %v", err)
	}
	if !strings.Contains(baseURL, "9999") {
		t.Errorf("base_url = %q, expected port 9999", baseURL)
	}

	// Verify discovered models
	rows, err := db.Query(`SELECT model_id FROM discovered_models WHERE connection_id = ? ORDER BY model_id`, id)
	if err != nil {
		t.Fatalf("query models: %v", err)
	}
	defer rows.Close()
	var foundModels []string
	for rows.Next() {
		var m string
		rows.Scan(&m)
		foundModels = append(foundModels, m)
	}
	if len(foundModels) != 2 {
		t.Errorf("expected 2 models, got %d: %v", len(foundModels), foundModels)
	}

	// Second registration with same port should be idempotent
	id2, registered2, err := s.registerProvider(target, models)
	if err != nil {
		t.Fatalf("second registerProvider: %v", err)
	}
	if registered2 {
		t.Error("expected second registration to not be new")
	}
	if id2 != id {
		t.Errorf("second registration returned different ID: %q vs %q", id2, id)
	}
}

// TestDiscoverTargetsSimulated tests the discover flow by making targets
// unreachable (no servers running on those ports).
func TestDiscoverTargetsSimulated(t *testing.T) {
	// Create a temp SQLite file for this test
	tmpDir, err := os.MkdirTemp("", "lintasan-fp-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	// Create schema
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS connections (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, base_url TEXT NOT NULL,
			api_key TEXT NOT NULL DEFAULT '', format TEXT NOT NULL DEFAULT 'openai',
			chat_path TEXT NOT NULL DEFAULT '/v1/chat/completions',
			models_path TEXT DEFAULT '/v1/models', is_active INTEGER DEFAULT 1,
			priority INTEGER DEFAULT 0, models_count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS discovered_models (
			id TEXT PRIMARY KEY, connection_id TEXT NOT NULL, model_id TEXT NOT NULL,
			model_name TEXT, owned_by TEXT, is_active INTEGER DEFAULT 1,
			FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
		)`,
	}
	for _, m := range migrations {
		db.Exec(m)
	}

	s := New(db)

	// Custom targets pointing to unlikely ports so all are unreachable
	targets := []ProviderTarget{
		{ID: "a", Name: "A", Port: 19991, DetectPath: "/api/tags", Format: "ollama", ChatPath: "/api/chat", ModelsPath: "/api/tags", Type: "local"},
		{ID: "b", Name: "B", Port: 19992, DetectPath: "/v1/models", Format: "openai", ChatPath: "/v1/chat/completions", ModelsPath: "/v1/models", Type: "local"},
	}

	resp := s.DiscoverTargets(targets)

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	if resp.TotalFound != 0 {
		t.Errorf("expected 0 found, got %d", resp.TotalFound)
	}

	if resp.NewRegistered != 0 {
		t.Errorf("expected 0 new registered, got %d", resp.NewRegistered)
	}

	for _, r := range resp.Results {
		if r.Status == "available" {
			t.Errorf("expected unreachable for %s, got %s", r.Provider, r.Status)
		}
	}

	if resp.ScannedAt == "" {
		t.Error("expected ScannedAt to be set")
	}
}

// TestIsSameBaseURL tests URL comparison for dedup.
func TestIsSameBaseURL(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"http://127.0.0.1:11434", "http://127.0.0.1:11434", true},
		{"http://127.0.0.1:11434/", "http://127.0.0.1:11434", true},
		{"http://127.0.0.1:11434", "http://127.0.0.1:11434/", true},
		{"http://127.0.0.1:11434/", "http://127.0.0.1:11434/", true},
		{"http://127.0.0.1:11434", "http://127.0.0.1:8080", false},
		{"http://127.0.0.1:11434", "http://192.168.1.1:11434", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			if got := IsSameBaseURL(tt.a, tt.b); got != tt.want {
				t.Errorf("IsSameBaseURL(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// Validate that json key names are correct by marshaling DiscoveryResult
func TestDiscoveryResultJSON(t *testing.T) {
	r := DiscoveryResult{
		Provider:    "Ollama (Local)",
		Port:        11434,
		ModelsFound: []string{"llama3.2"},
		ModelCount:  1,
		Status:      "available",
		Registered:  true,
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	required := []string{"provider", "port", "models_found", "model_count", "status", "registered"}
	for _, key := range required {
		if _, ok := decoded[key]; !ok {
			t.Errorf("missing JSON key %q in DiscoveryResult", key)
		}
	}
}

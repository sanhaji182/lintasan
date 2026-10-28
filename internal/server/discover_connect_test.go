package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateConnectionSeedsModelsAndUpdatesExisting(t *testing.T) {
	s := newRESTTestServer(t)

	post := func(body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/connections", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		s.handleCreateConnection(rec, req)
		return rec
	}

	first := post(map[string]any{
		"name":        "Pollinations",
		"base_url":    "https://text.pollinations.ai",
		"api_key":     "sk-secret",
		"models":      []string{"openai", "openai-fast"},
		"chat_path":   "/openai",
		"models_path": "/models",
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("create: got %d (%s)", first.Code, first.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if got := int(created["models_count"].(float64)); got != 2 {
		t.Fatalf("models_count = %d, want 2", got)
	}
	id := created["id"].(string)

	var chatPath, modelsPath, apiKey string
	if err := s.db.Conn().QueryRow(
		`SELECT chat_path, models_path, api_key FROM connections WHERE id=?`, id,
	).Scan(&chatPath, &modelsPath, &apiKey); err != nil {
		t.Fatal(err)
	}
	if chatPath != "/openai" || modelsPath != "/models" {
		t.Fatalf("paths = %q/%q", chatPath, modelsPath)
	}
	if apiKey != "sk-secret" {
		t.Fatalf("api_key = %q", apiKey)
	}

	second := post(map[string]any{
		"name":     "Pollinations",
		"base_url": "https://text.pollinations.ai",
		"models":   []string{"openai"},
	})
	if second.Code != http.StatusOK {
		t.Fatalf("update: got %d (%s)", second.Code, second.Body.String())
	}
	var updated map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated["status"] != "updated" || updated["id"] != id {
		t.Fatalf("update response = %v", updated)
	}
	if got := int(updated["models_count"].(float64)); got != 1 {
		t.Fatalf("updated models_count = %d, want 1", got)
	}

	var conns, models int
	s.db.Conn().QueryRow(`SELECT COUNT(*) FROM connections WHERE name='Pollinations'`).Scan(&conns)
	s.db.Conn().QueryRow(`SELECT COUNT(*) FROM discovered_models WHERE connection_id=?`, id).Scan(&models)
	if conns != 1 {
		t.Fatalf("connections = %d, want 1", conns)
	}
	if models != 1 {
		t.Fatalf("models = %d, want 1", models)
	}
	var storedKey string
	s.db.Conn().QueryRow(`SELECT api_key FROM connections WHERE id=?`, id).Scan(&storedKey)
	if storedKey != "sk-secret" {
		t.Fatalf("reconnect without a key must preserve the old key, got %q", storedKey)
	}
}

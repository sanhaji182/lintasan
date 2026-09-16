package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleModelsSyncPostForConnection_ReportsUpstreamFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer upstream.Close()

	s := newRESTTestServer(t)
	connID := "test-post-sync-failure"
	_, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, format, models_path, is_active)
		VALUES (?, 'Blocked Provider', ?, 'openai', '/models', 1)
	`, connID, upstream.URL)
	if err != nil {
		t.Fatalf("insert connection: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleModelsSync(rec, reqWithPath("POST", "/api/models/sync", map[string]any{
		"connection_id": connID,
	}, nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("sync failure: got %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("sync failure must not report success: %s", rec.Body.String())
	}
}

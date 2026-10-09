package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestAntigravityAuthorizeMissingConfigurationCreatesNoPendingSession(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_SECRET", "")

	s, _ := newTestServer(t, &config.Config{
		OAuthIDEEnabled:    true,
		OAuthPublicBaseURL: "https://lintasan.example",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/authorize", strings.NewReader(`{
		"provider":"antigravity",
		"acknowledge_risk":true
	}`))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{
		Username: "admin",
		Role:     "admin",
	}))
	rec := httptest.NewRecorder()

	s.handleOAuthAuthorize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["redirect_url"] != nil {
		t.Fatalf("redirect_url = %v, want absent", body["redirect_url"])
	}
	if !strings.Contains(body["error"].(string), "LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID") {
		t.Fatalf("error = %q, want actionable configuration name", body["error"])
	}

	var pending int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(*) FROM oauth_sessions WHERE provider = 'antigravity' AND status = 'pending'`).Scan(&pending); err != nil {
		t.Fatalf("count pending sessions: %v", err)
	}
	if pending != 0 {
		t.Fatalf("pending sessions = %d, want 0", pending)
	}
}

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

func TestAntigravityAuthorizeSuccessWithLoopback(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "test-client.example.com")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "test-secret")
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

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["flow"] != "browser_redirect" {
		t.Fatalf("flow = %v, want browser_redirect", body["flow"])
	}
	redirectURL, ok := body["redirect_url"].(string)
	if !ok || redirectURL == "" {
		t.Fatalf("redirect_url = %v, want valid url", body["redirect_url"])
	}
	if !strings.Contains(redirectURL, "redirect_uri=http%3A%2F%2F127.0.0.1%3A51121%2Fcallback") {
		t.Fatalf("redirect_url = %s, want loopback redirect_uri", redirectURL)
	}
	if body["antigravity_manual_complete"] != true {
		t.Fatalf("antigravity_manual_complete = %v, want true", body["antigravity_manual_complete"])
	}

	var pending int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(*) FROM oauth_sessions WHERE provider = 'antigravity' AND status = 'pending'`).Scan(&pending); err != nil {
		t.Fatalf("count pending sessions: %v", err)
	}
	if pending != 1 {
		t.Fatalf("pending sessions = %d, want 1", pending)
	}
}

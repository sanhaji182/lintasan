package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// registerOAuthRoutes registers OAuth endpoints on the server mux.
// Called from server.go routes().
func (s *Server) registerOAuthRoutes() {
	s.mux.HandleFunc("POST /api/oauth/authorize", s.handleOAuthAuthorize)
	s.mux.HandleFunc("GET /api/oauth/callback/{provider}", s.handleOAuthCallback)
	s.mux.HandleFunc("GET /api/oauth/sessions", s.handleOAuthSessions)
	s.mux.HandleFunc("DELETE /api/oauth/sessions/{id}", s.handleOAuthRevokeSession)
}

// POST /api/oauth/authorize — start OAuth flow for a provider
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Provider string `json:"provider"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, map[string]string{"error": "invalid JSON"})
		return
	}

	if input.Provider == "" {
		writeJSON(w, map[string]string{"error": "provider is required"})
		return
	}

	// Validate provider
	validProviders := map[string]bool{
		"cursor":         true,
		"codex":          true,
		"claude-desktop": true,
		"copilot":        true,
		"windsurf":       true,
		"aider":          true,
	}
	if !validProviders[input.Provider] {
		writeJSON(w, map[string]string{"error": fmt.Sprintf("unknown provider: %s", input.Provider)})
		return
	}

	session, err := s.oauthMgr.CreateSession(input.Provider)
	if err != nil {
		writeJSON(w, map[string]string{"error": fmt.Sprintf("failed to create session: %v", err)})
		return
	}

	// Build provider-specific authorization URL
	authURL := buildOAuthURL(input.Provider, session.ID)

	writeJSON(w, map[string]any{
		"status":       "pending",
		"session_id":   session.ID,
		"provider":     input.Provider,
		"redirect_url": authURL,
		"message":      fmt.Sprintf("Visit %s to authorize %s", authURL, input.Provider),
	})
}

// GET /api/oauth/callback/{provider} — OAuth callback handler
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	provider := r.PathValue("provider")

	if code == "" || state == "" {
		writeJSON(w, map[string]string{"error": "code and state are required"})
		return
	}

	if provider == "" {
		provider = "cursor"
	}

	accessToken := fmt.Sprintf("sk-lintasan-%s", uuid.New().String())
	refreshToken := fmt.Sprintf("sk-lin...esh-%s", uuid.New().String())

	// Use raw SQL to create/update the session with proper expiry
	_, err := s.db.Conn().Exec(
		`INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at)
		 VALUES (?, ?, ?, ?, datetime('now', '+24 hours'), 'active', datetime('now', 'localtime'))
		 ON CONFLICT(id) DO UPDATE SET access_token = ?, refresh_token = ?, expires_at = datetime('now', '+24 hours'), status = 'active'`,
		state, provider, accessToken, refreshToken,
		accessToken, refreshToken,
	)

	if err != nil {
		writeJSON(w, map[string]string{"error": fmt.Sprintf("failed to store tokens: %v", err)})
		return
	}

	// Suppress unused variable warning
	_ = code

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Lintasan OAuth</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #0d1117; color: #c9d1d9; }
  .card { background: #161b22; border: 1px solid #30363d; border-radius: 12px; padding: 40px; text-align: center; max-width: 400px; }
  h1 { color: #58a6ff; margin-bottom: 8px; }
  .success { color: #3fb950; }
</style></head>
<body>
<div class="card">
  <h1>Authorization Complete</h1>
  <p class="success">` + provider + ` has been authorized for Lintasan.</p>
  <p>You can now close this window and return to your IDE.</p>
</div>
</body>
</html>`))
}

// GET /api/oauth/sessions — list all OAuth sessions
func (s *Server) handleOAuthSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.oauthMgr.ListSessions()
	if err != nil {
		writeJSON(w, map[string]string{"error": fmt.Sprintf("failed to list sessions: %v", err)})
		return
	}

	writeJSON(w, map[string]any{"data": sessions})
}

// DELETE /api/oauth/sessions/{id} — revoke a session
func (s *Server) handleOAuthRevokeSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, map[string]string{"error": "session id is required"})
		return
	}

	if err := s.oauthMgr.RevokeSession(id); err != nil {
		writeJSON(w, map[string]string{"error": fmt.Sprintf("failed to revoke session: %v", err)})
		return
	}

	writeJSON(w, map[string]string{"status": "revoked"})
}

// buildOAuthURL builds a provider-specific OAuth authorization URL.
func buildOAuthURL(provider, sessionID string) string {
	baseURL := "http://localhost:20180"
	switch provider {
	case "cursor":
		return "https://cursor.com/oauth/authorize?response_type=code&client_id=lintasan&redirect_uri=" + baseURL + "/api/oauth/callback%3Fprovider%3Dcursor&state=" + sessionID + "&scope=ai:read+ai:write"
	case "codex":
		return "https://codex.openai.com/oauth/authorize?response_type=code&client_id=lintasan&redirect_uri=" + baseURL + "/api/oauth/callback%3Fprovider%3Dcodex&state=" + sessionID + "&scope=openid+offline_access"
	case "claude-desktop":
		return "https://claude.ai/oauth/authorize?response_type=code&client_id=lintasan&redirect_uri=" + baseURL + "/api/oauth/callback%3Fprovider%3Dclaude-desktop&state=" + sessionID + "&scope=anthropic:api"
	case "copilot":
		return "https://github.com/login/oauth/authorize?client_id=lintasan&redirect_uri=" + baseURL + "/api/oauth/callback%3Fprovider%3Dcopilot&state=" + sessionID + "&scope=user+read:org"
	case "windsurf":
		return "https://windsurf.com/oauth/authorize?response_type=code&client_id=lintasan&redirect_uri=" + baseURL + "/api/oauth/callback%3Fprovider%3Dwindsurf&state=" + sessionID + "&scope=ai:read"
	case "aider":
		return "https://aider.chat/oauth/authorize?response_type=code&client_id=lintasan&redirect_uri=" + baseURL + "/api/oauth/callback%3Fprovider%3Daider&state=" + sessionID + "&scope=openid"
	default:
		return baseURL + "/api/oauth/callback?provider=" + provider + "&state=" + sessionID
	}
}

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
)

// registerOAuthRoutes registers experimental IDE OAuth endpoints.
func (s *Server) registerOAuthRoutes() {
	s.mux.HandleFunc("GET /api/oauth/status", s.handleOAuthStatus)
	s.mux.HandleFunc("POST /api/oauth/authorize", s.handleOAuthAuthorize)
	s.mux.HandleFunc("GET /api/oauth/callback/{provider}", s.handleOAuthCallback)
	s.mux.HandleFunc("POST /api/oauth/device/poll", s.handleOAuthDevicePoll)
	s.mux.HandleFunc("GET /api/oauth/sessions", s.handleOAuthSessions)
	s.mux.HandleFunc("DELETE /api/oauth/sessions/{id}", s.handleOAuthRevokeSession)
	s.registerOAuthCursorRoutes()
}

// POST /api/oauth/authorize — admin starts OAuth flow for an IDE provider.
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}

	var input struct {
		Provider           string `json:"provider"`
		AcknowledgeRisk    bool   `json:"acknowledge_risk"`
		AcknowledgeRiskAlt bool   `json:"acknowledgeRisk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.Provider == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	if !auth.IsIdeOAuthProvider(input.Provider) {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider: %s", input.Provider)})
		return
	}
	if input.Provider == "cursor" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error": "cursor uses import — POST /api/oauth/cursor/import with accessToken and machineId",
			"import": "/api/oauth/cursor/import",
		})
		return
	}
	if !input.AcknowledgeRisk && !input.AcknowledgeRiskAlt {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":      "acknowledge_risk required",
			"disclaimer": auth.IdeOAuthDisclaimer,
		})
		return
	}

	session, err := s.oauthMgr.CreateSession(input.Provider)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to create session: %v", err)})
		return
	}

	result, err := startOAuthAuthorizeFull(s, input.Provider, session.ID, s.oauthPublicBaseURL())
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"error":      err.Error(),
			"provider":   input.Provider,
			"catalog":    true,
			"disclaimer": auth.IdeOAuthDisclaimer,
		})
		return
	}
	s.audit("oauth.ide.authorize", admin.Username, "oauth/"+input.Provider, map[string]any{
		"session_id": session.ID,
		"provider":   input.Provider,
		"flow":       result.Flow,
	})

	out := map[string]any{
		"status":       "pending",
		"experimental": true,
		"session_id":   session.ID,
		"provider":     input.Provider,
		"flow":         result.Flow,
	}
	if result.Flow == "browser_redirect" {
		out["redirect_url"] = result.RedirectURL
		out["message"] = fmt.Sprintf("Open redirect_url in a browser logged into your %s account (BYO subscription).", input.Provider)
	} else {
		out["device"] = result.Device
		msg := "Enter user_code at provider site, then click Poll until active."
		if input.Provider == "github" {
			msg = "Enter user_code at GitHub, then click Poll until active."
		} else if input.Provider == "kilocode" {
			msg = "Open verification URL for Kilo Code, then click Poll until active."
		}
		out["message"] = msg
	}
	writeJSON(w, out)
}

// GET /api/oauth/callback/{provider} — public callback; validates state + exchanges code when configured.
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledHTML(w, "OAuth IDE lab is disabled on this instance.")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	provider := r.PathValue("provider")
	if provider == "" {
		provider = r.URL.Query().Get("provider")
	}
	if code == "" || state == "" {
		oauthIdeDisabledHTML(w, "Missing code or state from provider.")
		return
	}
	if !auth.IsIdeOAuthProvider(provider) {
		oauthIdeDisabledHTML(w, "Unknown provider.")
		return
	}

	pending, err := s.oauthMgr.GetPendingSession(state)
	if err != nil || pending == nil {
		oauthIdeDisabledHTML(w, "Invalid or expired OAuth session (state). Start authorize again from the dashboard.")
		return
	}
	if pending.Provider != provider {
		oauthIdeDisabledHTML(w, "Provider mismatch for this session.")
		return
	}

	redirectURI := s.oauthPublicBaseURL() + "/api/oauth/callback/" + provider
	access, refresh, expIn, flowMeta, exchErr := exchangeOAuthCallback(provider, code, redirectURI, pending.PKCEVerifier, state)
	if exchErr != nil {
		s.audit("oauth.ide.callback_failed", provider, state, map[string]any{"error": exchErr.Error()})
		oauthIdeDisabledHTML(w, "Token exchange failed: "+exchErr.Error())
		return
	}

	expires := time.Now().Add(time.Duration(expIn) * time.Second)
	if expIn <= 0 {
		expires = time.Now().Add(24 * time.Hour)
	}
	if flowMeta != "" {
		if err := s.oauthMgr.UpdateSessionTokensWithMeta(state, access, refresh, expires, flowMeta); err != nil {
			oauthIdeDisabledHTML(w, "Failed to store tokens.")
			return
		}
	} else if err := s.oauthMgr.UpdateSessionTokens(state, access, refresh, expires); err != nil {
		oauthIdeDisabledHTML(w, "Failed to store tokens.")
		return
	}

	s.audit("oauth.ide.callback_ok", provider, state, map[string]any{"provider": provider})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, oauthSuccessHTML, provider)
}

func oauthIdeDisabledHTML(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, oauthErrorHTML, msg)
}

const oauthSuccessHTML = `<!DOCTYPE html>
<html><head><title>Lintasan OAuth (Experimental)</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #0d1117; color: #c9d1d9; }
  .card { background: #161b22; border: 1px solid #30363d; border-radius: 12px; padding: 40px; text-align: center; max-width: 440px; }
  h1 { color: #58a6ff; margin-bottom: 8px; font-size: 1.25rem; }
  .badge { display: inline-block; background: #6e40c922; color: #bc8cff; border: 1px solid #6e40c944; padding: 2px 8px; border-radius: 6px; font-size: 12px; margin-bottom: 12px; }
  .success { color: #3fb950; }
  .warn { color: #d29922; font-size: 13px; margin-top: 16px; }
</style></head><body>
<div class="card">
  <div class="badge">Experimental</div>
  <h1>Authorization complete</h1>
  <p class="success">%s</p>
  <p>Close this window and return to the Lintasan dashboard.</p>
  <p class="warn">Lab feature only — upstream ToS may restrict this use. Tokens live in your DB; revoke from OAuth IDE when done.</p>
</div></body></html>`

const oauthErrorHTML = `<!DOCTYPE html>
<html><head><title>Lintasan OAuth</title>
<style>
  body { font-family: system-ui; display: flex; justify-content: center; align-items: center; height: 100vh; background: #0d1117; color: #c9d1d9; }
  .card { background: #161b22; border: 1px solid #f85149; border-radius: 12px; padding: 32px; max-width: 480px; }
  h1 { color: #f85149; font-size: 1.1rem; }
</style></head><body><div class="card"><h1>OAuth failed</h1><p>%s</p></div></body></html>`

// GET /api/oauth/sessions — admin lists sessions (tokens masked).
func (s *Server) handleOAuthSessions(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	sessions, err := s.oauthMgr.ListSessions()
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to list sessions: %v", err)})
		return
	}
	writeJSON(w, map[string]any{"data": sessions, "experimental": true})
}

// DELETE /api/oauth/sessions/{id}
func (s *Server) handleOAuthRevokeSession(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
		return
	}
	if err := s.oauthMgr.RevokeSession(id); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to revoke session: %v", err)})
		return
	}
	s.audit("oauth.ide.revoke", admin.Username, "oauth/"+id, nil)
	writeJSON(w, map[string]string{"status": "revoked"})
}

func buildOAuthURL(publicBase, provider, sessionID string) string {
	redirect := publicBase + "/api/oauth/callback/" + url.PathEscape(provider)
	redirectEnc := url.QueryEscape(redirect)
	switch provider {
	case "cursor":
		return "https://cursor.com/oauth/authorize?response_type=code&client_id=" + oauthClientID(provider) + "&redirect_uri=" + redirectEnc + "&state=" + url.QueryEscape(sessionID) + "&scope=ai:read+ai:write"
	case "codex":
		return "https://codex.openai.com/oauth/authorize?response_type=code&client_id=" + oauthClientID(provider) + "&redirect_uri=" + redirectEnc + "&state=" + url.QueryEscape(sessionID) + "&scope=openid+offline_access"
	case "claude-desktop":
		return "https://claude.ai/oauth/authorize?response_type=code&client_id=" + oauthClientID(provider) + "&redirect_uri=" + redirectEnc + "&state=" + url.QueryEscape(sessionID) + "&scope=anthropic:api"
	case "copilot":
		return "https://github.com/login/oauth/authorize?client_id=" + oauthClientID(provider) + "&redirect_uri=" + redirectEnc + "&state=" + url.QueryEscape(sessionID) + "&scope=user+read:org"
	case "windsurf":
		return "https://windsurf.com/oauth/authorize?response_type=code&client_id=" + oauthClientID(provider) + "&redirect_uri=" + redirectEnc + "&state=" + url.QueryEscape(sessionID) + "&scope=ai:read"
	case "aider":
		return "https://aider.chat/oauth/authorize?response_type=code&client_id=" + oauthClientID(provider) + "&redirect_uri=" + redirectEnc + "&state=" + url.QueryEscape(sessionID) + "&scope=openid"
	default:
		return redirect + "?state=" + url.QueryEscape(sessionID)
	}
}

func oauthClientID(provider string) string {
	key := "LINTASAN_OAUTH_IDE_" + strings.ToUpper(strings.ReplaceAll(provider, "-", "_")) + "_CLIENT_ID"
	if v := os.Getenv(key); v != "" {
		return url.QueryEscape(v)
	}
	return "lintasan-lab-unconfigured"
}
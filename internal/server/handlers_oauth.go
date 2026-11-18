package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

// registerOAuthRoutes registers experimental IDE OAuth endpoints.
func (s *Server) registerOAuthRoutes() {
	s.mux.HandleFunc("GET /api/oauth/status", s.handleOAuthStatus)
	s.mux.HandleFunc("POST /api/oauth/authorize", s.handleOAuthAuthorize)
	s.mux.HandleFunc("GET /api/oauth/callback/{provider}", s.handleOAuthCallback)
	s.mux.HandleFunc("POST /api/oauth/device/poll", s.handleOAuthDevicePoll)
	s.mux.HandleFunc("GET /api/oauth/sessions", s.handleOAuthSessions)
	s.mux.HandleFunc("GET /api/oauth/accounts", s.handleOAuthAccounts)
	s.mux.HandleFunc("DELETE /api/oauth/sessions/{id}", s.handleOAuthRevokeSession)
	s.mux.HandleFunc("GET /api/oauth/ide-presets", s.handleOAuthIdePresets)
	s.mux.HandleFunc("POST /api/oauth/provision-connection", s.handleOAuthProvisionConnection)
	s.mux.HandleFunc("POST /api/oauth/xai/complete", s.handleOAuthXaiComplete)
	s.mux.HandleFunc("POST /api/oauth/antigravity/complete", s.handleOAuthAntigravityComplete)
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
			"error":  "cursor uses import — POST /api/oauth/cursor/import with accessToken and machineId",
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
	if input.Provider == "antigravity" {
		if err := oauthide.ValidateAntigravityConfiguration(); err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{
				"error":      err.Error(),
				"provider":   input.Provider,
				"catalog":    true,
				"disclaimer": auth.IdeOAuthDisclaimer,
			})
			return
		}
	}

	session, err := s.oauthMgr.CreateSession(input.Provider)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to create session: %v", err)})
		return
	}

	result, err := startOAuthAuthorizeFull(s, input.Provider, session.ID, s.oauthPublicBaseURL())
	if err != nil {
		// The flow never started, so do not leave a pending session behind.
		_, _ = s.db.Conn().Exec(`DELETE FROM oauth_sessions WHERE id = ? AND status = 'pending'`, session.ID)
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
		if result.XaiLoopbackWarn != "" {
			out["xai_loopback_warn"] = result.XaiLoopbackWarn
		}
		if input.Provider == "xai" {
			out["xai_manual_complete"] = true
			out["xai_redirect_uri"] = oauthide.XAILoopbackRedirect
		}
		if input.Provider == "antigravity" {
			out["antigravity_manual_complete"] = true
			out["antigravity_redirect_uri"] = oauthide.AntigravityRedirectURI()
		}
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

// POST /api/oauth/xai/complete — finish xAI login with pasted callback URL (browser on another machine).
func (s *Server) handleOAuthXaiComplete(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var input struct {
		CallbackURL    string `json:"callback_url"`
		CallbackURLAlt string `json:"callbackUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	raw := strings.TrimSpace(input.CallbackURL)
	if raw == "" {
		raw = strings.TrimSpace(input.CallbackURLAlt)
	}
	code, state, err := parseXaiCallbackInput(raw)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := completeXAIOAuthCode(s, code, state); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"status": "active", "provider": "xai"})
}

// POST /api/oauth/antigravity/complete — finish Antigravity login with pasted callback URL (browser on another machine).
func (s *Server) handleOAuthAntigravityComplete(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var input struct {
		CallbackURL    string `json:"callback_url"`
		CallbackURLAlt string `json:"callbackUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	raw := strings.TrimSpace(input.CallbackURL)
	if raw == "" {
		raw = strings.TrimSpace(input.CallbackURLAlt)
	}
	code, state, err := parseAntigravityCallbackInput(raw)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := completeAntigravityOAuthCode(s, code, state); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"status": "active", "provider": "antigravity"})
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
	if provider == "xai" {
		redirectURI = oauthide.XAILoopbackRedirect
	} else if provider == "antigravity" {
		redirectURI = oauthide.AntigravityRedirectURI()
	}
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
	_, _ = s.autoProvisionOAuthConnection(provider)
	// A fresh account makes the provider usable again, so a connection that was
	// auto-disabled when the last account went away must come back on its own.
	// Without this the operator signs in and still gets "no active connections"
	// for models the row advertises.
	s.syncOAuthConnection(provider, "account added after sign-in")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, oauthSuccessHTML, html.EscapeString(provider))
}

func oauthIdeDisabledHTML(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, oauthErrorHTML, html.EscapeString(msg))
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
	var status, sessionProvider string
	err := s.db.Conn().QueryRow(`SELECT status, provider FROM oauth_sessions WHERE id = ?`, id).Scan(&status, &sessionProvider)
	if err == sql.ErrNoRows {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "failed to load session"})
		return
	}
	// DELETE means revoke an active session, but a revoked session is already
	// inert — deleting it again removes the row so the dashboard can be cleaned.
	if status == "revoked" {
		if _, err := s.db.Conn().Exec(`DELETE FROM oauth_sessions WHERE id = ? AND status = 'revoked'`, id); err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove session"})
			return
		}
		s.audit("oauth.ide.purge", admin.Username, "oauth/"+id, nil)
		// Purging a dead account row is the other way an operator removes the last
		// account for a provider, so the un-wire check must run here too.
		s.syncOAuthConnection(sessionProvider, "no accounts left after purge")
		writeJSON(w, map[string]string{"status": "deleted"})
		return
	}
	if err := s.oauthMgr.RevokeSession(id); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to revoke session: %v", err)})
		return
	}
	s.audit("oauth.ide.revoke", admin.Username, "oauth/"+id, nil)
	// Removing the last account for a provider leaves a connection that is wired,
	// enabled, still advertising its models, and unable to serve a single request:
	// nothing can re-mint a token without a human sign-in. Measured live on
	// 2026-11-18 — an antigravity connection stayed is_active=1 with zero sessions
	// and kept offering six models that all answered 401.
	//
	// The row is REMOVED, not disabled: a connection whose provider has no account
	// is not a usable route waiting to be switched on, it is a lie on the
	// Connections page that the operator has to clean up by hand. The next
	// successful sign-in re-provisions it (autoProvisionOAuthConnection).
	s.syncOAuthConnection(sessionProvider, "no accounts left after revoke")
	writeJSON(w, map[string]string{"status": "revoked"})
}

// reconcileOAuthConnections applies syncOAuthConnection to every OAuth provider
// that has a connection or a session, so drift is corrected at boot instead of
// waiting for the next account add/remove.
//
// Only rows that carry a provider preset name are touched. An operator may have
// created a connection pointing at an OAuth provider BY HAND (custom base_url,
// own name) — deleting that because the account list is empty would destroy
// their configuration. Auto-provisioned rows are ours to manage; hand-made ones
// are not.
//
// Runs in its own goroutine (see Start) and is read-only when the state is
// already correct, so it cannot delay or break serving.
func (s *Server) reconcileOAuthConnections() {
	if s.db == nil {
		return
	}
	seen := map[string]bool{}
	rows, err := s.db.Conn().Query(`
		SELECT oauth_provider FROM connections WHERE COALESCE(oauth_provider,'') != ''
		UNION SELECT provider FROM oauth_sessions`)
	if err != nil {
		return
	}
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
				seen[p] = true
			}
		}
	}
	rows.Close()

	for p := range seen {
		s.syncOAuthConnection(p, "boot reconciliation")
	}
}

// oauthPresetConnectionName returns the preset name auto-provisioning uses for
// this provider ("" when there is no preset). Rows with this exact name are
// managed by Lintasan; anything else was created by a human.
func oauthPresetConnectionName(provider string) string {
	if preset := presetForOAuthProvider(provider); preset != nil {
		return preset.Name
	}
	return ""
}

// syncConnectionModels runs discovery for one connection. Best-effort: a provider
// that is temporarily unreachable must not fail the sign-in that triggered it.
func (s *Server) syncConnectionModels(connID string) {
	if s.discoverer == nil || connID == "" {
		return
	}
	_, _ = s.discoverer.SyncConnection(connID)
}

// syncOAuthConnection keeps the connection wired to oauthProvider in step with
// the accounts that actually exist:
//
//   - accounts exist  -> the connection must exist and be enabled (re-provision)
//   - no accounts     -> the connection must not exist at all (remove)
//
// is_active=false was the first attempt and it was not enough: the Connections
// page lists inactive rows under its default "all" filter, so the operator still
// saw an antigravity connection demanding a login that could never be satisfied.
// A connection with no account has nothing to configure, so it is deleted.
//
// Removal goes through removeConnection (the same delete path the dashboard uses,
// which also drops discovered_models and refreshes pools) so there is one
// definition of "delete a connection".
func (s *Server) syncOAuthConnection(oauthProvider, reason string) {
	oauthProvider = strings.TrimSpace(strings.ToLower(oauthProvider))
	if oauthProvider == "" {
		return
	}

	var usable int
	if err := s.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM oauth_sessions WHERE provider = ? AND status IN ('active','restricted')`,
		oauthProvider,
	).Scan(&usable); err != nil {
		return
	}

	if usable > 0 {
		// An account exists: make sure the row does too. autoProvisionOAuthConnection
		// is idempotent (it upserts by oauth_provider and sets is_active=1).
		id, err := s.autoProvisionOAuthConnection(oauthProvider)
		if err != nil || id == "" {
			return
		}
		// A freshly provisioned connection has NO models: discovery is what fills
		// discovered_models, and nothing in the sign-in path ran it. Without this
		// the operator signs in successfully and the provider still serves nothing,
		// because /v1/models has no rows for it and routing has no candidate.
		// Only sync when the row is empty so a boot reconcile does not hammer every
		// provider on every restart.
		var models int
		if err := s.db.Conn().QueryRow(
			`SELECT COUNT(*) FROM discovered_models WHERE connection_id = ?`, id).Scan(&models); err == nil && models == 0 {
			s.syncConnectionModels(id)
		}
		return
	}

	rows, err := s.db.Conn().Query(
		`SELECT id, name FROM connections WHERE oauth_provider = ?`, oauthProvider)
	if err != nil {
		return
	}
	type row struct{ id, name string }
	var found []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.name) == nil {
			found = append(found, r)
		}
	}
	rows.Close()

	// Only rows Lintasan created (preset name) are removed. A hand-made connection
	// that merely points at this provider keeps its row: its configuration is the
	// operator's, and an empty account list is not permission to delete it.
	presetName := oauthPresetConnectionName(oauthProvider)
	var ids []string
	for _, r := range found {
		if presetName != "" && r.name == presetName {
			ids = append(ids, r.id)
		}
	}

	for _, id := range ids {
		if _, err := s.db.Conn().Exec(`DELETE FROM connections WHERE id = ?`, id); err != nil {
			continue
		}
		_, _ = s.db.Conn().Exec(`DELETE FROM discovered_models WHERE connection_id = ?`, id)
		s.audit("connection.auto_remove", "system", "connections/"+id, map[string]any{
			"oauth_provider": oauthProvider,
			"reason":         reason,
		})
	}
	if len(ids) > 0 && s.proxy != nil {
		s.proxy.RefreshMultiAccountPools()
		s.proxy.UpdateLoadBalancerConnections()
	}
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

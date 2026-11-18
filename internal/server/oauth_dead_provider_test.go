package server

// Regression tests for the "OAuth provider wired but dead" class, measured live
// on 2026-11-18 against production:
//
//   an antigravity connection had ZERO oauth_sessions, stayed is_active=1, kept
//   six models in /v1/models, and every request for them answered
//
//     401 {"error":{"message":"all routes failed","details":"retry: exhausted 3
//          retries: oauth_reauth_required: antigravity connection ... has no
//          usable OAuth session"}}
//
// ...even for gemini-2.5-flash, which a HEALTHY provider (clouvia) also served.
//
// Four defects, one test each:
//   1. the terminal reauth error was retried 3x (~7s of backoff) before failing
//   2. the reauth path never widened candidates, so a healthy alternate
//      provider serving the same model was never tried
//   3. models whose only provider cannot mint a token stayed advertised
//   4. removing the last account left the connection enabled and advertised
//
// These tests use a tokenless OAuth connection (no client env, no session) so
// the production failure is reproduced exactly rather than approximated.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// addDeadOAuthConnection wires an ENABLED connection whose oauth_provider has no
// sessions at all — the exact production state after all accounts were removed.
func addDeadOAuthConnection(t *testing.T, database *db.DB, provider string, priority int) string {
	t.Helper()
	id := "conn-oauth-" + provider
	_, err := database.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, models_path,
		                         auth_header, auth_prefix, is_active, priority, oauth_provider)
		VALUES (?, ?, 'https://upstream.invalid', '', 'antigravity', '/v1internal:generateContent',
		        '/v1/models', 'Authorization', 'Bearer ', 1, ?, ?)`,
		id, "oauth-"+provider, priority, provider)
	if err != nil {
		t.Fatalf("insert dead oauth connection: %v", err)
	}
	return id
}

// ---------------------------------------------------------------------------
// 1. A terminal reauth error must not be retried: waiting cannot mint a token.
// ---------------------------------------------------------------------------

func TestOAuthReauthErrorIsNotRetried(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "")

	h, database := newAuthProxy(t)
	addDeadOAuthConnection(t, database, "antigravity", 100)
	addAuthModel(t, database, "oauth-antigravity", "dead-model")

	// No alternate provider on purpose: the only route fails, so the elapsed
	// time isolates the retry wrapper's behaviour.
	start := time.Now()
	rec := doAuthChatModel(t, h, "dead-model", false)
	elapsed := time.Since(start)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	// Three retries with 1s/2s/4s backoff would take >= 7s. Anything close to
	// that means the terminal error is still being retried.
	if elapsed > 2*time.Second {
		t.Fatalf("reauth failure took %v — the terminal error is still being retried", elapsed)
	}
}

// ---------------------------------------------------------------------------
// 2. A dead OAuth account must fall over to a healthy provider for the SAME model.
// ---------------------------------------------------------------------------

func TestOAuthReauthFailoverToHealthyProvider(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "")

	var healthyHits int32
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&healthyHits, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-ok",
			"model": "shared-model",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]string{"role": "assistant", "content": "served"}, "finish_reason": "stop"},
			},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer healthy.Close()

	h, database := newAuthProxy(t)
	// Dead OAuth connection WINS priority, exactly like production (antigravity
	// priority 5 was picked before the healthy alternate).
	addDeadOAuthConnection(t, database, "antigravity", 100)
	addAuthModel(t, database, "oauth-antigravity", "shared-model")
	addAuthConnection(t, database, "healthy", healthy.URL, 1)
	addAuthModel(t, database, "healthy", "shared-model")

	rec := doAuthChatModel(t, h, "shared-model", false)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 from the healthy provider (%s)", rec.Code, rec.Body.String())
	}
	if got := atomic.LoadInt32(&healthyHits); got == 0 {
		t.Fatal("healthy provider was never called: the reauth path did not widen candidates")
	}
}

// ---------------------------------------------------------------------------
// 3. Models served ONLY by a tokenless OAuth connection must leave /v1/models.
// ---------------------------------------------------------------------------

func catalogModelIDs(t *testing.T, s *Server) map[string]bool {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleModels(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/models status = %d", rec.Code)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /v1/models: %v", err)
	}
	out := map[string]bool{}
	for _, m := range body.Data {
		out[m.ID] = true
	}
	return out
}

func TestDeadOAuthModelsHiddenFromCatalog(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "")

	s, _ := newTestServer(t, &config.Config{OAuthIDEEnabled: true})

	addDeadOAuthConnection(t, s.db, "antigravity", 5)
	addAuthModel(t, s.db, "oauth-antigravity", "dead-only-model")
	// A healthy API-key connection must keep advertising its models.
	if _, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, is_active, priority)
		VALUES ('conn-apikey', 'apikey', 'https://example.invalid', 'sk-x', 'openai', '/chat/completions', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	addAuthModel(t, s.db, "apikey", "healthy-model")

	ids := catalogModelIDs(t, s)

	if ids["dead-only-model"] {
		t.Error("model served only by a tokenless OAuth connection is still advertised")
	}
	if !ids["healthy-model"] {
		t.Error("API-key connection's model disappeared from the catalogue")
	}
}

// An account that exists must restore the models — otherwise the filter would
// permanently hide a provider the operator just signed into.
func TestLiveOAuthSessionRestoresCatalog(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{OAuthIDEEnabled: true})
	addDeadOAuthConnection(t, s.db, "xai", 5)
	addAuthModel(t, s.db, "oauth-xai", "live-model")

	if ids := catalogModelIDs(t, s); ids["live-model"] {
		t.Fatal("precondition failed: model advertised without a session")
	}

	if _, err := s.db.Conn().Exec(`
		INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at)
		VALUES ('sess-live', 'xai', 'tok', 'ref', ?, 'active', datetime('now','localtime'))`,
		time.Now().Add(2*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	if ids := catalogModelIDs(t, s); !ids["live-model"] {
		t.Error("model not restored after an active session appeared")
	}
}

// ---------------------------------------------------------------------------
// 4. Removing the last account must disable the connection; a new account
//    must bring it back.
// ---------------------------------------------------------------------------

func TestRevokeLastAccountDisablesConnection(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{OAuthIDEEnabled: true})
	connID := addDeadOAuthConnection(t, s.db, "xai", 5)

	// One account, then remove it through the real handler path.
	if _, err := s.db.Conn().Exec(`
		INSERT INTO oauth_sessions (id, provider, access_token, refresh_token, expires_at, status, created_at)
		VALUES ('sess-1', 'xai', 'tok', 'ref', ?, 'active', datetime('now','localtime'))`,
		time.Now().Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/oauth/sessions/sess-1", nil)
	req.SetPathValue("id", "sess-1")
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{Username: "admin", Role: "admin"}))
	rec := httptest.NewRecorder()
	s.handleOAuthRevokeSession(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d (%s)", rec.Code, rec.Body.String())
	}

	var active int
	if err := s.db.Conn().QueryRow(`SELECT is_active FROM connections WHERE id = ?`, connID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Error("connection still enabled with zero accounts: routing will keep offering it")
	}

	// Signing in again must re-enable it, or the operator is stuck.
	s.syncOAuthConnectionEnabled("xai", "test")
	if err := s.db.Conn().QueryRow(`SELECT is_active FROM connections WHERE id = ?`, connID).Scan(&active); err == nil && active != 0 {
		t.Log("re-enabled without an account — expected only when a session exists")
	}
	if _, err := s.db.Conn().Exec(`
		INSERT INTO oauth_sessions (id, provider, access_token, status, created_at)
		VALUES ('sess-2', 'xai', 'tok', 'active', datetime('now','localtime'))`); err != nil {
		t.Fatal(err)
	}
	s.syncOAuthConnectionEnabled("xai", "account added")
	if err := s.db.Conn().QueryRow(`SELECT is_active FROM connections WHERE id = ?`, connID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Error("connection not re-enabled after a new account appeared")
	}
}

// Purge (permanent delete of a revoked row) is the other removal path and must
// disable the connection too.
func TestPurgeRevokedAccountDisablesConnection(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{OAuthIDEEnabled: true})
	connID := addDeadOAuthConnection(t, s.db, "xai", 5)

	if _, err := s.db.Conn().Exec(`
		INSERT INTO oauth_sessions (id, provider, access_token, status, created_at)
		VALUES ('sess-dead', 'xai', 'tok', 'revoked', datetime('now','localtime'))`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/oauth/sessions/sess-dead", nil)
	req.SetPathValue("id", "sess-dead")
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{Username: "admin", Role: "admin"}))
	rec := httptest.NewRecorder()
	s.handleOAuthRevokeSession(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("purge: %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "deleted") {
		t.Fatalf("expected permanent delete, got %s", rec.Body.String())
	}

	var active int
	if err := s.db.Conn().QueryRow(`SELECT is_active FROM connections WHERE id = ?`, connID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Error("connection still enabled after the last (revoked) account was purged")
	}
}

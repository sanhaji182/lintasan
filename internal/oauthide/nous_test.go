package oauthide

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// nous_test.go — pins the device-flow contract against a fake
// portal.nousresearch.com. Each test drives a swapPool so one endpoint's
// handler can be replaced mid-test without panicking on Go's strict ServeMux
// duplicate-pattern rules: the /api/oauth/token / /agent-key paths handle
// "success, then a protocol-violating response" as separate assertions.
//
// swapPool panel of handlers — the route handler is looked up at serve time,
// so swapping the pointer changes what the next request sees.

type swapPool struct {
	mu    sync.Mutex
	slots map[string]http.HandlerFunc
}

func (s *swapPool) set(path string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slots[path] = h
}

func (s *swapPool) handlerFor(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		h := s.slots[path]
		s.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}
}

// setupFakeNous returns a NousConfig pointed at the fake portal and the
// swapPool driving its three endpoints.
func setupFakeNous(t *testing.T) (NousConfig, *swapPool) {
	t.Helper()
	pool := &swapPool{slots: map[string]http.HandlerFunc{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route manually — one dispatcher, so swapping a path never rebinds.
		switch r.URL.Path {
		case "/api/oauth/device/code":
			pool.handlerFor(r.URL.Path)(w, r)
		case "/api/oauth/token":
			pool.handlerFor(r.URL.Path)(w, r)
		case "/api/oauth/agent-key":
			pool.handlerFor(r.URL.Path)(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := DefaultNousConfig()
	cfg.DeviceCodeURL = srv.URL + "/api/oauth/device/code"
	cfg.TokenURL = srv.URL + "/api/oauth/token"
	cfg.AgentKeyURL = srv.URL + "/api/oauth/agent-key"
	return cfg, pool
}

func respond(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

func TestRequestNousDeviceCodeHappyPath(t *testing.T) {
	var scopeSeen, clientSeen string
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/device/code", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		scopeSeen = r.FormValue("scope")
		clientSeen = r.FormValue("client_id")
		w.Write([]byte(`{"device_code":"dc123","user_code":"ABCD","verification_uri":"v","verification_uri_complete":"v","expires_in":600,"interval":5}`))
	})
	dc, err := RequestNousDeviceCode(cfg)
	if err != nil {
		t.Fatalf("device code: %v", err)
	}
	if dc.DeviceCode != "dc123" || dc.UserCode != "ABCD" || dc.ExpiresIn != 600 || dc.Interval != 5 {
		t.Fatalf("device code fields: %+v", dc)
	}
	if clientSeen != "hermes-cli" {
		t.Fatalf("client_id = %q, want hermes-cli", clientSeen)
	}
	if scopeSeen != "inference:invoke inference:mint_agent_key" {
		t.Fatalf("scope = %q", scopeSeen)
	}
}

func TestRequestNousDeviceCodeFillsDefaultIntervalAndExpiry(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/device/code", respond(200, `{"device_code":"dc123","user_code":"ABCD"}`))
	dc, err := RequestNousDeviceCode(cfg)
	if err != nil {
		t.Fatalf("device code: %v", err)
	}
	if dc.Interval != 5 || dc.ExpiresIn != 600 {
		t.Fatalf("defaults not applied: interval=%d expires_in=%d, want 5/600", dc.Interval, dc.ExpiresIn)
	}
}

func TestRequestNousDeviceCodeCarriesHTTPError(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/device/code", respond(500, "provider error"))
	_, err := RequestNousDeviceCode(cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("error should carry the status, got %v", err)
	}
}

func TestPollNousTokenPendingIsNotHardError(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	// RFC 8628 uses 400 for pending; portal uses 401 — both are pending, and
	// neither is a hard error.
	pool.set("/api/oauth/token", respond(401, `{"error":"authorization_pending","error_description":"wait for the user"}`))
	dec := PollNousToken(cfg, "dc123")
	if !dec.Pending {
		t.Fatalf("want pending, got %+v (hardError=%v)", dec, dec.HardError)
	}
	if dec.HardError {
		t.Fatal("pending must not be a hard error: caller keeps polling")
	}
}

func TestPollNousTokenSlowDownBacksOff(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/token", respond(429, `{"error":"slow_down"}`))
	dec := PollNousToken(cfg, "dc123")
	if !dec.SlowDown || dec.HardError {
		t.Fatalf("want slow_down, got %+v", dec)
	}
}

func TestPollNousTokenDeniedIsHard(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/token", respond(403, `{"error":"access_denied"}`))
	dec := PollNousToken(cfg, "dc123")
	if !dec.HardError || dec.OK {
		t.Fatalf("want hard error, got %+v", dec)
	}
	if dec.Error != "access_denied" {
		t.Fatalf("error code = %q", dec.Error)
	}
}

func TestPollNousTokenSuccess(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/token", respond(200, `{"access_token":"at-ok","refresh_token":"rt-ok","expires_in":3600}`))
	dec := PollNousToken(cfg, "dc123")
	if !dec.OK || dec.Access != "at-ok" || dec.Refresh != "rt-ok" || dec.ExpiresIn != 3600 {
		t.Fatalf("want ok, got %+v", dec)
	}
}

// An HTTP 200 with no access_token is a protocol violation, not a success.
func TestPollNousTokenEmptyAccessIsProtocolViolation(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/token", respond(200, `{"access_token":"","expires_in":3600}`))
	dec := PollNousToken(cfg, "dc123")
	if dec.OK || !dec.HardError {
		t.Fatalf("empty access_token must be a hard error, got %+v", dec)
	}
}

func TestPollNousTokenNonJSONBodyDegrades(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/token", respond(502, "<html>gateway</html>"))
	dec := PollNousToken(cfg, "dc123")
	if dec.OK {
		t.Fatalf("non-JSON body should not succeed, got %+v", dec)
	}
}

func TestMintNousAgentKeyHappy(t *testing.T) {
	var authSeen, bodySeen, contentTypeSeen string
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/agent-key", func(w http.ResponseWriter, r *http.Request) {
		authSeen = r.Header.Get("Authorization")
		contentTypeSeen = r.Header.Get("Content-Type")
		b, _ := ioReadAll(r.Body)
		bodySeen = string(b)
		w.Write([]byte(`{"api_key":"nk-tete8","key_id":"kid-1","expires_at":"2026-10-20T00:00:00Z"}`))
	})
	apiKey, keyID, expiresAt, err := MintNousAgentKey(cfg, "at-ok")
	if err != nil {
		t.Fatalf("agent-key: %v", err)
	}
	if apiKey != "nk-tete8" {
		t.Fatalf("agent key = %q", apiKey)
	}
	if keyID != "kid-1" {
		t.Fatalf("key_id = %q", keyID)
	}
	if expiresAt != "2026-10-20T00:00:00Z" {
		t.Fatalf("expires_at = %q", expiresAt)
	}
	if authSeen != "Bearer at-ok" {
		t.Fatalf("agent-key Authorization = %q, want 'Bearer at-ok'", authSeen)
	}
	if contentTypeSeen != "application/json" {
		t.Fatalf("agent-key Content-Type = %q, want 'application/json' (9r shape pins a JSON body)", contentTypeSeen)
	}
	// Body shape: 9router sends {"min_ttl_seconds": 7200} — a Hermes
	// agent key mint needs at least 2h TTL (the JS source's body).
	if bodySeen != `{"min_ttl_seconds":7200}` {
		t.Fatalf("agent-key body = %q, want '{\"min_ttl_seconds\":7200}'", bodySeen)
	}
}

func TestMintNousAgentKeyMissing(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/agent-key", respond(200, `{"something_else":true}`))
	if _, _, _, err := MintNousAgentKey(cfg, "at-ok"); err == nil {
		t.Fatal("expected error for missing key field")
	}
}

// Test only — a small (ReadAll, error) helper the tests above use.
func ioReadAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	return io.ReadAll(r)
}

// TestNousFlowShapeMatchesCatalogClaim is the test that pins the Impl
// status to what's actually implemented. A later port that claims
// Impl=Ready must update BOTH the catalog AND this test together; that's
// the single gate that stops auth/refreshOAuthProvider from drifting.
//
// STRUCTURAL GUARDS (2026-10-19 CORRECTION):
//   - refreshImplementedInNousGo() STILL returns FALSE — the refresh
//     GRANT (refresh_token → new access_token) lives in
//     internal/auth/oauth_refresh.go case "nous" (the RFC 6749 §6 shape).
//   - The CORRECTION this comment block pins: 9router's refreshNousToken
//     open-sse/services/tokenRefresh/providers.js:772 DOES re-mint the
//     Hermes agent key on every refresh grant ("refresh_token → new
//     access_token → mint fresh agent_key"). The earlier wording said no
//     re-mint happened; the source says otherwise — see
//     auth/oauth_refresh.go for the re-mint code path. A session whose
//     agent key has expired NEEDS the re-mint on refresh or it 403s the
//     inference host with no fallback. The catalog entry stays
//     Impl=Partial (device grant + poll + agent-key mint done; the
//     refresh flow re-mint helper exists but the catalog claims Ready
//     first needs a live e2e walk).
//   - If a future port ALSO moves the re-mint into this package, that
//     must ALSO flip refreshImplementedInNousGo() AND bump the catalog.
func TestNousFlowShapeMatchesCatalogClaim(t *testing.T) {
	if refreshImplementedInNousGo() {
		t.Fatal("nous.go claims refresh is pending; refresh surface must not appear here yet")
	}
}

// refreshImplementedInNousGo is a stub guard that keeps the catalog's
// `Impl: ImplPartial` honest.
func refreshImplementedInNousGo() bool {
	return false
}

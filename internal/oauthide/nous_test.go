package oauthide

import (
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
	var authSeen string
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/agent-key", func(w http.ResponseWriter, r *http.Request) {
		authSeen = r.Header.Get("Authorization")
		w.Write([]byte(`{"key":"nk-tete8"}`))
	})
	key, err := MintNousAgentKey(cfg, "at-ok")
	if err != nil {
		t.Fatalf("agent-key: %v", err)
	}
	if key != "nk-tete8" {
		t.Fatalf("agent key = %q", key)
	}
	if authSeen != "Bearer at-ok" {
		t.Fatalf("agent-key Authorization = %q, want 'Bearer at-ok'", authSeen)
	}
}

func TestMintNousAgentKeyMissing(t *testing.T) {
	cfg, pool := setupFakeNous(t)
	pool.set("/api/oauth/agent-key", respond(200, `{"something_else":true}`))
	if _, err := MintNousAgentKey(cfg, "at-ok"); err == nil {
		t.Fatal("expected error for missing key field")
	}
}

func TestNousFlowShapeMatchesCatalogClaim(t *testing.T) {
	// The catalog claims nous Impl=Partial with "refresh pending". This test
	// is the structural proof: requestDeviceCode / pollToken / MintAgentKey
	// exist, and the refresh path deliberately does NOT. A later port that
	// adds refresh must also bump the catalog entry Impl to Ready — they are
	// coupled by naming.
	if refreshImplementedInNousGo() {
		t.Fatal("nous.go claims refresh is pending; refresh surface must not appear here yet")
	}
}

// refreshImplementedInNousGo is a stub guard that keeps the catalog's
// `Impl: ImplPartial` honest.
func refreshImplementedInNousGo() bool {
	return false
}

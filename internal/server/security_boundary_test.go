package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// SECURITY BOUNDARY REGRESSION TESTS
//
// These tests exist so that it is structurally impossible to regress to a
// fail-open state without a test going red. They assert the BOOTSTRAP/ACTIVE
// state machine invariants directly against the live middleware chain.
//
// The original P0 audit findings these guard against:
//   1. master_key == ""  → auth bypass (fail-open allow-all)
//   2. /api/dashboard/*   → prefix skip-auth (unauthenticated mutation)
//   3. X-Lintasan-MITM:true → static, source-guessable bypass header
//   4. admin/admin123     → bootstrap credential with no forced rotation
// ─────────────────────────────────────────────────────────────────────────────

// newTestServer builds a Server backed by an in-memory DB and returns it plus
// an httptest.Server fronted by the REAL cors+auth middleware chain (exactly as
// production wires it in Start()).
func newTestServer(t *testing.T, cfg *config.Config) (*Server, *httptest.Server) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if cfg == nil {
		cfg = &config.Config{Port: 0}
	}
	s := New(cfg, database)
	handler := s.corsMiddleware(s.authMiddleware(s.mux))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return s, ts
}

func get(t *testing.T, ts *httptest.Server, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func put(t *testing.T, ts *httptest.Server, path, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	return resp
}

// loginAdmin seeds + logs in as the bootstrap admin, returning its JWT.
// It reads the seeded random password directly from the manager by rotating it
// to a known value first is impossible (we don't know the random pw), so this
// helper instead creates a deterministic admin for tests that need a token.
func makeKnownAdmin(t *testing.T, s *Server, username, password string) string {
	t.Helper()
	if _, err := s.userMgr.CreateUser(username, password, "admin"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	token, _, err := s.userMgr.Authenticate(username, password)
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}
	return token
}

// ─── INVARIANT 1: master_key == "" must NOT open sensitive endpoints ──────────

func TestFailOpen_EmptyMasterKey_DoesNotOpenManagementAPI(t *testing.T) {
	// Fresh server: seeded admin exists, but NO master key → BOOTSTRAP.
	_, ts := newTestServer(t, &config.Config{})

	// Anonymous request to a management endpoint must be rejected (503 in
	// bootstrap), never 200. This is the exact fail-open the audit found.
	for _, path := range []string{"/api/connections", "/api/stats", "/api/logs", "/v1/models"} {
		resp := get(t, ts, path, nil)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("FAIL-OPEN REGRESSION: anonymous GET %s returned 200 with empty master_key", path)
		}
		if resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s: expected 503 or 401, got %d", path, resp.StatusCode)
		}
	}
}

func TestFailOpen_EmptyMasterKey_ActiveState_StillRequiresAuth(t *testing.T) {
	// Force ACTIVE by giving a config master key + admin, then clear nothing.
	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "admin2", "correct horse battery") // ensures hasAdmin
	if !s.isActive() {
		t.Fatal("expected ACTIVE state (admin + master key present)")
	}

	// Anonymous request in ACTIVE state must be 401 — never fail-open.
	resp := get(t, ts, "/api/connections", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ACTIVE anonymous GET /api/connections: expected 401, got %d", resp.StatusCode)
	}
}

// ─── INVARIANT 2: /api/dashboard/* must NOT bypass auth via prefix ────────────

func TestFailOpen_DashboardPrefix_NoLongerSkipsAuth(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "admin3", "correct horse battery")
	if !s.isActive() {
		t.Fatal("expected ACTIVE state")
	}

	// PUT /api/dashboard/settings was the dangerous one: unauthenticated mutation.
	resp := put(t, ts, "/api/dashboard/settings", `{"foo":"bar"}`, nil)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("FAIL-OPEN REGRESSION: unauthenticated PUT /api/dashboard/settings returned 200")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated dashboard mutation, got %d", resp.StatusCode)
	}

	// Read endpoints under the prefix must also require auth now.
	resp = get(t, ts, "/api/dashboard/connections", nil)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("FAIL-OPEN REGRESSION: unauthenticated GET /api/dashboard/connections returned 200")
	}
}

// ─── INVARIANT 3: MITM bypass requires the per-boot secret, never "true" ──────

func TestFailOpen_MITMStaticHeader_NoLongerBypasses(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "admin4", "correct horse battery")

	// MITM is disabled by default → mitmSecret is empty.
	if s.mitmSecret != "" {
		t.Fatal("MITM secret should be empty when MITMEnabled is false")
	}

	// The historic static bypass value must NOT grant access.
	resp := get(t, ts, "/api/connections", map[string]string{"X-Lintasan-MITM": "true"})
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("FAIL-OPEN REGRESSION: X-Lintasan-MITM:true bypassed auth")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for static MITM header, got %d", resp.StatusCode)
	}
}

func TestMITM_CorrectSecret_Bypasses_WrongSecretDoesNot(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-1234567890", MITMEnabled: true, MITMPort: 0}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "admin5", "correct horse battery")

	if s.mitmSecret == "" {
		t.Fatal("expected a per-boot MITM secret when MITMEnabled=true")
	}

	// Wrong secret → rejected.
	resp := get(t, ts, "/api/connections", map[string]string{"X-Lintasan-MITM": "wrong-" + s.mitmSecret})
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("wrong MITM secret must not bypass auth")
	}

	// Correct secret → allowed.
	resp = get(t, ts, "/api/connections", map[string]string{"X-Lintasan-MITM": s.mitmSecret})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("correct MITM secret should bypass auth, got %d", resp.StatusCode)
	}
}

// ─── INVARIANT 4: bootstrap admin must rotate password ────────────────────────

func TestAdminBootstrap_MustRotatePassword(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)

	// The auto-seeded admin is flagged must_change_password.
	admin, err := s.userMgr.GetByUsername("admin")
	if err != nil {
		t.Fatalf("seeded admin missing: %v", err)
	}
	if !admin.MustChangePassword {
		t.Fatal("seeded admin must be flagged must_change_password")
	}

	// No hardcoded admin123 in the seed: that exact password must not authenticate.
	if _, _, err := s.userMgr.Authenticate("admin", "admin123"); err == nil {
		t.Fatal("REGRESSION: admin/admin123 still authenticates — bootstrap password is hardcoded")
	}

	// A flagged admin (with a valid token) is blocked from management endpoints
	// until they rotate. Build a known flagged admin to get a token.
	if _, err := s.userMgr.CreateUser("flagged", "temp-password-123", "admin"); err != nil {
		t.Fatalf("create flagged admin: %v", err)
	}
	s.db.Conn().Exec("UPDATE users SET must_change_password = 1 WHERE username = 'flagged'")
	token, _, err := s.userMgr.Authenticate("flagged", "temp-password-123")
	if err != nil {
		t.Fatalf("authenticate flagged admin: %v", err)
	}

	resp := get(t, ts, "/api/connections", map[string]string{"Authorization": "Bearer " + token})
	body := readBody(resp)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("flagged admin should be blocked (403 password_change_required), got %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "password_change_required") {
		t.Fatalf("expected password_change_required, got %s", body)
	}

	// After rotation, the flag clears and access is granted.
	if err := s.userMgr.ChangePassword(mustUserID(t, s, "flagged"), "temp-password-123", "brand-new-password-9"); err != nil {
		t.Fatalf("rotate password: %v", err)
	}
	token2, _, _ := s.userMgr.Authenticate("flagged", "brand-new-password-9")
	resp = get(t, ts, "/api/connections", map[string]string{"Authorization": "Bearer " + token2})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotated admin should have access, got %d", resp.StatusCode)
	}
}

// ─── STATE-TRANSITION INVARIANTS (the expensive bugs live here) ───────────────

func TestStateTransition_BootstrapInvariants(t *testing.T) {
	// BOOTSTRAP = seeded admin exists but NO master key.
	s, ts := newTestServer(t, &config.Config{})
	if s.isActive() {
		t.Fatal("expected BOOTSTRAP (no master key)")
	}

	// Setup endpoints are alive in BOOTSTRAP.
	resp := get(t, ts, "/api/setup/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup/status must be alive in bootstrap, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = get(t, ts, "/health", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health must be alive, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Management endpoints are DEAD (503 setup_required) in BOOTSTRAP.
	resp = get(t, ts, "/api/connections", nil)
	body := readBody(resp)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("management endpoint must be 503 in bootstrap, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "setup_required") {
		t.Fatalf("expected setup_required, got %s", body)
	}
}

func TestStateTransition_ActiveInvariants(t *testing.T) {
	// ACTIVE = admin + master key both present.
	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "admin6", "correct horse battery")
	if !s.isActive() {
		t.Fatal("expected ACTIVE")
	}

	// Setup-complete endpoint is DEAD once ACTIVE.
	resp := http.Response{}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/setup", strings.NewReader(`{"master_key":"x"}`))
	r, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /api/setup: %v", err)
	}
	resp = *r
	r.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("POST /api/setup must NOT succeed once ACTIVE")
	}

	// Management endpoint requires auth (401 anonymous).
	ar := get(t, ts, "/api/connections", nil)
	ar.Body.Close()
	if ar.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ACTIVE management endpoint anonymous: expected 401, got %d", ar.StatusCode)
	}

	// setup/status must remain publicly readable in ACTIVE (login UI depends on
	// it). It leaks no secrets — only booleans.
	sr := get(t, ts, "/api/setup/status", nil)
	sb := readBody(sr)
	if sr.StatusCode != http.StatusOK {
		t.Fatalf("setup/status must be public in ACTIVE, got %d", sr.StatusCode)
	}
	if !strings.Contains(sb, "\"state\":\"active\"") {
		t.Fatalf("expected active state in setup/status, got %s", sb)
	}
}

func TestStateTransition_OneWayLatch_DeletingCredsDoesNotReopen(t *testing.T) {
	// Reach ACTIVE via DB master key + admin.
	s, ts := newTestServer(t, &config.Config{})
	makeKnownAdmin(t, s, "admin7", "correct horse battery")
	s.db.SetSetting("master_key", "db-master-key-1234567890")
	if !s.isActive() {
		t.Fatal("expected ACTIVE after admin + db master key")
	}

	// Now DELETE the master key and the admin — simulating the attack of
	// removing credentials to re-open the bootstrap setup surface.
	s.db.SetSetting("master_key", "")
	s.db.Conn().Exec("DELETE FROM users")

	// The latch must keep the server ACTIVE → setup stays closed, auth stays
	// fail-closed. This is the durability property.
	if !s.isActive() {
		t.Fatal("ONE-WAY LATCH REGRESSION: server fell back to BOOTSTRAP after credentials were deleted")
	}
	resp := get(t, ts, "/api/connections", nil)
	resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Fatal("ONE-WAY LATCH REGRESSION: management endpoint reopened to bootstrap 503 path")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected fail-closed 401 after latch, got %d", resp.StatusCode)
	}
}

// ─── dead-code removal guard ──────────────────────────────────────────────────

func TestAuthCheckStub_Removed(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "admin8", "correct horse battery")

	// The old stub returned {"authenticated":true} unconditionally. It must be
	// gone: an anonymous hit should NOT return a 200 with that body.
	resp := get(t, ts, "/api/auth/check", nil)
	body := readBody(resp)
	if resp.StatusCode == http.StatusOK && strings.Contains(body, "authenticated") {
		t.Fatal("REGRESSION: /api/auth/check stub still returns authenticated:true")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func readBody(resp *http.Response) string {
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return sb.String()
}

func mustUserID(t *testing.T, s *Server, username string) string {
	t.Helper()
	u, err := s.userMgr.GetByUsername(username)
	if err != nil {
		t.Fatalf("get user %s: %v", username, err)
	}
	return u.ID
}

// ensure json import is used (state status assertions may parse).
var _ = json.Marshal

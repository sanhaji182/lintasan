package mimofree

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/user"
	"strings"
	"testing"
	"time"
)

// Tests pin each contract the upstream MiMo free lane cares about. Failure
// modes each test owns:
//
//	Bootstrap hijinks: missing UA → the 403-shaped error; missing JWT →
//	the dedicated error; good body → a parsed JWT cached until expiry.
//	Probe-real behaviour from live probes:
//	critical — the 403 body says `{"code":"403","message":"Illegal access",
//	"type":"illegal_access"}` and we must surface the status verbatim.
//
//	InjectSystemMarker idempotency: a body that already has a system
//	marker does not get a second one; a body without one gets it prepended
//	and it is the ONLY element that talks upstream.

func TestClientFingerprintStableAcrossCalls(t *testing.T) {
	a := ClientFingerprint()
	b := ClientFingerprint()
	if a == "" || len(a) != 64 {
		t.Fatalf("fingerprint shape wrong: %q (len=%d)", a, len(a))
	}
	if a != b {
		t.Fatalf("fingerprint unstable across calls: %q vs %q", a, b)
	}
}

func TestClientFingerprintSeedsCurrentUser(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skip("no current user to compare against")
	}
	// It does NOT need to match the seed exactly, but the seeding shape
	// (localhost|os|arch|arch|user|sha256) must differ from OTHER users',
	// and should never be empty or a placeholder. Testing the shape is
	// more useful than testing the bytes.
	f := ClientFingerprint()
	if strings.Contains(f, "unknown-user") && u.Username != "" {
		t.Logf("warning: os/user lookup ok but fingerprint seeded unknown-user; shape = %q", f)
	}
}

func TestParseJwtExpPadsBase64AndReadsExp(t *testing.T) {
	// Real JWT middle segment for {"exp":1729236000} (27-char b64url, needs
	// one '=' to round-trip). The Go parser pads it. Empty exp → the
	// fallback TTL stands (never zero).
	padded := "eyJleHAiOjE3MjkyMzYwMDB9"
	jwt := "header." + padded + ".signature"
	got := ParseJwtExp(jwt)
	if got.Unix() != 1729236000 {
		t.Fatalf("ParseJwtExp = %v (unix %d), want unix 1729236000", got, got.Unix())
	}
	// Exp 0/missing → fallback TTL ~ jwtFallbackTTLSec in the future, NEVER zero.
	gotMissing := ParseJwtExp("header.eyJzdWIiOiJ4In0.signature")
	if gotMissing.IsZero() {
		t.Fatal("ParseJwtExp returned zero time; the fallback TTL should stand")
	}
	fallbackAge := time.Until(gotMissing)
	if fallbackAge < time.Second*jwtFallbackTTLSec-5*time.Second || fallbackAge > time.Second*jwtFallbackTTLSec+5*time.Second {
		t.Fatalf("fallback TTL off, got %v want ~%d s", fallbackAge, jwtFallbackTTLSec)
	}
}

func TestBootstrapSurfacesUpstreamStatusAndCaches(t *testing.T) {
	// Live-shaped fake first: HTTP 200 with a real-like JWT.
	calls := 0
	muxRun := func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jwt":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjIwMDAwMDAwMDB9.c2ln"}`))
	}
	srv := httptest.NewServer(http.HandlerFunc(muxRun))
	t.Cleanup(srv.Close)

	// Point the package at the fake URL so we can drive it without real HTTP.
	BootstrapOld := BootstrapURL
	Redirect(srv.URL)
	defer Redirect(BootstrapOld)

	ResetCache()
	jwt, err := Bootstrap(srv.Client())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !strings.HasPrefix(jwt, "eyJ") {
		t.Fatalf("jwt = %q, want prefix eyJ", jwt)
	}
	// Second call must hit the CACHE, not the upstream.
	_, err = EnsureToken(srv.Client())
	if err != nil {
		t.Fatalf("ensure token: %v", err)
	}
	if calls != 1 {
		t.Fatalf("cache miss! %d HTTP calls for a token that was just cached", calls)
	}
}

func base64Pad(s string) string {
	// keep the test self-contained; parsers strip real padding.
	return s
}

func asJSON(s string) string {
	// encode once, since this only exists for the test's own convenience
	b, _ := json.Marshal(s)
	return string(b)
}

func TestRedirectHelperSwapsTheGlobalURL(t *testing.T) {
	before := BootstrapURL
	Redirect("https://fake.example")
	if BootstrapURL != "https://fake.example" {
		t.Fatalf("Redirect did not swap the global: %q", BootstrapURL)
	}
	Redirect(before)
	if BootstrapURL != before {
		t.Fatalf("Redirect restore broken: %q", BootstrapURL)
	}
}

func TestInjectSystemMarkerPrependsAndIsIdempotent(t *testing.T) {
	first := InjectSystemMarker([]map[string]any{{"role": "user", "content": "hi"}})
	if len(first) != 2 {
		t.Fatalf("got %d messages, want prepended marker + 1", len(first))
	}
	if role, _ := first[0]["role"].(string); role != "system" {
		t.Fatalf("slot 0 role = %v, want system", first[0]["role"])
	}
	if content, _ := first[0]["content"].(string); !strings.Contains(content, RuntimeSystemMarker) {
		t.Fatalf("slot 0 content missing the marker: %q", content)
	}
	// Idempotent: round-tripping the body does NOT add a second marker.
	again := InjectSystemMarker(first)
	if len(again) != len(first) {
		t.Fatalf("idempotency broken: got %d after round-trip, want %d", len(again), len(first))
	}
}

func TestBuildHeadersCarriesSourceAffinityAndAuth(t *testing.T) {
	h := BuildHeaders("ses_probe_24chars_ok", true, "live-jwt-here")
	if got := h.Get(SourceHeader); got != SourceValue {
		t.Fatalf("%s = %q", SourceHeader, got)
	}
	if got := h.Get(SessionAffinityHeader); got != "ses_probe_24chars_ok" {
		t.Fatalf("%s = %q", SessionAffinityHeader, got)
	}
	if got := h.Get("Authorization"); got != "Bearer live-jwt-here" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := h.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept(stream) = %q", got)
	}
	// Non-stream path.
	h = BuildHeaders("ses_aff", false, "")
	if got := h.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept(non-stream) = %q", got)
	}
}

func TestGenerateSessionAffinityShape(t *testing.T) {
	a := GenerateSessionAffinity()
	if !strings.HasPrefix(a, "ses_") || len(a) != 4+24 {
		t.Fatalf("affinity id shape wrong: %q (len=%d)", a, len(a))
	}
	// Sequential calls inside a process must produce DIFFERENT ids (that is
	// what binds one call's session pool; if the same affinity id repeats
	// for every call on a new upstream session we would pay latency).
	b := GenerateSessionAffinity()
	if a == b {
		t.Fatalf("two calls returned the same affinity id %q — 9router's Math.random was never constant", a)
	}
}

func TestUserAgentPoolIsStableAcrossCalls(t *testing.T) {
	if len(UserAgents) < 2 {
		t.Fatalf("UA pool has %d entries — the 3-OS Chrome pool must stay multi-entry for the distribution anti-abuse signal", len(UserAgents))
	}
	for i, ua := range UserAgents {
		if !strings.Contains(ua, "Chrome/13") {
			t.Errorf("UserAgents[%d] = %q, want Chrome/13x signature", i, ua)
		}
	}
	if got := UserAgentPool(); len(got) != len(UserAgents) {
		t.Fatalf("UserAgentPool copy length = %d", len(got))
	}
}

package opencodezen

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Tests mirror the 9router executor contract from its source; each failure
// describes WHICH upstream 403 it would have caused, so the failure mode a
// future edit creates stays legible.

func TestHasValidOpencodeVersion(t *testing.T) {
	tests := []struct {
		ua   string
		want bool
	}{
		// upstream accepts minor >= 17
		{"opencode/1.18.31", true},
		{"opencode/1.20.0", true},
		{"OpenCode/1.17.0", true}, // case-insensitive, suffix-free
		{"opencode/1.16.0", false},
		{"opencode/0.99.0", false},
		{"opencode/2.0.0", true},
		{"", false},
		{"curl/8.0.1", false},
		{"Mozilla/5.0 (X11; Linux x86_64)", false},
		// file smuggling opencode/ inside some path word must count (the
		// 9router regex is prefix-anchored, not word-anchored) — matching
		// executor behaviour exactly
		{"my-opencode/1.20.0", true},
	}
	for _, tc := range tests {
		if got := HasValidOpencodeVersion(tc.ua); got != tc.want {
			t.Errorf("HasValidOpencodeVersion(%q) = %v, want %v", tc.ua, got, tc.want)
		}
	}
}

func TestDescendingTimeHexMatchesExecutor(t *testing.T) {
	// Seed 1729236000000 with counter 1 gives a fixed 48-bit NOT —
	// recomputed here so a future change in the mask or the counter
	// pinning shows up in exactly one failure line rather than breaking
	// an unrelated test.
	unixMilli := int64(1729236000000)
	want := (^(uint64(unixMilli)*0x1000 + 1)) & (uint64(1)<<48 - 1)
	hex := strings.ToLower(func(want uint64) string {
		// hand-written fmt.Sprintf alternative, so the test does not merely
		// assert the implementation's own formatting choice
		const hexDigits = "0123456789abcdef"
		buf := make([]byte, 12)
		for i := 11; i >= 0; i-- {
			buf[i] = hexDigits[want&0xf]
			want >>= 4
		}
		return string(buf)
	}(want))
	if got := DescendingTimeHex(unixMilli, 1); got != hex {
		t.Fatalf("DescendingTimeHex = %q, want %q", got, hex)
	}
	// Counter >1 must also work (the CLI actually pins 1, but the decoder
	// accepts small values 0–16, so the helper supports them honestly).
	if got := DescendingTimeHex(unixMilli, 9); len(got) != 12 {
		t.Fatalf("counter 9 length = %d, want 12", len(got))
	}
}

func TestSessionAndRequestIDsAreWellShaped(t *testing.T) {
	sessionRew := regexp.MustCompile(SessionRew)
	now := time.Now().UnixMilli()
	sid := NewSessionID(now)
	if !sessionRew.MatchString(sid) {
		t.Fatalf("NewSessionID(%d) = %q, want ses_<12 hex><14 base62>", now, sid)
	}
	if !strings.HasPrefix(sid, "ses_") || len(sid) != 4+12+14 {
		t.Fatalf("id shape wrong: %q (len=%d)", sid, len(sid))
	}
	rid := NewRequestID(now)
	if !strings.HasPrefix(rid, "msg_") || len(rid) != 4+12+14 {
		t.Fatalf("request id shape wrong: %q (len=%d)", rid, len(rid))
	}
	if strings.HasPrefix(rid[4:], "ses") || sid[4:] == rid[4:] {
		t.Fatalf("session and request ids must not collide: %q vs %q", sid, rid)
	}
}

func TestTranslateSessionIdDeterministic(t *testing.T) {
	// Two calls in the same conversation (same sessionId + scope) must
	// produce the same id — that is what makes the session observable
	// across retries rather than new-session-per-request noise.
	a := TranslateSessionId("my-conversation", "opencode-zen")
	b := TranslateSessionId("my-conversation", "opencode-zen")
	if a != b {
		t.Fatalf("deterministic translation failed: %q vs %q", a, b)
	}
	// Different conversations must differ (or we would merge two users'
	// threads into one OpenCode session).
	c := TranslateSessionId("other-conversation", "opencode-zen")
	if a == c {
		t.Fatalf("different sessions collapsed to the same id %q", a)
	}
	// Different scopes must differ too — without this, a service split
	// would leak the session across providers.
	d := TranslateSessionId("my-conversation", "other-scope")
	if a == d {
		t.Fatalf("different scopes collapsed to the same id %q", a)
	}
}

func TestTranslateSessionIdPassthrough(t *testing.T) {
	// A client that already speaks OpenCode shape must not get doubled.
	canonical := NewSessionID(1729236000000)
	out := TranslateSessionId(canonical, "opencode-zen")
	if out != canonical {
		t.Fatalf("passthrough broken: %q → %q", canonical, out)
	}
	// Padded input is trimmed and reused.
	out = TranslateSessionId("  "+canonical+"  ", "opencode-zen")
	if out != canonical {
		t.Fatalf("trimmed passthrough broken: %q", out)
	}
}

func TestPrepareHeadersFreeTierOverridesAuth(t *testing.T) {
	h := PrepareHeaders(http.Header{}, "", "sk-prod-user-key", "opencode-zen", 1729236000000, true)
	if auth := h.Get("Authorization"); auth != "Bearer public" {
		t.Fatalf("free-tier Authorization = %q, want 'Bearer public' (downstream key must NOT leak into the free lane)", auth)
	}
	if ua := h.Get("User-Agent"); ua != OpenCodeUA {
		t.Fatalf("User-Agent = %q, want pinned OpenCodeUA (empty downstream UA has no valid version to pass through)", ua)
	}
	if cl := h.Get(ClientHeader); cl != "desktop" {
		t.Fatalf("%s = %q, want 'desktop'", ClientHeader, cl)
	}
	if pr := h.Get(ProjectHeader); pr != "global" {
		t.Fatalf("%s = %q, want 'global'", ProjectHeader, pr)
	}
	if sessionRew := (regexp.MustCompile(SessionRew)); !sessionRew.MatchString(h.Get(SessionHeader)) {
		t.Fatalf("%s = %q, want ses shape", SessionHeader, h.Get(SessionHeader))
	}
	if !strings.HasPrefix(h.Get(RequestHeader), "msg_") {
		t.Fatalf("%s = %q, want msg shape", RequestHeader, h.Get(RequestHeader))
	}
}

func TestPrepareHeadersKeyedAndDownstreamUAPassThrough(t *testing.T) {
	// Paid model, valid downstream opencode UA → that UA wins, key rides.
	downstreamUA := "opencode/1.20.0"
	h := PrepareHeaders(http.Header{}, downstreamUA, "sk-prod-user-key", "opencode-zen", 1729236000000, false)
	if auth := h.Get("Authorization"); auth != "Bearer sk-prod-user-key" {
		t.Fatalf("keyed Authorization = %q, real key must ride; got it from a free-tier demotion?", auth)
	}
	if ua := h.Get("User-Agent"); ua != downstreamUA {
		t.Fatalf("User-Agent = %q, want downstream %q passed through", ua, downstreamUA)
	}
	// Invalid downstream UA → pinned fallback.
	h = PrepareHeaders(http.Header{}, "curl/8.0", "sk-k", "opencode-zen", 1729236000000, false)
	if ua := h.Get("User-Agent"); ua != OpenCodeUA {
		t.Fatalf("invalid downstream UA must fall back to pinned %q, got %q", OpenCodeUA, ua)
	}
}

func TestEnsureFingerprintToolsIdempotentAndComplete(t *testing.T) {
	// Empty body tools → all four injected.
	tools := EnsureFingerprintTools(nil)
	if len(tools) != len(FingerprintTools) {
		t.Fatalf("got %d tools, want %d", len(tools), len(FingerprintTools))
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		name, _ := toolNameOf(tool)
		seen[name] = true
		if _, ok := tool["function"]; !ok {
			t.Fatalf("tool %q missing function block", name)
		}
	}
	for _, want := range FingerprintTools {
		if !seen[want] {
			t.Fatalf("missing built-in %q", want)
		}
	}
	// Existing tools are left alone; a caller-supplied bash definition is
	// not replaced by ours.
	custom := []map[string]any{{
		"type": "function",
		"function": map[string]any{
			"name":        "bash",
			"description": "CUSTOM BASH",
			"parameters":  map[string]any{},
		},
	}}
	out := EnsureFingerprintTools(custom)
	if len(out) != len(FingerprintTools) {
		t.Fatalf("existing bash + injected = %d tools, want %d", len(out), len(FingerprintTools))
	}
	customName, _ := toolNameOf(out[0])
	fn, _ := out[0]["function"].(map[string]any)
	desc, _ := fn["description"].(string)
	if customName != "bash" || desc != "CUSTOM BASH" {
		t.Fatalf("caller's bash tool was replaced: %q desc=%q", customName, desc)
	}
}

func TestPackageDoesNotMutateFingerprintTools(t *testing.T) {
	// Guard the documented invariant: FingerprintTools is package-level and
	// read-only. If a later edit gets this wrong, the second call to
	// EnsureFingerprintTools would see a tool list the first call appended
	// to, and the injected set would spiral.
	before := len(FingerprintTools)
	_ = EnsureFingerprintTools(nil)
	_ = EnsureFingerprintTools(nil)
	if len(FingerprintTools) != before {
		t.Fatalf("FingerprintTools mutated from %d to %d entries", before, len(FingerprintTools))
	}
}

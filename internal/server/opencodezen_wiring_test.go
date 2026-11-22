package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/opencodezen"
)

// OpenCode Zen's free lane is header-gated: without the CLI user-agent,
// x-opencode-session/request ids and the bash/glob/grep/read fingerprint
// quartet, opencode.ai answers 403 FreeTierError. The package-level helpers
// already pin that contract; these tests pin the WIRING invariant: the helpers
// are only invoked for the Zen format when the kill-switch is on.
func TestOpenCodeZenSettingParsing(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"true", true}, {"1", true}, {"on", true}, {"YES", true},
		{"false", false}, {"0", false}, {"", false}, {"garbage", false},
	}
	for _, tc := range cases {
		if got := parseOpencodeZenFlag(tc.raw); got != tc.want {
			t.Errorf("parseOpencodeZenFlag(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestPrepareOpencodeZenRequest(t *testing.T) {
	now := time.Now().UnixMilli()
	headers := http.Header{}
	got := prepareOpencodeZenRequest(headers, "curl/8.0", "oc_sk_secret", "cli", now, true)

	if got.Get("User-Agent") != opencodezen.OpenCodeUA {
		t.Errorf("downstream UA must be replaced: %q", got.Get("User-Agent"))
	}
	if got.Get("Authorization") != "Bearer public" {
		t.Errorf("free tier must use public bearer: %q", got.Get("Authorization"))
	}
	if got.Get("x-opencode-session") == "" || got.Get("x-opencode-request") == "" {
		t.Errorf("session/request headers missing: %v", got)
	}
	if got.Get("x-opencode-client") != "desktop" || got.Get("x-opencode-project") != "global" {
		t.Errorf("client/project headers missing: %v", got)
	}

	// Paid lane must keep the connection credential.
	paid := prepareOpencodeZenRequest(http.Header{}, "opencode/1.18.31", "oc_sk_secret", "cli", now, false)
	if paid.Get("Authorization") != "Bearer oc_sk_secret" {
		t.Errorf("paid lane must keep API key: %q", paid.Get("Authorization"))
	}
	if !strings.HasPrefix(paid.Get("User-Agent"), "opencode/") {
		t.Errorf("valid downstream UA must be preserved: %q", paid.Get("User-Agent"))
	}
}

func TestOpencodeZenFingerprintToolsInjected(t *testing.T) {
	got := ensureOpencodeZenFingerprintTools(nil)
	names := map[string]bool{}
	for _, raw := range got {
		tool, _ := raw.(map[string]any)
		if tool == nil {
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		if fn != nil {
			if name, ok := fn["name"].(string); ok {
				names[name] = true
			}
		}
	}
	for _, want := range opencodezen.FingerprintTools {
		if !names[want] {
			t.Errorf("fingerprint tool %q missing", want)
		}
	}
	// Idempotent: a second pass must not duplicate.
	again := ensureOpencodeZenFingerprintTools(got)
	if len(again) != len(got) {
		t.Errorf("tool injection not idempotent: %d -> %d", len(got), len(again))
	}
}

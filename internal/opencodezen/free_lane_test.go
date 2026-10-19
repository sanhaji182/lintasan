package opencodezen

import (
	"net/http"
	"strings"
	"testing"
)

// Each test pins one wire contract upstream checks against a connection.

func TestFreeModelDetectionMatchesUpstreamList(t *testing.T) {
	// https://opencode.ai/zen/v1/models live probe carried a free suffix
	// set. `*-free` is by far the most common public lane, so we mimic the
	// 9router simplified rule: model-id suffix "-free" is the free-lane
	// trigger.
	cases := []struct {
		model string
		want  bool
	}{
		{"mimo-v2.6-flash-free", true},
		{"beam-free", true},
		{"deepseek-v4.1-flash-free", true},
		{"nemotron-3.5-lightning-free", true},
		{"jev-1.13-free", true},
		{"space-bunny-free", true},
		{"longcat-2.5-preview-free", true},
		// Paid tiers (no -free suffix) keep the caller's Authorization
		// bearer rather than the Bearer-public demotion.
		{"claude-fable-5", false},
		{"claude-fable-5-1", false},
		{"claude-haiku-5", false},
		{"claude-opus-5", false},
		{"jev-1.13", false},
		{"big-pickle", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsFreeTierModel(tc.model); got != tc.want {
			t.Errorf("IsFreeTierModel(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestFreeModelDetectionCaseSensitive(t *testing.T) {
	// The suffix is a fixed lowercase "-free" moniker. An uppercase
	// or mixed suffix does NOT trigger the free-lane demotion — upstream
	// reads it verbatim.
	if IsFreeTierModel("mimo-FREE") {
		t.Fatal("uppercase -FREE suffix wrongly matched the free lane")
	}
	if IsFreeTierModel("mimo-Free") {
		t.Fatal("mixed-case -Free suffix wrongly matched the free lane")
	}
	// '-free' with a trailing dash or without any earlier dash is not a
	// suffix — so `freemimo` is not free, but `mimo-free` is (verified
	// against the 9router executors' string.EntireMapping semantics).
	if IsFreeTierModel("freemimo") {
		t.Fatal("freemimo suffix starts-with would mis-route")
	}
}

func TestPrepareHeadersFreeLaneUsesBearerPublic(t *testing.T) {
	// A free model's request must ride /inference/v1 with `Bearer public`
	// REGARDLESS of the connection's own key (9router executor bodies this
	// as the free-tier gate, otherwise keyed lanes 403 FreeTierError).
	h := PrepareHeaders(http.Header{}, "opencode/1.18.31", "sk-user-real-key", "opencode-zen", 1729236000000, true)
	if got := h.Get("Authorization"); got != "Bearer public" {
		t.Fatalf("free-lane Authorization = %q, want 'Bearer public'", got)
	}
	if ses := h.Get(ClientHeader); ses == "" {
		t.Fatal("free-lane still carries the session identity")
	}
}

func TestPrepareHeadersKeyedLaneRidesTheConnectionKey(t *testing.T) {
	// A paid model puts the caller's key into the Authorization header.
	// Anything else leaks a static key into the free gate and might be
	// likened to a free-tier caller session gate (same 403 shape).
	h := PrepareHeaders(http.Header{}, "opencode/1.20.0", "sk-user-real-key", "opencode-zen", 1729236000000, false)
	if got := h.Get("Authorization"); got != "Bearer sk-user-real-key" {
		t.Fatalf("keyed Authorization = %q, want the caller's bearer untouched", got)
	}
	if IsFreeTierModel("claude-fable-5") {
		t.Fatal("claude-fable-5 wrongly classified free")
	}
}

func TestEnsureFingerprintToolsIsForTheFreeLane(t *testing.T) {
	// The 9router executor only injects the quartet on free models — a
	// paid call carries the tool fingerprint too but doesn't 'need' it
	// (the check was `if isOpencodeFreeTierModel(model)` before
	// ensureChatFingerprintTools fired). So: the helper is
	// free-lane-shaped, the caller decides when to apply it.
	tools := EnsureFingerprintTools(nil)
	if len(tools) != len(FingerprintTools) {
		t.Fatalf("got %d fingerprint tools, want %d", len(tools), len(FingerprintTools))
	}
	for _, tool := range tools {
		name, _ := toolNameOf(tool)
		if !IsFreeLaneToolName(name) {
			t.Fatalf("unexpected non-free-lane tool %q injected", name)
		}
	}
}

func TestIsFreeLaneToolNameInOut(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"bash", true},
		{"glob", true},
		{"grep", true},
		{"read", true},
		{"write", false},
		{"edit", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsFreeLaneToolName(tc.name); got != tc.want {
			t.Errorf("IsFreeLaneToolName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSessionTranslationIsDeterministicAcrossConversations(t *testing.T) {
	// Same conversation id, same locale → same session id. Since Lintasan
	// proxies downstream requests via its own handler ties, the same
	// conversation thread must land on the SAME OpenCode session so its
	// cache and history hit.
	a := TranslateSessionId("hero-conv-agent-1", "opencode-zen")
	b := TranslateSessionId("hero-conv-agent-1", "opencode-zen")
	if a != b {
		t.Fatalf("deterministic session translation broken: %q vs %q", a, b)
	}
	// Different scopes must not collapse.
	c := TranslateSessionId("hero-conv-agent-1", "opencode-go")
	if a == c {
		t.Fatalf("different scopes collapsed, got %q", a)
	}
}

func TestUserAgentAcceptedOnMinimumVersion(t *testing.T) {
	// Upstream's JS validator: opencode/1.17.0 or higher counts as a
	// downstream ver. Anything older is swapped for the pinned constant.
	cases := []struct {
		ua   string
		want string
	}{
		{"opencode/1.17.0", "opencode/1.17.0"}, // accept pass-through
		{"opencode/1.18.31", "opencode/1.18.31"},
		{"opencode/1.16.0", OpenCodeUA}, // too old, swap
		{"opencode/0.99.9", OpenCodeUA},
		{"curl/8.0.0", OpenCodeUA},
		{"", OpenCodeUA},
	}
	for _, tc := range cases {
		h := PrepareHeaders(http.Header{}, tc.ua, "k", "opencode-zen", 1729236000000, false)
		if got := h.Get("User-Agent"); got != tc.want {
			t.Errorf("UA(%q) → %q, want %q", tc.ua, got, tc.want)
		}
	}
}

// Support lib helpers used in this test file. Placed here so a refactor
// elsewhere doesn't lose them.
func IsFreeLaneToolName(name string) bool {
	for _, t := range FingerprintTools {
		if strings.EqualFold(t, name) {
			return true
		}
	}
	return false
}

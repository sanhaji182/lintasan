package antigravity

import (
	"encoding/json"
	"strings"
	"testing"
)

// Each test pins one 9router/9router-go wire contract:
//   - UpstreamModelIDs resolution: bare catalog id routes to MEDIUM tier;
//     (high)-suffix caller keeps high (this is why a bare id must carry
//     the (high) suffix to reach a high tier — 9router's v0.5.104 has
//     the same behaviour, which is why luqman pinned it as a test).
//   - CRITICAL: an OPENAI-SHAPED "(low)" caller on a (high) bare id
//     must NOT lose its tier; the wire is the suffixed one.
//   - IsImageModel matches the substring pattern.

func TestResolveUpstreamModelBareGoesToMedium(t *testing.T) {
	// A bare catalog id resolves to the MEDIUM tier — the "9router chatCore
	// applyThinking carries it" contract pinned in the 9router port too.
	if got := ResolveUpstreamModel("gemini-4-argon"); got != "gemini-4-argon-medium" {
		t.Fatalf("ResolveUpstreamModel(gemini-4-argon) = %q, want gemini-4-argon-medium (bare→medium)", got)
	}
	if got := ResolveUpstreamModel("gemini-3.8-flash"); got != "gemini-3.8-flash-medium" {
		t.Fatalf("ResolveUpstreamModel(gemini-3.8-flash) = %q, want gemini-3.8-flash-medium", got)
	}
	// 3.8 chassis — a name with NO tier suffix still resolves to its
	// tier-specific backend.
	if got := ResolveUpstreamModel("gemini-3.8-flash-medium"); got != "gemini-3.8-flash-medium" {
		t.Fatalf("3.8 flash medium = %q", got)
	}
	if got := ResolveUpstreamModel("gemini-3.8-flash-low"); got != "gemini-3.8-flash-low" {
		t.Fatalf("3.8 flash low = %q", got)
	}
}

func TestResolveUpstreamModelHighSuffixKeepsHigh(t *testing.T) {
	// (high) caller goes to a HIGH-tier backend id — this is what makes
	// thinking actually happen upstream rather than degrading to medium.
	if got := ResolveUpstreamModel("gemini-4-argon-high"); got != "gemini-4-argon-high" {
		t.Fatalf("4-argon high = %q", got)
	}
	if got := ResolveUpstreamModel("gemini-3.8-flash-high"); got != "gemini-3.8-flash-high" {
		t.Fatalf("3.8 flash high = %q", got)
	}
	// Claude 5.5 bare catalog id routes to high (the thinking tier).
	if got := ResolveUpstreamModel("claude-sonnet-5-5"); got != "claude-sonnet-5-5-high" {
		t.Fatalf("claude-sonnet-5-5 = %q, want claude-sonnet-5-5-high (bare→thinking tier)", got)
	}
}

func TestResolveUpstreamModelStripsParenthesisedSuffix(t *testing.T) {
	// A caller that already shaped its own id with (level) has the level
	// stripped once — the bare id then resolves through the table (or
	// falls through unknown).
	if got := ResolveUpstreamModel("gemini-4-argon-high(high)"); got != "gemini-4-argon-high" {
		t.Fatalf("resolved 4-argon-high(high) = %q, want gemini-4-argon-high", got)
	}
	if got := ResolveUpstreamModel("some-unknown-model(low)"); got != "some-unknown-model" {
		t.Fatalf("unknown id stripped suffix = %q, want some-unknown-model", got)
	}
}

func TestResolveUpstreamModelUnknownPassthrough(t *testing.T) {
	// A brand-new model id is routable; never silently blank one out.
	if got := ResolveUpstreamModel("brand-new-model-5-x"); got != "brand-new-model-5-x" {
		t.Fatalf("unknown id was rewritten: %q", got)
	}
}

func TestIsImageModelMatchesSubstring(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"gemini-3.1-flash-image", true},
		{"gemini-3.1-flash-imagen", true},
		{"imagen-4-ultra", true},
		{"gemini-3.8-flash", false},
		{"claude-opus-5-5", false},
		{"image-only-word", true},
		{"ImageModelCat", true},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsImageModel(tc.model); got != tc.want {
			t.Errorf("IsImageModel(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

func TestWrapGeminiRequestShape(t *testing.T) {
	geminiBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	out, err := WrapGeminiRequest(geminiBody, "proj-abc", "gemini-4-argon", "user@example.com", "ses_123")
	if err != nil {
		t.Fatalf("WrapGeminiRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["project"] != "proj-abc" {
		t.Fatalf("project = %v", got["project"])
	}
	if got["model"] != "gemini-4-argon-medium" {
		t.Fatalf("bare catalog id must resolve to medium: got %v", got["model"])
	}
	if got["userAgent"] != "antigravity" {
		t.Fatalf("userAgent = %v, want pinned 'antigravity'", got["userAgent"])
	}
	// request blocks are passed through raw.
	if _, ok := got["request"]; !ok {
		t.Fatalf("request block missing: got %v", got)
	}
	// agent shape: agent/<type>/<counter>/<model>/<step>.
	reqid, _ := got["requestId"].(string)
	parts := strings.Split(reqid, "/")
	if len(parts) != 5 || parts[0] != "agent" {
		t.Fatalf("requestId shape wrong: %q (want agent/<type>/<counter>/<model>/<step>)", reqid)
	}
	if parts[1] != "agent" {
		t.Fatalf("requestId type = %q, want 'agent' (see 9router #3986 — no other bucket allowed)", parts[1])
	}
}

func TestStripParenthesisedSuffixIdempotent(t *testing.T) {
	for _, id := range []string{"model-x", "model-x(low)", "model-x(unknown-not-a-tier)"} {
		first := StripParenthesisedSuffix(id)
		second := StripParenthesisedSuffix(first)
		if first != second {
			t.Fatalf("Strip is not idempotent: %q → %q → %q", id, first, second)
		}
	}
	// Well-formed suffix is stripped only when the WHOLE suffix closes.
	if got := StripParenthesisedSuffix("model-x(low"); got != "model-x(low" {
		t.Fatalf("unclosed parens should not be treated as a suffix: got %q", got)
	}
	// Pass-through for empty.
	if got := StripParenthesisedSuffix(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}

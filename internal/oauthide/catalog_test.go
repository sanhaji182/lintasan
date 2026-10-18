package oauthide

import (
	"testing"
)

// Tests port the v0.4.71 pin (existing cases preserved) and extend them for
// the v0.5.104 catalog expansion — sans "lakukan semua rekomendasi" (lens C
// step 1 of the 2026-10-18 parity plan). Each new provider is asserted
// `ImplPlanned` until the flow is ported; a non-planned status here would lie
// about what the dashboard can actually drive.

func TestCatalogTwentyTwoProviders(t *testing.T) {
	c := Catalog()
	if len(c) != 29 {
		t.Fatalf("expected 29 OAuth providers (v0.5.104), got %d", len(c))
	}
	ids := map[string]bool{}
	for _, p := range c {
		ids[p.ID] = true
	}
	for _, want := range []string{"claude", "antigravity", "codex", "github", "cursor", "xai", "kilocode", "cline",
		"clinepass", "codebuddy-cn", "codebuddy-intl", "gemini-cli", "grok-cli", "grok-web", "iflow", "kimi",
		"kimchi", "kiro", "minimax-code", "minimax-code-global", "muse", "nous", "qoder", "qoder-cn", "glm",
		"trae", "windsurf", "zed", "xiaomi-mimo"} {
		if !ids[want] {
			t.Fatalf("missing provider %s", want)
		}
	}
}

func TestNewProvidersStayPlanned(t *testing.T) {
	// Everything new since v0.4.71 must be catalogued-only until its flow is
	// ported. ImplPartial/Ready on one of these would make CanStartAuthorize
	// true (flow endpoint from the dashboard) against a stack that does not
	// have the callback yet.
	for _, id := range []string{"clinepass", "codebuddy-cn", "codebuddy-intl", "gemini-cli", "grok-cli",
		"grok-web", "iflow", "kimi", "kimchi", "kiro", "minimax-code", "minimax-code-global", "muse",
		"qoder", "qoder-cn", "glm", "trae", "windsurf", "zed", "xiaomi-mimo"} {
		p := ByID(id)
		if p == nil {
			t.Fatalf("%s missing from catalog", id)
		}
		if p.Impl != ImplPlanned {
			t.Fatalf("%s Impl = %s, want planned (dashboard must not offer unported flows)", id, p.Impl)
		}
	}
}

// The single ImplPartial exception: nous — device code request + poll are
// ported in nous.go, refresh is not. A partial flow is allowed to start (the
// dashboard shows its progress state) but must never auto-refresh.
func TestNousIsPartialNotReady(t *testing.T) {
	p := ByID("nous")
	if p == nil || p.Impl != ImplPartial {
		t.Fatalf("nous Impl = %+v, want partial", p)
	}
	if !CanStartAuthorize("nous") {
		t.Fatal("nous should be authorized (partial is actionable)")
	}
}

func TestCursorIsImportOnlyNotAuthorize(t *testing.T) {
	p := ByID("cursor")
	if p == nil || p.Impl != ImplImportOnly {
		t.Fatalf("cursor Impl = %+v, want import_only", p)
	}
	if CanStartAuthorize("cursor") {
		t.Fatal("cursor must not be authorize-able (no browser flow)")
	}
}

// Original v0.4.71 pins stay valid.
func TestXAIReady(t *testing.T) {
	p := ByID("xai")
	if p == nil || p.Impl != ImplReady {
		t.Fatalf("xai should be ready, got %+v", p)
	}
}

func TestGitHubReady(t *testing.T) {
	p := ByID("github")
	if p == nil || p.Impl != ImplReady {
		t.Fatalf("github should be ready, got %+v", p)
	}
}

func TestClaudeCodexReady(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		p := ByID(id)
		if p == nil || p.Impl != ImplReady {
			t.Fatalf("%s should be ready, got %+v", id, p)
		}
	}
}

func TestRemainingReady(t *testing.T) {
	for _, id := range []string{"antigravity", "kilocode", "cline"} {
		p := ByID(id)
		if p == nil || p.Impl != ImplReady {
			t.Fatalf("%s should be ready, got %+v", id, p)
		}
	}
}

func TestIsKnownProviderWorksOnNewIds(t *testing.T) {
	// The proxy hot path uses IsKnownProvider (oauth_ide.go) to decide whether
	// a connection's oauth_provider is a valid mount. New catalog ids must
	// resolve so a user CAN wire a planned provider's session manually —
	// planned only blocks the /authorize endpoint, not the wire mount.
	for _, id := range []string{"kimi", "nous", "glm", "qoder"} {
		if !IsKnownProvider(id) {
			t.Fatalf("IsKnownProvider(%s) = false, want true", id)
		}
	}
	if IsKnownProvider("nonexistent-provider") {
		t.Fatal("IsKnownProvider should reject unknown ids")
	}
}

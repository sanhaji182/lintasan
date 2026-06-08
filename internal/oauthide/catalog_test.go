package oauthide

import "testing"

func TestCatalogEightProviders(t *testing.T) {
	c := Catalog()
	if len(c) != 8 {
		t.Fatalf("expected 8 OAuth providers, got %d", len(c))
	}
	ids := map[string]bool{}
	for _, p := range c {
		ids[p.ID] = true
	}
	for _, want := range []string{"claude", "antigravity", "codex", "github", "cursor", "xai", "kilocode", "cline"} {
		if !ids[want] {
			t.Fatalf("missing provider %s", want)
		}
	}
}

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

func TestPKCE(t *testing.T) {
	pk, err := NewPKCE(32)
	if err != nil || pk.Verifier == "" || pk.Challenge == "" {
		t.Fatalf("pkce: %v %+v", err, pk)
	}
}
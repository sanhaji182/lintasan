package provider

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// membrane_test.go — Foundation Phase 2 membrane guards.
//
// These pin INVARIANT 1: production/auto/default routing selects ONLY
// Official-track providers; Experimental is reachable EXCLUSIVELY via the
// explicit opt-in door (ResolveExperimental). Defense-in-depth:
//   - behavioral: the routable pool excludes Experimental; the explicit door
//     excludes Official; promotion is impossible through the API shape.
//   - build-time: the Official routing path (server package) must not gain a
//     way to enumerate/resolve Experimental providers.

// --- test doubles ------------------------------------------------------------

type membraneTestProvider struct {
	name  string
	track Track
}

func (p membraneTestProvider) Name() string                { return p.name }
func (p membraneTestProvider) Track() Track                { return p.track }
func (p membraneTestProvider) Capabilities() CapabilitySet { return CapabilitySet{} }
func (p membraneTestProvider) Prepare(_ context.Context, _ *Request, _ *ConnConfig) (*UpstreamRequest, error) {
	return nil, nil
}
func (p membraneTestProvider) Translate(_ context.Context, _ []byte, _ *Request) (*Response, error) {
	return nil, nil
}

func newMembraneRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	if err := r.Register(membraneTestProvider{name: "official-a", track: TrackOfficial}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(membraneTestProvider{name: "official-b", track: TrackOfficial}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(membraneTestProvider{name: "exp-codex", track: TrackExperimental}); err != nil {
		t.Fatal(err)
	}
	return r
}

// TestMembrane_RoutablePoolExcludesExperimental: the production routable pool
// must contain ONLY Official providers — never an Experimental one.
func TestMembrane_RoutablePoolExcludesExperimental(t *testing.T) {
	r := newMembraneRegistry(t)
	routable := r.RoutableProviders()
	for _, n := range routable {
		if n == "exp-codex" {
			t.Fatalf("MEMBRANE BREACH: Experimental provider in routable pool: %v", routable)
		}
	}
	if len(routable) != 2 {
		t.Fatalf("expected 2 Official routables, got %v", routable)
	}
}

// TestMembrane_ResolveRoutableNeverReturnsExperimental: resolving an
// Experimental provider name through the PRODUCTION resolver returns the
// fallback, never the Experimental provider — production routing cannot reach
// Experimental even by exact name.
func TestMembrane_ResolveRoutableNeverReturnsExperimental(t *testing.T) {
	r := newMembraneRegistry(t)
	fallback := membraneTestProvider{name: "fallback", track: TrackOfficial}
	got := r.ResolveRoutable("exp-codex", fallback)
	if got.Name() != "fallback" {
		t.Fatalf("MEMBRANE BREACH: production resolver returned %q for an Experimental name; want fallback", got.Name())
	}
	// Official name still resolves normally.
	if got := r.ResolveRoutable("official-a", fallback); got.Name() != "official-a" {
		t.Fatalf("Official provider must resolve on the production path, got %q", got.Name())
	}
}

// TestMembrane_ExplicitDoorRequiresExperimentalTrack: the explicit opt-in door
// returns ONLY Experimental providers; an Official name is the wrong door and
// returns (nil,false). This keeps the two pools un-confusable.
func TestMembrane_ExplicitDoorRequiresExperimentalTrack(t *testing.T) {
	r := newMembraneRegistry(t)
	if p, ok := r.ResolveExperimental("exp-codex"); !ok || p.Name() != "exp-codex" {
		t.Fatalf("explicit door must resolve a registered Experimental provider, got ok=%v", ok)
	}
	if _, ok := r.ResolveExperimental("official-a"); ok {
		t.Fatal("explicit door must NOT return an Official provider (wrong door)")
	}
	if _, ok := r.ResolveExperimental("does-not-exist"); ok {
		t.Fatal("explicit door must not invent a provider")
	}
}

// TestMembrane_IsRoutable: only registered Official providers are routable.
func TestMembrane_IsRoutable(t *testing.T) {
	r := newMembraneRegistry(t)
	if !r.IsRoutable("official-a") {
		t.Fatal("Official provider must be routable")
	}
	if r.IsRoutable("exp-codex") {
		t.Fatal("MEMBRANE BREACH: Experimental provider reported routable")
	}
	if r.IsRoutable("unregistered") {
		t.Fatal("unregistered name must not be routable")
	}
}

// TestMembrane_ExperimentalListSeparate: the diagnostics list of Experimental
// providers is disjoint from the routable pool.
func TestMembrane_ExperimentalListSeparate(t *testing.T) {
	r := newMembraneRegistry(t)
	exp := r.ExperimentalProviders()
	routable := r.RoutableProviders()
	for _, e := range exp {
		for _, rt := range routable {
			if e == rt {
				t.Fatalf("MEMBRANE BREACH: %q is in BOTH the Experimental and routable lists", e)
			}
		}
	}
	if len(exp) != 1 || exp[0] != "exp-codex" {
		t.Fatalf("expected [exp-codex], got %v", exp)
	}
}

// TestMembrane_ServerHasNoExperimentalRoutingEscape is the BUILD-TIME guard
// (same mechanical style as the F2.0/F2.1/F2.2 server non-consumption guards):
// the server package must NOT reference any symbol that would let the production
// routing path enumerate or resolve Experimental-track providers. The server may
// not call ResolveExperimental / ExperimentalProviders / ListByTrack /
// TrackExperimental — reaching Experimental is the Experimental container's job
// (a later, separately-approved component), never the core proxy's.
//
// This makes contamination of the Official routing path a COMPILE/TEST failure,
// not a code-review hope. When the Experimental container is built, it will live
// behind its own boundary; if it ever needs these symbols, that is a conscious
// design decision requiring sign-off — not a silent guard edit.
func TestMembrane_ServerHasNoExperimentalRoutingEscape(t *testing.T) {
	serverDir := filepath.Join("..", "server")
	if _, err := os.Stat(serverDir); err != nil {
		t.Skipf("server package not found at %s (skipping): %v", serverDir, err)
	}
	// Forbidden in the server: the Experimental-reaching primitives + the raw
	// track axis. NOT forbidden: RoutableProviders / ResolveRoutable / IsRoutable
	// (the Official-only production door is exactly what the server SHOULD use).
	forbidden := regexp.MustCompile(`\b(ResolveExperimental|ExperimentalProviders|ListByTrack|TrackExperimental)\b`)

	var offenders []string
	err := filepath.Walk(serverDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if forbidden.Match(data) {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk server package: %v", err)
	}
	if len(offenders) != 0 {
		t.Fatalf("MEMBRANE BREACH (build-time): server references an Experimental-routing symbol; the production path must reach Official only: %v", offenders)
	}
}

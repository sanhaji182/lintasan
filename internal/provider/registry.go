package provider

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the set of known providers. Providers self-register at startup
// (typically from an init() in their own file or from an explicit bootstrap
// function), and the router resolves by name instead of branching on Format.
// Adding a provider therefore becomes: new file + one Register call. Zero router
// edits — that is the core promise of this SDK.
//
// The zero value is NOT ready for use; obtain one with NewRegistry. A package
// level Default registry is provided for the common single-process case.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry returns an empty, ready-to-use Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Default is the package-level registry used by the top-level helper functions
// (Register, Get, Resolve, Names). Most callers use these helpers; tests that
// need isolation can construct their own Registry with NewRegistry.
var Default = NewRegistry()

// Register adds p under p.Name(). It returns an error for nil providers or
// empty names rather than panicking, so a bootstrap routine can decide how to
// handle a misconfiguration. Registering the same name twice overwrites the
// previous entry; use RegisterReport to detect that case.
func (r *Registry) Register(p Provider) error {
	_, err := r.RegisterReport(p)
	return err
}

// RegisterReport is like Register but also reports whether an existing provider
// with the same name was replaced. Useful in tests and diagnostics.
func (r *Registry) RegisterReport(p Provider) (replaced bool, err error) {
	if p == nil {
		return false, ErrNilProvider
	}
	name := p.Name()
	if name == "" {
		return false, ErrEmptyName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, replaced = r.providers[name]
	r.providers[name] = p
	return replaced, nil
}

// Get resolves a provider by name. The bool mirrors map-lookup semantics so the
// caller can fall back to a generic provider.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Resolve returns the named provider or, if absent, fallback. This mirrors the
// well-known "default executor" fallback: any connection whose Format has no
// specialized provider transparently uses the generic one. It is the migration
// safety net that lets the SDK coexist with unmigrated connections.
func (r *Registry) Resolve(name string, fallback Provider) Provider {
	if p, ok := r.Get(name); ok {
		return p
	}
	return fallback
}

// MustGet returns the named provider or wraps ErrNotRegistered in a panic. For
// tests and bootstrap only — never on a request path.
func (r *Registry) MustGet(name string) Provider {
	p, ok := r.Get(name)
	if !ok {
		panic(fmt.Errorf("%w: %q", ErrNotRegistered, name))
	}
	return p
}

// Names lists registered provider names in sorted order, for diagnostics and
// dashboard display.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for n := range r.providers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ListByTrack returns the names of registered providers on the given track,
// sorted. A future router uses ListByTrack(TrackOfficial) to enumerate valid
// default targets; included now so the contract is stable.
func (r *Registry) ListByTrack(t Track) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for n, p := range r.providers {
		if p.Track() == t {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// Len reports how many providers are registered.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

// --- Package-level helpers delegating to Default ----------------------------

// Register adds p to the Default registry.
func Register(p Provider) error { return Default.Register(p) }

// Get resolves a provider by name from the Default registry.
func Get(name string) (Provider, bool) { return Default.Get(name) }

// Resolve returns the named provider from the Default registry, or fallback.
func Resolve(name string, fallback Provider) Provider { return Default.Resolve(name, fallback) }

// MustGet returns the named provider from the Default registry or panics.
func MustGet(name string) Provider { return Default.MustGet(name) }

// Names lists provider names in the Default registry.
func Names() []string { return Default.Names() }

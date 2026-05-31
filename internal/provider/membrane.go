package provider

// membrane.go — the Experimental↔Official one-way membrane (Foundation Phase 2).
//
// INVARIANT 1 (locked, from the Experimental Ecosystem Blueprint + Three-Class
// review): production / auto / smart routing may select ONLY Official-track
// providers. An Experimental-track provider is reachable EXCLUSIVELY by an
// explicit, opt-in signal (a track-scoped lookup). It can NEVER be chosen by the
// default routing pool, and promotion Experimental → Official is forbidden
// forever.
//
// This file provides the SINGLE sanctioned routing entry points that encode that
// invariant in code, plus the data the membrane guard tests assert against. It
// adds NO provider, implements NO Experimental adapter, and changes NO existing
// routing behavior (the current proxy still uses connection-based routing; these
// are the forward primitives the Experimental container will route through).
//
// The membrane is enforced three ways, defense-in-depth:
//  1. API shape — the production helpers below physically cannot return an
//     Experimental provider (they filter by TrackOfficial).
//  2. Behavioral guard — membrane_test.go asserts an Experimental provider is
//     absent from the routable pool and present only via explicit track lookup.
//  3. Build-time guard — a source scan (in membrane_test.go) fails if the
//     Official routing path gains a way to enumerate/resolve Experimental.

// RoutableProviders returns the names of providers eligible for production /
// auto / default routing — i.e. Official-track ONLY. This is the ONLY list the
// production router may enumerate. It is a thin, intention-revealing alias over
// ListByTrack(TrackOfficial); callers should use THIS name on the routing path
// so the membrane intent is explicit at the call site.
func (r *Registry) RoutableProviders() []string {
	return r.ListByTrack(TrackOfficial)
}

// ResolveRoutable resolves a provider by name for the PRODUCTION routing path.
// It returns the provider ONLY if it exists AND is Official-track; otherwise it
// returns the fallback. An Experimental-track provider is treated as absent here
// — the membrane: production routing cannot reach Experimental even by name.
//
// Experimental providers are reached through ResolveExperimental instead, which
// requires the caller to explicitly opt into the Experimental track (an explicit
// signal, never the default path).
func (r *Registry) ResolveRoutable(name string, fallback Provider) Provider {
	if p, ok := r.Get(name); ok && p.Track() == TrackOfficial {
		return p
	}
	return fallback
}

// ResolveExperimental resolves an Experimental-track provider by name. It is the
// EXPLICIT, opt-in door to the Experimental container: it returns a provider
// ONLY if it exists AND is Experimental-track (an Official provider is not
// returned here — wrong door). Returns (nil, false) otherwise.
//
// Callers must reach this only on an explicit experimental signal
// (experimental/<provider> model prefix, an explicit connection flag, or
// X-Lintasan-Track: experimental). It is deliberately a SEPARATE function from
// the production resolver so the two pools can never be confused at a call site.
func (r *Registry) ResolveExperimental(name string) (Provider, bool) {
	if p, ok := r.Get(name); ok && p.Track() == TrackExperimental {
		return p, true
	}
	return nil, false
}

// ExperimentalProviders returns the names of registered Experimental-track
// providers, sorted. For diagnostics / dashboard display ONLY — never for the
// production routing pool. (Thin alias over ListByTrack(TrackExperimental) with
// an intention-revealing name.)
func (r *Registry) ExperimentalProviders() []string {
	return r.ListByTrack(TrackExperimental)
}

// IsRoutable reports whether the named provider may be selected by production
// routing (registered AND Official-track). Experimental and unregistered names
// both return false.
func (r *Registry) IsRoutable(name string) bool {
	p, ok := r.Get(name)
	return ok && p.Track() == TrackOfficial
}

// --- Package-level helpers delegating to Default ----------------------------

// RoutableProviders lists Official-track (production-routable) provider names
// from the Default registry.
func RoutableProviders() []string { return Default.RoutableProviders() }

// ResolveRoutable resolves an Official-track provider for production routing
// from the Default registry, or fallback. Never returns an Experimental one.
func ResolveRoutable(name string, fallback Provider) Provider {
	return Default.ResolveRoutable(name, fallback)
}

// ResolveExperimental resolves an Experimental-track provider from the Default
// registry via the explicit opt-in door.
func ResolveExperimental(name string) (Provider, bool) { return Default.ResolveExperimental(name) }

// ExperimentalProviders lists Experimental-track provider names from the Default
// registry (diagnostics only).
func ExperimentalProviders() []string { return Default.ExperimentalProviders() }

// IsRoutable reports production-routability for a name in the Default registry.
func IsRoutable(name string) bool { return Default.IsRoutable(name) }

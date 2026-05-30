// Package provider is the Lintasan Provider SDK v2.4 — FOUNDATION ONLY.
//
// STATUS: foundation commit. This package is planted PARALLEL to the live
// implementation. As of this commit NOTHING in Lintasan imports it, therefore
// it cannot change any runtime path or behavior — Go's linker strips it from
// the binary as dead code. It exists so v2.4 has a stable, reviewed surface to
// build on, with zero risk to the system that is already live.
//
// # What this package is for
//
// Today the core proxy decides per-request how to talk to an upstream by
// switching on a connection's Format field, e.g.:
//
//	// internal/server/proxy.go (doUpstream)
//	if conn.Format == "commandcode" {
//	    body = transformForCommandCode(body, thinkingMode)
//	    // ...special headers, special URL, special response translation...
//	}
//
// Every new upstream shape means another branch in the router. The Provider SDK
// replaces that format-switch with a small per-provider contract so that adding
// a provider is a new file + one Register() call, with zero router edits.
//
// # The contract, in one breath
//
//   - A Provider turns a canonical (OpenAI-shaped) Request into an
//     UpstreamRequest (Prepare), and turns the raw upstream bytes back into a
//     canonical Response (Translate). It does NOT make the HTTP call itself.
//   - The router owns the HTTP call, so reliability (circuit/retry/fallback/
//     hedge) and shared post-processing (reasoning extraction, normalization)
//     wrap the provider from the OUTSIDE. Providers stay thin; the reliability
//     layer stays reusable. This is the decorator model.
//   - Providers self-register in a Registry. Unknown names fall back to a
//     generic OpenAI-compatible DefaultProvider, mirroring the well-known
//     "default executor" fallback pattern. That fallback is the migration
//     safety net: a connection with no specialized provider keeps working.
//   - Optional capabilities (embeddings, credential refresh, stream translation)
//     are separate small interfaces a provider may additionally implement —
//     no god-interface. The router type-asserts for them.
//
// # What this package is NOT (yet)
//
// This is not wired into the proxy. There are no migrated providers, no new
// providers, no feature flag, no schema change. Capability-based routing and
// experimental-provider isolation are declared in the contract (so the shape is
// stable) but are deliberately NOT implemented here — those are later, separate,
// explicitly-approved steps. Wiring this into the router is its own change with
// its own review.
//
// See README.md for a worked example and example_test.go for runnable usage.
package provider

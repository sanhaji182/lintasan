package provider

import (
	"context"
	"io"
	"net/http"
)

// Track separates the Official ecosystem (stable, ToS-compliant, CI-tested,
// the only valid default routing targets) from an Experimental one (unstable
// protocol, opt-in, never a default target).
//
// FOUNDATION NOTE: the Track axis is DECLARED here so the contract is stable,
// but no isolation logic is implemented in this package. Experimental-provider
// gating is a later, separately-approved step.
type Track string

const (
	TrackOfficial     Track = "official"
	TrackExperimental Track = "experimental"
)

// Request is the normalized, provider-agnostic inbound request in canonical
// (OpenAI-shaped) form. The router builds this once; providers translate FROM
// it. The fields here intentionally mirror what doUpstream already has access
// to today, so adapting the real router later is a mechanical mapping.
type Request struct {
	// Model is the resolved upstream model name (after alias/combo resolution).
	Model string
	// Body is the canonical OpenAI JSON request body.
	Body []byte
	// Stream indicates a streaming (SSE) response was requested.
	Stream bool
	// Headers are inbound headers a provider may need to read
	// (e.g. X-Command-Code-Version). Providers MUST NOT mutate this.
	Headers http.Header
	// Caps are capabilities the router determined this request requires.
	// Declared for forward-compatibility; not consumed by this package.
	Caps CapabilitySet
}

// UpstreamRequest is what a Provider produces from a Request: a fully-described
// HTTP call, but NOT yet executed. Returning a value (instead of performing the
// call) is the load-bearing design decision — it lets the router wrap execution
// with circuit/retry/fallback/hedge. Reliability stays OUTSIDE the provider.
type UpstreamRequest struct {
	URL    string
	Method string
	Header http.Header
	// Body is the provider-specific upstream body (e.g. a CommandCode
	// threadId/config/params envelope, or a passthrough OpenAI body).
	Body []byte
}

// Response is the normalized outbound response in canonical (OpenAI-shaped)
// form. A Provider's Translate turns native upstream bytes into this. Shared
// post-processing (reasoning extraction, normalization) is applied by the
// router AFTER Translate and is deliberately not the provider's concern.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// ConnConfig is the per-connection data the SDK needs. Its fields mirror the
// columns the live Connection row already carries, so the SDK is backward
// compatible by construction: no schema change, no new columns required to
// adapt an existing connection. (This is a standalone struct in the foundation
// commit — it is NOT coupled to the live Connection type, so importing this
// package can never drag in or alter the DB layer.)
type ConnConfig struct {
	ID         string
	Name       string
	BaseURL    string
	APIKey     string
	Format     string // retained for the compat/fallback resolution path
	ChatPath   string
	AuthHeader string
	AuthPrefix string
	Priority   int
}

// Provider is the core contract. It captures exactly what the doUpstream
// format-switch and response post-processing chain do today, but per-provider
// instead of as branches in the router.
//
// A Provider MUST be safe for concurrent use: the registry hands out a single
// shared instance per name, and the router may call it from many goroutines.
// Keep providers stateless (configuration in ConnConfig, not in fields that
// change per request).
type Provider interface {
	// Name is the stable registry key (e.g. "openai", "commandcode").
	Name() string

	// Track reports whether this provider is Official or Experimental.
	Track() Track

	// Capabilities declares what this provider can do. Declaration only in the
	// foundation commit; capability-based routing is a later step.
	Capabilities() CapabilitySet

	// Prepare turns a canonical Request into a concrete UpstreamRequest.
	// This is the home for what currently lives as
	// "if conn.Format == x { transformForCommandCode(...) }" in the router.
	// It MUST NOT perform the HTTP call.
	Prepare(ctx context.Context, req *Request, conn *ConnConfig) (*UpstreamRequest, error)

	// Translate turns raw upstream bytes into a canonical Response. This is the
	// home for what currently lives as translateCCAlphaToOpenAI() and the
	// per-format response massaging in the router. Shared post-processing
	// (reasoning extraction, normalization) is applied by the router after this.
	Translate(ctx context.Context, raw []byte, req *Request) (*Response, error)
}

// --- Optional capability interfaces (Go-idiomatic, no god-interface) ---------
//
// A Provider implements ONLY the optional interfaces it supports; the router
// type-asserts for them. Adding a new optional interface never breaks existing
// providers — that non-breaking property is the reason for this shape.

// Embedder is implemented by providers that can serve /v1/embeddings.
type Embedder interface {
	Embed(ctx context.Context, req *Request, conn *ConnConfig) (*UpstreamRequest, error)
}

// CredentialRefresher isolates per-provider credential refresh (e.g. OAuth
// token rotation) so that logic never leaks into the shared router.
type CredentialRefresher interface {
	Refresh(ctx context.Context, conn *ConnConfig) (*ConnConfig, error)
}

// StreamTranslator handles SSE->SSE conversion for providers whose streaming
// event shape differs from canonical OpenAI. Optional; absence means the
// router treats the stream as passthrough.
type StreamTranslator interface {
	TranslateStream(ctx context.Context, upstream io.Reader, w io.Writer, req *Request) error
}

package provider

import (
	"context"
	"net/http"
	"strings"
)

// DefaultProvider is the generic OpenAI-compatible provider and the SDK's
// safety net: any connection whose Format has no specialized provider resolves
// to this one (see Registry.Resolve), so coexistence with unmigrated
// connections is zero-risk. It corresponds to the "no special handling" path
// of the current router.
//
// FOUNDATION NOTE: this is a STUB in the sense that it is not wired into the
// live proxy and performs no provider-specific transformation — it passes the
// canonical OpenAI body straight through. It is fully functional and tested as
// an SDK component, but it does not (and in this commit cannot) affect any
// runtime path.
type DefaultProvider struct {
	name string
}

// NewDefaultProvider returns a generic OpenAI-compatible provider registered
// under name. Use a descriptive name like "openai" or "generic".
func NewDefaultProvider(name string) *DefaultProvider {
	if name == "" {
		name = "default"
	}
	return &DefaultProvider{name: name}
}

func (d *DefaultProvider) Name() string { return d.name }

func (d *DefaultProvider) Track() Track { return TrackOfficial }

// Capabilities is intentionally conservative: it declares only the broadly-true
// capabilities (streaming, tool calling) so that a future capability filter
// treats the default as widely eligible rather than narrowly specialized.
func (d *DefaultProvider) Capabilities() CapabilitySet {
	return NewCapabilitySet(CapStreaming, CapToolCalling)
}

// Prepare builds a standard OpenAI-compatible POST. It honors the connection's
// ChatPath / AuthHeader / AuthPrefix overrides, falling back to OpenAI defaults
// EXACTLY as the live router does today (proxy.go:981-987): an empty AuthPrefix
// becomes "Bearer ". The body is passed through unchanged because it is already
// canonical. This faithfulness is intentional — the SDK must not silently
// change behavior relative to the live path it shadows.
func (d *DefaultProvider) Prepare(ctx context.Context, req *Request, conn *ConnConfig) (*UpstreamRequest, error) {
	if req == nil || conn == nil {
		return nil, ErrPrepare
	}
	chatPath := conn.ChatPath
	if chatPath == "" {
		chatPath = "/v1/chat/completions"
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")

	authHeader := conn.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authPrefix := conn.AuthPrefix
	if authPrefix == "" {
		authPrefix = "Bearer "
	}
	if conn.APIKey != "" {
		h.Set(authHeader, authPrefix+conn.APIKey)
	}

	return &UpstreamRequest{
		URL:    strings.TrimRight(conn.BaseURL, "/") + chatPath,
		Method: http.MethodPost,
		Header: h,
		Body:   req.Body, // passthrough: already canonical OpenAI
	}, nil
}

// Translate passes canonical OpenAI bytes through unchanged. In the live router
// the shared post-processing (reasoning extraction, normalization) runs AFTER
// this step and stays the router's responsibility, not the provider's.
func (d *DefaultProvider) Translate(ctx context.Context, raw []byte, req *Request) (*Response, error) {
	return &Response{Status: http.StatusOK, Body: raw}, nil
}

// compile-time assertion that DefaultProvider satisfies Provider.
var _ Provider = (*DefaultProvider)(nil)

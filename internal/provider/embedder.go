package provider

import (
	"context"
	"net/http"
	"strings"
)

// embeddingsPath is the canonical OpenAI-compatible embeddings endpoint. It is
// fixed (not a per-connection override like ChatPath) because the live handler
// HandleEmbeddings hardcodes "/v1/embeddings"; faithfully mirroring that path
// is what guarantees byte-for-byte parity between this Embedder and the inline
// path it shadows.
const embeddingsPath = "/v1/embeddings"

// Embed implements the Embedder optional interface for the generic
// OpenAI-compatible provider. It builds (but does NOT execute) the upstream
// /v1/embeddings POST, mirroring internal/server/proxy.go HandleEmbeddings
// EXACTLY:
//
//   - URL    = TrimRight(conn.BaseURL,"/") + "/v1/embeddings"
//   - Method = POST
//   - Header = Content-Type: application/json, plus the auth header
//   - Body   = req.Body passthrough (the original request bytes, unchanged)
//
// The auth logic reproduces the live handler's faithful quirk: an empty
// AuthHeader defaults to "Authorization", an empty AuthPrefix defaults to
// "Bearer ", and the header is set ONLY when APIKey is non-empty. This is the
// same shape as DefaultProvider.Prepare, just pointed at the embeddings path.
//
// Like Prepare, this MUST NOT perform the HTTP call — the router executes the
// returned UpstreamRequest so reliability/streaming concerns stay outside the
// provider.
func (d *DefaultProvider) Embed(ctx context.Context, req *Request, conn *ConnConfig) (*UpstreamRequest, error) {
	if req == nil || conn == nil {
		return nil, ErrPrepare
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
		URL:    strings.TrimRight(conn.BaseURL, "/") + embeddingsPath,
		Method: http.MethodPost,
		Header: h,
		Body:   req.Body, // passthrough: the original embeddings request bytes
	}, nil
}

// compile-time assertion that DefaultProvider satisfies the Embedder optional
// interface. The capability EXECUTION path (serving /v1/embeddings) is distinct
// from capability ROUTING (Satisfies/eligibility, which is F2.4) — this is an
// execution contract only.
var _ Embedder = (*DefaultProvider)(nil)

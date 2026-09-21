package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// Provider adapts Qoder to Lintasan's provider SDK.
//
// TRACK — this is the load-bearing decision in the whole package. Track() returns
// TrackExperimental, so the production resolver (which selects TrackOfficial only)
// can never pick a Qoder connection. Qoder is reachable exclusively through the
// explicit experimental door. That is deliberate for two independent reasons:
//
//  1. Compliance. Lintasan's README states the project does not support reverse
//     engineering commercial IDE internal endpoints. This provider does exactly
//     that, so it must not be able to appear in ordinary routing without someone
//     opting in and, in doing so, knowingly changing what that sentence means.
//  2. Stability. The protocol is re-derived, undocumented and demonstrably
//     volatile: during this package's own development, credentials that worked
//     were observed to stop working, and the upstream service returned new
//     undocumented failure codes. None of that is a property a default routing
//     target should have.
//
// WIRING STATUS — read before enabling:
//
// The SDK router currently consumes only Prepare(). StreamTranslator and
// CredentialRefresher are declared in the SDK but not yet called on the request
// path. Qoder needs both: the response is an enveloped frame stream rather than
// OpenAI SSE, and credentials need a session exchange.
//
// So Prepare() here is correct and tested, and it is deliberately paired with a
// Translate() that refuses rather than silently returning the wrong shape. A
// request routed here through the SDK path therefore fails loudly instead of
// streaming envelope frames to a client expecting OpenAI chunks. Enabling Qoder
// for real requires either wiring StreamTranslator, or handling the format on the
// router's existing format-switch path — a separate, explicit change.
type Provider struct {
	sessions *SessionManager
}

// compile-time assertions.
var (
	_ provider.Provider = (*Provider)(nil)
)

// Name is the registry key.
func (p *Provider) Name() string { return "qoder" }

// Track is always Experimental — see the type doc for why this is non-negotiable.
func (p *Provider) Track() provider.Track { return provider.TrackExperimental }

// Capabilities declares what the upstream surface offers. Declaration only: per
// the SDK's Invariant 5 these are display values and are never trusted for
// routing decisions.
func (p *Provider) Capabilities() provider.CapabilitySet {
	return provider.NewCapabilitySet(
		provider.CapReasoning,
		provider.CapStreaming,
		provider.CapToolCalling,
		provider.CapCoding,
		provider.CapVision,
	)
}

// NewProvider builds a Qoder provider. salt is the install salt that seeds the
// device identity (see Fingerprinter); region selects the endpoint set.
func NewProvider(salt, region string, httpClient *http.Client) *Provider {
	return &Provider{sessions: NewSessionManager(salt, region, httpClient)}
}

// Sessions exposes the session manager, for discovery and diagnostics.
func (p *Provider) Sessions() *SessionManager { return p.sessions }

// Prepare builds a signed, envelope-encoded chat request.
//
// It reads the canonical OpenAI body from the request and converts it whole:
// the caller's model, messages and tools are carried across, and the Qoder
// template supplies the parts the protocol requires but a client cannot know
// (the system instructions, the tool schemas, the nested context object).
func (p *Provider) Prepare(ctx context.Context, req *provider.Request, conn *provider.ConnConfig) (*provider.UpstreamRequest, error) {
	if req == nil || conn == nil {
		return nil, fmt.Errorf("qoder: nil request or connection")
	}
	if strings.TrimSpace(conn.APIKey) == "" {
		return nil, fmt.Errorf("qoder: connection %q has no credential", conn.ID)
	}

	var inbound struct {
		Model     string           `json:"model"`
		Messages  []map[string]any `json:"messages"`
		Tools     []any            `json:"tools"`
		Stream    bool             `json:"stream"`
		MaxTokens int              `json:"max_tokens"`
	}
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &inbound); err != nil {
			return nil, fmt.Errorf("qoder: decode inbound body: %w", err)
		}
	}
	if inbound.Model == "" {
		return nil, fmt.Errorf("qoder: inbound request names no model")
	}
	if len(inbound.Messages) == 0 {
		return nil, fmt.Errorf("qoder: inbound request has no messages")
	}

	sess, err := p.sessions.session(ctx, conn.APIKey)
	if err != nil {
		return nil, err
	}

	body, err := BuildChatBody(ChatRequest{
		Model:       inbound.Model,
		Messages:    inbound.Messages,
		Tools:       inbound.Tools,
		Stream:      true, // the Qoder chat surface is streaming-only
		MaxTokens:   inbound.MaxTokens,
		IsReasoning: isReasoningModel(inbound.Model),
		UserType:    sess.Identity.UserType,
	})
	if err != nil {
		return nil, err
	}

	encoded, err := EncodeBase64(body)
	if err != nil {
		return nil, err
	}

	url := p.sessions.Endpoints().ChatStreamURL
	headers, err := p.sessions.BuildAPIHeaders(sess, []byte(encoded), pathOf(url), "text/event-stream", map[string]string{
		"x-model-key":    inbound.Model,
		"x-model-source": "system",
	})
	if err != nil {
		return nil, err
	}

	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}

	return &provider.UpstreamRequest{
		URL:    url,
		Method: http.MethodPost,
		Header: h,
		Body:   []byte(encoded),
	}, nil
}

// Translate refuses rather than mis-translating.
//
// A Qoder response is a stream of envelope-wrapped frames, not an OpenAI
// response body. Returning the raw bytes as if they were canonical would push
// envelope JSON to the client and surface as an unparseable-garbage bug far from
// its cause. Refusing keeps the failure at the boundary.
//
// If a non-streaming path is ever needed, it belongs here: collect the frames,
// reassemble the deltas, and emit a single canonical chat.completion object.
func (p *Provider) Translate(ctx context.Context, raw []byte, req *provider.Request) (*provider.Response, error) {
	return nil, fmt.Errorf("qoder: Translate is not implemented — the response is an enveloped frame stream; " +
		"use ConsumeStream/ParseStreamFrame, or wire provider.StreamTranslator")
}

// isReasoningModel reports whether a model key denotes a reasoning model.
//
// The model catalogue carries an authoritative is_reasoning flag, but Prepare has
// only the key. The suffix is the reliable signal available at that point; an
// unrecognised model is treated as non-reasoning, which under-requests a larger
// token budget rather than over-requesting one.
func isReasoningModel(model string) bool {
	low := strings.ToLower(model)
	return strings.Contains(low, "reasoning") || strings.Contains(low, "_r") ||
		strings.Contains(low, "38max") || strings.Contains(low, "thinking")
}

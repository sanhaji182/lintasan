package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// initProviderSDK wires the v3 Provider SDK (F1 — Official Layer Integration)
// into the proxy handler WITHOUT changing any runtime behavior by default.
//
// Design contract (locked in the approved F1 plan):
//   - The registry + DefaultProvider are always constructed, but they only take
//     effect when the kill-switch flag `provider_sdk_enabled` is true.
//   - Default is FALSE. With the flag off, doUpstream takes the untouched legacy
//     path, so production is bit-for-bit identical to pre-F1.
//   - The DefaultProvider is the official adapter for every OpenAI-compatible
//     connection (OpenAI, Anthropic, Gemini, DeepSeek, Groq). They all share the
//     same upstream shape today, so one faithful adapter covers all five.
//   - commandcode is NEVER routed through the SDK (it is RE-derived / experimental
//     and stays on the legacy path — out of F1 scope). The seam guards on Format.
func (p *ProxyHandler) initProviderSDK(database *db.DB) {
	// Per-handler registry (not the package-level Default) so tests stay isolated
	// and no global state leaks between handler instances.
	p.providerReg = provider.NewRegistry()
	p.defaultProvider = provider.NewDefaultProvider("openai")
	// Register the generic Official adapter. Resolve() falls back to it for any
	// Format without a specialized provider, which in F1 is all of them.
	_ = p.providerReg.Register(p.defaultProvider)

	// Kill-switch: default false. Read from the existing settings table (same
	// mechanism as thinking_mode / quality_filter_threshold) — NO schema change.
	p.providerSDK = false
	if v, err := database.GetSetting("provider_sdk_enabled"); err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "on", "yes":
			p.providerSDK = true
		}
	}

	// F2.3 capability shadow kill-switch: independent of provider_sdk_enabled,
	// default false. When on, the chat router evaluates candidate capability
	// eligibility in OBSERVE-ONLY mode (records, never excludes). Same parsing
	// contract. Read once at startup so the hot path only checks a bool.
	p.capabilityShadow = false
	if v, err := database.GetSetting("capability_shadow_enabled"); err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "on", "yes":
			p.capabilityShadow = true
		}
	}
	// F2.3 Option A: the shadow evidence aggregator. Always constructed (cheap,
	// empty maps); only WRITTEN when the shadow flag is on. Surfaced read-only
	// via GET /api/capabilities/shadow for the bake report.
	p.shadowStats = provider.NewShadowAggregator()

	// F2.5 embedder kill-switch: independent of provider_sdk_enabled and also
	// default false. When on, HandleEmbeddings builds the upstream request via
	// the provider Embedder; when off, the inline path is byte-for-byte
	// unchanged. Same parsing contract (true/1/on/yes, case-insensitive).
	p.embedderSDK = false
	if v, err := database.GetSetting("embedder_sdk_enabled"); err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "on", "yes":
			p.embedderSDK = true
		}
	}

	// F2.4 capability ENFORCEMENT kill-switch: independent of every other flag,
	// default false. When on, the chat router DROPS candidates that positively
	// fail the request's required capabilities (data-backed only; fail-open for
	// missing data; never empties the pool). It reuses the EXACT F2.3 resolver +
	// evaluator, so enforce acts on precisely what shadow observed. Same parsing
	// contract; read once at startup so the hot path only checks a bool.
	//   - enforce OFF, shadow OFF → pure legacy (default).
	//   - enforce OFF, shadow ON  → F2.3 observe-only.
	//   - enforce ON              → evaluate + record + filter (recording always
	//     on under enforcement so observability never goes dark).
	p.capabilityEnforce = false
	if v, err := database.GetSetting("capability_enforce_enabled"); err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "on", "yes":
			p.capabilityEnforce = true
		}
	}
}

// providerSDKEligible reports whether a connection may use the SDK path.
// Two gates, both mandatory:
//  1. the kill-switch flag is on, and
//  2. the connection is NOT commandcode (legacy/experimental, out of F1 scope).
func (p *ProxyHandler) providerSDKEligible(conn *Connection) bool {
	if !p.providerSDK || p.providerReg == nil {
		return false
	}
	if conn == nil || conn.Format == "commandcode" {
		return false
	}
	return true
}

// connToConfig maps a live Connection row to the SDK's standalone ConnConfig.
// The field mapping is intentionally 1:1 with the columns doUpstream already
// reads, so the SDK is backward compatible by construction (no schema change).
func connToConfig(conn *Connection) *provider.ConnConfig {
	return &provider.ConnConfig{
		ID:         conn.ID,
		Name:       conn.Name,
		BaseURL:    conn.BaseURL,
		APIKey:     conn.APIKey,
		Format:     conn.Format,
		ChatPath:   conn.ChatPath,
		AuthHeader: conn.AuthHeader,
		AuthPrefix: conn.AuthPrefix,
		Priority:   conn.Priority,
	}
}

// buildUpstreamViaSDK runs the Provider SDK's Prepare step ONLY and returns a
// concrete *http.Request ready for p.client.Do. It deliberately does NOT use
// provider.Dispatch: Dispatch buffers the full response body (ReadAll), which
// is incompatible with the streaming path. By stopping at Prepare and handing
// the assembled *http.Request back to the caller, the response — streaming or
// not — is handled by the existing, untouched router logic.
//
// This is the heart of F1: the request is BUILT by the provider, but EXECUTED
// and POST-PROCESSED by the router exactly as before.
func (p *ProxyHandler) buildUpstreamViaSDK(ctx context.Context, conn *Connection, body []byte, inboundHeaders http.Header) (*http.Request, error) {
	prov := p.providerReg.Resolve(conn.Format, p.defaultProvider)
	req := &provider.Request{
		Body:    body,
		Headers: inboundHeaders,
	}
	up, err := prov.Prepare(ctx, req, connToConfig(conn))
	if err != nil {
		return nil, err
	}
	upReq, err := http.NewRequestWithContext(ctx, up.Method, up.URL, strings.NewReader(string(up.Body)))
	if err != nil {
		return nil, err
	}
	for k, vs := range up.Header {
		for _, v := range vs {
			upReq.Header.Add(k, v)
		}
	}
	return upReq, nil
}

// buildEmbeddingsViaSDK runs the Provider SDK's Embed step ONLY (F2.5) and
// returns a concrete *http.Request ready for p.client.Do. It mirrors
// buildUpstreamViaSDK exactly, but routes through the optional Embedder
// interface (capability EXECUTION, not capability ROUTING — it deliberately
// touches no capability-selection / eligibility symbol).
//
// The provider is resolved by Format with the DefaultProvider fallback, then
// type-asserted to Embedder. The DefaultProvider satisfies Embedder, so the
// fallback always yields a usable embedder for the F2.5 target connections.
// The assembled request is byte-for-byte identical to the inline HandleEmbeddings
// path: same URL, POST, Content-Type, faithful auth (empty-prefix => "Bearer "),
// and the original body bytes passed through unchanged.
func (p *ProxyHandler) buildEmbeddingsViaSDK(ctx context.Context, conn *Connection, body []byte) (*http.Request, error) {
	prov := p.providerReg.Resolve(conn.Format, p.defaultProvider)
	emb, ok := prov.(provider.Embedder)
	if !ok {
		// Fallback: the generic DefaultProvider implements Embedder.
		emb, ok = p.defaultProvider.(provider.Embedder)
		if !ok {
			return nil, provider.ErrPrepare
		}
	}
	up, err := emb.Embed(ctx, &provider.Request{Body: body}, connToConfig(conn))
	if err != nil {
		return nil, err
	}
	upReq, err := http.NewRequestWithContext(ctx, up.Method, up.URL, strings.NewReader(string(up.Body)))
	if err != nil {
		return nil, err
	}
	for k, vs := range up.Header {
		for _, v := range vs {
			upReq.Header.Add(k, v)
		}
	}
	return upReq, nil
}

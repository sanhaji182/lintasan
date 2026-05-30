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

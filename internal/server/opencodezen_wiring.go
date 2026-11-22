package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/opencodezen"
)

// opencodeZenEnabled reports the dashboard kill-switch. Default OFF: a
// connection with format=opencodezen is inert until the operator opts in,
// matching qoder_enabled / provider_sdk_enabled semantics.
func opencodeZenEnabled(p *ProxyHandler) bool {
	if p == nil || p.db == nil {
		return false
	}
	v, err := p.db.GetSetting("opencode_zen_enabled")
	if err != nil {
		return false
	}
	return parseOpencodeZenFlag(v)
}

func parseOpencodeZenFlag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "on", "yes":
		return true
	}
	return false
}

// prepareOpencodeZenRequest builds the fingerprinted upstream headers. The
// downstream UA is preserved only when it already looks like a real OpenCode
// client; otherwise the known-good constant is substituted. Free-tier models
// must ride the `Bearer public` identity (a real key gets 403 FreeTierError).
func prepareOpencodeZenRequest(h http.Header, downstreamUA, apiKey, serviceName string, unixMilli int64, freeTier bool) http.Header {
	return opencodezen.PrepareHeaders(h, downstreamUA, apiKey, serviceName, unixMilli, freeTier)
}

// ensureOpencodeZenFingerprintTools injects the bash/glob/grep/read quartet
// into an OpenAI-shaped tools array and returns it. Bodies are kept as
// []any because the request is re-marshalled by the caller.
func ensureOpencodeZenFingerprintTools(raw []any) []any {
	tools := make([]map[string]any, 0, len(raw)+len(opencodezen.FingerprintTools))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			tools = append(tools, m)
		}
	}
	out := opencodezen.EnsureFingerprintTools(tools)
	result := make([]any, 0, len(out))
	for _, m := range out {
		result = append(result, m)
	}
	return result
}

// applyOpencodeZenWire makes the outbound request OpenCode-Zen-shaped:
// fingerprint headers plus the four built-in tools. Called only for
// format=opencodezen AND when opencode_zen_enabled is on. The body is
// re-marshalled with the injected tools; all other fields are untouched.
func applyOpencodeZenWire(upReq *http.Request, body []byte, downstreamUA, apiKey, serviceName string, now time.Time) []byte {
	freeTier := false
	var req map[string]any
	if err := json.Unmarshal(body, &req); err == nil {
		model, _ := req["model"].(string)
		freeTier = opencodezen.IsFreeTierModel(model)
		if rawTools, ok := req["tools"].([]any); ok {
			req["tools"] = ensureOpencodeZenFingerprintTools(rawTools)
		} else {
			req["tools"] = ensureOpencodeZenFingerprintTools(nil)
		}
		if out, err := json.Marshal(req); err == nil {
			body = out
		}
	}
	prepareOpencodeZenRequest(upReq.Header, downstreamUA, apiKey, serviceName, now.UnixMilli(), freeTier)
	return body
}

package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// capability_shadow.go — F2.3 server-side shadow routing hook (observe-only).
//
// This is the server's ONLY F2.3 surface. It extracts primitive, capability-
// vocabulary-free signals from the request and delegates the entire capability
// evaluation to provider.ShadowEvaluate (the facade). It deliberately holds NO
// capability constants and performs no eligibility check itself — the lookup
// and matching all happen inside the provider package — so the F2.0/F2.1/F2.2
// server non-consumption guards stay green unchanged.
//
// CONTRACT: this function MUST NOT mutate, reorder, filter, or select from
// `candidates`. It reads them (by Format) and records an observation. Selection
// happens entirely after this call and is identical whether the flag is on/off.

// extractRequestSignals reads the primitive capability signals from a parsed
// chat request body. Pure; no capability vocabulary leaks into the server.
func extractRequestSignals(req map[string]any, stream bool) provider.RequestSignals {
	s := provider.RequestSignals{Stream: stream}

	if _, ok := req["tools"]; ok {
		s.HasTools = true
	}
	if _, ok := req["functions"]; ok {
		s.HasTools = true
	}

	// response_format: {"type":"json_object"} or {"type":"json_schema",...}
	if rf, ok := req["response_format"].(map[string]any); ok {
		if t, _ := rf["type"].(string); t == "json_object" || t == "json_schema" {
			s.HasJSONMode = true
		}
	}

	// Vision: any message whose content is a multipart array containing an
	// image / image_url part. Text-only requests (content is a string) are not
	// vision. Conservative: only flags on an explicit image part.
	if msgs, ok := req["messages"].([]any); ok {
		for _, m := range msgs {
			mm, ok := m.(map[string]any)
			if !ok {
				continue
			}
			parts, ok := mm["content"].([]any)
			if !ok {
				continue
			}
			for _, p := range parts {
				pp, ok := p.(map[string]any)
				if !ok {
					continue
				}
				switch t, _ := pp["type"].(string); t {
				case "image", "image_url", "input_image":
					s.HasVision = true
				}
			}
		}
	}
	return s
}

// runCapabilityShadow performs the F2.3 observe-only capability evaluation. It
// is a complete no-op when the flag is off (a single bool check, zero added
// latency). When on, it evaluates the candidate pool via the provider facade
// and RECORDS the result (response header + stderr log) WITHOUT changing
// `candidates` in any way.
func (p *ProxyHandler) runCapabilityShadow(w http.ResponseWriter, req map[string]any, stream bool, candidates []*Connection) {
	if !p.capabilityShadow {
		return // default OFF: zero behavior change, zero added work
	}
	if len(candidates) == 0 {
		return
	}

	signals := extractRequestSignals(req, stream)

	ids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.Format) // coarse: capability lookup is keyed by Format
	}

	result := provider.ShadowEvaluate(signals, ids)

	// Observability ONLY. The selection logic below this hook is unaffected.
	w.Header().Set("X-Lintasan-Capability-Shadow",
		fmt.Sprintf("required=%d candidates=%d would_exclude=%d",
			len(result.Required), len(result.Decisions), len(result.WouldExclude)))
	if len(result.WouldExclude) > 0 {
		fmt.Fprintf(os.Stderr,
			"[capability-shadow] required=%v would_exclude=%v (OBSERVE-ONLY, not excluded)\n",
			result.Required, result.WouldExclude)
	}
}

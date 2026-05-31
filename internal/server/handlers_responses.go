package server

import (
	"net/http"
)

// handlers_responses.go — Codex Official Layer `/v1/responses` ingress.
// M0 SCAFFOLDING ONLY (see proposals/codex-build-plan.md).
//
// SCOPE LOCK (M0): this handler is wired to the route and gated by the
// `responses_api_enabled` flag (default OFF, parsed in initProviderSDK), but it
// performs NO translation, NO streaming, NO tool loop, and NEVER touches the
// existing chat pipeline. Behavior:
//   - flag OFF (default)  → 404 Not Found  (route inert; prod sees no new surface)
//   - flag ON             → 501 Not Implemented (honest: M0 has no implementation)
//
// This deliberately avoids any fake/no-op success path (a locked exclusion).
// M1 fills in request translation, M2 the streaming Responses SSE emitter, M3
// the tool round-trip with call_id fidelity. The route inherits the existing
// /v1/* auth middleware (fail-closed 401 without a token) — no new auth code.

// HandleResponses is the entry point for POST /v1/responses (Codex ingress).
// M0: flag-gated no-op (404 when off, 501 when on). It does NOT read the body,
// invoke the pipeline, or emit any model output.
func (p *ProxyHandler) HandleResponses(w http.ResponseWriter, r *http.Request) {
	if !p.responsesAPI {
		// Flag OFF (default): the Responses surface does not exist yet. Returning
		// 404 keeps prod byte-identical to a build without this route.
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	// Flag ON but M0: scaffolding only — no streaming, no tool loop. Be honest
	// rather than returning a fake success. Implementation lands in M1–M3.
	w.Header().Set("X-Lintasan-Ingress", "responses")
	http.Error(w,
		`{"error":"responses API not implemented yet (M0 scaffolding)","object":"error"}`,
		http.StatusNotImplemented)
}

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/sanhaji182/lintasan-go/internal/translator"
)

// handlers_responses.go — Codex Official Layer `/v1/responses` ingress.
//
// M0: scaffolding (flag-gated 404/501).
// M1: request translation (Responses → canonical chat) in internal/translator.
// M2: streaming text path — translate the request, then run the EXISTING chat
//     handler through a writer-interception adapter that re-frames the canonical
//     chat SSE into Responses API typed events.
//
// SCOPE LOCK (M2): TEXT streaming only. NO tool execution, NO tool loop, NO
// routing/provider change, NO prod change (flag default OFF). The core pipeline
// (HandleChatCompletions and everything it calls) is invoked UNCHANGED; the only
// new behavior is request translation at ingress and event re-framing at egress.
//
// Behavior by flag:
//   - flag OFF (default) → 404 (route inert; prod byte-identical to no route)
//   - flag ON            → translate + stream via adapter (M2)
//
// The route inherits the existing /v1/* auth middleware (fail-closed 401 without
// a token) — no new auth code here.

// HandleResponses is the entry point for POST /v1/responses (Codex ingress).
func (p *ProxyHandler) HandleResponses(w http.ResponseWriter, r *http.Request) {
	if !p.responsesAPI {
		// Flag OFF (default): the Responses surface does not exist. 404 keeps
		// prod byte-identical to a build without this route.
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeResponsesError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		writeResponsesError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// M1: translate Responses request → canonical chat-completions request.
	chatReq, err := translator.ResponsesToOpenAIRequest(raw)
	if err != nil {
		// Sentinel validation errors → 400 with the specific message.
		writeResponsesError(w, http.StatusBadRequest, responsesErrMessage(err))
		return
	}

	// Codex always streams on /responses; force streaming for the core handler so
	// the adapter receives an SSE chunk stream to re-frame. (M6 could add a
	// non-streaming JSON path, deferred.)
	chatReq["stream"] = true

	model, _ := chatReq["model"].(string)

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		writeResponsesError(w, http.StatusInternalServerError, "failed to encode translated request")
		return
	}

	// Re-enter the EXISTING chat pipeline with a synthetic request carrying the
	// translated body. The core handler is invoked unchanged; only the writer is
	// wrapped so its SSE output is re-framed into Responses events.
	chatHTTPReq := r.Clone(r.Context())
	chatHTTPReq.Body = io.NopCloser(bytes.NewReader(chatBody))
	chatHTTPReq.ContentLength = int64(len(chatBody))
	chatHTTPReq.URL.Path = "/v1/chat/completions"

	adapter := newResponsesStreamAdapter(w, model)
	p.HandleChatCompletions(adapter, chatHTTPReq)
	adapter.finalize()
}

// writeResponsesError writes a JSON error in a shape Codex tolerates, before any
// stream has started.
func writeResponsesError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error":  map[string]any{"message": msg, "type": "invalid_request_error"},
		"object": "error",
	})
}

// responsesErrMessage extracts a stable message for known validation sentinels,
// falling back to the error's own text.
func responsesErrMessage(err error) string {
	switch {
	case errors.Is(err, translator.ErrResponsesNoModel),
		errors.Is(err, translator.ErrResponsesNoInput),
		errors.Is(err, translator.ErrResponsesBadInput),
		errors.Is(err, translator.ErrResponsesBadItem),
		errors.Is(err, translator.ErrResponsesNoCallID),
		errors.Is(err, translator.ErrResponsesBadToolsTy):
		return err.Error()
	default:
		return err.Error()
	}
}

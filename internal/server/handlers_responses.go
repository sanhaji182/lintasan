package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/sanhaji182/lintasan-go/internal/metrics"
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
	if !p.responsesAPIEnabled() {
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

	// M5: record tools that could not be represented as chat functions (Codex
	// sends provider built-ins like web_search alongside real functions). The
	// translation itself is pure; observability lives here so a shrunk tool list
	// is visible in /metrics rather than silent.
	if _, dropped := translator.ResponsesToolsStats(raw); dropped > 0 {
		metrics.RecordResponsesToolsDropped(dropped)
	}

	// M6: honor the client's `stream` preference. Codex itself always streams,
	// but the Responses API is also used by plain HTTP clients that expect a
	// single JSON body. Absent `stream` defaults to TRUE, preserving the
	// pre-M6 behavior for every existing caller; only an explicit false takes
	// the buffered path.
	wantStream := true
	if v, ok := raw["stream"].(bool); ok {
		wantStream = v
	}
	chatReq["stream"] = wantStream

	model, _ := chatReq["model"].(string)

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		writeResponsesError(w, http.StatusInternalServerError, "failed to encode translated request")
		return
	}

	// Re-enter the EXISTING chat pipeline with a synthetic request carrying the
	// translated body. The core handler is invoked unchanged; only the writer is
	// wrapped so its output is re-framed into the Responses shape.
	chatHTTPReq := r.Clone(r.Context())
	chatHTTPReq.Body = io.NopCloser(bytes.NewReader(chatBody))
	chatHTTPReq.ContentLength = int64(len(chatBody))
	chatHTTPReq.URL.Path = "/v1/chat/completions"

	if !wantStream {
		p.handleResponsesBuffered(w, chatHTTPReq, model)
		return
	}

	adapter := newResponsesStreamAdapter(w, model)
	p.HandleChatCompletions(adapter, chatHTTPReq)
	adapter.finalize()
}

// handleResponsesBuffered serves the non-streaming path (M6): it captures the
// chat handler's JSON body, converts it to a Responses object, and writes that
// as a single response.
//
// The upstream status is honored: a non-2xx chat response is passed through
// verbatim (body and status), so an auth failure or a rate limit reaches the
// client as itself instead of being relabeled as a Responses translation error.
func (p *ProxyHandler) handleResponsesBuffered(w http.ResponseWriter, chatReq *http.Request, model string) {
	rec := &bufferedResponseWriter{header: http.Header{}, status: http.StatusOK}
	p.HandleChatCompletions(rec, chatReq)

	body := rec.body.Bytes()

	// Non-2xx → passthrough verbatim. The client gets the upstream's own error.
	if rec.status < 200 || rec.status > 299 {
		for k, vs := range rec.header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(rec.status)
		w.Write(body)
		return
	}

	var chatResp map[string]any
	if err := json.Unmarshal(body, &chatResp); err != nil {
		writeResponsesError(w, http.StatusBadGateway, "upstream returned a non-JSON body")
		return
	}

	resp, err := translator.OpenAIResponseToResponses(chatResp)
	if err != nil {
		metrics.RecordResponsesStream(false, metrics.ResponsesTerminalFailed, 0, false)
		logResponsesBuffered(model, "failed", 0, false)
		writeResponsesError(w, http.StatusBadGateway, responsesErrMessage(err))
		return
	}
	// The chat body may omit `model` (some upstreams do); fall back to the
	// model we routed on so the client never sees an empty model field.
	if m, _ := resp["model"].(string); m == "" {
		resp["model"] = model
	}

	// Same observability contract as the streaming path: exactly one metrics
	// record + one structured log line per request, so /metrics counts every
	// Responses turn regardless of which path served it.
	toolCalls, hadText := responsesOutputStats(resp)
	metrics.RecordResponsesStream(true, metrics.ResponsesTerminalCompleted, toolCalls, hadText)
	logResponsesBuffered(model, "completed", toolCalls, hadText)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// responsesOutputStats counts the tool-call items and reports whether any
// assistant text was produced, mirroring what the streaming emitter tracks.
func responsesOutputStats(resp map[string]any) (toolCalls int, hadText bool) {
	items, _ := resp["output"].([]any)
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "function_call":
			toolCalls++
		case "message":
			hadText = true
		}
	}
	return toolCalls, hadText
}

// logResponsesBuffered writes ONE structured line per non-streaming Responses
// request, in the same shape as the streaming adapter's logStream. Counts and
// terminal state only — NO prompt content, NO call_id values, NO secrets.
func logResponsesBuffered(model, terminal string, toolCalls int, hadText bool) {
	if !metricsEnabled() {
		return
	}
	fmt.Fprintf(os.Stderr,
		"lintasan.responses ingress=responses-nonstream model=%q started=%t terminal=%s tool_calls=%d had_text=%t\n",
		model, terminal == "completed", terminal, toolCalls, hadText)
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

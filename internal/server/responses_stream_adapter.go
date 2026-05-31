package server

import (
	"bytes"
	"net/http"

	"github.com/sanhaji182/lintasan-go/internal/translator"
)

// responses_stream_adapter.go — Codex M2: writer-interception adapter that
// re-frames the canonical chat-completions SSE stream (which the EXISTING,
// UNTOUCHED chat pipeline writes) into OpenAI Responses API typed events.
//
// DESIGN (codex-build-plan.md M2, decision: writer-interception):
//   - HandleResponses translates the Responses request → canonical chat request
//     (M1), then calls the existing HandleChatCompletions with THIS adapter in
//     place of the real http.ResponseWriter.
//   - The chat handler writes `data: {chat chunk}` / `data: [DONE]` exactly as
//     today; it has NO knowledge of Responses. The adapter intercepts those
//     writes and emits typed Responses events instead.
//   - Net effect: ZERO change to the proxy/router/provider pipeline. The shim
//     lives entirely at the ingress edge.
//
// SCOPE LOCK (M2): TEXT streaming only. No tool execution, no tool loop, no
// routing/provider change. Tool-call deltas in the chat stream are not emitted
// as Responses function_call items here — that is M3. The terminal
// response.completed is always emitted (via Finish) so the stream is valid.

// responsesStreamAdapter implements http.ResponseWriter + http.Flusher. It sits
// between the chat handler and the real client connection, converting chat SSE
// into Responses SSE on the fly.
type responsesStreamAdapter struct {
	real    http.ResponseWriter
	emitter *translator.ResponsesStreamEmitter

	header      http.Header
	wroteHeader bool
	statusCode  int

	// buf accumulates partial SSE lines across Write calls (the chat handler may
	// write a chunk in pieces; SSE frames are newline-delimited).
	buf bytes.Buffer

	// passthrough is set when the upstream returned a non-2xx status BEFORE any
	// stream event was produced — in that case we forward the body verbatim so
	// the client sees the real error instead of a fabricated empty stream.
	passthrough bool
}

func newResponsesStreamAdapter(real http.ResponseWriter, model string) *responsesStreamAdapter {
	return &responsesStreamAdapter{
		real:       real,
		emitter:    translator.NewResponsesStreamEmitter(model),
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

// Header returns the adapter's own header map so the chat handler's header
// mutations don't leak chat-specific values (e.g. Content-Type) to the client
// until WriteHeader decides the final set.
func (a *responsesStreamAdapter) Header() http.Header { return a.header }

// WriteHeader captures the status. On a non-2xx it switches to passthrough so
// the upstream error body is forwarded unchanged. On 2xx it forces the
// Responses SSE content type and writes the real header once.
func (a *responsesStreamAdapter) WriteHeader(status int) {
	if a.wroteHeader {
		return
	}
	a.statusCode = status
	if status < 200 || status >= 300 {
		// Upstream/handler error before streaming: forward verbatim.
		a.passthrough = true
		copyHeader(a.real.Header(), a.header)
		a.real.WriteHeader(status)
		a.wroteHeader = true
		return
	}
	// Success: emit a Responses event stream regardless of the chat content type.
	dst := a.real.Header()
	copyHeader(dst, a.header)
	dst.Set("Content-Type", "text/event-stream")
	dst.Set("Cache-Control", "no-cache")
	dst.Set("Connection", "keep-alive")
	dst.Set("X-Lintasan-Ingress", "responses")
	a.real.WriteHeader(http.StatusOK)
	a.wroteHeader = true
}

// Write intercepts the chat handler's output. In passthrough mode it forwards
// bytes unchanged. Otherwise it parses complete SSE lines, feeds the emitter,
// and writes the resulting Responses events. It always reports len(p) written so
// the chat handler never sees a short write.
func (a *responsesStreamAdapter) Write(p []byte) (int, error) {
	if !a.wroteHeader {
		a.WriteHeader(http.StatusOK)
	}
	if a.passthrough {
		return a.real.Write(p)
	}

	a.buf.Write(p)
	for {
		idx := bytes.IndexByte(a.buf.Bytes(), '\n')
		if idx < 0 {
			break
		}
		line := string(a.buf.Next(idx + 1))
		for _, ev := range a.emitter.Process(line) {
			if _, err := a.real.Write([]byte(ev)); err != nil {
				return len(p), err
			}
		}
		a.flush()
	}
	return len(p), nil
}

// Flush implements http.Flusher so the chat handler's flush calls propagate.
func (a *responsesStreamAdapter) Flush() { a.flush() }

func (a *responsesStreamAdapter) flush() {
	if f, ok := a.real.(http.Flusher); ok {
		f.Flush()
	}
}

// finalize drains any buffered partial line, then emits the terminal lifecycle
// (response.completed) if streaming. MUST be called after the chat handler
// returns. In passthrough mode it is a no-op. Idempotent at the emitter level.
func (a *responsesStreamAdapter) finalize() {
	if a.passthrough {
		return
	}
	if !a.wroteHeader {
		a.WriteHeader(http.StatusOK)
	}
	// Process any trailing buffered line that lacked a newline.
	if a.buf.Len() > 0 {
		line := a.buf.String()
		a.buf.Reset()
		for _, ev := range a.emitter.Process(line) {
			a.real.Write([]byte(ev))
		}
	}
	for _, ev := range a.emitter.Finish() {
		a.real.Write([]byte(ev))
	}
	a.flush()
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		// Don't override the SSE content type we set explicitly.
		if http.CanonicalHeaderKey(k) == "Content-Type" {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

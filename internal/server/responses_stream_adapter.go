package server

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/metrics"
	"github.com/sanhaji182/lintasan-go/internal/translator"
)

// responses_stream_adapter.go — Codex M2/M4: writer-interception adapter that
// re-frames the canonical chat-completions SSE stream (which the EXISTING,
// UNTOUCHED chat pipeline writes) into OpenAI Responses API typed events.
//
// DESIGN (codex-build-plan.md, decision: writer-interception):
//   - HandleResponses translates the Responses request → canonical chat request
//     (M1), then calls the existing HandleChatCompletions with THIS adapter in
//     place of the real http.ResponseWriter.
//   - The chat handler writes `data: {chat chunk}` / `data: [DONE]` exactly as
//     today; it has NO knowledge of Responses. The adapter intercepts those
//     writes and emits typed Responses events instead.
//   - Net effect: ZERO change to the proxy/router/provider pipeline. The shim
//     lives entirely at the ingress edge.
//
// M4 (Hardening & Observability) adds:
//   - Honest terminal-state handling. A stream NEVER ends silently:
//       • clean `[DONE]`              → response.completed
//       • foreign body after start    → response.failed     (the chat handler's
//         panic-recover / http.Error path writes a non-SSE body mid-stream)
//       • started but no `[DONE]`     → response.incomplete  (upstream truncation;
//         Codex treats this as a retryable error, vs masking it as completed)
//   - Metrics: one RecordResponsesStream call per stream (counts only).
//   - Structured diagnostics: one stderr log line per stream (no secrets, no
//     prompt content, no call_id values — just counts + terminal state).

// responsesStreamAdapter implements http.ResponseWriter + http.Flusher. It sits
// between the chat handler and the real client connection, converting chat SSE
// into Responses SSE on the fly.
type responsesStreamAdapter struct {
	real    http.ResponseWriter
	emitter *translator.ResponsesStreamEmitter
	model   string

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

	// sawErrorBody is set when a non-SSE, non-blank line arrives AFTER streaming
	// started — i.e. the chat handler's panic-recover / http.Error path wrote an
	// error body mid-stream. M4 turns that into a response.failed terminal
	// instead of silently dropping it.
	sawErrorBody bool

	// finalized guards single metrics/log recording.
	finalized bool
}

func newResponsesStreamAdapter(real http.ResponseWriter, model string) *responsesStreamAdapter {
	return &responsesStreamAdapter{
		real:       real,
		emitter:    translator.NewResponsesStreamEmitter(model),
		model:      model,
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
		a.noteForeignLine(line)
		for _, ev := range a.emitter.Process(line) {
			if _, err := a.real.Write([]byte(ev)); err != nil {
				return len(p), err
			}
		}
		a.flush()
	}
	return len(p), nil
}

// noteForeignLine flags a line that is NOT a valid chat SSE line and not blank/
// comment — the signature of the chat handler's panic-recover / http.Error body
// written mid-stream. It does NOT flag normal SSE `data:` lines or blanks.
func (a *responsesStreamAdapter) noteForeignLine(line string) {
	t := strings.TrimSpace(line)
	if t == "" {
		return
	}
	if strings.HasPrefix(t, "data:") || strings.HasPrefix(t, "event:") || strings.HasPrefix(t, ":") {
		return
	}
	// A non-SSE, non-blank line after the stream started → error body.
	a.sawErrorBody = true
}

// Flush implements http.Flusher so the chat handler's flush calls propagate.
func (a *responsesStreamAdapter) Flush() { a.flush() }

func (a *responsesStreamAdapter) flush() {
	if f, ok := a.real.(http.Flusher); ok {
		f.Flush()
	}
}

// finalize drains any buffered partial line, then emits the correct terminal
// event (M4): completed on a clean [DONE], failed on a foreign error body,
// incomplete on a started-but-unterminated stream. It NEVER ends silently. MUST
// be called after the chat handler returns. In passthrough mode it records the
// (passthrough) outcome but emits no Responses events. Idempotent.
func (a *responsesStreamAdapter) finalize() {
	if a.finalized {
		return
	}
	a.finalized = true

	if a.passthrough {
		// The client already received the verbatim upstream error; no Responses
		// terminal applies. Record it as a failed Responses stream for metrics.
		metrics.RecordResponsesStream(false, metrics.ResponsesTerminalFailed, 0, false)
		a.logStream(false, "passthrough")
		return
	}
	if !a.wroteHeader {
		a.WriteHeader(http.StatusOK)
	}
	// Process any trailing buffered line that lacked a newline.
	if a.buf.Len() > 0 {
		line := a.buf.String()
		a.buf.Reset()
		a.noteForeignLine(line)
		for _, ev := range a.emitter.Process(line) {
			a.real.Write([]byte(ev))
		}
	}

	// Decide the terminal. If the emitter already terminated (it saw [DONE] and
	// emitted response.completed during Process), these are idempotent no-ops.
	var events []string
	switch {
	case a.emitter.TerminalState() != "":
		// Already terminated cleanly via [DONE] → completed. Nothing to add.
	case a.sawErrorBody:
		events = a.emitter.Fail(translator.TerminalFailed, "upstream error after stream start")
	case a.emitter.Started():
		// Stream began but never saw a completion marker → truncation. Report it
		// honestly as incomplete (a Codex error terminal) rather than masking it
		// as completed.
		events = a.emitter.Fail(translator.TerminalIncomplete, "stream ended without completion marker")
	default:
		// Nothing was ever emitted (no content, no [DONE]) → close cleanly so the
		// stream is still a valid, terminated Responses stream.
		events = a.emitter.Finish()
	}
	for _, ev := range events {
		a.real.Write([]byte(ev))
	}
	a.flush()

	// M4 observability: one metrics record + one structured log line per stream.
	a.recordMetrics()
	a.logStream(a.emitter.Started(), a.emitter.TerminalState())
}

// recordMetrics reports the finished stream to the metrics package (counts only).
func (a *responsesStreamAdapter) recordMetrics() {
	var term metrics.ResponsesTerminal
	switch a.emitter.TerminalState() {
	case "completed":
		term = metrics.ResponsesTerminalCompleted
	case "failed":
		term = metrics.ResponsesTerminalFailed
	case "incomplete":
		term = metrics.ResponsesTerminalIncomplete
	default:
		term = metrics.ResponsesTerminalCompleted
	}
	metrics.RecordResponsesStream(a.emitter.Started(), term, a.emitter.ToolCallCount(), a.emitter.HadText())
}

// logStream writes ONE structured diagnostic line per Responses stream. It
// carries counts + terminal state only — NO prompt content, NO call_id values,
// NO secrets. Gated by the same metrics switch as /metrics so operators can
// silence it.
func (a *responsesStreamAdapter) logStream(started bool, terminal string) {
	if !metricsEnabled() {
		return
	}
	if terminal == "" {
		terminal = "none"
	}
	fmt.Fprintf(os.Stderr,
		"lintasan.responses ingress=responses model=%q started=%t terminal=%s tool_calls=%d had_text=%t\n",
		a.model, started, terminal, a.emitter.ToolCallCount(), a.emitter.HadText())
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

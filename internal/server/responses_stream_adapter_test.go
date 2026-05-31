package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/metrics"
)

// responses_stream_adapter_test.go — Codex M2 writer-interception (Tier 2/4).
//
// Verifies the adapter re-frames a canonical chat SSE stream into a valid
// Responses event stream WITHOUT the chat handler knowing, and that the terminal
// response.completed is always emitted (the load-bearing invariant). Also covers
// the non-2xx passthrough path.

// writeChatSSE simulates exactly what HandleChatCompletions writes for a
// streaming text response.
func writeChatSSE(a *responsesStreamAdapter, chunks []string) {
	a.Header().Set("Content-Type", "text/event-stream")
	a.WriteHeader(200)
	for _, c := range chunks {
		a.Write([]byte(c))
		a.Flush()
	}
	a.Write([]byte("data: [DONE]\n\n"))
	a.finalize()
}

func TestM2Adapter_ReframesChatStream(t *testing.T) {
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	writeChatSSE(a, []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n",
	})

	body := rec.Body.String()

	// Content type must be the Responses SSE type, NOT the chat one.
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type: got %q", ct)
	}
	if rec.Header().Get("X-Lintasan-Ingress") != "responses" {
		t.Fatal("missing X-Lintasan-Ingress header")
	}

	// Must contain the typed lifecycle and terminal.
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.completed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("output missing %q\nfull:\n%s", want, body)
		}
	}
	// Must NOT leak the chat [DONE] sentinel.
	if strings.Contains(body, "data: [DONE]") {
		t.Fatalf("chat [DONE] leaked into Responses stream:\n%s", body)
	}
	// Terminal must be last.
	idx := strings.LastIndex(body, "event: response.completed")
	if idx < 0 || strings.Contains(body[idx:], "event: response.output_text.delta") {
		t.Fatal("response.completed must be the terminal event")
	}
}

// TestM4Adapter_TruncationIsIncomplete: M4 milestone transition. M2 closed a
// truncated stream (no [DONE]) with response.completed; M4 reports it honestly
// as response.incomplete (a Codex error terminal) instead of masking it. Either
// way the stream is NEVER left without a terminal.
func TestM4Adapter_TruncationIsIncomplete(t *testing.T) {
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	a.Header().Set("Content-Type", "text/event-stream")
	a.WriteHeader(200)
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
	// Upstream truncates: NO [DONE]. finalize must close with an honest terminal.
	a.finalize()

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.incomplete") {
		t.Fatalf("truncated stream must close with response.incomplete (M4):\n%s", body)
	}
	if strings.Contains(body, "event: response.completed") {
		t.Fatal("truncated stream must NOT be masked as completed")
	}
}

func TestM2Adapter_NonStreamJSONReframed(t *testing.T) {
	// When the chat handler writes a single JSON chunk (some cache paths emit one
	// `data: {...}` then [DONE]), the adapter still produces a valid stream.
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	writeChatSSE(a, []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"whole answer\"}}]}\n\n",
	})
	body := rec.Body.String()
	if !strings.Contains(body, "whole answer") {
		t.Fatalf("content missing:\n%s", body)
	}
	if !strings.Contains(body, "event: response.completed") {
		t.Fatalf("no terminal:\n%s", body)
	}
}

func TestM2Adapter_ErrorPassthrough(t *testing.T) {
	// Non-2xx before streaming → forward the upstream error body verbatim, NOT a
	// fabricated empty Responses stream.
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	a.Header().Set("Content-Type", "application/json")
	a.WriteHeader(429)
	a.Write([]byte(`{"error":"rate limited"}`))
	a.finalize()

	if rec.Code != 429 {
		t.Fatalf("status: got %d want 429", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "rate limited") {
		t.Fatalf("error body not passed through: %s", body)
	}
	if strings.Contains(body, "response.completed") {
		t.Fatal("must NOT emit Responses events on error passthrough")
	}
}

func TestM2Adapter_PartialLineBuffering(t *testing.T) {
	// The chat handler may write an SSE frame in pieces; the adapter must buffer
	// until a full newline-terminated line is available.
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	a.WriteHeader(200)
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"split"))
	a.Write([]byte(" content\"}}]}\n\n"))
	a.Write([]byte("data: [DONE]\n\n"))
	a.finalize()

	body := rec.Body.String()
	if !strings.Contains(body, "split content") {
		t.Fatalf("partial line not reassembled:\n%s", body)
	}
	if !strings.Contains(body, "event: response.completed") {
		t.Fatal("no terminal event")
	}
}

// TestM3Adapter_FunctionCallThroughWriter asserts the full M3 path through the
// writer-interception adapter: a chat stream carrying tool_calls is re-framed
// into a Responses function_call item with the call_id preserved VERBATIM, all
// without the chat handler knowing about Responses.
func TestM3Adapter_FunctionCallThroughWriter(t *testing.T) {
	const callID = "call_adapter_99"
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	a.WriteHeader(200)
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"" + callID + "\",\"function\":{\"name\":\"search\",\"arguments\":\"{}\"}}]}}]}\n\n"))
	a.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
	a.Write([]byte("data: [DONE]\n\n"))
	a.finalize()

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_item.added") {
		t.Fatalf("no function_call item added:\n%s", body)
	}
	if !strings.Contains(body, `"type":"function_call"`) {
		t.Fatalf("function_call item type missing:\n%s", body)
	}
	if !strings.Contains(body, `"call_id":"`+callID+`"`) {
		t.Fatalf("call_id not preserved verbatim through the adapter:\n%s", body)
	}
	if !strings.Contains(body, "event: response.completed") {
		t.Fatal("no terminal event")
	}
}

// TestM4Adapter_ErrorBodyAfterStreamStart asserts the M4 hardening path: when
// the chat handler writes a non-SSE error body AFTER streaming has started
// (its panic-recover / http.Error path), the adapter closes with
// response.failed instead of leaving the stream silently truncated.
func TestM4Adapter_ErrorBodyAfterStreamStart(t *testing.T) {
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	a.WriteHeader(200)
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"working\"}}]}\n\n"))
	// The chat handler hit an error mid-stream and wrote a JSON error body
	// (not an SSE frame) into the same writer.
	a.Write([]byte("{\"error\":\"internal server error\"}\n"))
	a.finalize()

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.failed") {
		t.Fatalf("mid-stream error must close with response.failed:\n%s", body)
	}
	if strings.Contains(body, "event: response.completed") {
		t.Fatal("a mid-stream error must NOT be masked as completed")
	}
}

// TestM4Adapter_MetricsRecorded asserts finalize records exactly one Responses
// stream into the metrics counters, with the correct terminal + tool count.
func TestM4Adapter_MetricsRecorded(t *testing.T) {
	before := metrics.ResponsesStats()

	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	a.WriteHeader(200)
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_m\",\"function\":{\"name\":\"f\",\"arguments\":\"{}\"}}]}}]}\n\n"))
	a.Write([]byte("data: [DONE]\n\n"))
	a.finalize()
	// Idempotent: a second finalize must NOT double-count.
	a.finalize()

	after := metrics.ResponsesStats()
	if d := after.StreamsStarted - before.StreamsStarted; d != 1 {
		t.Fatalf("StreamsStarted delta: got %d want 1 (double-count?)", d)
	}
	if d := after.StreamsCompleted - before.StreamsCompleted; d != 1 {
		t.Fatalf("StreamsCompleted delta: got %d want 1", d)
	}
	if d := after.ToolCalls - before.ToolCalls; d != 1 {
		t.Fatalf("ToolCalls delta: got %d want 1", d)
	}
	if d := after.TextStreams - before.TextStreams; d != 1 {
		t.Fatalf("TextStreams delta: got %d want 1", d)
	}
}

// TestM4Adapter_FlushPropagates asserts the adapter forwards Flush() to a
// flushing underlying writer (streaming correctness).
func TestM4Adapter_FlushPropagates(t *testing.T) {
	fw := &flushCountWriter{ResponseWriter: httptest.NewRecorder()}
	a := newResponsesStreamAdapter(fw, "gpt-4o")
	a.WriteHeader(200)
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"))
	if fw.flushes == 0 {
		t.Fatal("adapter did not propagate Flush to the underlying writer")
	}
	a.finalize()
}

// flushCountWriter counts Flush calls to prove streaming flushes propagate.
type flushCountWriter struct {
	http.ResponseWriter
	flushes int
}

func (f *flushCountWriter) Flush() { f.flushes++ }

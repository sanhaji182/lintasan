package server

import (
	"net/http/httptest"
	"strings"
	"testing"
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

func TestM2Adapter_TerminalOnTruncation(t *testing.T) {
	rec := httptest.NewRecorder()
	a := newResponsesStreamAdapter(rec, "gpt-4o")
	a.Header().Set("Content-Type", "text/event-stream")
	a.WriteHeader(200)
	a.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
	// Upstream truncates: NO [DONE]. finalize must still close the stream.
	a.finalize()

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.completed") {
		t.Fatalf("truncated stream not closed with response.completed:\n%s", body)
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

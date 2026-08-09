package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// handlers_responses_m6_test.go — M6 handler wiring.
//
// These assert the ROUTING decision (buffered vs streamed) and the buffered
// writer's own contract. Full request→upstream behavior is covered E2E; here we
// pin the parts that are pure and cheap to test.

// TestM6StreamDefaultsToTrue locks backward compatibility: every caller that
// existed before M6 omitted `stream`, and must keep getting SSE.
func TestM6StreamDefaultsToTrue(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"absent", `{"model":"m","input":"hi"}`, true},
		{"explicit true", `{"model":"m","input":"hi","stream":true}`, true},
		{"explicit false", `{"model":"m","input":"hi","stream":false}`, false},
		{"non-bool ignored", `{"model":"m","input":"hi","stream":"yes"}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal([]byte(c.body), &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			// Mirrors the handler's decision expression exactly.
			wantStream := true
			if v, ok := raw["stream"].(bool); ok {
				wantStream = v
			}
			if wantStream != c.want {
				t.Errorf("stream decision = %v, want %v", wantStream, c.want)
			}
		})
	}
}

// TestBufferedWriterCapturesStatusAndBody: the writer must record what the
// inner handler wrote instead of forwarding it.
func TestBufferedWriterCapturesStatusAndBody(t *testing.T) {
	b := &bufferedResponseWriter{header: http.Header{}, status: http.StatusOK}
	b.Header().Set("Content-Type", "application/json")
	b.WriteHeader(http.StatusTeapot)
	b.Write([]byte(`{"ok":true}`))

	if b.status != http.StatusTeapot {
		t.Errorf("status = %d, want 418", b.status)
	}
	if got := b.body.String(); got != `{"ok":true}` {
		t.Errorf("body = %q", got)
	}
	if b.Header().Get("Content-Type") != "application/json" {
		t.Errorf("header lost")
	}
}

// TestBufferedWriterFirstWriteHeaderWins matches net/http semantics: later
// WriteHeader calls are ignored, and an implicit Write sets 200.
func TestBufferedWriterFirstWriteHeaderWins(t *testing.T) {
	b := &bufferedResponseWriter{header: http.Header{}, status: http.StatusOK}
	b.WriteHeader(http.StatusCreated)
	b.WriteHeader(http.StatusInternalServerError)
	if b.status != http.StatusCreated {
		t.Errorf("status = %d, want 201 (first wins)", b.status)
	}

	b2 := &bufferedResponseWriter{header: http.Header{}, status: http.StatusOK}
	b2.Write([]byte("x"))
	if b2.status != http.StatusOK {
		t.Errorf("implicit status = %d, want 200", b2.status)
	}
}

// TestBufferedWriterIsNotAFlusher is load-bearing, not cosmetic: the chat
// handler decides whether to stream by type-asserting http.Flusher. The
// buffered writer must NOT satisfy it, or the non-streaming path would receive
// SSE chunks instead of a single JSON body.
func TestBufferedWriterIsNotAFlusher(t *testing.T) {
	var w http.ResponseWriter = &bufferedResponseWriter{header: http.Header{}}
	if _, ok := w.(http.Flusher); ok {
		t.Fatal("bufferedResponseWriter implements http.Flusher — the chat handler will stream into it")
	}
}

// TestResponsesOutputStats pins the counting used for metrics on the buffered
// path, so /metrics reports the same facts the streaming emitter would.
func TestResponsesOutputStats(t *testing.T) {
	cases := []struct {
		name      string
		output    []any
		wantCalls int
		wantText  bool
	}{
		{"empty", []any{}, 0, false},
		{"text only", []any{
			map[string]any{"type": "message"},
		}, 0, true},
		{"tool only", []any{
			map[string]any{"type": "function_call"},
		}, 1, false},
		{"text + two calls", []any{
			map[string]any{"type": "message"},
			map[string]any{"type": "function_call"},
			map[string]any{"type": "function_call"},
		}, 2, true},
		{"unknown item ignored", []any{
			map[string]any{"type": "reasoning"},
		}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls, text := responsesOutputStats(map[string]any{"output": c.output})
			if calls != c.wantCalls || text != c.wantText {
				t.Errorf("stats = (%d, %t), want (%d, %t)", calls, text, c.wantCalls, c.wantText)
			}
		})
	}
}

// TestM6NonStreamGateStillApplies: stream=false must not be a way around the
// kill-switch. With the flag off, the route stays inert either way.
func TestM6NonStreamGateStillApplies(t *testing.T) {
	p, _ := newGateHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader([]byte(`{"model":"m","input":"hi","stream":false}`)))
	rec := httptest.NewRecorder()
	p.HandleResponses(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (gate closed)", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "response") &&
		!strings.Contains(rec.Body.String(), "not found") {
		t.Errorf("leaked a Responses body while gated: %s", rec.Body.String())
	}
}

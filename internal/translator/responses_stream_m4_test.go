package translator

import (
	"strings"
	"testing"
)

// responses_stream_m4_test.go — Codex M4 hardening: error terminal states +
// deterministic lifecycle. Asserts the stream is NEVER left without a terminal
// and that response.failed / response.incomplete are well-formed.

// TestM4_FailEmitsFailedTerminal asserts Fail(TerminalFailed) produces a
// response.failed terminal carrying an error object, and that TerminalState
// reflects it.
func TestM4_FailEmitsFailedTerminal(t *testing.T) {
	e := NewResponsesStreamEmitter("gpt-4o")
	// Some text arrived, then upstream errored.
	_ = e.Process(`data: {"choices":[{"delta":{"content":"partial"}}]}`)
	evs := parseEvents(t, e.Fail(TerminalFailed, "boom"))
	if len(evs) == 0 {
		t.Fatal("Fail must emit at least the terminal event")
	}
	last := evs[len(evs)-1]
	if last.Type != "response.failed" {
		t.Fatalf("expected response.failed terminal, got %q", last.Type)
	}
	resp, _ := last.Data["response"].(map[string]any)
	if resp["status"] != "failed" {
		t.Fatalf("response.status: got %v want failed", resp["status"])
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("response.failed must carry an error object")
	}
	if errObj["message"] != "boom" {
		t.Fatalf("error message: got %v want boom", errObj["message"])
	}
	if e.TerminalState() != "failed" {
		t.Fatalf("TerminalState: got %q want failed", e.TerminalState())
	}
}

// TestM4_IncompleteTerminal asserts Fail(TerminalIncomplete) → response.incomplete.
func TestM4_IncompleteTerminal(t *testing.T) {
	e := NewResponsesStreamEmitter("gpt-4o")
	_ = e.Process(`data: {"choices":[{"delta":{"content":"half"}}]}`)
	evs := parseEvents(t, e.Fail(TerminalIncomplete, "truncated"))
	last := evs[len(evs)-1]
	if last.Type != "response.incomplete" {
		t.Fatalf("expected response.incomplete, got %q", last.Type)
	}
	resp, _ := last.Data["response"].(map[string]any)
	if resp["status"] != "incomplete" {
		t.Fatalf("status: got %v want incomplete", resp["status"])
	}
	if e.TerminalState() != "incomplete" {
		t.Fatalf("TerminalState: got %q want incomplete", e.TerminalState())
	}
}

// TestM4_TerminalIdempotent asserts that once a terminal is emitted, neither a
// second Fail nor a Finish can emit a second terminal (no double-close).
func TestM4_TerminalIdempotent(t *testing.T) {
	e := NewResponsesStreamEmitter("gpt-4o")
	_ = e.Process(`data: {"choices":[{"delta":{"content":"x"}}]}`)
	first := e.Fail(TerminalFailed, "err")
	if len(first) == 0 {
		t.Fatal("first Fail should emit a terminal")
	}
	if got := e.Fail(TerminalFailed, "err2"); got != nil {
		t.Fatalf("second Fail must be a no-op, got %v", got)
	}
	if got := e.Finish(); got != nil {
		t.Fatalf("Finish after Fail must be a no-op, got %v", got)
	}
	if got := e.Process(`data: [DONE]`); got != nil {
		t.Fatalf("[DONE] after terminal must be a no-op, got %v", got)
	}
}

// TestM4_CompletedThenFailNoop asserts a clean completion wins: after [DONE]
// emits response.completed, a later Fail is a no-op (we never downgrade a
// completed stream to failed).
func TestM4_CompletedThenFailNoop(t *testing.T) {
	e := NewResponsesStreamEmitter("gpt-4o")
	_ = e.Process(`data: {"choices":[{"delta":{"content":"hi"}}]}`)
	_ = e.Process(`data: [DONE]`)
	if e.TerminalState() != "completed" {
		t.Fatalf("expected completed, got %q", e.TerminalState())
	}
	if got := e.Fail(TerminalFailed, "late"); got != nil {
		t.Fatal("Fail after completed must be a no-op")
	}
	if e.TerminalState() != "completed" {
		t.Fatal("terminal state must remain completed")
	}
}

// TestM4_FailDeterministic asserts the failed terminal is byte-identical across
// runs (no map-iteration nondeterminism in the error object).
func TestM4_FailDeterministic(t *testing.T) {
	run := func() string {
		e := NewResponsesStreamEmitter("gpt-4o")
		e.Process(`data: {"choices":[{"delta":{"content":"x"}}]}`)
		var sb strings.Builder
		for _, ev := range e.Fail(TerminalFailed, "same reason") {
			sb.WriteString(ev)
		}
		return sb.String()
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("non-deterministic failed terminal:\nA=%q\nB=%q", a, b)
	}
}

// TestM4_FailBeforeAnyOutput asserts Fail on a fresh emitter still emits
// response.created first (so the stream is well-formed) then the error terminal.
func TestM4_FailBeforeAnyOutput(t *testing.T) {
	e := NewResponsesStreamEmitter("gpt-4o")
	evs := parseEvents(t, e.Fail(TerminalFailed, "early"))
	if len(evs) < 2 {
		t.Fatalf("expected created + failed, got %d events", len(evs))
	}
	if evs[0].Type != "response.created" {
		t.Fatalf("first event must be response.created, got %q", evs[0].Type)
	}
	if evs[len(evs)-1].Type != "response.failed" {
		t.Fatalf("last event must be response.failed, got %q", evs[len(evs)-1].Type)
	}
}

// TestM4_AccessorsReflectState sanity-checks ToolCallCount / HadText / Started.
func TestM4_AccessorsReflectState(t *testing.T) {
	e := NewResponsesStreamEmitter("gpt-4o")
	if e.Started() || e.HadText() || e.ToolCallCount() != 0 {
		t.Fatal("fresh emitter accessors must be zero/false")
	}
	e.Process(`data: {"choices":[{"delta":{"content":"hi"}}]}`)
	e.Process(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_z","function":{"name":"f","arguments":"{}"}}]}}]}`)
	if !e.Started() {
		t.Fatal("Started must be true after first event")
	}
	if !e.HadText() {
		t.Fatal("HadText must be true after a text delta")
	}
	if e.ToolCallCount() != 1 {
		t.Fatalf("ToolCallCount: got %d want 1", e.ToolCallCount())
	}
}

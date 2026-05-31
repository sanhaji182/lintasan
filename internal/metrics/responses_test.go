package metrics

import "testing"

// TestResponsesCounters verifies the Codex Responses-shim counters increment
// per terminal state and that the snapshot reflects them. Counters are package
// globals, so this test reads deltas rather than absolute values to stay robust
// against other tests in the package.
func TestResponsesCounters(t *testing.T) {
	before := ResponsesStats()

	// A completed stream with text and 2 tool calls.
	RecordResponsesStream(true, ResponsesTerminalCompleted, 2, true)
	// A failed stream (started, no tools, no text).
	RecordResponsesStream(true, ResponsesTerminalFailed, 0, false)
	// An incomplete stream with text.
	RecordResponsesStream(true, ResponsesTerminalIncomplete, 0, true)
	// A passthrough-style record: not started, failed terminal.
	RecordResponsesStream(false, ResponsesTerminalFailed, 0, false)

	after := ResponsesStats()

	if d := after.StreamsStarted - before.StreamsStarted; d != 3 {
		t.Fatalf("StreamsStarted delta: got %d want 3", d)
	}
	if d := after.StreamsCompleted - before.StreamsCompleted; d != 1 {
		t.Fatalf("StreamsCompleted delta: got %d want 1", d)
	}
	if d := after.StreamsFailed - before.StreamsFailed; d != 2 {
		t.Fatalf("StreamsFailed delta: got %d want 2", d)
	}
	if d := after.StreamsIncomplete - before.StreamsIncomplete; d != 1 {
		t.Fatalf("StreamsIncomplete delta: got %d want 1", d)
	}
	if d := after.ToolCalls - before.ToolCalls; d != 2 {
		t.Fatalf("ToolCalls delta: got %d want 2", d)
	}
	if d := after.TextStreams - before.TextStreams; d != 2 {
		t.Fatalf("TextStreams delta: got %d want 2", d)
	}
}

// TestResponsesCountersZeroToolCalls verifies a stream with zero tool calls does
// not bump the tool-call counter.
func TestResponsesCountersZeroToolCalls(t *testing.T) {
	before := ResponsesStats()
	RecordResponsesStream(true, ResponsesTerminalCompleted, 0, true)
	after := ResponsesStats()
	if d := after.ToolCalls - before.ToolCalls; d != 0 {
		t.Fatalf("zero-tool stream bumped ToolCalls by %d", d)
	}
}

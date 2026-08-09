package metrics

import "sync/atomic"

// responses.go — Codex Official Layer (/v1/responses) observability counters.
//
// M4 (Hardening & Observability): these track the Responses ingress shim — how
// many Responses streams were served, how they terminated (completed / failed /
// incomplete), and how much tool activity flowed through. They answer the
// operational question "is the Codex shim healthy and how is it being used?"
// WITHOUT capturing any prompt content, call_id values, or secrets — they are
// plain monotonic counters.
//
// Atomic and cheap; the Responses path is flag-gated (default OFF), so when the
// shim is disabled these stay at zero and add no cost.
//
// Distinct from the proxy response-cache counters (cache.go) and the H3 memory
// counters — those count different subsystems.
var (
	responsesStreamsStarted    atomic.Int64 // streams where response.created was emitted
	responsesStreamsCompleted  atomic.Int64 // streams terminated by response.completed
	responsesStreamsFailed     atomic.Int64 // streams terminated by response.failed
	responsesStreamsIncomplete atomic.Int64 // streams terminated by response.incomplete
	responsesToolCalls         atomic.Int64 // function_call items emitted (cumulative)
	responsesTextStreams       atomic.Int64 // streams that emitted at least one text delta
	responsesToolsDropped      atomic.Int64 // tools.N entries dropped as unrepresentable in chat (M5)
)

// ResponsesTerminal is the terminal state a Responses stream ended in. It is the
// label the emitter reports to RecordResponsesStream so the counters stay in
// lockstep with what the client actually observed.
type ResponsesTerminal string

const (
	// ResponsesTerminalCompleted — response.completed (the success terminal).
	ResponsesTerminalCompleted ResponsesTerminal = "completed"
	// ResponsesTerminalFailed — response.failed (upstream/translation error).
	ResponsesTerminalFailed ResponsesTerminal = "failed"
	// ResponsesTerminalIncomplete — response.incomplete (truncated/partial).
	ResponsesTerminalIncomplete ResponsesTerminal = "incomplete"
)

// RecordResponsesStream records one finished Responses stream. terminal is the
// state it ended in; toolCalls is the number of function_call items emitted;
// hadText reports whether any text delta was emitted. Call exactly once per
// stream, at finalize. No-op-safe for the zero values.
func RecordResponsesStream(started bool, terminal ResponsesTerminal, toolCalls int, hadText bool) {
	if started {
		responsesStreamsStarted.Add(1)
	}
	switch terminal {
	case ResponsesTerminalCompleted:
		responsesStreamsCompleted.Add(1)
	case ResponsesTerminalFailed:
		responsesStreamsFailed.Add(1)
	case ResponsesTerminalIncomplete:
		responsesStreamsIncomplete.Add(1)
	}
	if toolCalls > 0 {
		responsesToolCalls.Add(int64(toolCalls))
	}
	if hadText {
		responsesTextStreams.Add(1)
	}
}

// RecordResponsesToolsDropped adds n unrepresentable tools.N entries (provider
// built-ins like web_search, nameless/malformed tool entries) that were dropped
// during a single request's tools translation. Called once per request from the
// handler after ResponsesToolsStats, so a silently-shrunk tool list is visible
// operationally instead of only in a debug log. No-op for n<=0.
func RecordResponsesToolsDropped(n int) {
	if n > 0 {
		responsesToolsDropped.Add(int64(n))
	}
}

// ResponsesStatsSnapshot is a point-in-time snapshot of the Responses counters.
type ResponsesStatsSnapshot struct {
	StreamsStarted    int64
	StreamsCompleted  int64
	StreamsFailed     int64
	StreamsIncomplete int64
	ToolCalls         int64
	TextStreams       int64
	ToolsDropped      int64
}

// ResponsesStats returns a snapshot for the /metrics collector.
func ResponsesStats() ResponsesStatsSnapshot {
	return ResponsesStatsSnapshot{
		StreamsStarted:    responsesStreamsStarted.Load(),
		StreamsCompleted:  responsesStreamsCompleted.Load(),
		StreamsFailed:     responsesStreamsFailed.Load(),
		StreamsIncomplete: responsesStreamsIncomplete.Load(),
		ToolCalls:         responsesToolCalls.Load(),
		TextStreams:       responsesTextStreams.Load(),
		ToolsDropped:      responsesToolsDropped.Load(),
	}
}

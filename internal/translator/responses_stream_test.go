package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

// responses_stream_test.go — Codex M2 streaming conformance (Tier 2).
//
// These tests pin the EXACT typed-event lifecycle Codex requires and the
// non-negotiable terminal `response.completed`. They are golden-sequence tests:
// deterministic ordering is asserted, and the stream is verified to NEVER end
// without the terminal event.

// parseEvents splits emitted SSE strings into (eventName, dataJSON) pairs.
func parseEvents(t *testing.T, raw []string) []struct {
	Event string
	Type  string
	Data  map[string]any
} {
	t.Helper()
	var out []struct {
		Event string
		Type  string
		Data  map[string]any
	}
	for _, chunk := range raw {
		var ev, dataLine string
		for _, line := range strings.Split(chunk, "\n") {
			if strings.HasPrefix(line, "event: ") {
				ev = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
			}
		}
		if dataLine == "" {
			continue
		}
		var d map[string]any
		if err := json.Unmarshal([]byte(dataLine), &d); err != nil {
			t.Fatalf("event data is not valid JSON: %q (err %v)", dataLine, err)
		}
		typ, _ := d["type"].(string)
		// The event: line MUST mirror the JSON type field (both Codex parser paths).
		if ev != typ {
			t.Fatalf("event-name/type mismatch: event:%q type:%q", ev, typ)
		}
		out = append(out, struct {
			Event string
			Type  string
			Data  map[string]any
		}{ev, typ, d})
	}
	return out
}

func runEmitter(t *testing.T, model string, chatLines []string) []struct {
	Event string
	Type  string
	Data  map[string]any
} {
	t.Helper()
	e := NewResponsesStreamEmitter(model)
	var raw []string
	for _, l := range chatLines {
		raw = append(raw, e.Process(l)...)
	}
	raw = append(raw, e.Finish()...)
	return parseEvents(t, raw)
}

func types(evs []struct {
	Event string
	Type  string
	Data  map[string]any
}) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Type)
	}
	return out
}

// TestM2_TextStream_ExactLifecycle asserts the full mandatory event order for a
// simple multi-delta text completion.
func TestM2_TextStream_ExactLifecycle(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	got := types(evs)
	want := []string{
		"response.created",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	if len(got) != len(want) {
		t.Fatalf("event count: got %d %v want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event[%d]: got %q want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestM2_TerminalCompletedAlwaysPresent is the load-bearing invariant: Codex
// errors if the stream closes without response.completed. We assert it appears
// even when the upstream truncates (no [DONE]).
func TestM2_TerminalCompletedAlwaysPresent(t *testing.T) {
	cases := map[string][]string{
		"normal_done": {
			`data: {"choices":[{"delta":{"content":"hi"}}]}`,
			`data: [DONE]`,
		},
		"truncated_no_done": {
			`data: {"choices":[{"delta":{"content":"partial"}}]}`,
			// stream just ends — emitter.Finish() must still close it
		},
		"empty_stream": {
			// no chunks at all
		},
		"only_done": {
			`data: [DONE]`,
		},
	}
	for name, chat := range cases {
		t.Run(name, func(t *testing.T) {
			evs := runEmitter(t, "gpt-4o", chat)
			if len(evs) == 0 {
				t.Fatal("no events emitted; stream must always be a valid Responses stream")
			}
			last := evs[len(evs)-1]
			if last.Type != "response.completed" {
				t.Fatalf("stream did NOT end with response.completed (got %q) — Codex would error", last.Type)
			}
			// completed must carry a response object with usage.
			resp, ok := last.Data["response"].(map[string]any)
			if !ok {
				t.Fatal("response.completed missing 'response' object")
			}
			if _, ok := resp["usage"].(map[string]any); !ok {
				t.Fatal("response.completed missing usage")
			}
			if resp["status"] != "completed" {
				t.Fatalf("completed status: got %v want completed", resp["status"])
			}
		})
	}
}

// TestM2_DeltaTextConcatenation asserts the accumulated text appears in the
// output_text.done and final message item, in order.
func TestM2_DeltaTextConcatenation(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"content":"foo "}}]}`,
		`data: {"choices":[{"delta":{"content":"bar "}}]}`,
		`data: {"choices":[{"delta":{"content":"baz"}}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	var doneText string
	var completedText string
	for _, e := range evs {
		if e.Type == "response.output_text.done" {
			doneText, _ = e.Data["text"].(string)
		}
		if e.Type == "response.completed" {
			resp := e.Data["response"].(map[string]any)
			out := resp["output"].([]any)
			item := out[0].(map[string]any)
			content := item["content"].([]any)
			part := content[0].(map[string]any)
			completedText, _ = part["text"].(string)
		}
	}
	if doneText != "foo bar baz" {
		t.Fatalf("output_text.done text: got %q want %q", doneText, "foo bar baz")
	}
	if completedText != "foo bar baz" {
		t.Fatalf("completed message text: got %q want %q", completedText, "foo bar baz")
	}
}

// TestM2_Deterministic asserts identical input yields byte-identical event
// output across runs (no map-iteration nondeterminism, fixed IDs).
func TestM2_Deterministic(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"content":"deterministic"}}]}`,
		`data: [DONE]`,
	}
	run := func() string {
		e := NewResponsesStreamEmitter("gpt-4o")
		var sb strings.Builder
		for _, l := range chat {
			for _, ev := range e.Process(l) {
				sb.WriteString(ev)
			}
		}
		for _, ev := range e.Finish() {
			sb.WriteString(ev)
		}
		return sb.String()
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("non-deterministic output:\nA=%q\nB=%q", a, b)
	}
}

// TestM2_UsageMapped asserts upstream chat usage is renamed to Responses shape.
func TestM2_UsageMapped(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"content":"x"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	last := evs[len(evs)-1]
	resp := last.Data["response"].(map[string]any)
	usage := resp["usage"].(map[string]any)
	// JSON numbers decode as float64.
	if usage["input_tokens"].(float64) != 11 || usage["output_tokens"].(float64) != 7 || usage["total_tokens"].(float64) != 18 {
		t.Fatalf("usage not mapped correctly: %v", usage)
	}
}

// TestM3_ToolOnlyResponse_NoMessageItem asserts the M3 milestone transition:
// M2 SKIPPED tool-call deltas; M3 EMITS them as function_call items. A pure
// tool-call response (no text) must therefore emit a function_call item but NO
// message item and NO text deltas (matching the real OpenAI Responses API),
// while still terminating with response.completed.
func TestM3_ToolOnlyResponse_NoMessageItem(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"f","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	sawFunctionCall := false
	for _, e := range evs {
		if e.Type == "response.output_text.delta" {
			t.Fatal("pure tool call must not emit text deltas")
		}
		// A message item must NOT be opened for a pure tool call.
		if e.Type == "response.output_item.added" {
			item, _ := e.Data["item"].(map[string]any)
			if item["type"] == "message" {
				t.Fatal("pure tool call must not open a message item")
			}
			if item["type"] == "function_call" {
				sawFunctionCall = true
			}
		}
	}
	if !sawFunctionCall {
		t.Fatal("M3 must emit a function_call item for a tool-call delta")
	}
	if evs[len(evs)-1].Type != "response.completed" {
		t.Fatal("tool-only stream must still terminate with response.completed")
	}
}

// TestM2_NoTextNoDeltaEvents asserts an empty-content stream emits the lifecycle
// (created..completed) but zero output_text.delta events.
func TestM2_NoTextNoDeltaEvents(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"content":""}}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	for _, e := range evs {
		if e.Type == "response.output_text.delta" {
			t.Fatal("empty content must not produce a delta event")
		}
	}
	if evs[len(evs)-1].Type != "response.completed" {
		t.Fatal("must still terminate with response.completed")
	}
}

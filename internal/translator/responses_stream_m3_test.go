package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

// responses_stream_m3_test.go — Codex M3 tool round-trip (ACCEPTANCE CORE).
//
// The make-or-break of Codex compatibility is call_id fidelity: the chat
// tool_call.id must survive VERBATIM into the Responses function_call item
// (outbound), and the inbound function_call_output.call_id must survive verbatim
// into the chat tool message (handled by ResponsesToOpenAIRequest, M1). These
// tests assert the full loop with byte-identical call_id at every hop.

// findFunctionCallItems returns all function_call items observed in
// output_item.done events, in emission order.
func findFunctionCallItems(evs []struct {
	Event string
	Type  string
	Data  map[string]any
}) []map[string]any {
	var out []map[string]any
	for _, e := range evs {
		if e.Type != "response.output_item.done" {
			continue
		}
		item, _ := e.Data["item"].(map[string]any)
		if item == nil {
			continue
		}
		if item["type"] == "function_call" {
			out = append(out, item)
		}
	}
	return out
}

// TestM3_FunctionCallEmitted_CallIDVerbatim is the load-bearing assertion:
// a chat tool_call with id "call_abc123" must produce a Responses function_call
// item carrying call_id "call_abc123" — byte-identical.
func TestM3_FunctionCallEmitted_CallIDVerbatim(t *testing.T) {
	const callID = "call_abc123XYZ"
	chat := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"` + callID + `","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Jakarta\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)

	// output_item.added for the function_call must carry the verbatim call_id.
	var addedCallID string
	for _, e := range evs {
		if e.Type == "response.output_item.added" {
			item, _ := e.Data["item"].(map[string]any)
			if item["type"] == "function_call" {
				addedCallID, _ = item["call_id"].(string)
			}
		}
	}
	if addedCallID != callID {
		t.Fatalf("output_item.added call_id: got %q want %q (VERBATIM)", addedCallID, callID)
	}

	fns := findFunctionCallItems(evs)
	if len(fns) != 1 {
		t.Fatalf("expected exactly 1 function_call item, got %d", len(fns))
	}
	fn := fns[0]
	if fn["call_id"] != callID {
		t.Fatalf("function_call.done call_id: got %q want %q (VERBATIM)", fn["call_id"], callID)
	}
	if fn["name"] != "get_weather" {
		t.Fatalf("function name: got %q want get_weather", fn["name"])
	}
	if fn["arguments"] != `{"city":"Jakarta"}` {
		t.Fatalf("accumulated arguments: got %q want %q", fn["arguments"], `{"city":"Jakarta"}`)
	}
	// And the completed.output array must contain that same function_call item.
	last := evs[len(evs)-1]
	if last.Type != "response.completed" {
		t.Fatalf("must terminate with completed, got %q", last.Type)
	}
	resp := last.Data["response"].(map[string]any)
	out := resp["output"].([]any)
	if len(out) != 1 {
		t.Fatalf("completed.output should hold 1 item (the function_call), got %d", len(out))
	}
	item0 := out[0].(map[string]any)
	if item0["call_id"] != callID {
		t.Fatalf("completed.output function_call call_id: got %q want %q", item0["call_id"], callID)
	}
}

// TestM3_ParallelToolCalls_DistinctCallIDs asserts multiple parallel tool calls
// each preserve their own call_id, in order.
func TestM3_ParallelToolCalls_DistinctCallIDs(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_AAA","function":{"name":"f1","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_BBB","function":{"name":"f2","arguments":"{\"x\":1}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	fns := findFunctionCallItems(evs)
	if len(fns) != 2 {
		t.Fatalf("expected 2 function_call items, got %d", len(fns))
	}
	if fns[0]["call_id"] != "call_AAA" || fns[0]["name"] != "f1" {
		t.Fatalf("first tool call wrong: %v", fns[0])
	}
	if fns[1]["call_id"] != "call_BBB" || fns[1]["name"] != "f2" {
		t.Fatalf("second tool call wrong: %v", fns[1])
	}
	if fns[1]["arguments"] != `{"x":1}` {
		t.Fatalf("second tool args: got %q", fns[1]["arguments"])
	}
}

// TestM3_MixedTextAndToolCall asserts a response with BOTH assistant text and a
// tool call emits a message item (text) AND a function_call item, with the
// completed.output array carrying both in deterministic order.
func TestM3_MixedTextAndToolCall(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"content":"Let me check. "}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_mix","function":{"name":"lookup","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)

	// Must have a message item AND a function_call item.
	sawMsg, sawFn := false, false
	for _, e := range evs {
		if e.Type == "response.output_item.done" {
			item, _ := e.Data["item"].(map[string]any)
			switch item["type"] {
			case "message":
				sawMsg = true
			case "function_call":
				sawFn = true
				if item["call_id"] != "call_mix" {
					t.Fatalf("mixed function_call call_id: got %q", item["call_id"])
				}
			}
		}
	}
	if !sawMsg || !sawFn {
		t.Fatalf("mixed response must emit BOTH message and function_call (msg=%v fn=%v)", sawMsg, sawFn)
	}
	// completed.output holds both, message first (output_index 0).
	resp := evs[len(evs)-1].Data["response"].(map[string]any)
	out := resp["output"].([]any)
	if len(out) != 2 {
		t.Fatalf("completed.output should hold 2 items, got %d", len(out))
	}
	if out[0].(map[string]any)["type"] != "message" {
		t.Fatalf("first output item should be the message, got %v", out[0])
	}
	if out[1].(map[string]any)["type"] != "function_call" {
		t.Fatalf("second output item should be the function_call, got %v", out[1])
	}
}

// TestM3_RoundTrip_CallIDFidelity is THE acceptance-core test. It simulates a
// full agent loop and asserts call_id is byte-identical at every hop:
//
//	turn 1: Lintasan emits function_call(call_id=X)
//	  client runs the tool, sends turn 2 with function_call_output(call_id=X)
//	turn 2: ResponsesToOpenAIRequest must produce a chat `tool` message whose
//	        tool_call_id == X, AND replay the function_call as an assistant
//	        message whose tool_calls[0].id == X.
//
// If X is not preserved verbatim across this loop, Codex compatibility FAILS.
func TestM3_RoundTrip_CallIDFidelity(t *testing.T) {
	const callID = "call_roundtrip_7f3a"

	// ── Turn 1: emitter produces a function_call with this call_id. ──
	chat := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"` + callID + `","function":{"name":"search","arguments":"{\"q\":\"go\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	fns := findFunctionCallItems(evs)
	if len(fns) != 1 || fns[0]["call_id"] != callID {
		t.Fatalf("turn 1: function_call call_id not preserved: %v", fns)
	}
	emittedName, _ := fns[0]["name"].(string)
	emittedArgs, _ := fns[0]["arguments"].(string)

	// ── Turn 2: the client sends back the SAME call_id as a function_call_output
	// plus a replay of the function_call, in the Responses `input` array. ──
	turn2 := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "find go docs"},
			map[string]any{
				"type":      "function_call",
				"call_id":   callID,
				"name":      emittedName,
				"arguments": emittedArgs,
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  `{"results":["golang.org"]}`,
			},
		},
	}
	chatReq, err := ResponsesToOpenAIRequest(turn2)
	if err != nil {
		t.Fatalf("turn 2 translation failed: %v", err)
	}
	// Mimic the real handler path: the request is JSON-marshaled then re-parsed
	// by HandleChatCompletions, which normalizes Go-native []map[string]any into
	// []any. Round-trip here so the test inspects exactly what the pipeline sees.
	chatReq = jsonRoundTrip(t, chatReq)
	msgs, _ := chatReq["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("turn 2: expected 3 chat messages (user, assistant tool_call, tool result), got %d: %v", len(msgs), msgs)
	}

	// Assistant tool_call replay must carry the verbatim id.
	assistant, _ := msgs[1].(map[string]any)
	if assistant["role"] != "assistant" {
		t.Fatalf("turn 2 msg[1] should be assistant, got %v", assistant["role"])
	}
	tcs, _ := assistant["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("turn 2 assistant should replay 1 tool_call, got %d", len(tcs))
	}
	tc0, _ := tcs[0].(map[string]any)
	if tc0["id"] != callID {
		t.Fatalf("turn 2 assistant tool_call.id: got %q want %q (VERBATIM)", tc0["id"], callID)
	}

	// Tool result must carry the verbatim tool_call_id linking back to the call.
	toolMsg, _ := msgs[2].(map[string]any)
	if toolMsg["role"] != "tool" {
		t.Fatalf("turn 2 msg[2] should be tool, got %v", toolMsg["role"])
	}
	if toolMsg["tool_call_id"] != callID {
		t.Fatalf("turn 2 tool result tool_call_id: got %q want %q (VERBATIM)", toolMsg["tool_call_id"], callID)
	}
	if !strings.Contains(toolMsg["content"].(string), "golang.org") {
		t.Fatalf("turn 2 tool result content lost: %v", toolMsg["content"])
	}

	// The acceptance invariant, stated explicitly: same id at all four hops.
	if !(fns[0]["call_id"] == callID && tc0["id"] == callID && toolMsg["tool_call_id"] == callID) {
		t.Fatal("CALL_ID FIDELITY FAILED — Codex compatibility would be broken")
	}
}

// TestM3_Deterministic asserts tool-call streams are deterministic across runs.
func TestM3_Deterministic(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_det","function":{"name":"f","arguments":"{\"a\":1}"}}]}}]}`,
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
	if a, b := run(), run(); a != b {
		t.Fatalf("non-deterministic tool stream:\nA=%q\nB=%q", a, b)
	}
}

// TestM3_ToolCallNoID_NotFabricated asserts that a tool_call delta which never
// carries an id is NOT emitted with a fabricated call_id (it can't round-trip).
// The stream must still be valid (terminates with completed).
func TestM3_ToolCallNoID_NotFabricated(t *testing.T) {
	chat := []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"f","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}
	evs := runEmitter(t, "gpt-4o", chat)
	if len(findFunctionCallItems(evs)) != 0 {
		t.Fatal("a tool_call with no id must NOT be emitted with a fabricated call_id")
	}
	if evs[len(evs)-1].Type != "response.completed" {
		t.Fatal("stream must still terminate with completed")
	}
}

// TestM3_ResponsesInputToolChoiceForcedRoundTrips sanity-checks the request side
// preserves a forced tool_choice across the round trip (object form normalize).
func TestM3_ResponsesInputToolChoiceForcedRoundTrips(t *testing.T) {
	req := map[string]any{
		"model":       "gpt-4o",
		"input":       "go",
		"tools":       []any{map[string]any{"type": "function", "name": "f", "parameters": map[string]any{}}},
		"tool_choice": map[string]any{"type": "function", "name": "f"},
	}
	out, err := ResponsesToOpenAIRequest(req)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	out = jsonRoundTrip(t, out)
	tc, _ := out["tool_choice"].(map[string]any)
	fn, _ := tc["function"].(map[string]any)
	if fn["name"] != "f" {
		t.Fatalf("forced tool_choice not normalized: %v", out["tool_choice"])
	}
	// Tools must be nested chat shape.
	tools, _ := out["tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("tools missing after translation: %v", out["tools"])
	}
	t0, _ := tools[0].(map[string]any)
	if _, ok := t0["function"].(map[string]any); !ok {
		t.Fatalf("tool not nested into chat shape: %v", t0)
	}
}

// jsonRoundTrip marshals a translated request and re-parses it, mimicking the
// real handler path (handler json.Marshals chatReq, HandleChatCompletions
// re-parses). This normalizes Go-native []map[string]any → []any exactly as the
// pipeline sees it, so tests assert against production-shaped data.
func jsonRoundTrip(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

// responses_nonstream_m6_test.go — M6: non-streaming Responses path.
//
// The contract that matters operationally: a client asking for stream=false
// must get the SAME response object it would have assembled from the streaming
// terminal event. These tests pin the shape and, most importantly, assert
// stream/non-stream PARITY so the two paths cannot drift apart.

func mustResponses(t *testing.T, chat map[string]any) map[string]any {
	t.Helper()
	got, err := OpenAIResponseToResponses(chat)
	if err != nil {
		t.Fatalf("OpenAIResponseToResponses: %v", err)
	}
	return got
}

// chatBody builds a canonical non-streaming chat-completion body.
func chatBody(content string, toolCalls []any) map[string]any {
	msg := map[string]any{"role": "assistant"}
	if content != "" {
		msg["content"] = content
	}
	if toolCalls != nil {
		msg["tool_calls"] = toolCalls
	}
	return map[string]any{
		"model":   "test-model",
		"choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": "stop"}},
		"usage": map[string]any{
			"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18,
		},
	}
}

func TestM6TextResponseShape(t *testing.T) {
	got := mustResponses(t, chatBody("Hello there", nil))

	if got["object"] != "response" {
		t.Errorf("object = %v, want response", got["object"])
	}
	if got["status"] != "completed" {
		t.Errorf("status = %v, want completed", got["status"])
	}
	if got["model"] != "test-model" {
		t.Errorf("model = %v, want test-model", got["model"])
	}
	if id, _ := got["id"].(string); !strings.HasPrefix(id, "resp_") {
		t.Errorf("id = %q, want resp_ prefix", id)
	}

	out, _ := got["output"].([]any)
	if len(out) != 1 {
		t.Fatalf("output len = %d, want 1", len(out))
	}
	item, _ := out[0].(map[string]any)
	if item["type"] != "message" || item["role"] != "assistant" || item["status"] != "completed" {
		t.Errorf("message item = %+v", item)
	}
	content, _ := item["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1", len(content))
	}
	part, _ := content[0].(map[string]any)
	if part["type"] != "output_text" || part["text"] != "Hello there" {
		t.Errorf("content part = %+v", part)
	}

	usage, _ := got["usage"].(map[string]any)
	if usage["input_tokens"] != 11 || usage["output_tokens"] != 7 || usage["total_tokens"] != 18 {
		t.Errorf("usage = %+v, want renamed to Responses vocabulary", usage)
	}
}

// TestM6ToolOnlyEmitsNoMessageItem: a pure tool-call turn must NOT invent an
// empty message item — same rule the streaming emitter follows.
func TestM6ToolOnlyEmitsNoMessageItem(t *testing.T) {
	got := mustResponses(t, chatBody("", []any{
		map[string]any{
			"id":       "call_abc123",
			"type":     "function",
			"function": map[string]any{"name": "get_weather", "arguments": `{"city":"Jakarta"}`},
		},
	}))

	out, _ := got["output"].([]any)
	if len(out) != 1 {
		t.Fatalf("output len = %d, want 1 (function_call only, no empty message)", len(out))
	}
	item, _ := out[0].(map[string]any)
	if item["type"] != "function_call" {
		t.Fatalf("item type = %v, want function_call", item["type"])
	}
	if item["call_id"] != "call_abc123" {
		t.Errorf("call_id = %v, want call_abc123 preserved verbatim", item["call_id"])
	}
	if item["name"] != "get_weather" {
		t.Errorf("name = %v", item["name"])
	}
	if item["arguments"] != `{"city":"Jakarta"}` {
		t.Errorf("arguments = %v", item["arguments"])
	}
}

// TestM6TextAndToolOrdering: message item first, then tool calls in upstream
// order — matching the streaming emitter's output_index ordering.
func TestM6TextAndToolOrdering(t *testing.T) {
	got := mustResponses(t, chatBody("Let me check.", []any{
		map[string]any{"id": "call_1", "function": map[string]any{"name": "a", "arguments": "{}"}},
		map[string]any{"id": "call_2", "function": map[string]any{"name": "b", "arguments": "{}"}},
	}))

	out, _ := got["output"].([]any)
	if len(out) != 3 {
		t.Fatalf("output len = %d, want 3 (message + 2 calls)", len(out))
	}
	types := make([]string, len(out))
	ids := make([]string, len(out))
	for i, o := range out {
		m, _ := o.(map[string]any)
		types[i], _ = m["type"].(string)
		ids[i], _ = m["call_id"].(string)
	}
	if types[0] != "message" || types[1] != "function_call" || types[2] != "function_call" {
		t.Errorf("ordering = %v, want message then function_calls", types)
	}
	if ids[1] != "call_1" || ids[2] != "call_2" {
		t.Errorf("call order = %v, want call_1 then call_2", ids[1:])
	}
}

// TestM6MissingArgumentsDefaultsToEmptyObject mirrors the streaming path, where
// an absent arguments string becomes "{}" rather than "".
func TestM6MissingArgumentsDefaultsToEmptyObject(t *testing.T) {
	got := mustResponses(t, chatBody("", []any{
		map[string]any{"id": "call_x", "function": map[string]any{"name": "noargs"}},
	}))
	out, _ := got["output"].([]any)
	item, _ := out[0].(map[string]any)
	if item["arguments"] != "{}" {
		t.Errorf("arguments = %q, want \"{}\"", item["arguments"])
	}
}

// TestM6ToolCallWithoutIDIsSkipped: a call with no id can never be answered
// with a matching function_call_output, so it must be dropped, not fabricated.
func TestM6ToolCallWithoutIDIsSkipped(t *testing.T) {
	got := mustResponses(t, chatBody("hi", []any{
		map[string]any{"function": map[string]any{"name": "orphan", "arguments": "{}"}},
	}))
	out, _ := got["output"].([]any)
	for _, o := range out {
		m, _ := o.(map[string]any)
		if m["type"] == "function_call" {
			t.Fatalf("id-less tool call was emitted: %+v", m)
		}
	}
}

func TestM6BadUpstreamIsAnError(t *testing.T) {
	cases := map[string]map[string]any{
		"nil body":     nil,
		"no choices":   {"model": "m"},
		"empty choice": {"model": "m", "choices": []any{}},
		"no message":   {"model": "m", "choices": []any{map[string]any{"index": 0}}},
	}
	for name, body := range cases {
		if _, err := OpenAIResponseToResponses(body); err != ErrResponsesBadUpstream {
			t.Errorf("%s: err = %v, want ErrResponsesBadUpstream", name, err)
		}
	}
}

func TestM6MissingUsageIsZeroFilled(t *testing.T) {
	got := mustResponses(t, map[string]any{
		"model": "m",
		"choices": []any{map[string]any{
			"message": map[string]any{"role": "assistant", "content": "x"},
		}},
	})
	usage, _ := got["usage"].(map[string]any)
	for _, k := range []string{"input_tokens", "output_tokens", "total_tokens"} {
		if usage[k] != 0 {
			t.Errorf("usage[%s] = %v, want 0", k, usage[k])
		}
	}
}

// TestM6ParityWithStreamingTerminal is the important one: build the SAME turn
// through the streaming emitter and through the non-streaming converter, then
// compare the resulting response objects field by field (ignoring the random
// ids, which differ by construction). If someone changes one path's shape, this
// fails.
func TestM6ParityWithStreamingTerminal(t *testing.T) {
	const model = "parity-model"

	// --- streaming path: feed canonical chat SSE chunks, take the terminal.
	em := NewResponsesStreamEmitter(model)
	var events []string
	events = append(events, em.Process(`data: {"choices":[{"delta":{"content":"Hello there"}}]}`)...)
	events = append(events, em.Process(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc123","function":{"name":"get_weather","arguments":"{\"city\":\"Jakarta\"}"}}]}}]}`)...)
	events = append(events, em.Process(`data: {"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)...)
	events = append(events, em.Finish()...)

	var streamResp map[string]any
	for _, ev := range events {
		if !strings.Contains(ev, `"type":"response.completed"`) {
			continue
		}
		payload := ev[strings.Index(ev, "data: ")+len("data: "):]
		payload = strings.TrimSpace(payload)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			t.Fatalf("parse terminal event: %v", err)
		}
		streamResp, _ = parsed["response"].(map[string]any)
	}
	if streamResp == nil {
		t.Fatal("streaming path produced no response.completed terminal")
	}

	// --- non-streaming path: same turn as a buffered chat body.
	buffered := mustResponses(t, map[string]any{
		"model": model,
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": "Hello there",
				"tool_calls": []any{map[string]any{
					"id":       "call_abc123",
					"function": map[string]any{"name": "get_weather", "arguments": `{"city":"Jakarta"}`},
				}},
			},
		}},
		"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
	})

	// Top-level scalars must match exactly.
	for _, k := range []string{"object", "status", "model"} {
		if streamResp[k] != buffered[k] {
			t.Errorf("%s: streaming=%v buffered=%v", k, streamResp[k], buffered[k])
		}
	}

	// Usage must match exactly.
	su, _ := json.Marshal(streamResp["usage"])
	bu, _ := json.Marshal(buffered["usage"])
	if string(su) != string(bu) {
		t.Errorf("usage differs:\n  streaming=%s\n  buffered =%s", su, bu)
	}

	// Output items must match once the random ids are normalized away.
	so := normalizeOutputIDs(t, streamResp["output"])
	bo := normalizeOutputIDs(t, buffered["output"])
	if so != bo {
		t.Errorf("output differs between paths:\n  streaming=%s\n  buffered =%s", so, bo)
	}
}

// normalizeOutputIDs strips the per-run random `id` fields (msg_*/fc_*) so two
// structurally identical outputs compare equal. call_id is NOT stripped — it
// must survive verbatim and is part of the contract.
func normalizeOutputIDs(t *testing.T, output any) string {
	t.Helper()
	items, _ := output.([]any)
	cleaned := make([]any, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		copied := map[string]any{}
		for k, v := range m {
			if k == "id" {
				continue
			}
			copied[k] = v
		}
		cleaned = append(cleaned, copied)
	}
	b, err := json.Marshal(cleaned)
	if err != nil {
		t.Fatalf("marshal normalized output: %v", err)
	}
	return string(b)
}

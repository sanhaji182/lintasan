package translator

import (
	"errors"
	"reflect"
	"testing"
)

// responses_test.go — Codex M1 translator tests (request direction).
//
// Locks: deterministic translation (same input → same output), deterministic
// validation (sentinel errors in fixed order), tool-item translation (NOT tool
// execution), and the M0 invariant that FormatResponses stays unwired from the
// generic 5-format machinery. Response-direction (M2) stub stays unimplemented.

// TestResponsesFormatConstant: the 6th format constant value.
func TestResponsesFormatConstant(t *testing.T) {
	if FormatResponses != "responses" {
		t.Errorf("FormatResponses must be \"responses\", got %q", FormatResponses)
	}
}

// TestResponsesNotWiredIntoExistingFormats: M1 must NOT add FormatResponses to
// AllFormats() — existing translator behavior stays byte-identical.
func TestResponsesNotWiredIntoExistingFormats(t *testing.T) {
	for _, f := range AllFormats() {
		if f == FormatResponses {
			t.Fatal("FormatResponses must NOT be in AllFormats() (would alter existing translator behavior)")
		}
	}
	if len(AllFormats()) != 5 {
		t.Errorf("expected 5 existing formats, got %d", len(AllFormats()))
	}
}

// TestResponsesResponseDirectionImplemented: the response-direction helper was
// an M2-era stub; M6 implements it for the non-streaming path. A body with no
// usable choice is now a real error (ErrResponsesBadUpstream), not a
// not-implemented marker. Shape and stream/non-stream parity are covered in
// responses_nonstream_m6_test.go.
func TestResponsesResponseDirectionImplemented(t *testing.T) {
	// No choices → upstream error, NOT ErrResponsesNotImplemented.
	_, err := OpenAIResponseToResponses(map[string]any{"object": "chat.completion"})
	if errors.Is(err, ErrResponsesNotImplemented) {
		t.Fatal("OpenAIResponseToResponses is still a stub — M6 should have implemented it")
	}
	if !errors.Is(err, ErrResponsesBadUpstream) {
		t.Errorf("err = %v, want ErrResponsesBadUpstream", err)
	}

	// A well-formed body converts successfully.
	got, err := OpenAIResponseToResponses(map[string]any{
		"model": "m",
		"choices": []any{map[string]any{
			"message": map[string]any{"role": "assistant", "content": "hi"},
		}},
	})
	if err != nil {
		t.Fatalf("valid body: unexpected error %v", err)
	}
	if got["object"] != "response" || got["status"] != "completed" {
		t.Errorf("unexpected response object: %+v", got)
	}
}

// ── Validation (deterministic, sentinel errors) ─────────────────────────────

func TestValidateResponsesRequest(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want error
	}{
		{"ok-string-input", map[string]any{"model": "gpt-5", "input": "hi"}, nil},
		{"ok-array-input", map[string]any{"model": "gpt-5", "input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"},
		}}, nil},
		{"no-model", map[string]any{"input": "hi"}, ErrResponsesNoModel},
		{"empty-model", map[string]any{"model": "", "input": "hi"}, ErrResponsesNoModel},
		{"no-input", map[string]any{"model": "gpt-5"}, ErrResponsesNoInput},
		{"bad-input-type", map[string]any{"model": "gpt-5", "input": 42}, ErrResponsesBadInput},
		{"bad-item", map[string]any{"model": "gpt-5", "input": []any{"notamap"}}, ErrResponsesBadItem},
		{"fcall-no-callid", map[string]any{"model": "gpt-5", "input": []any{
			map[string]any{"type": "function_call", "name": "x", "arguments": "{}"},
		}}, ErrResponsesNoCallID},
		{"fcall-output-no-callid", map[string]any{"model": "gpt-5", "input": []any{
			map[string]any{"type": "function_call_output", "output": "ok"},
		}}, ErrResponsesNoCallID},
		{"bad-tools-type", map[string]any{"model": "gpt-5", "input": "hi", "tools": "nope"}, ErrResponsesBadToolsTy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateResponsesRequest(tc.raw); !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// TestValidateDeterministicOrder: model is checked before input (fixed order).
func TestValidateDeterministicOrder(t *testing.T) {
	// Missing BOTH model and input → must always report model first.
	err := ValidateResponsesRequest(map[string]any{})
	if !errors.Is(err, ErrResponsesNoModel) {
		t.Errorf("check order must be model-first, got %v", err)
	}
}

// ── Translation (deterministic) ─────────────────────────────────────────────

// TestTranslateStringInput: string input + instructions → system+user messages.
func TestTranslateStringInput(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model":             "gpt-5",
		"instructions":      "be terse",
		"input":             "hello",
		"max_output_tokens": float64(256),
		"temperature":       0.5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["model"] != "gpt-5" {
		t.Errorf("model: got %v", out["model"])
	}
	if out["max_tokens"] != float64(256) {
		t.Errorf("max_output_tokens must rename to max_tokens, got %v", out["max_tokens"])
	}
	if out["temperature"] != 0.5 {
		t.Errorf("temperature passthrough failed: %v", out["temperature"])
	}
	msgs, _ := out["messages"].([]map[string]any)
	if len(msgs) != 2 {
		t.Fatalf("expected system+user, got %d messages", len(msgs))
	}
	if msgs[0]["role"] != "system" || msgs[0]["content"] != "be terse" {
		t.Errorf("system message wrong: %+v", msgs[0])
	}
	if msgs[1]["role"] != "user" || msgs[1]["content"] != "hello" {
		t.Errorf("user message wrong: %+v", msgs[1])
	}
}

// TestTranslateDeterministic: same input twice → byte-identical output (via
// reflect.DeepEqual). Guards against map-iteration nondeterminism leaking out.
func TestTranslateDeterministic(t *testing.T) {
	in := map[string]any{
		"model":        "gpt-5",
		"instructions": "sys",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "a"},
			map[string]any{"type": "function_call", "call_id": "c1", "name": "f", "arguments": map[string]any{"x": 1}},
			map[string]any{"type": "function_call_output", "call_id": "c1", "output": "done"},
		},
		"tools": []any{
			map[string]any{"type": "function", "name": "f", "description": "d", "parameters": map[string]any{"type": "object"}},
		},
		"tool_choice": "auto",
	}
	a, err1 := ResponsesToOpenAIRequest(in)
	b, err2 := ResponsesToOpenAIRequest(in)
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v %v", err1, err2)
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("translation is not deterministic (DeepEqual mismatch on identical input)")
	}
}

// TestTranslateToolRoundTripShapes: function_call → assistant tool_calls,
// function_call_output → tool message, call_id preserved verbatim BOTH ways.
// (This is request-schema translation, NOT tool execution/loop — M1 scope.)
func TestTranslateToolCallItems(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "call_abc", "name": "get_weather", "arguments": `{"city":"jkt"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_abc", "output": "sunny"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := out["messages"].([]map[string]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	// assistant tool_call
	if msgs[0]["role"] != "assistant" {
		t.Errorf("first msg role: %v", msgs[0]["role"])
	}
	tcs, ok := msgs[0]["tool_calls"].([]any)
	if !ok || len(tcs) != 1 {
		t.Fatalf("expected 1 tool_call, got %v", msgs[0]["tool_calls"])
	}
	tc := tcs[0].(map[string]any)
	if tc["id"] != "call_abc" {
		t.Errorf("call_id not preserved on tool_call: %v", tc["id"])
	}
	fn := tc["function"].(map[string]any)
	if fn["name"] != "get_weather" || fn["arguments"] != `{"city":"jkt"}` {
		t.Errorf("function fields wrong: %+v", fn)
	}
	// tool result
	if msgs[1]["role"] != "tool" {
		t.Errorf("second msg role: %v", msgs[1]["role"])
	}
	if msgs[1]["tool_call_id"] != "call_abc" {
		t.Errorf("call_id not preserved on tool result: %v", msgs[1]["tool_call_id"])
	}
	if msgs[1]["content"] != "sunny" {
		t.Errorf("tool result content wrong: %v", msgs[1]["content"])
	}
}

// TestTranslateToolsShape: Responses flat tool shape → chat nested shape.
func TestTranslateToolsShape(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": "hi",
		"tools": []any{
			map[string]any{"type": "function", "name": "f", "description": "d", "parameters": map[string]any{"type": "object"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools := out["tools"].([]map[string]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0]["type"] != "function" {
		t.Errorf("tool type: %v", tools[0]["type"])
	}
	fn, ok := tools[0]["function"].(map[string]any)
	if !ok {
		t.Fatalf("tool not nested into function: %+v", tools[0])
	}
	if fn["name"] != "f" || fn["description"] != "d" {
		t.Errorf("nested function fields wrong: %+v", fn)
	}
}

// TestTranslateMultimodalInput: an input_image part yields chat multipart content.
func TestTranslateMultimodalInput(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "what is this"},
				map[string]any{"type": "input_image", "image_url": "https://x/y.png"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := out["messages"].([]map[string]any)
	parts, ok := msgs[0]["content"].([]any)
	if !ok {
		t.Fatalf("multimodal content must be an array, got %T", msgs[0]["content"])
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(parts))
	}
	img := parts[1].(map[string]any)
	if img["type"] != "image_url" {
		t.Errorf("expected image_url part, got %v", img["type"])
	}
	iu := img["image_url"].(map[string]any)
	if iu["url"] != "https://x/y.png" {
		t.Errorf("image url wrong: %v", iu["url"])
	}
}

// TestTranslateRejectsInvalid: translation surfaces validation errors (no panic,
// deterministic sentinel).
func TestTranslateRejectsInvalid(t *testing.T) {
	if _, err := ResponsesToOpenAIRequest(map[string]any{"input": "hi"}); !errors.Is(err, ErrResponsesNoModel) {
		t.Errorf("expected ErrResponsesNoModel, got %v", err)
	}
}

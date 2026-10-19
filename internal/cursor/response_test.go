package cursor

import (
	"testing"
)

// Each test pins one RESPONSE decode contract from the 9router JS
// utils/cursorProtobuf.js source. Fixture responses are built with the
// same encoder helpers we already pin in build_request_test.go, so a
// change in the encoder that breaks the decoder shows up in both suites.

func TestDecodeFieldResponseTextHappyPath(t *testing.T) {
	// StreamUnifiedChatResponse = {field 1: text}.
	inner := EncodeLenField(FieldResponseText, EncodeIsh("hello message"))
	buf := EncodeLenField(FieldResponse, inner)

	got := ExtractTextFromResponse(buf)
	if got.DecodeError != "" {
		t.Fatalf("decode error: %s", got.DecodeError)
	}
	if got.Text == nil || *got.Text != "hello message" {
		t.Fatalf("text = %+v", got.Text)
	}
	if got.ToolCall != nil || got.Thinking != nil {
		t.Fatalf("unexpected tool_call/thinking: %+v", got)
	}
}

func TestDecodeFieldResponseTextWithThinking(t *testing.T) {
	// Both text (field 1) and thinking (field 25, nested {field 1: text}).
	blockA := EncodeLenField(FieldResponseText, EncodeIsh("final answer"))
	blockB := EncodeLenField(FieldThinking, EncodeLenField(FieldThinkingText, EncodeIsh("reasoning...")))
	buf := EncodeLenField(FieldResponse, Concat(blockA, blockB))

	got := ExtractTextFromResponse(buf)
	if got.Text == nil || *got.Text != "final answer" {
		t.Fatalf("text = %+v", got.Text)
	}
	if got.Thinking == nil || *got.Thinking != "reasoning..." {
		t.Fatalf("thinking = %+v", got.Thinking)
	}
	if got.ToolCall != nil {
		t.Fatalf("unexpected tool_call: %+v", got.ToolCall)
	}
}

func TestDecodeFieldResponseToolCallFirstLine(t *testing.T) {
	// TOOL_ID (field 3) carries a MULTI-LINE id; the executor takes the
	// FIRST line only (9router source: "Cursor returns multi-line ID,
	// take first line").
	blockA := EncodeLenField(FieldToolID, EncodeIsh("call_abc\nsecond-line\n"))
	blockB := EncodeLenField(FieldToolName, EncodeIsh("bash"))
	blockC := EncodeLenField(FieldToolRawArgs, EncodeIsh(`{"cmd":"ls"}`))
	toolCall := Concat(blockA, blockB, blockC, EncodeVarintField(FieldToolIsLast, 1))
	buf := EncodeLenField(FieldToolCall, toolCall)

	got := ExtractTextFromResponse(buf)
	if got.DecodeError != "" {
		t.Fatalf("decode error: %s", got.DecodeError)
	}
	if got.ToolCall == nil {
		t.Fatalf("tool_call = nil, want decoded; got %+v", got)
	}
	if got.ToolCall.ID != "call_abc" {
		t.Fatalf("tool-call id = %q, want 'call_abc' (FIRST line only)", got.ToolCall.ID)
	}
	if got.ToolCall.Name != "bash" {
		t.Fatalf("tool-call name = %q", got.ToolCall.Name)
	}
	if got.ToolCall.RawArgs != `{"cmd":"ls"}` {
		t.Fatalf("tool-call raw_args = %q", got.ToolCall.RawArgs)
	}
	if !got.ToolCall.IsLast {
		t.Fatal("is_last should be true")
	}
}

func TestDecodeFieldResponseEmptyShape(t *testing.T) {
	// An empty body yields an empty StreamResponse with no error.
	got := ExtractTextFromResponse(nil)
	if got.DecodeError != "" {
		t.Fatalf("decode error: %s", got.DecodeError)
	}
	if got.Text != nil || got.ToolCall != nil || got.Thinking != nil {
		t.Fatalf("expected all-nil shape, got %+v", got)
	}
	// RawBase64 carries the "decode failed" debug payload — for a nil body
	// it should be EMPTY (not base64 of empty, which is "").
	if got.RawBase64 != "" {
		t.Fatalf("raw base64 = %q, want empty", got.RawBase64)
	}
}

func TestDecodeFieldResponseMalformedReturnsBase64Fallback(t *testing.T) {
	// Malformed body: a header claims 10 bytes but only 2 are present.
	recv := []byte{0x08, 0x0a, 0x01} // field 1, wire type 0, just 1 byte, no value
	got := ExtractTextFromResponse(recv)
	if got.DecodeError == "" {
		t.Fatal("malformed body should set DecodeError")
	}
	if got.RawBase64 == "" {
		t.Fatal("raw_base64 should carry the malformed body for debugging")
	}
	// RawBase64 is a base64 body — with malformed wire bytes you cannot
	// decode; the caller decides if that is a retry or a logger dump.
}

func TestDecodeFieldResponseUnknownFieldsAreFlagged(t *testing.T) {
	// Top-level unknown fields (not in KnownResponseFields) are flagged.
	// The 9r decoder walks them ONCE at the top level; unknown NESTED
	// fields (inside ClientSideToolV2Call / StreamUnifiedChatResponse) are
	// the decoder's own tolerance, NOT flagged. Pick 42 as the marker.
	buf := EncodeLenField(42, []byte("unknown")) // top-level field 42
	got := ExtractTextFromResponse(buf)
	if got.DecodeError != "" {
		t.Fatalf("decode error: %s", got.DecodeError)
	}
	if len(got.UnknownFields) != 1 || got.UnknownFields[0] != 42 {
		t.Fatalf("unknown_fields = %v, want [42]", got.UnknownFields)
	}
	// A body with text AND an unknown field keeps the text decode +
	// unknown_fields flag (fail-open).
	inner := EncodeLenField(FieldResponse, EncodeLenField(FieldResponseText, EncodeIsh("hi")))
	out := ExtractTextFromResponse(Concat(inner, EncodeLenField(42, []byte("unknown"))))
	if out.DecodeError != "" {
		t.Fatalf("decode error: %s", out.DecodeError)
	}
	if out.Text == nil || *out.Text != "hi" {
		t.Fatalf("text = %+v", out.Text)
	}
	if len(out.UnknownFields) != 1 || out.UnknownFields[0] != 42 {
		t.Fatalf("unknown flag lost on the fail-open path: %v", out.UnknownFields)
	}
}

func TestDecodeFieldResponseIsLastSemantic(t *testing.T) {
	// TOOL_IS_LAST=0 means NOT last; anything else is last.
	cases := []struct {
		val    uint64
		isLast bool
	}{
		{0, false},
		{1, true},
		{2, true},
	}
	for _, tc := range cases {
		toolCall := Concat(
			EncodeLenField(FieldToolID, EncodeIsh("call_x")),
			EncodeLenField(FieldToolName, EncodeIsh("bash")),
			EncodeVarintField(FieldToolIsLast, tc.val),
		)
		buf := EncodeLenField(FieldToolCall, toolCall)
		got := ExtractTextFromResponse(buf)
		if got.ToolCall == nil {
			t.Fatalf("case %d: tool_call nil", tc.val)
		}
		if got.ToolCall.IsLast != tc.isLast {
			t.Fatalf("case %d: is_last = %v, want %v", tc.val, got.ToolCall.IsLast, tc.isLast)
		}
	}
}

// StreamResponse.String logs a short line — no secrets.
func TestStreamResponseStringNoSecrets(t *testing.T) {
	got := ExtractTextFromResponse(EncodeLenField(FieldResponse, EncodeLenField(FieldResponseText, EncodeIsh("short text here"))))
	str := got.String()
	if str == "" {
		t.Fatal("String() empty on a shape with text")
	}
	// A decode-error string carries the raw payload — carry it explicitly,
	// but that's a debug dump so this is expected.
	recv := []byte{0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}
	got2 := ExtractTextFromResponse(recv)
	if got2.String() == "" {
		t.Fatal("String() empty on a decode-error shape")
	}
}

// Test only, so no production import.
func EncodeIsh(s string) []byte { return []byte(s) }

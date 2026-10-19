package windsurf

import (
	"bytes"
	"testing"
)

// Each test pins one wire-shape contract against a hand-built protobuf, so a
// later map edit shows up as exactly the wire field it breaks.

func TestEncVarintSingleByteAndMulti(t *testing.T) {
	// 0: single byte, 0x00.
	if got := EncVarint(0); !bytes.Equal(got, []byte{0x00}) {
		t.Fatalf("varint(0) = %v, want [0x00]", got)
	}
	// 127: single byte 0x7f.
	if got := EncVarint(127); !bytes.Equal(got, []byte{0x7f}) {
		t.Fatalf("varint(127) = %v, want [0x7f]", got)
	}
	// 128: two bytes 0x80 + 0x01.
	if got := EncVarint(128); !bytes.Equal(got, []byte{0x80, 0x01}) {
		t.Fatalf("varint(128) = %v, want [0x80 0x01]", got)
	}
	// Round-trip with the decoder.
	v := uint64(0xdeadbeef)
	enc := EncVarint(v)
	decoded, off := ReadVarint(enc, 0)
	if decoded != v || off != len(enc) {
		t.Fatalf("round-trip 0x%x → off=%d (len=%d), value=%d", v, off, len(enc), decoded)
	}
}

func TestTagAndStrFieldShapes(t *testing.T) {
	// Field 1, wire type 2 (length-delimited): tag byte is 1<<3|2 = 0x0a.
	// Then a length 3, then "abc".
	got := Str(1, "abc")
	if !bytes.Equal(got, []byte{0x0a, 0x03, 'a', 'b', 'c'}) {
		t.Fatalf("Str(1, 'abc') = %v, want [0x0a 0x03 'a' 'b' 'c']", got)
	}
	// Field 2 varint: tag 2<<3|0 = 0x10, then the value.
	got = VarintField(2, 300)
	// 300 is two varint bytes.
	if !bytes.Equal(got, []byte{0x10, 0xac, 0x02}) {
		t.Fatalf("VarintField(2, 300) = %v, want [0x10 0xac 0x02]", got)
	}
}

func TestMetadataShape(t *testing.T) {
	m := Metadata("KEY", "sessABCD")
	// All six fields are length-delimited. Decode them by hand so the probe
	// test actually reads the wire rather than comparing to a snapshot.
	texts := []string{}
	off := 0
	for off < len(m) {
		tag, o := ReadVarint(m, off)
		off = o
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)
		if wireType != 2 {
			t.Fatalf("metadata should only contain length-delimited fields, saw wireType %d", wireType)
		}
		ln, o := ReadVarint(m, off)
		off = o
		if ln > uint64(len(m)-off) {
			t.Fatal("truncated metadata")
		}
		texts = append(texts, string(m[off:off+int(ln)]))
		off += int(ln)
		_ = fieldNum
	}
	if len(texts) != 6 {
		t.Fatalf("metadata has %d fields, want 6", len(texts))
	}
	if texts[0] != "KEY" || texts[5] != "en-US" {
		t.Fatalf("metadata order or content off: %v", texts)
	}
}

func TestChatMessageShapeWithAndWithoutToolCallID(t *testing.T) {
	m := ChatMessage("user", "hello", "")
	// Two fields: role, content.
	if got := DecodeStringField(m, 1); got != "user" {
		t.Fatalf("role = %q", got)
	}
	if got := DecodeStringField(m, 2); got != "hello" {
		t.Fatalf("content = %q", got)
	}
	// With tool call id, field 3 is present.
	m = ChatMessage("tool", "the output", "call_123")
	if got := DecodeStringField(m, 3); got != "call_123" {
		t.Fatalf("tool call id = %q", got)
	}
}

func TestGetChatMessageRequestShape(t *testing.T) {
	// Caller resolves the alias first (matching 9router-go's call site).
	wireModel := ResolveModelID("swe-1.6")
	body := GetChatMessageRequest("KEY", wireModel, [][]byte{ChatMessage("user", "hi", "")})
	// Walk: 1 metadata (bytes), 2 cascade_id (string), 3 model (nested), 4 message.
	cascade := DecodeStringField(body, 2)
	if len(cascade) != 32 { // RandHex(16) → 32 hex chars
		t.Fatalf("cascade_id len = %d, want 32", len(cascade))
	}
	// Field 3 is a nested message {1: model}; the alias the caller resolved
	// must reach the wire.
	if !bytes.Contains(body, []byte("swe-1-6")) {
		t.Fatalf("caller-resolved alias did not reach the wire: body[:64] = %q", Preview(string(body)))
	}
}

func TestResolveModelIDAliasesOnlyKnownEntries(t *testing.T) {
	if got := ResolveModelID("swe-1.6"); got != "swe-1-6" {
		t.Fatalf("alias for swe-1.6 = %q", got)
	}
	if got := ResolveModelID("claude-haiku-4.5"); got != "MODEL_PRIVATE_11" {
		t.Fatalf("alias for claude-haiku-4.5 = %q", got)
	}
	// Unknown id passes through unmodified — a claim-not-in-table model id
	// should NOT be silently coerced to something else.
	if got := ResolveModelID("brand-new-model"); got != "brand-new-model" {
		t.Fatalf("unknown id rewritten: %q", got)
	}
}

func TestGRPCWebFrameRoundTrip(t *testing.T) {
	payload := []byte{0x08, 0x01}
	frame := GRPCWebFrame(payload)
	if len(frame) != 5+len(payload) {
		t.Fatalf("frame len = %d, want %d", len(frame), 5+len(payload))
	}
	if frame[0] != 0x00 {
		t.Fatalf("flag = %d, want 0x00", frame[0])
	}
	// Length 2 as 4-byte BE.
	if frame[1] != 0 || frame[2] != 0 || frame[3] != 0 || frame[4] != 2 {
		t.Fatalf("BE length encoded wrong: %v", frame[1:5])
	}
	// Split back out.
	frames, err := SplitGRPCWebFrames(frame)
	if err != nil {
		t.Fatalf("split error: %v", err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], payload) {
		t.Fatalf("frame round-trip failed: %v", frames)
	}
}

func TestSplitGRPCWebFramesOnMultipleFrames(t *testing.T) {
	a := []byte{0x08, 0x01}
	b := []byte{0x12, 0x02, 'x', 'y'}
	var body []byte
	body = append(body, GRPCWebFrame(a)...)
	body = append(body, GRPCWebFrame(b)...)
	frames, err := SplitGRPCWebFrames(body)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if !bytes.Equal(frames[0], a) || !bytes.Equal(frames[1], b) {
		t.Fatalf("frames out of order or scrambled: %v", frames)
	}
}

func TestSplitGRPCWebFramesRejectsTruncatedValues(t *testing.T) {
	// A frame header says "10 bytes follow" but body has only 3.
	short := append([]byte{0x00, 0x00, 0x00, 0x00, 0x0a}, []byte{1, 2, 3}...)
	if _, err := SplitGRPCWebFrames(short); err == nil {
		t.Fatal("short body accepted, want a length-mismatch error")
	}
	// Header alone is truncated (no length bytes at all).
	if _, err := SplitGRPCWebFrames([]byte{0x00, 0x00}); err == nil {
		t.Fatal("truncated header accepted, want an error")
	}
}

func TestDecodeCompletionChunkVariants(t *testing.T) {
	// Content: field 1 wrapping {field 1 = text}.
	contentMsg := Msg(1, Str(1, "hello"))
	ch := DecodeCompletionChunk(contentMsg)
	if ch.Kind != "content" || ch.Text != "hello" {
		t.Fatalf("content: %+v", ch)
	}
	// Done: field 3 wrapping DoneChunk{field 1 = UsageStats{1: prompt, 2: completion}}
	// — the completion chunk carries a NESTED DoneChunk, which then carries a
	// NESTED UsageStats. Skipping the wrap (`Msg(3, usage)` direct) is the
	// exact shape the 9router-go decoder explicitly rejects.
	usage := bytes.Join([][]byte{VarintField(1, 12), VarintField(2, 34)}, nil)
	doneField := Msg(3, Msg(1, usage))
	ch = DecodeCompletionChunk(doneField)
	if ch.Kind != "done" || ch.PromptTokens != 12 || ch.CompletionTokens != 34 {
		t.Fatalf("done: %+v", ch)
	}
	// Error: field 4 wrapping {1: message}.
	errMsg := Msg(4, Str(1, "quota exhausted"))
	ch = DecodeCompletionChunk(errMsg)
	if ch.Kind != "error" || ch.Message != "quota exhausted" {
		t.Fatalf("error: %+v", ch)
	}
	// Empty / unknown messages must not panic or invent tokens.
	ch = DecodeCompletionChunk([]byte{})
	if ch.Kind != "unknown" {
		t.Fatalf("empty body should decode to unknown, got %+v", ch)
	}
}

func TestCollectContent(t *testing.T) {
	chunks := []Chunk{
		{Kind: "content", Text: "hi "},
		{Kind: "done", PromptTokens: 1, CompletionTokens: 2},
		{Kind: "content", Text: "there"},
		{Kind: "error", Message: "ignored"},
	}
	if got := CollectContent(chunks); got != "hi there" {
		t.Fatalf("CollectContent = %q, want %q", got, "hi there")
	}
}

func TestRandHexLengthAndUniqueness(t *testing.T) {
	a := RandHex(16)
	b := RandHex(16)
	if len(a) != 32 {
		t.Fatalf("len = %d, want 32", len(a))
	}
	if a == b {
		t.Fatalf("two RandHex(16) calls produced the same value %q", a)
	}
}

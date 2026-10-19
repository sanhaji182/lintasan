package cursor

import (
	"bytes"
	"strings"
	"testing"
)

// Each test pins one wire contract from the 9router JS source (v0.5.104,
// utils/cursorProtobuf.js) so a future field reorder or primitive change
// breaks exactly the case it breaks.

func TestEncodeVarintKnownShapes(t *testing.T) {
	cases := []struct {
		v    uint64
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{0x7f, []byte{0x7f}},
		{0x80, []byte{0x80, 0x01}},
		{0xdeadbeef, []byte{0xef, 0xfd, 0xb6, 0xf5, 0x0d}},
		// Max uint64: 10 bytes.
		{^uint64(0), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}},
	}
	for _, tc := range cases {
		got := EncodeVarint(tc.v)
		if !bytes.Equal(got, tc.want) {
			t.Errorf("EncodeVarint(%d) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

func TestEncodeVarintFieldRoundTrip(t *testing.T) {
	// Field 46 (UnifiedMode), wire type 0, value 2 (AGENT):
	// tag = (46<<3)|0 = 368 → varint 0xf0 0x02. Value 2.
	got := EncodeVarintField(46, 2)
	want := []byte{0xf0, 0x02, 0x02}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarintField(46, 2) = %v, want %v", got, want)
	}
	// Round-trip through the decoder.
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if len(fields) != 1 || fields[0].Number != 46 || fields[0].WireType != WireTypeVarint || fields[0].Varint != 2 {
		t.Fatalf("round-trip failed: %+v", fields)
	}
}

func TestEncodeTagShape(t *testing.T) {
	// Field 1, wire type 0 → tag 0x08 (single byte).
	if got := EncodeTag(1, WireTypeVarint); !bytes.Equal(got, []byte{0x08}) {
		t.Fatalf("EncodeTag(1, 0) = %v, want [0x08]", got)
	}
	// Field 1, wire type 2 → tag 0x0a.
	if got := EncodeTag(1, WireTypeLen); !bytes.Equal(got, []byte{0x0a}) {
		t.Fatalf("EncodeTag(1, 2) = %v, want [0x0a]", got)
	}
	// Field 54, wire type 2 → tag 0x1b2 (two-byte varint).
	got := EncodeTag(54, WireTypeLen)
	if !bytes.Equal(got, []byte{0xb2, 0x03}) {
		t.Fatalf("EncodeTag(54, 2) = %v, want [0xb2 0x03]", got)
	}
}

func TestEncodeLenFieldStringAndBytes(t *testing.T) {
	// Field 3 (Instruction), value "hello":
	// tag 0x1a, length 5, bytes "hello".
	got := EncodeStringField(3, "hello")
	want := []byte{0x1a, 0x05, 'h', 'e', 'l', 'l', 'o'}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeStringField(3, 'hello') = %v, want %v", got, want)
	}
	// Bytes payload — same shape.
	got = EncodeLenField(1, []byte{0xff, 0x00})
	want = []byte{0x0a, 0x02, 0xff, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeLenField(1, ...) = %v, want %v", got, want)
	}
	// Empty payload still carries a length 0.
	got = EncodeLenField(2, nil)
	want = []byte{0x12, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeLenField(2, nil) = %v, want %v", got, want)
	}
}

func TestEncodeFixed64FieldIsLittleEndianDouble(t *testing.T) {
	// 1.0 in IEEE 754 LE is 0x00 0x00 0x00 0x00 0x00 0x00 0xf0 0x3f.
	got := EncodeFixed64Field(4, 1.0)
	want := []byte{0x21, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeFixed64Field(4, 1.0) = %v, want %v", got, want)
	}
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if len(fields) != 1 || fields[0].Double != 1.0 {
		t.Fatalf("round-trip failed: %+v", fields)
	}
}

func TestEncodeFixed32FieldIsLittleEndianFloat(t *testing.T) {
	// 0.5 in IEEE 754 single LE is 0x00 0x00 0x00 0x3f.
	got := EncodeFixed32Field(6, 0.5)
	want := []byte{0x35, 0x00, 0x00, 0x00, 0x3f}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeFixed32Field(6, 0.5) = %v, want %v", got, want)
	}
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if len(fields) != 1 || fields[0].Double != float64(float32(0.5)) {
		t.Fatalf("round-trip failed: %+v", fields)
	}
}

func TestDecodeFieldsMultiField(t *testing.T) {
	// A two-field body — field 1 varint 1 (USER), field 13 string "id-1".
	buf := Concat(
		EncodeVarintField(1, RoleUser),
		EncodeStringField(13, "id-1"),
	)
	fields, err := DecodeFields(buf)
	if err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(fields))
	}
	if fields[0].Varint != RoleUser {
		t.Fatalf("field 1 varint = %d, want %d", fields[0].Varint, RoleUser)
	}
	if string(fields[1].Bytes) != "id-1" {
		t.Fatalf("field 13 bytes = %q, want 'id-1'", fields[1].Bytes)
	}
}

func TestDecodeFieldsRejectsTruncatedValueLength(t *testing.T) {
	// Field 1, wire type 2, claims 10 bytes but body has only 3.
	bad := []byte{0x0a, 0x0a, 1, 2, 3}
	if _, err := DecodeFields(bad); err == nil {
		t.Fatal("truncated length-delimited field accepted")
	}
	// Field 1, wire type 0 with NO value bytes.
	if _, err := DecodeFields([]byte{0x08}); err == nil {
		t.Fatal("varint without value accepted")
	}
}

func TestDecodeFieldsRejectsUnknownWireType(t *testing.T) {
	// Wire type 3 (START_GROUP) is not handled by Cursor's wire; the JS
	// decoder rejects it.
	if _, err := DecodeFields([]byte{0x0b, 0x00}); err == nil {
		t.Fatal("unknown wire type 3 accepted")
	}
}

func TestFormatAndParseToolNameRoundTrip(t *testing.T) {
	name, id := "bash", "1234" // compact id (no internal underscore)
	formatted := FormatToolName(name, id)
	if formatted != "bash_1234" {
		t.Fatalf("formatted = %q", formatted)
	}
	gotName, gotID := ParseToolName(formatted)
	if gotName != name || gotID != id {
		t.Fatalf("round-trip: name=%q id=%q, want %q/%q", gotName, gotID, name, id)
	}
	// A tool name with underscores in it still round-trips via
	// LastIndexByte — matching the JS regex's LAST-separator semantics.
	name2, id2 := ParseToolName("my_tool_abc123")
	if name2 != "my_tool" || id2 != "abc123" {
		t.Fatalf("underscored name: %q/%q", name2, id2)
	}
	// The 'call_1234' shape from Cursor's tool IDs also works — the
	// LAST underscore is the separator, so "call_1234" collapses to
	// name "bash_call" / id "1234" when named 'bash'. Document that the
	// caller should use ONLY underscore-free ids (the 9router executor
	// does this; it does not preserve nested underscores).
	// A tool with no call id passes through unformatted.
	if got := FormatToolName(name, ""); got != name {
		t.Fatalf("empty id must not add a suffix: %q", got)
	}
	// The parser's early-return shapes.
	for _, bad := range []string{"", "_", "bash_", "_x"} {
		gotName, gotID := ParseToolName(bad)
		if gotName != "" || gotID != "" {
			t.Fatalf("ParseToolName(%q) = (%q, %q), want empty/empty", bad, gotName, gotID)
		}
	}
	if !strings.Contains(formatted, "_") {
		t.Fatal("test sanity: the formatted form should carry the underscore separator")
	}
}

func TestGRPCWebFrameRoundTrip(t *testing.T) {
	payload := []byte{0x08, 0x01}
	frame := GRPCWebFrame(payload)
	if len(frame) != 5+len(payload) {
		t.Fatalf("frame len = %d, want %d", len(frame), 5+len(payload))
	}
	if frame[0] != 0x00 {
		t.Fatalf("flag = %d, want 0x00 (uncompressed)", frame[0])
	}
	// Big-endian 32-bit length of 2.
	if frame[1] != 0 || frame[2] != 0 || frame[3] != 0 || frame[4] != 2 {
		t.Fatalf("BE length encoded wrong: %v", frame[1:5])
	}
	frames, err := SplitGRPCWebFrames(frame)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(frames) != 1 || !bytes.Equal(frames[0], payload) {
		t.Fatalf("frame round-trip failed: %v", frames)
	}
}

func TestSplitGRPCWebFramesMultiFrameAndReject(t *testing.T) {
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
	// Truncated header.
	if _, err := SplitGRPCWebFrames([]byte{0x00, 0x00}); err == nil {
		t.Fatal("truncated header accepted")
	}
	// Frame claims more bytes than the body has.
	short := append([]byte{0x00, 0x00, 0x00, 0x00, 0x0a}, []byte{1, 2, 3}...)
	if _, err := SplitGRPCWebFrames(short); err == nil {
		t.Fatal("short body accepted")
	}
}

func TestConcatOrderIsPreserved(t *testing.T) {
	a := []byte{1, 2}
	b := []byte{3}
	c := []byte{4, 5, 6}
	got := Concat(a, b, c)
	want := []byte{1, 2, 3, 4, 5, 6}
	if !bytes.Equal(got, want) {
		t.Fatalf("Concat = %v, want %v", got, want)
	}
}

package kiro

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// Each test pins one wire-frame contract. Because CRC is over the ENTIRE
// preceding prefix (per the AWS spec, matching the JS emulator), ANY
// byte-order / length bug shows up as an invalid CRC rather than a
// silent mis-parse — a property the tests exploit.

func TestCRC32MatchesKnownVectors(t *testing.T) {
	// "123456789" → CRC32 (checksum standard, Morton/Williams) = 0xCBF43926.
	// The document is the classic vector; the AWS EventStream prelude uses
	// this algorithm实名.
	if got := CRC32([]byte("123456789")); got != 0xCBF43926 {
		t.Fatalf("CRC32('123456789') = %#x, want 0xCBF43926", got)
	}
	// Empty buffer → 0 (the CRC of nothing).
	if got := CRC32(nil); got != 0 {
		t.Fatalf("CRC32(empty) = %#x", got)
	}
}

func TestEventFrameRoundTrip(t *testing.T) {
	headers := map[string][]byte{
		"message-type": []byte("event"),
		"event-type":   []byte("chunk"),
	}
	payload := []byte(`{"text":"hi there"}`)
	frame := EncodeEventFrame(headers, payload)
	parsed, err := ParseEventFrame(frame)
	if err != nil {
		t.Fatalf("ParseEventFrame: %v", err)
	}
	if parsed.TotalLength != len(frame) {
		t.Fatalf("total length mismatch: %d vs %d", parsed.TotalLength, len(frame))
	}
	// Look up the two string headers by name — the test verifies the wire
	// had both; the assertion is simple enough to write per key.
	if got, ok := parsed.Headers["message-type"].(string); !ok || got != "event" {
		t.Fatalf("header message-type = %v, want string 'event'", parsed.Headers["message-type"])
	}
	if got, ok := parsed.Headers["event-type"].(string); !ok || got != "chunk" {
		t.Fatalf("header event-type = %v, want string 'chunk'", parsed.Headers["event-type"])
	}
	// Payload is byte-identical.
	if !bytes.Equal(parsed.Payload, payload) {
		t.Fatalf("payload corrupted: %q vs %q", parsed.Payload, payload)
	}
}

func TestEventFrameRejectsTruncatedPrelude(t *testing.T) {
	// Fewer than 16 bytes = insufficient for a prelude + message CRC.
	if _, err := ParseEventFrame(make([]byte, 15)); err == nil {
		t.Fatal("truncated prelude accepted")
	}
	if _, err := ParseEventFrame([]byte{}); err == nil {
		t.Fatal("empty body accepted")
	}
}

func TestEventFrameRejectsPreludeCRCMismatch(t *testing.T) {
	// Encode a valid frame, then flip one bit inside the preludeCrc field.
	headers := map[string][]byte{"x": []byte("y")}
	frame := EncodeEventFrame(headers, nil)
	frame[8] ^= 1 // bit-flip the prelude CRC
	if _, err := ParseEventFrame(frame); err == nil || !strings.Contains(err.Error(), "prelude CRC") {
		t.Fatalf("expected prelude CRC mismatch, got %v", err)
	}
}

func TestEventFrameRejectsMessageCRCMismatch(t *testing.T) {
	headers := map[string][]byte{"x": []byte("y")}
	frame := EncodeEventFrame(headers, []byte(`{"p":1}`))
	// Flip one bit at the very last byte (inside the message-CRC tail).
	frame[len(frame)-1] ^= 1
	if _, err := ParseEventFrame(frame); err == nil || !strings.Contains(err.Error(), "message CRC") {
		t.Fatalf("expected message CRC mismatch, got %v", err)
	}
}

func TestEventFrameRejectsLyingTotalLength(t *testing.T) {
	// totalLength says 16, but the buffer has 40 bytes. Caller-supplied
	// data must not out-stretch the wire it claims.
	fake := make([]byte, 40)
	binary.BigEndian.PutUint32(fake[0:4], 16) // totalLength
	headersLength := 0
	binary.BigEndian.PutUint32(fake[4:8], uint32(headersLength))
	if _, err := ParseEventFrame(fake); err == nil {
		t.Fatal("lying total length accepted")
	}
}

func TestEventFrameRejectsLyingHeadersLength(t *testing.T) {
	// headersLength claims 100KB, buffer only has 40 bytes.
	fake := make([]byte, 40)
	binary.BigEndian.PutUint32(fake[0:4], uint32(len(fake)))
	binary.BigEndian.PutUint32(fake[4:8], 100<<10)
	// Fix the prelude CRC so the test isolates the length rejection.
	binary.BigEndian.PutUint32(fake[8:12], CRC32(fake[0:8]))
	if _, err := ParseEventFrame(fake); err == nil || !strings.Contains(err.Error(), "headers length") {
		t.Fatalf("expected headers-length rejection, got %v", err)
	}
}

func TestEventFrameRejectsDuplicateHeaderNames(t *testing.T) {
	// Hand-roll a frame whose headers section contains "a" TWICE — 9router
	// explicitly rejects that (it produces a struct that cannot be mapped
	// 1:1 into Go's map). Each entry is: name_len=1, name 'a', type 7,
	// value_len=0x0001, value byte.
	dupe := append(
		[]byte{1, 'a', 7, 0, 1, 'b'},
		[]byte{1, 'a', 7, 0, 1, 'c'}...,
	)
	total := 12 + len(dupe) + 4
	buf := make([]byte, 0, total)
	l := make([]byte, 4)
	binary.BigEndian.PutUint32(l, uint32(total))
	buf = append(buf, l...)
	l = make([]byte, 4)
	binary.BigEndian.PutUint32(l, uint32(len(dupe)))
	buf = append(buf, l...)
	binary.BigEndian.PutUint32(l, CRC32(buf[0:8]))
	buf = append(buf, l...)
	buf = append(buf, dupe...)
	binary.BigEndian.PutUint32(l, CRC32(buf))
	buf = append(buf, l...)
	if _, err := ParseEventFrame(buf); err == nil || !strings.Contains(err.Error(), "duplicate header") {
		t.Fatalf("expected duplicate-header rejection, got %v", err)
	}
}

func TestEventFrameRejectsUnknownHeaderType(t *testing.T) {
	// Type 0xaa is not in the AWS EventStream header type table (0..9).
	unknown := []byte{1, 'x', 0xaa, 0, 0}
	total := 12 + len(unknown) + 4
	buf := make([]byte, 0, total)
	l := make([]byte, 4)
	binary.BigEndian.PutUint32(l, uint32(total))
	buf = append(buf, l...)
	l = make([]byte, 4)
	binary.BigEndian.PutUint32(l, uint32(len(unknown)))
	buf = append(buf, l...)
	binary.BigEndian.PutUint32(l, CRC32(buf[0:8]))
	buf = append(buf, l...)
	buf = append(buf, unknown...)
	binary.BigEndian.PutUint32(l, CRC32(buf))
	buf = append(buf, l...)
	if _, err := ParseEventFrame(buf); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("expected unknown-type rejection, got %v", err)
	}
}

func TestPayloadJSONOnEmptyOrBlank(t *testing.T) {
	// Empty / whitespace payload → nil map without an error (control frame).
	f := &Frame{Payload: nil}
	m, err := f.PayloadJSON()
	if err != nil || m != nil {
		t.Fatalf("expected nil,nil, got %v,%v", m, err)
	}
	f = &Frame{Payload: []byte("   ")}
	m, err = f.PayloadJSON()
	if err != nil || m != nil {
		t.Fatalf("blank payload: got %v,%v", m, err)
	}
	// JSON payload decodes.
	f = &Frame{Payload: []byte(`{"k":"v"}`)}
	m, err = f.PayloadJSON()
	if err != nil || m == nil || m["k"] != "v" {
		t.Fatalf("json payload: got %v,%v", m, err)
	}
	// Non-JSON fails with a clear message.
	f = &Frame{Payload: []byte(`{bad`)}
	_, err = f.PayloadJSON()
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("non-JSON expected clear error, got %v", err)
	}
}

func TestEncodeEventFrameDeterministicOrder(t *testing.T) {
	// Two runs of EncodeEventFrame with the same input produce the same
	// bytes (this is what makes a fixture / wire snapshot re-runnable).
	headers := map[string][]byte{"z": []byte("1"), "a": []byte("2"), "m": []byte("3")}
	first := EncodeEventFrame(headers, []byte(`{"n":1}`))
	second := EncodeEventFrame(headers, []byte(`{"n":1}`))
	if !bytes.Equal(first, second) {
		t.Fatal("EncodeEventFrame is not deterministic")
	}
}

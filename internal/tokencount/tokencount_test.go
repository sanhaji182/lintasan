package tokencount

import "testing"

func TestCount_EmptyIsZero(t *testing.T) {
	if got := Count(""); got != 0 {
		t.Fatalf("Count(\"\") = %d, want 0", got)
	}
}

func TestCount_NonEmptyIsPositive(t *testing.T) {
	got := Count("The quick brown fox jumps over the lazy dog.")
	if got <= 0 {
		t.Fatalf("Count returned %d, want > 0", got)
	}
	// A ~44-char sentence should be well under 44 tokens and over 1.
	if got > 44 {
		t.Fatalf("Count = %d, implausibly high for a short sentence", got)
	}
}

func TestCount_LongerTextMoreTokens(t *testing.T) {
	short := Count("hello world")
	long := Count("hello world " + "the quick brown fox jumps over the lazy dog repeatedly")
	if long <= short {
		t.Fatalf("longer text should have more tokens: short=%d long=%d", short, long)
	}
}

func TestCountMessages_StringContent(t *testing.T) {
	msgs := []any{
		map[string]any{"role": "system", "content": "You are a helpful assistant."},
		map[string]any{"role": "user", "content": "Explain goroutines in Go."},
	}
	got := CountMessages(msgs)
	if got <= 0 {
		t.Fatalf("CountMessages = %d, want > 0", got)
	}
}

func TestCountMessages_StructuredContent(t *testing.T) {
	msgs := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "describe this"},
				map[string]any{"type": "text", "text": "and this too"},
			},
		},
	}
	got := CountMessages(msgs)
	if got <= 0 {
		t.Fatalf("CountMessages with structured content = %d, want > 0", got)
	}
}

func TestCountMessages_IgnoresNonMessages(t *testing.T) {
	msgs := []any{"not a map", 42, nil}
	if got := CountMessages(msgs); got != 0 {
		t.Fatalf("CountMessages over junk = %d, want 0", got)
	}
}

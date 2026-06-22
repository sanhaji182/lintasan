package optimizer

import (
	"testing"
)

func TestOptimizePrompt_RemovesVerbosePhrases(t *testing.T) {
	input := "I would like you to help me. Can you please write code?"
	got := OptimizePrompt(input, false)
	if contains(got, "I would like you to") {
		t.Errorf("expected 'I would like you to' to be removed, got: %s", got)
	}
	if contains(got, "Can you please") {
		t.Errorf("expected 'Can you please' to be removed, got: %s", got)
	}
	if !contains(got, "help me") {
		t.Errorf("expected 'help me' to remain, got: %s", got)
	}
}

func TestOptimizePrompt_VerboseReplacements(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Note: regex patterns have trailing space, so input must have trailing space too
		{"Due to the fact that X is true", "Because X is true"},
		{"At this point in time we need X", "Now we need X"},
		{"In the event that it fails", "If it fails"},
		{"For the purpose of testing", "For testing"},
		{"With regard to your question", "Regarding your question"},
		{"In addition to that we also need", "Also we also need"},
		{"in order to compile", "To compile"},
	}

	for _, tt := range tests {
		got := OptimizePrompt(tt.input, false)
		if got != tt.expected {
			t.Errorf("OptimizePrompt(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestOptimizePrompt_SystemExtraFiltering(t *testing.T) {
	input := "very really quite important system prompt"
	gotSystem := OptimizePrompt(input, true)
	gotNonSystem := OptimizePrompt(input, false)

	// System mode should strip filler words like 'very', 'really', 'quite'
	if contains(gotSystem, "very") {
		t.Errorf("system mode: expected 'very' removed, got: %s", gotSystem)
	}
	// Non-system mode should NOT strip filler words
	if !contains(gotNonSystem, "very") {
		t.Errorf("non-system mode: expected 'very' preserved, got: %s", gotNonSystem)
	}
}

func TestOptimizePrompt_CollapsesWhitespace(t *testing.T) {
	input := "hello    world\n\n\n\nfoo"
	got := OptimizePrompt(input, false)
	if contains(got, "    ") {
		t.Errorf("expected multiple spaces collapsed, got: %s", got)
	}
	if contains(got, "\n\n\n") {
		t.Errorf("expected multiple newlines collapsed, got: %s", got)
	}
}

func TestOptimizeMessages_Deduplicates(t *testing.T) {
	messages := []any{
		map[string]any{"role": "user", "content": "Hello world"},
		map[string]any{"role": "user", "content": "Hello world"}, // duplicate
		map[string]any{"role": "assistant", "content": "Response A"},
		map[string]any{"role": "assistant", "content": ""},
	}

	result, saved := OptimizeMessages(messages)
	if saved <= 0 {
		t.Errorf("expected saved tokens > 0 for duplicate message, got %d", saved)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 messages after dedup, got %d", len(result))
	}
}

func TestOptimizeMessages_KeepsSystemMessages(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "You are a helpful assistant"},
		map[string]any{"role": "user", "content": "Hello"},
	}

	result, saved := OptimizeMessages(messages)
	if len(result) != 2 {
		t.Errorf("expected both messages preserved, got %d", len(result))
	}
	if saved != 0 {
		t.Errorf("expected 0 saved for distinct messages, got %d", saved)
	}
}

func TestOptimizeMessages_NonMapPassthrough(t *testing.T) {
	messages := []any{
		"string message",
		42,
	}

	result, _ := OptimizeMessages(messages)
	if len(result) != 2 {
		t.Errorf("expected 2 non-map messages preserved, got %d", len(result))
	}
}

func TestOptimizeMessages_EmptyMessages(t *testing.T) {
	result, saved := OptimizeMessages([]any{})
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d", len(result))
	}
	if saved != 0 {
		t.Errorf("expected 0 saved for empty input, got %d", saved)
	}
}

func TestOptimizeMessages_ContentArrayPassthrough(t *testing.T) {
	// Multimodal messages have content as array, not string
	messages := []any{
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,abc"}},
			},
		},
	}

	result, _ := OptimizeMessages(messages)
	if len(result) != 1 {
		t.Errorf("expected 1 message preserved, got %d", len(result))
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

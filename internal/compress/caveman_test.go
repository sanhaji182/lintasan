package compress

import (
	"testing"
)

func TestInjectCavemanStandard(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "Hello"},
	}
	result := InjectCaveman(msgs, "standard")
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(result))
	}
	if result[0]["role"] != "system" {
		t.Error("first message should be system")
	}
	content, ok := result[0]["content"].(string)
	if !ok || content == "" {
		t.Error("system message should have content")
	}
	if !contains(content, "terse") && !contains(content, "short") {
		t.Error("should contain caveman instructions")
	}
}

func TestInjectCavemanLite(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "Hello"},
	}
	result := InjectCaveman(msgs, "lite")
	content := result[0]["content"].(string)
	if !contains(content, "concise") {
		t.Error("lite mode should mention concise")
	}
}

func TestInjectCavemanAggressive(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "Hello"},
	}
	result := InjectCaveman(msgs, "aggressive")
	content := result[0]["content"].(string)
	if !contains(content, "Caveman") {
		t.Error("aggressive mode should mention Caveman")
	}
}

func TestInjectCavemanOff(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "Hello"},
	}
	result := InjectCaveman(msgs, "off")
	if len(result) != 1 {
		t.Error("off mode should not add system message")
	}
}

func TestInjectCavemanWithExistingSystem(t *testing.T) {
	msgs := []map[string]any{
		{"role": "system", "content": "You are helpful."},
		{"role": "user", "content": "Hello"},
	}
	result := InjectCaveman(msgs, "standard")
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	content := result[0]["content"].(string)
	if !contains(content, "You are helpful") {
		t.Error("should preserve existing system message")
	}
	if !contains(content, "terse") {
		t.Error("should append caveman instructions")
	}
}

func TestInjectCavemanPrompt(t *testing.T) {
	if InjectCavemanPrompt("lite") == "" {
		t.Error("lite prompt should not be empty")
	}
	if InjectCavemanPrompt("standard") == "" {
		t.Error("standard prompt should not be empty")
	}
	if InjectCavemanPrompt("aggressive") == "" {
		t.Error("aggressive prompt should not be empty")
	}
}

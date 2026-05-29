package translator

import (
	"testing"
)

func TestDetectFormatOpenAI(t *testing.T) {
	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	format := DetectFormat(body)
	if format != FormatOpenAI {
		t.Errorf("expected OpenAI, got %s", format)
	}
}

func TestDetectFormatAnthropic(t *testing.T) {
	body := []byte(`{"model":"claude-3","system":"You are helpful","messages":[{"role":"user","content":"hello"}],"max_tokens":100}`)
	format := DetectFormat(body)
	if format != FormatAnthropic {
		t.Errorf("expected Anthropic, got %s", format)
	}
}

func TestDetectFormatGemini(t *testing.T) {
	body := []byte(`{"contents":[{"parts":[{"text":"hello"}]}]}`)
	format := DetectFormat(body)
	if format != FormatGemini {
		t.Errorf("expected Gemini, got %s", format)
	}
}

func TestTranslateOpenAIToAnthropic(t *testing.T) {
	input := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	result, err := Translate(input, FormatOpenAI, FormatAnthropic, false)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if len(result) == 0 {
		t.Error("empty result")
	}
}

func TestTranslateAnthropicToOpenAI(t *testing.T) {
	input := []byte(`{"model":"claude-3","messages":[{"role":"user","content":"hello"}],"max_tokens":100}`)
	result, err := Translate(input, FormatAnthropic, FormatOpenAI, false)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if len(result) == 0 {
		t.Error("empty result")
	}
}

func TestTranslateOpenAIToGemini(t *testing.T) {
	input := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	result, err := Translate(input, FormatOpenAI, FormatGemini, false)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if len(result) == 0 {
		t.Error("empty result")
	}
}

func TestTranslateSameFormat(t *testing.T) {
	input := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	result, err := Translate(input, FormatOpenAI, FormatOpenAI, false)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if string(result) != string(input) {
		t.Error("same format should return identical input")
	}
}

func TestTranslateInvalidJSON(t *testing.T) {
	input := []byte(`not json`)
	_, err := Translate(input, FormatOpenAI, FormatAnthropic, false)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDetectStreamFormat(t *testing.T) {
	tests := []struct {
		line   string
		format Format
	}{
		{`data: {"choices":[{"delta":{"content":"hi"}}]}`, FormatOpenAI},
		{`data: {"type":"content_block_delta","delta":{"text":"hi"}}`, FormatAnthropic},
	}

	for _, tt := range tests {
		format := DetectStreamFormat(tt.line)
		if format != tt.format {
			t.Errorf("line %s: expected %s, got %s", tt.line, tt.format, format)
		}
	}
}

func TestTranslateToolDefinitions(t *testing.T) {
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get weather",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	result := TranslateToolDefinitions(tools, FormatOpenAI, FormatAnthropic)
	if len(result) == 0 {
		t.Error("expected translated tools")
	}
}

func TestTranslateWithSystemMessage(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4",
		"system": "You are helpful",
		"messages": [{"role": "user", "content": "hello"}]
	}`)
	result, err := Translate(input, FormatOpenAI, FormatAnthropic, false)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if len(result) == 0 {
		t.Error("empty result")
	}
}

func TestTranslateStreaming(t *testing.T) {
	input := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	result, err := Translate(input, FormatOpenAI, FormatAnthropic, false)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	if len(result) == 0 {
		t.Error("empty result")
	}
}

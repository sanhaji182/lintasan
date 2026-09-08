package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateCCAlphaToOpenAI_Success(t *testing.T) {
	raw := []byte(`{"type":"start"}
{"type":"reasoning-delta","text":"thinking about life"}
{"type":"text-delta","text":"hello world"}
{"type":"finish","finishReason":"stop"}
`)
	out := translateCCAlphaToOpenAI(raw)
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("failed to unmarshal translated response: %v", err)
	}

	choices, ok := resp["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatalf("missing choices in response")
	}
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)

	if msg["content"] != "hello world" {
		t.Errorf("expected content 'hello world', got '%v'", msg["content"])
	}
	if msg["reasoning_content"] != "thinking about life" {
		t.Errorf("expected reasoning_content 'thinking about life', got '%v'", msg["reasoning_content"])
	}
	if choice["finish_reason"] != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%v'", choice["finish_reason"])
	}
}

func TestTranslateCCAlphaToOpenAI_ErrorResponse(t *testing.T) {
	raw := []byte(`{"success":false,"error":{"code":"FORBIDDEN","status":403,"message":"Model/provider not recognized"}}`)
	out := translateCCAlphaToOpenAI(raw)
	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("failed to unmarshal translated error: %v", err)
	}

	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'error' field in translated output, got %s", string(out))
	}
	if errObj["message"] != "Model/provider not recognized" {
		t.Errorf("expected message 'Model/provider not recognized', got '%v'", errObj["message"])
	}
	if errObj["code"] != "FORBIDDEN" {
		t.Errorf("expected code 'FORBIDDEN', got '%v'", errObj["code"])
	}
}

func TestPipeCCAlphaStreamToOpenAI(t *testing.T) {
	p := &ProxyHandler{}
	rawStream := []byte(`{"type":"start"}
{"type":"reasoning-delta","text":"Let me think"}
{"type":"text-delta","text":"Hi"}
{"type":"text-delta","text":" there!"}
{"type":"finish","finishReason":"stop"}
`)
	rec := httptest.NewRecorder()
	buf, tokens := p.pipeCCAlphaStreamToOpenAI(bytes.NewReader(rawStream), rec, nil, "deepseek/deepseek-v4-pro")

	if tokens == 0 {
		t.Errorf("expected tokens > 0, got %d", tokens)
	}
	if string(buf) != "Hi there!" {
		t.Errorf("expected stream buffer 'Hi there!', got '%s'", string(buf))
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data: [DONE]\n\n") {
		t.Errorf("expected stream to end with 'data: [DONE]\\n\\n', got %s", body)
	}
	if !strings.Contains(body, "chat.completion.chunk") {
		t.Errorf("expected stream chunks with 'chat.completion.chunk', got %s", body)
	}
	if !strings.Contains(body, "Let me think") {
		t.Errorf("expected reasoning chunk with 'Let me think', got %s", body)
	}
}

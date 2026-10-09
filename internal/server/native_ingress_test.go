package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/translator"
)

func TestNativeIngressGatesDefaultOffAndFlipLive(t *testing.T) {
	p := newTestProxyHandler(t)
	database := p.db
	if p.claudeMessagesAPIEnabled() || p.geminiNativeAPIEnabled() {
		t.Fatal("native ingress gates must default false")
	}
	if err := database.SetSetting(claudeMessagesAPISettingKey, "true"); err != nil {
		t.Fatal(err)
	}
	if !p.claudeMessagesAPIEnabled() {
		t.Fatal("claude gate did not observe DB flip without restart")
	}
	if err := database.SetSetting(geminiNativeAPISettingKey, "true"); err != nil {
		t.Fatal(err)
	}
	if !p.geminiNativeAPIEnabled() {
		t.Fatal("gemini gate did not observe DB flip without restart")
	}
	if err := database.SetSetting(claudeMessagesAPISettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	if p.claudeMessagesAPIEnabled() {
		t.Fatal("claude gate did not observe DB disable without restart")
	}
}

func TestNativeIngressFlagOffReturns404(t *testing.T) {
	p := newTestProxyHandler(t)
	cases := []struct {
		name string
		path string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"claude", "/v1/messages", p.HandleClaudeMessages},
		{"gemini", "/v1beta/models/gemini-test:generateContent", p.HandleGeminiGenerateContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`))
			w := httptest.NewRecorder()
			tc.fn(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestNativeBufferedResponseShapes(t *testing.T) {
	chat := []byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)

	anthropic, err := translator.OpenAIResponseToAnthropic(chat)
	if err != nil {
		t.Fatal(err)
	}
	var a map[string]any
	if err := json.Unmarshal(anthropic, &a); err != nil {
		t.Fatal(err)
	}
	if a["type"] != "message" || a["stop_reason"] != "end_turn" {
		t.Fatalf("unexpected Anthropic response: %s", anthropic)
	}
	content, _ := a["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("Anthropic content=%v", a["content"])
	}

	gemini, err := translator.OpenAIResponseToGemini(chat)
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]any
	if err := json.Unmarshal(gemini, &g); err != nil {
		t.Fatal(err)
	}
	candidates, _ := g["candidates"].([]any)
	if len(candidates) != 1 {
		t.Fatalf("Gemini candidates=%v", g["candidates"])
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (f *flushRecorder) Flush() { f.flushes++ }

func TestClaudeNativeStreamIsIncrementalAndTerminal(t *testing.T) {
	real := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	a := newNativeStreamAdapter(real, translator.FormatAnthropic, "test-model")

	first := `data: {"choices":[{"delta":{"content":"hel"},"finish_reason":null}]}` + "\n\n"
	second := `data: {"choices":[{"delta":{"content":"lo"},"finish_reason":null}]}` + "\n\n"
	if _, err := a.Write([]byte(first)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(real.Body.String(), `"text":"hel"`) {
		t.Fatalf("first chunk not emitted immediately: %s", real.Body.String())
	}
	before := real.Body.Len()
	if _, err := a.Write([]byte(second + "data: [DONE]\n\n")); err != nil {
		t.Fatal(err)
	}
	if real.Body.Len() <= before || !strings.Contains(real.Body.String(), `"text":"lo"`) {
		t.Fatalf("second chunk not emitted incrementally: %s", real.Body.String())
	}
	a.finalize()
	body := real.Body.String()
	if !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: content_block_start") {
		t.Fatalf("missing Anthropic stream start events: %s", body)
	}
	if !strings.Contains(body, "event: content_block_stop") || !strings.Contains(body, "event: message_stop") {
		t.Fatalf("missing Anthropic terminal events: %s", body)
	}
	if real.flushes == 0 {
		t.Fatal("stream never flushed")
	}
}

func TestGeminiNativeStreamShapeAndTerminal(t *testing.T) {
	real := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	a := newNativeStreamAdapter(real, translator.FormatGemini, "gemini-test")
	input := `data: {"choices":[{"delta":{"content":"hello"},"finish_reason":null}]}` + "\n\n" + "data: [DONE]\n\n"
	if _, err := a.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	a.finalize()
	body := real.Body.String()
	if !strings.Contains(body, `"candidates"`) || !strings.Contains(body, `"finishReason":"STOP"`) {
		t.Fatalf("unexpected Gemini stream: %s", body)
	}
	// Every SSE data payload must be valid JSON.
	s := bufio.NewScanner(strings.NewReader(body))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &v); err != nil {
			t.Fatalf("invalid Gemini SSE JSON %q: %v", line, err)
		}
	}
}

func TestParseGeminiNativePath(t *testing.T) {
	m, stream, ok := parseGeminiNativePath("/v1beta/models/gemini-2.5-pro:generateContent")
	if !ok || stream || m != "gemini-2.5-pro" {
		t.Fatalf("non-stream: model=%q stream=%v ok=%v", m, stream, ok)
	}
	m, stream, ok = parseGeminiNativePath("/v1beta/models/gemini-2.5-pro:streamGenerateContent")
	if !ok || !stream || m != "gemini-2.5-pro" {
		t.Fatalf("stream: model=%q stream=%v ok=%v", m, stream, ok)
	}
	if _, _, ok := parseGeminiNativePath("/v1beta/models/a/b:generateContent"); ok {
		t.Fatal("nested model path accepted")
	}
}

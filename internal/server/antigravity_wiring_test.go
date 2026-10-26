package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsAntigravityNative(t *testing.T) {
	p := &ProxyHandler{}
	cases := []struct {
		name string
		conn *Connection
		want bool
	}{
		{"oauth provider", &Connection{OAuthProvider: "antigravity"}, true},
		{"native format", &Connection{Format: "Antigravity"}, true},
		{"ordinary openai", &Connection{Format: "openai"}, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		if got := p.isAntigravityNative(tc.conn); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAntigravityUpstreamEnvelope(t *testing.T) {
	type captured struct {
		path, query string
		header      http.Header
		body        []byte
	}
	var got captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.query = r.URL.RawQuery
		got.header = r.Header.Clone()
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &ProxyHandler{}
	conn := &Connection{ID: "ag-1", BaseURL: srv.URL, APIKey: "oauth-token", OAuthProvider: "antigravity"}
	body := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"system","content":"be terse"},{"role":"user","content":"hi"}],"max_tokens":16,"stream":true}`)

	req, err := p.antigravityUpstream(context.Background(), conn, body)
	if err != nil {
		t.Fatalf("antigravityUpstream: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	resp.Body.Close()

	if got.path != "/v1internal:streamGenerateContent" || !strings.Contains(got.query, "alt=sse") {
		t.Fatalf("path=%q query=%q", got.path, got.query)
	}
	if auth := got.header.Get("Authorization"); auth != "Bearer oauth-token" {
		t.Errorf("Authorization = %q", auth)
	}
	if ua := got.header.Get("User-Agent"); ua != "antigravity/ide/2.22.0 darwin/arm64" {
		t.Errorf("User-Agent = %q", ua)
	}

	var envelope struct {
		Project   string         `json:"project"`
		Model     string         `json:"model"`
		UserAgent string         `json:"userAgent"`
		RequestID string         `json:"requestId"`
		Request   map[string]any `json:"request"`
	}
	if err := json.Unmarshal(got.body, &envelope); err != nil {
		t.Fatalf("envelope JSON: %v", err)
	}
	if envelope.Project == "" || envelope.Model != "gemini-3.8-flash-medium" || envelope.UserAgent != "antigravity" {
		t.Fatalf("envelope identity wrong: %+v", envelope)
	}
	if !strings.HasPrefix(envelope.RequestID, "agent/agent/") {
		t.Errorf("requestId = %q", envelope.RequestID)
	}
	if _, exists := envelope.Request["model"]; exists {
		t.Error("inner Gemini request must not carry model; the envelope does")
	}
	contents, _ := envelope.Request["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("contents = %#v", envelope.Request["contents"])
	}
	system, _ := envelope.Request["systemInstruction"].(map[string]any)
	if system == nil {
		t.Fatal("system message must become systemInstruction")
	}
}

func TestAntigravityNonStreamResponse(t *testing.T) {
	raw := []byte(`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7},"modelVersion":"gemini-3.8-flash-n"}}`)
	p := &ProxyHandler{}
	out, prompt, completion, err := p.antigravityNonStreamResponse(raw, "gemini-3.8-flash")
	if err != nil {
		t.Fatalf("non-stream: %v", err)
	}
	if prompt != 5 || completion != 2 {
		t.Errorf("usage = %d/%d, want 5/2", prompt, completion)
	}
	var resp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Created int64  `json:"created"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Model != "gemini-3.8-flash" || !strings.HasPrefix(resp.ID, "chatcmpl-ant-") || resp.Created == 0 {
		t.Errorf("response identity wrong: %+v", resp)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "Hello" || resp.Choices[0].FinishReason != "stop" {
		t.Errorf("choice = %+v", resp.Choices)
	}
}

func TestAntigravityStreamToOpenAI(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":""}]}}]}}`,
		``,
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Hel"}]}}]}}`,
		``,
		`data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"lo"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}}`,
		``,
	}, "\n")

	p := &ProxyHandler{}
	rec := httptest.NewRecorder()
	commit := &streamCommit{}
	collected, completion, err := p.streamAntigravityToOpenAI(io.NopCloser(strings.NewReader(sse)), rec, rec, "gemini-3.8-flash", commit)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if string(collected) != "Hello" || completion != 2 {
		t.Errorf("collected=%q completion=%d", collected, completion)
	}
	out := rec.Body.String()
	for _, want := range []string{`"role":"assistant"`, `"content":"Hel"`, `"content":"lo"`, `"finish_reason":"stop"`, `"prompt_tokens":5`, `"completion_tokens":2`, "data: [DONE]"} {
		if !strings.Contains(out, want) {
			t.Errorf("SSE missing %q:\n%s", want, out)
		}
	}
	if !commit.Committing() {
		t.Error("successful stream must commit")
	}
}

func TestAntigravityStreamErrorBeforeContentIsRetryable(t *testing.T) {
	sse := "data: {\"error\":{\"message\":\"quota exhausted\"}}\n\n"
	p := &ProxyHandler{}
	rec := httptest.NewRecorder()
	commit := &streamCommit{}
	_, _, err := p.streamAntigravityToOpenAI(io.NopCloser(strings.NewReader(sse)), rec, rec, "m", commit)
	if err == nil || !strings.Contains(err.Error(), "quota exhausted") {
		t.Fatalf("expected quota error, got %v", err)
	}
	if !commit.Retryable() {
		t.Error("error before content must stay retryable")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("nothing should be written before commit, got %q", rec.Body.String())
	}
}

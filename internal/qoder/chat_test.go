package qoder

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// sseFrame builds an upstream stream frame in the real envelope shape, as the
// bare JSON payload. ParseStreamFrame consumes payloads, not SSE lines —
// ConsumeStream is what strips the "data:" prefix.
func sseFrame(body string, status any) string {
	b, _ := json.Marshal(body)
	s, _ := json.Marshal(status)
	return `{"headers":{},"body":` + string(b) + `,"statusCodeValue":` + string(s) + `}`
}

// sseLine wraps a frame as a complete SSE line, as it appears on the wire.
func sseLine(body string, status any) string {
	return "data:" + sseFrame(body, status) + "\n\n"
}

// TestParseStreamFrameDetectsLoginExpired is the regression test for the
// package's most dangerous failure mode.
//
// A credential that upstream has stopped honouring is NOT reported via the HTTP
// response status. The stream opens with 200 and the rejection arrives as the
// first frame's payload. A reader that waits for content blocks until the
// client times out, and the gateway reports a hang rather than a bad credential.
// This test pins that the frame is recognised as an error.
func TestParseStreamFrameDetectsLoginExpired(t *testing.T) {
	frame := sseFrame(`{"code":"105","message":"Login expired"}`, 403)

	_, err := ParseStreamFrame([]byte(frame))
	if err == nil {
		t.Fatal("login-expired frame was not detected; a dead credential would hang the stream")
	}
	if !err.IsLoginExpired() {
		t.Errorf("error not classified as login-expired: %+v", err)
	}
	if err.Status != 403 {
		t.Errorf("status = %d, want 403", err.Status)
	}
	if got := HTTPStatusFor(err); got != http.StatusUnauthorized {
		t.Errorf("HTTPStatusFor = %d, want 401", got)
	}
}

// TestParseStreamFrameDetectsQueueState covers upstream's throttling signal.
//
// A busy model is not a broken credential, and conflating the two would take a
// healthy account out of rotation. The two must be distinguishable.
func TestParseStreamFrameDetectsQueueState(t *testing.T) {
	frame := sseFrame(`{"code":"10605","message":"queued","isQueued":true,"retryAfterSeconds":30}`, 403)

	_, err := ParseStreamFrame([]byte(frame))
	if err == nil {
		t.Fatal("queue frame was not detected")
	}
	if !err.IsQueued() {
		t.Errorf("error not classified as queued: %+v", err)
	}
	if err.IsLoginExpired() {
		t.Error("a queue state must not be mistaken for an expired credential")
	}
	if err.RetryAfterSeconds != 30 {
		t.Errorf("RetryAfterSeconds = %d, want 30", err.RetryAfterSeconds)
	}
	if got := HTTPStatusFor(err); got != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatusFor = %d, want 503", got)
	}
}

// TestParseStreamFrameContent verifies a normal content frame decodes fully.
func TestParseStreamFrameContent(t *testing.T) {
	inner := `{"choices":[{"delta":{"role":"assistant","content":"PONG"}}],"usage":{"prompt_tokens":7,"completion_tokens":2}}`
	d, err := ParseStreamFrame([]byte(sseFrame(inner, 200)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Content != "PONG" {
		t.Errorf("Content = %q, want PONG", d.Content)
	}
	if d.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", d.Role)
	}
	if d.InputTokens != 7 || d.OutputTokens != 2 {
		t.Errorf("usage = (%d,%d), want (7,2)", d.InputTokens, d.OutputTokens)
	}
}

// TestParseStreamFrameReasoningContent covers reasoning models, whose deltas
// arrive in a separate field that must not be dropped or merged into content.
func TestParseStreamFrameReasoningContent(t *testing.T) {
	inner := `{"choices":[{"delta":{"reasoning_content":"thinking..."}}]}`
	d, err := ParseStreamFrame([]byte(sseFrame(inner, 200)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Reasoning != "thinking..." {
		t.Errorf("Reasoning = %q, want thinking...", d.Reasoning)
	}
	if d.Content != "" {
		t.Errorf("content should be empty for a reasoning-only frame, got %q", d.Content)
	}
}

// TestParseStreamFrameUsageOnly keeps token accounting working when usage
// arrives on its own frame with no content.
func TestParseStreamFrameUsageOnly(t *testing.T) {
	inner := `{"usage":{"prompt_tokens":11,"completion_tokens":4}}`
	d, err := ParseStreamFrame([]byte(sseFrame(inner, 200)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.InputTokens != 11 || d.OutputTokens != 4 {
		t.Errorf("usage = (%d,%d), want (11,4)", d.InputTokens, d.OutputTokens)
	}
	if !d.IsEmpty() {
		t.Error("a usage-only frame should carry no content")
	}
}

// TestParseStreamFrameIgnoresMalformed pins the fail-open behaviour on frames
// that are not part of the protocol. Upstream interleaves keep-alives and
// metadata; treating every unrecognised frame as a fatal error would abort
// otherwise-healthy streams.
func TestParseStreamFrameIgnoresMalformed(t *testing.T) {
	for _, frame := range []string{
		`not json at all`,
		`{"headers":{}}`,
		`{"body":""}`,
		`{"body":"not json"}`,
		`{"body":"{\"code\":\"0\"}"}`,
	} {
		d, err := ParseStreamFrame([]byte(frame))
		if err != nil {
			t.Errorf("frame %q produced an error: %v", frame, err)
		}
		if !d.IsEmpty() {
			t.Errorf("frame %q produced a non-empty delta: %+v", frame, d)
		}
	}
}

// TestParseEnvelopeErrorStringStatus covers the alternate encoding where the
// envelope status is a string rather than a number.
func TestParseEnvelopeErrorStringStatus(t *testing.T) {
	var env map[string]any
	if err := json.Unmarshal([]byte(`{"body":"{\"code\":\"105\",\"message\":\"Login expired\"}","statusCodeValue":"403"}`), &env); err != nil {
		t.Fatal(err)
	}
	e := parseEnvelopeError([]byte(`{"body":"{\"code\":\"105\",\"message\":\"Login expired\"}","statusCodeValue":"403"}`))
	if e == nil || !e.IsLoginExpired() {
		t.Fatalf("string-encoded status not handled: %+v", e)
	}
}

// TestUpstreamErrorString makes sure the error message is useful in logs.
func TestUpstreamErrorString(t *testing.T) {
	e := &UpstreamError{Status: 403, Code: "10605", Message: "queued", Queued: true, RetryAfterSeconds: 30}
	msg := e.Error()
	for _, want := range []string{"403", "10605", "queued", "30"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error string %q missing %q", msg, want)
		}
	}
}

// TestBuildChatBodyFromTemplate exercises the body builder against the real
// embedded template. It asserts on structure and on the values that matter
// operationally, without pinning the whole document — the template is upstream's
// contract and may legitimately change.
func TestBuildChatBodyFromTemplate(t *testing.T) {
	installTestTemplate(t)
	body, err := BuildChatBody(ChatRequest{
		Model: "qmodel_38max",
		Messages: []map[string]any{
			{"role": "system", "content": "be brief"},
			{"role": "user", "content": "say PONG"},
		},
		Stream:      true,
		IsReasoning: true,
		UserType:    "personal_standard",
	})
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("produced body is not valid JSON: %v", err)
	}

	// The model selection block must name the requested model.
	mc, ok := got["model_config"].(map[string]any)
	if !ok {
		t.Fatal("model_config missing")
	}
	if mc["key"] != "qmodel_38max" {
		t.Errorf("model_config.key = %v, want qmodel_38max", mc["key"])
	}
	if mc["is_reasoning"] != true {
		t.Errorf("model_config.is_reasoning = %v, want true", mc["is_reasoning"])
	}

	// Upstream correlates a turn using these; all must be present and distinct
	// enough to not collide.
	for _, k := range []string{"request_id", "request_set_id", "chat_record_id", "session_id"} {
		v, _ := got[k].(string)
		if v == "" {
			t.Errorf("%s is empty", k)
		}
		if strings.Contains(v, "00000000-0000") {
			t.Errorf("%s still holds the template placeholder: %s", k, v)
		}
	}
	if got["request_id"] != got["chat_record_id"] {
		t.Error("request_id and chat_record_id identify the same turn and must match")
	}
	if got["stream"] != true {
		t.Errorf("stream = %v, want true", got["stream"])
	}
	if got["aliyun_user_type"] != "personal_standard" {
		t.Errorf("aliyun_user_type = %v", got["aliyun_user_type"])
	}

	// The prompt must appear in the echoed context fields, otherwise upstream
	// sees the template's placeholder text as the user's request.
	cc, _ := got["chat_context"].(map[string]any)
	txt, _ := cc["text"].(map[string]any)
	if txt["text"] != "say PONG" {
		t.Errorf("chat_context.text.text = %v, want 'say PONG'", txt["text"])
	}
	extra, _ := cc["extra"].(map[string]any)
	oc, _ := extra["originalContent"].(map[string]any)
	if oc["text"] != "say PONG" {
		t.Errorf("originalContent.text = %v, want 'say PONG'", oc["text"])
	}

	// business.begin_at must be a live timestamp, not the template's zero.
	biz, _ := got["business"].(map[string]any)
	if fr, ok := biz["begin_at"].(float64); !ok || fr <= 0 {
		t.Errorf("business.begin_at = %v, want a positive timestamp", biz["begin_at"])
	}
	if biz["name"] != "say PONG" {
		t.Errorf("business.name = %v, want 'say PONG'", biz["name"])
	}

	// The caller's own system prompt must be preserved and the template's must
	// not be prepended when one is supplied.
	msgs, _ := got["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("messages missing")
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "be brief" {
		t.Errorf("caller's system prompt not preserved as first message: %+v", first)
	}
}

// TestBuildChatBodyInjectsTemplateSystemPrompt covers the complementary case: no
// caller system prompt means the template's instructions must be carried over.
func TestBuildChatBodyInjectsTemplateSystemPrompt(t *testing.T) {
	installTestTemplate(t)
	body, err := BuildChatBody(ChatRequest{
		Model:    "auto",
		Messages: []map[string]any{{"role": "user", "content": "hi"}},
	})
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	msgs, _ := got["messages"].([]any)
	if len(msgs) < 2 {
		t.Fatalf("expected the template system prompt plus the user turn, got %d messages", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("first message role = %v, want system", first["role"])
	}
}

// TestBuildChatBodyIsConcurrencySafe is a race-detector-visible check that the
// shared template is never mutated by a request. Building bodies in parallel and
// comparing them to a serial build would be the broad test; here the cheaper
// and sharper assertion is that two concurrent builds differ in their per-request
// identifiers, which they could not if they shared one mutable map.
func TestBuildChatBodyIsConcurrencySafe(t *testing.T) {
	installTestTemplate(t)
	const n = 8
	ids := make(chan string, n)
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			b, err := BuildChatBody(ChatRequest{
				Model:    "auto",
				Messages: []map[string]any{{"role": "user", "content": "hi"}},
			})
			if err != nil {
				t.Errorf("BuildChatBody: %v", err)
				done <- struct{}{}
				return
			}
			var got map[string]any
			_ = json.Unmarshal(b, &got)
			ids <- got["request_id"].(string)
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	close(ids)

	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate request_id across concurrent builds: %s (template is shared mutable state)", id)
		}
		seen[id] = true
	}
}

// TestBuildChatBodyRejectsEmptyModel guards the one input the builder cannot
// invent.
func TestBuildChatBodyRejectsEmptyModel(t *testing.T) {
	if _, err := BuildChatBody(ChatRequest{Messages: []map[string]any{{"role": "user", "content": "hi"}}}); err == nil {
		t.Error("expected an error for an empty model")
	}
}

// TestBuildChatBodyHandlesToolCallsAndParts covers tool definitions and
// array-shaped (multi-part) message content.
func TestBuildChatBodyHandlesToolCallsAndParts(t *testing.T) {
	installTestTemplate(t)
	body, err := BuildChatBody(ChatRequest{
		Model: "auto",
		Messages: []map[string]any{
			{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "part one "},
				map[string]any{"type": "text", "text": "part two"},
			}},
		},
		Tools: []any{map[string]any{"type": "function"}},
	})
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	// Multi-part content must be flattened for the prompt echo fields.
	cc, _ := got["chat_context"].(map[string]any)
	txt, _ := cc["text"].(map[string]any)
	if txt["text"] != "part one part two" {
		t.Errorf("multi-part content not flattened: %v", txt["text"])
	}
	if _, ok := got["tools"]; !ok {
		t.Error("tools were dropped from the request")
	}
}

// TestParseModelListShape covers the real envelope, including the snake_case
// field names and the enabled-only filter.
func TestParseModelListShape(t *testing.T) {
	raw := []byte(`{
	  "assistant": [
	    {"key":"qmodel_38max","display_name":"Qoder 38 Max","enable":true,"is_default":true,"is_reasoning":true,"max_input_tokens":180000},
	    {"key":"qfmodel","display_name":"Qoder Fast","enable":true},
	    {"key":"disabled_one","display_name":"Off","enable":false}
	  ],
	  "chat": [{"key":"chat_only","enable":true}]
	}`)

	models, err := parseModelList(raw)
	if err != nil {
		t.Fatalf("parseModelList: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2 (disabled must be excluded): %v", len(models), ModelKeys(models))
	}
	// Sorted by key for deterministic output.
	if models[0].Key != "qfmodel" || models[1].Key != "qmodel_38max" {
		t.Errorf("unexpected order: %v", ModelKeys(models))
	}
	if models[1].Name() != "Qoder 38 Max" {
		t.Errorf("display name not read from display_name: %q", models[1].Name())
	}
	if !models[1].IsReasoning {
		t.Error("is_reasoning not read")
	}
	if models[1].MaxInputTokens != 180000 {
		t.Errorf("max_input_tokens = %d", models[1].MaxInputTokens)
	}
}

// TestParseModelListFallsBackToChatGroup keeps an account with no assistant-group
// models usable rather than reporting it as empty.
func TestParseModelListFallsBackToChatGroup(t *testing.T) {
	raw := []byte(`{"chat":[{"key":"chat_only","enable":true}]}`)
	models, err := parseModelList(raw)
	if err != nil {
		t.Fatalf("parseModelList: %v", err)
	}
	if len(models) != 1 || models[0].Key != "chat_only" {
		t.Errorf("chat-group fallback failed: %v", ModelKeys(models))
	}
}

// TestParseModelListIgnoresCamelCase documents the field-name trap: upstream uses
// snake_case, and a parser expecting camelCase yields nothing while looking
// perfectly reasonable.
func TestParseModelListIgnoresCamelCase(t *testing.T) {
	raw := []byte(`{"assistant":[{"key":"a","displayName":"Wrong Case","enable":true}]}`)
	models, err := parseModelList(raw)
	if err != nil {
		t.Fatalf("parseModelList: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models", len(models))
	}
	if models[0].DisplayName != "" {
		t.Errorf("camelCase field was unexpectedly read: %q", models[0].DisplayName)
	}
	// The fallback keeps the model usable even without a display name.
	if models[0].Name() != "a" {
		t.Errorf("Name() should fall back to the key, got %q", models[0].Name())
	}
}

// TestInferSurface covers the surface-selection heuristic.
func TestInferSurface(t *testing.T) {
	cases := map[string]string{
		"qmodel_gemini_flash": "gemini",
		"claude-sonnet-4":     "claude",
		"qmodel_38max":        "codex",
		"auto":                "codex",
	}
	for model, want := range cases {
		if got := inferSurface(model); got != want {
			t.Errorf("inferSurface(%q) = %q, want %q", model, got, want)
		}
	}
}

// TestConsumeStreamDetectsMidStreamRejection is the end-to-end version of the
// login-expired test: a stream that emits a rejection frame must surface an
// error rather than silently completing.
func TestConsumeStreamDetectsMidStreamRejection(t *testing.T) {
	stream := sseLine(`{"choices":[{"delta":{"role":"assistant"}}]}`, 200) +
		sseLine(`{"code":"105","message":"Login expired"}`, 403) +
		"data:[DONE]\n\n"

	_, err := ConsumeStream(context.Background(), strings.NewReader(stream), "auto", nil)
	if err == nil {
		t.Fatal("stream rejection was not surfaced")
	}
	var ue *UpstreamError
	if !errors.As(err, &ue) || !ue.IsLoginExpired() {
		t.Errorf("error not classified as login-expired: %v", err)
	}
}

// TestConsumeStreamForwardsDeltasAndUsage verifies a healthy stream is decoded
// and accounted correctly.
func TestConsumeStreamForwardsDeltasAndUsage(t *testing.T) {
	stream := sseLine(`{"choices":[{"delta":{"role":"assistant","content":"P"}}]}`, 200) +
		sseLine(`{"choices":[{"delta":{"content":"ONG"}}]}`, 200) +
		sseLine(`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`, 200) +
		"data:[DONE]\n\n"

	var got strings.Builder
	outcome, err := ConsumeStream(context.Background(), strings.NewReader(stream), "auto", func(d StreamDelta) error {
		got.WriteString(d.Content)
		return nil
	})
	if err != nil {
		t.Fatalf("ConsumeStream: %v", err)
	}
	if got.String() != "PONG" {
		t.Errorf("reassembled content = %q, want PONG", got.String())
	}
	if outcome.InputTokens != 5 || outcome.OutputTokens != 3 {
		t.Errorf("usage = (%d,%d), want (5,3)", outcome.InputTokens, outcome.OutputTokens)
	}
	if !outcome.HadContent {
		t.Error("HadContent should be true for a stream with content")
	}
}

// TestConsumeStreamHonoursContextCancellation keeps a caller's cancellation from
// being swallowed and turned into a normal completion.
func TestConsumeStreamHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream := strings.Repeat(sseLine(`{"choices":[{"delta":{"content":"x"}}]}`, 200), 100)
	if _, err := ConsumeStream(ctx, strings.NewReader(stream), "auto", func(StreamDelta) error { return nil }); err == nil {
		t.Error("expected the cancelled context to abort the stream")
	}
}

// TestHTTPStatusForUnclassified keeps an unknown upstream failure from leaking a
// success status to the client.
func TestHTTPStatusForUnclassified(t *testing.T) {
	if got := HTTPStatusFor(&UpstreamError{Message: "something odd"}); got != http.StatusBadGateway {
		t.Errorf("HTTPStatusFor(unclassified) = %d, want 502", got)
	}
	if got := HTTPStatusFor(&UpstreamError{Status: 429}); got != 429 {
		t.Errorf("HTTPStatusFor(429) = %d, want 429", got)
	}
}

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// minimalQoderTemplate is the structural skeleton the Qoder request builder
// depends on, with none of the vendor's content. The real template is
// operator-provisioned because it belongs to the vendor, so tests supply a shape
// rather than a copy.
const minimalQoderTemplate = `{
  "request_id": "{UUID1}", "request_set_id": "{UUID2}", "chat_record_id": "{UUID3}",
  "stream": true, "chat_task": "FREE_INPUT",
  "chat_context": {"chatPrompt": "", "extra": {"context": [], "modelConfig": {"key": "auto"},
    "originalContent": {"type": "text", "text": "p"}}, "features": [], "imageUrls": null,
    "text": {"type": "text", "text": "p"}},
  "is_reply": true, "is_retry": false, "session_id": "{UUID4}", "code_language": "",
  "source": 1, "version": "3", "chat_prompt": "", "parameters": {"max_tokens": 16384},
  "aliyun_user_type": "personal_standard", "session_type": "qoder",
  "agent_id": "agent_common", "task_id": "common",
  "model_config": {"key": "auto", "display_name": "Auto", "model": "", "format": "openai",
    "is_vl": false, "is_reasoning": false, "api_key": "", "url": "", "source": "system",
    "max_input_tokens": 180000},
  "messages": [{"role": "system", "content": "TEST"}], "tools": [],
  "business": {"product": "ide", "version": "1.1.3", "type": "agent", "id": "{UUID5}",
    "name": "p", "begin_at": {TIME1}, "stage": "start"}
}`

// provisionQoderTemplate installs a structural template for the test and clears
// it afterwards, so template state cannot leak between tests.
func provisionQoderTemplate(t *testing.T) {
	t.Helper()
	if err := qoder.SetTemplate([]byte(minimalQoderTemplate)); err != nil {
		t.Fatalf("install template: %v", err)
	}
	t.Cleanup(func() { _ = qoder.SetTemplate([]byte(minimalQoderTemplate)) })
}

// TestQoderInertWhenDisabled is the most important test in this file.
//
// With qoder_enabled absent (the default), no provider is constructed and a
// Qoder-format connection must NOT be diverted — it falls through to the legacy
// path. If this ever fails, an Experimental provider becomes live on a
// deployment that never opted in.
func TestQoderInertWhenDisabled(t *testing.T) {
	h := newTestProxyHandler(t)

	if h.qoderEnabled() {
		t.Fatal("Qoder provider is active without qoder_enabled being set")
	}
	if h.isQoder(&Connection{Format: qoderFormat}) != true {
		t.Error("format detection should still recognise a qoder connection")
	}
	// The branch guard is `isQoder && qoderEnabled`, so detection being true while
	// the provider is nil is the correct inert state.
}

// TestQoderRequiresProvisionedTemplate checks the second gate: enabling the
// provider without provisioning the template must leave it inactive rather than
// accepting requests it cannot build.
func TestQoderRequiresProvisionedTemplate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(qoder.TemplateEnvVar, filepath.Join(dir, "absent.json"))
	t.Setenv("LINTASAN_QODER_SALT", "test-salt")

	h := newTestProxyHandlerWithSettings(t, map[string]string{"qoder_enabled": "true"})

	// The env var points at a missing file, so provisioning fails and the provider
	// must stay nil.
	if h.qoderEnabled() {
		t.Error("provider active with an unreadable template path")
	}
}

// TestQoderActivatesWithTemplateAndFlag is the positive path: both gates open.
func TestQoderActivatesWithTemplateAndFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "template.json")
	if err := os.WriteFile(path, []byte(minimalQoderTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(qoder.TemplateEnvVar, path)
	t.Setenv("LINTASAN_QODER_SALT", "test-salt")

	h := newTestProxyHandlerWithSettings(t, map[string]string{"qoder_enabled": "true"})

	if !h.qoderEnabled() {
		t.Fatal("provider did not activate with both the flag and a provisioned template")
	}
}

// TestQoderNotInGenericRegistry guards the containment property.
//
// The SDK registry resolves by Format with a fallback to the generic provider. If
// Qoder were registered there it would also become resolvable by other paths, so
// it is deliberately kept out and reached only through the explicit branch.
func TestQoderNotInGenericRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "template.json")
	if err := os.WriteFile(path, []byte(minimalQoderTemplate), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(qoder.TemplateEnvVar, path)

	h := newTestProxyHandlerWithSettings(t, map[string]string{"qoder_enabled": "true"})

	if _, ok := h.providerReg.Get("qoder"); ok {
		t.Error("qoder must not be registered in the generic provider registry")
	}
	// The generic fallback must still resolve for a qoder Format, which is what the
	// legacy path relies on when the provider is inert.
	if got := h.providerReg.Resolve(qoderFormat, h.defaultProvider); got != h.defaultProvider {
		t.Error("a qoder-format connection must fall back to the generic provider in the SDK registry")
	}
}

// TestQoderUpstreamRefusesWhenInert keeps the failure legible: a Qoder connection
// on an unconfigured deployment must produce an actionable error, not a nil
// dereference or a silent wrong request.
func TestQoderUpstreamRefusesWhenInert(t *testing.T) {
	h := newTestProxyHandler(t)
	conn := &Connection{ID: "c-qoder", Name: "Qoder", Format: qoderFormat, APIKey: "pt-x"}

	_, err := h.qoderUpstream(context.Background(), conn, []byte(`{"model":"auto","messages":[]}`))
	if err == nil {
		t.Fatal("expected an error for a qoder connection while the provider is inert")
	}
	msg := err.Error()
	if !strings.Contains(msg, qoder.TemplateEnvVar) && !strings.Contains(msg, "disabled") {
		t.Errorf("error should name the missing gate, got: %v", err)
	}
}

// TestStreamQoderToOpenAITranslatesFrames drives the client-facing translation
// against a synthetic envelope stream and asserts on the OpenAI chunks produced.
func TestStreamQoderToOpenAITranslatesFrames(t *testing.T) {
	h := newTestProxyHandler(t)

	stream := strings.Join([]string{
		`data:{"headers":{},"body":"{\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"PO\"}}]}","statusCodeValue":200}`,
		`data:{"headers":{},"body":"{\"choices\":[{\"delta\":{\"content\":\"NG\"}}]}","statusCodeValue":200}`,
		`data:{"headers":{},"body":"{\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3}}","statusCodeValue":200}`,
		`data:[DONE]`,
	}, "\n\n") + "\n\n"

	rec := httptest.NewRecorder()
	flusher, _ := any(rec).(http.Flusher)

	buf, chunks, err := h.streamQoderToOpenAI(context.Background(), io.NopCloser(strings.NewReader(stream)), rec, flusher, "auto")
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	body := rec.Body.String()

	// The client must receive OpenAI chunks, never the envelope.
	if strings.Contains(body, "statusCodeValue") {
		t.Error("envelope frames leaked to the client")
	}
	if !strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Error("no OpenAI chunks produced")
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Error("stream was not terminated with [DONE]")
	}
	if chunks == 0 {
		t.Error("no chunks counted")
	}
	// The accumulated text is what the non-streaming/cache path would store.
	if string(buf) != "PONG" {
		t.Errorf("accumulated content = %q, want PONG", buf)
	}
	// Usage must survive into the terminal frame.
	if !strings.Contains(body, `"prompt_tokens":5`) {
		t.Error("usage was dropped from the terminal frame")
	}
}

// TestStreamQoderSurfacesRefusalAsErrorFrame pins the behaviour that makes this
// provider's failures diagnosable.
//
// The stream opens with HTTP 200 and the refusal arrives inside it. Headers are
// already flushed by the time the failure is known, so the only channel left is a
// typed SSE error frame. A client that gets that can retry; a client that gets a
// silent empty stream cannot tell refusal from a slow model.
func TestStreamQoderSurfacesRefusalAsErrorFrame(t *testing.T) {
	h := newTestProxyHandler(t)

	stream := `data:{"headers":{},"body":"{\"code\":\"105\",\"message\":\"Login expired\"}","statusCodeValue":403}` + "\n\n"

	rec := httptest.NewRecorder()
	flusher, _ := any(rec).(http.Flusher)

	_, _, err := h.streamQoderToOpenAI(context.Background(), io.NopCloser(strings.NewReader(stream)), rec, flusher, "auto")
	if err == nil {
		t.Fatal("a refused credential must surface as an error, not a clean end of stream")
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"error"`) {
		t.Error("no SSE error frame was written for the client")
	}
	// The guidance matters as much as the detection: a caller told to discard the
	// credential would throw away one that works again within minutes.
	if !strings.Contains(body, "transient") && !strings.Contains(body, "Retry") {
		t.Errorf("error frame should tell the caller to retry rather than discard the credential: %s", body)
	}
	if !strings.Contains(body, "credential_cooldown") {
		t.Errorf("expected a credential_cooldown error type, got: %s", body)
	}
}

// TestQoderErrorDiagnosisQueueDistinctFromCredential keeps a throttled model from
// being reported as a credential problem — conflating them would take a healthy
// account out of rotation.
func TestQoderErrorDiagnosisQueueDistinctFromCredential(t *testing.T) {
	msg, kind := qoderErrorDiagnosis(&qoder.UpstreamError{
		Status: 403, Code: qoder.QueueErrorCode, Queued: true, RetryAfterSeconds: 30,
	})
	if kind != "upstream_queue" {
		t.Errorf("kind = %q, want upstream_queue", kind)
	}
	if !strings.Contains(msg, "30") {
		t.Errorf("queue message should carry the retry delay, got: %q", msg)
	}

	loginMsg, loginKind := qoderErrorDiagnosis(&qoder.UpstreamError{Status: 403, Code: qoder.LoginExpiredCode})
	if loginKind != "credential_cooldown" {
		t.Errorf("kind = %q, want credential_cooldown", loginKind)
	}
	if !strings.Contains(loginMsg, "transient") {
		t.Errorf("credential message should say the condition is transient, got: %q", loginMsg)
	}
}

// TestQoderNonStreamAssemblesResponse verifies the non-streaming shape: the frame
// stream is collected into a single OpenAI completion object rather than passed
// through as envelopes.
func TestQoderNonStreamAssemblesResponse(t *testing.T) {
	h := newTestProxyHandler(t)

	stream := strings.Join([]string{
		`data:{"headers":{},"body":"{\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"PO\"}}]}","statusCodeValue":200}`,
		`data:{"headers":{},"body":"{\"choices\":[{\"delta\":{\"content\":\"NG\"}}]}","statusCodeValue":200}`,
		`data:{"headers":{},"body":"{\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":2}}","statusCodeValue":200}`,
		`data:[DONE]`,
	}, "\n\n") + "\n\n"

	raw, err := h.qoderNonStreamResponse(context.Background(), io.NopCloser(strings.NewReader(stream)), "auto")
	if err != nil {
		t.Fatalf("qoderNonStreamResponse: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if got["object"] != "chat.completion" {
		t.Errorf("object = %v, want chat.completion", got["object"])
	}
	choices, _ := got["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices = %d, want 1", len(choices))
	}
	ch, _ := choices[0].(map[string]any)
	msg, _ := ch["message"].(map[string]any)
	if msg["content"] != "PONG" {
		t.Errorf("assembled content = %v, want PONG", msg["content"])
	}
	if ch["finish_reason"] != "stop" {
		t.Errorf("finish_reason = %v, want stop", ch["finish_reason"])
	}
	usage, _ := got["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(9) {
		t.Errorf("usage not carried through: %v", usage)
	}
}

// TestQoderNonStreamSurfacesRefusal keeps the non-streaming path from reporting a
// refused credential as an empty success.
func TestQoderNonStreamSurfacesRefusal(t *testing.T) {
	h := newTestProxyHandler(t)

	stream := `data:{"headers":{},"body":"{\"code\":\"105\",\"message\":\"Login expired\"}","statusCodeValue":403}` + "\n\n"

	if _, err := h.qoderNonStreamResponse(context.Background(), io.NopCloser(strings.NewReader(stream)), "auto"); err == nil {
		t.Fatal("a refused credential must be an error, not an empty completion")
	}
}

// newTestProxyHandlerWithSettings builds a handler through the REAL constructor
// against an in-memory database carrying the given settings, so the activation
// tests exercise initQoder exactly as startup does rather than poking the field
// directly.
func newTestProxyHandlerWithSettings(t *testing.T, settings map[string]string) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	for k, v := range settings {
		if err := database.SetSetting(k, v); err != nil {
			t.Fatalf("set setting %s: %v", k, err)
		}
	}
	return NewProxyHandler(&config.Config{}, database)
}

// unused import guards: io is used by the helpers above in the real file; keep
// the reference explicit so a refactor cannot silently drop it.
var _ = io.Discard

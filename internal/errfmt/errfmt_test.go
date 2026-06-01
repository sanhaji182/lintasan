package errfmt

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFromStatus_OpenAIShape(t *testing.T) {
	body := []byte(`{"error":{"message":"Incorrect API key provided: sk-test","type":"invalid_request_error","code":"invalid_api_key"}}`)
	e := FromStatus(401, body, "upstream status 401")
	if e.Message != "Incorrect API key provided: sk-test" {
		t.Errorf("message: %q", e.Message)
	}
	if e.Code != "invalid_api_key" {
		t.Errorf("code: %q", e.Code)
	}
	if e.Type != TypeAuthenticationError {
		t.Errorf("type: %q", e.Type)
	}
}

func TestFromStatus_AnthropicShape(t *testing.T) {
	body := []byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	e := FromStatus(401, body, "upstream status 401")
	if e.Message != "invalid x-api-key" {
		t.Errorf("message: %q", e.Message)
	}
	if e.Code != "authentication_error" {
		t.Errorf("code: %q", e.Code)
	}
}

func TestFromStatus_GoogleShape(t *testing.T) {
	body := []byte(`{"error":{"code":401,"message":"API key not valid. Please pass a valid API key.","status":"UNAUTHENTICATED"}}`)
	e := FromStatus(401, body, "")
	if !strings.Contains(e.Message, "API key not valid") {
		t.Errorf("message: %q", e.Message)
	}
	// Google doesn't return a code string we recognize — but msg mentions
	// "API key", so the 401 path should normalize to invalid_api_key.
	if e.Code != CodeInvalidAPIKey {
		t.Errorf("code: %q (want %q)", e.Code, CodeInvalidAPIKey)
	}
	if e.Type != TypeAuthenticationError {
		t.Errorf("type: %q", e.Type)
	}
}

func TestFromStatus_GenericString(t *testing.T) {
	body := []byte(`{"error":"Invalid proxy server token passed"}`)
	e := FromStatus(401, body, "fallback")
	if e.Message != "Invalid proxy server token passed" {
		t.Errorf("message: %q", e.Message)
	}
	if e.Code != CodeInvalidAPIKey {
		t.Errorf("expected auth-normalized code, got %q", e.Code)
	}
}

func TestFromStatus_EmptyBody(t *testing.T) {
	e := FromStatus(503, []byte(""), "")
	if e.Message != "upstream status 503" {
		t.Errorf("expected default msg, got %q", e.Message)
	}
	if e.Type != TypeUpstreamError {
		t.Errorf("type: %q", e.Type)
	}
}

func TestFromStatus_NotFound_Model(t *testing.T) {
	body := []byte(`{"error":{"message":"The model 'foo' does not exist","type":"invalid_request_error","code":"model_not_found"}}`)
	e := FromStatus(404, body, "")
	if e.Code != "model_not_found" {
		t.Errorf("code: %q", e.Code)
	}
}

func TestFromStatus_RateLimit(t *testing.T) {
	// Both cases: upstream sets `type` (Anthropic/LiteLLM pattern) or
	// `code` (OpenAI pattern) — our parser falls back from code→type, so
	// both yield the same result.
	for _, body := range []string{
		`{"error":{"message":"Rate limit reached","type":"rate_limit_error","code":"rate_limit_error"}}`,
		`{"error":{"message":"Slow down please","type":"rate_limit_error"}}`,
	} {
		e := FromStatus(429, []byte(body), "")
		if e.Type != TypeRateLimitError {
			t.Errorf("type: %q", e.Type)
		}
		if e.Code != "rate_limit_error" {
			t.Errorf("code: %q (want rate_limit_error)", e.Code)
		}
	}
}

func TestFromStatus_SumopodShape(t *testing.T) {
	// Sumopod is a LiteLLM proxy — its error shape: {"error":{"message":"...","type":"...","code":"..."}}
	body := []byte(`{"error":{"message":"Authentication Error, Invalid proxy server token passed. Received API Key = sk-...KVQQ","type":"auth_error","code":"401"}}`)
	e := FromStatus(401, body, "")
	if !strings.Contains(e.Message, "Invalid proxy server token") {
		t.Errorf("message: %q", e.Message)
	}
	// upstream code "401" is not in known map → falls through to status path
	// 401 + msg contains "token" → invalid_api_key
	if e.Code != CodeInvalidAPIKey {
		t.Errorf("code: %q (want %q)", e.Code, CodeInvalidAPIKey)
	}
}

func TestFromNetworkError_Refused(t *testing.T) {
	e := FromNetworkError(errors.New("dial tcp 127.0.0.1:9999: connect: connection refused"))
	if e.Code != CodeConnRefused {
		t.Errorf("code: %q", e.Code)
	}
}

func TestFromNetworkError_Timeout(t *testing.T) {
	e := FromNetworkError(errors.New("context deadline exceeded"))
	if e.Code != CodeTimeout {
		t.Errorf("code: %q", e.Code)
	}
}

func TestFromNetworkError_Nil(t *testing.T) {
	e := FromNetworkError(nil)
	if e == nil {
		t.Fatal("expected non-nil")
	}
	if e.Type != TypeNetworkError {
		t.Errorf("type: %q", e.Type)
	}
}

func TestWrite_Shape(t *testing.T) {
	rec := httptest.NewRecorder()
	e := New("bad key", TypeAuthenticationError, CodeInvalidAPIKey)
	Write(rec, 401, e, nil, map[string]any{"success": false, "latency_ms": 42})

	if rec.Code != 401 {
		t.Errorf("status: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: %q", ct)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if out["error"] == nil {
		t.Fatal("expected error field")
	}
	em := out["error"].(map[string]any)
	if em["code"] != CodeInvalidAPIKey {
		t.Errorf("code: %v", em["code"])
	}
	if em["type"] != TypeAuthenticationError {
		t.Errorf("type: %v", em["type"])
	}
	if out["data"] != nil {
		t.Errorf("expected null data on error, got %v", out["data"])
	}
	if out["latency_ms"] != float64(42) {
		t.Errorf("extra field: %v", out["latency_ms"])
	}
}

func TestWrite_SuccessShape(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, 200, nil, map[string]any{"models": 12}, map[string]any{"latency_ms": 145})
	if rec.Code != 200 {
		t.Errorf("status: %d", rec.Code)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["error"] != nil {
		t.Errorf("expected null error on success, got %v", out["error"])
	}
	if out["data"] == nil {
		t.Error("expected data on success")
	}
}

func TestHintForMessage(t *testing.T) {
	if HintForMessage("Invalid API key") == "" {
		t.Error("expected hint for API key message")
	}
	if HintForMessage("some random thing") != "" {
		t.Error("expected empty hint for unrelated message")
	}
	// Word-boundary check: "/v1/models" should NOT trigger model hint
	if HintForMessage("dial tcp 127.0.0.1:9999: connect: connection refused (path /v1/models)") != "" {
		t.Error("URL path '/v1/models' must not trigger model hint")
	}
	// But "model not found" SHOULD trigger it
	if HintForMessage("The model 'gpt-99' was not found") == "" {
		t.Error("expected model hint for 'model not found'")
	}
}

func TestNew_Defaults(t *testing.T) {
	e := New("oops", "", "")
	if e.Type != TypeServerError {
		t.Errorf("default type: %q", e.Type)
	}
	if e.Code != CodeUnknown {
		t.Errorf("default code: %q", e.Code)
	}
}

// smoke test for the http package import (kept for vet to be happy)
var _ = http.StatusOK

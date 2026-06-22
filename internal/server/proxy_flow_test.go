package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

// TestProxyFlow_SimpleChatCompletion exercises the full proxy path:
// mock upstream → seed connection & model → auth → chat completion → assert response shape.
func TestProxyFlow_SimpleChatCompletion(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-for-proxy-flow-test-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "proxy-admin", "proxy-pass-123")

	if !s.isActive() {
		t.Fatal("expected ACTIVE state")
	}

	// --- Mock upstream that responds like a real LLM API -----------------------
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "chat/completions") {
			t.Errorf("expected chat/completions path, got %s", r.URL.Path)
		}
		// Verify auth header was forwarded
		if r.Header.Get("Authorization") == "" {
			t.Errorf("expected Authorization header on upstream request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-mock123",
			"object": "chat.completion",
			"created": 1700000000,
			"model": "gpt-4o-mock",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello! I'm a mock assistant."
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20,
				"total_tokens": 30
			}
		}`))
	}))
	t.Cleanup(mockUpstream.Close)

	// --- Seed connection pointing to our mock upstream ---------------------------
	_, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, is_active, priority)
		VALUES (?, ?, ?, ?, ?, ?, 1, 10)`,
		"proxy-test-conn", "Proxy Test", mockUpstream.URL, "sk-mock-key", "openai", "/v1/chat/completions")
	if err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	// --- Seed discovered model so proxy can route to it -------------------------
	_, err = s.db.Conn().Exec(`
		INSERT INTO discovered_models (id, connection_id, model_id, model_name)
		VALUES (?, ?, ?, ?)`,
		"dm-proxy-test-conn-gpt4o", "proxy-test-conn", "gpt-4o-mock", "GPT-4o Mock")
	if err != nil {
		t.Fatalf("seed model: %v", err)
	}

	// --- Send authenticated chat completion request -----------------------------
	body := `{"model":"gpt-4o-mock","messages":[{"role":"user","content":"Hello"}]}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-master-key-for-proxy-flow-test-1234567890")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("chat completion request failed: %v", err)
	}
	defer resp.Body.Close()

	// --- Assert response status and shape ----------------------------------------
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %s", ct)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("failed to parse response JSON: %v\nbody: %s", err, string(respBody))
	}

	// Verify top-level shape
	if result["id"] == nil {
		t.Error("expected 'id' in response")
	}
	if result["object"] != "chat.completion" {
		t.Errorf("expected object='chat.completion', got %v", result["object"])
	}

	// Verify choices
	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatal("expected non-empty choices array")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		t.Fatal("expected choice to be a map")
	}
	msg, ok := choice["message"].(map[string]any)
	if !ok {
		t.Fatal("expected message in choice")
	}
	if msg["role"] != "assistant" {
		t.Errorf("expected role='assistant', got %v", msg["role"])
	}
	if msg["content"].(string) == "" {
		t.Errorf("expected non-empty content")
	}

	// Verify usage
	usage, ok := result["usage"].(map[string]any)
	if !ok {
		t.Fatal("expected usage in response")
	}
	if usage["total_tokens"] == nil {
		t.Errorf("expected total_tokens in usage")
	}
}

// TestProxyFlow_NoSuchModel verifies that a request for an unknown model
// returns a proper 404 rather than hanging or panicking.
func TestProxyFlow_NoSuchModel(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-proxy-404-1234567890"}
	_, ts := newTestServer(t, cfg)

	body := `{"model":"nonexistent-model-v99","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-master-key-proxy-404-1234567890")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404 or 503 for unknown model, got %d: %s", resp.StatusCode, string(b))
	}
}

// TestProxyFlow_MissingModelField verifies that omitting the model field
// returns a 400 Bad Request.
func TestProxyFlow_MissingModelField(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-proxy-400-1234567890"}
	_, ts := newTestServer(t, cfg)

	body := `{"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-master-key-proxy-400-1234567890")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for missing model, got %d: %s", resp.StatusCode, string(b))
	}
}

// TestProxyFlow_InvalidJSON verifies that malformed JSON body returns 400.
func TestProxyFlow_InvalidJSON(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-proxy-badjson-1234567890"}
	_, ts := newTestServer(t, cfg)

	body := `{this is not valid json`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-master-key-proxy-badjson-1234567890")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", resp.StatusCode, string(b))
	}
}

// TestProxyFlow_RequestBodyHeaders checks that X-Lintasan-* headers are
// set on responses from the proxy handler.
func TestProxyFlow_ResponseHeaders(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-proxy-headers-1234567890"}
	s, ts := newTestServer(t, cfg)
	makeKnownAdmin(t, s, "proxy-headers-admin", "pass123")

	// Simple mock that returns quickly
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	t.Cleanup(mock.Close)

	_, _ = s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, is_active, priority)
		VALUES (?, ?, ?, ?, ?, ?, 1, 10)`,
		"proxy-hdr-conn", "HDR Test", mock.URL, "sk-key", "openai", "/v1/chat/completions")
	_, _ = s.db.Conn().Exec(`
		INSERT INTO discovered_models (id, connection_id, model_id)
		VALUES (?, ?, ?)`,
		"dm-hdr-gpt4", "proxy-hdr-conn", "gpt-4o")

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-master-key-proxy-headers-1234567890")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}

	// Check for proxy-specific headers
	headersOfInterest := []string{"X-Lintasan-Task-Class", "X-Lintasan-Route-Profile", "X-Lintasan-Mode"}
	for _, h := range headersOfInterest {
		if v := resp.Header.Get(h); v == "" {
			t.Errorf("expected header %s to be set", h)
		} else {
			t.Logf("header %s = %s", h, v)
		}
	}
}

// TestProxyFlow_NoAuth verifies that unauthenticated requests to /v1/chat/completions are rejected.
func TestProxyFlow_NoAuth(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-proxy-noauth-1234567890"}
	_, ts := newTestServer(t, cfg)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401 for unauthenticated request, got %d: %s", resp.StatusCode, string(b))
	}
}

// TestProxyFlow_ModelsList verifies GET /v1/models returns a list without error.
func TestProxyFlow_ModelsList(t *testing.T) {
	cfg := &config.Config{MasterKey: "test-master-key-proxy-models-1234567890"}
	s, ts := newTestServer(t, cfg)

	// Seed a discovered model so the list has data
	_, _ = s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority)
		VALUES (?, ?, ?, ?, ?, 1, 10)`,
		"models-test-conn", "Models Test", "http://localhost:9999", "sk-key", "openai")
	_, _ = s.db.Conn().Exec(`
		INSERT INTO discovered_models (id, connection_id, model_id, model_name)
		VALUES (?, ?, ?, ?)`,
		"dm-models-test", "models-test-conn", "test-model-1", "Test Model 1")

	req, _ := http.NewRequest("GET", ts.URL+"/v1/models", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.MasterKey))

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("models request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 for /v1/models, got %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]any
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("failed to parse models response: %v\nbody: %s", err, string(b))
	}
	t.Logf("/v1/models response: object=%v, data count=%v", result["object"], len(result["data"].([]any)))
}

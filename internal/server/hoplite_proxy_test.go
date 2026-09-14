package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestHopliteModelsAreAdvertisedOnlyWhenConfigured(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"proj_1","name":"Acme App"},{"id":"proj_2","name":"Tools"}]}`))
	}))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	token := makeKnownAdmin(t, s, "models-admin", "correct horse battery")

	resp := hopliteRequest(t, ts, http.MethodGet, "/v1/models", "", token)
	body := decodeEnvelope(t, resp)
	if strings.Contains(mustJSON(t, body), "hoplite-agent/") {
		t.Fatal("unconfigured Hoplite models were advertised")
	}

	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	resp = hopliteRequest(t, ts, http.MethodGet, "/v1/models", "", token)
	body = decodeEnvelope(t, resp)
	encoded := mustJSON(t, body)
	for _, want := range []string{"hoplite-agent/proj_1", "hoplite-agent/proj_2", "Hoplite Agent"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("models response missing %q: %s", want, encoded)
		}
	}
}

func TestHopliteChatCompletionCreatesPollsAndMapsTerminalResult(t *testing.T) {
	var creates atomic.Int32
	var operationID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/threads":
			creates.Add(1)
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			operationID, _ = in["clientOperationId"].(string)
			if in["projectId"] != "proj_1" || in["prompt"] != "Fix the flaky test" {
				t.Fatalf("bad create payload: %#v", in)
			}
			if in["autoFix"] != false || in["autoMerge"] != false {
				t.Fatalf("unsafe defaults: %#v", in)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_1","projectId":"proj_1","status":"queued"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_1":
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_1","projectId":"proj_1","status":"ready","pullRequests":[{"url":"https://example.test/pr/1","number":1,"state":"open"}]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_1/messages":
			_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"m1","role":"assistant","kind":"thinking","content":"private trace"},{"id":"m2","role":"assistant","kind":"chat","content":"Implemented and tested."}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	s.hoplitePollInterval, s.hopliteProxyTimeout = time.Millisecond, time.Second
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	token := makeKnownAdmin(t, s, "chat-admin", "correct horse battery")
	payload := `{"model":"hoplite-agent/proj_1","messages":[{"role":"user","content":"Fix the flaky test"}]}`
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", payload, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%#v", resp.StatusCode, decodeEnvelope(t, resp))
	}
	body := decodeEnvelope(t, resp)
	encoded := mustJSON(t, body)
	for _, want := range []string{"Implemented and tested.", "thr_1", "https://example.test/pr/1", "hoplite-agent/proj_1"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("completion missing %q: %s", want, encoded)
		}
	}
	if strings.Contains(encoded, "private trace") {
		t.Fatalf("thinking leaked into completion: %s", encoded)
	}
	if creates.Load() != 1 || operationID == "" {
		t.Fatalf("creates=%d operationID=%q", creates.Load(), operationID)
	}
}

func TestHopliteChatCompletionRejectsStreamingWithoutUpstream(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	token := makeKnownAdmin(t, s, "stream-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"hoplite-agent/proj_1","stream":true,"messages":[{"role":"user","content":"x"}]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want=400", resp.StatusCode)
	}
	if calls.Load() != 0 {
		t.Fatalf("stream rejection contacted upstream %d times", calls.Load())
	}
}

func TestHopliteChatCompletionMapsTerminalFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_bad","projectId":"proj_1","status":"queued"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_bad","projectId":"proj_1","status":"failed","runStatus":"failed"}}`))
	}))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	s.hoplitePollInterval, s.hopliteProxyTimeout = time.Millisecond, time.Second
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	token := makeKnownAdmin(t, s, "failure-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"hoplite-agent/proj_1","messages":[{"role":"user","content":"x"}]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d want=502", resp.StatusCode)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

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

func TestHopliteExplicitThreadRollsContextIntoNewThread(t *testing.T) {
	var creates atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_existing":
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_existing","projectId":"proj_1","status":"ready"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_existing/messages":
			_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"user_old","role":"user","kind":"chat","content":"Initial request"},{"id":"assistant_old","role":"assistant","kind":"chat","content":"Initial answer"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/threads":
			creates.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			prompt, _ := payload["prompt"].(string)
			for _, want := range []string{"Previous Hoplite conversation", "Initial request", "Initial answer", "Continue"} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("rollover prompt missing %q: %s", want, prompt)
				}
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_rollover","projectId":"proj_1","status":"queued"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_rollover":
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_rollover","projectId":"proj_1","status":"ready"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_rollover/messages":
			_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"new","role":"assistant","kind":"chat","content":"continued answer"}]}`))
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
	token := makeKnownAdmin(t, s, "continue-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"hoplite-agent/proj_1","thread_id":"thr_existing","messages":[{"role":"user","content":"Continue"}]}`, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%#v", resp.StatusCode, decodeEnvelope(t, resp))
	}
	if got := resp.Header.Get("X-Lintasan-Thread-Id"); got != "thr_rollover" {
		t.Fatalf("thread header=%q", got)
	}
	if got := resp.Header.Get("X-Lintasan-Continuation-Mode"); got != "context-rollover" {
		t.Fatalf("continuation mode=%q", got)
	}
	body := mustJSON(t, decodeEnvelope(t, resp))
	if !strings.Contains(body, "continued answer") || !strings.Contains(body, "thr_rollover") || !strings.Contains(body, "thr_existing") {
		t.Fatalf("unexpected completion: %s", body)
	}
	if creates.Load() != 1 {
		t.Fatalf("creates=%d", creates.Load())
	}
}

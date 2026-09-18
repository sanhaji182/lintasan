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

func TestHopliteExplicitThreadContinuesWithoutCreatingThread(t *testing.T) {
	var creates, appends atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/threads":
			creates.Add(1)
			t.Fatal("continuation must not create a new thread")
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_existing":
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_existing","projectId":"proj_1","status":"ready"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_existing/messages":
			if appends.Load() == 0 {
				_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"old","role":"assistant","kind":"chat","content":"old answer"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"old","role":"assistant","kind":"chat","content":"old answer"},{"id":"new","role":"assistant","kind":"chat","content":"continued answer"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/threads/thr_existing/messages":
			appends.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["content"] != "Continue" {
				t.Fatalf("append payload=%#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true,"message":{"id":"user_new"},"queued":true}`))
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
	if got := resp.Header.Get("X-Lintasan-Thread-Id"); got != "thr_existing" {
		t.Fatalf("thread header=%q", got)
	}
	body := mustJSON(t, decodeEnvelope(t, resp))
	if !strings.Contains(body, "continued answer") || !strings.Contains(body, "thr_existing") {
		t.Fatalf("unexpected completion: %s", body)
	}
	if creates.Load() != 0 || appends.Load() != 1 {
		t.Fatalf("creates=%d appends=%d", creates.Load(), appends.Load())
	}
}

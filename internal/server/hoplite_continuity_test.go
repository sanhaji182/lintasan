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

func TestHopliteAgentIdentitySessionIsolationAndContinuity(t *testing.T) {
	var createdThreads []string
	var createdPrompts []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/threads":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			prompt, _ := payload["prompt"].(string)
			createdPrompts = append(createdPrompts, prompt)
			threadID := "thr_auto_" + time.Now().Format("150405.000000000")
			if strings.Contains(prompt, "devops task 1") {
				threadID = "thr_devops_1"
			} else if strings.Contains(prompt, "devops task 2") {
				threadID = "thr_devops_2"
			} else if strings.Contains(prompt, "coder task") {
				threadID = "thr_coder_1"
			} else if strings.Contains(prompt, "devops fresh") {
				threadID = "thr_devops_3"
			}
			createdThreads = append(createdThreads, threadID)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"` + threadID + `","projectId":"proj_1","status":"queued"}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/threads/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/threads/"), "/")
			id := parts[0]
			if len(parts) > 1 && parts[1] == "messages" {
				// return mock transcript for thread
				_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"m1","role":"user","kind":"chat","content":"History from ` + id + `"},{"id":"m2","role":"assistant","kind":"chat","content":"Answer from ` + id + `"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"` + id + `","projectId":"proj_1","status":"ready"}}`))
			}
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

	// 1. Agent DevOps calls turn 1
	req1 := `{"model":"hoplite-agent/proj_1","messages":[{"role":"user","content":"devops task 1"}],"user":"devops"}`
	resp1 := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", req1, "test-master-key-1234567890")
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("turn 1 status=%d body=%#v", resp1.StatusCode, decodeEnvelope(t, resp1))
	}
	if got := resp1.Header.Get("X-Lintasan-Thread-Id"); got != "thr_devops_1" {
		t.Fatalf("turn 1 thread=%q", got)
	}
	if got := resp1.Header.Get("X-Lintasan-Agent-Id"); got != "agent:devops" {
		t.Fatalf("turn 1 agent_id=%q", got)
	}
	if got := resp1.Header.Get("X-Lintasan-Continuation-Mode"); got != "" {
		t.Fatalf("turn 1 should be fresh, got continuation=%q", got)
	}

	// 2. Agent DevOps calls turn 2 (without thread_id) -> must automatically continue from thr_devops_1
	req2 := `{"model":"hoplite-agent/proj_1","messages":[{"role":"user","content":"devops task 2"}],"user":"devops"}`
	resp2 := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", req2, "test-master-key-1234567890")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("turn 2 status=%d body=%#v", resp2.StatusCode, decodeEnvelope(t, resp2))
	}
	if got := resp2.Header.Get("X-Lintasan-Thread-Id"); got != "thr_devops_2" {
		t.Fatalf("turn 2 thread=%q", got)
	}
	if got := resp2.Header.Get("X-Lintasan-Continuation-Mode"); got != "context-rollover" {
		t.Fatalf("turn 2 continuation mode=%q", got)
	}
	// Prompt in turn 2 must contain history from thr_devops_1
	lastPrompt := createdPrompts[len(createdPrompts)-1]
	if !strings.Contains(lastPrompt, "History from thr_devops_1") {
		t.Fatalf("turn 2 did not roll over thr_devops_1 history: %s", lastPrompt)
	}

	// 3. Agent Coder calls (different agent identity) -> must NOT see devops history and must create fresh thread
	req3 := `{"model":"hoplite-agent/proj_1","messages":[{"role":"user","content":"coder task"}],"agent_id":"coder"}`
	resp3 := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", req3, "test-master-key-1234567890")
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("coder turn status=%d body=%#v", resp3.StatusCode, decodeEnvelope(t, resp3))
	}
	if got := resp3.Header.Get("X-Lintasan-Thread-Id"); got != "thr_coder_1" {
		t.Fatalf("coder turn thread=%q", got)
	}
	if got := resp3.Header.Get("X-Lintasan-Agent-Id"); got != "agent:coder" {
		t.Fatalf("coder turn agent_id=%q", got)
	}
	coderPrompt := createdPrompts[len(createdPrompts)-1]
	if strings.Contains(coderPrompt, "thr_devops") {
		t.Fatalf("coder turn leaked devops history: %s", coderPrompt)
	}

	// 4. Agent DevOps calls with reset_thread -> starts fresh thread without rollover
	req4 := `{"model":"hoplite-agent/proj_1","messages":[{"role":"user","content":"devops fresh"}],"user":"devops","reset_thread":true}`
	resp4 := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", req4, "test-master-key-1234567890")
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("reset turn status=%d body=%#v", resp4.StatusCode, decodeEnvelope(t, resp4))
	}
	if got := resp4.Header.Get("X-Lintasan-Thread-Id"); got != "thr_devops_3" {
		t.Fatalf("reset turn thread=%q", got)
	}
	resetPrompt := createdPrompts[len(createdPrompts)-1]
	if strings.Contains(resetPrompt, "History from") {
		t.Fatalf("reset turn rolled over history when it should be fresh: %s", resetPrompt)
	}
}

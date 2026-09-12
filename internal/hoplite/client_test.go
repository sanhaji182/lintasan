package hoplite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientListProjectsUsesAPIKeyAndParsesResponse(t *testing.T) {
	const key = "hop_test_secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/projects" {
			t.Fatalf("request = %s %s, want GET /api/projects", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != key {
			t.Fatalf("X-Api-Key = %q, want test key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"proj_1","name":"acme/app","defaultBranch":"main","repos":[{"repoFullName":"acme/app","repositoryId":"repo_1"}]}]}`))
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, key, upstream.Client())
	projects, meta, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if meta.StatusCode != http.StatusOK || len(projects) != 1 {
		t.Fatalf("meta/projects = %#v/%#v", meta, projects)
	}
	if projects[0].ID != "proj_1" || projects[0].Repos[0].RepoFullName != "acme/app" {
		t.Fatalf("unexpected project: %#v", projects[0])
	}
}

func TestClientCreateThreadSendsSafeDefaultsAndIdempotency(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/threads" {
			t.Fatalf("request = %s %s, want POST /api/threads", r.Method, r.URL.Path)
		}
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if got["projectId"] != "proj_1" || got["prompt"] != "Fix the failing test" {
			t.Fatalf("unexpected payload: %#v", got)
		}
		if got["autoFix"] != false || got["autoMerge"] != false {
			t.Fatalf("unsafe defaults: %#v", got)
		}
		if got["clientOperationId"] != "lintasan-op-1" {
			t.Fatalf("missing idempotency key: %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_1","projectId":"proj_1","title":"Fix test","status":"queued"},"run":{"id":"run_1","status":"queued"}}`))
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, "hop_test", upstream.Client())
	result, meta, err := client.CreateThread(context.Background(), CreateThreadRequest{
		ProjectID:         "proj_1",
		Prompt:            "Fix the failing test",
		Title:             "Fix test",
		ClientOperationID: "lintasan-op-1",
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if meta.StatusCode != http.StatusCreated || result.Thread.ID != "thr_1" || result.Run == nil || result.Run.ID != "run_1" {
		t.Fatalf("unexpected result: meta=%#v result=%#v", meta, result)
	}
}

func TestClientListsAndReadsThreadState(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads":
			if got := r.URL.Query().Get("projectId"); got != "proj_1" {
				t.Fatalf("projectId = %q", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"threads":[{"id":"thr_1","projectId":"proj_1","status":"running","pullRequests":[]}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_1":
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_1","projectId":"proj_1","status":"succeeded","pullRequests":[{"url":"https://github.com/acme/app/pull/1","number":1}]}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/threads/thr_1/messages":
			_, _ = w.Write([]byte(`{"ok":true,"messages":[{"id":"msg_1","role":"assistant","content":"Done"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, "hop_test", upstream.Client())
	threads, _, err := client.ListThreads(context.Background(), "proj_1")
	if err != nil || len(threads) != 1 || threads[0].Status != "running" {
		t.Fatalf("ListThreads: threads=%#v err=%v", threads, err)
	}
	thread, _, err := client.GetThread(context.Background(), "thr_1")
	if err != nil || len(thread.PullRequests) != 1 || thread.PullRequests[0].Number != 1 {
		t.Fatalf("GetThread: thread=%#v err=%v", thread, err)
	}
	messages, _, err := client.ListMessages(context.Background(), "thr_1")
	if err != nil || len(messages) != 1 || messages[0].Content != "Done" {
		t.Fatalf("ListMessages: messages=%#v err=%v", messages, err)
	}
}

func TestClientErrorPreservesOperationalMetadataWithoutLeakingKey(t *testing.T) {
	const key = "hop_super_secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req_123")
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error":"rate_limit"}`))
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, key, upstream.Client())
	_, _, err := client.ListProjects(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	upErr, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("error type = %T, want *UpstreamError", err)
	}
	if upErr.StatusCode != 429 || upErr.Code != "rate_limit" || upErr.RequestID != "req_123" || upErr.RetryAfter != "17" {
		t.Fatalf("unexpected upstream error: %#v", upErr)
	}
	if strings.Contains(err.Error(), key) {
		t.Fatal("error leaked API key")
	}
}

func TestClientAcceptsMessageResponsesLargerThanErrorBodyLimit(t *testing.T) {
	largeContent := strings.Repeat("x", 40_000)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":       true,
			"messages": []map[string]any{{"id": "msg_large", "role": "assistant", "content": largeContent}},
		})
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, "hop_test", upstream.Client())
	messages, _, err := client.ListMessages(context.Background(), "thr_1")
	if err != nil {
		t.Fatalf("ListMessages large response: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != largeContent {
		t.Fatalf("large message was truncated: count=%d len=%d", len(messages), len(messages[0].Content))
	}
}

func TestClientRejectsSemanticFailureInsideHTTP200(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"capability_unavailable"}`))
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, "hop_test", upstream.Client())
	_, _, err := client.ListProjects(context.Background())
	if err == nil {
		t.Fatal("HTTP 200 with ok=false must fail")
	}
	upErr, ok := err.(*UpstreamError)
	if !ok || upErr.Code != "capability_unavailable" {
		t.Fatalf("unexpected semantic error: %T %#v", err, err)
	}
}

func TestNewClientAppliesBoundedTimeoutWhenHTTPClientHasNone(t *testing.T) {
	client := NewClient("https://api.hoplite.sh", "hop_test", &http.Client{})
	if client.httpClient.Timeout <= 0 || client.httpClient.Timeout > 30*time.Second {
		t.Fatalf("timeout = %v, want bounded <= 30s", client.httpClient.Timeout)
	}
}

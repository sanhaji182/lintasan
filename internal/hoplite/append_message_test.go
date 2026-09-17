package hoplite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAppendMessageUsesThreadUpdateEndpoint(t *testing.T) {
	const key = "hop_test_secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/threads/thr_1/messages" {
			t.Fatalf("request = %s %s, want POST /api/threads/thr_1/messages", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != key {
			t.Fatalf("X-Api-Key = %q, want test key", got)
		}
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		metadata, _ := got["metadata"].(map[string]any)
		if got["content"] != "Continue the work" || metadata["model"] != "claude-fable-5-1" {
			t.Fatalf("unexpected payload: %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true,"message":{"id":"msg_2"},"queued":true,"run":{"id":"run_2","status":"queued"}}`))
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, key, upstream.Client())
	result, meta, err := client.AppendMessage(context.Background(), "thr_1", AppendMessageRequest{
		Content:  "Continue the work",
		Metadata: MessageMetadata{Model: "claude-fable-5-1"},
	})
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if meta.StatusCode != http.StatusCreated || result.Message == nil || result.Message.ID != "msg_2" || !result.Queued {
		t.Fatalf("meta/result = %#v/%#v", meta, result)
	}
}

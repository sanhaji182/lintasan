package hoplite

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientRetriesTransientInvalidAPIKey(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_api_key"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_retry","projectId":"proj_1","status":"queued"}}`))
	}))
	defer upstream.Close()

	client := NewClient(upstream.URL, "hop_test", upstream.Client())
	result, _, err := client.CreateThread(context.Background(), CreateThreadRequest{ProjectID: "proj_1", Prompt: "test"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if result.Thread.ID != "thr_retry" || attempts.Load() != 3 {
		t.Fatalf("thread=%q attempts=%d", result.Thread.ID, attempts.Load())
	}
}

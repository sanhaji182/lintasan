package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleAnalyticsStream_Once(t *testing.T) {
	s := newRESTTestServer(t)

	// Seed one request_log
	_, err := s.db.Conn().Exec(`
		INSERT INTO request_logs (id, connection_id, provider, model, status, input_tokens, output_tokens, latency_ms, cached, created_at)
		VALUES ('req-1', 'conn-1', 'openai', 'gpt-4o', 200, 10, 20, 150.0, 1, datetime('now'))
	`)
	if err != nil {
		t.Fatalf("insert request_logs: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/analytics/stream?once=true", nil)
	rec := httptest.NewRecorder()

	s.handleAnalyticsStream(rec, req)

	res := rec.Result()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q; want text/event-stream", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `data: {"status":"connected"}`) {
		t.Errorf("missing handshake; got %q", body)
	}
	if !strings.Contains(body, "event: stats") {
		t.Errorf("missing event: stats; got %q", body)
	}
	if !strings.Contains(body, `"total_requests":1`) {
		t.Errorf("expected total_requests:1 in payload; got %q", body)
	}
	if !strings.Contains(body, `"cached_hits":1`) {
		t.Errorf("expected cached_hits:1 in payload; got %q", body)
	}
}

func TestHandleAnalyticsStream_ContextCancellation(t *testing.T) {
	s := newRESTTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/analytics/stream", nil).WithContext(ctx)

	flusher := &testFlusher{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}, 1)}

	doneCh := make(chan struct{})
	go func() {
		s.handleAnalyticsStream(flusher, req)
		close(doneCh)
	}()

	// Wait for initial flush
	select {
	case <-flusher.flushed:
	case <-time.After(500 * time.Millisecond):
	}

	// Cancel context to simulate client disconnect
	cancel()

	select {
	case <-doneCh:
		// Cleanly exited loop on context cancel
	case <-time.After(1 * time.Second):
		t.Fatal("handleAnalyticsStream did not terminate upon context cancellation")
	}
}

type testFlusher struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func (f *testFlusher) Flush() {
	select {
	case f.flushed <- struct{}{}:
	default:
	}
}

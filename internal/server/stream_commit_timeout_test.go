package server

// stream_commit_timeout_test.go — the unbounded non-streaming read.
//
// Found while live-verifying the streaming failover: a non-streaming Qoder request
// against an exhausted pool returned HTTP 200 and then nothing, and the request sat
// until the CLIENT gave up — 120,002 ms recorded as "context canceled". The server
// would have waited longer. Qoder produces this shape when its account pool is spent.
//
// These tests pin the bound and its two failure modes, because the interesting part is
// not that a timeout exists but that the blocked goroutine cannot outlive it.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// blockingBody never produces a byte until it is closed, then unblocks with an error.
// This is the stalled-upstream shape: the Read hangs and only Close releases it.
type blockingBody struct {
	released chan struct{}
	closed   bool
}

func newBlockingBody() *blockingBody {
	return &blockingBody{released: make(chan struct{})}
}

func (b *blockingBody) Read(p []byte) (int, error) {
	<-b.released
	return 0, io.ErrUnexpectedEOF
}

func (b *blockingBody) Close() error {
	if !b.closed {
		b.closed = true
		close(b.released)
	}
	return nil
}

// TestReadUpstreamBodyBoundedTimesOutAndReleases: the read gives up, the caller gets an
// error rather than a hang, and closing the body releases the blocked Read.
func TestReadUpstreamBodyBoundedTimesOutAndReleases(t *testing.T) {
	h := newTestProxyHandler(t)
	body := newBlockingBody()

	start := time.Now()
	_, err := h.readUpstreamBodyBounded(context.Background(), body, 150*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a stalled body must produce an error, not a hang")
	}
	if !strings.Contains(err.Error(), "no body within") {
		t.Errorf("error should name the condition, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout took %s; the bound did not apply", elapsed)
	}
	if !body.closed {
		t.Error("the body must be closed, or the blocked Read is never released and the goroutine leaks")
	}
}

// TestReadUpstreamBodyBoundedReturnsBodyOnSuccess: the healthy path is unchanged.
func TestReadUpstreamBodyBoundedReturnsBodyOnSuccess(t *testing.T) {
	h := newTestProxyHandler(t)
	body := io.NopCloser(strings.NewReader("hello"))

	got, err := h.readUpstreamBodyBounded(context.Background(), body, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", string(got), "hello")
	}
}

// TestReadUpstreamBodyBoundedHonoursParentCancel: a client that disconnects must not be
// held for the full timeout, and the error must be the parent's so the caller can tell
// a cancelled client from a stalled upstream.
func TestReadUpstreamBodyBoundedHonoursParentCancel(t *testing.T) {
	h := newTestProxyHandler(t)
	body := newBlockingBody()
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := h.readUpstreamBodyBounded(ctx, body, 30*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error after the parent was cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should be the parent's cancellation, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("waited %s after the client cancelled; the parent must win over the timeout", elapsed)
	}
}

// TestNonStreamReadTimeoutIsConfigurable keeps the default from becoming the only
// option: an operator must be able to move it without a rebuild, and a bogus value must
// fall back rather than disable the bound.
func TestNonStreamReadTimeoutIsConfiguredOrDefault(t *testing.T) {
	h := newTestProxyHandler(t)
	if got := h.qoderNonStreamReadTimeout(); got != 180*time.Second {
		t.Errorf("default = %s, want 180s", got)
	}

	h2 := newTestProxyHandlerWithSettings(t, map[string]string{
		"qoder_nonstream_read_timeout_seconds": "45",
	})
	if got := h2.qoderNonStreamReadTimeout(); got != 45*time.Second {
		t.Errorf("configured = %s, want 45s", got)
	}

	// A non-numeric or non-positive value must not remove the bound.
	h3 := newTestProxyHandlerWithSettings(t, map[string]string{
		"qoder_nonstream_read_timeout_seconds": "not-a-number",
	})
	if got := h3.qoderNonStreamReadTimeout(); got != 180*time.Second {
		t.Errorf("bogus value = %s, want the 180s default (never unbounded)", got)
	}
}

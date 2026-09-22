package qoder

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// silentReader blocks forever without producing data, the way a queued upstream
// behaves after accepting the request.
type silentReader struct {
	closed chan struct{}
}

func newSilentReader() *silentReader { return &silentReader{closed: make(chan struct{})} }

func (s *silentReader) Read(p []byte) (int, error) {
	<-s.closed
	return 0, io.EOF
}

// TestIdleTimeoutDetectsSilentStream is the regression test for the hang that
// this file exists to prevent.
//
// Measured behaviour: upstream returned HTTP 200, opened the stream, and then
// produced nothing for 150 seconds. A plain blocking read reports that as the
// caller's own deadline expiring, which tells an operator nothing — it is
// indistinguishable from a dead connection or a merely slow model. The idle
// deadline converts it into a typed, retryable diagnosis.
func TestIdleTimeoutDetectsSilentStream(t *testing.T) {
	sr := newSilentReader()
	defer close(sr.closed)

	start := time.Now()
	_, err := ConsumeStreamWithIdleTimeout(context.Background(), sr, "auto",
		80*time.Millisecond, 50*time.Millisecond, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a stream that produced nothing must not be reported as a clean completion")
	}
	if !errors.Is(err, ErrStreamStalled) {
		t.Fatalf("expected ErrStreamStalled, got: %v", err)
	}
	// It must fire on the idle window, not on any outer deadline — the whole point
	// is to be much faster than the caller giving up.
	if elapsed > 2*time.Second {
		t.Errorf("stall took %v to detect; should be near the idle window", elapsed)
	}
}

// TestFirstByteTimeoutIsMoreGenerous checks that the initial allowance is used for
// the first frame and the shorter one afterwards, since a queued model takes
// longer to start than to continue.
func TestFirstByteTimeoutIsMoreGenerous(t *testing.T) {
	sr := newSilentReader()
	defer close(sr.closed)

	start := time.Now()
	_, err := ConsumeStreamWithIdleTimeout(context.Background(), sr, "auto",
		300*time.Millisecond, 40*time.Millisecond, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrStreamStalled) {
		t.Fatalf("expected ErrStreamStalled, got: %v", err)
	}
	// Fired on the FIRST-byte allowance, so it must be beyond the idle window.
	if elapsed < 250*time.Millisecond {
		t.Errorf("stalled after %v, which is the idle window not the first-byte allowance", elapsed)
	}
}

// TestActivityResetsIdleWindow proves a healthy slow stream is not killed. A model
// that takes a while but keeps producing must survive, or the timeout would break
// long generations.
func TestActivityResetsIdleWindow(t *testing.T) {
	frame := sseLine(`{"choices":[{"delta":{"content":"x"}}]}`, 200)

	// Produce a frame every 30ms for ~200ms: far longer than the 60ms idle window,
	// but never idle for that long.
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < 6; i++ {
			if _, err := pw.Write([]byte(frame + "\n\n")); err != nil {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		pw.Write([]byte("data:[DONE]\n\n"))
	}()

	var got strings.Builder
	outcome, err := ConsumeStreamWithIdleTimeout(context.Background(), pr, "auto",
		200*time.Millisecond, 60*time.Millisecond, func(d StreamDelta) error {
			got.WriteString(d.Content)
			return nil
		})
	if err != nil {
		t.Fatalf("a continuously-active stream must not stall: %v", err)
	}
	if got.String() != "xxxxxx" {
		t.Errorf("content = %q, want xxxxxx", got.String())
	}
	if !outcome.HadContent {
		t.Error("HadContent should be true")
	}
}

// TestIdleTimeoutLeavesHealthyStreamAlone: a stream that answers immediately must
// complete normally and without waiting for any deadline.
func TestIdleTimeoutLeavesHealthyStreamAlone(t *testing.T) {
	stream := sseLine(`{"choices":[{"delta":{"role":"assistant","content":"PONG"}}]}`, 200) +
		"data:[DONE]\n\n"

	start := time.Now()
	var got strings.Builder
	_, err := ConsumeStreamWithIdleTimeout(context.Background(), strings.NewReader(stream), "auto",
		time.Second, time.Second, func(d StreamDelta) error {
			got.WriteString(d.Content)
			return nil
		})
	if err != nil {
		t.Fatalf("healthy stream failed: %v", err)
	}
	if got.String() != "PONG" {
		t.Errorf("content = %q, want PONG", got.String())
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("healthy stream took %v; it should not wait on the idle timer", elapsed)
	}
}

// TestStallPreemptsContextDeadline confirms the stall is reported as a stall even
// when the caller also has a deadline. The distinction matters: one means "upstream
// went quiet" and is worth retrying elsewhere, the other means "the client gave
// up".
func TestStallPreemptsContextDeadline(t *testing.T) {
	sr := newSilentReader()
	defer close(sr.closed)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ConsumeStreamWithIdleTimeout(ctx, sr, "auto", 40*time.Millisecond, 40*time.Millisecond, nil)
	if !errors.Is(err, ErrStreamStalled) {
		t.Fatalf("expected the idle window to win over a longer context deadline, got: %v", err)
	}
}

// TestStreamStillSurfacesProtocolErrors verifies the liveness wrapper does not
// mask the protocol-level failures it wraps. A refusal must still arrive as an
// UpstreamError, not as a stall.
func TestStreamStillSurfacesProtocolErrors(t *testing.T) {
	stream := sseLine(`{"code":"105","message":"Login expired"}`, 403)

	_, err := ConsumeStreamWithIdleTimeout(context.Background(), strings.NewReader(stream), "auto",
		time.Second, time.Second, nil)
	if err == nil {
		t.Fatal("expected the refusal to surface")
	}
	var ue *UpstreamError
	if !errors.As(err, &ue) || !ue.IsLoginExpired() {
		t.Fatalf("expected the credential refusal, got: %v", err)
	}
	if errors.Is(err, ErrStreamStalled) {
		t.Error("a protocol refusal must not be reported as a stall")
	}
}

// TestDescribeStreamErrorKeepsRetryableDistinct pins the operator-facing
// classification. Retryable vs not is the difference between a router that tries
// another credential and one that fails the request.
func TestDescribeStreamErrorKeepsRetryableDistinct(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		wantKind      string
		wantRetryable bool
	}{
		{"stalled", ErrStreamStalled, "upstream_stalled", true},
		{"queue", &UpstreamError{Status: 403, Code: QueueErrorCode, Queued: true, RetryAfterSeconds: 30}, "upstream_queue", true},
		{"credential", &UpstreamError{Status: 403, Code: LoginExpiredCode}, "credential_cooldown", true},
		{"deadline", context.DeadlineExceeded, "deadline_exceeded", false},
		{"cancelled", context.Canceled, "client_cancelled", false},
		{"other upstream", &UpstreamError{Status: 400, Code: "400", Message: "bad request"}, "qoder_upstream_error", false},
	}
	for _, c := range cases {
		msg, kind, retryable := DescribeStreamError(c.err)
		if kind != c.wantKind {
			t.Errorf("%s: kind = %q, want %q", c.name, kind, c.wantKind)
		}
		if retryable != c.wantRetryable {
			t.Errorf("%s: retryable = %v, want %v", c.name, retryable, c.wantRetryable)
		}
		if msg == "" {
			t.Errorf("%s: empty message", c.name)
		}
	}
}

// TestDescribeStreamErrorNilIsSafe keeps a nil error from panicking a caller that
// asks for a description defensively.
func TestDescribeStreamErrorNilIsSafe(t *testing.T) {
	msg, kind, retryable := DescribeStreamError(nil)
	if msg != "" || kind != "" || retryable {
		t.Errorf("nil error should describe as empty, got (%q, %q, %v)", msg, kind, retryable)
	}
}

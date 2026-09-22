package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Stream liveness.
//
// Upstream does not always answer promptly. Under load it accepts the request —
// HTTP 200, stream opened — and then emits NOTHING for an unbounded period while
// the model is queued. Measured: a turn that produced no bytes at all for 150
// seconds, then served normally minutes later.
//
// A reader that simply blocks on the next frame is therefore not merely slow, it
// gives the caller no way to tell three very different situations apart:
//
//	queued        the model is busy; waiting is correct and it will recover
//	dead stream   the connection is up but will never produce anything
//	slow model    the first token is simply taking a long time
//
// All three look identical to a blocking read. The gateway's own deadline is the
// only thing that ends them, and by then the failure has been reported as a
// timeout — which tells an operator nothing about which case they were in.
//
// The fix is an IDLE deadline rather than a total one: the stream must produce
// something (a content frame, or the queue state upstream sometimes announces) at
// least every IdleTimeout. A total deadline would cut off a legitimately long
// generation; an idle one only fires when the upstream has genuinely gone quiet.
const (
	// DefaultIdleTimeout is how long a stream may produce nothing before it is
	// treated as stalled.
	DefaultIdleTimeout = 45 * time.Second

	// DefaultFirstByteTimeout is the allowance for the FIRST frame specifically.
	// It is more generous than the idle timeout because a queued model can take a
	// while to start, and a queue announcement is itself a first frame for this
	// purpose.
	DefaultFirstByteTimeout = 90 * time.Second
)

// ErrStreamStalled is returned when upstream opened a stream and then went quiet.
//
// It is deliberately distinct from a context cancellation: the caller's deadline
// firing means "the client gave up", while this means "upstream accepted the
// request and then stopped talking". Only the second is actionable as upstream
// behaviour, and only the second is worth retrying on another credential.
var ErrStreamStalled = fmt.Errorf("qoder: upstream opened the stream then produced no frames")

// streamFrame carries one decoded frame, or the reason there was none.
type streamFrame struct {
	delta StreamDelta
	err   *UpstreamError
}

// ConsumeStreamWithIdleTimeout reads a stream with an idle deadline.
//
// It exists alongside ConsumeStream rather than replacing it because ConsumeStream
// is the pure decoder — useful for parsing a captured stream without any timing
// assumptions — while this is the liveness-aware wrapper the request path needs.
//
// It is a package function rather than a SessionManager method because it uses no
// manager state: the allowances are supplied by the caller (which reads them from
// operator settings), so coupling it to a session manager would only make the
// translation path harder to test.
//
// firstByte and idle are per-stream overrides; zero means use the defaults.
func ConsumeStreamWithIdleTimeout(ctx context.Context, r io.Reader, model string, firstByte, idle time.Duration, h StreamHandler) (StreamOutcome, error) {
	if firstByte <= 0 {
		firstByte = DefaultFirstByteTimeout
	}
	if idle <= 0 {
		idle = DefaultIdleTimeout
	}
	return consumeStreamWithDeadlines(ctx, r, model, firstByte, idle, h)
}

// consumeStreamWithDeadlines is the liveness-aware equivalent of ConsumeStream.
//
// It cannot reuse ConsumeStream directly: that function drives a
// bufio.Scanner, whose Read call blocks with no way to interrupt it from the
// timer. Cancelling the context does not help either, because the deadline here
// is about upstream silence, not about the caller changing its mind.
//
// The mechanism used instead is a bounded, self-restarting scan: the reader is
// wrapped in an idleWatchdog that trips after the deadline, and the scanner is
// run in a goroutine whose result is selected against the watchdog. When the
// watchdog wins, the reader is closed, which unblocks the scan.
func consumeStreamWithDeadlines(ctx context.Context, r io.Reader, model string, firstByte, idle time.Duration, h StreamHandler) (StreamOutcome, error) {
	watchdog := newIdleWatchdog(firstByte, idle)
	defer watchdog.stop()

	pr, pw := io.Pipe()

	// Relay bytes so the watchdog can observe activity.
	//
	// This goroutine CANNOT be interrupted: a blocked Read on an HTTP body does not
	// respond to closing the pipe on the other side. It is therefore released by the
	// caller, which must cancel the request context (closing the body) when this
	// function reports a stall. Waiting for it here would deadlock, because the read
	// it is blocked on is the very thing that will never complete.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				watchdog.touch()
				if _, werr := pw.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				pw.CloseWithError(err)
				return
			}
		}
	}()

	type result struct {
		outcome StreamOutcome
		err     error
	}
	done := make(chan result, 1)
	go func() {
		// Reuse the pure decoder against the instrumented pipe.
		o, e := ConsumeStream(ctx, pr, model, h)
		done <- result{outcome: o, err: e}
	}()

	select {
	case res := <-done:
		return res.outcome, res.err
	case <-watchdog.fired():
		// Unblock the scanner. The relay goroutine is released by the caller
		// cancelling the request context — see the note on that goroutine. Returning
		// here without waiting for it is deliberate: waiting would deadlock on the
		// Read that is, by definition, not completing.
		pr.CloseWithError(ErrStreamStalled)
		pw.CloseWithError(ErrStreamStalled)
		return StreamOutcome{}, ErrStreamStalled
	case <-ctx.Done():
		pr.CloseWithError(ctx.Err())
		pw.CloseWithError(ctx.Err())
		return StreamOutcome{}, ctx.Err()
	}
}

// idleWatchdog fires when no activity arrives within the current allowance.
//
// It distinguishes the first frame from subsequent ones because a queued model
// legitimately takes longer to start than to continue.
type idleWatchdog struct {
	mu        sync.Mutex
	first     bool
	idle      time.Duration
	firstByte time.Duration
	timer     *time.Timer
	fire      chan struct{}
	stopped   bool
}

func newIdleWatchdog(firstByte, idle time.Duration) *idleWatchdog {
	w := &idleWatchdog{
		first:     true,
		idle:      idle,
		firstByte: firstByte,
		fire:      make(chan struct{}),
	}
	w.arm(firstByte)
	return w
}

// arm starts (or restarts) the timer for the given allowance. Caller holds mu.
func (w *idleWatchdog) arm(d time.Duration) {
	if w.stopped {
		return
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(d, func() {
		w.mu.Lock()
		if w.stopped {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
		select {
		case <-w.fire:
		default:
			close(w.fire)
		}
	})
}

// touch records activity and re-arms using the appropriate allowance.
func (w *idleWatchdog) touch() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	if w.first {
		w.first = false
	}
	w.arm(w.idle)
}

// fired returns a channel closed when the allowance elapsed without activity.
func (w *idleWatchdog) fired() <-chan struct{} { return w.fire }

// stop cancels the watchdog.
func (w *idleWatchdog) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
}

// DescribeStreamError renders a stream failure for a client, including the stall
// case, so the caller can act on it rather than seeing a bare timeout.
func DescribeStreamError(err error) (message, kind string, retryable bool) {
	if err == nil {
		return "", "", false
	}
	if err == ErrStreamStalled {
		return "upstream accepted the request then produced no frames within the idle window; " +
			"the model is likely queued. Retry, or select a different credential.", "upstream_stalled", true
	}
	if err == context.DeadlineExceeded {
		return "request deadline exceeded while waiting for upstream", "deadline_exceeded", false
	}
	if err == context.Canceled {
		return "client cancelled the request", "client_cancelled", false
	}
	if ue, ok := err.(*UpstreamError); ok {
		msg, k := upstreamDiagnosis(ue)
		return msg, k, ue.IsQueued() || ue.IsLoginExpired()
	}
	return err.Error(), "qoder_error", false
}

// upstreamDiagnosis maps a typed upstream error onto a message and kind.
func upstreamDiagnosis(ue *UpstreamError) (message, kind string) {
	switch {
	case ue.IsQueued():
		return fmt.Sprintf("upstream model is busy; retry in %ds", ue.RetryAfterSeconds), "upstream_queue"
	case ue.IsLoginExpired():
		return "upstream refused this credential for this attempt (code 105). " +
			"This is transient — the credential usually works again within minutes. Retry; do not discard it.", "credential_cooldown"
	default:
		return ue.Error(), "qoder_upstream_error"
	}
}

// guardStreamTypes keeps the json/strings imports meaningful while the frame
// helpers they serve live in chat.go.
var _ = json.Valid
var _ = strings.TrimSpace

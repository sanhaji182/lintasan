// Package experimental hosts the isolation substrate for the Experimental
// provider ecosystem (Foundation Phase 3+). It is deliberately SEPARATE from the
// core proxy/provider routing packages: nothing here is on the Official routing
// path, and the membrane (internal/provider/membrane.go) guarantees the
// production router cannot reach anything that lives here.
//
// Phase 3 scope (E1 process isolation): a provider-agnostic out-of-process
// subprocess harness. It launches a child process, talks to it over stdio,
// enforces per-call timeouts, and contains crashes/hangs/panics so a misbehaving
// Experimental agent can NEVER take down the gateway. It transports bytes and
// knows nothing about ACP, JSON-RPC, or any specific provider — that protocol
// layer is Phase 4, built ON TOP of this.
//
// What this package does NOT do (locked scope): no provider implementation, no
// E2 credential vault, no admission harness, no ACP. Those are later, separately
// scoped. This is the isolation skeleton only.
package experimental

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Errors returned by the harness. They are CONTAINED: a caller on a request
// path treats any of these as "this Experimental candidate is unavailable" and
// moves on — none of them should ever escape as a gateway-level failure.
var (
	// ErrNotStarted is returned when an operation needs a running child but the
	// subprocess was never started (or already stopped).
	ErrNotStarted = errors.New("experimental: subprocess not started")
	// ErrAlreadyStarted is returned by Start on an already-running subprocess.
	ErrAlreadyStarted = errors.New("experimental: subprocess already started")
	// ErrTimeout is returned when a Request exceeds its deadline. The child is
	// considered unhealthy after a timeout (it may be killed by Stop).
	ErrTimeout = errors.New("experimental: request timed out")
	// ErrChildExited is returned when the child process has exited (crash,
	// non-zero exit, or clean shutdown) and can no longer serve requests.
	ErrChildExited = errors.New("experimental: child process exited")
)

// Config describes how to launch and bound an Experimental subprocess. It is
// intentionally minimal and protocol-agnostic.
type Config struct {
	// Name is a human label for logs/metrics (e.g. "exp-codex"). Not a secret.
	Name string
	// Path is the executable to launch (e.g. an agent CLI). Required.
	Path string
	// Args are the launch arguments.
	Args []string
	// Env is the child environment. The harness passes EXACTLY this (it does not
	// inherit the parent env by default), so a caller controls precisely what the
	// child can see — the basis for credential isolation (E2 builds on this).
	Env []string
	// Dir is the working directory for the child (optional).
	Dir string
	// StartTimeout bounds how long Start waits for the process to come up.
	// Zero uses defaultStartTimeout.
	StartTimeout time.Duration
	// RequestTimeout is the default per-request deadline when a call does not
	// supply its own context deadline. Zero uses defaultRequestTimeout.
	RequestTimeout time.Duration
	// StopTimeout bounds graceful shutdown before the child is force-killed.
	// Zero uses defaultStopTimeout.
	StopTimeout time.Duration
}

const (
	defaultStartTimeout   = 10 * time.Second
	defaultRequestTimeout = 60 * time.Second
	defaultStopTimeout    = 5 * time.Second
)

// Subprocess is an out-of-process, isolated child used to host an Experimental
// agent. It is safe for concurrent callers: requests are serialized through an
// internal mutex (one in-flight request per child; pool multiple Subprocesses
// for concurrency). A crash/hang/panic in the child is contained — it surfaces
// as a CONTAINED error, never as a panic in the gateway.
type Subprocess struct {
	cfg Config

	mu      sync.Mutex // serializes Start/Stop/Request and guards the fields below
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	started bool
	exited  bool
	exitErr error
	done    chan struct{} // closed by the reaper goroutine when the child exits
}

// New returns an unstarted Subprocess for cfg. Call Start before Request.
func New(cfg Config) *Subprocess {
	if cfg.StartTimeout <= 0 {
		cfg.StartTimeout = defaultStartTimeout
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.StopTimeout <= 0 {
		cfg.StopTimeout = defaultStopTimeout
	}
	return &Subprocess{cfg: cfg}
}

// Name returns the configured label.
func (s *Subprocess) Name() string { return s.cfg.Name }

// Running reports whether the child is started and has not exited.
func (s *Subprocess) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started && !s.exited
}

// Start launches the child process and wires stdio. It is idempotent-guarded:
// starting an already-started subprocess returns ErrAlreadyStarted. Any failure
// to launch is returned as a contained error (the gateway decides to skip this
// Experimental provider; it does not crash).
func (s *Subprocess) Start(ctx context.Context) (err error) {
	// Harness-side panic containment: even a programming error in setup must not
	// propagate to the caller's goroutine as a panic.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("experimental: panic during Start: %v", r)
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return ErrAlreadyStarted
	}
	if s.cfg.Path == "" {
		return errors.New("experimental: Config.Path is required")
	}

	startCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		startCtx, cancel = context.WithTimeout(ctx, s.cfg.StartTimeout)
		defer cancel()
	}

	// Note: we deliberately do NOT bind the child's lifetime to startCtx (that
	// would kill it when Start returns). The child lives until Stop. startCtx
	// only bounds the launch handshake.
	cmd := exec.Command(s.cfg.Path, s.cfg.Args...)
	cmd.Env = s.cfg.Env // EXACTLY this env — no implicit inheritance
	cmd.Dir = s.cfg.Dir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("experimental: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("experimental: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("experimental: launch %q: %w", s.cfg.Path, err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.stdout = bufio.NewReader(stdout)
	s.started = true
	s.exited = false
	s.exitErr = nil
	s.done = make(chan struct{})

	// Reap the child in the background so Running()/exit state is accurate and
	// no zombie lingers. The Wait result is recorded as a contained exitErr, and
	// `done` is closed exactly once so Stop can wait without polling.
	doneCh := s.done
	go func() {
		werr := cmd.Wait()
		s.mu.Lock()
		s.exited = true
		s.exitErr = werr
		s.mu.Unlock()
		close(doneCh)
	}()

	// Respect a launch deadline: if the context is already done, stop the child.
	select {
	case <-startCtx.Done():
		// Launch raced past its deadline; tear down so we don't leak a process.
		_ = s.stopLocked()
		return fmt.Errorf("experimental: start deadline: %w", startCtx.Err())
	default:
	}
	return nil
}

// Request writes one newline-delimited message to the child and reads one
// newline-delimited response, bounded by the call's context deadline (or the
// configured RequestTimeout). It serializes concurrent callers. Every failure
// mode — timeout, child exit, write/read error, panic — is returned as a
// CONTAINED error; the child is never allowed to hang or crash the caller.
//
// This is a protocol-agnostic byte transport: it does not parse the payload.
// The ACP/JSON-RPC framing (Phase 4) is layered on top of this primitive.
//
// Request is the SYNCHRONOUS request/response primitive: it suits exchanges
// where exactly one response follows one request (initialize, session/new,
// shutdown). For a streaming exchange where the agent emits an interleaved
// burst of notifications + server→client requests before the terminal response
// (the ACP prompt turn), use the WriteLine + ReadLine primitives instead, which
// let a higher-level demux loop drive the frame stream. Request is now defined
// in terms of those two halves, so its behavior (combined-lock write-then-read,
// same timeout/containment) is byte-for-byte unchanged.
func (s *Subprocess) Request(ctx context.Context, line []byte) (resp []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("experimental: panic during Request: %v", r)
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	if werr := s.writeLineLocked(line); werr != nil {
		return nil, werr
	}
	return s.readLineLocked(ctx)
}

// WriteLine writes one newline-delimited message to the child. It is the write
// half of Request, exposed so a streaming protocol loop (the ACP prompt-turn
// demux) can write a request or a reply and then read multiple frames. Failures
// (not-started, child-exited, write error, panic) are CONTAINED errors.
//
// SAFETY: WriteLine and ReadLine each take the subprocess mutex independently,
// so callers MUST drive a single logical exchange sequentially (one in-flight
// turn per child — pool multiple Subprocesses for concurrency, per the E1
// contract). Concurrent interleaving of WriteLine/ReadLine across goroutines on
// the same child is a caller error.
func (s *Subprocess) WriteLine(ctx context.Context, line []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("experimental: panic during WriteLine: %v", r)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLineLocked(line)
}

// ReadLine reads one newline-delimited frame from the child, bounded by the
// call's context deadline (or the configured RequestTimeout). It is the read
// half of Request. Use it in a loop to drain a stream of frames (notifications
// + server→client requests) until the terminal response arrives.
//
// CONTAINMENT CONTRACT (unchanged from Request): the read runs off-goroutine so
// a hung child cannot block past the deadline; on timeout/error the child is
// considered UNHEALTHY and the caller MUST Stop() it (a lingering read goroutine
// is reaped when Stop closes stdin / kills the process and stdout hits EOF).
// Never start a second ReadLine on a child whose prior ReadLine timed out
// without Stopping it first — two concurrent reads on one stdout is a caller
// error.
func (s *Subprocess) ReadLine(ctx context.Context) (resp []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("experimental: panic during ReadLine: %v", r)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLineLocked(ctx)
}

// writeLineLocked is the unlocked write half; the caller holds s.mu.
func (s *Subprocess) writeLineLocked(line []byte) error {
	if !s.started {
		return ErrNotStarted
	}
	if s.exited {
		return fmt.Errorf("%w: %v", ErrChildExited, s.exitErr)
	}
	if _, werr := s.stdin.Write(append(line, '\n')); werr != nil {
		return fmt.Errorf("experimental: write: %w", werr)
	}
	return nil
}

// readLineLocked is the unlocked read half; the caller holds s.mu. Off-goroutine
// read + select on the deadline so a hung child is bounded and contained.
func (s *Subprocess) readLineLocked(ctx context.Context) ([]byte, error) {
	if !s.started {
		return nil, ErrNotStarted
	}
	if s.exited {
		return nil, fmt.Errorf("%w: %v", ErrChildExited, s.exitErr)
	}

	reqCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, s.cfg.RequestTimeout)
		defer cancel()
	}

	// Read one line with a deadline, off-goroutine so a hung child cannot block
	// us past the timeout. The reader goroutine may outlive a timeout; that is
	// acceptable because a timed-out child is considered unhealthy and will be
	// Stopped (killed) by the caller/pool.
	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		data, rerr := s.stdout.ReadBytes('\n')
		ch <- readResult{data: data, err: rerr}
	}()

	select {
	case <-reqCtx.Done():
		return nil, fmt.Errorf("%w: %v", ErrTimeout, reqCtx.Err())
	case rr := <-ch:
		if rr.err != nil && len(rr.data) == 0 {
			// EOF/closed pipe with no data → the child very likely exited.
			return nil, fmt.Errorf("experimental: read: %w", rr.err)
		}
		// Trim the trailing newline if present.
		out := rr.data
		if n := len(out); n > 0 && out[n-1] == '\n' {
			out = out[:n-1]
		}
		return out, nil
	}
}

// Stop terminates the child gracefully (close stdin → wait → kill on timeout).
// It is safe to call multiple times and on a never-started subprocess.
func (s *Subprocess) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked()
}

// stopLocked is the unlocked Stop body; the caller holds s.mu. It releases the
// lock while waiting for the child to exit (the reaper goroutine needs the lock
// to record exit state), then re-acquires it before returning so the deferred
// Unlock in Stop is balanced.
func (s *Subprocess) stopLocked() error {
	if !s.started || s.cmd == nil {
		return nil
	}
	// Signal EOF so a well-behaved child can exit cleanly.
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	doneCh := s.done
	proc := s.cmd.Process
	stopTimeout := s.cfg.StopTimeout

	// Release the lock so the reaper goroutine can record exit state + close
	// doneCh. We re-acquire before returning (caller's deferred Unlock balances).
	s.mu.Unlock()
	var killErr error
	if doneCh != nil {
		select {
		case <-doneCh:
			// Clean (or already-dead) exit reaped.
		case <-time.After(stopTimeout):
			// Force-kill a hung child — isolation guarantee: it cannot hang forever.
			if proc != nil {
				killErr = proc.Kill()
			}
			<-doneCh // wait for the reaper to observe the kill (bounded: kill is terminal)
		}
	}
	s.mu.Lock()

	s.started = false
	return killErr
}

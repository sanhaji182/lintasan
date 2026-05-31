package experimental

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// subprocess_test.go — E1 process isolation tests (Foundation Phase 3).
//
// These prove the isolation guarantees with a REAL child process: the test
// binary re-execs itself in one of several "fake child" modes selected by an
// env var (the standard Go subprocess-test pattern). The guarantees:
//   - echo round-trip works (the byte transport is correct);
//   - a HUNG child is contained by the request timeout (ErrTimeout), not a hang;
//   - a CRASHED/exited child surfaces as a contained error, not a gateway crash;
//   - Stop force-kills a child that ignores EOF (it can't hang shutdown forever).

const childModeEnv = "LINTASAN_EXP_TEST_CHILD"

// TestMain dispatches to a fake-child behavior when the harness launches this
// same binary with childModeEnv set; otherwise it runs the normal test suite.
func TestMain(m *testing.M) {
	switch os.Getenv(childModeEnv) {
	case "echo":
		runEchoChild()
		return
	case "hang":
		// Read one line, then sleep far past any test timeout without replying.
		bufio.NewReader(os.Stdin).ReadString('\n')
		time.Sleep(30 * time.Second)
		return
	case "crash":
		// Exit immediately with a non-zero code (simulates a crash on start).
		os.Exit(3)
	case "exit-after-one":
		// Answer one request, then exit — the next request must be contained.
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		fmt.Fprintf(os.Stdout, "got:%s", line) // includes the trailing newline
		os.Exit(0)
	case "ignore-eof":
		// Never exit on stdin EOF; only a kill stops it. Tests Stop's force-kill.
		for {
			time.Sleep(time.Second)
		}
	case "acp-agent":
		runACPAgentChild()
		return
	}
	os.Exit(m.Run())
}

// runACPAgentChild is a scripted fake SPEC-ACP agent for the broker tests. It
// speaks the JSON-RPC lifecycle over stdio:
//
//	initialize      → {protocolVersion: 1, agentInfo}
//	session/new     → {sessionId}
//	session/prompt  → emits a session/update NOTIFICATION (sessionUpdate:
//	                  "agent_message_chunk" with text), then a session/update
//	                  (sessionUpdate:"tool_call", toolCallId="tc-1"), then a
//	                  session/request_permission REQUEST (id), reads the host's
//	                  outcome, then responds to the prompt id with stopReason +
//	                  an echo of the toolCallId + the granted optionId. This lets
//	                  the broker tests assert: stream drain, text accumulation,
//	                  tool-call tracking (identifier fidelity), and permission
//	                  round-trip.
//	shutdown        → {}
func runACPAgentChild() {
	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	writeLine := func(v any) {
		b, _ := json.Marshal(v)
		w.Write(b)
		w.WriteByte('\n')
		w.Flush()
	}

	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			json.Unmarshal([]byte(line), &msg)

			switch msg.Method {
			case "initialize":
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"name": "fake-acp"}}})
			case "session/new":
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"sessionId": "sess-42"}})
			case "session/prompt":
				promptID := msg.ID
				// (a) a text chunk NOTIFICATION — proves the broker drains + accumulates.
				writeLine(map[string]any{"jsonrpc": "2.0", "method": "session/update",
					"params": map[string]any{"sessionId": "sess-42",
						"update": map[string]any{"sessionUpdate": "agent_message_chunk",
							"content": map[string]any{"type": "text", "text": "thinking…"}}}})
				// (b) a tool_call NOTIFICATION (toolCallId tracked verbatim).
				writeLine(map[string]any{"jsonrpc": "2.0", "method": "session/update",
					"params": map[string]any{"sessionId": "sess-42",
						"update": map[string]any{"sessionUpdate": "tool_call",
							"toolCallId": "tc-1", "kind": "execute", "status": "pending"}}})
				// (c) a permission REQUEST (has id) — broker must answer it.
				writeLine(map[string]any{"jsonrpc": "2.0", "id": "perm-1",
					"method": "session/request_permission",
					"params": map[string]any{"sessionId": "sess-42", "toolCallId": "tc-1",
						"options": []map[string]any{
							{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
							{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"}}}})
				// Read the broker's permission reply.
				permLine, _ := r.ReadString('\n')
				var pr struct {
					Result struct {
						Outcome struct {
							Outcome  string `json:"outcome"`
							OptionID string `json:"optionId"`
						} `json:"outcome"`
					} `json:"result"`
				}
				json.Unmarshal([]byte(permLine), &pr)
				// (d) terminal response to the prompt id, echoing the outcome.
				writeLine(map[string]any{"jsonrpc": "2.0", "id": promptID,
					"result": map[string]any{"stopReason": "end_turn",
						"content": json.RawMessage(`{"echoedToolCallId":"tc-1","permission":"` + pr.Result.Outcome.OptionID + `"}`)}})
			case "shutdown":
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{}})
			default:
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"error": map[string]any{"code": -32601, "message": "method not found"}})
			}
		}
		if err != nil {
			return
		}
	}
}

func runEchoChild() {
	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			// Echo back uppercased, newline-terminated.
			fmt.Fprintf(w, "%s\n", strings.ToUpper(strings.TrimRight(line, "\n")))
			w.Flush()
		}
		if err != nil {
			return
		}
	}
}

// childConfig returns a Config that re-execs THIS test binary in the given mode.
func childConfig(t *testing.T, mode string, reqTimeout, stopTimeout time.Duration) Config {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return Config{
		Name:           "test-child-" + mode,
		Path:           exe,
		Env:            append(os.Environ(), childModeEnv+"="+mode),
		RequestTimeout: reqTimeout,
		StopTimeout:    stopTimeout,
	}
}

// TestE1_EchoRoundTrip proves the byte transport works end-to-end with a real
// out-of-process child.
func TestE1_EchoRoundTrip(t *testing.T) {
	s := New(childConfig(t, "echo", 5*time.Second, 2*time.Second))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Stop()

	if !s.Running() {
		t.Fatal("expected Running() true after Start")
	}
	resp, err := s.Request(context.Background(), []byte("hello"))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if string(resp) != "HELLO" {
		t.Fatalf("echo round-trip: got %q want %q", resp, "HELLO")
	}
	// A second request on the same child also works (serialized).
	resp2, err := s.Request(context.Background(), []byte("world"))
	if err != nil {
		t.Fatalf("request 2: %v", err)
	}
	if string(resp2) != "WORLD" {
		t.Fatalf("second round-trip: got %q want %q", resp2, "WORLD")
	}
}

// TestE1_TimeoutIsContained proves a HUNG child is contained by the request
// timeout — the call returns ErrTimeout, it does NOT block the caller forever.
func TestE1_TimeoutIsContained(t *testing.T) {
	s := New(childConfig(t, "hang", 200*time.Millisecond, 1*time.Second))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Stop()

	start := time.Now()
	_, err := s.Request(context.Background(), []byte("please-hang"))
	elapsed := time.Since(start)

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout from a hung child, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout was not enforced promptly: %v", elapsed)
	}
}

// TestE1_CrashOnStartIsContained proves a child that exits immediately surfaces
// as a contained error on the first request, not a gateway crash.
func TestE1_CrashOnStartIsContained(t *testing.T) {
	s := New(childConfig(t, "crash", 1*time.Second, 1*time.Second))
	// Start itself may succeed (launch is fast); the crash is observed by the
	// reaper. Give it a moment, then a request must be contained.
	if err := s.Start(context.Background()); err != nil {
		// Some platforms may report the failure at Start — also acceptable.
		return
	}
	// Allow the reaper to observe the exit.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.Running() {
		time.Sleep(10 * time.Millisecond)
	}
	_, err := s.Request(context.Background(), []byte("anyone-there"))
	if err == nil {
		t.Fatal("expected a contained error from a crashed child, got nil")
	}
}

// TestE1_ExitAfterOneIsContained proves that after a child exits mid-session,
// the next request is contained (no panic, no hang).
func TestE1_ExitAfterOneIsContained(t *testing.T) {
	s := New(childConfig(t, "exit-after-one", 1*time.Second, 1*time.Second))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Stop()

	resp, err := s.Request(context.Background(), []byte("first"))
	if err != nil {
		t.Fatalf("first request should succeed: %v", err)
	}
	if !strings.Contains(string(resp), "got:first") {
		t.Fatalf("unexpected first response: %q", resp)
	}
	// Child has now exited. Let the reaper see it, then the next request must be
	// a contained error.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.Running() {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := s.Request(context.Background(), []byte("second")); err == nil {
		t.Fatal("expected contained error after child exit, got nil")
	}
}

// TestE1_StopForceKillsHungChild proves Stop cannot hang forever: a child that
// ignores stdin EOF is force-killed after StopTimeout.
func TestE1_StopForceKillsHungChild(t *testing.T) {
	s := New(childConfig(t, "ignore-eof", 1*time.Second, 200*time.Millisecond))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	start := time.Now()
	_ = s.Stop()
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("Stop did not force-kill promptly: %v", elapsed)
	}
	if s.Running() {
		t.Fatal("child should not be Running after Stop")
	}
}

// TestE1_LifecycleGuards covers the not-started / double-start / stop-idempotent
// paths.
func TestE1_LifecycleGuards(t *testing.T) {
	s := New(childConfig(t, "echo", 1*time.Second, 1*time.Second))

	// Request before Start → ErrNotStarted.
	if _, err := s.Request(context.Background(), []byte("x")); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("expected ErrNotStarted before Start, got %v", err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Double Start → ErrAlreadyStarted.
	if err := s.Start(context.Background()); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("expected ErrAlreadyStarted, got %v", err)
	}
	// Stop twice is safe.
	if err := s.Stop(); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("second stop should be safe, got %v", err)
	}
}

// TestE1_MissingPath proves a misconfigured launch is a contained error.
func TestE1_MissingPath(t *testing.T) {
	s := New(Config{Name: "no-path"})
	if err := s.Start(context.Background()); err == nil {
		t.Fatal("expected error for empty Path, got nil")
	}
}

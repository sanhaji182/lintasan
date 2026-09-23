package server

// stream_commit_qoder_test.go — the failover that was missing on the Qoder path.
//
// Qoder opens its stream with a role-only chunk and can then refuse the credential
// (code 105) INSIDE that 200. Before the commit gate existed, the role chunk was
// written straight to the client, which committed the response and made the refusal
// terminal: the client got an error frame on a 200 and never a retry, while the
// failover machinery and the "105 is retryable" classification sat unused.
//
// These tests drive the real frame parser and the real writer through the gate, so
// they pin the ordering that makes retry possible rather than a mock's idea of it.

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

// qoderRoleThenRefusal is the exact shape observed live: a role preamble, then the
// refusal, then the stream ends. `statusCodeValue: 403` with code 105 in the body.
const qoderRoleThenRefusal = `data:{"headers":{},"body":"{\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"index\":0}]}","statusCodeValue":200}` + "\n\n" +
	`data:{"headers":{},"body":"{\"code\":\"105\",\"message\":\"Login expired\"}","statusCodeValue":403}` + "\n\n"

// qoderRoleThenContent is a healthy turn: role preamble, then real text.
const qoderRoleThenContent = `data:{"headers":{},"body":"{\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"index\":0}]}","statusCodeValue":200}` + "\n\n" +
	`data:{"headers":{},"body":"{\"choices\":[{\"delta\":{\"content\":\"PONG\"},\"index\":0}]}","statusCodeValue":200}` + "\n\n"

// TestQoderRolePreambleDoesNotCommit is the regression pin. A role-only chunk must
// not reach the client, because reaching the client is what closed the failover.
func TestQoderRolePreambleDoesNotCommit(t *testing.T) {
	h := newTestProxyHandler(t)
	rec := httptest.NewRecorder()
	commit := &streamCommit{}

	buf, _, _, err := h.streamQoderToOpenAICost(
		context.Background(), io.NopCloser(strings.NewReader(qoderRoleThenRefusal)),
		rec, rec, "auto", commit)

	if err == nil {
		t.Fatal("the refusal must be returned to the caller, not swallowed")
	}
	if !commit.Retryable() {
		t.Fatal("a role preamble followed by a refusal must stay retryable — " +
			"otherwise there is no failover, which is the bug this gate fixes")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("nothing may reach the client before the commit, got %q", rec.Body.String())
	}
	if len(buf) != 0 {
		t.Errorf("no user-visible content was produced, got %q", string(buf))
	}
}

// TestQoderRefusalIsNotWrittenByTheStreamer: the diagnosis must be the caller's
// decision, because only the caller knows whether the attempt can be retried.
func TestQoderRefusalIsNotWrittenByTheStreamer(t *testing.T) {
	h := newTestProxyHandler(t)
	rec := httptest.NewRecorder()

	_, _, _, err := h.streamQoderToOpenAICost(
		context.Background(), io.NopCloser(strings.NewReader(qoderRoleThenRefusal)),
		rec, rec, "auto", &streamCommit{})

	if err == nil {
		t.Fatal("expected the refusal to be returned")
	}
	if strings.Contains(rec.Body.String(), "error") {
		t.Errorf("the streamer must not write the error frame itself; got %q", rec.Body.String())
	}
}

// TestQoderHealthyStreamCommitsAndDeliversEverything: a healthy turn is served
// normally through the gate. Content commits, the terminal frame reaches the
// client, and nothing is left held.
//
// Note what this test does NOT expect: the frame's `role:"assistant"` never reaches
// the handler at all, because a delta with only a role is skipped upstream by
// `StreamDelta.IsEmpty()` before any callback. So the gate's job on the Qoder path is
// narrower than "hold the preamble" — it is to hold anything that arrives before the
// first CONTENT, which in practice is reasoning frames (they pass IsEmpty) and any
// keep-alive the frame parser lets through. The refusal shape is still covered:
// TestQoderRolePreambleDoesNotCommit drives exactly the observed live sequence, where
// the role frame is dropped upstream and the failure arrives with nothing written.
func TestQoderHealthyStreamCommitsAndDeliversEverything(t *testing.T) {
	h := newTestProxyHandler(t)
	rec := httptest.NewRecorder()
	commit := &streamCommit{}

	buf, chunks, _, err := h.streamQoderToOpenAICost(
		context.Background(), io.NopCloser(strings.NewReader(qoderRoleThenContent)),
		rec, rec, "auto", commit)

	if err != nil {
		t.Fatalf("healthy stream failed: %v", err)
	}
	if commit.Retryable() {
		t.Error("a stream that produced content must be committed")
	}
	body := rec.Body.String()

	if !strings.Contains(body, `"content":"PONG"`) {
		t.Errorf("content missing; body=%q", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Errorf("terminal sentinel missing; body=%q", body)
	}
	if !strings.Contains(string(buf), "PONG") {
		t.Errorf("streamBuffer must carry the user-visible text, got %q", string(buf))
	}
	if chunks == 0 {
		t.Error("chunk count should have advanced")
	}
}

// TestAbandonDropsTheFailedCandidatesHeldFrames is the correctness half of the
// gate: the held frames belong to the candidate being discarded, so flushing them
// into the next candidate's stream would splice two upstreams together.
func TestAbandonDropsTheFailedCandidatesHeldFrames(t *testing.T) {
	h := newTestProxyHandler(t)
	rec := httptest.NewRecorder()
	commit := &streamCommit{}

	if _, _, _, err := h.streamQoderToOpenAICost(
		context.Background(), io.NopCloser(strings.NewReader(qoderRoleThenRefusal)),
		rec, rec, "auto", commit); err == nil {
		t.Fatal("expected the refusal")
	}

	// The caller decides to retry, so it abandons this attempt.
	commit.Abandon()

	// The next candidate writes its own preamble and content.
	if err := commit.WriteFrame(rec, rec, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"FRESH\"}}]}\n\n")); err != nil {
		t.Fatalf("WriteFrame on the retry: %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, `"role":"assistant"`) {
		t.Errorf("the failed candidate's held frame leaked into the retry; body=%q", body)
	}
	if !strings.Contains(body, "FRESH") {
		t.Errorf("the retry's own frame missing; body=%q", body)
	}
}

// TestDeferFramePassesThroughOnceCommitted keeps the gate from becoming a buffer
// that grows forever on a long healthy stream.
func TestDeferFramePassesThroughOnceCommitted(t *testing.T) {
	rec := httptest.NewRecorder()
	commit := &streamCommit{}
	if err := commit.WriteFrame(rec, rec, []byte("data: first\n\n")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if err := commit.DeferFrame(rec, rec, []byte("data: second\n\n")); err != nil {
		t.Fatalf("DeferFrame: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "second") {
		t.Error("after the commit, DeferFrame must write through rather than hold")
	}
}

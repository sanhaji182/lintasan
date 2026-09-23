package server

// stream_commit_test.go — the buffered-until-first-content streaming path.
//
// The regression these tests exist for: the streaming branch used to commit the
// response before reading any upstream frame, which made every `continue` in the
// candidate loop unreachable. For a provider that reports failure in the status line
// that was harmless, because those failures are handled before the commit. For Qoder
// — which reports a refused credential, a busy model and moderation INSIDE a 200 —
// the in-stream error was logged and returned, so the client got an error frame on a
// 200 and never a retry on another account.
//
// The tests below pin the three states that matter, and they are deliberately about
// the COMMIT rather than about any one provider:
//
//   * nothing written yet  -> an attempt may be discarded (retry possible)
//   * status written       -> too late; report
//   * content written      -> too late; report
//
// Plus the two properties that keep the fix from becoming a regression of its own:
// keep-alives must not commit (or a chatty upstream re-creates the bug), and a
// healthy stream must be byte-identical to what the old code emitted.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// flushableRecorder is an httptest.ResponseRecorder that reports itself as a
// Flusher, so the commit path can be driven without a real TCP server.
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func newFlushable() *flushableRecorder {
	return &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (f *flushableRecorder) Flush() { f.flushes++ }

func (f *flushableRecorder) headerWritten() bool {
	return f.ResponseRecorder.Flushed || f.Code != 0 && f.Code != http.StatusOK || f.Body.Len() > 0 || f.flushes > 0
}

// --- the commit state machine ------------------------------------------------

// TestStreamCommitAllowsRetryBeforeAnyWrite pins the precondition the whole fix
// depends on: while nothing has been sent, the attempt is discardable.
func TestStreamCommitAllowsRetryBeforeAnyWrite(t *testing.T) {
	c := &streamCommit{}
	if !c.Retryable() {
		t.Fatal("a fresh attempt must be retryable — otherwise no failover is possible at all")
	}
	if c.Committing() {
		t.Error("a fresh attempt must not report as committing")
	}
}

// TestStreamCommitHeadersFlushedClosesRetry: sending the status is the point of no
// return. `w.WriteHeader` cannot be undone, so this must report as committed.
func TestStreamCommitHeadersFlushedClosesRetry(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	c.FlushHeaders(w, http.StatusOK)

	if c.Retryable() {
		t.Error("status sent must not be retryable")
	}
	if !c.Committing() {
		t.Error("status sent must report as committing")
	}
}

// TestStreamCommitFrameWriteClosesRetry covers the content case.
func TestStreamCommitFrameWriteClosesRetry(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	c.FlushHeaders(w, http.StatusOK)
	if err := c.WriteFrame(w, w, []byte("data: {\"x\":1}\n\n")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if c.Retryable() {
		t.Error("a written frame must not be retryable")
	}
}

// TestWriteFrameSendsStatusWhenHeadersNotFlushed: a provider that emits a frame
// before any explicit status still needs one on the wire.
func TestWriteFrameSendsStatusWhenHeadersNotFlushed(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	if err := c.WriteFrame(w, w, []byte("data: x\n\n")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if !c.Committing() {
		t.Error("WriteFrame must commit")
	}
	if !strings.Contains(w.Body.String(), "data: x") {
		t.Errorf("frame not written, body=%q", w.Body.String())
	}
}

// --- the pump, and the keep-alive trap ---------------------------------------

// TestPumpCommitsOnFirstContentFrame is the healthy path: content reaches the
// client and the bytes are unchanged.
func TestPumpCommitsOnFirstContentFrame(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	in := "data: {\"a\":1}\n\ndata: {\"b\":2}\n\ndata: [DONE]\n\n"

	got, err := pumpStreamToClient(strings.NewReader(in), w, w, c)
	if err != nil {
		t.Fatalf("pump: %v", err)
	}
	if string(got) != in {
		t.Errorf("collected mismatch:\n got %q\nwant %q", string(got), in)
	}
	if w.Body.String() != in {
		t.Errorf("client bytes mismatch:\n got %q\nwant %q", w.Body.String(), in)
	}
	if c.Retryable() {
		t.Error("a stream that produced content must be committed")
	}
}

// TestPumpKeepAlivesDoNotCommit is the trap that would silently re-create the bug.
//
// An upstream that opens the stream, sends comments or metadata for a while, then
// fails must NOT have committed anything — otherwise the status is on the wire and
// the candidate loop still cannot retry. "Received bytes" is not "told the client
// something".
func TestPumpKeepAlivesDoNotCommit(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	in := ": keep-alive\n\n: keep-alive\n\n"

	if _, err := pumpStreamToClient(strings.NewReader(in), w, w, c); err != nil {
		t.Fatalf("pump: %v", err)
	}
	if !c.Retryable() {
		t.Error("comments/keep-alives must not commit the response")
	}
	if w.Body.Len() != 0 {
		t.Errorf("nothing client-actionable should have been sent, got %q", w.Body.String())
	}
}

// TestPumpDoneSentinelAloneDoesNotCommit: "[DONE]" with no content is an empty
// completion, not a served request.
func TestPumpDoneSentinelAloneDoesNotCommit(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	if _, err := pumpStreamToClient(strings.NewReader("data: [DONE]\n\n"), w, w, c); err != nil {
		t.Fatalf("pump: %v", err)
	}
	if !c.Retryable() {
		t.Error("a bare [DONE] must not commit — no content was served")
	}
}

// TestPumpPartialFrameThenEOFCommits: content that arrives in a trailing partial
// frame must still reach the client rather than being dropped.
func TestPumpPartialFrameThenEOFCommits(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	// No trailing \n\n — the frame is incomplete, then the body ends.
	if _, err := pumpStreamToClient(strings.NewReader("data: {\"tail\":true}"), w, w, c); err != nil {
		t.Fatalf("pump: %v", err)
	}
	if !strings.Contains(w.Body.String(), "tail") {
		t.Errorf("trailing partial frame dropped, body=%q", w.Body.String())
	}
	if c.Retryable() {
		t.Error("a delivered trailing frame must commit")
	}
}

// TestPumpTransportErrorBeforeContentIsRetryable: the failure mode the fix exists
// for on the generic provider path — a connection that dies before producing
// anything must still be able to fall over to another candidate.
func TestPumpTransportErrorBeforeContentIsRetryable(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	boom := errors.New("upstream reset")

	_, err := pumpStreamToClient(io.MultiReader(strings.NewReader(": hi\n\n"), errReader{boom}), w, w, c)
	if !errors.Is(err, boom) {
		t.Fatalf("want the transport error, got %v", err)
	}
	if !c.Retryable() {
		t.Error("a transport failure with no content sent must stay retryable")
	}
}

// TestPumpFrameThenTransportErrorIsNotRetryable: once content is out, the request
// cannot be retried, and the handler must not pretend otherwise.
func TestPumpFrameThenTransportErrorIsNotRetryable(t *testing.T) {
	w := newFlushable()
	c := &streamCommit{}
	boom := errors.New("upstream reset")

	_, err := pumpStreamToClient(io.MultiReader(strings.NewReader("data: {\"a\":1}\n\n"), errReader{boom}), w, w, c)
	if !errors.Is(err, boom) {
		t.Fatalf("want the transport error, got %v", err)
	}
	if c.Retryable() {
		t.Error("content was already delivered; this must not be retryable")
	}
}

// errReader fails with a fixed error instead of EOF.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// closerFunc adapts a function to io.Closer, for asserting the failed attempt's
// upstream body is released before a retry.
type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// --- the decision -------------------------------------------------------------

// TestHandleStreamFailureRetriesWhenNothingWritten is the core behaviour change:
// an in-stream failure with a clean client response hands control back to the
// candidate loop.
//
// The error used is the real one the streaming path produces for a busy model —
// qoder.ErrStreamStalled, which the taxonomy marks retryable. A generic error is
// deliberately NOT used here: the taxonomy decides what is retryable *in principle*
// and this handler decides what is *still possible*, and the test must exercise both
// halves rather than a case where they cannot be told apart.
func TestHandleStreamFailureRetriesWhenNothingWritten(t *testing.T) {
	h := newTestProxyHandler(t)
	w := newFlushable()
	c := &streamCommit{}
	closed := false

	retry := h.handleStreamFailure(streamFailure{
		Err:    qoder.ErrStreamStalled,
		Commit: c,
		Body:   closerFunc(func() error { closed = true; return nil }),
	}, w, w)

	if !retry {
		t.Fatal("a retryable in-stream failure before any write must be retried on another candidate")
	}
	if !closed {
		t.Error("the failed attempt's upstream body must be closed before retrying")
	}
	if w.Body.Len() != 0 {
		t.Errorf("nothing should have been sent to the client, got %q", w.Body.String())
	}
}

// TestHandleStreamFailureDoesNotRetryUnclassifiedError: retryable-in-principle and
// still-possible are separate questions. An unclassified failure must not be walked
// across the whole pool — the client gets the diagnosis immediately.
func TestHandleStreamFailureDoesNotRetryUnclassifiedError(t *testing.T) {
	h := newTestProxyHandler(t)
	w := newFlushable()
	closed := false

	retry := h.handleStreamFailure(streamFailure{
		Err:    errors.New("something unrecognised"),
		Commit: &streamCommit{},
		Body:   closerFunc(func() error { closed = true; return nil }),
	}, w, w)

	if retry {
		t.Fatal("an unclassified error must not consume the rest of the pool")
	}
	if !closed {
		t.Error("the failed attempt's upstream body must still be released")
	}
	if !strings.Contains(w.Body.String(), "error") {
		t.Errorf("client must receive the diagnosis, got %q", w.Body.String())
	}
}

// TestHandleStreamFailureReportsWhenCommitted: after the commit there is no retry
// left, and the client must get the diagnosis rather than a silent truncation.
func TestHandleStreamFailureReportsWhenCommitted(t *testing.T) {
	h := newTestProxyHandler(t)
	w := newFlushable()
	c := &streamCommit{}
	c.MarkBodyWritten()

	retry := h.handleStreamFailure(streamFailure{
		Err:    errors.New("stream stalled"),
		Commit: c,
	}, w, w)

	if retry {
		t.Fatal("a committed response must not be retried")
	}
	body := w.Body.String()
	if !strings.Contains(body, "error") {
		t.Errorf("client must receive a diagnosis frame, got %q", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Errorf("stream must be terminated deliberately, got %q", body)
	}
}

// TestHandleStreamFailureNilErrIsNotAHandledFailure: a clean stream is not a
// failure, and reporting it as one would end every healthy stream early.
func TestHandleStreamFailureNilErrIsNotAHandledFailure(t *testing.T) {
	h := newTestProxyHandler(t)
	w := newFlushable()
	if h.handleStreamFailure(streamFailure{Err: nil, Commit: &streamCommit{}}, w, w) {
		t.Fatal("a nil error must not be treated as a handled failure")
	}
	if w.Body.Len() != 0 {
		t.Errorf("no frame should be sent for a clean stream, got %q", w.Body.String())
	}
}

// TestClassifyStreamFailureKeepsQoderTaxonomy: the retry decision must come from the
// existing classification rather than a second opinion invented here, or the two
// paths would drift.
func TestClassifyStreamFailureKeepsQoderTaxonomy(t *testing.T) {
	// A bare error is not in the retryable taxonomy.
	retryable, kind, detail := classifyStreamFailure(errors.New("anything"))
	if retryable {
		t.Error("an unclassified error must not be retried — that would walk the whole pool")
	}
	if kind == "" || detail == "" {
		t.Errorf("classification must always produce a kind and a detail, got %q/%q", kind, detail)
	}
}

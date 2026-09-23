package server

// stream_commit.go — deferred header commit for the streaming path, so a provider
// that reports failure INSIDE a 200 stream gets the same failover as one that
// reports it in the status line.
//
// WHY THIS EXISTS
//
// The streaming branch used to commit the response before reading a single upstream
// frame:
//
//     w.WriteHeader(resp.StatusCode)
//     ... read frames ...
//
// Once that runs, the status is on the wire and the candidate loop's every
// `continue` is dead code — Go has no way to take it back. That is harmless for a
// provider that reports failure in the status, because those failures are handled
// before the commit. It is fatal for a provider that reports failure *inside* a 200:
//
//   Qoder reports a refused credential, a busy model and content moderation all
//   INSIDE a stream that opened with HTTP 200, so a caller that only inspects the
//   status code sees a healthy stream that never produces content.
//
// On that path the in-stream error was logged and returned. The client got an error
// frame on a 200 and no retry, despite the failover machinery being right there and
// `code 105` being explicitly classified retryable — "7 of 9 credentials served
// normally again after reporting it".
//
// The fix is to defer the commit until the candidate has proven it is serving:
// the status (if not 200) or the first real frame (if 200). Everything before that
// is a failed ATTEMPT, not a response, and an attempt may be discarded.
//
// WHAT IS deliberately NOT DONE
//
// Carrying an attempt across into the next candidate — piping the new upstream's
// content into the stream already opened for the old one — is not attempted. It
// would change the client-visible contract (a stream that silently spans two
// upstreams) and the cost/observability accounting that currently assumes one
// connection per request. A failure after the commit therefore reports itself as it
// always did. See makeStreamFailure, which is where that boundary is enforced.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// streamCommit tracks how far the client response has been committed, which is the
// single fact that decides whether another candidate may be tried.
type streamCommit struct {
	statusWritten bool
	bodyWritten   bool

	// deferred holds SSE frames that arrived before the response was viable —
	// role-only chunks, keep-alives, metadata. They are not written yet because
	// writing would commit, and they are not discarded because a client expects
	// them. They are flushed, in order, immediately before the first frame that
	// does commit.
	deferred []byte
}

// MarkStatusWritten records that the response status is on the wire.
func (c *streamCommit) MarkStatusWritten() { c.statusWritten = true }

// MarkBodyWritten records that client-visible body bytes are on the wire.
func (c *streamCommit) MarkBodyWritten() { c.bodyWritten = true }

// Committing reports whether the client response is already in flight.
//
// This is the read-only view used by the caller to decide whether the normal
// success bookkeeping applies.
func (c *streamCommit) Committing() bool { return c.statusWritten || c.bodyWritten }

// Retryable reports whether discarding this attempt is still possible, i.e. nothing
// has been sent to the client. This is the ONLY condition under which the candidate
// loop may `continue`.
func (c *streamCommit) Retryable() bool { return !c.statusWritten && !c.bodyWritten }

// FlushHeaders sets the SSE headers and sends the response status.
//
// Called only once the attempt is considered viable. From here on a failure cannot
// be retried, which is why this is a single named step rather than a bare
// WriteHeader scattered through the branch.
func (c *streamCommit) FlushHeaders(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(status)
	c.statusWritten = true
}

// WriteFrame writes one client-visible unit and records the commit.
//
// A frame is the minimum amount a streaming client would act on. For an SSE
// gateway that is one `\n\n`-terminated event, so committing on a frame rather than
// on a label means the status can still be revised if upstream sends metadata first.
func (c *streamCommit) WriteFrame(w http.ResponseWriter, flusher http.Flusher, frame []byte) error {
	if len(frame) == 0 {
		return nil
	}
	// A caller that reaches here without having flushed headers (a provider emitting
	// a frame before any explicit status) still needs a status on the wire.
	if !c.statusWritten {
		c.FlushHeaders(w, http.StatusOK)
	}
	if err := c.writeDeferred(w, flusher); err != nil {
		return err
	}
	if _, err := w.Write(frame); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	c.bodyWritten = true
	return nil
}

// DeferFrame holds a frame back until something worth committing on arrives.
//
// This is the streaming flow-control primitive. A provider that opens a stream with
// a role-only chunk, a keep-alive, or a metadata event has told the client nothing
// yet — and if it then fails, the attempt must still be discardable. Holding the
// frame keeps both properties: the attempt stays retryable, and the client still
// receives those frames (they are flushed in order by WriteFrame).
//
// Once committed this is a straight pass-through.
func (c *streamCommit) DeferFrame(w http.ResponseWriter, flusher http.Flusher, frame []byte) error {
	if len(frame) == 0 {
		return nil
	}
	if c.Committing() {
		return c.WriteFrame(w, flusher, frame)
	}
	c.deferred = append(c.deferred, frame...)
	return nil
}

// writeDeferred flushes frames held by DeferFrame, before the committing frame.
func (c *streamCommit) writeDeferred(w http.ResponseWriter, flusher http.Flusher) error {
	if len(c.deferred) == 0 {
		return nil
	}
	held := c.deferred
	c.deferred = nil
	if _, err := w.Write(held); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// Abandon discards everything held for an attempt that is being retried.
//
// Required for correctness of the failover: the held frames belong to the FAILED
// candidate. Flushing them into the next candidate's stream would splice two
// upstreams' output together, and a duplicate role/`id` chunk is exactly the kind of
// malformed stream that is hard to attribute later.
func (c *streamCommit) Abandon() { c.deferred = nil }

// Flush releases everything held and marks the response committed.
//
// The counterpart to Abandon, and the reason DeferFrame is safe: on a SUCCESSFUL
// attempt the held frames are exactly what the client should receive, so the caller
// releases them once the attempt can no longer fail. Without this the gate would
// hold a healthy stream forever — a bug this method exists to close, caught by
// TestQoderHealthyStreamCommitsAndDeliversEverything.
func (c *streamCommit) Flush(w http.ResponseWriter, flusher http.Flusher) error {
	// A stream that never produced a committing frame still needs its status on the
	// wire, or the client sees a connection close with no response at all.
	if !c.statusWritten {
		c.FlushHeaders(w, http.StatusOK)
	}
	if err := c.writeDeferred(w, flusher); err != nil {
		return err
	}
	c.bodyWritten = true
	return nil
}

// pumpStreamToClient copies an already-canonical SSE body to the client, committing
// on the first frame that carries content.
//
// This replaces the previous read-and-write loop for the generic provider path. It
// keeps the streaming behaviour identical for a healthy provider and gains one
// property: a failure that occurs before the first frame is now retryable, because
// nothing has been written.
func pumpStreamToClient(body io.Reader, w http.ResponseWriter, flusher http.Flusher, commit *streamCommit) ([]byte, error) {
	var collected []byte
	buf := make([]byte, 4096)
	var pending []byte

	emit := func(frame []byte) error {
		return commit.WriteFrame(w, flusher, frame)
	}

	for {
		n, er := body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			collected = append(collected, chunk...)

			// Work frame by frame: committing mid-frame would leave the client with
			// a truncated event it cannot parse if the next read fails.
			pending = append(pending, chunk...)
			for {
				idx := indexDoubleNewline(pending)
				if idx < 0 {
					break
				}
				frame := pending[:idx+2]
				pending = pending[idx+2:]

				switch {
				case commit.Committing():
					// Past the gate: everything is passed through verbatim,
					// including the terminal [DONE].
					if werr := commit.WriteFrame(w, flusher, frame); werr != nil {
						return collected, werr
					}
				case hasContent(frame):
					// First client-actionable frame: flush what was held, then this.
					if werr := emit(frame); werr != nil {
						return collected, werr
					}
				default:
					// Not worth committing on — a comment, keep-alive or metadata
					// frame. Hand it to the commit gate, which holds it until a frame
					// that does commit and then flushes it in order.
					if werr := commit.DeferFrame(w, flusher, frame); werr != nil {
						return collected, werr
					}
				}
			}
		}
		if er != nil {
			if er == io.EOF {
				// A trailing partial frame must not have its content dropped.
				if len(pending) > 0 && hasContent(pending) {
					if werr := emit(pending); werr != nil {
						return collected, werr
					}
				}
				return collected, nil
			}
			return collected, er
		}
	}
}

// indexDoubleNewline returns the index of the first "\n\n" in b, or -1.
func indexDoubleNewline(b []byte) int {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '\n' && b[i+1] == '\n' {
			return i
		}
	}
	return -1
}

// hasContent reports whether an SSE frame carries anything a client would render.
//
// Only data-bearing frames count: a comment/keep-alive (`: ping`) or an empty event
// must not commit the response, or an upstream that opens a stream and then fails
// would still be unrecoverable.
func hasContent(frame []byte) bool {
	for _, line := range splitLines(frame) {
		if len(line) == 0 || line[0] == ':' {
			continue
		}
		const p = "data:"
		if len(line) >= len(p) && string(line[:len(p)]) == p {
			rest := trimSpaceBytes(line[len(p):])
			if len(rest) > 0 && !isDoneSentinel(rest) {
				return true
			}
		}
	}
	return false
}

func isDoneSentinel(b []byte) bool { return string(b) == "[DONE]" }

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			line := b[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			out = append(out, line)
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}

func trimSpaceBytes(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 {
		last := b[len(b)-1]
		if last == ' ' || last == '\t' || last == '\r' || last == '\n' {
			b = b[:len(b)-1]
			continue
		}
		break
	}
	return b
}

// streamFailure carries everything a failure handler needs to decide between
// retrying another candidate and reporting the failure to the client.
type streamFailure struct {
	// Err is the failure, or nil for a clean stream.
	Err error
	// Commit reports how far the client response has gone.
	Commit *streamCommit
	// Budget is the same object the caller used for Commit, named separately so the
	// handler reads as "what may I still spend" at the call site.
	Budget *streamCommit
	// Body is the upstream body, so a retry can also close it.
	Body io.Closer
	// Conn is the connection this attempt used.
	Conn *Connection
	// Response is the upstream response (headers/status), for the non-retry path.
	Response *http.Response
}

// handleStreamFailure decides what to do about a failed streaming attempt and
// performs whichever action it chooses.
//
// Returns rehandled=true when the caller should `continue` to the next candidate.
// Returns false when the handler has dealt with the failure itself (either by
// reporting it to a client that already has the stream, or by succeeding) and the
// caller should stop.
//
// The decision is one line on purpose: **an attempt may be discarded only while
// nothing has been written to the client.** Everything else follows from that. In
// particular the retry is NOT gated on which error occurred — the existing
// classification (`IsQueued() || IsLoginExpired()`) already decides what is
// *retryable in principle*, and this decides what is *still possible*.
func (p *ProxyHandler) handleStreamFailure(f streamFailure, w http.ResponseWriter, flusher http.Flusher, streamBuffer *[]byte, tokensOut *int, cost *costSample) bool {
	if f.Err == nil {
		return false
	}

	retryableInPrinciple, kind, detail := classifyStreamFailure(f.Err)

	// A failure after the commit cannot be retried: the status and possibly content
	// are already with the client. Report it the way the pre-buffering code did.
	if f.Commit == nil || !f.Commit.Retryable() {
		p.reportStreamFailure(w, flusher, f.Err, kind, detail)
		return false
	}

	// Nothing written yet: this attempt is discardable. Close the upstream body, drop
	// anything the failed candidate had held back, and let the candidate loop try the
	// next one.
	if f.Body != nil {
		f.Body.Close()
	}
	if f.Commit != nil {
		f.Commit.Abandon()
	}

	if !retryableInPrinciple {
		// Not worth another candidate (a malformed request, say). Send the diagnosis
		// now rather than burning the rest of the pool on it.
		p.reportStreamFailure(w, flusher, f.Err, kind, detail)
		return false
	}

	return true
}

// classifyStreamFailure exposes the Qoder error taxonomy as (retryable, kind, detail).
//
// The taxonomy already existed in the qoder package and was reachable only from the
// non-stream path. It is surfaced here so the streaming path cannot drift from it.
func classifyStreamFailure(err error) (bool, string, string) {
	msg, kind, retryable := qoder.DescribeStreamError(err)
	if kind == "" {
		kind = "stream_error"
	}
	if msg == "" && err != nil {
		msg = err.Error()
	}
	return retryable, kind, msg
}

// reportStreamFailure writes a terminal SSE error frame and closes the stream.
//
// The status cannot be revised at this point — either the commit happened or we are
// deliberately refusing to hand the client a bare connection close with no
// explanation — so the diagnosis travels in the body, which is the only channel
// still open. This mirrors the pre-buffering behaviour for the committed case.
func (p *ProxyHandler) reportStreamFailure(w http.ResponseWriter, flusher http.Flusher, err error, kind, detail string) {
	frame := fmt.Sprintf("data: {\"error\":{\"message\":%s,\"type\":%s}}\n\n",
		jsonString(detail), jsonString(kind))
	if _, werr := w.Write([]byte(frame)); werr != nil {
		return
	}
	w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

// jsonString encodes s as a JSON string literal.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

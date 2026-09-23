package server

// qoder_wiring.go — activation of the Qoder provider.
//
// WHY A FORMAT BRANCH AND NOT THE PROVIDER SDK
//
// The provider SDK router consumes only Prepare(). StreamTranslator and
// CredentialRefresher are declared in the SDK but are not invoked on the request
// path, and Qoder needs both: its responses are enveloped frames rather than
// OpenAI SSE, and its credentials need a session exchange before any call.
//
// Wiring StreamTranslator into the router would be a change to the shared SDK
// request path, which every Official provider also travels. That is a large
// blast radius for one Experimental provider. Instead this follows the
// commandcode precedent already in doUpstream: an explicit `format` branch, so
// the behaviour is additive and the shared path is untouched.
//
// SAFETY POSTURE
//
//   - The kill-switch (`qoder_enabled`) defaults to FALSE. With it off, this file
//     changes nothing: no provider is constructed and no request is diverted.
//   - Qoder is reachable only through its own connection. It is never added to the
//     generic fallback pool.
//   - The provider is registered under the name "qoder" only, so
//     `providerReg.Resolve(conn.Format, defaultProvider)` continues to fall back
//     to the generic provider for every other format.
//   - The request template must be provisioned (LINTASAN_QODER_TEMPLATE). Without
//     it the provider refuses with a named error rather than sending a malformed
//     request.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// qoderFormat is the connection Format value that routes through this provider.
const qoderFormat = "qoder"

// qoderEnabled reports whether the Qoder provider is switched on for this handler,
// and whether it is usable (a template must be provisioned).
//
// Both conditions are checked because an enabled-but-unprovisioned deployment
// would otherwise accept Qoder connections and fail every request with a
// template error at call time. Reporting it as unavailable keeps the failure at
// configuration time.
func (p *ProxyHandler) qoderEnabled() bool {
	return p.qoderProvider != nil
}

// initQoder installs the Qoder provider when the kill-switch is on and a request
// template has been provisioned.
//
// Failure to provision is deliberately NOT fatal: the server still starts and
// serves every other provider, and Qoder connections fail with a clear error.
// Making it fatal would let an Experimental provider's misconfiguration prevent
// the gateway from starting.
func (p *ProxyHandler) initQoder() {
	p.qoderProvider = nil

	enabled := false
	if v, err := p.db.GetSetting("qoder_enabled"); err == nil {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "on", "yes":
			enabled = true
		}
	}
	if !enabled {
		return
	}

	// The template is provisioned per-process from the environment. It is not
	// committed to the repository: it contains the upstream client's system
	// instructions and tool schemas, which belong to the vendor.
	if err := qoder.InitTemplateFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "[qoder] enabled but template provisioning failed: %v\n", err)
		return
	}
	if !qoder.TemplateReady() {
		fmt.Fprintf(os.Stderr, "[qoder] enabled but %s is not set — Qoder connections will be rejected\n", qoder.TemplateEnvVar)
		return
	}

	salt := strings.TrimSpace(os.Getenv("LINTASAN_QODER_SALT"))
	region := strings.TrimSpace(os.Getenv("LINTASAN_QODER_REGION"))
	if region == "" {
		region = "global"
	}

	p.qoderProvider = qoder.NewProvider(salt, region, nil)
	fmt.Fprintf(os.Stderr, "[qoder] active (region=%s, template provisioned, salt=%s)\n",
		region, saltState(salt))
}

// saltState describes the install salt without revealing it, so an operator can
// tell a provisioned deployment from an unprovisioned one in the logs.
//
// An empty salt is not an error, but it is worth flagging: every deployment that
// shares it presents the same device identity for a given account, which is
// exactly the correlation the salt exists to break.
func saltState(salt string) string {
	if salt == "" {
		return "NOT SET (device identity will collide across deployments)"
	}
	return "set"
}

// isQoder reports whether a connection should be routed through the Qoder
// provider.
func (p *ProxyHandler) isQoder(conn *Connection) bool {
	return conn != nil && strings.EqualFold(conn.Format, qoderFormat)
}

// qoderUpstream builds the signed, envelope-encoded upstream request for a Qoder
// connection and returns it ready for p.client.Do.
//
// The HTTP call itself stays with the caller so that reliability wrapping,
// circuit breaking and the streaming response path are unchanged — the same
// division of responsibility the SDK uses for Prepare().
func (p *ProxyHandler) qoderUpstream(ctx context.Context, conn *Connection, body []byte) (*http.Request, error) {
	if !p.qoderEnabled() {
		if !qoder.TemplateReady() {
			return nil, fmt.Errorf("qoder: connection %q requires the request template; set %s and enable the provider",
				conn.ID, qoder.TemplateEnvVar)
		}
		return nil, fmt.Errorf("qoder: connection %q is configured but the provider is disabled (enable qoder_enabled)", conn.ID)
	}

	// The inbound body is the canonical OpenAI request. Streaming is forced true
	// inside Prepare because the Qoder chat surface is streaming-only; a
	// non-streaming client is served by consuming that stream (see
	// qoderNonStreamResponse).
	up, err := p.qoderProvider.Prepare(ctx, &provider.Request{Body: body}, connToConfig(conn))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, up.Method, up.URL, strings.NewReader(string(up.Body)))
	if err != nil {
		return nil, fmt.Errorf("qoder: build upstream request: %w", err)
	}
	for k, vs := range up.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept-Encoding", "identity")
	return req, nil
}

// streamQoderToOpenAI reads Qoder's enveloped frame stream and writes canonical
// OpenAI SSE to the client.
//
// The error path is the reason this cannot be a byte pump. Qoder reports a
// refused credential, a busy model and content moderation all INSIDE a stream that
// opened with HTTP 200, so a caller that only inspects the status code sees a
// healthy stream that never produces content. ConsumeStream surfaces those as
// typed errors; here they are translated into an SSE error frame so the client
// gets a diagnosis rather than a hang.
//
// Failed attempts in the pool are common for this upstream — roughly half, at the
// time of writing — and they clear on retry. Reporting them precisely is what
// lets an operator tell "this provider is misconfigured" apart from "this
// provider is having a bad minute".
func (p *ProxyHandler) streamQoderToOpenAI(ctx context.Context, streamBody io.ReadCloser, w http.ResponseWriter, flusher http.Flusher, model string) ([]byte, int, error) {
	// No commit gate: this entry point has no candidate loop behind it, so it
	// commits on the first chunk exactly as it always has.
	buf, chunks, _, err := p.streamQoderToOpenAICost(ctx, streamBody, w, flusher, model, nil)
	return buf, chunks, err
}

// streamQoderToOpenAICost is streamQoderToOpenAI, also returning the upstream's own
// cost figures for the turn.
//
// The charge is read off Qoder's usage frame rather than derived from tokens: the
// vendor states consumption follows "task complexity", and measurement bears that
// out — the same model and prompt cost 4.617 credits on a cold prompt prefix and
// 0.449 with the prefix cached. Token counts cannot reconstruct that; the reported
// figure can.
//
// `commit` gates every client-visible write when non-nil, which is what makes this
// provider retryable. Qoder opens its stream with a role-only chunk, and a refusal
// (code 105) can arrive after it — so writing on the first chunk would commit the
// response and strand the failover, which is the bug this gate exists to fix. When
// commit is nil the legacy write-immediately behaviour applies, for callers with no
// candidate to fall back to.
func (p *ProxyHandler) streamQoderToOpenAICost(ctx context.Context, streamBody io.ReadCloser, w http.ResponseWriter, flusher http.Flusher, model string, commit *streamCommit) ([]byte, int, costSample, error) {
	// Qoder turns own the body's lifetime. The idle watchdog detects a stalled
	// stream by giving up on a blocking Read, and that Read is only released when
	// the body is closed — so a stall MUST close it, or the reader goroutine stays
	// parked forever. Cancelling here and closing on the way out covers both the
	// stall path and the normal one.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// The caller closes the body when it is going to retry (so the read below is
	// released first); closing twice is harmless, and on the non-retry path this is
	// the only close.
	if commit == nil || !commit.Retryable() {
		defer streamBody.Close()
	} else {
		defer func() {
			if !commit.Retryable() {
				streamBody.Close()
			}
		}()
	}
	body := io.Reader(streamBody)
	reqID := "chatcmpl-" + qoder.NewRequestID()[:12]
	created := qoder.NowUnix()

	var streamBuffer []byte
	chunks := 0

	// writeFrame is the single client-write path, so no chunk can bypass the gate.
	//
	// `commits` says whether this frame is something a client acts on. Only a frame
	// that carries content, reasoning or tool calls does; a role-only announcement
	// does not, and holding it is what keeps a refusal that arrives right afterwards
	// (the observed code-105 shape) retryable. See streamCommit.DeferFrame.
	writeFrame := func(frame []byte, commits bool) error {
		if commit == nil {
			if _, err := w.Write(frame); err != nil {
				return err
			}
			if flusher != nil {
				flusher.Flush()
			}
			return nil
		}
		if commits {
			return commit.WriteFrame(w, flusher, frame)
		}
		return commit.DeferFrame(w, flusher, frame)
	}

	outcome, err := qoder.ConsumeStream(ctx, body, model, func(d qoder.StreamDelta) error {
		delta := map[string]any{}
		if d.Role != "" {
			delta["role"] = d.Role
		}
		if d.Reasoning != "" {
			delta["reasoning_content"] = d.Reasoning
		}
		if d.Content != "" {
			delta["content"] = d.Content
		}
		if len(d.ToolCalls) > 0 {
			delta["tool_calls"] = d.ToolCalls
		}
		if len(delta) == 0 {
			return nil
		}

		chunk := qoder.ChatChunk(reqID, created, model, delta, nil)
		raw, mErr := json.Marshal(chunk)
		if mErr != nil {
			return nil
		}
		// A frame carrying only a role announcement, a keep-alive or a metadata
		// update is not something a client acts on, so it is held rather than
		// committed. Anything with content, reasoning or tool calls is the real
		// thing and commits — a client that has received it cannot be un-served.
		commits := d.Content != "" || d.Reasoning != "" || len(d.ToolCalls) > 0
		if err := writeFrame([]byte("data: "+string(raw)+"\n\n"), commits); err != nil {
			return err
		}

		// Accumulate only the user-visible text for the non-streaming cache path.
		streamBuffer = append(streamBuffer, []byte(d.Content)...)
		chunks++
		return nil
	})

	if err != nil {
		// With no commit gate this entry point has no candidate to fall back to, so
		// it reports the failure itself: a diagnosis frame plus a deliberate
		// terminal, because a silent stream close is indistinguishable from a slow
		// model. The failover path returns instead and lets its caller decide — see
		// streamQoderToOpenAICost's `commit` note.
		if commit == nil {
			msg, kind := qoderErrorDiagnosis(err)
			frame, _ := json.Marshal(map[string]any{
				"error": map[string]any{"message": msg, "type": kind},
			})
			_, _ = w.Write([]byte("data: " + string(frame) + "\n\ndata: [DONE]\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
		return streamBuffer, chunks, qoderCostSample(outcome), err
	}

	// Terminal frame so a client knows the stream ended deliberately.
	finish := "stop"
	if outcome.ToolCalls > 0 {
		finish = "tool_calls"
	}
	done := qoder.ChatChunk(reqID, created, model, map[string]any{}, finish)
	if outcome.InputTokens > 0 || outcome.OutputTokens > 0 {
		done["usage"] = map[string]any{
			"prompt_tokens":     outcome.InputTokens,
			"completion_tokens": outcome.OutputTokens,
			"total_tokens":      outcome.InputTokens + outcome.OutputTokens,
		}
	}
	raw, _ := json.Marshal(done)
	// One frame, not two: the terminal chunk and the [DONE] sentinel together. The
	// terminal always commits — a completed turn is a served turn.
	if err := writeFrame([]byte("data: "+string(raw)+"\n\ndata: [DONE]\n\n"), true); err != nil {
		return streamBuffer, chunks, qoderCostSample(outcome), err
	}
	return streamBuffer, chunks, qoderCostSample(outcome), nil
}

// qoderCostSample converts a stream outcome into a loggable charge.
//
// `Reported` is carried through so a turn whose usage frame omitted `credits` is
// logged as NULL rather than as a free turn.
func qoderCostSample(outcome qoder.StreamOutcome) costSample {
	return costSample{
		Credits:      outcome.Credits,
		CachedTokens: outcome.CachedTokens,
		Reported:     outcome.CreditsReported,
	}
}

// qoderErrorDiagnosis maps a Qoder error onto a client-facing message and type.
//
// The 105 case deliberately tells the caller to RETRY rather than to treat the
// credential as broken. Measured across five sweeps, 7 of 9 credentials served
// normally again after reporting it, and no credential was permanently dead. A
// client that reads it as "this account is gone" would discard a working
// credential, and a router that did the same would walk its whole pool and mark
// all of it dead.
func qoderErrorDiagnosis(err error) (message, kind string) {
	// Delegated so the stall case, the queue case and the credential case are all
	// described in one place rather than diverging between call sites.
	msg, k, _ := qoder.DescribeStreamError(err)
	return msg, k
}

// FirstByteTimeout exposes the first-frame allowance to other handlers.
func (p *ProxyHandler) FirstByteTimeout() time.Duration { return p.qoderFirstByteTimeout() }

// IdleTimeout exposes the idle allowance to other handlers.
func (p *ProxyHandler) IdleTimeout() time.Duration { return p.qoderIdleTimeout() }

// qoderRegion reports the configured upstream region.
func (p *ProxyHandler) qoderRegion() string {
	if r := strings.TrimSpace(os.Getenv("LINTASAN_QODER_REGION")); r != "" {
		return r
	}
	return "global"
}

// qoderIdleTimeout is the per-request idle allowance, overridable by an operator.
//
// The default is deliberately short relative to a normal turn: a stream that has
// produced nothing for this long is far more likely to be queued or dead than
// slowly thinking, and the caller is better served by a diagnosis than by
// waiting. A queued model recovers, so the error is retryable.
func (p *ProxyHandler) qoderIdleTimeout() time.Duration {
	if v, err := p.db.GetSetting("qoder_idle_timeout_seconds"); err == nil {
		if n, perr := strconv.Atoi(strings.TrimSpace(v)); perr == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return qoder.DefaultIdleTimeout
}

// qoderFirstByteTimeout is the more generous allowance for the first frame, since
// a queued model can take a while to start producing.
func (p *ProxyHandler) qoderFirstByteTimeout() time.Duration {
	if v, err := p.db.GetSetting("qoder_first_byte_timeout_seconds"); err == nil {
		if n, perr := strconv.Atoi(strings.TrimSpace(v)); perr == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return qoder.DefaultFirstByteTimeout
}

// qoderNonStreamResponse collects a full Qoder turn into one OpenAI response
// object, for clients that did not ask for SSE.
//
// The Qoder chat surface is streaming-only, so a non-streaming caller is served
// by consuming the stream to completion and assembling the result. The upstream
// call is identical either way; only the client-facing shape differs.
func (p *ProxyHandler) qoderNonStreamResponse(ctx context.Context, streamBody io.ReadCloser, model string) ([]byte, error) {
	b, _, err := p.qoderNonStreamResponseCost(ctx, streamBody, model)
	return b, err
}

// qoderNonStreamResponseCost is qoderNonStreamResponse, also returning the
// upstream's own cost figures for the turn.
func (p *ProxyHandler) qoderNonStreamResponseCost(ctx context.Context, streamBody io.ReadCloser, model string) ([]byte, costSample, error) {
	// Same body-lifetime rule as the streaming path: a stall is detected by
	// abandoning a blocking Read, and only closing the body releases it.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var content, reasoning strings.Builder
	var toolCalls []any
	body := io.Reader(streamBody)

	outcome, err := qoder.ConsumeStreamWithIdleTimeout(ctx, body, model, p.qoderFirstByteTimeout(), p.qoderIdleTimeout(), func(d qoder.StreamDelta) error {
		content.WriteString(d.Content)
		reasoning.WriteString(d.Reasoning)
		if len(d.ToolCalls) > 0 {
			toolCalls = append(toolCalls, d.ToolCalls...)
		}
		return nil
	})
	if err != nil {
		return nil, qoderCostSample(outcome), err
	}

	finish := "stop"
	message := map[string]any{"role": "assistant", "content": content.String()}
	if len(toolCalls) > 0 {
		finish = "tool_calls"
		message["tool_calls"] = toolCalls
		if content.Len() == 0 {
			message["content"] = nil
		}
	}
	// Reasoning is surfaced separately so a client that understands it can show
	// it, and one that does not still gets the answer.
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}

	resp := map[string]any{
		"id":      "chatcmpl-" + qoder.NewRequestID()[:12],
		"object":  "chat.completion",
		"created": qoder.NowUnix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       message,
			"finish_reason": finish,
		}},
		"usage": map[string]any{
			"prompt_tokens":     outcome.InputTokens,
			"completion_tokens": outcome.OutputTokens,
			"total_tokens":      outcome.InputTokens + outcome.OutputTokens,
		},
	}
	b, err := json.Marshal(resp)
	return b, qoderCostSample(outcome), err
}

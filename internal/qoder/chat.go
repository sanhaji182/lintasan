package qoder

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatRequest is the normalized inbound request the builder consumes.
type ChatRequest struct {
	// Model is the upstream model key to run.
	Model string
	// Messages is the inbound conversation in OpenAI shape.
	Messages []map[string]any
	// Tools is the inbound tool definitions, when the caller supplied any.
	Tools []any
	// Stream indicates the caller asked for SSE.
	Stream bool
	// MaxTokens overrides the template's token ceiling when non-zero.
	MaxTokens int
	// IsReasoning marks a reasoning-capable model.
	IsReasoning bool
	// UserType is replayed to upstream as the account's plan type.
	UserType string
}

// BuildChatBody renders a complete upstream chat request from the template.
//
// Field assignment is structural (map writes) rather than textual substitution
// into the raw JSON. That distinction matters: a prompt containing a quote or a
// backslash would corrupt the document under string substitution, producing a
// malformed request body and a confusing upstream error. Only the identifier
// and timestamp tokens are textually replaced beforehand, and those are
// generated hex or digits.
func BuildChatBody(req ChatRequest) ([]byte, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("qoder: chat request has no model")
	}

	tmpl, err := requestTemplate()
	if err != nil {
		return nil, err
	}
	body, err := deepCopyMap(tmpl)
	if err != nil {
		return nil, err
	}

	// Per-request identifiers. Upstream correlates a turn using all four.
	requestID := NewUUID()
	body["request_id"] = requestID
	body["request_set_id"] = NewUUID()
	body["chat_record_id"] = requestID
	body["session_id"] = NewUUID()
	body["stream"] = req.Stream
	if req.UserType != "" {
		body["aliyun_user_type"] = req.UserType
	}

	prompt := lastUserPrompt(req.Messages)

	// The model selection block. display_name is cosmetic but is echoed back in
	// some responses, so it is kept consistent with the key.
	if mc, ok := body["model_config"].(map[string]any); ok {
		mc["key"] = req.Model
		mc["display_name"] = req.Model
		mc["is_reasoning"] = req.IsReasoning
		if req.IsReasoning {
			// Reasoning models accept a larger completion budget upstream.
			if params, ok := body["parameters"].(map[string]any); ok {
				params["max_tokens"] = 32768
			}
		}
	}
	if req.MaxTokens > 0 {
		if params, ok := body["parameters"].(map[string]any); ok {
			params["max_tokens"] = req.MaxTokens
		}
	}

	// The prompt is placed in three places because upstream reads different ones
	// depending on the code path it takes. Supplying only one leaves the others
	// showing the template's placeholder text.
	if cc, ok := body["chat_context"].(map[string]any); ok {
		if txt, ok := cc["text"].(map[string]any); ok {
			txt["text"] = prompt
		}
		if extra, ok := cc["extra"].(map[string]any); ok {
			if oc, ok := extra["originalContent"].(map[string]any); ok {
				oc["text"] = prompt
			}
			if mc, ok := extra["modelConfig"].(map[string]any); ok {
				mc["key"] = req.Model
				mc["is_reasoning"] = req.IsReasoning
			}
		}
	}

	// The business block is the turn's accounting record.
	if biz, ok := body["business"].(map[string]any); ok {
		biz["id"] = NewUUID()
		biz["begin_at"] = UnixMs()
		biz["name"] = truncate(prompt, 30)
	}

	// Conversation. Template system instructions are carried over only when the
	// caller did not supply their own, so a caller that sets a system prompt
	// keeps full control of the request.
	msgs := buildMessages(templateMessages(tmpl), req.Messages)
	body["messages"] = msgs

	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}

	return json.Marshal(body)
}

// templateMessages returns the conversation skeleton from a template.
func templateMessages(tmpl map[string]any) []map[string]any {
	raw, ok := tmpl["messages"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, m := range raw {
		if mm, ok := m.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}

// buildMessages assembles the conversation sent upstream: the template's system
// instructions (when the caller supplied none) followed by the caller's turns
// in their original order.
func buildMessages(tmpl, incoming []map[string]any) []map[string]any {
	hasSystem := false
	for _, m := range incoming {
		if role, _ := m["role"].(string); role == "system" {
			hasSystem = true
			break
		}
	}

	out := make([]map[string]any, 0, len(tmpl)+len(incoming))
	if !hasSystem {
		for _, m := range tmpl {
			if role, _ := m["role"].(string); role == "system" {
				if cp, err := deepCopyMap(m); err == nil {
					out = append(out, cp)
				}
			}
		}
	}
	out = append(out, incoming...)
	return out
}

// lastUserPrompt returns the text of the most recent user turn, used for the
// prompt-echo fields. Content may be a plain string or an array of parts.
func lastUserPrompt(msgs []map[string]any) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if role, _ := msgs[i]["role"].(string); role != "user" {
			continue
		}
		if s := messageText(msgs[i]); s != "" {
			return s
		}
	}
	return ""
}

// messageText extracts text from an OpenAI-shaped message whose content may be a
// string, an array of typed parts, or absent.
func messageText(m map[string]any) string {
	switch c := m["content"].(type) {
	case string:
		return c
	case []any:
		var b strings.Builder
		for _, part := range c {
			p, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if t, ok := p["text"].(string); ok {
				b.WriteString(t)
			}
		}
		return b.String()
	}
	return ""
}

// deepCopyMap deep-copies an arbitrary JSON tree so per-request mutation cannot
// leak into the shared template. A shallow copy would be a data race across
// concurrent requests — the nested maps are what get mutated.
func deepCopyMap(m map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("qoder: copy template: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("qoder: copy template: %w", err)
	}
	return out, nil
}

// ============================================================================
// Streaming response translation
// ============================================================================

// StreamDelta is one incremental update decoded from the upstream stream.
type StreamDelta struct {
	Role         string
	Content      string
	Reasoning    string
	ToolCalls    []any
	InputTokens  int
	OutputTokens int
	FinishReason string
}

// IsEmpty reports whether the delta carries nothing worth forwarding.
func (d StreamDelta) IsEmpty() bool {
	return d.Content == "" && d.Reasoning == "" && len(d.ToolCalls) == 0
}

// ParseStreamFrame decodes one upstream SSE data payload into a delta.
//
// It returns an error for a frame that reports failure, and a zero delta for a
// frame that carries nothing actionable. Both are normal outcomes: upstream
// interleaves keep-alives and metadata frames with content frames.
//
// The error path is the important one. Upstream signals a rejected credential,
// a busy model and content moderation the same way — inside a frame of an
// otherwise-successful HTTP 200 stream. Nothing about the response status
// distinguishes them, so this frame check is the only place the failure can be
// caught.
func ParseStreamFrame(raw []byte) (StreamDelta, *UpstreamError) {
	if e := parseEnvelopeError(raw); e != nil {
		return StreamDelta{}, e
	}

	var wrapper struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return StreamDelta{}, nil
	}
	if wrapper.Body == "" {
		return StreamDelta{}, nil
	}

	var inner map[string]any
	if err := json.Unmarshal([]byte(wrapper.Body), &inner); err != nil {
		return StreamDelta{}, nil
	}

	// Usage may arrive on the same frame as content, so it is captured before
	// the content check rather than returned early.
	var in, out int
	if usage, ok := inner["usage"].(map[string]any); ok {
		in = int(floatField(usage, "prompt_tokens"))
		out = int(floatField(usage, "completion_tokens"))
	}

	if choices, ok := inner["choices"].([]any); ok {
		for _, c := range choices {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			delta, ok := cm["delta"].(map[string]any)
			if !ok {
				continue
			}
			d := StreamDelta{
				Role:         strField(delta, "role"),
				Content:      strField(delta, "content"),
				Reasoning:    strField(delta, "reasoning_content"),
				InputTokens:  in,
				OutputTokens: out,
			}
			if tc, ok := delta["tool_calls"].([]any); ok && len(tc) > 0 {
				d.ToolCalls = tc
			}
			if fr, ok := cm["finish_reason"].(string); ok {
				d.FinishReason = fr
			}
			if d.Role != "" || d.Content != "" || d.Reasoning != "" || d.ToolCalls != nil {
				return d, nil
			}
		}
	}

	// A frame carrying only usage is still useful (token accounting).
	if in > 0 || out > 0 {
		return StreamDelta{InputTokens: in, OutputTokens: out}, nil
	}
	return StreamDelta{}, nil
}

// strField reads a string field from a decoded JSON object.
func strField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// floatField reads a numeric field from a decoded JSON object.
func floatField(m map[string]any, key string) float64 {
	f, _ := m[key].(float64)
	return f
}

// ============================================================================
// Client-facing chunk encoding
// ============================================================================

// ChatChunk builds an OpenAI-compatible streaming chunk.
func ChatChunk(id string, created int64, model string, delta map[string]any, finishReason any) map[string]any {
	return map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
			},
		},
	}
}

// NewCompletionID builds the client-facing response identifier.
func NewCompletionID() string { return "chatcmpl-" + NewRequestID() }

// NowUnix returns the current Unix time, exposed so callers do not need to
// import time just for the chunk timestamp.
func NowUnix() int64 { return time.Now().Unix() }

// StreamOutcome summarises a completed stream for logging and failover.
type StreamOutcome struct {
	InputTokens   int
	OutputTokens  int
	ContentLength int
	ToolCalls     int
	HadContent    bool
}

// StreamHandler receives decoded deltas while a stream is consumed. Returning an
// error aborts the stream.
type StreamHandler func(StreamDelta) error

// ConsumeStream reads an upstream SSE body, forwarding deltas and translating
// any in-stream protocol error into a Go error.
//
// This is deliberately a plain incrementing reader rather than a call into the
// router's generic stream piping: the frame format here is specific (an envelope
// wrapping an OpenAI-shaped body) and the failure mode is specific (a rejection
// that looks like a healthy 200). Keeping it self-contained means the behaviour
// is testable without a live upstream.
//
// chunkSink, when non-nil, receives each client-facing chunk as a marshalled
// JSON object.
func ConsumeStream(ctx context.Context, r io.Reader, model string, h StreamHandler) (StreamOutcome, error) {
	var outcome StreamOutcome
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return outcome, err
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				break
			}
			continue
		}

		delta, upstreamErr := ParseStreamFrame([]byte(payload))
		if upstreamErr != nil {
			// A dead credential is reported here and nowhere else.
			return outcome, upstreamErr
		}
		if delta.IsEmpty() && delta.InputTokens == 0 && delta.OutputTokens == 0 {
			continue
		}
		if delta.InputTokens > 0 || delta.OutputTokens > 0 {
			outcome.InputTokens = delta.InputTokens
			outcome.OutputTokens = delta.OutputTokens
		}
		if delta.Content != "" || delta.Reasoning != "" {
			outcome.HadContent = true
			outcome.ContentLength += len(delta.Content)
		}
		if len(delta.ToolCalls) > 0 {
			outcome.HadContent = true
			outcome.ToolCalls += len(delta.ToolCalls)
		}
		if h != nil {
			if err := h(delta); err != nil {
				return outcome, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return outcome, fmt.Errorf("qoder: read stream: %w", err)
	}
	return outcome, nil
}

// HTTPStatusFor maps an upstream error to the status a client should see.
//
// A dead credential becomes 401 rather than a passthrough 403, because from the
// gateway's perspective the configured credential is the thing that is wrong —
// and a 401 is what makes an operator look at the connection instead of the
// caller's request. A busy model becomes 503 with the requested delay, which is
// what makes an upstream that is merely throttling distinguishable from one
// that is failing.
func HTTPStatusFor(e *UpstreamError) int {
	if e == nil {
		return http.StatusOK
	}
	switch {
	case e.IsLoginExpired():
		return http.StatusUnauthorized
	case e.IsQueued():
		return http.StatusServiceUnavailable
	case e.Status >= 400 && e.Status <= 599:
		return e.Status
	default:
		return http.StatusBadGateway
	}
}

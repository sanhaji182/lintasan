package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// Chat turn loop: takes a user message in a conversation, assembles context,
// calls the gateway, streams the reply back as SSE, and persists the result.
//
// The upstream call is made through Lintasan's OWN /v1/chat/completions
// endpoint over loopback rather than reaching into ProxyHandler internals.
// That is deliberate: it means every existing feature — smart routing, combos,
// fallback chains, circuit breaking, semantic cache, cost tracking, guardrails,
// plugins — applies to chat for free and stays in one place. Duplicating the
// dispatch here would create a second, divergent code path.

// sseEvent writes one Server-Sent Event frame.
func sseEvent(w io.Writer, f http.Flusher, event string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		return err
	}
	if f != nil {
		f.Flush()
	}
	return nil
}

// postMessageRequest is the body of POST /api/conversations/{id}/messages.
type postMessageRequest struct {
	Content string `json:"content"`
	// Model overrides the conversation's model for this turn only.
	Model string `json:"model"`
	// AttachmentIDs reference files already uploaded to this conversation.
	AttachmentIDs []string `json:"attachment_ids"`
	// Stream defaults to true. Set false for a single JSON response (easier to
	// consume from scripts, and what the tests use).
	Stream *bool `json:"stream"`
	// UseTools enables the tool loop for this turn.
	UseTools *bool `json:"use_tools"`
	// Temperature / MaxTokens pass through to the model when set.
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
}

// modelContextWindow returns the usable context window for a model name.
// Unknown models get a conservative default rather than an optimistic one, so
// we summarise earlier instead of failing an upstream with a 400.
func modelContextWindow(model string) int {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "gemini"):
		return 1_000_000
	case strings.Contains(m, "claude"):
		return 200_000
	case strings.Contains(m, "gpt-4.1"), strings.Contains(m, "gpt-5"):
		return 400_000
	case strings.Contains(m, "gpt-4o"), strings.Contains(m, "gpt-4-turbo"):
		return 128_000
	case strings.Contains(m, "o1"), strings.Contains(m, "o3"), strings.Contains(m, "o4"):
		return 200_000
	case strings.Contains(m, "deepseek"):
		return 128_000
	case strings.Contains(m, "llama"), strings.Contains(m, "qwen"), strings.Contains(m, "mistral"):
		return 32_000
	case strings.Contains(m, "gpt-3.5"):
		return 16_000
	default:
		return 16_000
	}
}

// chatBudget builds the assembly budget for a model.
func chatBudget(model string) conversation.Budget {
	window := modelContextWindow(model)
	// Reserve a quarter of the window for the reply on small models, less on
	// huge ones where a fixed 4k is plenty.
	reserve := window / 4
	if reserve > 8192 {
		reserve = 8192
	}
	if reserve < 1024 {
		reserve = 1024
	}
	return conversation.Budget{
		MaxTokens:        window,
		ReserveForOutput: reserve,
		KeepRecent:       6,
	}
}

// handlePostMessage is the main chat turn endpoint.
func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	convID := r.PathValue("id")

	var in postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Content) == "" && len(in.AttachmentIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "content or attachment_ids is required")
		return
	}

	conv, err := st.Get(convID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "conversation not found")
		return
	}

	model := conv.Model
	if in.Model != "" {
		model = in.Model
	}
	if model == "" {
		model = s.defaultChatModel()
	}

	stream := true
	if in.Stream != nil {
		stream = *in.Stream
	}

	// Persist the user's turn first so a crash mid-reply still leaves the
	// prompt visible in the UI.
	attMeta, attText, err := s.loadAttachments(st, convID, in.AttachmentIDs)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	userMsg := &conversation.Message{
		ConversationID: convID,
		Role:           conversation.RoleUser,
		Content:        in.Content,
		Attachments:    attMeta,
	}
	if err := st.AppendMessage(userMsg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// First user turn names the thread, so the sidebar is not a wall of
	// "New chat". Best-effort: a failure here must not fail the turn.
	if conv.MsgCount == 0 && conv.Title == "New chat" && strings.TrimSpace(in.Content) != "" {
		title := deriveTitle(in.Content)
		if err := st.Update(convID, conversation.UpdateFields{Title: &title}); err == nil {
			conv.Title = title
		}
	}

	// Assemble context.
	budget := chatBudget(model)
	history, err := st.MessagesAfter(convID, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Drop the just-appended user message from history; it goes in as `extra`
	// with its attachment text attached.
	var prior []*conversation.Message
	for _, m := range history {
		if m.ID != userMsg.ID {
			prior = append(prior, m)
		}
	}
	extra := []*conversation.Message{userMsg}
	if attText != "" {
		copyMsg := *userMsg
		copyMsg.Content = strings.TrimSpace(userMsg.Content + "\n\n" + attText)
		extra = []*conversation.Message{&copyMsg}
	}
	asm := conversation.Assemble(conv, prior, extra, budget)

	payload := map[string]any{
		"model":    model,
		"messages": toWireMessages(asm.Messages),
		"stream":   stream,
	}
	if in.Temperature != nil {
		payload["temperature"] = *in.Temperature
	}
	if in.MaxTokens != nil {
		payload["max_tokens"] = *in.MaxTokens
	}

	// Tool loop is wired in chat_tools.go; when disabled this is a plain call.
	useTools := s.toolLoopEnabled() && r.URL.Query().Get("tools") != "false"
	if in.UseTools != nil {
		useTools = *in.UseTools && s.toolLoopEnabled()
	}

	if !stream {
		s.runTurnBlocking(w, r.Context(), st, conv, userMsg, payload, model, useTools)
		return
	}
	s.runTurnStreaming(w, r, st, conv, userMsg, payload, model, useTools)
}

// defaultChatModel picks the model for a new thread: the operator's configured
// default, falling back to whatever the first active connection advertises.
func (s *Server) defaultChatModel() string {
	if v := s.proxy.getSetting("default_model", ""); v != "" {
		return v
	}
	if v := s.proxy.getSetting("chat_default_model", ""); v != "" {
		return v
	}
	return "gpt-4o-mini"
}

// deriveTitle makes a short thread title from the first user message.
func deriveTitle(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if content == "" {
		return "New chat"
	}
	// Cut on a word boundary so titles do not end mid-word.
	const max = 60
	if len(content) <= max {
		return content
	}
	cut := content[:max]
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// loadAttachments resolves attachment ids into (a) the JSON metadata blob to
// store on the user message and (b) inline text for the model. It rejects ids
// belonging to another conversation so one thread cannot read another's files.
func (s *Server) loadAttachments(st *conversation.Store, convID string, ids []string) (string, string, error) {
	if len(ids) == 0 {
		return "", "", nil
	}
	type meta struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
		Mime     string `json:"mime"`
		Size     int64  `json:"size"`
	}
	metas := make([]meta, 0, len(ids))
	var texts []string
	for _, id := range ids {
		a, err := st.GetAttachment(id)
		if err != nil {
			return "", "", fmt.Errorf("attachment %s not found", id)
		}
		if a.ConversationID != convID {
			return "", "", fmt.Errorf("attachment %s belongs to a different conversation", id)
		}
		metas = append(metas, meta{ID: a.ID, Filename: a.Filename, Mime: a.Mime, Size: a.Size})
		if strings.TrimSpace(a.Extracted) != "" {
			texts = append(texts, fmt.Sprintf("--- file: %s ---\n%s", a.Filename, a.Extracted))
		}
	}
	// MessageID is filled in by the caller after the message exists; leaving it
	// blank here is fine because the association is by conversation.
	b, err := json.Marshal(metas)
	if err != nil {
		return "", "", err
	}
	return string(b), strings.Join(texts, "\n\n"), nil
}

// toWireMessages flattens assembled messages into OpenAI wire format.
func toWireMessages(msgs []conversation.AssembledMessage) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]any{
			"role":    m.Role,
			"content": m.Content,
		})
	}
	return out
}

// upstreamURL is the loopback address of this process's own gateway.
func (s *Server) upstreamURL() string {
	if s.chatUpstreamURL != "" {
		return s.chatUpstreamURL
	}
	port := s.cfg.Port
	if port <= 0 {
		port = 20180
	}
	return fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port)
}

// gatewayKey returns the credential the loopback call should present.
//
// The loopback call is authorised by the SAME authMiddleware as any external
// client, so it must present a key this process actually accepts. The master
// key can come from either the DB setting or the environment
// (LINTASAN_MASTER_KEY); checking only the DB setting makes chat 401 against
// itself on any deployment configured through the environment.
func (s *Server) gatewayKey() string {
	if v, _ := s.db.GetSetting("master_key"); v != "" {
		return v
	}
	if s.cfg.MasterKey != "" {
		return s.cfg.MasterKey
	}
	return ""
}

// nonStreamingTurnTimeout bounds one blocking turn (and one tool-loop round).
// Streaming turns are bounded by the client's own context instead, because a
// long generated reply is legitimate and must not be cut off.
const nonStreamingTurnTimeout = 90 * time.Second

// summariseTimeout bounds the compaction call. Summarising a transcript is a
// bounded job; if the upstream cannot do it in this window the conversation
// simply stays uncompacted and the next turn sends more context.
const summariseTimeout = 60 * time.Second

// chatClient is the HTTP client used for loopback upstream calls. It has no
// global timeout because streaming replies can legitimately run for minutes;
// per-request cancellation comes from the caller's context.
var chatClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	},
}

// runTurnBlocking performs the whole turn and returns one JSON object. Used by
// scripts and tests; the UI uses the streaming path.
func (s *Server) runTurnBlocking(w http.ResponseWriter, ctx context.Context, st *conversation.Store,
	conv *conversation.Conversation, userMsg *conversation.Message, payload map[string]any,
	model string, useTools bool) {

	var reply, thinking string
	var tokensIn, tokensOut int
	var err error
	if useTools {
		// The tool loop drives the whole turn: it issues the first call, runs any
		// requested tools, and re-issues until the model stops asking. Calling
		// performTurn first would burn an extra round trip and discard the
		// tool_calls it returned.
		reply, thinking, tokensIn, tokensOut, err = s.runToolLoop(ctx, payload, model, func(streamEvent) {})
	} else {
		reply, thinking, tokensIn, tokensOut, err = s.performTurn(ctx, payload, false)
	}
	asst := &conversation.Message{
		ConversationID: conv.ID,
		Role:           conversation.RoleAssistant,
		Content:        reply,
		Thinking:       thinking,
		ModelUsed:      model,
		TokensIn:       tokensIn,
		TokensOut:      tokensOut,
	}
	if err != nil {
		asst.Error = err.Error()
	}
	if aerr := st.AppendMessage(asst); aerr != nil {
		writeErr(w, http.StatusInternalServerError, "persist reply: "+aerr.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	s.autoCompact(ctx, st, conv, model)
	writeJSON(w, map[string]any{"message": asst, "user_message": userMsg})
}

// runTurnStreaming streams the assistant reply as SSE events:
//
//	event: start    {user_message, assistant_id, model}
//	event: delta    {text}
//	event: thinking {text}
//	event: tool     {name, args, result}   (tool loop only)
//	event: done     {message}
//	event: error    {message}
func (s *Server) runTurnStreaming(w http.ResponseWriter, r *http.Request, st *conversation.Store,
	conv *conversation.Conversation, userMsg *conversation.Message, payload map[string]any,
	model string, useTools bool) {

	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		// Without a flusher there is no streaming; fall back to a plain reply
		// rather than hanging the client.
		s.runTurnBlocking(w, r.Context(), st, conv, userMsg, payload, model, useTools)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default, which would defeat streaming.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	asst := &conversation.Message{
		ConversationID: conv.ID,
		Role:           conversation.RoleAssistant,
		ModelUsed:      model,
	}
	_ = sseEvent(w, flusher, "start", map[string]any{
		"user_message":   userMsg,
		"assistant_id":   asst.ID,
		"model":          model,
		"summarised":     false,
		"conversation_id": conv.ID,
	})

	ctx := r.Context()
	var sb strings.Builder
	var thinkSb strings.Builder
	tokensIn, tokensOut := 0, 0

	if useTools {
		// The tool loop is inherently multi-round: each round may produce tool
		// calls that must be answered before the model continues. Streaming
		// raw deltas across rounds would interleave unrelated text, so the loop
		// runs to completion and the final answer is emitted once. Tool
		// invocations still stream live, which is what the user watches.
		reply, thinking, tin, tout, terr := s.runToolLoop(ctx, payload, model, func(ev streamEvent) {
			_ = sseEvent(w, flusher, "tool", map[string]any{
				"name": ev.ToolName, "args": ev.ToolArgs, "result": ev.ToolResult,
			})
		})
		tokensIn, tokensOut = tin, tout
		if thinking != "" {
			thinkSb.WriteString(thinking)
			_ = sseEvent(w, flusher, "thinking", map[string]any{"text": thinking})
		}
		if terr != nil {
			asst.Error = terr.Error()
			if aerr := st.AppendMessage(asst); aerr != nil {
				_ = sseEvent(w, flusher, "error", map[string]any{"message": "persist reply: " + aerr.Error()})
				return
			}
			_ = sseEvent(w, flusher, "error", map[string]any{"message": terr.Error()})
			return
		}
		sb.WriteString(reply)
		if reply != "" {
			_ = sseEvent(w, flusher, "delta", map[string]any{"text": reply})
		}
		asst.Content = sb.String()
		asst.Thinking = thinkSb.String()
		asst.TokensIn = tokensIn
		asst.TokensOut = tokensOut
		if aerr := st.AppendMessage(asst); aerr != nil {
			_ = sseEvent(w, flusher, "error", map[string]any{"message": "persist reply: " + aerr.Error()})
			return
		}
		_ = sseEvent(w, flusher, "done", map[string]any{"message": asst})
		s.autoCompact(ctx, st, conv, model)
		return
	}

	err := s.streamTurn(ctx, payload, useTools, func(ev streamEvent) {
		switch ev.Kind {
		case streamDelta:
			sb.WriteString(ev.Text)
			_ = sseEvent(w, flusher, "delta", map[string]any{"text": ev.Text})
		case streamThinking:
			thinkSb.WriteString(ev.Text)
			_ = sseEvent(w, flusher, "thinking", map[string]any{"text": ev.Text})
		case streamTool:
			_ = sseEvent(w, flusher, "tool", map[string]any{
				"name": ev.ToolName, "args": ev.ToolArgs, "result": ev.ToolResult,
			})
		case streamUsage:
			tokensIn, tokensOut = ev.TokensIn, ev.TokensOut
		}
	})

	asst.Content = sb.String()
	asst.Thinking = thinkSb.String()
	asst.TokensIn = tokensIn
	asst.TokensOut = tokensOut
	if err != nil {
		asst.Error = err.Error()
	}
	if aerr := st.AppendMessage(asst); aerr != nil {
		_ = sseEvent(w, flusher, "error", map[string]any{"message": "persist reply: " + aerr.Error()})
		return
	}

	if err != nil {
		_ = sseEvent(w, flusher, "error", map[string]any{"message": err.Error()})
		return
	}
	_ = sseEvent(w, flusher, "done", map[string]any{"message": asst})

	s.autoCompact(ctx, st, conv, model)
}

// streamEventKind discriminates streamEvent.
type streamEventKind int

const (
	streamDelta streamEventKind = iota
	streamThinking
	streamTool
	streamUsage
)

// streamEvent is one thing that happened during a turn.
type streamEvent struct {
	Kind       streamEventKind
	Text       string
	ToolName   string
	ToolArgs   string
	ToolResult string
	TokensIn   int
	TokensOut  int
}

// streamTurn issues the loopback request and decodes the upstream SSE stream,
// invoking emit for every delta / thinking chunk / tool result.
func (s *Server) streamTurn(ctx context.Context, payload map[string]any, useTools bool, emit func(streamEvent)) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.upstreamURL(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if k := s.gatewayKey(); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}
	// Mark the call so request logs can distinguish chat-UI traffic from API
	// clients, and so we can exclude it from recursive chat handling.
	req.Header.Set("X-Lintasan-Source", "chat-ui")

	resp, err := chatClient.Do(req)
	if err != nil {
		return fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	// usage may arrive in the final chunk; capture it whatever the format.
	var usage struct{ in, out int }
	defer func() {
		if usage.in > 0 || usage.out > 0 {
			emit(streamEvent{Kind: streamUsage, TokensIn: usage.in, TokensOut: usage.out})
		}
	}()

	scanner := bufio.NewScanner(resp.Body)
	// SSE lines can exceed the default 64KB buffer when a chunk carries a long
	// tool argument or a big reasoning block.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // ignore malformed keep-alive frames
		}
		parseUpstreamChunk(chunk, &usage, emit)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read upstream stream: %w", err)
	}
	return nil
}

// parseUpstreamChunk extracts text, reasoning and usage from one SSE chunk.
// It tolerates the several shapes providers use: choices[].delta.content,
// delta.reasoning_content (DeepSeek), delta.reasoning (OpenRouter), and a flat
// `response` field some gateways emit.
func parseUpstreamChunk(chunk map[string]any, usage *struct{ in, out int }, emit func(streamEvent)) {
	if u, ok := chunk["usage"].(map[string]any); ok {
		if v, ok := numField(u, "prompt_tokens"); ok {
			usage.in = v
		}
		if v, ok := numField(u, "completion_tokens"); ok {
			usage.out = v
		}
	}

	if choices, ok := chunk["choices"].([]any); ok {
		for _, c := range choices {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			delta, ok := cm["delta"].(map[string]any)
			if !ok {
				// Non-streaming shape nested in a stream frame.
				if msg, ok := cm["message"].(map[string]any); ok {
					delta = msg
				} else {
					continue
				}
			}
			if s, ok := delta["content"].(string); ok && s != "" {
				emit(streamEvent{Kind: streamDelta, Text: s})
			}
			for _, key := range []string{"reasoning_content", "reasoning", "thinking"} {
				if s, ok := delta[key].(string); ok && s != "" {
					emit(streamEvent{Kind: streamThinking, Text: s})
					break
				}
			}
			// Tool call deltas are handled by the tool loop in chat_tools.go,
			// which re-issues with full tool results rather than streaming them.
		}
	}

	// Some providers put the text at the top level during reasoning summaries.
	if s, ok := chunk["response"].(string); ok && s != "" {
		emit(streamEvent{Kind: streamDelta, Text: s})
	}
}

// numField reads a numeric JSON field that may be float64 or json.Number.
func numField(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n), true
		}
	}
	return 0, false
}

// performTurn runs a non-streaming turn through the loopback gateway and
// returns the assistant text.
//
// The whole round trip is bounded: without a deadline a stalled upstream holds
// the HTTP handler open until the client gives up, which looks like a hang
// rather than an error. Streaming turns are NOT bounded here — a long reply is
// legitimate — but each individual non-streaming call is.
func (s *Server) performTurn(ctx context.Context, payload map[string]any, useTools bool) (string, string, int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, nonStreamingTurnTimeout)
	defer cancel()

	p := make(map[string]any, len(payload))
	for k, v := range payload {
		p[k] = v
	}
	p["stream"] = false

	body, err := json.Marshal(p)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.upstreamURL(), bytes.NewReader(body))
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lintasan-Source", "chat-ui")
	if k := s.gatewayKey(); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}

	resp, err := chatClient.Do(req)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, 0, fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", 0, 0, fmt.Errorf("decode upstream response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", "", 0, 0, fmt.Errorf("upstream returned no choices")
	}
	thinking := out.Choices[0].Message.ReasoningContent
	if thinking == "" {
		thinking = out.Choices[0].Message.Reasoning
	}
	return out.Choices[0].Message.Content, thinking,
		out.Usage.PromptTokens, out.Usage.CompletionTokens, nil
}

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/translator"
)

// handlers_native_ingress.go — native Anthropic Messages and Gemini
// generateContent ingresses over the existing chat-completions pipeline.
//
// Both surfaces are additive and default-OFF. Request translation happens at the
// ingress edge, the existing routing/fallback/cache/logging pipeline runs
// unchanged, and the response is translated back into the client's native wire
// shape. No provider, connection, or routing table is added here.

func (p *ProxyHandler) HandleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	if !p.claudeMessagesAPIEnabled() {
		nativeIngressNotFound(w)
		return
	}
	raw, err := readNativeIngressJSON(r)
	if err != nil {
		writeNativeIngressError(w, http.StatusBadRequest, err.Error())
		return
	}
	chatReq := translator.AnthropicToOpenAIRequest(raw)
	model, _ := chatReq["model"].(string)
	if strings.TrimSpace(model) == "" {
		writeNativeIngressError(w, http.StatusBadRequest, "model is required")
		return
	}
	messages, _ := chatReq["messages"].([]map[string]any)
	if len(messages) == 0 {
		writeNativeIngressError(w, http.StatusBadRequest, "messages are required")
		return
	}
	wantStream, _ := raw["stream"].(bool)
	chatReq["stream"] = wantStream
	chatHTTPReq, err := nativeChatRequest(r, chatReq)
	if err != nil {
		writeNativeIngressError(w, http.StatusInternalServerError, "failed to encode translated request")
		return
	}
	if !wantStream {
		p.handleNativeBuffered(w, chatHTTPReq, translator.FormatAnthropic)
		return
	}
	adapter := newNativeStreamAdapter(w, translator.FormatAnthropic, model)
	p.HandleChatCompletions(adapter, chatHTTPReq)
	adapter.finalize()
}

func (p *ProxyHandler) HandleGeminiGenerateContent(w http.ResponseWriter, r *http.Request) {
	if !p.geminiNativeAPIEnabled() {
		nativeIngressNotFound(w)
		return
	}
	raw, err := readNativeIngressJSON(r)
	if err != nil {
		writeNativeIngressError(w, http.StatusBadRequest, err.Error())
		return
	}
	model, streaming, ok := parseGeminiNativePath(r.URL.Path)
	if !ok {
		nativeIngressNotFound(w)
		return
	}
	// Gemini carries the model in the URL, not the JSON body. The translator
	// already normalizes an optional `models/` prefix.
	raw["model"] = model
	chatReq := translator.GeminiToOpenAIRequest(raw)
	if strings.TrimSpace(model) == "" {
		writeNativeIngressError(w, http.StatusBadRequest, "model is required")
		return
	}
	messages, _ := chatReq["messages"].([]map[string]any)
	if len(messages) == 0 {
		writeNativeIngressError(w, http.StatusBadRequest, "contents are required")
		return
	}
	// Gemini streaming is selected by the :streamGenerateContent method in the
	// path. `alt=sse` is accepted but not required because this is an SSE-only
	// ingress; non-stream generateContent remains buffered JSON.
	chatReq["stream"] = streaming
	chatHTTPReq, err := nativeChatRequest(r, chatReq)
	if err != nil {
		writeNativeIngressError(w, http.StatusInternalServerError, "failed to encode translated request")
		return
	}
	if !streaming {
		p.handleNativeBuffered(w, chatHTTPReq, translator.FormatGemini)
		return
	}
	adapter := newNativeStreamAdapter(w, translator.FormatGemini, model)
	p.HandleChatCompletions(adapter, chatHTTPReq)
	adapter.finalize()
}

func readNativeIngressJSON(r *http.Request) (map[string]any, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body")
	}
	defer r.Body.Close()
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON body")
	}
	return raw, nil
}

func nativeChatRequest(r *http.Request, chatReq map[string]any) (*http.Request, error) {
	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, err
	}
	out := r.Clone(r.Context())
	out.Body = io.NopCloser(bytes.NewReader(body))
	out.ContentLength = int64(len(body))
	out.URL.Path = "/v1/chat/completions"
	out.URL.RawPath = ""
	return out, nil
}

func (p *ProxyHandler) handleNativeBuffered(w http.ResponseWriter, chatReq *http.Request, dst translator.Format) {
	rec := &bufferedResponseWriter{header: http.Header{}, status: http.StatusOK}
	p.HandleChatCompletions(rec, chatReq)
	body := rec.body.Bytes()
	if rec.status < 200 || rec.status > 299 {
		copyHeader(w.Header(), rec.header)
		w.WriteHeader(rec.status)
		_, _ = w.Write(body)
		return
	}
	var translated []byte
	var err error
	switch dst {
	case translator.FormatAnthropic:
		translated, err = translator.OpenAIResponseToAnthropic(body)
	case translator.FormatGemini:
		translated, err = translator.OpenAIResponseToGemini(body)
	default:
		err = fmt.Errorf("unsupported native response format")
	}
	if err != nil {
		writeNativeIngressError(w, http.StatusBadGateway, "failed to translate upstream response")
		return
	}
	copyHeader(w.Header(), rec.header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(translated)
}

func parseGeminiNativePath(path string) (model string, streaming bool, ok bool) {
	const prefix = "/v1beta/models/"
	if !strings.HasPrefix(path, prefix) {
		return "", false, false
	}
	rest := strings.TrimPrefix(path, prefix)
	switch {
	case strings.HasSuffix(rest, ":generateContent"):
		model = strings.TrimSuffix(rest, ":generateContent")
	case strings.HasSuffix(rest, ":streamGenerateContent"):
		model = strings.TrimSuffix(rest, ":streamGenerateContent")
		streaming = true
	default:
		return "", false, false
	}
	model = strings.TrimSpace(model)
	return model, streaming, model != "" && !strings.Contains(model, "/")
}

func nativeIngressNotFound(w http.ResponseWriter) {
	http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
}

func writeNativeIngressError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": message, "type": "invalid_request_error"},
	})
}

// nativeStreamAdapter converts each OpenAI chat-completion SSE chunk as it is
// written. It does NOT aggregate: every complete data line is translated and
// flushed immediately, preserving true streaming and backpressure.
type nativeStreamAdapter struct {
	real        http.ResponseWriter
	dst         translator.Format
	model       string
	header      http.Header
	status      int
	wroteHeader bool
	passthrough bool
	buf         bytes.Buffer
	sawDone     bool
	started     bool
}

func newNativeStreamAdapter(real http.ResponseWriter, dst translator.Format, model string) *nativeStreamAdapter {
	return &nativeStreamAdapter{real: real, dst: dst, model: model, header: http.Header{}, status: http.StatusOK}
}

func (a *nativeStreamAdapter) Header() http.Header { return a.header }

func (a *nativeStreamAdapter) WriteHeader(status int) {
	if a.wroteHeader {
		return
	}
	a.status = status
	copyHeader(a.real.Header(), a.header)
	if status < 200 || status > 299 {
		a.passthrough = true
		a.real.WriteHeader(status)
		a.wroteHeader = true
		return
	}
	a.real.Header().Set("Content-Type", "text/event-stream")
	a.real.Header().Set("Cache-Control", "no-cache")
	a.real.Header().Set("X-Lintasan-Ingress", string(a.dst))
	a.real.WriteHeader(http.StatusOK)
	a.wroteHeader = true
}

func (a *nativeStreamAdapter) Write(p []byte) (int, error) {
	if !a.wroteHeader {
		a.WriteHeader(http.StatusOK)
	}
	if a.passthrough {
		_, err := a.real.Write(p)
		return len(p), err
	}
	a.buf.Write(p)
	for {
		idx := bytes.IndexByte(a.buf.Bytes(), '\n')
		if idx < 0 {
			break
		}
		line := string(a.buf.Next(idx + 1))
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") && strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")) == "[DONE]" {
			a.sawDone = true
			continue
		}
		if translated := translator.TranslateStreamLine(line, translator.FormatOpenAI, a.dst); translated != "" {
			if a.dst == translator.FormatAnthropic && !a.started {
				start := map[string]any{
					"type": "message_start",
					"message": map[string]any{
						"id": "msg_lintasan", "type": "message", "role": "assistant", "model": a.model,
						"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
						"usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
					},
				}
				b, _ := json.Marshal(start)
				if _, err := fmt.Fprintf(a.real, "event: message_start\ndata: %s\n\n", b); err != nil {
					return len(p), err
				}
				if _, err := io.WriteString(a.real, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"); err != nil {
					return len(p), err
				}
				a.started = true
			}
			if _, err := a.real.Write([]byte(translated)); err != nil {
				return len(p), err
			}
			a.flush()
		}
	}
	return len(p), nil
}

func (a *nativeStreamAdapter) Flush() { a.flush() }

func (a *nativeStreamAdapter) flush() {
	if f, ok := a.real.(http.Flusher); ok {
		f.Flush()
	}
}

func (a *nativeStreamAdapter) finalize() {
	if a.passthrough {
		return
	}
	// Drain a final line without a trailing newline.
	if a.buf.Len() > 0 {
		line := a.buf.String()
		a.buf.Reset()
		if translated := translator.TranslateStreamLine(line, translator.FormatOpenAI, a.dst); translated != "" {
			_, _ = a.real.Write([]byte(translated))
		}
	}
	// Native clients expect an explicit terminal signal. Anthropic needs the
	// message_delta/message_stop pair; Gemini's final candidate carries STOP.
	if a.sawDone {
		switch a.dst {
		case translator.FormatAnthropic:
			if a.started {
				_, _ = io.WriteString(a.real, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			}
			_, _ = io.WriteString(a.real, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":0}}\n\n")
			_, _ = io.WriteString(a.real, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		case translator.FormatGemini:
			terminal := map[string]any{"candidates": []map[string]any{{"content": map[string]any{"parts": []any{}, "role": "model"}, "finishReason": "STOP"}}, "modelVersion": a.model}
			b, _ := json.Marshal(terminal)
			_, _ = fmt.Fprintf(a.real, "data: %s\n\n", b)
		}
	}
	a.flush()
}

package server

// antigravity_wiring.go — native Google Antigravity (Cloud Code Assist) chat.
//
// WHY A FORMAT BRANCH
//
// The OAuth IDE auto-provision used to create this provider with format=openai
// and /v1/chat/completions. Antigravity does not speak OpenAI: chat generation
// lives at /v1internal:generateContent (and :streamGenerateContent?alt=sse)
// behind a project/model/requestId envelope. The generic path therefore hit
// Google with an unknown route and returned HTML 404 for every model.
//
// This adapter builds the native envelope, then reuses the existing
// OpenAI↔Gemini translator for the request and response bodies. It is a text
// MVP: Gemini text parts and usage are translated; tool-call parts are not
// emitted yet.

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

	"github.com/sanhaji182/lintasan-go/internal/antigravity"
	"github.com/sanhaji182/lintasan-go/internal/translator"
)

const antigravityFormat = "antigravity"

func (p *ProxyHandler) isAntigravityNative(conn *Connection) bool {
	if conn == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(conn.OAuthProvider), antigravityFormat) ||
		strings.EqualFold(strings.TrimSpace(conn.Format), antigravityFormat)
}

func (p *ProxyHandler) antigravityUpstream(ctx context.Context, conn *Connection, body []byte) (*http.Request, error) {
	apiKey := strings.TrimSpace(conn.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("antigravity: connection %q has no OAuth access token", conn.ID)
	}

	var openAI map[string]any
	if err := json.Unmarshal(body, &openAI); err != nil {
		return nil, fmt.Errorf("antigravity: decode OpenAI request: %w", err)
	}
	model, _ := openAI["model"].(string)
	if strings.TrimSpace(model) == "" {
		model = conn.TargetModel
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("antigravity: request has no model")
	}
	stream, _ := openAI["stream"].(bool)

	geminiReq := translator.OpenAIToGeminiRequest(openAI)
	// The envelope carries the model; Gemini's request body does not.
	delete(geminiReq, "model")

	wireModel := antigravity.ResolveUpstreamModel(model)
	envelope := map[string]any{
		"project":   antigravity.GenerateProjectID(),
		"model":     wireModel,
		"userAgent": "antigravity",
		"requestId": antigravity.BuildIdeRequestID(wireModel, "agent"),
		"request":   geminiReq,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("antigravity: encode envelope: %w", err)
	}

	base := strings.TrimRight(strings.TrimSpace(conn.BaseURL), "/")
	if base == "" {
		base = antigravity.AntigravityIDEBaseURL
	}
	path := "/v1internal:generateContent"
	accept := "application/json"
	if stream {
		path = "/v1internal:streamGenerateContent?alt=sse"
		accept = "text/event-stream"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("antigravity: build upstream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", antigravity.AntigravityIDEUserAgent)
	req.Header.Set("Accept-Encoding", "identity")
	return req, nil
}

// antigravityUsage returns (prompt, completion). Cloud Code omits
// candidatesTokenCount when the turn ends on MAX_TOKENS during thinking, so
// fall back to total-prompt for a truthful completion figure.
func antigravityUsage(resp map[string]any) (int, int) {
	usage, _ := resp["usageMetadata"].(map[string]any)
	if usage == nil {
		return 0, 0
	}
	prompt := intFromAnyValue(usage["promptTokenCount"])
	completion := intFromAnyValue(usage["candidatesTokenCount"])
	total := intFromAnyValue(usage["totalTokenCount"])
	if completion == 0 && total > prompt {
		completion = total - prompt
	}
	return prompt, completion
}

func intFromAnyValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

// antigravityNonStreamResponse unwraps {"response": <gemini>} and converts it
// to an OpenAI completion. prompt/completion are returned for request logging.
func (p *ProxyHandler) antigravityNonStreamResponse(raw []byte, model string) ([]byte, int, int, error) {
	var wrapper struct {
		Response map[string]any `json:"response"`
		Error    map[string]any `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, 0, 0, fmt.Errorf("antigravity: decode response: %w", err)
	}
	if wrapper.Error != nil {
		message, _ := wrapper.Error["message"].(string)
		if message == "" {
			message = "upstream error"
		}
		return nil, 0, 0, fmt.Errorf("antigravity: %s", message)
	}
	resp := wrapper.Response
	if resp == nil {
		return nil, 0, 0, fmt.Errorf("antigravity: empty response")
	}

	promptTokens, completionTokens := antigravityUsage(resp)
	if usage, ok := resp["usageMetadata"].(map[string]any); ok {
		if _, exists := usage["promptTokenCount"]; !exists {
			usage["promptTokenCount"] = promptTokens
		}
		if _, exists := usage["candidatesTokenCount"]; !exists {
			usage["candidatesTokenCount"] = completionTokens
		}
	}
	if version, _ := resp["modelVersion"].(string); version != "" {
		resp["model"] = version
	} else {
		resp["model"] = model
	}

	geminiBody, err := json.Marshal(resp)
	if err != nil {
		return nil, promptTokens, completionTokens, err
	}
	openAIBody, err := translator.GeminiResponseToOpenAI(geminiBody)
	if err != nil {
		return nil, promptTokens, completionTokens, err
	}
	var openAI map[string]any
	if err := json.Unmarshal(openAIBody, &openAI); err != nil {
		return nil, promptTokens, completionTokens, err
	}
	// The client asked for its own model id, not Google's internal version.
	openAI["model"] = model
	openAI["created"] = time.Now().Unix()
	openAI["id"] = fmt.Sprintf("chatcmpl-ant-%d", time.Now().UnixNano())
	result, err := json.Marshal(openAI)
	return result, promptTokens, completionTokens, err
}

// streamAntigravityToOpenAI turns Cloud Code SSE (data: {"response": <gemini>})
// into OpenAI SSE. Role-only output is deferred so a refusal before the first
// content frame keeps the attempt retryable.
func (p *ProxyHandler) streamAntigravityToOpenAI(streamBody io.ReadCloser, w http.ResponseWriter, flusher http.Flusher, model string, commit *streamCommit) ([]byte, int, error) {
	if commit == nil {
		commit = &streamCommit{}
	}
	defer streamBody.Close()

	reqID := fmt.Sprintf("chatcmpl-ant-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	var collected []byte
	promptTokens, completionTokens := 0, 0
	finishReason := "stop"

	writeChunk := func(delta map[string]any, finish any, commits bool, usage map[string]any) error {
		chunk := map[string]any{
			"id":      reqID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []any{map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finish,
			}},
		}
		if usage != nil {
			chunk["usage"] = usage
		}
		raw, err := json.Marshal(chunk)
		if err != nil {
			return nil
		}
		frame := []byte("data: " + string(raw) + "\n\n")
		if commits {
			return commit.WriteFrame(w, flusher, frame)
		}
		return commit.DeferFrame(w, flusher, frame)
	}

	if err := writeChunk(map[string]any{"role": "assistant", "content": ""}, nil, false, nil); err != nil {
		return collected, completionTokens, err
	}

	var streamErr error
	scanner := bufio.NewScanner(streamBody)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var wrapper struct {
			Response map[string]any `json:"response"`
			Error    map[string]any `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &wrapper); err != nil {
			continue
		}
		if wrapper.Error != nil {
			message, _ := wrapper.Error["message"].(string)
			if message == "" {
				message = "upstream error"
			}
			streamErr = fmt.Errorf("antigravity: %s", message)
			break
		}
		resp := wrapper.Response
		if resp == nil {
			continue
		}
		if pt, ct := antigravityUsage(resp); pt > 0 || ct > 0 {
			promptTokens, completionTokens = pt, ct
		}
		candidates, _ := resp["candidates"].([]any)
		if len(candidates) == 0 {
			continue
		}
		cand, _ := candidates[0].(map[string]any)
		if text := antigravityTextParts(cand); text != "" {
			collected = append(collected, text...)
			if err := writeChunk(map[string]any{"content": text}, nil, true, nil); err != nil {
				return collected, completionTokens, err
			}
		}
		if fr, _ := cand["finishReason"].(string); fr != "" {
			finishReason = antigravityFinishReason(fr)
		}
	}
	if err := scanner.Err(); err != nil {
		streamErr = err
	}

	if streamErr != nil {
		if commit.Retryable() {
			return collected, completionTokens, streamErr
		}
		frame, _ := json.Marshal(map[string]any{
			"error": map[string]any{"message": streamErr.Error(), "type": "antigravity_error"},
		})
		_ = commit.WriteFrame(w, flusher, []byte("data: "+string(frame)+"\n\ndata: [DONE]\n\n"))
		return collected, completionTokens, streamErr
	}

	var usage map[string]any
	if promptTokens > 0 || completionTokens > 0 {
		usage = map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		}
	}
	if err := writeChunk(map[string]any{}, finishReason, true, usage); err != nil {
		return collected, completionTokens, err
	}
	if err := commit.WriteFrame(w, flusher, []byte("data: [DONE]\n\n")); err != nil {
		return collected, completionTokens, err
	}
	return collected, completionTokens, nil
}

func antigravityTextParts(candidate map[string]any) string {
	content, _ := candidate["content"].(map[string]any)
	parts, _ := content["parts"].([]any)
	var b strings.Builder
	for _, part := range parts {
		pm, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if thought, _ := pm["thought"].(bool); thought {
			continue
		}
		if text, ok := pm["text"].(string); ok {
			b.WriteString(text)
		}
	}
	return b.String()
}

func antigravityFinishReason(reason string) string {
	switch reason {
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "content_filter"
	default:
		return "stop"
	}
}

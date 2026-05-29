package translator

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Streaming translation
// ─────────────────────────────────────────────────────────────────────────────

// TranslateStream translates a streaming SSE body from srcFormat to dstFormat.
// Returns the translated SSE body bytes.
func TranslateStream(body io.Reader, srcFormat, dstFormat Format) ([]byte, error) {
	if srcFormat == dstFormat {
		return io.ReadAll(body)
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		translated := TranslateStreamLine(line, srcFormat, dstFormat)
		if translated != "" {
			buf.WriteString(translated)
		}
	}

	return buf.Bytes(), nil
}

// TranslateStreamLine translates a single SSE line between formats.
// Returns the translated SSE line (e.g., "data: {...}\n\n") or empty string
// if the line should be dropped.
func TranslateStreamLine(line string, srcFormat, dstFormat Format) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Parse based on source format
	switch srcFormat {
	case FormatAnthropic:
		return translateAnthropicStreamLine(line, dstFormat)
	case FormatGemini:
		return translateGeminiStreamLine(line, dstFormat)
	case FormatOpenAI:
		return translateOpenAIStreamLine(line, dstFormat)
	case FormatCohere:
		return translateCohereStreamLine(line, dstFormat)
	case FormatMistral:
		return translateMistralStreamLine(line, dstFormat)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Anthropic streaming → any format
// ─────────────────────────────────────────────────────────────────────────────

func translateAnthropicStreamLine(line string, dst Format) string {
	// Strip "event:" prefix
	if strings.HasPrefix(line, "event:") {
		return "" // event type lines are metadata, skip
	}

	// Strip "data:" prefix
	payload := line
	if strings.HasPrefix(line, "data:") {
		payload = strings.TrimSpace(line[5:])
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}

	eventType, _ := event["type"].(string)
	if eventType != "content_block_delta" {
		return ""
	}

	delta, _ := event["delta"].(map[string]any)
	deltaType, _ := delta["type"].(string)

	// Skip thinking deltas
	if deltaType == "thinking_delta" {
		return ""
	}

	var content string
	if deltaType == "text_delta" {
		content, _ = delta["text"].(string)
	} else if deltaType == "input_json_delta" {
		content, _ = delta["partial"].(string)
	}

	if content == "" {
		return ""
	}

	return formatStreamChunk(content, "", dst, event)
}

// ─────────────────────────────────────────────────────────────────────────────
// Gemini streaming → any format
// ─────────────────────────────────────────────────────────────────────────────

func translateGeminiStreamLine(line string, dst Format) string {
	payload := line
	if strings.HasPrefix(line, "data:") {
		payload = strings.TrimSpace(line[5:])
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}

	candidates, _ := event["candidates"].([]any)
	if len(candidates) == 0 {
		return ""
	}

	cand, _ := candidates[0].(map[string]any)
	content, _ := cand["content"].(map[string]any)
	parts, _ := content["parts"].([]any)

	var text string
	for _, part := range parts {
		pm, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := pm["text"].(string); ok {
			text += t
		}
	}

	if text == "" {
		return ""
	}

	fr, _ := cand["finishReason"].(string)
	finishReason := ""
	if fr != "" && fr != "FINISH_REASON_UNSPECIFIED" {
		finishReason = geminiFinishReasonToOpenAI(fr)
	}

	return formatStreamChunk(text, finishReason, dst, event)
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenAI streaming → any format
// ─────────────────────────────────────────────────────────────────────────────

func translateOpenAIStreamLine(line string, dst Format) string {
	payload := line
	if strings.HasPrefix(line, "data:") {
		payload = strings.TrimSpace(line[5:])
	}

	if payload == "[DONE]" {
		return ""
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}

	choices, _ := event["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}

	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	if delta == nil {
		return ""
	}

	content, _ := delta["content"].(string)
	if content == "" {
		return ""
	}

	finishReason, _ := choice["finish_reason"].(string)
	return formatStreamChunk(content, finishReason, dst, event)
}

// ─────────────────────────────────────────────────────────────────────────────
// Cohere streaming → any format
// ─────────────────────────────────────────────────────────────────────────────

func translateCohereStreamLine(line string, dst Format) string {
	payload := line
	if strings.HasPrefix(line, "data:") {
		payload = strings.TrimSpace(line[5:])
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}

	// Cohere streaming: "text" field in stream chunks
	text, _ := event["text"].(string)
	if text == "" {
		// Check for "delta" → "message" structure
		if delta, ok := event["delta"].(map[string]any); ok {
			text, _ = delta["message"].(string)
		}
	}
	if text == "" {
		return ""
	}

	return formatStreamChunk(text, "", dst, event)
}

// ─────────────────────────────────────────────────────────────────────────────
// Mistral streaming → any format
// ─────────────────────────────────────────────────────────────────────────────

func translateMistralStreamLine(line string, dst Format) string {
	payload := line
	if strings.HasPrefix(line, "data:") {
		payload = strings.TrimSpace(line[5:])
	}

	if payload == "[DONE]" {
		return ""
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}

	// Mistral stream is nearly identical to OpenAI
	choices, _ := event["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}

	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	if delta == nil {
		return ""
	}

	content, _ := delta["content"].(string)
	if content == "" {
		return ""
	}

	finishReason, _ := choice["finish_reason"].(string)
	return formatStreamChunk(content, finishReason, dst, event)
}

// ─────────────────────────────────────────────────────────────────────────────
// Stream output formatting
// ─────────────────────────────────────────────────────────────────────────────

// formatStreamChunk formats a content delta into the target format's SSE line.
func formatStreamChunk(content, finishReason string, dst Format, srcEvent map[string]any) string {
	switch dst {
	case FormatOpenAI, FormatMistral:
		chunk := map[string]any{
			"id":      "chatcmpl-stream",
			"object":  "chat.completion.chunk",
			"created": 0,
			"model":   "",
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"content": content,
					},
					"finish_reason": nil,
				},
			},
		}
		if finishReason != "" {
			choices := chunk["choices"].([]map[string]any)
			choices[0]["finish_reason"] = finishReason
		}
		return "data: " + mustJSON(chunk) + "\n\n"

	case FormatAnthropic:
		event := map[string]any{
			"type": "content_block_delta",
			"delta": map[string]any{
				"type": "text_delta",
				"text": content,
			},
		}
		return "event: content_block_delta\ndata: " + mustJSON(event) + "\n\n"

	case FormatGemini:
		event := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{{"text": content}},
						"role":  "model",
					},
				},
			},
		}
		if finishReason != "" {
			candidates := event["candidates"].([]map[string]any)
			candidates[0]["finishReason"] = finishReason
		}
		return "data: " + mustJSON(event) + "\n\n"

	case FormatCohere:
		event := map[string]any{
			"text": content,
		}
		return "data: " + mustJSON(event) + "\n\n"
	}

	return ""
}

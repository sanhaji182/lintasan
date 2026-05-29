package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Anthropic ↔ OpenAI request/response translation
// ─────────────────────────────────────────────────────────────────────────────

// AnthropicToOpenAIRequest converts an Anthropic Messages API request to OpenAI format.
func AnthropicToOpenAIRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)

	if model, ok := raw["model"].(string); ok {
		result["model"] = model
	}
	if stream, ok := raw["stream"].(bool); ok {
		result["stream"] = stream
	}

	// System prompt → system message
	var msgs []map[string]any
	if system, ok := raw["system"].(string); ok && system != "" {
		msgs = append(msgs, map[string]any{
			"role":    "system",
			"content": system,
		})
	}

	// Convert messages
	if messages, ok := raw["messages"].([]any); ok {
		msgs = append(msgs, anthropicMessagesToOpenAI(messages)...)
	}
	if len(msgs) > 0 {
		result["messages"] = msgs
	}

	// Parameters
	copyIfPresent(raw, result, "max_tokens")
	copyIfPresent(raw, result, "temperature")
	copyIfPresent(raw, result, "top_p")
	if stop, ok := raw["stop_sequences"]; ok {
		result["stop"] = stop
	}

	// Tools
	if tools, ok := raw["tools"].([]any); ok {
		result["tools"] = anthropicToolsToOpenAI(tools)
	}
	copyIfPresent(raw, result, "tool_choice")

	return result
}

// OpenAIToAnthropicRequest converts an OpenAI request to Anthropic Messages API format.
func OpenAIToAnthropicRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)

	if model, ok := raw["model"].(string); ok {
		result["model"] = model
	}
	if stream, ok := raw["stream"].(bool); ok {
		result["stream"] = stream
	}

	// Extract system prompt
	messages := extractMessages(raw)
	system, remaining := ExtractSystemPrompt(messages)
	if system != "" {
		result["system"] = system
	}

	// Convert messages
	if len(remaining) > 0 {
		result["messages"] = openaiMessagesToAnthropic(remaining)
	}

	// Parameters
	copyIfPresent(raw, result, "max_tokens")
	copyIfPresent(raw, result, "temperature")
	copyIfPresent(raw, result, "top_p")
	if stop, ok := raw["stop"]; ok {
		result["stop_sequences"] = stop
	}

	// Tools
	if tools, ok := raw["tools"].([]any); ok {
		result["tools"] = openaiToolsToAnthropic(tools)
	}
	copyIfPresent(raw, result, "tool_choice")

	return result
}

// AnthropicResponseToOpenAI converts an Anthropic response to OpenAI format.
func AnthropicResponseToOpenAI(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
	}

	var textContent string
	var toolCalls []map[string]any
	if content, ok := resp["content"].([]any); ok {
		for _, block := range content {
			bm, ok := block.(map[string]any)
			if !ok {
				continue
			}
			switch bm["type"] {
			case "text":
				if t, ok := bm["text"].(string); ok {
					textContent += t
				}
			case "tool_use":
				args, _ := json.Marshal(bm["input"])
				toolCalls = append(toolCalls, map[string]any{
					"id":   bm["id"],
					"type": "function",
					"function": map[string]any{
						"name":      bm["name"],
						"arguments": string(args),
					},
				})
			}
		}
	}

	model, _ := resp["model"].(string)
	id, _ := resp["id"].(string)
	if id == "" {
		id = fmt.Sprintf("anthropic-%s", model)
	}

	message := map[string]any{
		"role":    "assistant",
		"content": textContent,
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	finishReason := "stop"
	if sr, ok := resp["stop_reason"].(string); ok {
		finishReason = anthropicStopReasonToOpenAI(sr)
	}

	usage := extractUsageAnthropic(resp)

	openAI := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": resp["created_at"],
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
	}
	if usage != nil {
		openAI["usage"] = usage
	}

	return json.Marshal(openAI)
}

// OpenAIResponseToAnthropic converts an OpenAI response to Anthropic format.
func OpenAIResponseToAnthropic(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal openai response: %w", err)
	}

	model, _ := resp["model"].(string)
	var contentBlocks []map[string]any

	if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		if msg, ok := choice["message"].(map[string]any); ok {
			if text, ok := msg["content"].(string); ok && text != "" {
				contentBlocks = append(contentBlocks, map[string]any{
					"type": "text",
					"text": text,
				})
			}
			if tc, ok := msg["tool_calls"].([]any); ok {
				for _, t := range tc {
					tcMap, _ := t.(map[string]any)
					fn, _ := tcMap["function"].(map[string]any)
					var input any
					if args, ok := fn["arguments"].(string); ok {
						json.Unmarshal([]byte(args), &input)
					}
					if input == nil {
						input = map[string]any{}
					}
					contentBlocks = append(contentBlocks, map[string]any{
						"type":  "tool_use",
						"id":    tcMap["id"],
						"name":  fn["name"],
						"input": input,
					})
				}
			}
		}
	}

	if contentBlocks == nil {
		contentBlocks = []map[string]any{}
	}

	stopReason := "end_turn"
	if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		if fr, ok := choice["finish_reason"].(string); ok {
			stopReason = openaiFinishReasonToAnthropic(fr)
		}
	}

	result := map[string]any{
		"id":          fmt.Sprintf("msg_%s", randomID()),
		"type":        "message",
		"role":        "assistant",
		"model":       model,
		"content":     contentBlocks,
		"stop_reason": stopReason,
	}

	if usage, ok := resp["usage"].(map[string]any); ok {
		result["usage"] = map[string]any{
			"input_tokens":  intFromAny(usage["prompt_tokens"]),
			"output_tokens": intFromAny(usage["completion_tokens"]),
		}
	}

	return json.Marshal(result)
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func anthropicMessagesToOpenAI(messages []any) []map[string]any {
	var result []map[string]any
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		openaiMsg := map[string]any{"role": role}

		content := msg["content"]
		switch c := content.(type) {
		case string:
			openaiMsg["content"] = c
		case []any:
			var textParts []string
			for _, block := range c {
				bm, ok := block.(map[string]any)
				if !ok {
					continue
				}
				if bm["type"] == "text" {
					if t, ok := bm["text"].(string); ok {
						textParts = append(textParts, t)
					}
				}
			}
			openaiMsg["content"] = strings.Join(textParts, "")
		}
		result = append(result, openaiMsg)
	}
	return result
}

func openaiMessagesToAnthropic(messages []map[string]any) []map[string]any {
	var result []map[string]any
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content := msg["content"]

		var blocks []map[string]any
		switch c := content.(type) {
		case string:
			blocks = []map[string]any{{"type": "text", "text": c}}
		case []any:
			for _, item := range c {
				im, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if im["type"] == "text" {
					blocks = append(blocks, map[string]any{"type": "text", "text": im["text"]})
				}
			}
		}
		if blocks == nil {
			blocks = []map[string]any{}
		}

		result = append(result, map[string]any{
			"role":    role,
			"content": blocks,
		})
	}
	return result
}

func anthropicToolsToOpenAI(tools []any) []map[string]any {
	var result []map[string]any
	for _, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tm["name"],
				"description": tm["description"],
				"parameters":  tm["input_schema"],
			},
		})
	}
	return result
}

func openaiToolsToAnthropic(tools []any) []map[string]any {
	var result []map[string]any
	for _, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		fn, ok := tm["function"].(map[string]any)
		if !ok {
			continue
		}
		result = append(result, map[string]any{
			"name":         fn["name"],
			"description":  fn["description"],
			"input_schema": fn["parameters"],
		})
	}
	return result
}

func anthropicStopReasonToOpenAI(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

func openaiFinishReasonToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return "end_turn"
	}
}

func extractUsageAnthropic(resp map[string]any) map[string]any {
	usage, ok := resp["usage"].(map[string]any)
	if !ok {
		return nil
	}
	inputTokens := intFromAny(usage["input_tokens"])
	outputTokens := intFromAny(usage["output_tokens"])
	return map[string]any{
		"prompt_tokens":     inputTokens,
		"completion_tokens": outputTokens,
		"total_tokens":      inputTokens + outputTokens,
	}
}

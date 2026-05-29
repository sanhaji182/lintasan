package translator

import (
	"encoding/json"
	"fmt"
)

// ─────────────────────────────────────────────────────────────────────────────
// Cohere ↔ OpenAI request/response translation
// ─────────────────────────────────────────────────────────────────────────────

// CohereToOpenAIRequest converts a Cohere Chat API request to OpenAI format.
func CohereToOpenAIRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)
	result["model"] = raw["model"]

	// Cohere has "message" (single user message) and "chat_history" (prior turns)
	var msgs []map[string]any

	// preamble → system
	if preamble, ok := raw["preamble"].(string); ok && preamble != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": preamble})
	}

	// chat_history → messages
	if history, ok := raw["chat_history"].([]any); ok {
		for _, h := range history {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			role, _ := hm["role"].(string)
			message, _ := hm["message"].(string)
			if role == "CHATBOT" {
				role = "assistant"
			} else if role == "USER" {
				role = "user"
			}
			if message != "" {
				msgs = append(msgs, map[string]any{"role": role, "content": message})
			}
		}
	}

	// message → user message
	if message, ok := raw["message"].(string); ok && message != "" {
		msgs = append(msgs, map[string]any{"role": "user", "content": message})
	}

	if len(msgs) > 0 {
		result["messages"] = msgs
	}

	// Parameters
	copyIfPresent(raw, result, "temperature")
	copyIfPresent(raw, result, "top_p")
	if v, ok := raw["max_tokens"]; ok {
		result["max_tokens"] = v
	}
	if v, ok := raw["max_output_tokens"]; ok {
		result["max_tokens"] = v
	}
	if stop, ok := raw["stop_sequences"]; ok {
		result["stop"] = stop
	}
	if stream, ok := raw["stream"].(bool); ok {
		result["stream"] = stream
	}

	// Tools
	if tools, ok := raw["tools"].([]any); ok {
		result["tools"] = cohereToolsToOpenAI(tools)
	}

	return result
}

// OpenAIToCohereRequest converts an OpenAI request to Cohere Chat API format.
func OpenAIToCohereRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)

	if model, ok := raw["model"].(string); ok {
		result["model"] = model
	}
	if stream, ok := raw["stream"].(bool); ok {
		result["stream"] = stream
	}

	messages := extractMessages(raw)
	system, remaining := ExtractSystemPrompt(messages)
	if system != "" {
		result["preamble"] = system
	}

	// Convert messages: last user message → "message", rest → "chat_history"
	var chatHistory []map[string]any
	var lastUserMsg string

	for _, msg := range remaining {
		role, _ := msg["role"].(string)
		text := extractText(msg["content"])
		if role == "user" {
			if lastUserMsg != "" {
				chatHistory = append(chatHistory, map[string]any{
					"role":    "USER",
					"message": lastUserMsg,
				})
			}
			lastUserMsg = text
		} else if role == "assistant" {
			if lastUserMsg != "" {
				chatHistory = append(chatHistory, map[string]any{
					"role":    "USER",
					"message": lastUserMsg,
				})
				lastUserMsg = ""
			}
			chatHistory = append(chatHistory, map[string]any{
				"role":    "CHATBOT",
				"message": text,
			})
		}
	}

	if lastUserMsg != "" {
		result["message"] = lastUserMsg
	}
	if len(chatHistory) > 0 {
		result["chat_history"] = chatHistory
	}

	// Parameters
	copyIfPresent(raw, result, "temperature")
	copyIfPresent(raw, result, "top_p")
	copyIfPresentRename(raw, result, "max_tokens", "max_tokens")
	if stop, ok := raw["stop"]; ok {
		result["stop_sequences"] = stop
	}

	// Tools
	if tools, ok := raw["tools"].([]any); ok {
		result["tools"] = openaiToolsToCohere(tools)
	}

	return result
}

// CohereResponseToOpenAI converts a Cohere Chat response to OpenAI format.
func CohereResponseToOpenAI(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal cohere response: %w", err)
	}

	text, _ := resp["text"].(string)
	model, _ := resp["model"].(string)
	if model == "" {
		model, _ = resp["model_name"].(string)
	}

	finishReason := "stop"
	if fr, ok := resp["finish_reason"].(string); ok {
		finishReason = cohereFinishReasonToOpenAI(fr)
	}

	id, _ := resp["generation_id"].(string)
	if id == "" {
		id = fmt.Sprintf("cohere-%s", model)
	}

	openAI := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": text},
				"finish_reason": finishReason,
			},
		},
	}

	// Usage (Cohere uses "meta" → "tokens")
	if meta, ok := resp["meta"].(map[string]any); ok {
		if tokens, ok := meta["tokens"].(map[string]any); ok {
			inputTokens := intFromAny(tokens["input_tokens"])
			outputTokens := intFromAny(tokens["output_tokens"])
			openAI["usage"] = map[string]any{
				"prompt_tokens":     inputTokens,
				"completion_tokens": outputTokens,
				"total_tokens":      inputTokens + outputTokens,
			}
		}
	}

	return json.Marshal(openAI)
}

// OpenAIResponseToCohere converts an OpenAI response to Cohere format.
func OpenAIResponseToCohere(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal openai response: %w", err)
	}

	var text string
	if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		if msg, ok := choice["message"].(map[string]any); ok {
			text, _ = msg["content"].(string)
		}
	}

	finishReason := "COMPLETE"
	if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		if fr, ok := choice["finish_reason"].(string); ok {
			finishReason = openaiFinishReasonToCohere(fr)
		}
	}

	model, _ := resp["model"].(string)
	result := map[string]any{
		"text":          text,
		"model":         model,
		"finish_reason": finishReason,
	}

	if usage, ok := resp["usage"].(map[string]any); ok {
		result["meta"] = map[string]any{
			"tokens": map[string]any{
				"input_tokens":  intFromAny(usage["prompt_tokens"]),
				"output_tokens": intFromAny(usage["completion_tokens"]),
			},
		}
	}

	return json.Marshal(result)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func cohereFinishReasonToOpenAI(fr string) string {
	switch fr {
	case "COMPLETE":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "ERROR", "ERROR_TOXIC":
		return "content_filter"
	default:
		return "stop"
	}
}

func openaiFinishReasonToCohere(fr string) string {
	switch fr {
	case "stop":
		return "COMPLETE"
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "ERROR_TOXIC"
	default:
		return "COMPLETE"
	}
}

func cohereToolsToOpenAI(tools []any) []map[string]any {
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
				"parameters":  tm["parameter_definitions"],
			},
		})
	}
	return result
}

func openaiToolsToCohere(tools []any) []map[string]any {
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
			"name":               fn["name"],
			"description":        fn["description"],
			"parameter_definitions": fn["parameters"],
		})
	}
	return result
}

package translator

import (
	"encoding/json"
	"fmt"
)

// ─────────────────────────────────────────────────────────────────────────────
// OpenAI format — canonical internal representation
// ─────────────────────────────────────────────────────────────────────────────

// BuildOpenAIResponse creates a standard OpenAI chat completion response map.
func BuildOpenAIResponse(model, content, finishReason string, usage map[string]int) map[string]any {
	resp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%s", randomID()),
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": finishReason,
			},
		},
	}
	if usage != nil {
		resp["usage"] = map[string]any{
			"prompt_tokens":     usage["prompt_tokens"],
			"completion_tokens": usage["completion_tokens"],
			"total_tokens":      usage["total_tokens"],
		}
	}
	return resp
}

// BuildOpenAIStreamChunk creates a standard OpenAI streaming chunk map.
func BuildOpenAIStreamChunk(model, content string, finishReason *string) map[string]any {
	chunk := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%s", randomID()),
		"object":  "chat.completion.chunk",
		"created": 0,
		"model":   model,
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
	if finishReason != nil {
		choices := chunk["choices"].([]map[string]any)
		choices[0]["finish_reason"] = *finishReason
	}
	return chunk
}

// OpenAIToolsToMap converts raw tools to normalized maps.
func OpenAIToolsToMap(tools []any) []map[string]any {
	var result []map[string]any
	for _, t := range tools {
		if toolMap, ok := t.(map[string]any); ok {
			result = append(result, toolMap)
		}
	}
	return result
}

// randomID generates a deterministic-looking ID (no crypto/rand needed).
func randomID() string {
	return "lintasan_0000000000"
}

// mustJSON marshals to JSON string; returns "{}" on error.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

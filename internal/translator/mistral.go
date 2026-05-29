package translator

import (
	"encoding/json"
	"fmt"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mistral ↔ OpenAI request/response translation
// ─────────────────────────────────────────────────────────────────────────────

// MistralToOpenAIRequest converts a Mistral AI request to OpenAI format.
// Mistral's API is very similar to OpenAI — main differences are
// "safe_prompt" field and some parameter naming.
func MistralToOpenAIRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)

	// Model passthrough
	copyIfPresent(raw, result, "model")
	copyIfPresent(raw, result, "stream")
	copyIfPresent(raw, result, "temperature")
	copyIfPresent(raw, result, "top_p")
	copyIfPresent(raw, result, "max_tokens")

	// Messages — Mistral uses same format as OpenAI
	if messages, ok := raw["messages"].([]any); ok {
		result["messages"] = messages
	}

	// Tools — same format
	copyIfPresent(raw, result, "tools")
	copyIfPresent(raw, result, "tool_choice")

	// stop → stop
	copyIfPresent(raw, result, "stop")

	// random_seed → not in OpenAI, ignore
	// safe_prompt → not in OpenAI, ignore

	return result
}

// OpenAIToMistralRequest converts an OpenAI request to Mistral format.
func OpenAIToMistralRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)

	copyIfPresent(raw, result, "model")
	copyIfPresent(raw, result, "stream")
	copyIfPresent(raw, result, "temperature")
	copyIfPresent(raw, result, "top_p")
	copyIfPresent(raw, result, "max_tokens")
	copyIfPresent(raw, result, "messages")
	copyIfPresent(raw, result, "tools")
	copyIfPresent(raw, result, "tool_choice")
	copyIfPresent(raw, result, "stop")

	// Mistral-specific: safe_prompt defaults to false
	result["safe_prompt"] = false

	return result
}

// MistralResponseToOpenAI converts a Mistral Chat Completion response to OpenAI format.
// Mistral responses are nearly identical to OpenAI.
func MistralResponseToOpenAI(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal mistral response: %w", err)
	}

	// Mistral uses "id" with "cmpl-" prefix, same choices structure.
	// Just normalize model name if needed.
	model, _ := resp["model"].(string)

	// Build OpenAI response — already nearly identical
	openAI := map[string]any{
		"id":      resp["id"],
		"object":  "chat.completion",
		"created": resp["created"],
		"model":   model,
	}

	if choices, ok := resp["choices"].([]any); ok {
		openAI["choices"] = choices
	} else {
		openAI["choices"] = []map[string]any{}
	}

	if usage, ok := resp["usage"].(map[string]any); ok {
		openAI["usage"] = map[string]any{
			"prompt_tokens":     intFromAny(usage["prompt_tokens"]),
			"completion_tokens": intFromAny(usage["completion_tokens"]),
			"total_tokens":      intFromAny(usage["total_tokens"]),
		}
	}

	return json.Marshal(openAI)
}

// OpenAIResponseToMistral converts an OpenAI response to Mistral format.
func OpenAIResponseToMistral(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal openai response: %w", err)
	}

	// Mistral format is essentially the same as OpenAI
	result := map[string]any{
		"id":      resp["id"],
		"object":  "chat.completion",
		"created": resp["created"],
		"model":   resp["model"],
	}

	if choices, ok := resp["choices"].([]any); ok {
		result["choices"] = choices
	} else {
		result["choices"] = []map[string]any{}
	}

	if usage, ok := resp["usage"].(map[string]any); ok {
		result["usage"] = usage
	}

	return json.Marshal(result)
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool translation helpers
// ─────────────────────────────────────────────────────────────────────────────

// TranslateToolDefinitions translates tool definitions between formats.
func TranslateToolDefinitions(tools []any, srcFormat, dstFormat Format) []any {
	if srcFormat == dstFormat {
		return tools
	}

	// Step 1: convert to OpenAI canonical
	var openaiTools []map[string]any
	switch srcFormat {
	case FormatOpenAI:
		for _, t := range tools {
			if tm, ok := t.(map[string]any); ok {
				openaiTools = append(openaiTools, tm)
			}
		}
	case FormatAnthropic:
		openaiTools = anthropicToolsToOpenAI(tools)
	case FormatGemini:
		openaiTools = geminiToolsToOpenAI(tools)
	case FormatCohere:
		openaiTools = cohereToolsToOpenAI(tools)
	case FormatMistral:
		for _, t := range tools {
			if tm, ok := t.(map[string]any); ok {
				openaiTools = append(openaiTools, tm)
			}
		}
	}

	// Step 2: convert from OpenAI to destination
	switch dstFormat {
	case FormatOpenAI:
		result := make([]any, len(openaiTools))
		for i, t := range openaiTools {
			result[i] = t
		}
		return result
	case FormatAnthropic:
		anyTools := make([]any, len(openaiTools))
		for i, t := range openaiTools {
			anyTools[i] = t
		}
		return toAnySlice(openaiToolsToAnthropic(anyTools))
	case FormatGemini:
		anyTools := make([]any, len(openaiTools))
		for i, t := range openaiTools {
			anyTools[i] = t
		}
		return toAnySlice(openaiToolsToGemini(anyTools))
	case FormatCohere:
		anyTools := make([]any, len(openaiTools))
		for i, t := range openaiTools {
			anyTools[i] = t
		}
		return toAnySlice(openaiToolsToCohere(anyTools))
	case FormatMistral:
		result := make([]any, len(openaiTools))
		for i, t := range openaiTools {
			result[i] = t
		}
		return result
	}

	return tools
}

// geminiToolsToOpenAI converts Gemini tool declarations to OpenAI format.
func geminiToolsToOpenAI(tools []any) []map[string]any {
	var result []map[string]any
	for _, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		// Gemini wraps in functionDeclarations
		if decls, ok := tm["functionDeclarations"].([]any); ok {
			for _, d := range decls {
				dm, ok := d.(map[string]any)
				if !ok {
					continue
				}
				result = append(result, map[string]any{
					"type": "function",
					"function": map[string]any{
						"name":        dm["name"],
						"description": dm["description"],
						"parameters":  dm["parameters"],
					},
				})
			}
		}
	}
	return result
}

func toAnySlice[T any](s []T) []any {
	result := make([]any, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}

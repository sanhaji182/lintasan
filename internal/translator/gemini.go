package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Gemini ↔ OpenAI request/response translation
// ─────────────────────────────────────────────────────────────────────────────

// GeminiToOpenAIRequest converts a Gemini REST API request to OpenAI format.
func GeminiToOpenAIRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)

	// Model
	if model, ok := raw["model"].(string); ok {
		result["model"] = strings.TrimPrefix(model, "models/")
	}

	// System instruction → system message
	var msgs []map[string]any
	if si, ok := raw["systemInstruction"].(map[string]any); ok {
		if parts, ok := si["parts"].([]any); ok {
			var text string
			for _, p := range parts {
				if pm, ok := p.(map[string]any); ok {
					if t, ok := pm["text"].(string); ok {
						text += t
					}
				}
			}
			if text != "" {
				msgs = append(msgs, map[string]any{"role": "system", "content": text})
			}
		}
	}

	// Contents → messages
	if contents, ok := raw["contents"].([]any); ok {
		for _, c := range contents {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			geminiRole, _ := cm["role"].(string)
			role := "user"
			if geminiRole == "model" {
				role = "assistant"
			}

			var text string
			if parts, ok := cm["parts"].([]any); ok {
				for _, p := range parts {
					pm, ok := p.(map[string]any)
					if !ok {
						continue
					}
					if t, ok := pm["text"].(string); ok {
						text += t
					}
				}
			}
			if text != "" {
				msgs = append(msgs, map[string]any{"role": role, "content": text})
			}
		}
	}

	if len(msgs) > 0 {
		result["messages"] = msgs
	}

	// Generation config
	if gc, ok := raw["generationConfig"].(map[string]any); ok {
		copyIfPresent(gc, result, "temperature")
		copyIfPresent(gc, result, "top_p")
		if v, ok := gc["maxOutputTokens"]; ok {
			result["max_tokens"] = v
		}
		if v, ok := gc["topK"]; ok {
			result["top_k"] = v
		}
		if v, ok := gc["stopSequences"]; ok {
			result["stop"] = v
		}
	}

	return result
}

// OpenAIToGeminiRequest converts an OpenAI request to Gemini REST API format.
func OpenAIToGeminiRequest(raw map[string]any) map[string]any {
	result := make(map[string]any)

	if model, ok := raw["model"].(string); ok {
		result["model"] = strings.TrimPrefix(model, "models/")
	}

	messages := extractMessages(raw)

	// System → systemInstruction
	system, remaining := ExtractSystemPrompt(messages)
	if system != "" {
		result["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": system}},
		}
	}

	// Messages → contents
	var contents []map[string]any
	for _, msg := range remaining {
		role, _ := msg["role"].(string)
		geminiRole := "user"
		if role == "assistant" {
			geminiRole = "model"
		}

		text := extractText(msg["content"])
		if text == "" {
			continue
		}
		contents = append(contents, map[string]any{
			"role":  geminiRole,
			"parts": []map[string]any{{"text": text}},
		})
	}
	if len(contents) > 0 {
		result["contents"] = contents
	}

	// Generation config
	genConfig := make(map[string]any)
	if v, ok := raw["max_tokens"]; ok {
		genConfig["maxOutputTokens"] = v
	}
	copyIfPresent(raw, genConfig, "temperature")
	copyIfPresent(raw, genConfig, "top_p")
	if v, ok := raw["top_k"]; ok {
		genConfig["topK"] = v
	}
	if v, ok := raw["stop"]; ok {
		genConfig["stopSequences"] = v
	}
	if len(genConfig) > 0 {
		result["generationConfig"] = genConfig
	}

	// Tools
	if tools, ok := raw["tools"].([]any); ok && len(tools) > 0 {
		result["tools"] = openaiToolsToGemini(tools)
	}

	return result
}

// GeminiResponseToOpenAI converts a Gemini response to OpenAI format.
func GeminiResponseToOpenAI(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini response: %w", err)
	}

	var textContent string
	var finishReason string

	if candidates, ok := resp["candidates"].([]any); ok && len(candidates) > 0 {
		cand, _ := candidates[0].(map[string]any)
		if content, ok := cand["content"].(map[string]any); ok {
			if parts, ok := content["parts"].([]any); ok {
				for _, part := range parts {
					pm, ok := part.(map[string]any)
					if !ok {
						continue
					}
					if t, ok := pm["text"].(string); ok {
						textContent += t
					}
				}
			}
		}
		if fr, ok := cand["finishReason"].(string); ok {
			finishReason = geminiFinishReasonToOpenAI(fr)
		}
	}

	usage := extractUsageGemini(resp)
	model, _ := resp["model"].(string)

	openAI := map[string]any{
		"id":      fmt.Sprintf("gemini-%s", model),
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": textContent},
				"finish_reason": finishReason,
			},
		},
	}
	if usage != nil {
		openAI["usage"] = usage
	}

	return json.Marshal(openAI)
}

// OpenAIResponseToGemini converts an OpenAI response to Gemini format.
func OpenAIResponseToGemini(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal openai response: %w", err)
	}

	var textContent string
	if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		if msg, ok := choice["message"].(map[string]any); ok {
			textContent, _ = msg["content"].(string)
		}
	}

	finishReason := "STOP"
	if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		if fr, ok := choice["finish_reason"].(string); ok {
			finishReason = openaiFinishReasonToGemini(fr)
		}
	}

	model, _ := resp["model"].(string)

	result := map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"parts": []map[string]any{{"text": textContent}},
					"role":  "model",
				},
				"finishReason": finishReason,
			},
		},
		"modelVersion": model,
	}

	if usage, ok := resp["usage"].(map[string]any); ok {
		result["usageMetadata"] = map[string]any{
			"promptTokenCount":     intFromAny(usage["prompt_tokens"]),
			"candidatesTokenCount": intFromAny(usage["completion_tokens"]),
			"totalTokenCount":      intFromAny(usage["total_tokens"]),
		}
	}

	return json.Marshal(result)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func geminiFinishReasonToOpenAI(fr string) string {
	switch fr {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "MALFORMED_FUNCTION_CALL":
		return "content_filter"
	default:
		return "stop"
	}
}

func openaiFinishReasonToGemini(fr string) string {
	switch fr {
	case "stop":
		return "STOP"
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	default:
		return "STOP"
	}
}

func extractUsageGemini(resp map[string]any) map[string]any {
	um, ok := resp["usageMetadata"].(map[string]any)
	if !ok {
		return nil
	}
	prompt := intFromAny(um["promptTokenCount"])
	completion := intFromAny(um["candidatesTokenCount"])
	return map[string]any{
		"prompt_tokens":     prompt,
		"completion_tokens": completion,
		"total_tokens":      prompt + completion,
	}
}

func openaiToolsToGemini(tools []any) []map[string]any {
	var declarations []map[string]any
	for _, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		fn, ok := tm["function"].(map[string]any)
		if !ok {
			continue
		}
		declarations = append(declarations, map[string]any{
			"name":        fn["name"],
			"description": fn["description"],
			"parameters":  fn["parameters"],
		})
	}
	if len(declarations) == 0 {
		return nil
	}
	return []map[string]any{
		{"functionDeclarations": declarations},
	}
}

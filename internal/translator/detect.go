package translator

import (
	"encoding/json"
	"strings"
)

// Format represents a supported API format.
type Format string

const (
	FormatOpenAI    Format = "openai"
	FormatAnthropic Format = "anthropic"
	FormatGemini    Format = "gemini"
	FormatCohere    Format = "cohere"
	FormatMistral   Format = "mistral"
)

// AllFormats returns all supported formats.
func AllFormats() []Format {
	return []Format{FormatOpenAI, FormatAnthropic, FormatGemini, FormatCohere, FormatMistral}
}

// DetectFormat auto-detects the API format from a JSON request body.
// It examines structural markers unique to each format.
func DetectFormat(body []byte) Format {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return FormatOpenAI // default fallback
	}
	return DetectFormatFromMap(raw)
}

// DetectFormatFromMap detects format from a parsed JSON map.
func DetectFormatFromMap(raw map[string]any) Format {
	// Anthropic: has "messages" with content blocks AND has "max_tokens" but no "model" at top level
	// OR has "system" as top-level string field alongside messages
	// Key markers: "system" top-level field, content blocks with "type"/"source"
	if _, ok := raw["system"].(string); ok {
		if _, ok := raw["messages"]; ok {
			return FormatAnthropic
		}
	}
	if _, ok := raw["anthropic_version"]; ok {
		return FormatAnthropic
	}

	// Gemini: has "contents" array (not "messages"), "generationConfig", "systemInstruction"
	if _, ok := raw["contents"]; ok {
		if _, ok := raw["messages"]; !ok {
			return FormatGemini
		}
	}
	if _, ok := raw["generationConfig"]; ok {
		return FormatGemini
	}
	if _, ok := raw["systemInstruction"]; ok {
		return FormatGemini
	}

	// Cohere: has "message" (singular) or "chat_history", "preamble"
	if _, ok := raw["chat_history"]; ok {
		return FormatCohere
	}
	if _, ok := raw["preamble"]; ok {
		return FormatCohere
	}
	if _, ok := raw["message"]; ok {
		if _, ok := raw["messages"]; !ok {
			return FormatCohere
		}
	}

	// Mistral: has "messages" + "tools" with specific Mistral-style structure
	// or "safe_prompt" field which is Mistral-specific
	if _, ok := raw["safe_prompt"]; ok {
		return FormatMistral
	}

	// OpenAI: has "messages" array with role/content, "model", optionally "tools", "stream"
	// This is the default/dominant format
	if _, ok := raw["messages"]; ok {
		return FormatOpenAI
	}

	return FormatOpenAI
}

// DetectResponseFormat auto-detects the format of an API response.
func DetectResponseFormat(body []byte) Format {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return FormatOpenAI
	}

	// Anthropic response: has "content" array with type blocks, "stop_reason"
	if _, ok := raw["stop_reason"]; ok {
		if _, ok := raw["content"]; ok {
			return FormatAnthropic
		}
	}
	if _, ok := raw["anthropic_version"]; ok {
		return FormatAnthropic
	}

	// Gemini response: has "candidates" array, "usageMetadata"
	if _, ok := raw["candidates"]; ok {
		if _, ok := raw["usageMetadata"]; ok {
			return FormatGemini
		}
		return FormatGemini
	}

	// Cohere response: has "text" or "generation_id", "finish_reason"
	if _, ok := raw["generation_id"]; ok {
		return FormatCohere
	}
	if _, ok := raw["text"]; ok {
		if _, ok := raw["finish_reason"]; ok {
			return FormatCohere
		}
	}

	// Mistral response: has "choices" like OpenAI but also "id" with "cmpl-" prefix
	// or "model" with mistral prefix
	if model, ok := raw["model"].(string); ok {
		if strings.Contains(strings.ToLower(model), "mistral") {
			return FormatMistral
		}
	}

	// OpenAI response: has "choices" array with "message" or "delta"
	if _, ok := raw["choices"]; ok {
		return FormatOpenAI
	}

	return FormatOpenAI
}

// DetectStreamFormat detects format from a streaming SSE line.
func DetectStreamFormat(line string) Format {
	line = strings.TrimSpace(line)

	// Strip "data:" prefix
	if strings.HasPrefix(line, "data:") {
		line = strings.TrimSpace(line[5:])
	}
	if strings.HasPrefix(line, "event:") {
		// Anthropic uses "event:" lines
		return FormatAnthropic
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return FormatOpenAI
	}

	// Anthropic stream: has "type" field (content_block_delta, message_start, etc.)
	if typ, ok := raw["type"].(string); ok {
		switch typ {
		case "content_block_delta", "content_block_start", "content_block_stop",
			"message_start", "message_delta", "message_stop":
			return FormatAnthropic
		}
	}

	// Gemini stream: has "candidates"
	if _, ok := raw["candidates"]; ok {
		return FormatGemini
	}

	// OpenAI stream: has "choices" with "delta"
	if _, ok := raw["choices"]; ok {
		return FormatOpenAI
	}

	return FormatOpenAI
}

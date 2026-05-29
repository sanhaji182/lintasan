package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Translate is the main entry point: converts a request body from srcFormat to dstFormat.
func Translate(body []byte, srcFormat, dstFormat Format, isResponse bool) ([]byte, error) {
	if srcFormat == dstFormat {
		return body, nil
	}
	if isResponse {
		return translateResponse(body, srcFormat, dstFormat)
	}
	return translateRequest(body, srcFormat, dstFormat)
}

// translateRequest converts a request body from src to dst format.
func translateRequest(body []byte, src, dst Format) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	openaiReq, err := toOpenAIRequest(raw, src)
	if err != nil {
		return nil, fmt.Errorf("convert %s → openai: %w", src, err)
	}

	result, err := fromOpenAIRequest(openaiReq, dst)
	if err != nil {
		return nil, fmt.Errorf("convert openai → %s: %w", dst, err)
	}

	return json.Marshal(result)
}

// translateResponse converts a response body from src to dst format.
func translateResponse(body []byte, src, dst Format) ([]byte, error) {
	openaiResp, err := toOpenAIResponse(body, src)
	if err != nil {
		return nil, fmt.Errorf("convert %s → openai response: %w", src, err)
	}

	result, err := fromOpenAIResponse(openaiResp, dst)
	if err != nil {
		return nil, fmt.Errorf("convert openai → %s response: %w", dst, err)
	}

	return result, nil
}

func toOpenAIRequest(raw map[string]any, format Format) (map[string]any, error) {
	switch format {
	case FormatOpenAI:
		return raw, nil
	case FormatAnthropic:
		return AnthropicToOpenAIRequest(raw), nil
	case FormatGemini:
		return GeminiToOpenAIRequest(raw), nil
	case FormatCohere:
		return CohereToOpenAIRequest(raw), nil
	case FormatMistral:
		return MistralToOpenAIRequest(raw), nil
	default:
		return nil, fmt.Errorf("unsupported source format: %s", format)
	}
}

func fromOpenAIRequest(raw map[string]any, format Format) (map[string]any, error) {
	switch format {
	case FormatOpenAI:
		return raw, nil
	case FormatAnthropic:
		return OpenAIToAnthropicRequest(raw), nil
	case FormatGemini:
		return OpenAIToGeminiRequest(raw), nil
	case FormatCohere:
		return OpenAIToCohereRequest(raw), nil
	case FormatMistral:
		return OpenAIToMistralRequest(raw), nil
	default:
		return nil, fmt.Errorf("unsupported destination format: %s", format)
	}
}

func toOpenAIResponse(body []byte, format Format) ([]byte, error) {
	switch format {
	case FormatOpenAI:
		return body, nil
	case FormatAnthropic:
		return AnthropicResponseToOpenAI(body)
	case FormatGemini:
		return GeminiResponseToOpenAI(body)
	case FormatCohere:
		return CohereResponseToOpenAI(body)
	case FormatMistral:
		return MistralResponseToOpenAI(body)
	default:
		return nil, fmt.Errorf("unsupported source format: %s", format)
	}
}

func fromOpenAIResponse(body []byte, format Format) ([]byte, error) {
	switch format {
	case FormatOpenAI:
		return body, nil
	case FormatAnthropic:
		return OpenAIResponseToAnthropic(body)
	case FormatGemini:
		return OpenAIResponseToGemini(body)
	case FormatCohere:
		return OpenAIResponseToCohere(body)
	case FormatMistral:
		return OpenAIResponseToMistral(body)
	default:
		return nil, fmt.Errorf("unsupported destination format: %s", format)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared utility helpers
// ─────────────────────────────────────────────────────────────────────────────

// ExtractSystemPrompt extracts system/developer messages and returns the
// combined system prompt string and remaining non-system messages.
func ExtractSystemPrompt(messages []map[string]any) (string, []map[string]any) {
	var systemParts []string
	var remaining []map[string]any

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" || role == "developer" {
			text := extractText(msg["content"])
			if text != "" {
				systemParts = append(systemParts, text)
			}
		} else {
			remaining = append(remaining, msg)
		}
	}

	return strings.Join(systemParts, "\n\n"), remaining
}

// extractMessages extracts the messages array from a raw map as []map[string]any.
func extractMessages(raw map[string]any) []map[string]any {
	msgs, ok := raw["messages"].([]any)
	if !ok {
		return nil
	}
	var result []map[string]any
	for _, m := range msgs {
		if msg, ok := m.(map[string]any); ok {
			result = append(result, msg)
		}
	}
	return result
}

// copyIfPresent copies a key from src to dst if it exists and is non-nil.
func copyIfPresent(src, dst map[string]any, key string) {
	if v, ok := src[key]; ok && v != nil {
		dst[key] = v
	}
}

// copyIfPresentRename copies src[srcKey] to dst[dstKey].
func copyIfPresentRename(src, dst map[string]any, srcKey, dstKey string) {
	if v, ok := src[srcKey]; ok && v != nil {
		dst[dstKey] = v
	}
}

// intFromAny converts a numeric any to int.
func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

// extractText extracts text from string or content array.
func extractText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var result string
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					result += t
				}
			}
		}
		return result
	}
	return ""
}

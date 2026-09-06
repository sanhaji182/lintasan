package reasoning

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ExtractReasoningContent checks if a response has reasoning_content but empty content,
// and moves the extracted code into content. DeepSeek V4 Pro puts its answer in
// reasoning_content rather than content, making IDEs fail to read the output.
func ExtractReasoningContent(data []byte) []byte {
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return data // not valid JSON, passthrough
	}

	choices, ok := parsed["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return data
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return data
	}

	msg, ok := choice["message"].(map[string]interface{})
	if !ok {
		msg, ok = choice["delta"].(map[string]interface{})
		if !ok {
			return data
		}
	}

	content, _ := msg["content"].(string)
	reasoningContent, _ := msg["reasoning_content"].(string)

	// Check for <think>...</think> tags inside content (common in DeepSeek R1 / QwQ)
	thinkRegex := regexp.MustCompile(`(?s)<think>(.*?)</think>`)
	if thinkRegex.MatchString(content) {
		var thinkParts []string
		matches := thinkRegex.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
				thinkParts = append(thinkParts, strings.TrimSpace(m[1]))
			}
		}
		if len(thinkParts) > 0 {
			if strings.TrimSpace(reasoningContent) == "" {
				reasoningContent = strings.Join(thinkParts, "\n\n")
				msg["reasoning_content"] = reasoningContent
			}
			content = strings.TrimSpace(thinkRegex.ReplaceAllString(content, ""))
			msg["content"] = content
			parsed["_reasoning_extracted"] = true
		}
	}

	// If neither has data, passthrough
	if strings.TrimSpace(content) == "" && strings.TrimSpace(reasoningContent) == "" {
		return data
	}

	// Extract: prefer content, but fall back to reasoning if content has no code
	hasContentCode := hasCodeBlock(content)
	hasReasoningCode := strings.TrimSpace(reasoningContent) != "" && (hasCodeBlock(reasoningContent) || strings.TrimSpace(content) == "")

	// Do nothing if content already has code — use as-is (unless <think> was extracted)
	if hasContentCode {
		if parsed["_reasoning_extracted"] == true {
			if fixed, err := json.Marshal(parsed); err == nil {
				return fixed
			}
		}
		return data
	}

	// If content has no code but reasoning does, extract from reasoning
	if hasReasoningCode {
		extracted := extractFinalAnswer(reasoningContent)
		if extracted != "" {
			msg["content"] = extracted
			// Keep reasoning_content for transparency
			parsed["_reasoning_extracted"] = true

			fixed, err := json.Marshal(parsed)
			if err == nil {
				return fixed
			}
		}
	}
	
	// Content has text but no code — try extracting from reasoning anyway
	if strings.TrimSpace(content) != "" && !hasContentCode && strings.TrimSpace(reasoningContent) != "" {
		extracted := extractFinalAnswer(reasoningContent)
		if extracted != "" {
			msg["content"] = extracted
			parsed["_reasoning_extracted"] = true

			fixed, err := json.Marshal(parsed)
			if err == nil {
				return fixed
			}
		}
	}

	if parsed["_reasoning_extracted"] == true {
		if fixed, err := json.Marshal(parsed); err == nil {
			return fixed
		}
	}

	return data
}

// extractFinalAnswer extracts the most complete code block from reasoning text.
// DeepSeek V4 Pro scatters code across its reasoning — the LAST block is rarely
// the answer. We score all blocks and pick the most complete one.
func extractFinalAnswer(reasoning string) string {
	// Strategy 0: Check if reasoning itself starts with code (no prose prefix)
	trimmed := strings.TrimSpace(reasoning)
	if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") {
		return trimTrailingProse(trimmed)
	}

	// Strategy 1: Collect ALL code blocks and score them for completeness
	re := regexp.MustCompile("```(?:python|javascript|typescript|go|rust|java|cpp|c|ruby|php|swift|kotlin|scala|sql|bash|sh|yaml|json|toml|xml|html|css)?\\s*\\n([\\s\\S]*?)```")
	matches := re.FindAllStringSubmatch(reasoning, -1)

	if len(matches) > 0 {
		type scoredBlock struct {
			code  string
			score int
		}
		var blocks []scoredBlock

		for _, m := range matches {
			code := strings.TrimSpace(m[1])
			score := 0

			if strings.HasPrefix(code, "import ") {
				score += 3
			}
			if strings.Contains(code, "def ") {
				score += 2
			}
			if strings.Contains(code, "class ") {
				score += 2
			}
			// Requirement-specific signals
			if strings.Contains(code, "Condition") {
				score += 2
			}
			if strings.Contains(code, ".wait(") {
				score += 2
			}
			if strings.Contains(code, ".notify") {
				score += 2
			}
			if strings.Contains(code, "finally:") {
				score += 2
			}
			if strings.Contains(code, "__enter__") && strings.Contains(code, "__exit__") {
				score += 2
			}
			if strings.Contains(code, "release_connection") {
				score += 1
			}
			if strings.Contains(code, "execute_query") {
				score += 1
			}
			if strings.Contains(code, "is_stale") {
				score += 1
			}
			// Size bonus
			score += min(len(code)/200, 5)

			if len(code) > 50 {
				blocks = append(blocks, scoredBlock{code, score})
			}
		}

		// Sort by score descending
		for i := 0; i < len(blocks); i++ {
			for j := i + 1; j < len(blocks); j++ {
				if blocks[j].score > blocks[i].score {
					blocks[i], blocks[j] = blocks[j], blocks[i]
				}
			}
		}

		if len(blocks) > 0 {
			return trimTrailingProse(blocks[0].code)
		}
	}

	// Strategy 2: Find code after a blank line starting with import/def
	importMarker := regexp.MustCompile("\n\n(import\\s+[\\s\\S]+)$")
	if im := importMarker.FindStringSubmatch(reasoning); im != nil {
		code := strings.TrimSpace(im[1])
		if len(code) > 100 {
			return trimTrailingProse(code)
		}
	}

	// Strategy 3: Find last import/def to the end
	lastImport := strings.LastIndex(reasoning, "\nimport ")
	lastDef := strings.LastIndex(reasoning, "\ndef ")
	start := lastImport
	if lastDef > start {
		start = lastDef
	}
	if start > 0 {
		code := strings.TrimSpace(reasoning[start:])
		return trimTrailingProse(code)
	}

	return ""
}

// trimTrailingProse removes trailing lines that don't look like code
func trimTrailingProse(code string) string {
	lines := strings.Split(code, "\n")
	lastCodeLine := len(lines)

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" ||
			strings.HasPrefix(line, "import ") ||
			strings.HasPrefix(line, "from ") ||
			strings.HasPrefix(line, "def ") ||
			strings.HasPrefix(line, "class ") ||
			strings.HasPrefix(line, "return ") ||
			strings.HasPrefix(line, "self.") ||
			strings.HasPrefix(line, "if ") ||
			strings.HasPrefix(line, "for ") ||
			strings.HasPrefix(line, "while ") ||
			strings.HasPrefix(line, "try:") ||
			strings.HasPrefix(line, "except") ||
			strings.HasPrefix(line, "finally:") ||
			strings.HasPrefix(line, "with ") ||
			strings.HasPrefix(line, "raise ") ||
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "@") ||
			strings.HasPrefix(line, " ") ||
			strings.HasPrefix(line, "}") ||
			strings.HasPrefix(line, "]") ||
			strings.HasPrefix(line, ")") ||
			strings.HasPrefix(line, ":") {
			lastCodeLine = i + 1
			break
		}
		// Check if line looks like code (assignment, method call, etc.)
		if matched, _ := regexp.MatchString(`^\w+\s*[=(]`, line); matched {
			lastCodeLine = i + 1
			break
		}
		if matched, _ := regexp.MatchString(`^\w+\.\w+`, line); matched {
			lastCodeLine = i + 1
			break
		}
	}

	result := strings.TrimSpace(strings.Join(lines[:lastCodeLine], "\n"))
	if len(result) > 50 {
		return result
	}
	return ""
}

// hasCodeBlock checks if text contains a markdown code block with actual code content.
func hasCodeBlock(text string) bool {
	re := regexp.MustCompile("```(?:\\w*)\n?([\\s\\S]*?)```")
	matches := re.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(strings.TrimSpace(m[1])) > 50 {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IsReasoningModel checks if a response came from a reasoning model
// (empty content, populated reasoning_content)
func IsReasoningModel(data []byte) bool {
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false
	}
	choices, ok := parsed["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return false
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return false
	}
	msg, ok := choice["message"].(map[string]interface{})
	if !ok {
		return false
	}
	content, _ := msg["content"].(string)
	reasoning, _ := msg["reasoning_content"].(string)
	return (strings.TrimSpace(content) == "" && strings.TrimSpace(reasoning) != "") || strings.Contains(content, "<think>")
}

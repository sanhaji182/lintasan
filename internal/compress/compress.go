// Package compress provides context compression for LLM conversation histories.
// When token count exceeds threshold, it keeps system + last N messages intact
// and summarizes middle messages using regex-based extraction of key information.
package compress

import (
	"fmt"
	"regexp"
	"strings"
)

// Stats holds compression statistics for logging/debugging.
type Stats struct {
	OriginalTokens int     `json:"original_tokens"`
	CompressedTokens int   `json:"compressed_tokens"`
	CompressionRatio float64 `json:"compression_ratio"`
	MessagesBefore   int    `json:"messages_before"`
	MessagesAfter    int    `json:"messages_after"`
	WasCompressed    bool   `json:"was_compressed"`
}

// Compressor handles context compression for LLM messages.
type Compressor struct {
	maxTokens         int // max tokens before compression kicks in
	keepLastN         int // number of recent messages to keep intact
	compressThreshold int // token threshold to trigger compression
}

// New creates a new Compressor with the specified parameters.
// maxTokens: maximum allowed input tokens (compression threshold)
// keepLastN: number of recent messages to preserve without compression
func New(maxTokens, keepLastN, compressThreshold int) *Compressor {
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	if keepLastN <= 0 {
		keepLastN = 6
	}
	if compressThreshold <= 0 {
		compressThreshold = 8000
	}
	return &Compressor{
		maxTokens:         maxTokens,
		keepLastN:         keepLastN,
		compressThreshold: compressThreshold,
	}
}

// Compress analyzes the messages and compresses them if token count exceeds threshold.
// Returns compressed messages and statistics about the compression.
func (c *Compressor) Compress(messages []map[string]any) ([]map[string]any, Stats) {
	stats := Stats{
		OriginalTokens: estimateTokens(messages),
		MessagesBefore: len(messages),
	}

	// Passthrough if under threshold
	if stats.OriginalTokens <= c.compressThreshold {
		stats.CompressedTokens = stats.OriginalTokens
		stats.MessagesAfter = len(messages)
		return messages, stats
	}

	// Perform compression
	compressed := c.compressMessages(messages)

	stats.CompressedTokens = estimateTokens(compressed)
	stats.MessagesAfter = len(compressed)
	stats.WasCompressed = true

	// Calculate ratio safely
	if stats.OriginalTokens > 0 {
		stats.CompressionRatio = 1.0 - float64(stats.CompressedTokens)/float64(stats.OriginalTokens)
	}

	return compressed, stats
}

// compressMessages performs the actual compression logic.
func (c *Compressor) compressMessages(messages []map[string]any) []map[string]any {
	if len(messages) <= c.keepLastN+1 {
		return messages // Not enough messages to compress
	}

	// Identify message categories
	var systemMsgs []map[string]any
	var middleMsgs []map[string]any
	var lastMsgs []map[string]any

	for i, msg := range messages {
		role := getRole(msg)

		if role == "system" {
			systemMsgs = append(systemMsgs, msg)
		} else if i >= len(messages)-c.keepLastN {
			lastMsgs = append(lastMsgs, msg)
		} else {
			middleMsgs = append(middleMsgs, msg)
		}
	}

	// If no middle messages to compress, return as-is
	if len(middleMsgs) == 0 {
		return messages
	}

	// Build summary of middle messages
	summary := c.summarizeMessages(middleMsgs)
	summaryMsg := map[string]any{
		"role":    "system",
		"content": summary,
	}

	// Reconstruct: system + summary + last messages
	result := make([]map[string]any, 0, len(systemMsgs)+1+len(lastMsgs))
	result = append(result, systemMsgs...)
	result = append(result, summaryMsg)
	result = append(result, lastMsgs...)

	return result
}

// summarizeMessages creates a compact summary of middle messages using regex extraction.
func (c *Compressor) summarizeMessages(messages []map[string]any) string {
	var parts []string

	// Extract code blocks
	codeBlocks := extractCodeBlocks(messages)
	if len(codeBlocks) > 0 {
		parts = append(parts, "Code snippets discussed:")
		for i, block := range codeBlocks {
			if i >= 5 {
				parts = append(parts, "  [...]")
				break
			}
			// Truncate long code blocks
			if len(block) > 300 {
				block = block[:300] + "\n  [...]"
			}
			parts = append(parts, "  "+block)
		}
	}

	// Extract errors
	errors := extractErrors(messages)
	if len(errors) > 0 {
		parts = append(parts, "Errors/issues encountered:")
		for _, err := range errors {
			if len(err) > 200 {
				err = err[:200] + "..."
			}
			parts = append(parts, "  - "+err)
		}
	}

	// Extract decisions
	decisions := extractDecisions(messages)
	if len(decisions) > 0 {
		parts = append(parts, "Decisions made:")
		for _, d := range decisions {
			if len(d) > 150 {
				d = d[:150] + "..."
			}
			parts = append(parts, "  - "+d)
		}
	}

	// Extract tool calls
	toolCalls := extractToolCalls(messages)
	if len(toolCalls) > 0 {
		parts = append(parts, "Tools/functions used:")
		for _, tc := range toolCalls {
			if len(tc) > 150 {
				tc = tc[:150] + "..."
			}
			parts = append(parts, "  - "+tc)
		}
	}

	// Extract tool results
	toolResults := extractToolResults(messages)
	if len(toolResults) > 0 {
		parts = append(parts, "Tool results:")
		for _, tr := range toolResults {
			if len(tr) > 200 {
				tr = tr[:200] + "..."
			}
			parts = append(parts, "  - "+tr)
		}
	}

	// If we didn't extract anything specific, create a generic summary
	if len(parts) == 0 {
		var topics []string
		for _, msg := range messages {
			content := getContent(msg)
			if len(content) > 100 {
				// Get first meaningful sentence
				firstSent := extractFirstSentence(content)
				if firstSent != "" {
					topics = append(topics, firstSent)
				}
			}
		}
		if len(topics) > 0 {
			parts = append(parts, "Topics discussed:")
			for _, t := range topics {
				if len(t) > 150 {
					t = t[:150] + "..."
				}
				parts = append(parts, "  - "+t)
			}
		}
	}

	// Build final summary
	var sb strings.Builder
	sb.WriteString("[Previous conversation summary - ")
	sb.WriteString(fmt.Sprintf("%d", len(messages)))
	sb.WriteString(" messages compressed]\n\n")

	if len(parts) > 0 {
		sb.WriteString(strings.Join(parts, "\n"))
	} else {
		sb.WriteString("(No extractable content from previous messages)")
	}

	return sb.String()
}

// Helper functions

func getRole(msg map[string]any) string {
	if role, ok := msg["role"].(string); ok {
		return role
	}
	return ""
}

func getContent(msg map[string]any) string {
	switch v := msg["content"].(type) {
	case string:
		return v
	case []any:
		// Handle content as array (e.g., [{"type": "text", "text": "..."}])
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// Regex patterns for extraction
var (
	// Code block patterns (markdown and inline)
	codeBlockPattern = regexp.MustCompile("```(?:\\w+)?\\n?([\\s\\S]*?)```")
	inlineCodePattern = regexp.MustCompile("`([^`]+)`")

	// Error patterns
	errorPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:error[:\s]+[^\n]{0,200})`),
		regexp.MustCompile(`(?i)(?:panic[:\s]+[^\n]{0,200})`),
		regexp.MustCompile(`(?i)(?:failed[:\s]+[^\n]{0,200})`),
		regexp.MustCompile(`(?i)(?:exception[:\s]+[^\n]{0,200})`),
		regexp.MustCompile(`(?i)(?:warning[:\s]+[^\n]{0,200})`),
		regexp.MustCompile(`(?i)(?:cannot|can't|couldn't|unable to)[^\n]{0,150}`),
	}

	// Decision patterns
	decisionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:decided|agreed|should|must|need to|will use|going with)[^\n.]{0,150}`),
		regexp.MustCompile(`(?i)(?:let's go with|we'll use|we should|i'll use)[^\n.]{0,150}`),
		regexp.MustCompile(`(?i)(?:the solution is|the approach is|the plan is)[^\n.]{0,150}`),
		regexp.MustCompile(`(?i)(?:implemented|created|built|generated)[^\n.]{0,150}`),
	}

	// Tool call patterns
	toolCallPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:tool_call|function_call)[:\s]*(\w+)`),
		regexp.MustCompile(`(?i)(?:calling|called)[:\s]*(\w+)`),
		regexp.MustCompile(`(?i)(?:invoke|invoked)[:\s]*(\w+)`),
	}

	// Tool result patterns
	toolResultPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:tool result|function result)[:\s]*([^\n]{0,200})`),
		regexp.MustCompile(`(?i)(?:returned|result)[:\s]*([^\n]{0,200})`),
	}
)

func extractCodeBlocks(messages []map[string]any) []string {
	var blocks []string
	seen := make(map[string]bool)

	for _, msg := range messages {
		content := getContent(msg)

		// Extract markdown code blocks
		matches := codeBlockPattern.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			code := strings.TrimSpace(m[1])
			// Deduplicate similar blocks
			normalized := normalizeCode(code)
			if !seen[normalized] && len(code) > 20 {
				seen[normalized] = true
				blocks = append(blocks, code)
			}
		}

		// Also check for tool_calls content
		if tc, ok := msg["tool_calls"].([]any); ok {
			for _, call := range tc {
				if callMap, ok := call.(map[string]any); ok {
					funcName := ""
					if fn, ok := callMap["function"].(map[string]any); ok {
						if name, ok := fn["name"].(string); ok {
							funcName = name
						}
						if args, ok := fn["arguments"].(string); ok {
							code := "function " + funcName + "(" + args + ")"
							normalized := normalizeCode(code)
							if !seen[normalized] && len(code) > 30 {
								seen[normalized] = true
								blocks = append(blocks, code)
							}
						}
					}
				}
			}
		}
	}

	return blocks
}

func extractErrors(messages []map[string]any) []string {
	var errors []string
	seen := make(map[string]bool)

	for _, msg := range messages {
		content := getContent(msg)

		for _, pattern := range errorPatterns {
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, m := range matches {
				err := strings.TrimSpace(m[0])
				// Normalize for deduplication
				normalized := strings.ToLower(err[:min(len(err), 80)])
				if !seen[normalized] && len(err) > 10 {
					seen[normalized] = true
					errors = append(errors, err)
				}
			}
		}
	}

	return errors
}

func extractDecisions(messages []map[string]any) []string {
	var decisions []string
	seen := make(map[string]bool)

	for _, msg := range messages {
		content := getContent(msg)

		for _, pattern := range decisionPatterns {
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, m := range matches {
				dec := strings.TrimSpace(m[0])
				normalized := strings.ToLower(dec[:min(len(dec), 80)])
				if !seen[normalized] && len(dec) > 15 {
					seen[normalized] = true
					decisions = append(decisions, dec)
				}
			}
		}
	}

	return decisions
}

func extractToolCalls(messages []map[string]any) []string {
	var calls []string
	seen := make(map[string]bool)

	for _, msg := range messages {
		content := getContent(msg)

		for _, pattern := range toolCallPatterns {
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, m := range matches {
				if len(m) > 1 {
					call := strings.TrimSpace(m[1])
					if !seen[call] && len(call) > 0 {
						seen[call] = true
						calls = append(calls, call)
					}
				}
			}
		}

		// Check tool_calls field
		if tc, ok := msg["tool_calls"].([]any); ok {
			for _, call := range tc {
				if callMap, ok := call.(map[string]any); ok {
					if fn, ok := callMap["function"].(map[string]any); ok {
						if name, ok := fn["name"].(string); ok {
							if !seen[name] {
								seen[name] = true
								calls = append(calls, name)
							}
						}
					}
				}
			}
		}
	}

	return calls
}

func extractToolResults(messages []map[string]any) []string {
	var results []string
	seen := make(map[string]bool)

	for _, msg := range messages {
		content := getContent(msg)

		for _, pattern := range toolResultPatterns {
			matches := pattern.FindAllStringSubmatch(content, -1)
			for _, m := range matches {
				if len(m) > 1 {
					result := strings.TrimSpace(m[1])
					normalized := strings.ToLower(result[:min(len(result), 80)])
					if !seen[normalized] && len(result) > 10 {
						seen[normalized] = true
						results = append(results, result)
					}
				}
			}
		}

		// Check tool role messages
		if role := getRole(msg); role == "tool" {
			content := getContent(msg)
			if len(content) > 0 {
				normalized := strings.ToLower(content[:min(len(content), 80)])
				if !seen[normalized] {
					seen[normalized] = true
					results = append(results, content)
				}
			}
		}
	}

	return results
}

func extractFirstSentence(content string) string {
	// Find first sentence ending
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		return ""
	}

	// Look for sentence boundaries
	for i, ch := range trimmed {
		if ch == '.' || ch == '!' || ch == '?' {
			if i > 20 { // Minimum meaningful sentence
				return trimmed[:i+1]
			}
		}
	}

	// No sentence boundary found, return truncated
	if len(trimmed) > 100 {
		return trimmed[:100] + "..."
	}
	return trimmed
}

func normalizeCode(code string) string {
	// Remove variable names and specific values to find similar patterns
	normalized := strings.ToLower(code)
	// Remove common variable patterns
	normalized = regexp.MustCompile(`\w+:\s*`).ReplaceAllString(normalized, "")
	normalized = regexp.MustCompile(`\d+`).ReplaceAllString(normalized, "X")
	normalized = strings.TrimSpace(normalized)
	return normalized[:min(len(normalized), 100)]
}

// estimateTokens estimates token count using character-based heuristic.
// This is a rough approximation: 1 token ≈ 4 characters for English.
func estimateTokens(messages []map[string]any) int {
	var total int
	for _, msg := range messages {
		content := getContent(msg)
		total += estimateTokensStr(content)
	}
	return total
}

func estimateTokensStr(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + 3) / 4 // Rough: 1 token per 4 chars
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Default compressor instance
var defaultCompressor = New(8000, 6, 8000)

// Compress is a convenience function using default settings.
func Compress(messages []map[string]any) ([]map[string]any, Stats) {
	return defaultCompressor.Compress(messages)
}

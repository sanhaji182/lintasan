package compress

import (
	"encoding/json"
	"testing"
)

// TestNewCompressor tests Compressor creation with various parameters.
func TestNewCompressor(t *testing.T) {
	tests := []struct {
		name              string
		maxTokens         int
		keepLastN         int
		compressThreshold int
		wantMaxTokens     int
		wantKeepLastN     int
	}{
		{"defaults", 0, 0, 0, 8000, 6},
		{"custom values", 10000, 4, 10000, 10000, 4},
		{"negative values", -1, -5, -10, 8000, 6},
		{"zero values", 0, 0, 0, 8000, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.maxTokens, tt.keepLastN, tt.compressThreshold)
			if c.maxTokens != tt.wantMaxTokens {
				t.Errorf("maxTokens = %d, want %d", c.maxTokens, tt.wantMaxTokens)
			}
			if c.keepLastN != tt.wantKeepLastN {
				t.Errorf("keepLastN = %d, want %d", c.keepLastN, tt.wantKeepLastN)
			}
		})
	}
}

// TestCompressPassthrough tests that small messages pass through unchanged.
func TestCompressPassthrough(t *testing.T) {
	c := New(8000, 6, 8000)
	messages := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there!"},
	}

	compressed, stats := c.Compress(messages)

	if stats.WasCompressed {
		t.Error("expected WasCompressed to be false for small messages")
	}
	if len(compressed) != len(messages) {
		t.Errorf("expected %d messages, got %d", len(messages), len(compressed))
	}
	if stats.CompressionRatio != 0 {
		t.Error("expected zero compression ratio for small messages")
	}
}

// TestCompressThreshold tests compression triggers at threshold.
func TestCompressThreshold(t *testing.T) {
	c := New(2000, 2, 2000)

	// Create messages that exceed threshold (2000 tokens)
	largeContent := ""
	for i := 0; i < 100; i++ {
		largeContent += "This is a long line of text that adds up to more tokens. "
	}

	messages := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": "Question?"},
		{"role": "assistant", "content": "Answer!"},
	}

	originalTokens := estimateTokens(messages)
	t.Logf("Original tokens: %d", originalTokens)

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression to trigger")
	}
	if stats.OriginalTokens != originalTokens {
		t.Errorf("OriginalTokens = %d, want %d", stats.OriginalTokens, originalTokens)
	}
	if stats.CompressedTokens >= stats.OriginalTokens {
		t.Errorf("CompressedTokens (%d) should be less than OriginalTokens (%d)",
			stats.CompressedTokens, stats.OriginalTokens)
	}
	if stats.MessagesAfter >= stats.MessagesBefore {
		t.Error("MessagesAfter should be less than MessagesBefore after compression")
	}

	// Verify structure: system + summary + last 2 = 4 messages
	if len(compressed) != 4 {
		t.Errorf("expected 4 messages after compression, got %d", len(compressed))
	}

	// First should be system
	if compressed[0]["role"] != "system" {
		t.Error("first message should be system")
	}

	// Last should be last assistant
	if compressed[len(compressed)-1]["role"] != "assistant" {
		t.Error("last message should be assistant")
	}
}

// TestCompressKeepsSystemMessages tests that system messages are always preserved.
func TestCompressKeepsSystemMessages(t *testing.T) {
	c := New(1000, 2, 1000)

	largeContent := ""
	for i := 0; i < 50; i++ {
		largeContent += "Lorem ipsum dolor sit amet. "
	}

	messages := []map[string]any{
		{"role": "system", "content": "System prompt 1"},
		{"role": "system", "content": "System prompt 2"},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": "Question?"},
		{"role": "assistant", "content": "Answer!"},
	}

	compressed, _ := c.Compress(messages)

	// Count system messages in compressed result
	systemCount := 0
	for _, msg := range compressed {
		if msg["role"] == "system" {
			systemCount++
		}
	}

	if systemCount != 3 {
		// 2 original system + 1 summary message marked as system
		t.Errorf("expected 3 system-role messages, got %d", systemCount)
	}
}

// TestCompressExtractsCodeBlocks tests code block extraction.
func TestCompressExtractsCodeBlocks(t *testing.T) {
	c := New(500, 2, 500) // Lower threshold to trigger compression

	// Add enough content to exceed threshold
	largeContent := ""
	for i := 0; i < 40; i++ {
		largeContent += "Some additional context. "
	}

	messages := []map[string]any{
		{"role": "user", "content": "Here is my code:\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n``` " + largeContent},
		{"role": "assistant", "content": "I see your code." + largeContent},
		{"role": "user", "content": "And another:\n```python\ndef hello():\n    print(\"world\")\n``` " + largeContent},
		{"role": "assistant", "content": "Got it." + largeContent},
		{"role": "user", "content": "Fix it please" + largeContent},
		{"role": "assistant", "content": "Fixed!" + largeContent},
		{"role": "user", "content": "Thanks" + largeContent},
		{"role": "assistant", "content": "You're welcome!" + largeContent},
	}

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Find the summary message
	var summaryContent string
	for _, msg := range compressed {
		if msg["role"] == "system" {
			if content, ok := msg["content"].(string); ok {
				if len(content) > 30 && !containsOriginalSystemPrompt(content) {
					summaryContent = content
					break
				}
			}
		}
	}

	if summaryContent == "" {
		t.Error("expected summary content")
	}

	// Check code blocks were extracted
	if !contains(summaryContent, "func main()") {
		t.Error("expected Go code block to be in summary")
	}
	if !contains(summaryContent, "def hello()") {
		t.Error("expected Python code block to be in summary")
	}
}

func containsOriginalSystemPrompt(s string) bool {
	return contains(s, "You are a helpful assistant")
}

// TestCompressExtractsErrors tests error message extraction.
func TestCompressExtractsErrors(t *testing.T) {
	c := New(500, 2, 500) // Lower threshold

	largeContent := ""
	for i := 0; i < 30; i++ {
		largeContent += "Additional context. "
	}

	messages := []map[string]any{
		{"role": "user", "content": "I'm getting an error:\n```\nError: connection refused\n``` " + largeContent},
		{"role": "assistant", "content": "Try restarting the service." + largeContent},
		{"role": "user", "content": "Still failing:\n```\npanic: out of memory\n``` " + largeContent},
		{"role": "assistant", "content": "Add more RAM." + largeContent},
		{"role": "user", "content": "It works now!" + largeContent},
		{"role": "assistant", "content": "Great!" + largeContent},
		{"role": "user", "content": "Thanks" + largeContent},
		{"role": "assistant", "content": "Welcome" + largeContent},
	}

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Check errors were extracted in summary
	hasErrors := false
	for _, msg := range compressed {
		if content, ok := msg["content"].(string); ok {
			if contains(content, "Error") || contains(content, "panic") {
				hasErrors = true
				break
			}
		}
	}

	if !hasErrors {
		t.Error("expected errors to be extracted in summary")
	}
}

// TestCompressExtractsDecisions tests decision extraction.
func TestCompressExtractsDecisions(t *testing.T) {
	c := New(500, 2, 500) // Lower threshold

	largeContent := ""
	for i := 0; i < 30; i++ {
		largeContent += "Additional context. "
	}

	messages := []map[string]any{
		{"role": "user", "content": "Should we use Redis or Memcached? " + largeContent},
		{"role": "assistant", "content": "We decided to use Redis because it supports more data structures. " + largeContent},
		{"role": "user", "content": "Good choice. Let's proceed. " + largeContent},
		{"role": "assistant", "content": "Agreed. Implementing now. " + largeContent},
		{"role": "user", "content": "The implementation is complete. " + largeContent},
		{"role": "assistant", "content": "Excellent work!" + largeContent},
		{"role": "user", "content": "Run tests" + largeContent},
		{"role": "assistant", "content": "Tests passed!" + largeContent},
	}

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Check decisions were extracted
	hasDecisions := false
	for _, msg := range compressed {
		if content, ok := msg["content"].(string); ok {
			if contains(content, "decided") || contains(content, "Agreed") {
				hasDecisions = true
				break
			}
		}
	}

	if !hasDecisions {
		t.Error("expected decisions to be extracted in summary")
	}
}

// TestCompressToolCalls tests tool call extraction.
func TestCompressToolCalls(t *testing.T) {
	c := New(500, 2, 500) // Lower threshold

	largeContent := ""
	for i := 0; i < 30; i++ {
		largeContent += "Additional context. "
	}

	messages := []map[string]any{
		{"role": "user", "content": "Search for weather in NYC " + largeContent},
		{"role": "assistant", "content": "", "tool_calls": []any{
			map[string]any{
				"function": map[string]any{
					"name":      "get_weather",
					"arguments": "{\"location\": \"NYC\"}",
				},
			},
		}},
		{"role": "tool", "content": "The weather in NYC is 72°F and sunny. " + largeContent},
		{"role": "assistant", "content": "It's sunny and 72°F in NYC. " + largeContent},
		{"role": "user", "content": "Thanks" + largeContent},
		{"role": "assistant", "content": "You're welcome!" + largeContent},
	}

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Check tool calls were extracted
	hasToolCalls := false
	for _, msg := range compressed {
		if content, ok := msg["content"].(string); ok {
			if contains(content, "get_weather") || contains(content, "Tools") {
				hasToolCalls = true
				break
			}
		}
	}

	if !hasToolCalls {
		t.Error("expected tool calls to be extracted in summary")
	}
}

// TestCompressToolResults tests tool result extraction.
func TestCompressToolResults(t *testing.T) {
	c := New(500, 2, 500) // Lower threshold

	largeContent := ""
	for i := 0; i < 30; i++ {
		largeContent += "Additional context. "
	}

	messages := []map[string]any{
		{"role": "tool", "content": "File listing: src/main.go, src/lib.go, tests/main_test.go " + largeContent},
		{"role": "assistant", "content": "I found 3 Go files. " + largeContent},
		{"role": "user", "content": "Good" + largeContent},
		{"role": "assistant", "content": "Good!" + largeContent},
		{"role": "user", "content": "Continue" + largeContent},
		{"role": "assistant", "content": "Continuing..." + largeContent},
		{"role": "user", "content": "Ok" + largeContent},
		{"role": "assistant", "content": "Done!" + largeContent},
	}

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Check tool results were extracted
	hasToolResults := false
	for _, msg := range compressed {
		if content, ok := msg["content"].(string); ok {
			if contains(content, "File listing") || contains(content, "Tool result") {
				hasToolResults = true
				break
			}
		}
	}

	if !hasToolResults {
		t.Error("expected tool results to be extracted in summary")
	}
}

// TestCompressEmptyMessages tests handling of empty message list.
func TestCompressEmptyMessages(t *testing.T) {
	c := New(8000, 6, 8000)
	messages := []map[string]any{}

	compressed, stats := c.Compress(messages)

	if len(compressed) != 0 {
		t.Errorf("expected 0 messages, got %d", len(compressed))
	}
	if stats.OriginalTokens != 0 {
		t.Error("expected 0 original tokens")
	}
}

// TestCompressSingleMessage tests handling of single message.
func TestCompressSingleMessage(t *testing.T) {
	c := New(8000, 6, 8000)
	messages := []map[string]any{
		{"role": "user", "content": "Hello"},
	}

	compressed, stats := c.Compress(messages)

	if len(compressed) != 1 {
		t.Errorf("expected 1 message, got %d", len(compressed))
	}
	if stats.WasCompressed {
		t.Error("single message should not be compressed")
	}
}

// TestCompressKeepLastN tests that exactly keepLastN messages are kept.
func TestCompressKeepLastN(t *testing.T) {
	keepLastN := 3
	c := New(500, keepLastN, 500) // Lower threshold

	largeContent := ""
	for i := 0; i < 50; i++ {
		largeContent += "Long content here. "
	}

	messages := []map[string]any{
		{"role": "system", "content": "System"},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": "Response 1"},
		{"role": "user", "content": "Message to keep 1"},
		{"role": "assistant", "content": "Response to keep 2"},
		{"role": "user", "content": "Message to keep 3"},
		{"role": "assistant", "content": "Response to keep 4"},
	}

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Check last N messages are preserved (system + summary + 3 last = 5 messages)
	expected := 1 + 1 + keepLastN
	if len(compressed) != expected {
		t.Errorf("expected %d messages, got %d", expected, len(compressed))
	}

	// Verify the last messages are preserved (not summarized)
	lastMsg := compressed[len(compressed)-1]
	if lastMsg["content"] != "Response to keep 4" {
		t.Error("last message content was changed")
	}
}

// TestCompressContentArray tests handling of messages with content as array.
func TestCompressContentArray(t *testing.T) {
	c := New(500, 2, 500) // Lower threshold

	largeContent := ""
	for i := 0; i < 30; i++ {
		largeContent += "Additional context. "
	}

	messages := []map[string]any{
		{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": "Hello "},
			map[string]any{"type": "text", "text": "world " + largeContent},
		}},
		{"role": "assistant", "content": "Hi! " + largeContent},
		{"role": "user", "content": "Code: " + largeContent},
		{"role": "assistant", "content": "```\ntest\n``` " + largeContent},
		{"role": "user", "content": "Thanks" + largeContent},
		{"role": "assistant", "content": "Welcome!" + largeContent},
		{"role": "user", "content": "More" + largeContent},
		{"role": "assistant", "content": "More!" + largeContent},
		{"role": "user", "content": "End" + largeContent},
		{"role": "assistant", "content": "End!" + largeContent},
	}

	compressed, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Verify array content was handled - find the "End!" message in the last N preserved
	if len(compressed) > 0 {
		// With keepLastN=2, the last 2 original messages should be preserved
		// The "End!" message should be in one of the last positions
		foundEnd := false
		for _, msg := range compressed[len(compressed)-2:] {
			if msg["content"] == "End!"+largeContent {
				foundEnd = true
				break
			}
		}
		if !foundEnd {
			t.Errorf("expected to find 'End!' in last messages, got: %v",
				compressed[len(compressed)-2:])
		}
	}
}

// TestEstimateTokens tests token estimation.
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		messages []map[string]any
		min      int
		max      int
	}{
		{
			"empty",
			[]map[string]any{},
			0, 0,
		},
		{
			"short messages",
			[]map[string]any{
				{"role": "user", "content": "Hello"},
			},
			1, 5,
		},
		{
			"long message",
			[]map[string]any{
				{"role": "user", "content": "This is a much longer message that should have more tokens. " +
					"Adding more content to make it longer. " +
					"More sentences here. " +
					"Even more text to increase token count."},
			},
			10, 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := estimateTokens(tt.messages)
			if tokens < tt.min || tokens > tt.max {
				t.Errorf("estimateTokens() = %d, want between %d and %d", tokens, tt.min, tt.max)
			}
		})
	}
}

// TestDefaultCompressor tests the package-level Compress function.
func TestDefaultCompressor(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "System"},
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi!"},
	}

	compressed, stats := Compress(messages)

	if stats.OriginalTokens != stats.CompressedTokens {
		t.Error("default compressor should not compress short messages")
	}
	if len(compressed) != len(messages) {
		t.Error("default compressor should preserve message count for short messages")
	}
}

// TestCompressionRatio tests that compression ratio is calculated correctly.
func TestCompressionRatio(t *testing.T) {
	c := New(1000, 2, 1000)

	largeContent := ""
	for i := 0; i < 200; i++ {
		largeContent += "This is a long line of text. "
	}

	messages := []map[string]any{
		{"role": "system", "content": "System"},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": "Response"},
		{"role": "user", "content": "Keep this"},
		{"role": "assistant", "content": "Keep this too"},
		{"role": "user", "content": "Keep"},
		{"role": "assistant", "content": "Keep!"},
	}

	_, stats := c.Compress(messages)

	if !stats.WasCompressed {
		t.Error("expected compression")
	}

	// Ratio should be between 0 and 1
	if stats.CompressionRatio < 0 || stats.CompressionRatio > 1 {
		t.Errorf("CompressionRatio = %f, want between 0 and 1", stats.CompressionRatio)
	}

	// Ratio should be positive (compression reduced size)
	if stats.CompressionRatio <= 0 {
		t.Error("CompressionRatio should be positive after compression")
	}
}

// TestJSONRoundtrip tests that compressed messages can be JSON serialized/deserialized.
func TestJSONRoundtrip(t *testing.T) {
	c := New(1000, 2, 1000)

	messages := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant"},
		{"role": "user", "content": "```go\nfunc main() {}\n```"},
		{"role": "assistant", "content": "Got it"},
		{"role": "user", "content": "Error: failed"},
		{"role": "assistant", "content": "Fixed"},
		{"role": "user", "content": "Thanks"},
		{"role": "assistant", "content": "Welcome"},
		{"role": "user", "content": "Ok"},
		{"role": "assistant", "content": "Cool!"},
	}

	compressed, stats := c.Compress(messages)

	// Serialize to JSON
	data, err := json.Marshal(compressed)
	if err != nil {
		t.Fatalf("Failed to marshal compressed messages: %v", err)
	}

	// Deserialize
	var restored []map[string]any
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal compressed messages: %v", err)
	}

	if len(restored) != len(compressed) {
		t.Errorf("restored has %d messages, want %d", len(restored), len(compressed))
	}

	// Stats should also be JSON serializable
	statsData, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("Failed to marshal stats: %v", err)
	}

	var restoredStats Stats
	if err := json.Unmarshal(statsData, &restoredStats); err != nil {
		t.Fatalf("Failed to unmarshal stats: %v", err)
	}

	if restoredStats.OriginalTokens != stats.OriginalTokens {
		t.Errorf("OriginalTokens = %d, want %d", restoredStats.OriginalTokens, stats.OriginalTokens)
	}
}

// TestNoCompressionBelowThreshold tests no compression when under threshold.
func TestNoCompressionBelowThreshold(t *testing.T) {
	// Set threshold very high
	c := New(50000, 6, 50000)

	largeContent := ""
	for i := 0; i < 100; i++ {
		largeContent += "Some content. "
	}

	messages := []map[string]any{
		{"role": "system", "content": "System"},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
		{"role": "user", "content": largeContent},
		{"role": "assistant", "content": largeContent},
	}

	compressed, stats := c.Compress(messages)

	if stats.WasCompressed {
		t.Error("should not compress when under threshold")
	}
	if len(compressed) != len(messages) {
		t.Error("message count should not change")
	}
	if stats.OriginalTokens != stats.CompressedTokens {
		t.Error("token counts should match when not compressed")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

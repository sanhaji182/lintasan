package reasoning

import (
	"testing"
)

func TestIsReasoningModel_EmptyContentWithReasoning(t *testing.T) {
	data := []byte(`{
		"choices": [{
			"message": {
				"content": "",
				"reasoning_content": "Let me think about this..."
			}
		}]
	}`)
	if !IsReasoningModel(data) {
		t.Error("expected IsReasoningModel=true when content empty, reasoning_content populated")
	}
}

func TestIsReasoningModel_ContentNotEmpty(t *testing.T) {
	data := []byte(`{
		"choices": [{
			"message": {
				"content": "Here's my answer",
				"reasoning_content": ""
			}
		}]
	}`)
	if IsReasoningModel(data) {
		t.Error("expected IsReasoningModel=false when content is populated")
	}
}

func TestIsReasoningModel_InvalidJSON(t *testing.T) {
	data := []byte(`not-json`)
	if IsReasoningModel(data) {
		t.Error("expected IsReasoningModel=false for invalid JSON")
	}
}

func TestIsReasoningModel_NoChoices(t *testing.T) {
	data := []byte(`{}`)
	if IsReasoningModel(data) {
		t.Error("expected IsReasoningModel=false when no choices")
	}
}

func TestExtractReasoningContent_PassthroughOnInvalidJSON(t *testing.T) {
	data := []byte(`{invalid}`)
	got := ExtractReasoningContent(data)
	if string(got) != string(data) {
		t.Errorf("expected passthrough on invalid JSON, got changed output")
	}
}

func TestExtractReasoningContent_NormalResponse(t *testing.T) {
	input := []byte(`{
		"choices": [{
			"message": {
				"content": "Hello, how can I help?",
				"reasoning_content": ""
			}
		}]
	}`)
	got := ExtractReasoningContent(input)
	if string(got) != string(input) {
		t.Errorf("expected unchanged for normal response, got different output")
	}
}

func TestExtractReasoningContent_ContentHasCode(t *testing.T) {
	input := []byte("{\n\t\t\"choices\": [{\n\t\t\t\"message\": {\n\t\t\t\t\"content\": \"Here is the code:\\n```python\\nprint('hello')\\nprint('world')\\nprint('long enough for 50 char threshold test')\\n```\",\n\t\t\t\t\"reasoning_content\": \"Let me think about this long and carefully...\"\n\t\t\t}\n\t\t}]\n\t}")
	got := ExtractReasoningContent(input)
	if string(got) != string(input) {
		t.Errorf("expected unchanged when content already has code block")
	}
}

func TestExtractReasoningContent_ExtractsFromReasoning(t *testing.T) {
	input := []byte("{\n\t\t\"choices\": [{\n\t\t\t\"message\": {\n\t\t\t\t\"content\": \"Let me provide the answer\",\n\t\t\t\t\"reasoning_content\": \"I need to write a function\\n\\n```python\\ndef hello():\\n    print('hello world')\\n    return True\\n    # This is a comment that makes the code long enough\\n    # to pass the 50 char threshold\\n```\\n\\nThat should work\"\n\t\t\t}\n\t\t}]\n\t}")
	got := ExtractReasoningContent(input)
	if string(got) == string(input) {
		t.Error("expected output to differ from input when reasoning has code")
	}
}

func TestExtractReasoningContent_EmptyBoth(t *testing.T) {
	input := []byte(`{
		"choices": [{
			"message": {
				"content": "",
				"reasoning_content": ""
			}
		}]
	}`)
	got := ExtractReasoningContent(input)
	if string(got) != string(input) {
		t.Error("expected unchanged when both empty")
	}
}

// extract functions need 50+ char strings — see length checks in production code

func TestExtractFinalAnswer_CodeAfterImport(t *testing.T) {
	// Strategy 3: last import/def to end — needs content after search point to be >50 chars
	reasoning := "I should write a function\n\nimport os\nimport sys\n\ndef main():\n    return os.getcwd()\n\n# Extra code to make this long enough for the 50-char threshold\nprint('hello world')\n"
	got := extractFinalAnswer(reasoning)
	if got == "" {
		t.Fatal("expected non-empty extraction")
	}
	if !contains(got, "import") || !contains(got, "def main") {
		t.Errorf("expected code to contain import/def, got: %s", got)
	}
}

func TestExtractFinalAnswer_CodeBlockPreference(t *testing.T) {
	// The code block content must be >50 chars to pass hasCodeBlock checks
	reasoning := "Let me think...\n```python\ndef cool_function():\n    return 42\n\nclass MyHelper:\n    def helper(self):\n        return 'hello'\n```\nAnd that's the answer"
	got := extractFinalAnswer(reasoning)
	if got == "" {
		t.Fatal("expected non-empty extraction from code block")
	}
	if !contains(got, "cool_function") {
		t.Errorf("expected 'cool_function' in extracted code, got: %s", got)
	}
}

func TestExtractFinalAnswer_Empty(t *testing.T) {
	got := extractFinalAnswer("Just some prose without any code")
	if got != "" {
		t.Errorf("expected empty for prose-only input, got: %s", got)
	}
}

func TestHasCodeBlock_True(t *testing.T) {
	text := "Some text\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n    fmt.Println(\"world\")\n    fmt.Println(\"third line makes it long enough to pass 50 char limit\")\n}\n```\nmore text"
	if !hasCodeBlock(text) {
		t.Error("expected hasCodeBlock=true when code block > 50 chars")
	}
}

func TestHasCodeBlock_False(t *testing.T) {
	text := "Just some regular text without code blocks"
	if hasCodeBlock(text) {
		t.Error("expected hasCodeBlock=false for text without code blocks")
	}
}

func TestHasCodeBlock_EmptyBlock(t *testing.T) {
	text := "```\n\n```"
	if hasCodeBlock(text) {
		t.Error("expected hasCodeBlock=false for empty code block")
	}
}

func TestTrimTrailingProse_Basic(t *testing.T) {
	// Needs 50+ chars total to pass trimTrailingProse length check
	code := "def foo():\n    return 1\n\n# A very long comment that makes this string exceed 50 characters total\nx = 42\n\nThis is trailing prose"
	got := trimTrailingProse(code)
	if got == "" {
		t.Fatal("expected non-empty trim result")
	}
	if contains(got, "trailing prose") {
		t.Errorf("expected trailing prose to be trimmed, got: %s", got)
	}
	if !contains(got, "def foo()") {
		t.Errorf("expected function def to remain, got: %s", got)
	}
}

func TestTrimTrailingProse_NoTrailingProse(t *testing.T) {
	code := "def foo():\n    return 1\n\n# A very long comment that makes this string exceed 50 characters total\n"
	got := trimTrailingProse(code)
	if got == "" {
		t.Errorf("expected code preserved without trailing prose, got empty")
	}
	if !contains(got, "def foo()") {
		t.Errorf("expected function def to remain, got: %s", got)
	}
}

func TestTrimTrailingProse_ShortResult(t *testing.T) {
	got := trimTrailingProse("short")
	if got != "" {
		t.Errorf("expected empty for short result, got: %s", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

package compress

import (
	"encoding/json"
	"strings"
	"testing"
)

// Each test pins one B2 contract. The overriding invariant the 9router
// source pins: FAIL-OPEN — any shape-mismatch or decode error leaves the
// body byte-identical, never the caller's error.

func TestCompressToolResultsOpenAIShapeHappyPath(t *testing.T) {
	// A log-dump body matches CompressLogs (the 9router filters); use log
	// lines so the filter is exercised for real. (CompressGeneric only
	// shortens text when >10000 bytes OR has >2 blank lines in a row, so a
	// generic 500-byte stub would not move.)
	stub := strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 10)
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": "what does the log say"},
			map[string]any{
				"role":         "tool",
				"tool_call_id": "call_abc",
				"content":      stub,
			},
		},
	}
	stats := CompressToolResultsOpenAI(body)
	if stats.Mutated != 1 {
		t.Fatalf("mutated = %d, want 1", stats.Mutated)
	}
	if stats.BytesBefore <= stats.BytesAfter {
		t.Fatal("no compression: before <= after")
	}
	// The tool message's content is SHORTER in place.
	msgs := body["messages"].([]any)
	toolMsg := msgs[1].(map[string]any)
	newContent, _ := toolMsg["content"].(string)
	if len(newContent) >= len(stub) {
		t.Fatal("tool content not compressed")
	}
	// The user message is untouched.
	userMsg := msgs[0].(map[string]any)
	if _, ok := userMsg["content"].(string); !ok {
		t.Fatal("user message content replaced with non-string")
	}
}

func TestCompressToolResultsOpenAIIgnoresNonToolMessages(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			// Long assistant content (NOT a tool result) → must not be
			// compressed by this mutator.
			map[string]any{"role": "assistant", "content": strings.Repeat("assistant line\n", 50)},
			map[string]any{"role": "user", "content": strings.Repeat("user line\n", 50)},
			// Long system content → must not compress.
			map[string]any{"role": "system", "content": strings.Repeat("system line\n", 50)},
		},
	}
	stats := CompressToolResultsOpenAI(body)
	if stats.Mutated != 0 {
		t.Fatalf("mutated = %d, want 0 (assistant/user/system are not tool messages)", stats.Mutated)
	}
}

func TestCompressToolResultsOpenAIIsErrorSkip(t *testing.T) {
	// 9router's rtk/index.js guarantees that is_error / status:"error"
	// tool results NEVER get compressed — the caller might need the full
	// trace. Verify the skip is exact.
	stub := strings.Repeat("error frame detail line\n", 100)
	body := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "tool", "tool_call_id": "c1", "content": stub, "is_error": true},
			map[string]any{"role": "tool", "tool_call_id": "c2", "content": stub, "status": "error"},
		},
	}
	stats := CompressToolResultsOpenAI(body)
	if stats.Mutated != 0 {
		t.Fatalf("mutated = %d, want 0 (both tool messages carry error status)", stats.Mutated)
	}
	// And the content is byte-identical.
	msgs := body["messages"].([]any)
	if got, _ := msgs[0].(map[string]any)["content"].(string); got != stub {
		t.Fatal("error tool result was mutated")
	}
}

func TestCompressToolResultsOpenAIFailOpenOnBadShape(t *testing.T) {
	// A body with no "messages" exits after an empty stats block; no
	// mutation, no error.
	if stats := CompressToolResultsOpenAI(map[string]any{}); stats.Mutated != 0 {
		t.Fatalf("expected empty stats, got %+v", stats)
	}
	// messages not a list of maps.
	if stats := CompressToolResultsOpenAI(map[string]any{"messages": []any{"not-a-map"}}); stats.Mutated != 0 {
		t.Fatalf("expected empty stats, got %+v", stats)
	}
	// Fully random body.
	if stats := CompressToolResultsOpenAI(map[string]any{"foo": "bar"}); stats.Mutated != 0 {
		t.Fatalf("expected empty stats, got %+v", stats)
	}
}

func TestCompressToolResultsClaudeShapeHappyPath(t *testing.T) {
	toolUseID := "toolu_01"
	// Log-dump stub so CompressLogs shortens; CompressGeneric only
	// shortens over-threshold or blank-line-separated content.
	stub := strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 12)
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "what changed"},
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": toolUseID,
						"content":     stub,
					},
				},
			},
		},
	}
	stats := CompressToolResultsClaude(body)
	if stats.Mutated != 1 {
		t.Fatalf("mutated = %d, want 1", stats.Mutated)
	}
	// The tool_result's content is the compressed string.
	msgs := body["messages"].([]any)
	blk := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	newContent, _ := blk["content"].(string)
	if len(newContent) >= len(stub) {
		t.Fatal("tool_result content not compressed")
	}
}

// Claude's tool_result may carry its content as a list of typed text
// blocks — the 9router executor walks those too. Assert each one's
// mutated text is SHORTER, and nothing else on the body moved.
func TestCompressToolResultsClaudeHandlesTypedContentList(t *testing.T) {
	stub := strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 12)
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "t1",
						"content": []any{
							map[string]any{"type": "text", "text": stub},
							map[string]any{"type": "text", "text": stub},
						},
					},
				},
			},
		},
	}
	stats := CompressToolResultsClaude(body)
	if stats.Mutated != 2 {
		t.Fatalf("mutated = %d, want 2 (one per text block)", stats.Mutated)
	}
	blk := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	list := blk["content"].([]any)
	for i, sub := range list {
		text, _ := sub.(map[string]any)["text"].(string)
		if len(text) >= len(stub) {
			t.Fatalf("sub-block %d not compressed", i)
		}
	}
}

func TestCompressToolResultsClaudeIgnoresNonToolBlocks(t *testing.T) {
	// Use a log-dump shaped stub so CompressLogs would have compressed it
	// HAD the mutator stripped non-tool blocks; the mutator must leave it
	// untouched.
	stub := strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 100)
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					// These are NOT tool_result blocks.
					map[string]any{"type": "text", "text": stub},
					map[string]any{"type": "tool_use", "id": "t1", "input": stub},
				},
			},
		},
	}
	stats := CompressToolResultsClaude(body)
	if stats.Mutated != 0 {
		t.Fatalf("mutated = %d, want 0", stats.Mutated)
	}
	// The plain text block still carries the ORIGINAL stub (compare the
	// Go string, not a marshalled substring — JSON escaping of newline
	// makes a substring test brittle).
	blk := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	text, _ := blk["text"].(string)
	if text != stub {
		t.Fatalf("plain text got mutated/shortened: got %d bytes, want %d", len(text), len(stub))
	}
	// Same check on the tool_use block's `input` field.
	tu := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[1].(map[string]any)
	input, _ := tu["input"].(string)
	if input != stub {
		t.Fatalf("tool_use input got mutated: got %d bytes, want %d", len(input), len(stub))
	}
}

func TestCompressToolResultsAnyInPlaceDetectsOpenAI(t *testing.T) {
	// Use a log-dump shaped stub so CompressLogs actually shortens it
	// (CompressGeneric only shortens over-threshold content).
	stub := strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 12)
	src, _ := json.Marshal(map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "tool", "tool_call_id": "c1", "content": stub},
		},
	})
	out, stats := CompressToolResultsAnyInPlace(src)
	if stats.Mutated != 1 {
		t.Fatalf("mutated = %d, want 1", stats.Mutated)
	}
	if len(out) >= len(src) {
		t.Fatalf("no compression: out=%d src=%d", len(out), len(src))
	}
	// The output is still valid JSON and carries the compressed tool result.
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	msgs := decoded["messages"].([]any)
	toolMsg := msgs[0].(map[string]any)
	newContent, _ := toolMsg["content"].(string)
	if len(newContent) >= len(stub) {
		t.Fatalf("tool message content unchanged after dispatch: %d", len(newContent))
	}
	// OpenAI-shape check: the model field is still on the body.
	if _, ok := decoded["model"].(string); !ok {
		t.Fatal("openai body lost its model field")
	}
}

func TestCompressToolResultsAnyInPlaceDetectsClaude(t *testing.T) {
	// A log-dump shaped stub so the filter acts, in Claude's typed-content
	// tool_result shape.
	stub := strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 12)
	src, _ := json.Marshal(map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "assistant",
				"content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": stub}},
			},
		},
	})
	out, stats := CompressToolResultsAnyInPlace(src)
	if stats.Mutated != 1 {
		t.Fatalf("mutated = %d, want 1 (claude shape, model absent)", stats.Mutated)
	}
	if len(out) >= len(src) {
		t.Fatal("no compression on the Claude lane")
	}
}

func TestCompressToolResultsAnyInPlaceFailOpenOnBadJSON(t *testing.T) {
	bad := []byte(`{not json`)
	out, stats := CompressToolResultsAnyInPlace(bad)
	if stats.Mutated != 0 {
		t.Fatalf("mutated = %d, want 0", stats.Mutated)
	}
	if string(out) != string(bad) {
		t.Fatal("bad-JSON body must be returned byte-identical (fail-open)")
	}
}

func TestTryCompressToolResultsOpenAIRejectsNonOpenAIShape(t *testing.T) {
	src, _ := json.Marshal(map[string]any{"messages": []any{}})
	if _, _, err := TryCompressToolResultsOpenAI(src); err == nil {
		t.Fatal("expected an error for a non-OpenAI body")
	}
	// A valid OpenAI body with a LONG tool result triggers compression.
	// CompressGeneric only shortens over-threshold content, so use a log-
	// dump stub >10000 bytes to pin the path.
	src, _ = json.Marshal(map[string]any{
		"model":    "gpt-4o",
		"messages": []any{map[string]any{"role": "tool", "tool_call_id": "c", "content": strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 150)}},
	})
	out, _, err := TryCompressToolResultsOpenAI(src)
	if err != nil {
		t.Fatalf("TryCompressToolResultsOpenAI: %v", err)
	}
	if len(out) >= len(src) {
		t.Fatal("no compression")
	}
}

func TestSavingsStatsShape(t *testing.T) {
	// The savings shape is caller-visible; pin the maths. Use a log-dump
	// stub so the CompressLogs filter is exercised (not CompressGeneric).
	stub := strings.Repeat("2026-10-19 10:00:00 INFO starting up the service module line 1 for compression test\n", 20)
	body := map[string]any{
		"model":    "gpt-4o",
		"messages": []any{map[string]any{"role": "tool", "tool_call_id": "c1", "content": stub}},
	}
	stats := CompressToolResultsOpenAI(body)
	if stats.Mutated != 1 {
		t.Fatalf("mutated = %d", stats.Mutated)
	}
	if stats.SavingsBytes <= 0 || stats.SavingsPercent <= 0 {
		t.Fatalf("savings = %+v", stats)
	}
	if stats.PerMessage["c1"] <= 0 {
		t.Fatalf("per-message savings missing: %+v", stats.PerMessage)
	}
}

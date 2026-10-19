// rtk_inplace.go — B2 of the 9router parity plan
// (docs/plans/2026-10-18-9router-parity.md §4 B2).
//
// Port of 9router (decolua/9router @ v0.5.104) open-sse/rtk/* mutators —
// the IN-PLACE tool_result compression the 9router executor applies
// before the request body reaches the upstream:
//
//	OpenAI body shape:  messages[N].tool_calls[M].function.name +
//	                   messages[N].role "tool" tool_call_id + content
//	Claude body shape:  messages[N].content[K].type "tool_result" +
//	                   content[K].tool_use_id + content
//
// The mutators mutate the DECODED body in place (the caller re-serialises
// to JSON at the outbound edge). They FAIL OPEN unconditionally: any
// error — malformed shape, missing field, wrong nesting, absent name —
// leaves the body byte-identical, never the caller's error. The
// executor's 9router design records this as fail-open, never throw.
//
//	9router source (rtk/index.js `compressMessages`):
//	  1. walks `body.messages[]`
//	  2. skips `is_error:true` / `status:"error"` tool_results (the
//	     9router doc-comment on that file says this keeps traces)
//	  3. compresses each tool_result's content string in place via the
//	     CompressRTK filter (internal/compress/rtk.go already ports that
//	     — reuse, do not re-derive)
//	  4. returns a "savings" statistic for logging/observability.
//
// The Lintasan default hot-path does NOT call this (the compressor stays
// in `internal/compress`); when a caller (the proxy's OpenAI/Claude
// translator path) decides to wire the in-place mutators it does so via
// the `rtk_inplace_enabled` setting, default false — same pattern as
// `provider_sdk_enabled` on documented in the parity plan.
//
// SAFETY: this package never throws. A malformed body returns the same
// bytes, with no compression performed. Callers that want to PRE-FLIGHT
// the shape can use `Try`-suffixed helpers instead; the mutator path is
// always fail-open.
package compress

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BodyCompressStats is the caller-visible summary of what the mutator did.
type BodyCompressStats struct {
	Mutated        int                `json:"mutated"`               // number of tool_results compressed
	BytesBefore    int                `json:"bytes_before"`          // sum of original content byte lengths
	BytesAfter     int                `json:"bytes_after"`           // sum of compressed byte lengths
	SavingsBytes   int                `json:"savings_bytes"`         // before - after (when >= 0)
	SavingsPercent float64            `json:"savings_percent"`       // percent of original saved (0-1)
	PerMessage     map[string]float64 `json:"per_message,omitempty"` // per tool_call_id compression percent
}

// isErroredToolResult mirrors the 9router executor's is_error /
// status:"error" skip: a tool result that is reporting an ERROR to the
// caller should NEVER be compressed — the caller may need the full error
// trace to debug, and the executor keeps it that way (fail-open for
// traces, compress only successful results).
func isErroredToolResult(m map[string]any) bool {
	if b, ok := m["is_error"].(bool); ok && b {
		return true
	}
	// Claude's tool_result block puts `is_error` inside a content-array
	// entry; check that too.
	if s, ok := m["status"].(string); ok && strings.EqualFold(s, "error") {
		return true
	}
	if obj, ok := m["tool_result"].(map[string]any); ok {
		if b, ok := obj["is_error"].(bool); ok && b {
			return true
		}
	}
	return false
}

// compressToolResultContent is the single-content mutator. Returns
// (newContent, mutated bool). The mutation is the plain CompressRTK
// filter (>50 byte threshold, savings guard,Otherwise untouched).
func compressToolResultContent(content string) (string, bool) {
	if content == "" || len(content) < 50 {
		return content, false
	}
	newContent, pct := CompressRTK(content)
	if pct <= 0 || newContent == content {
		return content, false
	}
	return newContent, true
}

// CompressToolResultsOpenAI walks an already-decoded OpenAI body
// (map form), compressing any message whose role == "tool" carries a
// `content` string > the compressor threshold. The map is mutated in
// place; the caller re-serialises later. FAIL-OPEN.
func CompressToolResultsOpenAI(body map[string]any) *BodyCompressStats {
	msgs, ok := body["messages"].([]any)
	if !ok {
		return emptyStats()
	}
	stats := &BodyCompressStats{PerMessage: map[string]float64{}}
	for _, item := range msgs {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if !strings.EqualFold(role, "tool") {
			continue
		}
		callID, _ := msg["tool_call_id"].(string)
		if isErroredToolResult(msg) {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok {
			continue
		}
		newContent, mutated := compressToolResultContent(content)
		if !mutated {
			continue
		}
		stats.BytesBefore += len(content)
		stats.BytesAfter += len(newContent)
		msg["content"] = newContent
		stats.Mutated++
		if callID != "" {
			stats.PerMessage[callID] = float64(len(content)-len(newContent)) / float64(len(content))
		}
	}
	return finalizeStats(stats)
}

// CompressToolResultsClaude walks an already-decoded Claude Messages
// body (map form) and compresses any `tool_result` content block whose
// inner text string exceeds the compressor threshold. The map is
// mutated in place. FAIL-OPEN.
func CompressToolResultsClaude(body map[string]any) *BodyCompressStats {
	msgs, ok := body["messages"].([]any)
	if !ok {
		return emptyStats()
	}
	stats := &BodyCompressStats{PerMessage: map[string]float64{}}
	for _, item := range msgs {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blocks, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		for _, blkItem := range blocks {
			blk, ok := blkItem.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := blk["type"].(string)
			if !strings.EqualFold(blockType, "tool_result") {
				continue
			}
			toolUseID, _ := blk["tool_use_id"].(string)
			if isErroredToolResult(blk) {
				continue
			}
			// Claude's tool_result content may be a string OR a list of
			// {type:"text", text:"..."} blocks. The 9router executor
			// handles both; a typed-list compressor mutates each text
			// entry separately.
			switch c := blk["content"].(type) {
			case string:
				newContent, mutated := compressToolResultContent(c)
				if !mutated {
					continue
				}
				stats.BytesBefore += len(c)
				stats.BytesAfter += len(newContent)
				blk["content"] = newContent
				stats.Mutated++
				if toolUseID != "" {
					stats.PerMessage[toolUseID] = float64(len(c)-len(newContent)) / float64(len(c))
				}
			case []any:
				for _, subItem := range c {
					sub, ok := subItem.(map[string]any)
					if !ok {
						continue
					}
					if subType, _ := sub["type"].(string); !strings.EqualFold(subType, "text") {
						continue
					}
					text, _ := sub["text"].(string)
					newText, mutated := compressToolResultContent(text)
					if !mutated {
						continue
					}
					stats.BytesBefore += len(text)
					stats.BytesAfter += len(newText)
					sub["text"] = newText
					stats.Mutated++
					if toolUseID != "" {
						stats.PerMessage[toolUseID] = float64(len(text)-len(newText)) / float64(len(text))
					}
				}
			default:
				continue
			}
		}
	}
	return finalizeStats(stats)
}

// CompressToolResultsAnyInPlace walks a JSON body whose format is either
// OpenAI chat-completions or Claude Messages. It detects the shape and
// dispatches. FAIL-OPEN — unknown shape returns the input unchanged.
func CompressToolResultsAnyInPlace(bodyBytes []byte) ([]byte, *BodyCompressStats) {
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return bodyBytes, emptyStats()
	}
	// Distinguish OpenAI vs Claude: Claude bodies carry a top-level
	// "system" field in addition to "messages" (and do not carry
	// "model"). OpenAI bodies carry "model" and often "stream".
	if _, ok := body["model"].(string); ok {
		openAIStats := CompressToolResultsOpenAI(body)
		if openAIStats.Mutated == 0 {
			return bodyBytes, openAIStats
		}
		out, err := json.Marshal(body)
		if err != nil {
			return bodyBytes, emptyStats() // fail-open
		}
		return out, openAIStats
	}
	// Claude shape.
	claudeStats := CompressToolResultsClaude(body)
	if claudeStats.Mutated == 0 {
		return bodyBytes, claudeStats
	}
	out, err := json.Marshal(body)
	if err != nil {
		return bodyBytes, emptyStats() // fail-open
	}
	return out, claudeStats
}

// TryCompressToolResultsOpenAI returns a (mutatedBodyCopy, stats, error).
// It is the DISCRIMINATING version — returns an error when the shape is
// not OpenAI — for callers that want to know.
func TryCompressToolResultsOpenAI(bodyBytes []byte) ([]byte, *BodyCompressStats, error) {
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, ok := body["model"].(string); !ok {
		return nil, nil, fmt.Errorf("not an OpenAI chat-completions body (missing model)")
	}
	stats := CompressToolResultsOpenAI(body)
	if stats.Mutated == 0 {
		return bodyBytes, stats, nil
	}
	out, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	return out, stats, nil
}

func emptyStats() *BodyCompressStats {
	return &BodyCompressStats{PerMessage: map[string]float64{}}
}

func finalizeStats(stats *BodyCompressStats) *BodyCompressStats {
	if stats.PerMessage == nil {
		stats.PerMessage = map[string]float64{}
	}
	stats.SavingsBytes = stats.BytesBefore - stats.BytesAfter
	if stats.BytesBefore > 0 {
		stats.SavingsPercent = float64(stats.SavingsBytes) / float64(stats.BytesBefore)
	}
	return stats
}

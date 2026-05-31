package translator

import (
	"encoding/json"
	"strings"
)

// responses_stream.go — Codex M2: canonical chat-completions SSE → OpenAI
// **Responses API** typed event stream.
//
// M2 SCOPE: streaming TEXT path only (the MVP, per codex-feasibility-validation:
// Codex hardcodes stream=true on /responses, so the event stream IS the minimum
// viable surface). This file is a PURE, stateful event re-framer: given the
// canonical `data: {choices:[{delta:{content}}]}` lines the existing chat
// pipeline already emits, it produces the typed Responses events Codex parses.
//
// It implements NO tool execution and NO tool loop — function_call item emission
// and streamed tool arguments (`response.custom_tool_call_input.delta`) are M3.
// Tool-call deltas in the chat stream are skipped here (text path only); the
// terminal `response.completed` is still emitted so the stream is always valid.
//
// VERIFIED WIRE FACTS (from Codex open source — codex-feasibility-validation.md):
//   - Codex parses inbound events by their JSON `type` field
//     (process_responses_event, sse/responses.rs:266-394). We emit both the
//     `event: <type>` SSE line AND a `"type"` field in the data JSON (faithful
//     to the real OpenAI Responses API), so either parser path works.
//   - REQUIRED terminal: `response.completed` carrying id + usage. If the stream
//     ends without it, Codex raises ApiError::Stream("stream closed before
//     response.completed"). Emitter.Finish() guarantees this terminal even on
//     upstream truncation.
//   - Mandatory lifecycle: response.created → output_item.added →
//     output_text.delta×N → output_item.done → response.completed(+usage).
//     Unknown/extra events are ignored safely by Codex, so emitting the full
//     faithful lifecycle (incl. content_part.added/done) is safe.
//
// DETERMINISM: event ordering is fully deterministic; JSON marshals map keys in
// sorted order; IDs come from the package's fixed randomID() (same convention as
// BuildOpenAIStreamChunk), so golden-fixture tests are reproducible.

// ResponsesStreamEmitter re-frames a canonical chat SSE stream into Responses
// API typed events. It is stateful (tracks lifecycle position) but pure (no I/O):
// callers feed it chat SSE lines and write the returned event strings to the
// client. Not safe for concurrent use by multiple goroutines (one per stream).
type ResponsesStreamEmitter struct {
	model  string
	respID string
	itemID string

	preludeSent bool            // response.created + output_item.added + content_part.added
	completed   bool            // terminal events emitted
	text        strings.Builder // accumulated assistant text (for the *.done events)
	usage       map[string]any  // captured upstream usage, if any

	// finishReason from the last chat chunk that carried one (text path: "stop").
	finishReason string
}

// NewResponsesStreamEmitter creates an emitter for a single Responses stream.
// IDs follow the package's fixed randomID() convention (deterministic, matching
// BuildOpenAIStreamChunk) — fine for Codex, which is stateless and does not
// persist response IDs.
func NewResponsesStreamEmitter(model string) *ResponsesStreamEmitter {
	return &ResponsesStreamEmitter{
		model:  model,
		respID: "resp_" + randomID(),
		itemID: "msg_" + randomID(),
	}
}

// Process consumes ONE chat SSE line (e.g. `data: {...}` or `data: [DONE]`) and
// returns zero or more complete Responses SSE event strings (each terminated
// with the blank-line separator). Non-data lines and empty lines yield nil.
func (e *ResponsesStreamEmitter) Process(line string) []string {
	line = strings.TrimRight(line, "\r\n")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "data:") {
		// SSE comments / event: lines from the chat stream are not expected;
		// ignore them (the chat path emits only `data:` lines).
		return nil
	}
	payload := strings.TrimSpace(trimmed[len("data:"):])

	if payload == "[DONE]" {
		return e.finishEvents()
	}

	var chunk map[string]any
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		// Malformed chunk — skip it (forward-compatible, matches Codex tolerance).
		return nil
	}

	// Capture usage if the chunk carries it (OpenAI includes it in the final
	// chunk with stream_options.include_usage; some providers always do).
	if u, ok := chunk["usage"].(map[string]any); ok && u != nil {
		e.usage = mapChatUsageToResponses(u)
	}

	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return nil
	}
	choice, _ := choices[0].(map[string]any)
	if choice == nil {
		return nil
	}
	if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
		e.finishReason = fr
	}

	delta, _ := choice["delta"].(map[string]any)
	if delta == nil {
		return nil
	}

	// M2: TEXT path only. Tool-call deltas are intentionally not emitted here
	// (that is M3's function_call item lifecycle). A tool-only chunk produces no
	// event; the terminal completed event still closes the stream correctly.
	content, _ := delta["content"].(string)
	if content == "" {
		return nil
	}

	var events []string
	events = append(events, e.preludeEvents()...)
	e.text.WriteString(content)
	events = append(events, sseEvent(map[string]any{
		"type":          "response.output_text.delta",
		"item_id":       e.itemID,
		"output_index":  0,
		"content_index": 0,
		"delta":         content,
	}))
	return events
}

// Finish emits the terminal lifecycle if it has not already been emitted. The
// writer MUST call this when the underlying chat handler returns, so the stream
// always ends with response.completed even if the upstream truncated (no
// [DONE]). Idempotent.
func (e *ResponsesStreamEmitter) Finish() []string {
	return e.finishEvents()
}

// Started reports whether any Responses event has been emitted yet. The writer
// uses this to decide whether a non-2xx upstream body should pass through
// verbatim (nothing emitted yet) vs. be closed with a terminal event.
func (e *ResponsesStreamEmitter) Started() bool { return e.preludeSent }

// preludeEvents emits response.created + output_item.added + content_part.added
// exactly once, lazily on the first text delta (or on Finish for empty output).
func (e *ResponsesStreamEmitter) preludeEvents() []string {
	if e.preludeSent {
		return nil
	}
	e.preludeSent = true
	return []string{
		sseEvent(map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":         e.respID,
				"object":     "response",
				"created_at": 0,
				"status":     "in_progress",
				"model":      e.model,
				"output":     []any{},
			},
		}),
		sseEvent(map[string]any{
			"type":         "response.output_item.added",
			"output_index": 0,
			"item": map[string]any{
				"id":      e.itemID,
				"type":    "message",
				"status":  "in_progress",
				"role":    "assistant",
				"content": []any{},
			},
		}),
		sseEvent(map[string]any{
			"type":          "response.content_part.added",
			"item_id":       e.itemID,
			"output_index":  0,
			"content_index": 0,
			"part":          map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
		}),
	}
}

// finishEvents emits the terminal lifecycle (output_text.done →
// content_part.done → output_item.done → response.completed) exactly once. It
// first ensures the prelude was sent (covers empty-output streams), so the
// result is always a complete, valid Responses stream.
func (e *ResponsesStreamEmitter) finishEvents() []string {
	if e.completed {
		return nil
	}
	var events []string
	events = append(events, e.preludeEvents()...)
	e.completed = true

	full := e.text.String()
	usage := e.usage
	if usage == nil {
		usage = map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	}
	messageItem := map[string]any{
		"id":      e.itemID,
		"type":    "message",
		"status":  "completed",
		"role":    "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": full, "annotations": []any{}}},
	}

	events = append(events,
		sseEvent(map[string]any{
			"type":          "response.output_text.done",
			"item_id":       e.itemID,
			"output_index":  0,
			"content_index": 0,
			"text":          full,
		}),
		sseEvent(map[string]any{
			"type":          "response.content_part.done",
			"item_id":       e.itemID,
			"output_index":  0,
			"content_index": 0,
			"part":          map[string]any{"type": "output_text", "text": full, "annotations": []any{}},
		}),
		sseEvent(map[string]any{
			"type":         "response.output_item.done",
			"output_index": 0,
			"item":         messageItem,
		}),
		sseEvent(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     e.respID,
				"object": "response",
				"status": "completed",
				"model":  e.model,
				"output": []any{messageItem},
				"usage":  usage,
			},
		}),
	)
	return events
}

// sseEvent serializes a Responses event into the SSE wire form:
//
//	event: <type>\ndata: <json>\n\n
//
// The `event:` line mirrors the JSON `type` field so both Codex parser paths
// (event-name and JSON-type) agree.
func sseEvent(data map[string]any) string {
	typ, _ := data["type"].(string)
	return "event: " + typ + "\ndata: " + mustJSON(data) + "\n\n"
}

// mapChatUsageToResponses renames chat usage fields to the Responses shape.
func mapChatUsageToResponses(u map[string]any) map[string]any {
	out := map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	if v, ok := u["prompt_tokens"]; ok {
		out["input_tokens"] = v
	}
	if v, ok := u["completion_tokens"]; ok {
		out["output_tokens"] = v
	}
	if v, ok := u["total_tokens"]; ok {
		out["total_tokens"] = v
	}
	return out
}

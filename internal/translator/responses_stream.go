package translator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// responses_stream.go — Codex M2/M3: canonical chat-completions SSE → OpenAI
// **Responses API** typed event stream.
//
// M2 SCOPE (text): streaming text path — the MVP (Codex hardcodes stream=true on
// /responses). Pure, stateful event re-framer over the canonical
// `data: {choices:[{delta:{content}}]}` lines the existing chat pipeline emits.
//
// M3 SCOPE (tool round-trip — ACCEPTANCE CORE): emit `function_call` output items
// from chat `tool_calls` deltas, with **call_id preserved VERBATIM** from the
// chat tool_call.id. This is the make-or-break of Codex compatibility: if the
// call_id is not byte-identical end to end, the agent loop breaks. Inbound
// `function_call_output` items (the tool RESULT Codex sends back) are translated
// to chat `tool` messages in responses.go (M1, also verbatim call_id) — together
// they form the round-trip.
//
// STILL OUT OF SCOPE: actual tool EXECUTION (Lintasan never runs a tool — it only
// translates the events), provider changes, routing changes, prod changes.
//
// VERIFIED WIRE FACTS (from Codex open source — codex-feasibility-validation.md):
//   - Codex parses inbound events by their JSON `type` field
//     (process_responses_event, sse/responses.rs:266-394). We emit both the
//     `event: <type>` SSE line AND a `"type"` field in the data JSON.
//   - REQUIRED terminal: `response.completed` carrying id + usage; absence →
//     ApiError::Stream("stream closed before response.completed"). Finish()
//     guarantees it even on upstream truncation.
//   - Tool calls arrive as `function_call` items via output_item.added/.done;
//     arguments may be streamed OR delivered whole on output_item.done. We use
//     whole-on-done (deterministic, explicitly supported).
//   - Unknown/extra events are ignored safely by Codex (latitude for us).
//
// DETERMINISM: event ordering is deterministic; JSON marshals map keys sorted;
// IDs derive from the package's fixed randomID() plus a per-item index, so
// golden-fixture tests are reproducible across runs.

// ResponsesTerminalKind selects which error terminal Fail emits. It is the
// translator-package's own enum (decoupled from the metrics package's
// ResponsesTerminal label) so the translator stays dependency-free.
type ResponsesTerminalKind int

const (
	// TerminalFailed → response.failed (hard upstream/translation error).
	TerminalFailed ResponsesTerminalKind = iota
	// TerminalIncomplete → response.incomplete (partial/truncated turn).
	TerminalIncomplete
)

// ResponsesStreamEmitter re-frames a canonical chat SSE stream into Responses
// API typed events. Stateful (tracks the message item + N tool-call items) but
// pure (no I/O). One emitter per stream; not safe for concurrent use.
type ResponsesStreamEmitter struct {
	model  string
	respID string
	msgID  string

	createdSent bool // response.created emitted
	msgOpen     bool // message output_item.added + content_part.added emitted
	completed   bool // terminal events emitted

	// terminalState records HOW the stream ended ("completed"/"failed"/
	// "incomplete"), for M4 metrics + structured logging. Empty until a terminal
	// is emitted. hadText reports whether any text delta was emitted.
	terminalState string
	hadText       bool

	text  strings.Builder // accumulated assistant text (for the *.done events)
	usage map[string]any  // captured upstream usage, if any

	nextIndex int // next output_index to assign (message or tool item)
	msgIndex  int // output_index assigned to the message item (when opened)

	// tool-call accumulators, keyed by chat delta index; toolOrder preserves
	// first-appearance order for deterministic finalize.
	tools     map[int]*toolCallAccum
	toolOrder []int

	finishReason string
}

// toolCallAccum accumulates one streamed chat tool_call into a Responses
// function_call item. callID is the chat tool_call.id, preserved VERBATIM.
type toolCallAccum struct {
	chatIndex   int             // index within the chat delta tool_calls array
	outputIndex int             // Responses output_index (assigned when added)
	itemID      string          // Responses item id (fc_...), distinct from callID
	callID      string          // VERBATIM chat tool_call.id — the round-trip key
	name        string          // function name
	args        strings.Builder // accumulated arguments JSON string
	addedSent   bool            // output_item.added emitted for this item
}

// NewResponsesStreamEmitter creates an emitter for a single Responses stream.
func NewResponsesStreamEmitter(model string) *ResponsesStreamEmitter {
	return &ResponsesStreamEmitter{
		model:  model,
		respID: "resp_" + randomID(),
		msgID:  "msg_" + randomID(),
		tools:  make(map[int]*toolCallAccum),
	}
}

// Process consumes ONE chat SSE line and returns zero or more complete Responses
// SSE event strings (each terminated with the blank-line separator).
func (e *ResponsesStreamEmitter) Process(line string) []string {
	line = strings.TrimRight(line, "\r\n")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "data:") {
		return nil
	}
	payload := strings.TrimSpace(trimmed[len("data:"):])

	if payload == "[DONE]" {
		return e.finishEvents()
	}

	var chunk map[string]any
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		// Malformed chunk — skip (forward-compatible, matches Codex tolerance).
		return nil
	}

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

	var events []string

	// Text content → message item lifecycle (M2 path, preserved byte-for-byte).
	if content, _ := delta["content"].(string); content != "" {
		e.hadText = true
		events = append(events, e.ensureCreated()...)
		events = append(events, e.ensureMessageOpen()...)
		e.text.WriteString(content)
		events = append(events, sseEvent(map[string]any{
			"type":          "response.output_text.delta",
			"item_id":       e.msgID,
			"output_index":  e.msgIndex,
			"content_index": 0,
			"delta":         content,
		}))
	}

	// Tool-call deltas → function_call items (M3). Accumulate; emit
	// output_item.added once per item (when call_id is known), arguments whole
	// on output_item.done at finalize.
	if tcs, ok := delta["tool_calls"].([]any); ok {
		events = append(events, e.processToolCallDeltas(tcs)...)
	}

	return events
}

// processToolCallDeltas folds streamed chat tool_call deltas into accumulators
// and emits output_item.added for any newly-identified tool call.
func (e *ResponsesStreamEmitter) processToolCallDeltas(tcs []any) []string {
	var events []string
	for _, raw := range tcs {
		tc, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		idx := 0
		if v, ok := tc["index"].(float64); ok {
			idx = int(v)
		}
		acc := e.tools[idx]
		if acc == nil {
			acc = &toolCallAccum{chatIndex: idx}
			e.tools[idx] = acc
			e.toolOrder = append(e.toolOrder, idx)
		}
		// id + name arrive on the first delta for this index; preserve VERBATIM.
		if id, ok := tc["id"].(string); ok && id != "" {
			acc.callID = id
		}
		if fn, ok := tc["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && name != "" {
				acc.name = name
			}
			if args, ok := fn["arguments"].(string); ok && args != "" {
				acc.args.WriteString(args)
			}
		}
		// Open the function_call item once its call_id is known.
		if !acc.addedSent && acc.callID != "" {
			events = append(events, e.ensureCreated()...)
			acc.outputIndex = e.nextIndex
			e.nextIndex++
			acc.itemID = fmt.Sprintf("fc_%s_%d", randomID(), acc.chatIndex)
			acc.addedSent = true
			events = append(events, sseEvent(map[string]any{
				"type":         "response.output_item.added",
				"output_index": acc.outputIndex,
				"item": map[string]any{
					"id":        acc.itemID,
					"type":      "function_call",
					"status":    "in_progress",
					"call_id":   acc.callID,
					"name":      acc.name,
					"arguments": "",
				},
			}))
		}
	}
	return events
}

// Finish emits the terminal lifecycle if not already emitted. The writer MUST
// call this when the chat handler returns, so the stream always ends with
// response.completed even on upstream truncation. Idempotent.
func (e *ResponsesStreamEmitter) Finish() []string { return e.finishEvents() }

// Fail emits an ERROR terminal instead of response.completed, for the case where
// the upstream errors AFTER streaming has begun (so a passthrough HTTP error is
// no longer possible — the client is mid-stream). Per Codex's inbound parser,
// response.failed and response.incomplete are recognized error terminals; using
// one of them is strictly better than silently truncating the stream (which
// Codex treats as ApiError::Stream).
//
//   - kind ResponsesTerminalIncomplete → response.incomplete (partial/truncated;
//     the model produced some output but the turn didn't finish cleanly).
//   - anything else → response.failed (hard upstream/translation error).
//
// Idempotent: a no-op if a terminal was already emitted (e.g. completed arrived
// first). reason is a short, non-secret human string surfaced in the error
// object; never pass prompt content or upstream bodies that may carry secrets.
func (e *ResponsesStreamEmitter) Fail(kind ResponsesTerminalKind, reason string) []string {
	if e.completed {
		return nil
	}
	var events []string
	events = append(events, e.ensureCreated()...)
	e.completed = true

	evType := "response.failed"
	status := "failed"
	e.terminalState = "failed"
	if kind == TerminalIncomplete {
		evType = "response.incomplete"
		status = "incomplete"
		e.terminalState = "incomplete"
	}

	if reason == "" {
		reason = "upstream error"
	}
	usage := e.usage
	if usage == nil {
		usage = map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	}
	events = append(events, sseEvent(map[string]any{
		"type": evType,
		"response": map[string]any{
			"id":     e.respID,
			"object": "response",
			"status": status,
			"model":  e.model,
			"output": []any{},
			"usage":  usage,
			"error": map[string]any{
				"type":    "upstream_error",
				"message": reason,
			},
		},
	}))
	return events
}

// TerminalState reports how the stream ended ("completed"/"failed"/"incomplete"),
// or "" if no terminal has been emitted yet. For M4 metrics + logging.
func (e *ResponsesStreamEmitter) TerminalState() string { return e.terminalState }

// ToolCallCount reports the number of function_call items that were opened
// (call_id known). For M4 metrics + logging.
func (e *ResponsesStreamEmitter) ToolCallCount() int {
	n := 0
	for _, acc := range e.tools {
		if acc != nil && acc.addedSent {
			n++
		}
	}
	return n
}

// HadText reports whether any assistant text delta was emitted. For M4 logging.
func (e *ResponsesStreamEmitter) HadText() bool { return e.hadText }

// Started reports whether any Responses event has been emitted (response.created).
func (e *ResponsesStreamEmitter) Started() bool { return e.createdSent }

// ensureCreated emits response.created exactly once, on the first activity.
func (e *ResponsesStreamEmitter) ensureCreated() []string {
	if e.createdSent {
		return nil
	}
	e.createdSent = true
	return []string{sseEvent(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         e.respID,
			"object":     "response",
			"created_at": 0,
			"status":     "in_progress",
			"model":      e.model,
			"output":     []any{},
		},
	})}
}

// ensureMessageOpen emits the assistant message item lifecycle start
// (output_item.added + content_part.added) exactly once, lazily on first text.
// A message item is created ONLY when there is text — a pure tool-call response
// emits no message item (matching the real OpenAI Responses API).
func (e *ResponsesStreamEmitter) ensureMessageOpen() []string {
	if e.msgOpen {
		return nil
	}
	e.msgOpen = true
	e.msgIndex = e.nextIndex
	e.nextIndex++
	return []string{
		sseEvent(map[string]any{
			"type":         "response.output_item.added",
			"output_index": e.msgIndex,
			"item": map[string]any{
				"id":      e.msgID,
				"type":    "message",
				"status":  "in_progress",
				"role":    "assistant",
				"content": []any{},
			},
		}),
		sseEvent(map[string]any{
			"type":          "response.content_part.added",
			"item_id":       e.msgID,
			"output_index":  e.msgIndex,
			"content_index": 0,
			"part":          map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
		}),
	}
}

// finishEvents emits the terminal lifecycle exactly once: close the message item
// (if opened), close each tool-call item, then response.completed carrying the
// full ordered output array + usage. Always a complete, valid Responses stream.
func (e *ResponsesStreamEmitter) finishEvents() []string {
	if e.completed {
		return nil
	}
	var events []string
	events = append(events, e.ensureCreated()...)
	e.completed = true
	e.terminalState = "completed"

	// Collect output items keyed by output_index for a deterministic final array.
	outputs := map[int]map[string]any{}

	// 1. Close the message item (only if it was opened — i.e. there was text).
	if e.msgOpen {
		full := e.text.String()
		messageItem := map[string]any{
			"id":      e.msgID,
			"type":    "message",
			"status":  "completed",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": full, "annotations": []any{}}},
		}
		outputs[e.msgIndex] = messageItem
		events = append(events,
			sseEvent(map[string]any{
				"type":          "response.output_text.done",
				"item_id":       e.msgID,
				"output_index":  e.msgIndex,
				"content_index": 0,
				"text":          full,
			}),
			sseEvent(map[string]any{
				"type":          "response.content_part.done",
				"item_id":       e.msgID,
				"output_index":  e.msgIndex,
				"content_index": 0,
				"part":          map[string]any{"type": "output_text", "text": full, "annotations": []any{}},
			}),
			sseEvent(map[string]any{
				"type":         "response.output_item.done",
				"output_index": e.msgIndex,
				"item":         messageItem,
			}),
		)
	}

	// 2. Close each tool-call item, in first-appearance order. call_id VERBATIM.
	for _, idx := range e.toolOrder {
		acc := e.tools[idx]
		if acc == nil {
			continue
		}
		// Defensive: if added was never sent (no call_id ever arrived), skip —
		// a tool call with no id can't be rounded-tripped, so don't fabricate one.
		if !acc.addedSent {
			continue
		}
		args := acc.args.String()
		if args == "" {
			args = "{}"
		}
		fnItem := map[string]any{
			"id":        acc.itemID,
			"type":      "function_call",
			"status":    "completed",
			"call_id":   acc.callID,
			"name":      acc.name,
			"arguments": args,
		}
		outputs[acc.outputIndex] = fnItem
		events = append(events, sseEvent(map[string]any{
			"type":         "response.output_item.done",
			"output_index": acc.outputIndex,
			"item":         fnItem,
		}))
	}

	// 3. response.completed with the ordered output array + usage.
	usage := e.usage
	if usage == nil {
		usage = map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	}
	indices := make([]int, 0, len(outputs))
	for i := range outputs {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	outArr := make([]any, 0, len(indices))
	for _, i := range indices {
		outArr = append(outArr, outputs[i])
	}

	events = append(events, sseEvent(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":     e.respID,
			"object": "response",
			"status": "completed",
			"model":  e.model,
			"output": outArr,
			"usage":  usage,
		},
	}))
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

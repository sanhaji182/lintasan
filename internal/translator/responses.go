package translator

import (
	"encoding/json"
	"errors"
	"fmt"
)

// responses.go — Codex Official Layer translator: OpenAI **Responses API**
// request → canonical OpenAI chat-completions request.
//
// M1 SCOPE: REQUEST translation + validation ONLY. This file converts a parsed
// Responses request body into the canonical chat-completions map the existing
// pipeline consumes. It implements NO streaming, NO SSE, NO tool execution, and
// NO tool loop. It DOES translate tool-related INPUT items (`function_call`,
// `function_call_output`) because that is request-schema translation, not tool
// execution — the actual agentic loop (emitting tool calls, accepting results
// at runtime) is M3. The response-direction helper (OpenAIResponseToResponses)
// remains an M2 stub.
//
// It is deliberately NOT wired into translate.go's switches or AllFormats(): the
// Responses ingress handler (M2) calls ResponsesToOpenAIRequest directly, so the
// existing 5-format generic translator behavior stays byte-identical.
//
// VERIFIED WIRE FACTS (from Codex open source — codex-feasibility-validation.md):
//   - Codex sends the FULL `input` per HTTP request; no previous_response_id on
//     the HTTP transport → stateless passthrough, nothing to look up.
//   - store=false for non-Azure → no response store.
//   - stream is always true → streaming emitter is the MVP (M2), not here.
//   - `input` is either a string (shorthand) or an array of typed items:
//     message / function_call / function_call_output.
//   - `instructions` is a top-level system prompt string.
//   - tools are the Responses *flat* function shape (name/description/parameters
//     at the top level), vs chat's nested {type:function, function:{...}}.

// FormatResponses is the OpenAI Responses API format (Codex ingress), the 6th
// translator format. NOTE: intentionally not added to AllFormats() or the
// translate.go conversion switches — the Responses handler calls
// ResponsesToOpenAIRequest directly, keeping the generic 5-format translator
// unchanged.
const FormatResponses Format = "responses"

// Validation errors are sentinel values so callers (and tests) can match
// deterministically. The handler (M2) maps these to a 400.
var (
	ErrResponsesNoModel    = errors.New("responses: missing or empty 'model'")
	ErrResponsesNoInput    = errors.New("responses: missing 'input' (must be a string or an array of input items)")
	ErrResponsesBadInput   = errors.New("responses: 'input' must be a string or an array")
	ErrResponsesBadItem    = errors.New("responses: malformed input item")
	ErrResponsesNoCallID   = errors.New("responses: function_call/function_call_output item missing 'call_id'")
	ErrResponsesBadToolsTy = errors.New("responses: 'tools' must be an array")
)

// ErrResponsesBadUpstream marks a non-streaming upstream body that carries no
// usable assistant turn (no choices, or a choice with no message). The handler
// maps it to 502 rather than returning an empty but "completed" response, so a
// broken upstream is never disguised as a successful empty turn.
var ErrResponsesBadUpstream = errors.New("responses: upstream returned no usable choice")

// ValidateResponsesRequest performs deterministic structural validation of a
// parsed Responses request. It is pure (no I/O, no randomness) and returns a
// sentinel error on the first violation, in a fixed check order, so the same
// input always yields the same error. It validates SHAPE only — it does not
// execute or interpret tools.
func ValidateResponsesRequest(raw map[string]any) error {
	// 1. model — required, non-empty string.
	model, ok := raw["model"].(string)
	if !ok || model == "" {
		return ErrResponsesNoModel
	}

	// 2. input — required; string OR array.
	input, present := raw["input"]
	if !present {
		return ErrResponsesNoInput
	}
	switch in := input.(type) {
	case string:
		// ok — shorthand single user message.
	case []any:
		// validate each item structurally.
		for _, it := range in {
			item, ok := it.(map[string]any)
			if !ok {
				return ErrResponsesBadItem
			}
			switch itemType(item) {
			case "function_call", "function_call_output":
				if callID(item) == "" {
					return ErrResponsesNoCallID
				}
			}
			// "message" and unknown types pass shape validation; unknown types
			// are tolerated (translated best-effort or skipped) to match Codex's
			// own forward-compatibility (it ignores unknown events/items).
		}
	default:
		return ErrResponsesBadInput
	}

	// 3. tools — if present, must be an array.
	if tools, present := raw["tools"]; present {
		if _, ok := tools.([]any); !ok {
			return ErrResponsesBadToolsTy
		}
	}

	return nil
}

// ResponsesToOpenAIRequest converts a parsed Responses request body into the
// canonical OpenAI chat-completions request map. It validates first (returning a
// sentinel error), then performs a pure, deterministic translation. It executes
// no tools and runs no loop — tool-related input items are translated as message
// history (assistant tool_calls / tool results), exactly as the chat pipeline
// already understands them.
func ResponsesToOpenAIRequest(raw map[string]any) (map[string]any, error) {
	if err := ValidateResponsesRequest(raw); err != nil {
		return nil, err
	}

	result := make(map[string]any)
	result["model"] = raw["model"].(string)

	// Build messages: instructions (system) first, then translated input.
	var msgs []map[string]any
	if instr, ok := raw["instructions"].(string); ok && instr != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": instr})
	}

	switch in := raw["input"].(type) {
	case string:
		msgs = append(msgs, map[string]any{"role": "user", "content": in})
	case []any:
		msgs = append(msgs, responsesInputItemsToMessages(in)...)
	}
	if len(msgs) > 0 {
		result["messages"] = msgs
	}

	// Parameters. max_output_tokens → max_tokens (Responses rename).
	if v, ok := raw["max_output_tokens"]; ok && v != nil {
		result["max_tokens"] = v
	}
	copyIfPresent(raw, result, "temperature")
	copyIfPresent(raw, result, "top_p")
	copyIfPresent(raw, result, "stream")
	copyIfPresent(raw, result, "parallel_tool_calls")

	// Tools: Responses flat shape → chat nested shape.
	if tools, ok := raw["tools"].([]any); ok {
		result["tools"] = responsesToolsToOpenAI(tools)
	}
	// tool_choice: passthrough strings; normalize the object form.
	if tc, ok := raw["tool_choice"]; ok {
		result["tool_choice"] = responsesToolChoiceToOpenAI(tc)
	}

	return result, nil
}

// OpenAIResponseToResponses converts a canonical NON-STREAMING chat-completion
// response into a Responses-shaped object (M6).
//
// The output shape is deliberately identical to the `response` payload the
// streaming emitter puts on its terminal `response.completed` event
// (responses_stream.go finishEvents), so a client sees the same object whether
// it asked for stream=true or stream=false. Shared rules:
//
//   - a message item exists ONLY when there is assistant text (a pure tool-call
//     turn emits no message item, matching the real Responses API);
//   - tool calls become `function_call` items with call_id preserved VERBATIM,
//     in upstream order, after the message item;
//   - `arguments` is always a JSON string, defaulting to "{}" when absent;
//   - usage is renamed to the Responses vocabulary, zero-filled when missing.
//
// Errors: ErrResponsesBadUpstream when the body carries no usable choice, so
// the handler can answer 502 rather than inventing an empty turn.
func OpenAIResponseToResponses(raw map[string]any) (map[string]any, error) {
	if raw == nil {
		return nil, ErrResponsesBadUpstream
	}

	model, _ := raw["model"].(string)

	choices, ok := raw["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, ErrResponsesBadUpstream
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, ErrResponsesBadUpstream
	}
	message, ok := choice["message"].(map[string]any)
	if !ok {
		return nil, ErrResponsesBadUpstream
	}

	output := make([]any, 0, 2)

	// 1. Assistant text → message item (only when non-empty, mirroring the
	//    streaming emitter's lazy ensureMessageOpen).
	if text, _ := message["content"].(string); text != "" {
		output = append(output, map[string]any{
			"id":     "msg_" + randomID(),
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []any{map[string]any{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			}},
		})
	}

	// 2. Tool calls → function_call items, upstream order, call_id verbatim.
	if toolCalls, ok := message["tool_calls"].([]any); ok {
		for _, tc := range toolCalls {
			call, ok := tc.(map[string]any)
			if !ok {
				continue
			}
			fn, ok := call["function"].(map[string]any)
			if !ok {
				continue
			}
			callID, _ := call["id"].(string)
			if callID == "" {
				// No id → the client could never return a matching
				// function_call_output. Skip rather than fabricate one.
				continue
			}
			name, _ := fn["name"].(string)
			args, _ := fn["arguments"].(string)
			if args == "" {
				args = "{}"
			}
			output = append(output, map[string]any{
				"id":        "fc_" + randomID(),
				"type":      "function_call",
				"status":    "completed",
				"call_id":   callID,
				"name":      name,
				"arguments": args,
			})
		}
	}

	usage := map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	if u, ok := raw["usage"].(map[string]any); ok {
		usage = mapChatUsageToResponses(u)
	}

	return map[string]any{
		"id":     "resp_" + randomID(),
		"object": "response",
		"status": "completed",
		"model":  model,
		"output": output,
		"usage":  usage,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers (pure, deterministic)
// ─────────────────────────────────────────────────────────────────────────────

// responsesInputItemsToMessages translates the Responses `input` array into chat
// messages, preserving order. Handles message / function_call /
// function_call_output. Unknown item types are skipped (forward-compatible).
func responsesInputItemsToMessages(items []any) []map[string]any {
	var out []map[string]any
	for _, it := range items {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		switch itemType(item) {
		case "message", "":
			// "" covers the common shorthand {role, content} with no explicit type.
			out = append(out, responsesMessageToOpenAI(item))
		case "function_call":
			out = append(out, responsesFunctionCallToOpenAI(item))
		case "function_call_output":
			out = append(out, responsesFunctionCallOutputToOpenAI(item))
		default:
			// Unknown type → skip (Codex itself ignores unknown items).
		}
	}
	return out
}

// responsesMessageToOpenAI converts a Responses message item to a chat message.
// content may be a string or an array of typed parts (input_text/output_text/
// input_image). Text-only collapses to a string; any image yields a multipart
// array (chat vision shape) so multimodal survives.
func responsesMessageToOpenAI(item map[string]any) map[string]any {
	role, _ := item["role"].(string)
	if role == "" {
		role = "user"
	}
	msg := map[string]any{"role": role}

	switch c := item["content"].(type) {
	case string:
		msg["content"] = c
	case []any:
		var textOnly string
		var parts []map[string]any
		hasImage := false
		for _, p := range c {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			switch pm["type"] {
			case "input_text", "output_text", "text":
				if t, ok := pm["text"].(string); ok {
					textOnly += t
					parts = append(parts, map[string]any{"type": "text", "text": t})
				}
			case "input_image", "image_url", "image":
				hasImage = true
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": responsesImageURL(pm),
				})
			}
		}
		if hasImage {
			msg["content"] = anySlice(parts)
		} else {
			msg["content"] = textOnly
		}
	default:
		msg["content"] = ""
	}
	return msg
}

// responsesFunctionCallToOpenAI converts a Responses function_call input item
// (the model's prior tool call, replayed as history) into a chat assistant
// message bearing tool_calls. call_id is preserved verbatim.
func responsesFunctionCallToOpenAI(item map[string]any) map[string]any {
	cid := callID(item)
	name, _ := item["name"].(string)
	args := stringifyArguments(item["arguments"])
	return map[string]any{
		"role":    "assistant",
		"content": nil,
		"tool_calls": anySlice([]map[string]any{
			{
				"id":   cid,
				"type": "function",
				"function": map[string]any{
					"name":      name,
					"arguments": args,
				},
			},
		}),
	}
}

// responsesFunctionCallOutputToOpenAI converts a Responses function_call_output
// input item (the tool result the client sends back) into a chat `tool` message.
// tool_call_id is the same call_id, preserved verbatim (the load-bearing link).
func responsesFunctionCallOutputToOpenAI(item map[string]any) map[string]any {
	return map[string]any{
		"role":         "tool",
		"tool_call_id": callID(item),
		"content":      stringifyOutput(item["output"]),
	}
}

// responsesToolsToOpenAI maps the Responses tool array to chat tools.
//
// M5 (live Codex validation) taught us the Responses `tools` array is NOT
// uniformly the flat function shape. A real Codex CLI request carries three
// distinct kinds, and forcing them all into {type:"function", function:{...}}
// produced tools with an EMPTY function.name — upstream rejected the whole
// request with 400 "tools.N.function.name: Field required", so every Codex
// session died at the first turn regardless of the prompt.
//
// The three kinds, and what we do with each:
//
//  1. function (flat)  {type:"function", name, description, parameters}
//     → nest into the chat shape. The common case.
//
//  2. namespace        {type:"namespace", name, description, tools:[...]}
//     → a GROUP of function tools (e.g. Codex's "multi_agent_v1"). Flatten its
//     members into individual chat tools; the member names are what the model
//     actually calls, so they are preserved verbatim. Nested namespaces are
//     flattened recursively with a depth cap so a malformed/cyclic payload
//     cannot blow the stack.
//
//  3. provider built-ins  {type:"web_search"|"file_search"|"computer_use"|...}
//     → NOT expressible in chat-completions `tools` (no name, no schema; they
//     are executed by the provider, not the caller). Dropped, counted, and
//     never emitted as a nameless function. Dropping is the honest behavior:
//     the model simply doesn't see that capability, instead of the request
//     failing outright or a broken tool entry being sent upstream.
//
// Anything with a usable name still nests (forward-compatible with tool types
// we haven't seen yet); only genuinely nameless entries are dropped.
//
// This function stays PURE (no I/O, no globals) like the rest of the file — the
// drop count is exposed separately via ResponsesToolsStats so the handler, not
// the translator, owns observability.
func responsesToolsToOpenAI(tools []any) []map[string]any {
	out, _ := responsesToolsFlatten(tools, 0)
	return out
}

// ResponsesToolsStats reports how a Responses `tools` array translates: kept is
// the number of chat tools produced, dropped is the number of entries that
// could not be represented as chat functions (provider built-ins like
// web_search, nameless entries, malformed namespaces). Pure; intended for the
// handler's observability, so a silently-dropped capability is visible in
// metrics instead of being invisible. Non-array or absent tools → 0, 0.
func ResponsesToolsStats(raw map[string]any) (kept, dropped int) {
	tools, ok := raw["tools"].([]any)
	if !ok {
		return 0, 0
	}
	out, d := responsesToolsFlatten(tools, 0)
	return len(out), d
}

// responsesToolsMaxDepth bounds namespace recursion. Codex nests one level; the
// cap only exists so a hostile or malformed payload can't recurse without end.
const responsesToolsMaxDepth = 4

// responsesToolsFlatten does the real work, returning the chat tools plus the
// count of entries dropped as unrepresentable.
func responsesToolsFlatten(tools []any, depth int) ([]map[string]any, int) {
	var out []map[string]any
	dropped := 0
	if depth > responsesToolsMaxDepth {
		return nil, len(tools)
	}
	for _, t := range tools {
		tm, ok := t.(map[string]any)
		if !ok {
			dropped++
			continue
		}
		// Already-nested chat shape → pass through unchanged.
		if fn, ok := tm["function"].(map[string]any); ok {
			out = append(out, map[string]any{"type": "function", "function": fn})
			continue
		}
		// Namespace group → flatten its members.
		if kind, _ := tm["type"].(string); kind == "namespace" {
			nested, ok := tm["tools"].([]any)
			if !ok {
				dropped++
				continue
			}
			sub, subDropped := responsesToolsFlatten(nested, depth+1)
			out = append(out, sub...)
			dropped += subDropped
			continue
		}
		// Flat shape → nest, but ONLY when it carries a usable name. A tool
		// without a name cannot be a chat function (upstream 400s on it).
		name, _ := tm["name"].(string)
		if name == "" {
			dropped++
			continue
		}
		fn := map[string]any{"name": name}
		copyIfPresent(tm, fn, "description")
		copyIfPresent(tm, fn, "parameters")
		out = append(out, map[string]any{"type": "function", "function": fn})
	}
	return out, dropped
}

// responsesToolChoiceToOpenAI normalizes tool_choice. Strings ("auto"/"none"/
// "required") pass through. The object form {type:"function", name:"x"} (or
// already-nested {function:{name}}) is normalized to the chat nested shape.
func responsesToolChoiceToOpenAI(tc any) any {
	switch v := tc.(type) {
	case string:
		return v
	case map[string]any:
		if _, ok := v["function"].(map[string]any); ok {
			return v // already chat-shaped
		}
		if name, ok := v["name"].(string); ok {
			return map[string]any{
				"type":     "function",
				"function": map[string]any{"name": name},
			}
		}
		return v
	default:
		return tc
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// small pure utilities
// ─────────────────────────────────────────────────────────────────────────────

func itemType(item map[string]any) string {
	t, _ := item["type"].(string)
	return t
}

// callID reads call_id, tolerating the alternate "id" key some items carry.
func callID(item map[string]any) string {
	if c, ok := item["call_id"].(string); ok && c != "" {
		return c
	}
	if c, ok := item["id"].(string); ok {
		return c
	}
	return ""
}

// responsesImageURL normalizes a Responses image part into the chat image_url
// object {url:...}. Accepts image_url as a string or an object with a url field.
func responsesImageURL(pm map[string]any) map[string]any {
	switch iu := pm["image_url"].(type) {
	case string:
		return map[string]any{"url": iu}
	case map[string]any:
		return iu
	}
	// Some Responses variants use a top-level "image" url string.
	if s, ok := pm["image"].(string); ok {
		return map[string]any{"url": s}
	}
	return map[string]any{"url": ""}
}

// stringifyArguments returns tool-call arguments as a JSON string (chat expects
// a string). A string passes through; anything else is marshaled.
func stringifyArguments(v any) string {
	switch a := v.(type) {
	case string:
		return a
	case nil:
		return "{}"
	default:
		b, err := json.Marshal(a)
		if err != nil {
			return "{}"
		}
		return string(b)
	}
}

// stringifyOutput returns a tool result as a string (chat `tool` content). A
// string passes through; structured output is JSON-encoded.
func stringifyOutput(v any) string {
	switch o := v.(type) {
	case string:
		return o
	case nil:
		return ""
	default:
		b, err := json.Marshal(o)
		if err != nil {
			return fmt.Sprintf("%v", o)
		}
		return string(b)
	}
}

// anySlice converts a typed []map[string]any to []any so it serializes as a JSON
// array consistently with the rest of the canonical request.
func anySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

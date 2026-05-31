package translator

import "errors"

// responses.go — Codex Official Layer, M0 SCAFFOLDING ONLY.
//
// This file plants the package layout for the OpenAI **Responses API** as the
// 6th translator format (joining openai/anthropic/gemini/cohere/mistral). It is
// the ingress surface Codex CLI targets (`wire_api="responses"`, POST
// /v1/responses). See proposals/codex-build-plan.md (M0–M5).
//
// SCOPE LOCK (M0): scaffolding only. The translation functions are DECLARED with
// their final signatures but NOT IMPLEMENTED — they return
// ErrResponsesNotImplemented so the build is honest (no fake/no-op translation).
// M1 implements request translation (Responses→canonical chat); M2/M3 implement
// the streaming emitter + tool round-trip. Until then this file:
//   - adds NO behavior to the existing 5-format translator (FormatResponses is
//     deliberately NOT wired into translate.go's switches or AllFormats() yet),
//   - keeps every existing translator path byte-identical,
//   - exists so M1 has a typed home to fill in.
//
// VERIFIED WIRE FACTS (from Codex open source, see codex-feasibility-validation.md):
//   - Codex sends the FULL `input` per request over HTTP; no previous_response_id
//     on the HTTP transport (WS-only, and WS is off by default for custom TOML
//     providers) → Lintasan is a stateless passthrough.
//   - store=false for non-Azure providers → no response store needed.
//   - stream is always true → the streaming emitter (M2) is the real MVP.
//   - Mandatory terminal event: response.completed (+usage); unknown events are
//     ignored by Codex.

// FormatResponses is the OpenAI Responses API format (Codex ingress). It is the
// 6th translator format. NOTE (M0): intentionally not added to AllFormats() or
// the translate.go conversion switches until M1 implements the mapping, so the
// existing 5-format translator behavior is unchanged.
const FormatResponses Format = "responses"

// ErrResponsesNotImplemented marks the M0 scaffolding stubs. Replaced by real
// logic in M1 (request), M2 (streaming response), M3 (tool round-trip).
var ErrResponsesNotImplemented = errors.New("translator: responses format not implemented (M0 scaffolding)")

// ResponsesToOpenAIRequest converts a Responses API request body (parsed JSON)
// into the canonical OpenAI chat-completions request map the existing pipeline
// consumes. M1 will implement: input items→messages, instructions→system,
// tools shape, tool_choice, parallel_tool_calls, function_call_output→tool msgs,
// multimodal input_image parts.
//
// M0: NOT IMPLEMENTED — returns ErrResponsesNotImplemented.
func ResponsesToOpenAIRequest(raw map[string]any) (map[string]any, error) {
	_ = raw
	return nil, ErrResponsesNotImplemented
}

// OpenAIResponseToResponses converts a canonical OpenAI chat-completion response
// (non-streaming) into a Responses-shaped object. Provided for completeness;
// Codex itself always streams (the streaming emitter is M2). M1/M2 implement.
//
// M0: NOT IMPLEMENTED — returns ErrResponsesNotImplemented.
func OpenAIResponseToResponses(raw map[string]any) (map[string]any, error) {
	_ = raw
	return nil, ErrResponsesNotImplemented
}

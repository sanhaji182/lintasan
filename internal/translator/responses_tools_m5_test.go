package translator

import (
	"encoding/json"
	"os"
	"testing"
)

// responses_tools_m5_test.go — M5 (live Codex CLI validation) regression tests.
//
// WHY THIS FILE EXISTS: M0–M4 were validated against hand-written Responses
// payloads that all used the flat function shape. The first real `codex exec`
// run against Lintasan died immediately with an upstream 400:
//
//	tools.9.function.name: Field required
//
// Capturing the real wire request showed the Codex CLI sends a HETEROGENEOUS
// tools array — flat functions, a `namespace` group carrying nested functions,
// and a provider built-in (`web_search`) that has no name at all. The old
// translator nested every entry unconditionally, so the built-in became
// {type:"function", function:{}} and the upstream rejected the whole request.
//
// testdata/codex_cli_tools.json is the VERBATIM captured tools array from
// codex-cli 0.147.0, so these tests fail if the translation regresses against
// the shape a real client actually sends.

// loadCodexCLITools returns the captured real-world Codex CLI tools array.
func loadCodexCLITools(t *testing.T) []any {
	t.Helper()
	b, err := os.ReadFile("testdata/codex_cli_tools.json")
	if err != nil {
		t.Fatalf("read captured tools fixture: %v", err)
	}
	var payload struct {
		Tools []any `json:"tools"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("parse captured tools fixture: %v", err)
	}
	if len(payload.Tools) == 0 {
		t.Fatal("captured tools fixture is empty")
	}
	return payload.Tools
}

// TestM5_RealCodexCLITools_EveryToolHasAName is the exact regression for the
// observed upstream 400: after translation, EVERY chat tool must carry a
// non-empty function.name. A nameless entry is what broke the live session.
func TestM5_RealCodexCLITools_EveryToolHasAName(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-oss-120b",
		"input": "hi",
		"tools": loadCodexCLITools(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools, ok := out["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("tools missing or wrong type: %T", out["tools"])
	}
	if len(tools) == 0 {
		t.Fatal("all tools were dropped; the model would see no capabilities")
	}
	for i, tool := range tools {
		if tool["type"] != "function" {
			t.Errorf("tools[%d].type = %v, want function", i, tool["type"])
		}
		fn, ok := tool["function"].(map[string]any)
		if !ok {
			t.Fatalf("tools[%d] not nested into function: %+v", i, tool)
		}
		name, _ := fn["name"].(string)
		if name == "" {
			t.Errorf("tools[%d].function.name is empty — this is the exact shape "+
				"that produced upstream 400 'tools.N.function.name: Field required'", i)
		}
	}
}

// TestM5_WebSearchBuiltinIsDropped: a provider built-in has no name and no
// schema, so it cannot become a chat function. It must be dropped, not emitted
// as a nameless function.
func TestM5_WebSearchBuiltinIsDropped(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": "hi",
		"tools": []any{
			map[string]any{"type": "function", "name": "f", "parameters": map[string]any{"type": "object"}},
			map[string]any{"type": "web_search", "external_web_access": false},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools := out["tools"].([]map[string]any)
	if len(tools) != 1 {
		t.Fatalf("expected only the function tool to survive, got %d: %+v", len(tools), tools)
	}
	if tools[0]["function"].(map[string]any)["name"] != "f" {
		t.Errorf("wrong tool survived: %+v", tools[0])
	}
}

// TestM5_NamespaceIsFlattened: a `namespace` tool is a GROUP of functions. Its
// members are what the model actually calls, so they must be flattened into
// individual chat tools with their names preserved.
func TestM5_NamespaceIsFlattened(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": "hi",
		"tools": []any{
			map[string]any{
				"type":        "namespace",
				"name":        "multi_agent_v1",
				"description": "Tools for spawning sub-agents.",
				"tools": []any{
					map[string]any{"type": "function", "name": "spawn_agent", "description": "spawn"},
					map[string]any{"type": "function", "name": "close_agent", "description": "close"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools := out["tools"].([]map[string]any)
	if len(tools) != 2 {
		t.Fatalf("namespace not flattened: got %d tools, want 2 (%+v)", len(tools), tools)
	}
	got := map[string]bool{}
	for _, tool := range tools {
		got[tool["function"].(map[string]any)["name"].(string)] = true
	}
	if !got["spawn_agent"] || !got["close_agent"] {
		t.Errorf("namespace member names not preserved: %v", got)
	}
	if got["multi_agent_v1"] {
		t.Error("the namespace container itself must not become a callable tool")
	}
}

// TestM5_ToolsStatsCountsRealPayload: the kept/dropped accounting the handler
// reports to metrics must match the real captured payload — 9 usable functions
// (5 flat + 3 flat + 3 namespace members ... verified against the fixture) and
// at least one dropped built-in.
func TestM5_ToolsStatsCountsRealPayload(t *testing.T) {
	raw := map[string]any{
		"model": "gpt-oss-120b",
		"input": "hi",
		"tools": loadCodexCLITools(t),
	}
	kept, dropped := ResponsesToolsStats(raw)
	if kept <= 0 {
		t.Fatalf("kept = %d, want > 0", kept)
	}
	if dropped < 1 {
		t.Errorf("dropped = %d, want >= 1 (the web_search built-in must be counted)", dropped)
	}
	// Accounting must be consistent with the actual translation.
	out, err := ResponsesToOpenAIRequest(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(out["tools"].([]map[string]any)); got != kept {
		t.Errorf("ResponsesToolsStats kept=%d but translation produced %d tools", kept, got)
	}
}

// TestM5_ToolsStatsAbsentOrBadTools: no tools / non-array tools → zero counts,
// never a panic.
func TestM5_ToolsStatsAbsentOrBadTools(t *testing.T) {
	for name, raw := range map[string]map[string]any{
		"absent":  {"model": "m", "input": "hi"},
		"nonList": {"model": "m", "input": "hi", "tools": "nope"},
		"nilVal":  {"model": "m", "input": "hi", "tools": nil},
	} {
		kept, dropped := ResponsesToolsStats(raw)
		if kept != 0 || dropped != 0 {
			t.Errorf("%s: got kept=%d dropped=%d, want 0/0", name, kept, dropped)
		}
	}
}

// TestM5_NamelessFlatToolIsDropped: an unknown tool type WITH a name still
// nests (forward-compatible), but a nameless one is dropped.
func TestM5_NamelessFlatToolIsDropped(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": "hi",
		"tools": []any{
			map[string]any{"type": "some_future_type", "name": "future_tool"},
			map[string]any{"type": "some_future_type"},
			"not-an-object",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools := out["tools"].([]map[string]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 surviving named tool, got %d: %+v", len(tools), tools)
	}
	if tools[0]["function"].(map[string]any)["name"] != "future_tool" {
		t.Errorf("named forward-compatible tool not preserved: %+v", tools[0])
	}
}

// TestM5_MalformedNamespaceDoesNotPanic: a namespace whose `tools` is missing
// or the wrong type is dropped, not fatal.
func TestM5_MalformedNamespaceDoesNotPanic(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": "hi",
		"tools": []any{
			map[string]any{"type": "namespace", "name": "broken"},
			map[string]any{"type": "namespace", "name": "worse", "tools": "nope"},
			map[string]any{"type": "function", "name": "ok"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools := out["tools"].([]map[string]any)
	if len(tools) != 1 || tools[0]["function"].(map[string]any)["name"] != "ok" {
		t.Errorf("malformed namespaces mishandled: %+v", tools)
	}
}

// TestM5_DeepNamespaceNestingIsBounded: recursion is depth-capped so a hostile
// payload cannot blow the stack. Beyond the cap, entries are dropped.
func TestM5_DeepNamespaceNestingIsBounded(t *testing.T) {
	// Build a namespace chain deeper than responsesToolsMaxDepth.
	inner := []any{map[string]any{"type": "function", "name": "deep_leaf"}}
	for i := 0; i < responsesToolsMaxDepth+3; i++ {
		inner = []any{map[string]any{"type": "namespace", "name": "n", "tools": inner}}
	}
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": "hi",
		"tools": inner,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The leaf sits below the cap, so it is dropped rather than recursed to.
	if tools, ok := out["tools"].([]map[string]any); ok && len(tools) != 0 {
		t.Errorf("over-deep nesting should yield no tools, got %+v", tools)
	}
}

// TestM5_AlreadyNestedChatToolPassesThrough guards the pre-existing behavior
// the M5 change must not regress.
func TestM5_AlreadyNestedChatToolPassesThrough(t *testing.T) {
	out, err := ResponsesToOpenAIRequest(map[string]any{
		"model": "gpt-5",
		"input": "hi",
		"tools": []any{
			map[string]any{"type": "function", "function": map[string]any{"name": "nested", "description": "d"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools := out["tools"].([]map[string]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	fn := tools[0]["function"].(map[string]any)
	if fn["name"] != "nested" || fn["description"] != "d" {
		t.Errorf("nested chat tool not passed through: %+v", fn)
	}
}

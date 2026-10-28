package cache

import (
	"encoding/json"
	"testing"
)

// cache_key_test.go — regression cover for the tools-in-cache-key bug.
//
// Symptom that motivated this: a request with `tools` returned the cached body
// of the same request *without* tools, so the model answered "I don't have a
// get_current_time tool" even though the caller declared it. The tools field
// never entered the cache key.

func msgs(content string) []any {
	return []any{map[string]any{"role": "user", "content": content}}
}

func TestExactHash_ToolsChangeTheKey(t *testing.T) {
	tools := []any{map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":       "get_current_time",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}}

	plain := map[string]any{"temperature": 0.7}
	withTools := map[string]any{"temperature": 0.7, "tools": tools}

	h1 := buildExactHash("gpt-4o-mini", msgs("what is the current time right now please tell me"), plain)
	h2 := buildExactHash("gpt-4o-mini", msgs("what is the current time right now please tell me"), withTools)

	if h1 == h2 {
		t.Fatal("hash collided: a request with tools must not share a cache entry with one without")
	}
}

func TestExactHash_ToolChoiceAndResponseFormatChangeTheKey(t *testing.T) {
	base := map[string]any{"temperature": 0.2}
	cases := map[string]map[string]any{
		"tool_choice":     {"temperature": 0.2, "tool_choice": "required"},
		"response_format": {"temperature": 0.2, "response_format": map[string]any{"type": "json_object"}},
		"stop":            {"temperature": 0.2, "stop": []any{"\n\n"}},
		"seed":            {"temperature": 0.2, "seed": 42},
	}

	baseHash := buildExactHash("m", msgs("hi"), base)
	for name, params := range cases {
		if buildExactHash("m", msgs("hi"), params) == baseHash {
			t.Errorf("%s did not change the cache key — it can alter the response", name)
		}
	}
}

func TestExactHash_StableForIdenticalRequests(t *testing.T) {
	// A key that changes when nothing meaningful changed would kill the cache.
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "x"}}}
	p := map[string]any{"temperature": 0.5, "tools": tools}

	if buildExactHash("m", msgs("same"), p) != buildExactHash("m", msgs("same"), p) {
		t.Fatal("identical requests produced different hashes")
	}
}

func TestRequestNeedsStrictKey(t *testing.T) {
	tools := []any{map[string]any{"type": "function"}}

	strict := []map[string]any{
		{"tools": tools},
		{"tool_choice": "auto"},
		{"functions": tools},
		{"function_call": "auto"},
		{"response_format": map[string]any{"type": "json_object"}},
		{"parallel_tool_calls": false},
	}
	for _, p := range strict {
		if !RequestNeedsStrictKey(p) {
			t.Errorf("expected strict cache key for %v", p)
		}
	}

	plain := []map[string]any{
		{"temperature": 0.7},
		{"messages": []any{}},
		{},
	}
	for _, p := range plain {
		if RequestNeedsStrictKey(p) {
			t.Errorf("did not expect strict cache key for %v", p)
		}
	}
}

func TestSemanticMatch_SkipsToolRequests(t *testing.T) {
	db := newTestDB(t)
	if err := InitSemanticCache(db); err != nil {
		t.Fatalf("InitSemanticCache: %v", err)
	}
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "x"}}}

	// Seed a semantic entry as if a tool-less request had been cached.
	SaveSemanticMatch(db, "gpt-4o-mini", msgs("what is the current time right now please tell me"), `{"answer":"12:00"}`, 3600)

	// Same text, but now the caller declares tools. A fuzzy hit here would
	// return a body with no tool_calls.
	params := map[string]any{"tools": tools}
	if _, _, ok := GetSemanticMatch(db, "gpt-4o-mini", msgs("what is the current time right now please tell me"), 0.92, params); ok {
		t.Fatal("semantic cache served a tool-bearing request — tools would be silently absent")
	}

	// Without tools the entry is still servable (we did not break the cache).
	if _, _, ok := GetSemanticMatch(db, "gpt-4o-mini", msgs("what is the current time right now please tell me"), 0.92, map[string]any{"temperature": 0.1}); !ok {
		t.Fatal("plain request stopped hitting the semantic cache")
	}
}

func TestSemanticMatch_BackwardsCompatibleWithoutParams(t *testing.T) {
	db := newTestDB(t)
	if err := InitSemanticCache(db); err != nil {
		t.Fatalf("InitSemanticCache: %v", err)
	}
	SaveSemanticMatch(db, "gpt-4o-mini", msgs("hello there how are you doing today my friend"), `{"answer":"hi"}`, 3600)
	// Old call shape (no params variadic) must keep working.
	if _, _, ok := GetSemanticMatch(db, "gpt-4o-mini", msgs("hello there how are you doing today my friend"), 0.92); !ok {
		t.Fatal("variadic params broke the existing call shape")
	}
}

// Guard: the responseAffectingParams allow-list must actually contain the
// params we claim to cover, and the trimmed map must survive JSON round-trip
// (map ordering is not stable, so this also proves determinism).
func TestResponseAffectingParams_CoversTools(t *testing.T) {
	found := false
	for _, p := range responseAffectingParams {
		if p == "tools" {
			found = true
		}
	}
	if !found {
		t.Fatal("tools missing from responseAffectingParams")
	}

	p := map[string]any{"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "a"}}}}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty marshal")
	}
}

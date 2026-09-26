package combo

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func sampleCombosJSON() string {
	return `[
  {
    "name": "coding-combo",
    "strategy": "priority",
    "sticky_limit": 3,
    "entries": [
      {
        "model": "deepseek-v4-pro",
        "connection_ids": ["cc-alpha-001"],
        "api_keys": ["sk-aaa", "sk-bbb"]
      },
      {
        "model": "gpt-4o",
        "connection_ids": ["sumopod-001"]
      }
    ]
  },
  {
    "name": "fallback-combo",
    "strategy": "round-robin",
    "entries": [
      {
        "model": "claude-3.5-sonnet",
        "connection_ids": ["main-001", "main-002"]
      },
      {
        "model": "gemini-pro",
        "connection_ids": ["backup-001"]
      }
    ]
  }
]`
}

func TestLoadCombos(t *testing.T) {
	e := New()
	if err := e.LoadFromSettings(sampleCombosJSON()); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	combos := e.List()
	if len(combos) != 2 {
		t.Fatalf("expected 2 combos, got %d", len(combos))
	}

	// Verify coding-combo
	var cc *Combo
	for i := range combos {
		if combos[i].Name == "coding-combo" {
			cc = &combos[i]
			break
		}
	}
	if cc == nil {
		t.Fatal("coding-combo not found")
	}
	if cc.Strategy != StrategyPriority {
		t.Errorf("expected priority, got %s", cc.Strategy)
	}
	if cc.StickyLimit != 3 {
		t.Errorf("expected stickyLimit 3, got %d", cc.StickyLimit)
	}
	if len(cc.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cc.Entries))
	}
	if len(cc.Entries[0].APIKeys) != 2 {
		t.Fatalf("expected 2 API keys, got %d", len(cc.Entries[0].APIKeys))
	}
}

func TestPriorityStrategy(t *testing.T) {
	e := New()
	if err := e.LoadFromSettings(sampleCombosJSON()); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	// First resolve should return deepseek-v4-pro first
	res, err := e.Resolve("coding-combo")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if res[0].Model != "deepseek-v4-pro" {
		t.Errorf("expected deepseek-v4-pro first, got %s", res[0].Model)
	}
	if res[0].ConnectionID != "cc-alpha-001" {
		t.Errorf("expected cc-alpha-001, got %s", res[0].ConnectionID)
	}
	if res[0].APIKey == "" {
		t.Error("expected non-empty API key")
	}

	// After many failures > stickyLimit, should advance to next entry
	for i := 0; i <= 3; i++ {
		e.RecordFailure("coding-combo")
	}

	res, err = e.Resolve("coding-combo")
	if err != nil {
		t.Fatalf("Resolve after failures: %v", err)
	}
	if res[0].Model != "gpt-4o" {
		t.Errorf("expected gpt-4o after failures, got %s", res[0].Model)
	}
}

func TestPriorityStickySuccessResets(t *testing.T) {
	e := New()
	if err := e.LoadFromSettings(sampleCombosJSON()); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	// 2 failures (below limit of 3)
	e.RecordFailure("coding-combo")
	e.RecordFailure("coding-combo")

	// A success resets fail counter
	e.RecordSuccess("coding-combo")

	// 3 more failures — should still NOT advance because success reset fail count
	for i := 0; i < 3; i++ {
		e.RecordFailure("coding-combo")
	}

	res, err := e.Resolve("coding-combo")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// first 2 + success reset + 3 failures = failCount is 3, which does NOT exceed stickyLimit=3
	if res[0].Model != "deepseek-v4-pro" {
		t.Errorf("expected deepseek-v4-pro after success reset, got %s", res[0].Model)
	}
}

func TestRoundRobinStrategy(t *testing.T) {
	e := New()
	if err := e.LoadFromSettings(sampleCombosJSON()); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	// First call: claude-3.5-sonnet first
	res1, err := e.Resolve("fallback-combo")
	if err != nil {
		t.Fatalf("Resolve 1: %v", err)
	}
	if res1[0].Model != "claude-3.5-sonnet" {
		t.Errorf("Resolve 1: expected claude-3.5-sonnet, got %s", res1[0].Model)
	}

	// Second call: gemini-pro first
	res2, err := e.Resolve("fallback-combo")
	if err != nil {
		t.Fatalf("Resolve 2: %v", err)
	}
	if res2[0].Model != "gemini-pro" {
		t.Errorf("Resolve 2: expected gemini-pro, got %s", res2[0].Model)
	}

	// Third call: back to claude
	res3, err := e.Resolve("fallback-combo")
	if err != nil {
		t.Fatalf("Resolve 3: %v", err)
	}
	if res3[0].Model != "claude-3.5-sonnet" {
		t.Errorf("Resolve 3: expected claude-3.5-sonnet, got %s", res3[0].Model)
	}
}

func TestMultiAccountKeyRotation(t *testing.T) {
	e := New()
	jsonStr := `[{
    "name": "test-combo",
    "strategy": "priority",
    "sticky_limit": 1,
    "entries": [
      {
        "model": "test-model",
        "connection_ids": ["conn-1"],
        "api_keys": ["key-a", "key-b", "key-c"]
      }
    ]
  }]`
	if err := e.LoadFromSettings(jsonStr); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	keys := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		res, err := e.Resolve("test-combo")
		if err != nil {
			t.Fatalf("Resolve %d: %v", i, err)
		}
		keys = append(keys, res[0].APIKey)
	}

	// Should have cycled through key-a, key-b, key-c
	expected := strings.Join([]string{"key-a", "key-b", "key-c"}, ",")
	got := strings.Join(keys, ",")
	if got != expected {
		t.Errorf("expected keys %s, got %s", expected, got)
	}

	// Fourth call: should wrap back to key-a
	res, err := e.Resolve("test-combo")
	if err != nil {
		t.Fatalf("Resolve 4: %v", err)
	}
	if res[0].APIKey != "key-a" {
		t.Errorf("expected wrap to key-a, got %s", res[0].APIKey)
	}
}

func TestEmptyCombosReturnsError(t *testing.T) {
	e := New()
	if err := e.LoadFromSettings(""); err != nil {
		t.Fatalf("LoadFromSettings empty: %v", err)
	}

	_, err := e.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown combo")
	}
}

func TestConcurrentResolution(t *testing.T) {
	e := New()
	// Single-entry round-robin combo — safe for concurrent access
	jsonStr := `[{
    "name": "conc",
    "strategy": "round-robin",
    "entries": [
      { "model": "m1", "connection_ids": ["c1"] },
      { "model": "m2", "connection_ids": ["c2"] }
    ]
  }]`
	if err := e.LoadFromSettings(jsonStr); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan string, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := e.Resolve("conc")
			if err != nil {
				t.Errorf("concurrent Resolve: %v", err)
				return
			}
			results <- res[0].Model
		}()
	}
	wg.Wait()
	close(results)

	countM1, countM2 := 0, 0
	for m := range results {
		switch m {
		case "m1":
			countM1++
		case "m2":
			countM2++
		}
	}

	// Both models should have been resolved
	if countM1 == 0 {
		t.Error("m1 was never resolved")
	}
	if countM2 == 0 {
		t.Error("m2 was never resolved")
	}

	// Concurrent RecordSuccess/Failure should not panic
	var wg2 sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			e.RecordSuccess("conc")
		}()
	}
	wg2.Wait()
}

func TestLoadInvalidJSON(t *testing.T) {
	e := New()
	err := e.LoadFromSettings(`not json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestComboWithNoEntries(t *testing.T) {
	e := New()
	jsonStr := `[{"name": "empty", "strategy": "priority", "entries": []}]`
	if err := e.LoadFromSettings(jsonStr); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	_, err := e.Resolve("empty")
	if err == nil {
		t.Fatal("expected error for combo with no entries")
	}
}

func TestRecordFailureNoSticky(t *testing.T) {
	// RecordFailure on a round-robin combo should be a no-op (not panic)
	e := New()
	jsonStr := `[{"name": "rr", "strategy": "round-robin", "entries": [{"model": "m", "connection_ids": ["c"]}]}]`
	if err := e.LoadFromSettings(jsonStr); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}
	// Should not panic
	e.RecordFailure("rr")
}

func TestLoadCombos_ModelsArrayCompatibility(t *testing.T) {
	e := New()
	legacyJSON := `[{"name":"legacy-combo","strategy":"priority","models":["m1","m2"]}]`
	if err := e.LoadFromSettings(legacyJSON); err != nil {
		t.Fatalf("LoadFromSettings failed: %v", err)
	}

	combos := e.List()
	if len(combos) != 1 {
		t.Fatalf("expected 1 combo, got %d", len(combos))
	}
	if len(combos[0].Entries) != 2 {
		t.Fatalf("expected 2 synthesized entries, got %d", len(combos[0].Entries))
	}
	if combos[0].Entries[0].Model != "m1" || combos[0].Entries[1].Model != "m2" {
		t.Errorf("unexpected models: %v", combos[0].Entries)
	}

	// Should also be resolvable
	res, err := e.Resolve("legacy-combo")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 resolved entries, got %d", len(res))
	}
	if res[0].Model != "m1" || res[1].Model != "m2" {
		t.Errorf("unexpected resolved entries: %v", res)
	}
}

func TestResolve_EmptyConnectionIDs(t *testing.T) {
	e := New()
	cfg := `[{"name":"any-conn","strategy":"priority","entries":[{"model":"qwen-3"},{"model":"deepseek-v4"}]}]`
	if err := e.LoadFromSettings(cfg); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	res, err := e.Resolve("any-conn")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 resolved candidates, got %d", len(res))
	}
	if res[0].Model != "qwen-3" || res[0].ConnectionID != "" {
		t.Errorf("expected qwen-3 with empty ConnectionID, got %+v", res[0])
	}
}

func TestResolveSupportsProviderIdentity(t *testing.T) {
	e := New()
	jsonStr := `[{"name":"pooled","strategy":"priority","entries":[{"model":"shared","provider_id":"provider:qoder:api.qoder.com:/chat/completions"}]}]`
	if err := e.LoadFromSettings(jsonStr); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}
	resolved, err := e.Resolve("pooled")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved) != 1 || resolved[0].ProviderID != "provider:qoder:api.qoder.com:/chat/completions" || resolved[0].ConnectionID != "" {
		t.Fatalf("provider identity not preserved: %+v", resolved)
	}
}

func TestResolveSupportsCanonicalPinnedConnectionID(t *testing.T) {
	e := New()
	jsonStr := `[{"name":"pinned","strategy":"priority","entries":[{"model":"shared","connection_id":"conn-a"},{"model":"shared","connection_id":"conn-b"}]}]`
	if err := e.LoadFromSettings(jsonStr); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}
	resolved, err := e.Resolve("pinned")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(resolved) != 2 || resolved[0].ConnectionID != "conn-a" || resolved[1].ConnectionID != "conn-b" {
		t.Fatalf("canonical pinned entries not preserved: %+v", resolved)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := `[{"name":"test","strategy":"priority","sticky_limit":5,"entries":[{"model":"gpt-4","connection_ids":["c1","c2"],"api_keys":["sk-1"]}]}]`
	e := New()
	if err := e.LoadFromSettings(original); err != nil {
		t.Fatalf("LoadFromSettings: %v", err)
	}

	combos := e.List()
	if len(combos) != 1 {
		t.Fatalf("expected 1 combo, got %d", len(combos))
	}

	b, err := json.Marshal(combos)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), "gpt-4") {
		t.Errorf("round-trip failed, JSON: %s", b)
	}
}

// ---------------------------------------------------------------------------
// Example for godoc
// ---------------------------------------------------------------------------
func Example() {
	e := New()
	jsonStr := `[{"name":"example","strategy":"round-robin","entries":[{"model":"gpt-4","connection_ids":["conn-1"],"api_keys":["sk-abc"]}]}]`
	_ = e.LoadFromSettings(jsonStr)

	res, _ := e.Resolve("example")
	fmt.Println(res[0].Model, res[0].ConnectionID, res[0].APIKey)
	// Output: gpt-4 conn-1 sk-abc
}

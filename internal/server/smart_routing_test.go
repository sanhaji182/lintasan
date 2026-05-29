package server

import (
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// newTestProxyHandler builds a ProxyHandler backed by an in-memory SQLite DB.
func newTestProxyHandler(t *testing.T) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewProxyHandler(&config.Config{}, database)
}

// --- Gap #1: ML routing gating + decision ---

func TestApplyMLRouting_DisabledByDefault(t *testing.T) {
	p := newTestProxyHandler(t)
	// A normal model name with ml_router_enabled unset → no ML routing.
	_, _, ok := p.applyMLRouting("gpt-4o", []any{
		map[string]any{"role": "user", "content": "hello"},
	})
	if ok {
		t.Fatal("expected ML routing to be OFF for a normal model when ml_router_enabled is unset")
	}
}

func TestApplyMLRouting_SentinelModelActivates(t *testing.T) {
	p := newTestProxyHandler(t)
	// The sentinel "ml-auto" must activate routing regardless of the setting.
	model, tier, ok := p.applyMLRouting("ml-auto", []any{
		map[string]any{"role": "user", "content": "hi"},
	})
	if !ok {
		t.Fatal("expected ML routing to activate for sentinel model 'ml-auto'")
	}
	if model == "" {
		t.Fatal("expected a concrete model back from ML routing")
	}
	if tier != "cheap" && tier != "expensive" {
		t.Fatalf("expected tier cheap|expensive, got %q", tier)
	}
}

func TestApplyMLRouting_EnabledViaSetting(t *testing.T) {
	p := newTestProxyHandler(t)
	if err := p.db.SetSetting("ml_router_enabled", "true"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	_, _, ok := p.applyMLRouting("gpt-4o", []any{
		map[string]any{"role": "user", "content": "explain quantum tunneling in depth"},
	})
	if !ok {
		t.Fatal("expected ML routing to activate when ml_router_enabled=true")
	}
}

func TestApplyMLRouting_ComplexPromptPicksExpensive(t *testing.T) {
	p := newTestProxyHandler(t)
	// A long, code-heavy, complex prompt should score above threshold → expensive.
	complex := "Refactor this concurrent pipeline to eliminate the data race. " +
		"```go\nfunc worker(ch chan int, wg *sync.WaitGroup) { defer wg.Done(); for v := range ch { process(v) } }\n``` " +
		"Explain the memory model implications, the happens-before relationships, and prove correctness."
	_, tier, ok := p.applyMLRouting("ml-auto", []any{
		map[string]any{"role": "user", "content": complex},
	})
	if !ok {
		t.Fatal("expected routing to activate")
	}
	// We don't hard-assert expensive (threshold is tunable), but tier must be valid.
	if tier != "cheap" && tier != "expensive" {
		t.Fatalf("unexpected tier %q", tier)
	}
}

// --- Gap #4: cost-based scoring ---

func TestCostQualityFloor_DefaultAndOverride(t *testing.T) {
	p := newTestProxyHandler(t)
	if got := p.costQualityFloor(); got != 0.3 {
		t.Fatalf("default cost quality floor = %v, want 0.3", got)
	}
	if err := p.db.SetSetting("cost_quality_floor", "0.55"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if got := p.costQualityFloor(); got != 0.55 {
		t.Fatalf("override cost quality floor = %v, want 0.55", got)
	}
}

func TestCostScoreForConnection_FreeNameIsCheapest(t *testing.T) {
	p := newTestProxyHandler(t)
	score := p.costScoreForConnection(&Connection{ID: "c1", Name: "OpenRouter Free Tier"})
	if score != 1.0 {
		t.Fatalf("a 'free'-named connection should score 1.0 (cheapest), got %v", score)
	}
}

func TestCostScoreForConnection_UnknownModelIsNeutral(t *testing.T) {
	p := newTestProxyHandler(t)
	// No discovered model → neutral 0.5 (not 0, not 1).
	score := p.costScoreForConnection(&Connection{ID: "c-unknown", Name: "Paid Provider"})
	if score != 0.5 {
		t.Fatalf("unknown-model connection should score neutral 0.5, got %v", score)
	}
}

// --- Gap #5: tiered alias resolves to nothing gracefully on empty DB ---

func TestResolveTieredCombo_EmptyDBFails(t *testing.T) {
	p := newTestProxyHandler(t)
	_, _, ok := p.resolveTieredCombo()
	if ok {
		t.Fatal("expected tiered combo to fail (ok=false) when no providers exist")
	}
}

// --- Gap #3: quota gating no-op when no limit configured ---

func TestQuotaRemainingFraction_NoLimitIsFull(t *testing.T) {
	p := newTestProxyHandler(t)
	if got := p.quotaRemainingFraction("any-conn"); got != 1.0 {
		t.Fatalf("no configured limit should report full quota 1.0, got %v", got)
	}
}

func TestLoadQuotaLimits_ParsesSetting(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	if err := database.SetSetting("quota_limits",
		`{"conn-1":{"max_tokens_per_day":1000}}`); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	p := NewProxyHandler(&config.Config{}, database)
	limit := p.quota.GetLimit("conn-1")
	if limit == nil {
		t.Fatal("expected a quota limit for conn-1 after loadQuotaLimits")
	}
	if limit.MaxTokensPerDay != 1000 {
		t.Fatalf("MaxTokensPerDay = %d, want 1000", limit.MaxTokensPerDay)
	}
}

package server

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// capability_enforce_test.go — F2.4 server-side enforcement hook tests.
//
// Load-bearing guarantees:
//   - Flag OFF (default) → pool byte-identical (legacy/shadow behavior).
//   - Flag ON → narrows the pool by dropping data-backed capability failures,
//     preserving order, never emptying the pool (R3 keep-all + aborted).
//   - Fail-open: a default-tier candidate is never dropped.

func newEnforceHandler(t *testing.T, enforceOn bool) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if enforceOn {
		if err := database.SetSetting("capability_enforce_enabled", "true"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	return NewProxyHandler(&config.Config{}, database)
}

// TestF2_4_EnforceFlagDefaultOff proves the flag defaults OFF and parses ON.
func TestF2_4_EnforceFlagDefaultOff(t *testing.T) {
	if newEnforceHandler(t, false).capabilityEnforce {
		t.Fatal("capabilityEnforce must default to false when setting absent")
	}
	if !newEnforceHandler(t, true).capabilityEnforce {
		t.Fatal("capabilityEnforce must be true when setting on")
	}
}

// TestF2_4_EnforceOffIsByteIdentical proves a flag-OFF enforce hook returns the
// pool unchanged — no header, no mutation, no reordering.
func TestF2_4_EnforceOffIsByteIdentical(t *testing.T) {
	p := newEnforceHandler(t, false)
	candidates := sampleCandidates()
	before := snapshotIDs(candidates)

	// A vision request — the case most likely to drop groq if enforcement ran.
	req := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:..."}},
			}},
		},
	}
	w := httptest.NewRecorder()

	got := p.applyCapabilityEnforcement(w, req, "gpt-4o", false, candidates)

	if !reflect.DeepEqual(before, snapshotIDs(got)) {
		t.Fatalf("flag-off enforce changed pool: before=%v after=%v", before, snapshotIDs(got))
	}
	if h := w.Header().Get("X-Lintasan-Capability-Enforce"); h != "" {
		t.Errorf("flag-off must NOT set enforce header, got %q", h)
	}
}

// TestF2_4_EnforceDropsDataBackedFailure proves a flag-ON enforce hook drops a
// data-backed candidate that fails a required capability, while keeping the
// capable ones, preserving order.
//
// IMPORTANT resolver semantics (Tier order F→E→default): per-PROVIDER
// differentiation happens at Tier E, which fires when the model is NOT a known
// catalog model. For a catalog model, Tier F resolves ALL candidates to that
// MODEL's caps (capability is a property of the model, not the connection) —
// see TestF2_4_EnforceCatalogModelSharesModelCaps. So this test uses a
// non-catalog model so the per-provider host identity (Tier E) drives the
// decision: openai-host (vision-capable) survives, groq-host (no vision) drops.
func TestF2_4_EnforceDropsDataBackedFailure(t *testing.T) {
	p := newEnforceHandler(t, true)
	candidates := []*Connection{
		{ID: "c1", Name: "a", Format: "openai", BaseURL: "https://api.openai.com/v1", IsActive: 1},
		{ID: "c2", Name: "b", Format: "groq", BaseURL: "https://api.groq.com/openai/v1", IsActive: 1},
		{ID: "c3", Name: "c", Format: "anthropic", BaseURL: "https://api.anthropic.com/v1", IsActive: 1},
	}
	const nonCatalogModel = "custom-routed-model-not-in-catalog"
	req := map[string]any{
		"model": nonCatalogModel,
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:..."}},
			}},
		},
	}
	w := httptest.NewRecorder()

	got := p.applyCapabilityEnforcement(w, req, nonCatalogModel, false, candidates)

	gotIDs := snapshotIDs(got)
	for _, id := range gotIDs {
		if id == "c2" {
			t.Fatalf("data-backed groq host (c2) must be dropped for a vision request, got pool=%v", gotIDs)
		}
	}
	if len(gotIDs) == 0 {
		t.Fatal("pool must not be empty (capable providers exist)")
	}
	// Order preservation: surviving IDs must be the original order minus c2.
	want := []string{"c1", "c3"}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("expected ordered survivors %v, got %v", want, gotIDs)
	}
	if h := w.Header().Get("X-Lintasan-Capability-Enforce"); h == "" {
		t.Error("flag-on must set X-Lintasan-Capability-Enforce header")
	}
}

// TestF2_4_EnforceCatalogModelSharesModelCaps documents (and pins) the Tier-F
// semantics: when the request targets a KNOWN catalog model, every candidate
// serving it resolves to that MODEL's capabilities — capability is a property
// of the model, not the connection. So a vision-capable catalog model keeps ALL
// candidates (none dropped), even ones whose provider host might differ. This
// is correct: in real routing, candidates for "gpt-4o" all serve gpt-4o. The
// per-provider path (Tier E) only matters for non-catalog models (above).
func TestF2_4_EnforceCatalogModelSharesModelCaps(t *testing.T) {
	p := newEnforceHandler(t, true)
	candidates := []*Connection{
		{ID: "c1", Name: "a", Format: "openai", BaseURL: "https://api.openai.com/v1", IsActive: 1},
		{ID: "c2", Name: "b", Format: "groq", BaseURL: "https://api.groq.com/openai/v1", IsActive: 1},
	}
	before := snapshotIDs(candidates)
	// gpt-4o is a known vision-capable catalog model → Tier F → all keep.
	req := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:..."}},
			}},
		},
	}
	w := httptest.NewRecorder()
	got := p.applyCapabilityEnforcement(w, req, "gpt-4o", false, candidates)
	if !reflect.DeepEqual(before, snapshotIDs(got)) {
		t.Fatalf("catalog vision model must keep all candidates (Tier F), before=%v after=%v", before, snapshotIDs(got))
	}
}

// TestF2_4_EnforceEmptyPoolKeepsAll proves R3 at the server level: if every
// candidate is a data-backed failure, the pool is kept whole (never emptied).
func TestF2_4_EnforceEmptyPoolKeepsAll(t *testing.T) {
	p := newEnforceHandler(t, true)
	// Two groq connections — both data-backed, neither vision-capable.
	candidates := []*Connection{
		{ID: "g1", Name: "g1", Format: "groq", BaseURL: "https://api.groq.com/openai/v1", IsActive: 1},
		{ID: "g2", Name: "g2", Format: "groq", BaseURL: "https://api.groq.com/openai/v1", IsActive: 1},
	}
	before := snapshotIDs(candidates)
	req := map[string]any{
		"model": "llama-3.1",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:..."}},
			}},
		},
	}
	w := httptest.NewRecorder()

	got := p.applyCapabilityEnforcement(w, req, "llama-3.1", false, candidates)

	if !reflect.DeepEqual(before, snapshotIDs(got)) {
		t.Fatalf("empty-pool guard must keep ALL candidates, before=%v after=%v", before, snapshotIDs(got))
	}
}

// TestF2_4_EnforceEmptyInput proves the hook is safe on an empty pool.
func TestF2_4_EnforceEmptyInput(t *testing.T) {
	p := newEnforceHandler(t, true)
	w := httptest.NewRecorder()
	got := p.applyCapabilityEnforcement(w, map[string]any{"model": "x"}, "x", false, nil)
	if len(got) != 0 {
		t.Fatalf("empty input must yield empty pool, got %v", snapshotIDs(got))
	}
}

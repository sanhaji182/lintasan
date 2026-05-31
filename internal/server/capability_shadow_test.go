package server

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// F2.3 server-side shadow routing tests.
//
// The load-bearing guarantee: shadow evaluation RECORDS capability eligibility
// but NEVER mutates/reorders/filters the candidate pool, and is a complete
// no-op when the flag is off.

func newShadowHandler(t *testing.T, shadowOn bool) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if shadowOn {
		if err := database.SetSetting("capability_shadow_enabled", "true"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	return NewProxyHandler(&config.Config{}, database)
}

func sampleCandidates() []*Connection {
	return []*Connection{
		{ID: "c1", Name: "a", Format: "openai", BaseURL: "https://a", IsActive: 1, Priority: 10},
		{ID: "c2", Name: "b", Format: "groq", BaseURL: "https://b", IsActive: 1, Priority: 5},
		{ID: "c3", Name: "c", Format: "anthropic", BaseURL: "https://c", IsActive: 1, Priority: 1},
	}
}

func snapshotIDs(cands []*Connection) []string {
	ids := make([]string, len(cands))
	for i, c := range cands {
		ids[i] = c.ID
	}
	return ids
}

// TestF2_3_ShadowFlagDefaultOff proves the flag is OFF when the setting is absent.
func TestF2_3_ShadowFlagDefaultOff(t *testing.T) {
	p := newShadowHandler(t, false)
	if p.capabilityShadow {
		t.Fatal("capabilityShadow must default to false when setting absent")
	}
	on := newShadowHandler(t, true)
	if !on.capabilityShadow {
		t.Fatal("capabilityShadow must be true when setting on")
	}
}

// TestF2_3_ShadowDoesNotMutateCandidates is the core F2.3 guarantee: running the
// shadow hook (flag ON, with a vision request that groq would not satisfy) must
// leave the candidate slice byte-identical — same length, same order, same IDs.
func TestF2_3_ShadowDoesNotMutateCandidates(t *testing.T) {
	p := newShadowHandler(t, true)
	candidates := sampleCandidates()
	before := snapshotIDs(candidates)

	// A vision request: groq (no vision) WOULD be excluded under enforcement,
	// so this is the case most likely to tempt mutation. It must not.
	req := map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:..."}},
			}},
		},
	}
	w := httptest.NewRecorder()

	p.runCapabilityShadow(w, req, false, candidates)

	after := snapshotIDs(candidates)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("shadow MUTATED candidates: before=%v after=%v", before, after)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidate count changed: %d", len(candidates))
	}
	// Observability header must be set when on.
	if h := w.Header().Get("X-Lintasan-Capability-Shadow"); h == "" {
		t.Error("expected X-Lintasan-Capability-Shadow header when flag on")
	}
}

// TestF2_3_ShadowFlagOffIsNoOp proves that with the flag OFF, the hook does
// nothing — no header set, candidates untouched.
func TestF2_3_ShadowFlagOffIsNoOp(t *testing.T) {
	p := newShadowHandler(t, false)
	candidates := sampleCandidates()
	before := snapshotIDs(candidates)

	req := map[string]any{"model": "gpt-4o", "stream": true}
	w := httptest.NewRecorder()

	p.runCapabilityShadow(w, req, true, candidates)

	if !reflect.DeepEqual(before, snapshotIDs(candidates)) {
		t.Fatal("flag-off shadow must not touch candidates")
	}
	if h := w.Header().Get("X-Lintasan-Capability-Shadow"); h != "" {
		t.Errorf("flag-off must NOT set shadow header, got %q", h)
	}
}

// TestF2_3_ExtractRequestSignals checks the primitive signal extraction.
func TestF2_3_ExtractRequestSignals(t *testing.T) {
	// tools + json_mode + vision + stream
	req := map[string]any{
		"tools":           []any{map[string]any{"type": "function"}},
		"response_format": map[string]any{"type": "json_object"},
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "hi"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "x"}},
			}},
		},
	}
	s := extractRequestSignals(req, true)
	if !s.Stream || !s.HasTools || !s.HasJSONMode || !s.HasVision {
		t.Fatalf("expected all signals true, got %+v", s)
	}

	// Plain text request: only stream (if asked), nothing else.
	plain := map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}
	ps := extractRequestSignals(plain, false)
	if ps.Stream || ps.HasTools || ps.HasJSONMode || ps.HasVision {
		t.Fatalf("plain text request should have no signals, got %+v", ps)
	}
}

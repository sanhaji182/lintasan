package provider

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// --- F2.2 catalog provider-ID reconciliation --------------------------------

func TestCatalogProviderIDReconciliation(t *testing.T) {
	cases := map[string]struct {
		want   string
		wantOK bool
	}{
		"openai":    {"openai", true},
		"anthropic": {"anthropic", true},
		"gemini":    {"google", true}, // the one true mismatch: SDK gemini == catalog google
		"deepseek":  {"deepseek", true},
		"groq":      {"", false}, // D1 planned catalog gap
	}
	for sdkID, want := range cases {
		got, ok := CatalogProviderIDFor(sdkID)
		if ok != want.wantOK || got != want.want {
			t.Fatalf("CatalogProviderIDFor(%q) = (%q,%v) want (%q,%v)", sdkID, got, ok, want.want, want.wantOK)
		}
	}
}

// --- F2.2 catalog-derived capabilities --------------------------------------

func TestCatalogCapabilitiesForDerivesFromCatalog(t *testing.T) {
	// OpenAI lists vision, tools, streaming, json_mode across its models — all
	// must be reflected (mapped through the canonical vocab: tools->tool_calling).
	caps, ok := CatalogCapabilitiesFor("openai")
	if !ok {
		t.Fatal("openai must be found in catalog")
	}
	for _, c := range []Capability{CapVision, CapToolCalling, CapStreaming, CapJSONMode} {
		if !caps.Has(c) {
			t.Fatalf("openai catalog caps missing %q (got %v)", c, caps.List())
		}
	}
	// "chat" is baseline, never a capability.
	if caps.Has(Capability("chat")) {
		t.Fatal("baseline 'chat' tag must not appear as a capability")
	}
	// Catalog never tags embeddings — that is a DECLARED-ONLY cap (audit signal),
	// so it must NOT be in the catalog-derived set.
	if caps.Has(CapEmbeddings) {
		t.Fatal("catalog does not tag embeddings; it must not be catalog-derived")
	}
}

func TestCatalogCapabilitiesGeminiResolvesViaGoogle(t *testing.T) {
	// Proves the gemini->google reconciliation actually joins data: google models
	// tag vision + json_mode in the catalog.
	caps, ok := CatalogCapabilitiesFor("gemini")
	if !ok {
		t.Fatal("gemini must resolve to catalog provider 'google'")
	}
	if !caps.Has(CapVision) || !caps.Has(CapJSONMode) {
		t.Fatalf("gemini(->google) catalog caps missing vision/json_mode: %v", caps.List())
	}
}

func TestCatalogCapabilitiesGroqGapIsHonest(t *testing.T) {
	// D1: Groq is official but unbuildable in the catalog. The join must report
	// found=false and an empty set, NOT fabricate caps.
	caps, ok := CatalogCapabilitiesFor("groq")
	if ok {
		t.Fatal("groq has no catalog entry (D1); must report found=false")
	}
	if len(caps) != 0 {
		t.Fatalf("groq catalog caps must be empty, got %v", caps.List())
	}
}

// --- F2.2 diagnostic facade --------------------------------------------------

func TestCapabilityCatalogShape(t *testing.T) {
	rows := CapabilityCatalog()
	// One row per known official provider (the stable F2.1 set).
	if len(rows) != len(KnownOfficialProviders()) {
		t.Fatalf("expected %d rows, got %d", len(KnownOfficialProviders()), len(rows))
	}
	// Rows must be in stable sorted provider order.
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Provider > rows[i].Provider {
			t.Fatalf("rows not sorted by provider: %q before %q", rows[i-1].Provider, rows[i].Provider)
		}
	}

	byID := map[string]CapabilityInfo{}
	for _, r := range rows {
		byID[r.Provider] = r
	}

	// OpenAI: declares embeddings (not in catalog) => embeddings is DeclaredOnly.
	openai := byID["openai"]
	if !openai.InCatalog {
		t.Fatal("openai must be in catalog")
	}
	if !containsCap(openai.DeclaredOnly, CapEmbeddings) {
		t.Fatalf("openai embeddings must be declared_only (audit signal), got declared_only=%v", openai.DeclaredOnly)
	}
	if !containsCap(openai.Union, CapEmbeddings) || !containsCap(openai.Union, CapVision) {
		t.Fatalf("openai union must include declared+catalog caps, got %v", openai.Union)
	}

	// DeepSeek: declares reasoning (not catalog-tagged) => reasoning DeclaredOnly.
	deepseek := byID["deepseek"]
	if !containsCap(deepseek.DeclaredOnly, CapReasoning) {
		t.Fatalf("deepseek reasoning must be declared_only, got %v", deepseek.DeclaredOnly)
	}

	// Groq: planned catalog gap => InCatalog false, Catalog empty, declared still present.
	groq := byID["groq"]
	if groq.InCatalog {
		t.Fatal("groq must report in_catalog=false (D1)")
	}
	if len(groq.Catalog) != 0 {
		t.Fatalf("groq catalog caps must be empty, got %v", groq.Catalog)
	}
	if !containsCap(groq.Declared, CapStreaming) || !containsCap(groq.Declared, CapToolCalling) {
		t.Fatalf("groq declared caps must be the conservative baseline, got %v", groq.Declared)
	}
	// With no catalog caps, every declared cap is declared_only.
	if len(groq.DeclaredOnly) != len(groq.Declared) {
		t.Fatalf("groq: all declared caps should be declared_only when catalog empty")
	}
}

// TestCapabilityCatalogIsCanonical enforces D3 end-to-end: every capability the
// facade can surface (in any field) is a member of the canonical vocabulary.
func TestCapabilityCatalogIsCanonical(t *testing.T) {
	for _, r := range CapabilityCatalog() {
		for _, field := range [][]Capability{r.Declared, r.Catalog, r.Union, r.DeclaredOnly, r.CatalogOnly} {
			for _, c := range field {
				if !IsCanonical(c) {
					t.Fatalf("provider %q surfaced non-canonical capability %q", r.Provider, c)
				}
			}
		}
	}
}

// TestCapabilityCatalogEmptySlicesNotNil ensures JSON renders [] not null for
// empty capability fields (groq's Catalog/CatalogOnly).
func TestCapabilityCatalogEmptySlicesNotNil(t *testing.T) {
	for _, r := range CapabilityCatalog() {
		if r.Declared == nil || r.Catalog == nil || r.Union == nil || r.DeclaredOnly == nil || r.CatalogOnly == nil {
			t.Fatalf("provider %q has a nil capability slice (must be empty non-nil for clean JSON)", r.Provider)
		}
	}
}

// --- F2.2 NON-CONSUMPTION / SCOPE GUARD --------------------------------------
//
// F2.2 deliberately ALLOWS the server package to consume EXACTLY ONE capability
// symbol: the read-only diagnostic facade CapabilityCatalog (rendered by the
// /api/capabilities endpoint). Everything else stays internal:
//
//   - The F2.0 vocabulary primitives and F2.1 lookup primitives must remain
//     unreferenced by the server (their original guards live in their own test
//     files and continue to enforce that — they are unaffected because the
//     server only touches the new facade, not those symbols).
//   - The routing/eligibility primitive Satisfies MUST NOT appear in the server:
//     capability-based selection/filtering is F2.4, gated by its own checkpoint.
//     This is the load-bearing tripwire that keeps F2.2 observability-only.
func TestF2_2_ServerConsumesOnlyDiagnosticFacade(t *testing.T) {
	serverDir := filepath.Join("..", "server")
	if _, err := os.Stat(serverDir); err != nil {
		t.Skipf("server package not found at %s (skipping): %v", serverDir, err)
	}
	// Forbidden in the server: the routing primitive and the lower-level
	// capability tables/derivations. NOT forbidden: CapabilityCatalog (the facade)
	// and the CapabilityInfo type it returns.
	forbidden := regexp.MustCompile(`\b(Satisfies|officialCapabilities|CapabilitiesFor|CapabilityReport|CatalogCapabilitiesFor|CatalogTagsToSet|CatalogTagToCapability)\b`)

	var offenders []string
	err := filepath.Walk(serverDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if forbidden.Match(data) {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk server package: %v", err)
	}
	if len(offenders) != 0 {
		t.Fatalf("F2.2 scope violation: server references a non-facade capability symbol (routing/eligibility is F2.4): %v", offenders)
	}
}

// --- helpers -----------------------------------------------------------------

func containsCap(caps []Capability, target Capability) bool {
	for _, c := range caps {
		if c == target {
			return true
		}
	}
	return false
}

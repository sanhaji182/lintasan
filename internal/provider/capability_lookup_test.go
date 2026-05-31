package provider

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// --- F2.1 capability lookup --------------------------------------------------

func TestCapabilitiesForKnownProviders(t *testing.T) {
	cases := map[string][]Capability{
		"openai":    {CapStreaming, CapToolCalling, CapVision, CapJSONMode, CapEmbeddings},
		"anthropic": {CapStreaming, CapToolCalling, CapVision},
		"gemini":    {CapStreaming, CapToolCalling, CapVision, CapJSONMode},
		"deepseek":  {CapStreaming, CapToolCalling, CapVision, CapJSONMode, CapReasoning},
		"groq":      {CapStreaming, CapToolCalling},
	}
	for id, want := range cases {
		caps, ok := CapabilitiesFor(id)
		if !ok {
			t.Fatalf("CapabilitiesFor(%q): expected found=true", id)
		}
		if len(caps) != len(want) {
			t.Fatalf("CapabilitiesFor(%q): got %v want %v", id, caps.List(), want)
		}
		for _, c := range want {
			if !caps.Has(c) {
				t.Fatalf("CapabilitiesFor(%q): missing %q (got %v)", id, c, caps.List())
			}
		}
	}
}

func TestCapabilitiesForUnknownFallsBackToDefault(t *testing.T) {
	caps, ok := CapabilitiesFor("totally-unknown-provider")
	if ok {
		t.Fatal("unknown provider must report found=false (fallback, not an error)")
	}
	// Fallback must equal the conservative default exactly.
	if len(caps) != 2 || !caps.Has(CapStreaming) || !caps.Has(CapToolCalling) {
		t.Fatalf("fallback should be conservative default {streaming, tool_calling}, got %v", caps.List())
	}
}

// TestLookupDefaultMatchesProvider pins the lookup's fallback to the live
// DefaultProvider declaration. If someone changes DefaultProvider.Capabilities()
// (an F2.1-forbidden behavior touch) without updating the lookup, this fails.
func TestLookupDefaultMatchesProvider(t *testing.T) {
	got := defaultDeclaredCaps()
	want := NewDefaultProvider("x").Capabilities()
	if len(got) != len(want) {
		t.Fatalf("default lookup caps drifted from DefaultProvider: got %v want %v", got.List(), want.List())
	}
	for c := range want {
		if !got.Has(c) {
			t.Fatalf("default lookup missing %q that DefaultProvider declares", c)
		}
	}
}

// TestLookupCapabilitiesAreCanonical enforces D3: every capability the lookup
// table can ever return is a member of the canonical vocabulary.
func TestLookupCapabilitiesAreCanonical(t *testing.T) {
	for id, set := range officialCapabilities {
		for c := range set {
			if !IsCanonical(c) {
				t.Fatalf("provider %q declares non-canonical capability %q", id, c)
			}
		}
	}
	for c := range defaultDeclaredCaps() {
		if !IsCanonical(c) {
			t.Fatalf("default caps include non-canonical %q", c)
		}
	}
}

func TestCapabilitiesForReturnsCopy(t *testing.T) {
	caps, _ := CapabilitiesFor("openai")
	caps[CapReasoning] = true // mutate the returned copy
	// The table must be unaffected.
	again, _ := CapabilitiesFor("openai")
	if again.Has(CapReasoning) {
		t.Fatal("CapabilitiesFor leaked a mutable reference to the lookup table")
	}
}

func TestKnownOfficialProvidersSorted(t *testing.T) {
	got := KnownOfficialProviders()
	want := []string{"anthropic", "deepseek", "gemini", "groq", "openai"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("not sorted: got %v want %v", got, want)
		}
	}
}

// --- F2.1 first registry consumption -----------------------------------------

func TestCapabilityReportJoinsRegistry(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(NewDefaultProvider("openai"))
	_ = reg.Register(NewDefaultProvider("deepseek"))
	_ = reg.Register(NewDefaultProvider("some-custom-conn")) // not in official table

	report := CapabilityReport(reg)
	if len(report) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(report), report)
	}
	// openai gets its rich declared set...
	if !report["openai"].Has(CapVision) || !report["openai"].Has(CapEmbeddings) {
		t.Fatalf("openai report missing declared caps: %v", report["openai"].List())
	}
	// deepseek gets reasoning...
	if !report["deepseek"].Has(CapReasoning) {
		t.Fatalf("deepseek report missing reasoning: %v", report["deepseek"].List())
	}
	// unknown custom conn gets the conservative default, NOT an error/empty.
	cc := report["some-custom-conn"]
	if len(cc) != 2 || !cc.Has(CapStreaming) || !cc.Has(CapToolCalling) {
		t.Fatalf("custom conn should get conservative default, got %v", cc.List())
	}
}

func TestCapabilityReportNilRegistry(t *testing.T) {
	if r := CapabilityReport(nil); len(r) != 0 {
		t.Fatalf("nil registry should yield empty report, got %v", r)
	}
}

// --- F2.1 NON-CONSUMPTION GUARD ----------------------------------------------
//
// F2.1 is the FIRST registry consumption, but consumption stays INSIDE
// internal/provider. The server package (proxy/router/handlers/selection) must
// STILL reference none of the capability-lookup symbols. Exposing them over an
// HTTP/diagnostic surface is F2.2, gated by its own checkpoint; wiring them into
// selection/eligibility is F2.4. This guard fails loudly if either happens early.
func TestF2_1_LookupNotConsumedByServer(t *testing.T) {
	serverDir := filepath.Join("..", "server")
	if _, err := os.Stat(serverDir); err != nil {
		t.Skipf("server package not found at %s (skipping): %v", serverDir, err)
	}
	forbidden := regexp.MustCompile(`\b(CapabilitiesFor|CapabilityReport|KnownOfficialProviders|officialCapabilities)\b`)

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
		t.Fatalf("F2.1 scope violation: server package references capability-lookup symbols (read-only, in-package only until F2.2/F2.4): %v", offenders)
	}
}

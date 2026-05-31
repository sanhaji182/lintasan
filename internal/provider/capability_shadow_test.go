package provider

import "testing"

// --- F2.3 RequiredCaps derivation --------------------------------------------

func TestRequestSignalsRequiredCaps(t *testing.T) {
	cases := []struct {
		name string
		sig  RequestSignals
		want []Capability
	}{
		{"empty", RequestSignals{}, nil},
		{"stream", RequestSignals{Stream: true}, []Capability{CapStreaming}},
		{"tools", RequestSignals{HasTools: true}, []Capability{CapToolCalling}},
		{"vision", RequestSignals{HasVision: true}, []Capability{CapVision}},
		{"json", RequestSignals{HasJSONMode: true}, []Capability{CapJSONMode}},
		{"all", RequestSignals{Stream: true, HasTools: true, HasVision: true, HasJSONMode: true},
			[]Capability{CapStreaming, CapToolCalling, CapVision, CapJSONMode}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.sig.requiredCaps()
			if len(got) != len(c.want) {
				t.Fatalf("size mismatch: got %v want %v", got.List(), c.want)
			}
			for _, w := range c.want {
				if !got.Has(w) {
					t.Fatalf("missing %q in %v", w, got.List())
				}
			}
		})
	}
}

// --- F2.3 ShadowEvaluate -----------------------------------------------------

func TestShadowEvaluateRecordsButDoesNotExclude(t *testing.T) {
	// A vision request against openai (declares vision) + anthropic (declares
	// vision) + groq (does NOT declare vision in the F2.1 table).
	signals := RequestSignals{HasVision: true}
	ids := []string{"openai", "anthropic", "groq"}

	res := ShadowEvaluate(signals, ids)

	if len(res.Required) != 1 || res.Required[0] != CapVision {
		t.Fatalf("required should be [vision], got %v", res.Required)
	}
	if len(res.Decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(res.Decisions))
	}
	// groq lacks vision → would-exclude; openai/anthropic satisfy.
	byID := map[string]ShadowDecision{}
	for _, d := range res.Decisions {
		byID[d.Provider] = d
	}
	if !byID["openai"].Satisfies {
		t.Error("openai should satisfy vision")
	}
	if !byID["anthropic"].Satisfies {
		t.Error("anthropic should satisfy vision")
	}
	if byID["groq"].Satisfies {
		t.Error("groq should NOT satisfy vision (conservative caps)")
	}
	if len(res.WouldExclude) != 1 || res.WouldExclude[0] != "groq" {
		t.Fatalf("would_exclude should be [groq], got %v", res.WouldExclude)
	}
}

func TestShadowEvaluateEmptyRequiredSatisfiedByAll(t *testing.T) {
	// No required caps → every candidate trivially satisfies, would_exclude empty.
	res := ShadowEvaluate(RequestSignals{}, []string{"openai", "groq", "unknown-x"})
	if len(res.WouldExclude) != 0 {
		t.Fatalf("empty required must exclude nobody, got %v", res.WouldExclude)
	}
	for _, d := range res.Decisions {
		if !d.Satisfies {
			t.Errorf("%s should satisfy empty required", d.Provider)
		}
	}
}

func TestShadowEvaluateUnknownProviderUsesDefault(t *testing.T) {
	// Unknown identity falls back to conservative default {streaming,tool_calling}.
	// A streaming request → unknown satisfies; a vision request → unknown does not.
	if res := ShadowEvaluate(RequestSignals{Stream: true}, []string{"unknown-x"}); len(res.WouldExclude) != 0 {
		t.Errorf("unknown should satisfy streaming via default, got would_exclude=%v", res.WouldExclude)
	}
	if res := ShadowEvaluate(RequestSignals{HasVision: true}, []string{"unknown-x"}); len(res.WouldExclude) != 1 {
		t.Errorf("unknown should NOT satisfy vision via default, got would_exclude=%v", res.WouldExclude)
	}
}

func TestShadowEvaluateDeterministicOrder(t *testing.T) {
	// would_exclude is sorted for stable diagnostics.
	res := ShadowEvaluate(RequestSignals{HasVision: true}, []string{"groq", "openai", "unknown-z", "unknown-a"})
	want := []string{"groq", "unknown-a", "unknown-z"}
	if len(res.WouldExclude) != len(want) {
		t.Fatalf("got %v want %v", res.WouldExclude, want)
	}
	for i := range want {
		if res.WouldExclude[i] != want[i] {
			t.Fatalf("not sorted: got %v want %v", res.WouldExclude, want)
		}
	}
}

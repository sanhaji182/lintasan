package provider

import "sort"

// Capability lookup integration (F2.1 — F2 Design Baseline, decisions D1/D2/D3).
//
// SCOPE LOCK (F2.1): this file is the FIRST consumer of the F2.0 canonical
// vocabulary. It provides a READ-ONLY lookup table mapping an official
// provider identity to the capability set it declares, plus a registry-aware
// read-only report. It is deliberately inert with respect to behavior:
//
//   - It does NOT change routing.
//   - It does NOT change provider selection.
//   - It does NOT perform eligibility filtering.
//   - It does NOT touch DefaultProvider.Capabilities() (that stays conservative,
//     so the existing contract test and runtime behavior are untouched).
//   - It is NOT consumed by the server package (asserted by an F2.1 guard test);
//     exposing it via an HTTP/diagnostic surface is F2.2, not F2.1.
//   - It does NOT join the models catalog (Format->id resolution + catalog merge
//     is F2.2's concern). The table here is self-contained declarative data.
//
// Decision linkage:
//   - D2: capabilities come from a LOOKUP TABLE keyed by provider identity, NOT
//     from per-provider adapter types. One DefaultProvider + this table.
//   - D3: every capability in the table is a member of the canonical vocabulary
//     (asserted by TestLookupCapabilitiesAreCanonical).
//   - D1: Groq is Official but its catalog entry is a planned gap, so it declares
//     only the conservative baseline here until that entry lands.

// officialCapabilities is the declared capability set for each official provider
// identity, exactly as fixed in F2 Design Baseline §2 ("Target declared caps").
// Keys are the SDK provider identities (the names a provider registers under).
//
// This is the single source of the official providers' declared capabilities.
// It is built from canonical vocabulary constants only (D3).
var officialCapabilities = map[string]CapabilitySet{
	"openai":    NewCapabilitySet(CapStreaming, CapToolCalling, CapVision, CapJSONMode, CapEmbeddings),
	"anthropic": NewCapabilitySet(CapStreaming, CapToolCalling, CapVision),
	"gemini":    NewCapabilitySet(CapStreaming, CapToolCalling, CapVision, CapJSONMode),
	"deepseek":  NewCapabilitySet(CapStreaming, CapToolCalling, CapVision, CapJSONMode, CapReasoning),
	// D1: Groq is Official with a planned catalog gap — conservative baseline only
	// until a catalog entry is added (a separate additive task, not F2.1).
	"groq": NewCapabilitySet(CapStreaming, CapToolCalling),
}

// defaultDeclaredCaps mirrors DefaultProvider.Capabilities() exactly: the
// conservative baseline used for any provider identity not in the table. Keeping
// these in lockstep is intentional — the lookup must never silently disagree
// with the live provider's own declaration. Pinned by TestLookupDefaultMatchesProvider.
func defaultDeclaredCaps() CapabilitySet {
	return NewCapabilitySet(CapStreaming, CapToolCalling)
}

// copyCapabilitySet returns a defensive copy so callers can never mutate the
// lookup table's backing sets.
func copyCapabilitySet(s CapabilitySet) CapabilitySet {
	out := make(CapabilitySet, len(s))
	for c, on := range s {
		out[c] = on
	}
	return out
}

// CapabilitiesFor returns the declared capability set for an official provider
// identity. The bool reports whether the identity was found in the official
// table: false means the caller got the conservative default fallback (this is
// NOT an error — it is the migration safety net, mirroring Registry.Resolve).
//
// READ-ONLY: the returned set is a copy; mutating it cannot affect the table.
// This function makes NO routing/selection/eligibility decision — it only
// answers "what does this identity declare it can do?".
func CapabilitiesFor(providerID string) (CapabilitySet, bool) {
	if caps, ok := officialCapabilities[providerID]; ok {
		return copyCapabilitySet(caps), true
	}
	return defaultDeclaredCaps(), false
}

// KnownOfficialProviders lists the provider identities present in the official
// capability table, in stable sorted order. For diagnostics only.
func KnownOfficialProviders() []string {
	out := make([]string, 0, len(officialCapabilities))
	for id := range officialCapabilities {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// CapabilityReport is the FIRST registry consumption (F2.1 target): given a
// registry, it returns each registered provider's declared capability set by
// joining the registry's provider names with the lookup table. Providers whose
// name is not in the official table get the conservative default.
//
// READ-ONLY and side-effect free: it enumerates Names() and reads the table. It
// makes no selection and mutates nothing. It is the in-package primitive a
// future F2.2 diagnostic endpoint would render — but F2.1 does NOT expose it
// over HTTP (that is F2.2, gated by its own checkpoint).
func CapabilityReport(reg *Registry) map[string]CapabilitySet {
	if reg == nil {
		return map[string]CapabilitySet{}
	}
	report := make(map[string]CapabilitySet)
	for _, name := range reg.Names() {
		caps, _ := CapabilitiesFor(name)
		report[name] = caps
	}
	return report
}

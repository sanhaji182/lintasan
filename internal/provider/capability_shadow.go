package provider

import "sort"

// Capability shadow evaluation (F2.3 — F2 Design Baseline, shadow/observe-only).
//
// SCOPE LOCK (F2.3): this is the FIRST capability-AWARE routing surface, but it
// is OBSERVE-ONLY. It computes, for a set of candidate provider identities,
// whether each would satisfy a request's required capabilities — and returns
// that as data. It NEVER excludes, reorders, or selects anything. Enforcement
// (dropping non-satisfying providers from the pool) is F2.4, gated separately.
//
// DESIGN: the whole evaluation (RequiredCaps derivation + Satisfies) lives HERE,
// inside the provider package. The server passes only primitive request signals
// (RequestSignals) and candidate identities, and receives a ShadowResult. This
// is deliberate: it keeps `Satisfies`, `CapabilitiesFor`, and the capability
// vocabulary constants OUT of the server package, so the F2.0/F2.1/F2.2
// non-consumption guards stay green UNCHANGED. The server's only new capability
// symbol is the ShadowEvaluate facade — exactly the F2.2 facade discipline.
//
// COARSENESS NOTE: capabilities are looked up by provider identity (Format) via
// the F2.1 table. Many live connections share Format="openai" (deepseek, groq,
// etc.), so this lookup is COARSE. That is acceptable for SHADOW because it
// gates nothing — it is observability. F2.4 enforcement will need a precise
// per-connection identity before it can safely exclude on this data.

// RequestSignals are the raw, capability-vocabulary-free facts the server
// extracts from an inbound chat request. Keeping these primitive means the
// server never needs to import capability constants — the mapping to canonical
// capabilities happens here.
type RequestSignals struct {
	Stream      bool // request asked for SSE streaming
	HasTools    bool // request carried tools/functions
	HasVision   bool // request carried image/multimodal content
	HasJSONMode bool // request asked for json_object/json_schema response_format
}

// requiredCaps maps the primitive request signals to the canonical capability
// set the request requires. Pure; the single place signals become capabilities.
func (s RequestSignals) requiredCaps() CapabilitySet {
	req := CapabilitySet{}
	if s.Stream {
		req[CapStreaming] = true
	}
	if s.HasTools {
		req[CapToolCalling] = true
	}
	if s.HasVision {
		req[CapVision] = true
	}
	if s.HasJSONMode {
		req[CapJSONMode] = true
	}
	return req
}

// ShadowDecision is one candidate provider's shadow eligibility result.
type ShadowDecision struct {
	// Provider is the candidate provider identity (Format) evaluated.
	Provider string `json:"provider"`
	// Declared are the caps the F2.1 lookup declares for this identity.
	Declared []Capability `json:"declared"`
	// Satisfies reports whether Declared covers every RequiredCap. In SHADOW
	// mode this is RECORDED ONLY — a false here does NOT exclude the candidate.
	Satisfies bool `json:"satisfies"`
}

// ShadowResult is the full observe-only evaluation for one request against its
// candidate pool. It is pure data; the caller logs/metrics it and otherwise
// ignores it (selection is unchanged).
type ShadowResult struct {
	// Required is the canonical capability set the request needs.
	Required []Capability `json:"required"`
	// Decisions is one row per candidate, in input order.
	Decisions []ShadowDecision `json:"decisions"`
	// WouldExclude lists candidate identities that do NOT satisfy Required.
	// This is the load-bearing observability signal: under F2.4 enforcement
	// these WOULD be dropped. In F2.3 they are kept; this set must be observed
	// (ideally empty or expected) under real traffic before F2.4 is greenlit.
	WouldExclude []string `json:"would_exclude"`
}

// ShadowEvaluate performs the observe-only capability evaluation. For each
// candidate provider identity it looks up the declared caps (F2.1 table) and
// records whether they satisfy the request's required caps. It NEVER mutates,
// reorders, filters, or selects — it only returns what it observed.
//
// This is the ONLY capability symbol the server references for F2.3, and it is
// read-only by construction: it takes value inputs and returns a fresh value.
func ShadowEvaluate(signals RequestSignals, providerIDs []string) ShadowResult {
	required := signals.requiredCaps()
	res := ShadowResult{
		Required:     sortedCaps(required),
		Decisions:    make([]ShadowDecision, 0, len(providerIDs)),
		WouldExclude: []string{},
	}
	for _, id := range providerIDs {
		declared, _ := CapabilitiesFor(id) // fallback-safe; never errors
		ok := declared.Satisfies(required)
		res.Decisions = append(res.Decisions, ShadowDecision{
			Provider:  id,
			Declared:  sortedCaps(declared),
			Satisfies: ok,
		})
		if !ok {
			res.WouldExclude = append(res.WouldExclude, id)
		}
	}
	sort.Strings(res.WouldExclude)
	return res
}

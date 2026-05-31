package provider

// capability_enforce.go — F2.4 capability eligibility ENFORCEMENT (act).
//
// SCOPE (F2.4): this is the FIRST capability surface that ACTS — it can drop a
// candidate from the routing pool. It is the "+ act" half of "shadow + act":
// it reuses the EXACT F2.3 re-bake evaluator (ShadowEvaluateIdentity → the
// 3-tier resolveIdentityCaps) and then derives a fail-open keep mask from the
// result. It introduces NO new capability evaluation, NO new resolver, and NO
// new identity logic — only the decision to act on what shadow already computed.
//
// WHY THIS MATTERS (R4 eliminated by construction): enforcement and shadow share
// the SAME evaluator and the SAME drop predicate. What the F2.3 bake observed as
// `WouldExclude` is byte-for-byte what enforcement removes here. There is no
// second code path to diverge.
//
// FAIL-OPEN INVARIANT (Sans, 2026-05-31): a candidate is dropped ONLY when its
// caps were resolved from real data (Tier model or provider) AND positively fail
// to satisfy the request. A candidate resolved at the conservative default tier
// (missing data) is FailOpen and is NEVER dropped — absence of data must never
// eliminate a provider.
//
// EMPTY-POOL INVARIANT (R3): enforcement NARROWS a pool; it must never produce
// "no route." If applying the keep mask would remove every candidate, the mask
// is reverted to keep-all and Aborted is set — selection then proceeds on the
// full (un-narrowed) pool exactly as if enforcement were off for that request.
//
// FACADE DISCIPLINE (keeps the F2.2 guard GREEN): the server receives only a
// []bool keep mask + scalars. It never sees Satisfies / CapabilitiesFor /
// officialCapabilities / CatalogCapabilitiesFor / CatalogTagsToSet /
// CatalogTagToCapability — all of which stay inside this package. The server
// applies a boolean filter and references zero capability symbols.

// EnforcementResult is the pure-data output of EnforceEligibility. The server
// applies Keep and logs the embedded Shadow record; it never inspects a
// capability symbol.
type EnforcementResult struct {
	// Shadow is the full observe record (identical to what F2.3 produced), so the
	// existing ShadowAggregator + per-request evidence pipeline keep working with
	// zero changes. Enforcement records the SAME ShadowResult shadow did.
	Shadow ShadowResult

	// Keep is aligned 1:1 with the input identities order. true = retain in the
	// routing pool; false = drop (data-backed disqualification only).
	Keep []bool

	// Dropped lists the identity labels actually removed (the data-backed,
	// !FailOpen, !Satisfies set). Equal to Shadow.WouldExclude unless Aborted.
	Dropped []string

	// Aborted is true when the empty-pool guard (R3) reverted the exclusion: the
	// keep mask would have emptied the pool, so EVERY candidate is kept instead.
	// When Aborted, Keep is all-true and Dropped is empty.
	Aborted bool
}

// EnforceEligibility runs the F2.3 identity evaluator and derives a fail-open
// keep mask. It is pure: value in, value out, no mutation of the inputs, no I/O.
//
//   - For each candidate, keep := Satisfies OR FailOpen (default-tier kept).
//   - A data-backed candidate that fails the request is dropped.
//   - R3 empty-pool guard: if the mask would drop ALL candidates, revert to
//     keep-all and set Aborted=true (enforcement narrows, never zeroes).
//
// The Shadow field carries the unmodified observe result for the evidence
// pipeline, so enforcing never makes observability go dark.
func EnforceEligibility(signals RequestSignals, identities []CandidateIdentity) EnforcementResult {
	shadow := ShadowEvaluateIdentity(signals, identities)

	res := EnforcementResult{
		Shadow:  shadow,
		Keep:    make([]bool, len(shadow.Decisions)),
		Dropped: []string{},
	}

	keptCount := 0
	for i, d := range shadow.Decisions {
		// Keep when the candidate satisfies the request OR was resolved fail-open
		// (default tier / missing data). Drop ONLY data-backed positive failures.
		keep := d.Satisfies || d.FailOpen
		res.Keep[i] = keep
		if keep {
			keptCount++
		} else {
			res.Dropped = append(res.Dropped, d.Label)
		}
	}

	// R3 — never zero the pool. If enforcement would remove every candidate,
	// revert to keep-all. This protects against a (latent) state where the whole
	// pool is data-backed-incompatible: availability wins over capability purity.
	if len(res.Keep) > 0 && keptCount == 0 {
		for i := range res.Keep {
			res.Keep[i] = true
		}
		res.Dropped = []string{}
		res.Aborted = true
	}

	return res
}

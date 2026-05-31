package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// capability_enforce.go — F2.4 server-side capability ENFORCEMENT hook (act).
//
// This is the server's ONLY F2.4 surface. It mirrors runCapabilityShadow's
// identity construction EXACTLY (same CandidateIdentity fields, same request
// signals) and delegates the whole decision to provider.EnforceEligibility,
// which returns a plain []bool Keep mask the server applies to the pool. The
// F2.0/F2.1/F2.2 server non-consumption guards stay GREEN — the facade returns
// booleans, not capabilities. The server holds NO capability vocabulary and
// references none of the forbidden capability-resolution symbols; all of that
// stays inside the provider package.
//
// CONTRACT:
//   - Flag OFF (default): a single bool check, returns the pool unchanged —
//     byte-identical to pre-F2.4 (and to F2.3 observe-only).
//   - Flag ON: narrows the pool to the kept subset. NEVER reorders (kept order
//     preserved from the LB/task-class ordering done before this hook). NEVER
//     empties the pool (R3: if all would drop, keeps all and logs aborted).
//   - Records the SAME ShadowResult into shadowStats the F2.3 path records, so
//     observability never goes dark when enforcing, and feeds the same evidence
//     endpoint. Enforcement implies recording.
//
// FAIL-OPEN: a candidate resolved at the conservative default tier (missing
// data) is kept regardless of cap math — absence of data never eliminates.

// applyCapabilityEnforcement narrows `candidates` to those that satisfy (or are
// fail-open for) the request's required capabilities. Returns the (possibly
// shorter) pool. When the flag is off it returns `candidates` unchanged.
//
// The pool ordering is preserved: enforcement only removes, never reorders, so
// the LB/task-class preference established before this hook is respected on the
// surviving subset.
func (p *ProxyHandler) applyCapabilityEnforcement(w http.ResponseWriter, req map[string]any, resolvedModel string, stream bool, candidates []*Connection) []*Connection {
	if !p.capabilityEnforce {
		return candidates // default OFF: zero behavior change, zero added work
	}
	if len(candidates) == 0 {
		return candidates
	}

	signals := extractRequestSignals(req, stream)

	// Build identities IDENTICALLY to runCapabilityShadow — same fields, same
	// order — so enforcement acts on exactly what shadow observed (zero drift).
	identities := make([]provider.CandidateIdentity, 0, len(candidates))
	for _, c := range candidates {
		identities = append(identities, provider.CandidateIdentity{
			Format:  c.Format,
			Model:   resolvedModel,
			BaseURL: c.BaseURL,
		})
	}

	res := provider.EnforceEligibility(signals, identities)

	// Enforcement implies recording: fold the SAME observe result into the
	// evidence aggregator so /api/capabilities/shadow keeps reporting while
	// enforcing (observability never goes dark).
	p.shadowStats.Record(res.Shadow)

	// Structured per-request line (counts only, no PII) distinguishing act vs
	// observe and surfacing the empty-pool guard.
	fmt.Fprintf(os.Stderr,
		"[capability-enforce] model=%q required=%v candidates=%d dropped=%d aborted=%t tiers=m%d/p%d/d%d\n",
		resolvedModel, res.Shadow.Required, len(res.Shadow.Decisions), len(res.Dropped), res.Aborted,
		res.Shadow.TierCounts[provider.TierModel],
		res.Shadow.TierCounts[provider.TierProvider],
		res.Shadow.TierCounts[provider.TierDefault])

	w.Header().Set("X-Lintasan-Capability-Enforce",
		fmt.Sprintf("required=%d candidates=%d dropped=%d aborted=%t",
			len(res.Shadow.Required), len(res.Shadow.Decisions), len(res.Dropped), res.Aborted))

	// R3 guard already handled in the facade: if the mask would empty the pool,
	// EnforceEligibility returns Keep all-true + Aborted. So applying the mask is
	// always safe — it never yields an empty pool.
	if res.Aborted {
		return candidates // keep-all; enforcement narrowed nothing this request
	}

	kept := make([]*Connection, 0, len(candidates))
	for i, keep := range res.Keep {
		if keep {
			kept = append(kept, candidates[i])
		}
	}
	return kept
}

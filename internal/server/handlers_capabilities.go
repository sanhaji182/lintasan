package server

import (
	"encoding/json"
	"net/http"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// Capability diagnostics endpoint (F2.2 — F2 Design Baseline, decisions D1/D2/D3).
//
// SCOPE LOCK (F2.2): this is a READ-ONLY observability surface. It renders the
// provider-package diagnostic facade (provider.CapabilityCatalog) and nothing
// else. It is the ONLY place the server consumes a capability symbol, and it
// touches exactly ONE: the facade. Specifically it does NOT:
//   - change routing or provider selection,
//   - perform eligibility filtering (capability-based routing is a later phase),
//   - read or write any setting/flag, DB row, or request-path state,
//   - mutate anything — it serializes a deterministic in-memory snapshot.
//
// The join logic (declared caps vs catalog-tagged caps, vocabulary
// reconciliation, the Groq D1 gap) lives entirely in internal/provider; this
// handler is a thin JSON renderer so the server stays free of capability logic.

// handleCapabilities serves GET /api/capabilities — a read-only diagnostic view
// of each official provider's declared vs catalog-derived capabilities, plus the
// union and the two "unregistered" diffs that the F2 audit surfaced. Pure
// observability; no behavior change.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	rows := provider.CapabilityCatalog()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"data":  rows,
		"count": len(rows),
		// Static metadata so consumers know this is observability, not routing.
		"note": "Read-only capability diagnostics. Declared = SDK lookup; catalog = models-catalog tags (canonicalized). Routing does not consume capabilities (deferred to a later phase).",
	})
}

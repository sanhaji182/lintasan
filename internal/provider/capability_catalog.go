package provider

import (
	"sort"

	"github.com/sanhaji182/lintasan-go/internal/models"
)

// Capability catalog integration (F2.2 — F2 Design Baseline, decisions D1/D2/D3).
//
// SCOPE LOCK (F2.2): this file is the catalog JOIN + the single read-only
// DIAGNOSTIC FACADE that an /api/capabilities endpoint renders. It is the only
// capability symbol the server package is permitted to consume (see the F2.2
// guard test). It is strictly observability:
//
//   - It does NOT change routing.
//   - It does NOT change provider selection.
//   - It does NOT perform eligibility filtering (it never calls Satisfies).
//   - It does NOT touch DefaultProvider, the request path, or any flag.
//   - It introduces NO schema change; it reads the static models catalog and the
//     F2.1 lookup table, both of which are themselves read-only.
//
// What it ADDS over F2.1: F2.1 fixed the *declared* caps per official provider
// identity. F2.2 JOINS those declared caps with the caps the models catalog
// actually tags (mapped through the F2.0 canonical vocabulary), so the two
// historical vocabularies (SDK declaration vs catalog tags) become observable
// side-by-side. This is what makes the long-standing audit findings
// ("EXISTS-UNREGISTERED", "vocab mismatch") visible as live data instead of a
// one-off report.
//
// Decision linkage:
//   - D2: still one DefaultProvider + lookup table — the join reads the table,
//     it does NOT introduce per-provider adapter types.
//   - D3: every capability surfaced here is a canonical-vocabulary member
//     (declared caps come from F2.1 which is D3-pinned; catalog caps come from
//     CatalogTagsToSet which drops anything non-canonical).
//   - D1: Groq is Official but has no catalog entry yet, so its Catalog set is
//     empty and InCatalog is false — surfaced honestly, not hidden.

// sdkToCatalogProviderID maps an SDK official-provider identity (the keys of the
// F2.1 officialCapabilities table) to the corresponding models-catalog provider
// ID. The two vocabularies disagree on one name: the SDK calls Google's provider
// "gemini" while the catalog calls it "google". Groq has no catalog entry yet
// (D1), so it is deliberately absent here. Pure; no runtime caller on any path.
var sdkToCatalogProviderID = map[string]string{
	"openai":    "openai",
	"anthropic": "anthropic",
	"gemini":    "google", // RECONCILIATION: SDK "gemini" == catalog "google".
	"deepseek":  "deepseek",
	// "groq" intentionally omitted — planned catalog gap (D1).
}

// CatalogProviderIDFor returns the models-catalog provider ID for an SDK
// official-provider identity. The bool is false when the identity has no catalog
// counterpart (e.g. Groq's planned gap, D1). Read-only, pure.
func CatalogProviderIDFor(sdkID string) (string, bool) {
	id, ok := sdkToCatalogProviderID[sdkID]
	return id, ok
}

// CatalogCapabilitiesFor returns the canonical capability set DERIVED from the
// models catalog for an SDK official-provider identity. It unions the catalog
// tags of every model that provider lists, mapping each tag through the F2.0
// canonical vocabulary (CatalogTagsToSet drops baseline "chat" and unknowns).
//
// The bool reports whether the provider was found in the catalog at all. A false
// here is the honest "planned catalog gap" signal (D1), NOT an error.
//
// READ-ONLY: returns a fresh set; the catalog and the vocabulary are untouched.
// Makes no routing/selection/eligibility decision.
func CatalogCapabilitiesFor(sdkID string) (CapabilitySet, bool) {
	catalogID, ok := CatalogProviderIDFor(sdkID)
	if !ok {
		return CapabilitySet{}, false
	}
	pinfo := models.FindProvider(catalogID)
	if pinfo == nil {
		return CapabilitySet{}, false
	}
	union := CapabilitySet{}
	for _, m := range pinfo.Models {
		for cap := range CatalogTagsToSet(m.Capabilities) {
			union[cap] = true
		}
	}
	return union, true
}

// CapabilityInfo is one diagnostic row: how a single official provider's
// capabilities look across the two reconciled vocabularies, plus the derived
// union/diff. Pure data; safe to serialize for an observability endpoint.
type CapabilityInfo struct {
	// Provider is the SDK official-provider identity (lookup-table key).
	Provider string `json:"provider"`
	// CatalogProviderID is the models-catalog ID this maps to ("" when absent).
	CatalogProviderID string `json:"catalog_provider_id"`
	// InCatalog is false for a planned catalog gap (e.g. Groq, D1).
	InCatalog bool `json:"in_catalog"`
	// Declared are the caps the F2.1 lookup table declares for this provider.
	Declared []Capability `json:"declared"`
	// Catalog are the caps DERIVED from the models catalog tags (canonicalized).
	Catalog []Capability `json:"catalog"`
	// Union is Declared ∪ Catalog (what the system knows this provider can do).
	Union []Capability `json:"union"`
	// DeclaredOnly are caps declared by the SDK but NOT evidenced in the catalog
	// (e.g. embeddings/reasoning that are real but untagged) — the
	// "EXISTS-UNREGISTERED in catalog" audit signal.
	DeclaredOnly []Capability `json:"declared_only"`
	// CatalogOnly are caps the catalog tags but the SDK does NOT declare — the
	// "EXISTS-UNREGISTERED in SDK" audit signal.
	CatalogOnly []Capability `json:"catalog_only"`
}

// CapabilityCatalog is THE F2.2 read-only diagnostic facade. It enumerates the
// known official providers (the stable F2.1 set, independent of runtime registry
// state) and, for each, joins the declared caps with the catalog-derived caps,
// returning a deterministic, sorted snapshot.
//
// This is the single function the /api/capabilities endpoint renders. It is the
// ONLY capability symbol the server package is allowed to reference (enforced by
// the F2.2 server-consumption guard). It performs ZERO routing/selection and
// NEVER calls Satisfies — capability-based routing stays deferred to F2.4.
func CapabilityCatalog() []CapabilityInfo {
	ids := KnownOfficialProviders() // already sorted
	out := make([]CapabilityInfo, 0, len(ids))
	for _, id := range ids {
		declared, _ := CapabilitiesFor(id)
		catalogCaps, inCatalog := CatalogCapabilitiesFor(id)
		catalogID, _ := CatalogProviderIDFor(id)

		union := CapabilitySet{}
		for c := range declared {
			union[c] = true
		}
		for c := range catalogCaps {
			union[c] = true
		}

		declaredOnly := CapabilitySet{}
		for c := range declared {
			if !catalogCaps[c] {
				declaredOnly[c] = true
			}
		}
		catalogOnly := CapabilitySet{}
		for c := range catalogCaps {
			if !declared[c] {
				catalogOnly[c] = true
			}
		}

		out = append(out, CapabilityInfo{
			Provider:          id,
			CatalogProviderID: catalogID,
			InCatalog:         inCatalog,
			Declared:          sortedCaps(declared),
			Catalog:           sortedCaps(catalogCaps),
			Union:             sortedCaps(union),
			DeclaredOnly:      sortedCaps(declaredOnly),
			CatalogOnly:       sortedCaps(catalogOnly),
		})
	}
	return out
}

// sortedCaps returns the set's members as a stable, sorted slice. A non-nil
// empty slice is returned for an empty set so JSON renders [] not null.
func sortedCaps(s CapabilitySet) []Capability {
	out := make([]Capability, 0, len(s))
	for c, on := range s {
		if on {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

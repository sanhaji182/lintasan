package provider

// Capability declares a functional or tier attribute of a provider/model.
//
// FOUNDATION NOTE: this is the DECLARATION surface only. Capability-based
// routing (filtering eligible providers by required capabilities) is a later,
// separately-approved step. Providers may declare their capabilities now so the
// contract is stable; the router does not consume them in this commit.
type Capability string

const (
	// Functional capabilities.
	CapCoding      Capability = "coding"
	CapReasoning   Capability = "reasoning"
	CapToolCalling Capability = "tool_calling"
	CapStreaming   Capability = "streaming"
	CapVision      Capability = "vision"
	CapEmbeddings  Capability = "embeddings"
	CapLongContext Capability = "long_context"

	// Tier hints — an orthogonal axis to functional capabilities.
	CapCheap   Capability = "cheap"
	CapFast    Capability = "fast"
	CapPremium Capability = "premium"
)

// CapabilitySet is a set of capabilities a provider declares it supports.
type CapabilitySet map[Capability]bool

// NewCapabilitySet builds a set from the given capabilities.
func NewCapabilitySet(caps ...Capability) CapabilitySet {
	s := make(CapabilitySet, len(caps))
	for _, c := range caps {
		s[c] = true
	}
	return s
}

// Has reports whether the set contains c.
func (s CapabilitySet) Has(c Capability) bool { return s[c] }

// Satisfies reports whether this set covers every capability required by
// required. It is the primitive a future capability-based router would use to
// filter eligible providers; it is included now so the contract is stable, but
// it is not called from any runtime path in this commit.
func (s CapabilitySet) Satisfies(required CapabilitySet) bool {
	for c, need := range required {
		if need && !s[c] {
			return false
		}
	}
	return true
}

// List returns the capabilities in the set in a stable (sorted) order, for
// diagnostics and dashboard display.
func (s CapabilitySet) List() []Capability {
	out := make([]Capability, 0, len(s))
	for c, on := range s {
		if on {
			out = append(out, c)
		}
	}
	// simple insertion sort to avoid importing sort for a tiny slice
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

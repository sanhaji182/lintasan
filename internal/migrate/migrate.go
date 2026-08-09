// Package migrate turns a competitor router's export file into a Lintasan
// import plan.
//
// WHY THIS PACKAGE EXISTS: users coming from 9router / OmniRouter otherwise have
// to re-enter every provider connection by hand. Their export file already
// carries the whole setup — the job is translating it, and being honest about
// the parts that cannot come along.
//
// DESIGN CONSTRAINT — this package is PURE. It performs no I/O, touches no
// database, and never writes the uploaded bytes anywhere. It takes a []byte and
// returns a Plan. That keeps the risky part (a file full of plaintext API keys)
// confined to memory, and makes the classification logic trivially testable
// against a real captured export.
//
// The caller decides what to do with the Plan: render it as a preview, or apply
// it. Preview and import therefore cannot disagree — they are the same
// computation.
package migrate

import (
	"errors"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Classification
// ---------------------------------------------------------------------------

// Portability says whether a connection can be represented in Lintasan at all.
//
// The distinction is not cosmetic: it is the difference between "we can move
// this" and "no amount of importing will make this work". Conflating the two is
// what makes a migration tool feel broken.
type Portability string

const (
	// PortableDirect: the export carries the endpoint itself. Nothing to look up.
	PortableDirect Portability = "direct"

	// PortableViaPreset: the source router knew the endpoint from a built-in
	// adapter and did not write it into the export, but Lintasan has a preset
	// for that provider, so the endpoint is recoverable.
	PortableViaPreset Portability = "preset"

	// NotPortableOAuth: the provider authenticates via an OAuth/device flow with
	// refresh tokens and session state. Knowing the URL is not enough — it needs
	// a dedicated adapter. Explicitly out of scope; reported, never silently
	// dropped.
	NotPortableOAuth Portability = "oauth"

	// NotPortableUnknown: a built-in provider Lintasan has no preset for. The
	// user can add it by hand once they know the base URL.
	NotPortableUnknown Portability = "unknown_endpoint"
)

// Health reflects the connection's state in the SOURCE router.
//
// This matters more than it looks. A real export contained 920 dead keys out of
// 973 connections; importing them wholesale would greet a new user with a wall
// of red and read as "Lintasan is broken" when the breakage was inherited.
type Health string

const (
	HealthOK       Health = "healthy"
	HealthDead     Health = "dead"     // fatal auth/quota error at the source
	HealthInactive Health = "inactive" // explicitly disabled by the user
)

// ---------------------------------------------------------------------------
// Plan
// ---------------------------------------------------------------------------

// Connection is one provider connection, already mapped onto Lintasan's schema.
//
// APIKey is populated because an import that forces the user to re-enter every
// key has not really saved them anything. It is never logged and never rendered
// in a preview.
type Connection struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"-"` // never serialized to the client
	Format   string `json:"format"`
	Priority int    `json:"priority"`
	IsActive bool   `json:"is_active"`

	// Provenance — why this row ended up in this bucket. Shown to the user so
	// the outcome is explainable rather than magic.
	SourceProvider string      `json:"source_provider"`
	SourceID       string      `json:"source_id"`
	Portability    Portability `json:"portability"`
	Health         Health      `json:"health"`
	Reason         string      `json:"reason,omitempty"`

	// HasKey lets the UI show "key included" without exposing the key.
	HasKey bool `json:"has_key"`

	// Prefix is the short alias the source router used for this provider
	// ("sumo", "qd", …). Combo members are written as "<prefix>/<model>", so this
	// is what lets us tell whether a combo entry points at something importable.
	Prefix string `json:"prefix,omitempty"`
}

// SourcePrefix returns the source router's short alias for this connection.
func (c Connection) SourcePrefix() string { return c.Prefix }

// Combo is a model alias group.
//
// SkippedModels is the interesting field. Most real combos mix portable and
// non-portable members; importing them verbatim would leave dangling references
// that only fail at request time. We import what works and say what didn't.
type Combo struct {
	Name          string   `json:"name"`
	Models        []string `json:"models"`
	SkippedModels []string `json:"skipped_models,omitempty"`
	Partial       bool     `json:"partial"`
}

// Plan is the whole outcome of parsing an export: what can be imported, what
// cannot, and why. Rendering it is a preview; applying it is an import.
type Plan struct {
	Source string `json:"source"` // e.g. "9router"

	// Importable connections, already split by health so the caller can offer
	// "include dead ones too" without recomputing anything.
	Healthy []Connection `json:"healthy"`
	Unusable []Connection `json:"unusable"`

	// Connections that cannot be represented at all, kept for honest reporting.
	Blocked []Connection `json:"blocked"`

	Combos []Combo `json:"combos"`

	// Warnings carries anything the user should know that isn't per-row.
	Warnings []string `json:"warnings,omitempty"`
}

// Summary is a compact count-only view, safe to log.
type Summary struct {
	Healthy         int            `json:"healthy"`
	Unusable        int            `json:"unusable"`
	Blocked         int            `json:"blocked"`
	BlockedByReason map[string]int `json:"blocked_by_reason"`
	Combos          int            `json:"combos"`
	CombosPartial   int            `json:"combos_partial"`
}

// Summarize reduces a Plan to counts. No credentials, no endpoints — safe to
// write to logs or metrics.
func (p Plan) Summarize() Summary {
	s := Summary{
		Healthy:         len(p.Healthy),
		Unusable:        len(p.Unusable),
		Blocked:         len(p.Blocked),
		BlockedByReason: map[string]int{},
		Combos:          len(p.Combos),
	}
	for _, c := range p.Blocked {
		s.BlockedByReason[string(c.Portability)]++
	}
	for _, c := range p.Combos {
		if c.Partial {
			s.CombosPartial++
		}
	}
	return s
}

// Selected returns the connections to actually write, honoring the caller's
// choice about dead rows.
//
// Blocked connections are never returned: they are not importable by
// definition, and handing the user a row that cannot work is worse than telling
// them it can't.
func (p Plan) Selected(includeUnusable bool) []Connection {
	out := make([]Connection, 0, len(p.Healthy)+len(p.Unusable))
	out = append(out, p.Healthy...)
	if includeUnusable {
		out = append(out, p.Unusable...)
	}
	return out
}

// ---------------------------------------------------------------------------
// Source plugin
// ---------------------------------------------------------------------------

// PresetLookup resolves a source-router provider name to a Lintasan base URL.
//
// It is an interface rather than a map so the server can back it with the
// provider_presets table without this package importing the database.
type PresetLookup interface {
	// BaseURLFor returns the endpoint for a provider name, or ok=false when
	// Lintasan has no preset for it.
	BaseURLFor(provider string) (baseURL string, ok bool)
}

// Source parses one competitor's export format.
//
// Adding OmniRouter later means implementing this interface — the HTTP handlers,
// the UI, and the write path all consume Plan and need no changes.
type Source interface {
	// Name identifies the source router, e.g. "9router".
	Name() string

	// Detect reports whether these bytes look like this source's export. It must
	// be cheap and must not panic on arbitrary input.
	Detect(data []byte) bool

	// Parse converts the export into a Plan. presets may be nil, in which case
	// built-in providers are reported as NotPortableUnknown.
	Parse(data []byte, presets PresetLookup) (Plan, error)
}

// ErrUnrecognized is returned by Detect-based dispatch when no registered source
// claims the file.
var ErrUnrecognized = errors.New("migrate: unrecognized export format")

// registry holds the known sources in priority order.
var registry []Source

// Register adds a Source to the dispatch registry. Intended for init().
func Register(s Source) { registry = append(registry, s) }

// Registered returns the names of all registered sources.
func Registered() []string {
	names := make([]string, 0, len(registry))
	for _, s := range registry {
		names = append(names, s.Name())
	}
	return names
}

// Detect finds the source that recognizes these bytes.
func Detect(data []byte) (Source, error) {
	for _, s := range registry {
		if s.Detect(data) {
			return s, nil
		}
	}
	return nil, ErrUnrecognized
}

// Parse detects the format and parses it in one step.
func Parse(data []byte, presets PresetLookup) (Plan, error) {
	s, err := Detect(data)
	if err != nil {
		return Plan{}, err
	}
	p, err := s.Parse(data, presets)
	if err != nil {
		return Plan{}, fmt.Errorf("migrate: parsing %s export: %w", s.Name(), err)
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// mapPresets is a PresetLookup backed by a plain map, for tests and defaults.
type mapPresets map[string]string

func (m mapPresets) BaseURLFor(provider string) (string, bool) {
	if v, ok := m[strings.ToLower(strings.TrimSpace(provider))]; ok {
		return v, true
	}
	return "", false
}

// MapPresets builds a PresetLookup from a provider→baseURL map.
func MapPresets(m map[string]string) PresetLookup {
	lower := make(mapPresets, len(m))
	for k, v := range m {
		lower[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return lower
}

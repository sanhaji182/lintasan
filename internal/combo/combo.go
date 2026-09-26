package combo

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Strategy determines how entries are ordered during resolution.
type Strategy string

const (
	StrategyPriority   Strategy = "priority"
	StrategyRoundRobin Strategy = "round-robin"
)

// Entry describes one model + connections + API keys block inside a combo.
type Entry struct {
	Model         string   `json:"model"`
	ConnectionID  string   `json:"connection_id,omitempty"`
	ProviderID    string   `json:"provider_id,omitempty"`
	ConnectionIDs []string `json:"connection_ids,omitempty"`
	APIKeys       []string `json:"api_keys,omitempty"`
}

// Combo is a named combination of model entries with a strategy.
type Combo struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	Strategy    Strategy `json:"strategy"`
	StickyLimit int      `json:"sticky_limit,omitempty"`
	Entries     []Entry  `json:"entries"`
	Models      []string `json:"models,omitempty"`
	Description string   `json:"description,omitempty"`
}

// StickyState tracks success/failure counts for the priority strategy.
type StickyState struct {
	LastIndex    int // current sticky entry index
	SuccessCount int // consecutive successes
	FailCount    int // consecutive failures at current index
}

// ResolvedEntry is a single candidate returned by Resolve.
type ResolvedEntry struct {
	Model        string
	ConnectionID string
	ProviderID   string
	APIKey       string
}

// Engine holds combo definitions and runtime state.
type Engine struct {
	mu      sync.RWMutex
	combos  map[string]*Combo       // name → combo
	sticky  map[string]*StickyState // comboName → sticky state
	rrCount map[string]int          // comboName → next round-robin start index
	keyIdx  map[string]int          // "comboName:entryIdx" → next API key index
}

// New creates a ready-to-use Engine.
func New() *Engine {
	return &Engine{
		combos:  make(map[string]*Combo),
		sticky:  make(map[string]*StickyState),
		rrCount: make(map[string]int),
		keyIdx:  make(map[string]int),
	}
}

// LoadFromSettings parses a JSON array of Combo definitions and replaces the
// in-memory combos. It also resets all sticky/round-robin state.
func (e *Engine) LoadFromSettings(jsonStr string) error {
	if jsonStr == "" {
		e.mu.Lock()
		e.combos = make(map[string]*Combo)
		e.sticky = make(map[string]*StickyState)
		e.rrCount = make(map[string]int)
		e.keyIdx = make(map[string]int)
		e.mu.Unlock()
		return nil
	}

	var combos []Combo
	if err := json.Unmarshal([]byte(jsonStr), &combos); err != nil {
		return fmt.Errorf("combo: invalid settings JSON: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.combos = make(map[string]*Combo, len(combos))
	e.sticky = make(map[string]*StickyState)
	e.rrCount = make(map[string]int)
	e.keyIdx = make(map[string]int)

	for i := range combos {
		c := combos[i]
		if c.Name == "" {
			continue
		}
		// If Entries is empty but Models has items, synthesize Entries for backward compatibility
		if len(c.Entries) == 0 && len(c.Models) > 0 {
			for _, m := range c.Models {
				c.Entries = append(c.Entries, Entry{Model: m})
			}
		}
		e.combos[c.Name] = &c
		if c.Strategy == StrategyPriority {
			e.sticky[c.Name] = &StickyState{}
		}
		if c.Strategy == StrategyRoundRobin {
			e.rrCount[c.Name] = 0
		}
	}
	return nil
}

// Resolve returns the ordered list of (model, connectionID, apiKey) candidates
// for the named combo. Priority mode orders the sticky entry first; round-robin
// rotates on each call. Multi-account API keys rotate on each resolve.
func (e *Engine) Resolve(name string) ([]ResolvedEntry, error) {
	e.mu.RLock()
	combo, ok := e.combos[name]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("combo: unknown combo %q", name)
	}
	if len(combo.Entries) == 0 {
		return nil, fmt.Errorf("combo: combo %q has no entries", name)
	}

	var ordered []Entry

	switch combo.Strategy {
	case StrategyPriority:
		ordered = e.orderPriority(combo)
	case StrategyRoundRobin:
		ordered = e.orderRoundRobin(combo)
	default:
		// Default to round-robin for unknown strategies
		ordered = e.orderRoundRobin(combo)
	}

	// Flatten entries → ResolvedEntry with key rotation.
	var result []ResolvedEntry
	for entryIdx, entry := range ordered {
		k := e.rotateKey(name, entryIdx, entry)
		connectionIDs := entry.ConnectionIDs
		if entry.ConnectionID != "" {
			connectionIDs = []string{entry.ConnectionID}
		}
		if entry.ProviderID != "" {
			result = append(result, ResolvedEntry{
				Model: entry.Model, ProviderID: entry.ProviderID, APIKey: k,
			})
			continue
		}
		for _, cid := range connectionIDs {
			if cid != "" {
				result = append(result, ResolvedEntry{
					Model:        entry.Model,
					ConnectionID: cid,
					APIKey:       k,
				})
			}
		}
		// If entry has no specific connection IDs, emit candidate with empty ConnectionID
		// so resolver can match any active connection serving this model.
		if len(connectionIDs) == 0 {
			result = append(result, ResolvedEntry{
				Model:  entry.Model,
				APIKey: k,
			})
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("combo: no candidates resolved for combo %q", name)
	}
	return result, nil
}

// orderPriority returns entries with the sticky index first, then the rest.
func (e *Engine) orderPriority(c *Combo) []Entry {
	e.mu.RLock()
	state := e.sticky[c.Name]
	e.mu.RUnlock()

	idx := 0
	if state != nil {
		idx = state.LastIndex
		if idx < 0 || idx >= len(c.Entries) {
			idx = 0
		}
	}

	ordered := make([]Entry, 0, len(c.Entries))
	ordered = append(ordered, c.Entries[idx])
	for i, entry := range c.Entries {
		if i != idx {
			ordered = append(ordered, entry)
		}
	}
	return ordered
}

// orderRoundRobin returns entries rotated so the next entry is first.
func (e *Engine) orderRoundRobin(c *Combo) []Entry {
	e.mu.Lock()
	idx := e.rrCount[c.Name]
	e.rrCount[c.Name] = (idx + 1) % len(c.Entries)
	e.mu.Unlock()

	if idx >= len(c.Entries) {
		idx = 0
	}

	ordered := make([]Entry, 0, len(c.Entries))
	ordered = append(ordered, c.Entries[idx:]...)
	ordered = append(ordered, c.Entries[:idx]...)
	return ordered
}

// rotateKey returns the current API key for an entry and advances the rotation.
func (e *Engine) rotateKey(comboName string, entryIdx int, entry Entry) string {
	if len(entry.APIKeys) == 0 {
		return ""
	}

	key := fmt.Sprintf("%s:%d", comboName, entryIdx)

	e.mu.Lock()
	idx := e.keyIdx[key]
	e.keyIdx[key] = (idx + 1) % len(entry.APIKeys)
	e.mu.Unlock()

	if idx >= len(entry.APIKeys) {
		idx = 0
	}
	return entry.APIKeys[idx]
}

// RecordSuccess records a successful resolution for the combo, resetting
// the failure counter (priority strategy only).
func (e *Engine) RecordSuccess(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	state, ok := e.sticky[name]
	if !ok {
		return
	}
	state.FailCount = 0
	state.SuccessCount++
}

// RecordFailure records a failed resolution for the combo. In priority mode,
// if consecutive failures exceed the sticky limit, the sticky index advances
// to the next entry and counters reset.
func (e *Engine) RecordFailure(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	state, ok := e.sticky[name]
	if !ok {
		return
	}

	combo, ok := e.combos[name]
	if !ok || len(combo.Entries) == 0 {
		return
	}

	state.FailCount++
	state.SuccessCount = 0

	limit := combo.StickyLimit
	if limit <= 0 {
		limit = 1 // default: advance after 1 failure if stickyLimit not set
	}

	if state.FailCount > limit {
		state.LastIndex = (state.LastIndex + 1) % len(combo.Entries)
		state.FailCount = 0
	}
}

// List returns a copy of all loaded combo definitions.
func (e *Engine) List() []Combo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]Combo, 0, len(e.combos))
	for _, c := range e.combos {
		out = append(out, *c)
	}
	return out
}

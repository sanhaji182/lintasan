package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// builtinPresets re-runs the seeding handler's catalogue definition by calling
// the handler against a nil database, which returns before touching storage.
// The catalogue itself is what we want to assert on, so we read it from the
// same source the handler uses rather than duplicating it here.
func builtinPresetCatalogue(t *testing.T) []Preset {
	t.Helper()
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/presets/seed", nil)
	s.handleSeedBuiltinPresets(rec, req)
	if rec.Code != 503 {
		t.Fatalf("expected the nil-db guard to short-circuit with 503, got %d", rec.Code)
	}
	return seedCatalogue()
}

// Every preset must carry the fields a connection needs to be usable. A preset
// missing a base URL or name produces a connection that cannot route.
func TestBuiltinPresetsAreComplete(t *testing.T) {
	for _, p := range builtinPresetCatalogue(t) {
		if strings.TrimSpace(p.Name) == "" {
			t.Errorf("preset with base URL %q has no name", p.BaseURL)
		}
		if strings.TrimSpace(p.BaseURL) == "" {
			t.Errorf("preset %q has no base URL", p.Name)
		}
		if strings.TrimSpace(p.Format) == "" {
			t.Errorf("preset %q has no format", p.Name)
		}
		if !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
			t.Errorf("preset %q base URL %q is not an http(s) URL", p.Name, p.BaseURL)
		}
	}
}

// Seeding inserts by name, so two presets sharing a name would leave one of
// them permanently unreachable. Two sharing a base URL are a redundant pair the
// user has to disambiguate by hand.
func TestBuiltinPresetsHaveNoDuplicates(t *testing.T) {
	byName := map[string]string{}
	byURL := map[string]string{}
	for _, p := range builtinPresetCatalogue(t) {
		name := strings.ToLower(strings.TrimSpace(p.Name))
		if prev, dup := byName[name]; dup {
			t.Errorf("duplicate preset name %q (%s and %s)", p.Name, prev, p.BaseURL)
		}
		byName[name] = p.BaseURL

		url := strings.ToLower(strings.TrimRight(strings.TrimSpace(p.BaseURL), "/"))
		if prev, dup := byURL[url]; dup {
			t.Errorf("duplicate base URL %q (%q and %q)", p.BaseURL, prev, p.Name)
		}
		byURL[url] = p.Name
	}
}

// The importer resolves a competitor's provider id against these presets, and
// that lookup is what restores an endpoint the export omitted. If the aliases a
// preset answers to ever stop covering the spellings a competitor uses, the
// import silently falls back to blocking the connection.
func TestBuiltinPresetsResolveCompetitorProviderNames(t *testing.T) {
	catalogue := builtinPresetCatalogue(t)
	index := map[string]string{}
	for _, p := range catalogue {
		for _, alias := range presetAliases(p.Name, p.Domain) {
			if _, exists := index[alias]; !exists {
				index[alias] = p.BaseURL
			}
		}
	}

	// Provider ids as a 9router export spells them, with the endpoint each must
	// resolve to. These are the ids that appeared in a real export.
	cases := map[string]string{
		"xiaomi-mimo": "https://api.xiaomimimo.com/v1",
		"nvidia":      "https://integrate.api.nvidia.com/v1",
		"qoder":       "https://api.qoder.com/v1",
		"cerebras":    "https://api.cerebras.ai/v1",
		"deepseek":    "https://api.deepseek.com/v1",
		"openrouter":  "https://openrouter.ai/api/v1",
		"sambanova":   "https://api.sambanova.ai/v1",
		"chutes":      "https://llm.chutes.ai/v1",
		"venice":      "https://api.venice.ai/api/v1",
	}
	for providerID, wantURL := range cases {
		got, ok := index[providerID]
		if !ok {
			t.Errorf("provider id %q resolves to no preset; an import would block it as unknown_endpoint", providerID)
			continue
		}
		if got != wantURL {
			t.Errorf("provider id %q resolved to %q, want %q", providerID, got, wantURL)
		}
	}
}

// A preset whose base URL already ends in the version segment must not have the
// version appended again when the importer derives its API paths.
func TestBuiltinPresetPathsNeverDoubleVersion(t *testing.T) {
	for _, p := range builtinPresetCatalogue(t) {
		chat, models := apiPathsFor(p.BaseURL)
		base := strings.TrimRight(p.BaseURL, "/")
		for _, joined := range []string{base + chat, base + models} {
			if strings.Contains(joined, "/v1/v1") {
				t.Errorf("preset %q yields doubled version segment: %s", p.Name, joined)
			}
		}
	}
}

package migrate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ninerouter_test.go — tests driven by a REAL captured 9router export.
//
// testdata/9router_export.json is an actual export file with its secrets
// replaced by placeholders and the 922 near-identical xiaomi-mimo rows trimmed
// to 10 representatives. Everything else is verbatim.
//
// This matters: the previous milestone on this codebase (Codex /v1/responses)
// shipped four green milestones against hand-written fixtures and was still
// 100% broken against a real client, because the hand-written payloads encoded
// our ASSUMPTIONS about the format rather than the format. Tests here run
// against bytes a real 9router actually produced.

const fixture = "testdata/9router_export.json"

func loadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// presets mirrors what Lintasan's provider_presets table offers, including the
// four rows this feature adds.
func testPresets() PresetLookup {
	return MapPresets(map[string]string{
		"deepseek":     "https://api.deepseek.com/v1",
		"openrouter":   "https://openrouter.ai/api/v1",
		"xai":          "https://api.x.ai/v1",
		"ollama":       "http://localhost:11434/v1",
		"commandcode":  "https://api.commandcode.ai",
		"xiaomi-mimo":  "https://api.xiaomimimo.com/v1",
		"poolside":     "https://inference.poolside.ai/v1",
		"kilo-gateway": "https://api.kilo.ai/api/gateway",
		"nvidia":       "https://integrate.api.nvidia.com/v1",
	})
}

// --- detection -------------------------------------------------------------

func TestDetectRecognizesRealExport(t *testing.T) {
	s, err := Detect(loadFixture(t))
	if err != nil {
		t.Fatalf("real 9router export not recognized: %v", err)
	}
	if s.Name() != "9router" {
		t.Errorf("Name() = %q, want 9router", s.Name())
	}
}

func TestDetectRejectsUnrelatedInput(t *testing.T) {
	for name, data := range map[string]string{
		"empty":       ``,
		"not json":    `hello world`,
		"json array":  `[1,2,3]`,
		"other tool":  `{"version":1,"servers":[{"url":"x"}]}`,
		"truncated":   `{"providerConnections":`,
		"null fields": `{"providerConnections":null,"providerNodes":null}`,
	} {
		if _, err := Detect([]byte(data)); err == nil {
			t.Errorf("%s: expected rejection, got a match", name)
		}
	}
}

// --- the core mapping ------------------------------------------------------

// TestCustomConnectionsCarryTheirOwnBaseURL locks the key structural finding:
// connections to custom openai-compatible endpoints embed baseUrl in
// providerSpecificData, so no join against providerNodes is required.
func TestCustomConnectionsCarryTheirOwnBaseURL(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var direct int
	for _, c := range append(append([]Connection{}, plan.Healthy...), plan.Unusable...) {
		if c.Portability != PortableDirect {
			continue
		}
		direct++
		if c.BaseURL == "" {
			t.Errorf("direct connection %q has empty base_url", c.Name)
		}
		if !strings.HasPrefix(c.BaseURL, "http") {
			t.Errorf("connection %q: base_url %q is not a URL", c.Name, c.BaseURL)
		}
	}
	if direct == 0 {
		t.Fatal("no direct connections found; the fixture contains 16")
	}
}

// TestOAuthIsBlockedNotImported is the scope boundary made executable. OAuth
// providers must never land in an importable bucket, however healthy they look
// in the source.
func TestOAuthIsBlockedNotImported(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, c := range plan.Selected(true) { // even with "include dead"
		if c.Portability == NotPortableOAuth {
			t.Errorf("OAuth connection %q (%s) leaked into the import set",
				c.Name, c.SourceProvider)
		}
	}
	var blockedOAuth int
	for _, c := range plan.Blocked {
		if c.Portability == NotPortableOAuth {
			blockedOAuth++
			if c.Reason == "" {
				t.Errorf("blocked OAuth %q has no reason; the user must be told why", c.Name)
			}
		}
	}
	if blockedOAuth == 0 {
		t.Error("expected OAuth connections to be reported as blocked")
	}
}

// TestPresetRecoversBuiltinEndpoints: built-in providers carry no URL in the
// export; a preset must supply it, and without a preset the row must be blocked
// rather than imported with an empty endpoint.
func TestPresetRecoversBuiltinEndpoints(t *testing.T) {
	data := loadFixture(t)

	withPresets, err := Parse(data, testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var viaPreset int
	for _, c := range withPresets.Selected(true) {
		if c.Portability == PortableViaPreset {
			viaPreset++
			if c.BaseURL == "" {
				t.Errorf("preset-resolved %q still has empty base_url", c.SourceProvider)
			}
		}
	}
	if viaPreset == 0 {
		t.Fatal("no connections resolved via preset")
	}

	// Same file, no presets at all: those rows must become blocked, not silently
	// imported pointing nowhere.
	withoutPresets, err := Parse(data, nil)
	if err != nil {
		t.Fatalf("parse without presets: %v", err)
	}
	for _, c := range withoutPresets.Selected(true) {
		if c.BaseURL == "" {
			t.Errorf("connection %q imported with no endpoint when presets absent", c.Name)
		}
	}
	var unknown int
	for _, c := range withoutPresets.Blocked {
		if c.Portability == NotPortableUnknown {
			unknown++
		}
	}
	if unknown == 0 {
		t.Error("without presets, built-in providers should be blocked as unknown_endpoint")
	}
}

// TestNoConnectionIsImportedWithoutAnEndpoint is the invariant that makes the
// whole feature trustworthy: an imported row must be usable.
func TestNoConnectionIsImportedWithoutAnEndpoint(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, c := range plan.Selected(true) {
		if strings.TrimSpace(c.BaseURL) == "" {
			t.Errorf("importable connection %q has no base_url", c.Name)
		}
		if c.Format == "" {
			t.Errorf("importable connection %q has no format", c.Name)
		}
		if strings.TrimSpace(c.Name) == "" {
			t.Errorf("connection from %s has an empty name", c.SourceProvider)
		}
	}
}

// --- health ----------------------------------------------------------------

// TestDeadConnectionsAreSeparatedNotDropped: the 404/402 rows still exist in the
// plan, they are simply not selected by default. Dropping them outright would
// remove the user's choice.
func TestDeadConnectionsAreSeparatedNotDropped(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(plan.Unusable) == 0 {
		t.Fatal("fixture contains dead connections; none were classified as unusable")
	}
	def := plan.Selected(false)
	all := plan.Selected(true)
	if len(all) <= len(def) {
		t.Errorf("include-unusable did not widen the set: %d vs %d", len(all), len(def))
	}
	for _, c := range def {
		if c.Health != HealthOK {
			t.Errorf("default selection contains a %s connection (%q)", c.Health, c.Name)
		}
	}
}

// TestFatalErrorCodesAreTreatedAsDead pins the specific signals we rely on.
func TestFatalErrorCodesAreTreatedAsDead(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, c := range plan.Healthy {
		if c.Health != HealthOK {
			t.Errorf("%q in Healthy but marked %s", c.Name, c.Health)
		}
	}
	var dead, inactive int
	for _, c := range plan.Unusable {
		switch c.Health {
		case HealthDead:
			dead++
		case HealthInactive:
			inactive++
		default:
			t.Errorf("%q in Unusable but marked %s", c.Name, c.Health)
		}
	}
	if dead == 0 {
		t.Error("expected connections with fatal error codes to be marked dead")
	}
	_ = inactive
}

// --- combos ----------------------------------------------------------------

// TestPartialCombosImportPortableMembersOnly is the design decision from the
// brainstorm made executable: a combo mixing portable and non-portable members
// keeps the portable ones and reports the rest, instead of being dropped whole
// or imported with dangling references.
func TestPartialCombosImportPortableMembersOnly(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(plan.Combos) == 0 {
		t.Fatal("no combos parsed; fixture has 12")
	}

	var partial int
	for _, c := range plan.Combos {
		if len(c.Models) == 0 {
			t.Errorf("combo %q imported with zero models; it should have been skipped", c.Name)
		}
		if c.Partial {
			partial++
			if len(c.SkippedModels) == 0 {
				t.Errorf("combo %q marked partial but lists nothing skipped", c.Name)
			}
		}
		for _, m := range c.Models {
			for _, s := range c.SkippedModels {
				if m == s {
					t.Errorf("combo %q lists %q as both kept and skipped", c.Name, m)
				}
			}
		}
	}
	if partial == 0 {
		t.Error("fixture contains combos mixing portable and OAuth models; none marked partial")
	}
}

// TestCombosNeverReferenceUnimportableProviders guards against dangling refs.
func TestCombosNeverReferenceUnimportableProviders(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	blockedPrefix := map[string]bool{}
	for _, c := range plan.Blocked {
		if c.SourcePrefix() != "" {
			blockedPrefix[c.SourcePrefix()] = true
		}
	}
	for _, combo := range plan.Combos {
		for _, m := range combo.Models {
			if i := strings.Index(m, "/"); i > 0 {
				if blockedPrefix[m[:i]] {
					t.Errorf("combo %q keeps %q whose provider is not importable", combo.Name, m)
				}
			}
		}
	}
}

// --- safety ----------------------------------------------------------------

// TestPlanJSONNeverLeaksAPIKeys: the preview travels to a browser. Keys must not.
func TestPlanJSONNeverLeaksAPIKeys(t *testing.T) {
	raw := loadFixture(t)
	plan, err := Parse(raw, testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if strings.Contains(string(out), "sk-REDACTED") {
		t.Error("serialized Plan contains API key material")
	}
	// Positive control: the keys really are present in the parsed data.
	var found bool
	for _, c := range plan.Selected(true) {
		if c.APIKey != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("no API keys were parsed at all; import would produce unusable connections")
	}
}

// TestSummarizeIsLogSafe: counts only, no endpoints or names.
func TestSummarizeIsLogSafe(t *testing.T) {
	plan, err := Parse(loadFixture(t), testPresets())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s := plan.Summarize()
	if s.Healthy+s.Unusable+s.Blocked == 0 {
		t.Fatal("summary counts are all zero")
	}
	out, _ := json.Marshal(s)
	for _, needle := range []string{"http", "sk-", "api."} {
		if strings.Contains(string(out), needle) {
			t.Errorf("summary leaks %q: %s", needle, out)
		}
	}
}

// --- robustness ------------------------------------------------------------

// TestParseDoesNotPanicOnMalformedInput: this parses an uploaded file. It must
// fail, never crash the server.
func TestParseDoesNotPanicOnMalformedInput(t *testing.T) {
	inputs := []string{
		`{"providerConnections":"not-a-list","providerNodes":[]}`,
		`{"providerConnections":[{"provider":123}],"providerNodes":[]}`,
		`{"providerConnections":[{}],"providerNodes":[{}],"combos":[{}]}`,
		`{"providerConnections":[{"providerSpecificData":"oops"}],"providerNodes":[]}`,
		`{"providerConnections":[],"providerNodes":[],"combos":"nope"}`,
		`{"providerConnections":[null],"providerNodes":[null],"combos":[null]}`,
	}
	src := &nineRouter{}
	for i, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("input %d panicked: %v", i, r)
				}
			}()
			_, _ = src.Parse([]byte(in), testPresets())
		}()
	}
}

// TestParseIsDeterministic: preview and import must agree, so parsing the same
// bytes twice must produce the same plan.
func TestParseIsDeterministic(t *testing.T) {
	raw := loadFixture(t)
	a, err := Parse(raw, testPresets())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse(raw, testPresets())
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Error("parsing the same export twice produced different plans")
	}
}

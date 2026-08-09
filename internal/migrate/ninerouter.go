package migrate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ninerouter.go — parses a 9router backup export.
//
// FORMAT NOTES (derived from a real 1.18 MB export, not from documentation):
//
//   - Top level is an object with providerConnections, providerNodes, combos,
//     apiKeys, customModels, settings, …
//
//   - providerNodes describes user-defined "openai-compatible" endpoints:
//     {id, type, name, prefix, baseUrl, apiType}
//
//   - providerConnections is where the credentials live. Its `provider` field is
//     EITHER a providerNodes id (custom endpoint) OR a built-in adapter name
//     ("xiaomi-mimo", "qoder", …).
//
//   - Crucially, a connection to a custom endpoint DUPLICATES the endpoint into
//     providerSpecificData.baseUrl. So custom connections are self-contained and
//     need no join. We still read providerNodes to recover the prefix and a
//     human-readable name.
//
//   - Built-in adapters write no URL at all: 9router knows the endpoint from
//     compiled-in code. Those are only importable if Lintasan has a preset.
//
//   - Health signals: `isActive` (user toggle), `errorCode` (last upstream
//     failure) and `testStatus`. In the sample, 920 of 922 xiaomi-mimo rows sat
//     at errorCode 404 / testStatus "unavailable" — exhausted farmed keys.
//
//   - combos[].models are strings shaped "<prefix>/<model>".

func init() { Register(&nineRouter{}) }

type nineRouter struct{}

func (n *nineRouter) Name() string { return "9router" }

// --- wire types ------------------------------------------------------------
//
// Deliberately permissive: this parses a file a user uploaded. Fields we do not
// understand are ignored rather than rejected, and every type assertion is
// guarded, because a malformed upload must yield an error and never a panic.

type nrNode struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Prefix  string `json:"prefix"`
	BaseURL string `json:"baseUrl"`
	APIType string `json:"apiType"`
}

type nrConn struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	AuthType string `json:"authType"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Priority int    `json:"priority"`
	IsActive *bool  `json:"isActive"`
	APIKey   string `json:"apiKey"`

	ErrorCode  int    `json:"errorCode"`
	TestStatus string `json:"testStatus"`

	// Raw so a non-object value (seen in malformed files) cannot break decoding.
	ProviderSpecificData json.RawMessage `json:"providerSpecificData"`
}

type nrPSD struct {
	BaseURL  string `json:"baseUrl"`
	Prefix   string `json:"prefix"`
	APIType  string `json:"apiType"`
	NodeName string `json:"nodeName"`
}

type nrCombo struct {
	Name   string          `json:"name"`
	Models json.RawMessage `json:"models"`
}

type nrExport struct {
	ProviderConnections []json.RawMessage `json:"providerConnections"`
	ProviderNodes       []json.RawMessage `json:"providerNodes"`
	Combos              []json.RawMessage `json:"combos"`
}

// --- detection -------------------------------------------------------------

// Detect looks for the pair of keys that together identify a 9router backup.
// Requiring both avoids claiming any JSON that happens to have "combos".
func (n *nineRouter) Detect(data []byte) bool {
	var probe struct {
		ProviderConnections json.RawMessage `json:"providerConnections"`
		ProviderNodes       json.RawMessage `json:"providerNodes"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return isJSONArray(probe.ProviderConnections) && isJSONArray(probe.ProviderNodes)
}

func isJSONArray(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return strings.HasPrefix(t, "[")
}

// --- OAuth providers -------------------------------------------------------

// oauthProviders are adapters that authenticate through a login/refresh-token
// flow. Knowing their URL is not enough to use them, so they are reported as
// blocked rather than imported. Explicitly out of scope (design decision,
// 2026-08-09).
var oauthProviders = map[string]bool{
	"codex":     true,
	"qoder":     true,
	"grok-cli":  true,
	"cline":     true,
	"clinepass": true,
	"kilocode":  true,
	"gemini-cli": true,
	"claude-code": true,
}

func isOAuthProvider(provider, authType string) bool {
	if strings.EqualFold(strings.TrimSpace(authType), "oauth") {
		return true
	}
	return oauthProviders[strings.ToLower(strings.TrimSpace(provider))]
}

// --- parsing ---------------------------------------------------------------

func (n *nineRouter) Parse(data []byte, presets PresetLookup) (Plan, error) {
	var exp nrExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return Plan{}, fmt.Errorf("decode export: %w", err)
	}

	nodes := decodeNodes(exp.ProviderNodes)
	plan := Plan{Source: "9router"}

	// prefixOK tracks which combo prefixes end up backed by an importable
	// connection, so combo members can be filtered afterwards.
	prefixOK := map[string]bool{}
	prefixSeen := map[string]bool{}

	for _, raw := range exp.ProviderConnections {
		c, ok := decodeConn(raw)
		if !ok {
			continue
		}
		conn := mapConnection(c, nodes, presets)
		if conn.Prefix != "" {
			prefixSeen[conn.Prefix] = true
		}

		switch conn.Portability {
		case NotPortableOAuth, NotPortableUnknown:
			plan.Blocked = append(plan.Blocked, conn)
		default:
			if conn.Health == HealthOK {
				// A combo member is only kept if a HEALTHY connection backs its
				// prefix. A prefix that exists only via dead connections would
				// leave the combo referencing something the import won't create.
				if conn.Prefix != "" {
					prefixOK[conn.Prefix] = true
				}
				plan.Healthy = append(plan.Healthy, conn)
			} else {
				plan.Unusable = append(plan.Unusable, conn)
			}
		}
	}

	plan.Combos = mapCombos(exp.Combos, prefixOK, prefixSeen)
	plan.Warnings = buildWarnings(plan)

	sortPlan(&plan)
	return plan, nil
}

func decodeNodes(raws []json.RawMessage) map[string]nrNode {
	out := make(map[string]nrNode, len(raws))
	for _, raw := range raws {
		var nd nrNode
		if err := json.Unmarshal(raw, &nd); err != nil || nd.ID == "" {
			continue
		}
		out[nd.ID] = nd
	}
	return out
}

func decodeConn(raw json.RawMessage) (nrConn, bool) {
	var c nrConn
	if err := json.Unmarshal(raw, &c); err != nil {
		return nrConn{}, false
	}
	if c.Provider == "" && c.APIKey == "" && c.Name == "" {
		return nrConn{}, false // empty/null entry
	}
	return c, true
}

// mapConnection is the heart of the translation: decide where a row belongs and
// recover its endpoint.
func mapConnection(c nrConn, nodes map[string]nrNode, presets PresetLookup) Connection {
	psd := decodePSD(c.ProviderSpecificData)
	node, isCustom := nodes[c.Provider]

	out := Connection{
		APIKey:         c.APIKey,
		HasKey:         strings.TrimSpace(c.APIKey) != "",
		Format:         "openai", // every portable shape here is OpenAI-compatible
		Priority:       c.Priority,
		IsActive:       c.IsActive == nil || *c.IsActive,
		SourceProvider: c.Provider,
		SourceID:       c.ID,
		Health:         classifyHealth(c),
	}

	// Name: prefer the node's label, since a connection is often just "Key 1".
	// We deliberately sanitize: a source free-text field can contain anything
	// (including strings that look like secrets), and it ends up in a preview
	// sent to the browser, so it must not carry credential-shaped noise.
	out.Name = sanitizeName(pickName(isCustom, node.Name, c.Name, c.Provider))

	// Prefix drives combo resolution.
	switch {
	case psd.Prefix != "":
		out.Prefix = psd.Prefix
	case node.Prefix != "":
		out.Prefix = node.Prefix
	default:
		out.Prefix = shortProviderAlias(c.Provider)
	}

	// OAuth is checked first: an OAuth connection may also have a baseUrl, and
	// importing it would produce a connection that 401s forever.
	if isOAuthProvider(c.Provider, c.AuthType) {
		out.Portability = NotPortableOAuth
		out.Reason = "uses a login/refresh-token flow; needs a dedicated adapter"
		out.BaseURL = firstNonEmpty(psd.BaseURL, node.BaseURL)
		return out
	}

	// Custom endpoints are self-describing.
	if url := firstNonEmpty(psd.BaseURL, node.BaseURL); url != "" {
		out.BaseURL = url
		out.Portability = PortableDirect
		return out
	}

	// Built-in adapter: the export omits the URL, so a preset must supply it.
	if presets != nil {
		if url, ok := presets.BaseURLFor(c.Provider); ok && url != "" {
			out.BaseURL = url
			out.Portability = PortableViaPreset
			return out
		}
	}

	out.Portability = NotPortableUnknown
	out.Reason = fmt.Sprintf("no known endpoint for built-in provider %q", c.Provider)
	return out
}

func decodePSD(raw json.RawMessage) nrPSD {
	var p nrPSD
	if len(raw) == 0 {
		return p
	}
	// A non-object value here is normal in malformed files; ignore it.
	_ = json.Unmarshal(raw, &p)
	return p
}

// classifyHealth reads the source router's own verdict on the connection.
//
// We trust it rather than re-testing: the export is a snapshot, and probing
// hundreds of endpoints during an upload would be slow and rude.
func classifyHealth(c nrConn) Health {
	if c.IsActive != nil && !*c.IsActive {
		return HealthInactive
	}
	switch c.ErrorCode {
	case 401, 402, 403, 404:
		return HealthDead
	}
	return HealthOK
}

// shortProviderAlias derives a usable prefix for built-in providers, whose ids
// are plain names like "xiaomi-mimo".
func shortProviderAlias(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" || strings.HasPrefix(p, "openai-compatible-") ||
		strings.HasPrefix(p, "anthropic-compatible-") {
		return ""
	}
	return p
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// pickName chooses the most human-meaningful label available.
func pickName(isCustom bool, nodeName, connName, provider string) string {
	switch {
	case isCustom && nodeName != "" && connName != "":
		return nodeName + " — " + connName
	case isCustom && nodeName != "":
		return nodeName
	case connName != "":
		return connName
	default:
		return provider
	}
}

// secretShaped matches strings that look like credential material.
//
// Source free-text fields (a connection's `name`, `defaultModel`, …) are
// user-controlled and, in real exports, sometimes literally contain a key —
// people paste one into the name box. Those strings flow into a preview that is
// rendered in a browser and may be logged, so they get scrubbed here rather
// than trusted.
var secretShaped = regexp.MustCompile(
	`(?i)\b(sk|pk|api|key|token|secret|bearer)[-_][A-Za-z0-9_\-]{6,}|` +
		`\b(eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,})`)

// sanitizeName strips credential-shaped substrings and clamps length so a
// hostile or sloppy export cannot inject noise into the UI.
func sanitizeName(s string) string {
	s = strings.TrimSpace(secretShaped.ReplaceAllString(s, "[redacted]"))
	s = strings.TrimSpace(strings.Trim(s, "—- "))
	if s == "" {
		return "Imported connection"
	}
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:80]) + "…"
	}
	return s
}

// --- combos ----------------------------------------------------------------

// mapCombos keeps the members whose provider is importable and reports the rest.
//
// Design decision (2026-08-09): a combo mixing portable and non-portable members
// is imported with the portable ones only. Dropping the whole combo is too
// blunt — a 4-of-7 combo still works — and importing it verbatim would leave
// references to models that do not exist, which only surface as a failed request
// later.
func mapCombos(raws []json.RawMessage, prefixOK, prefixSeen map[string]bool) []Combo {
	var out []Combo
	for _, raw := range raws {
		var c nrCombo
		if err := json.Unmarshal(raw, &c); err != nil || c.Name == "" {
			continue
		}
		models := decodeModelList(c.Models)
		if len(models) == 0 {
			continue
		}

		combo := Combo{Name: c.Name}
		for _, m := range models {
			prefix, ok := comboPrefix(m)
			// Keep entries with no prefix: they are plain model ids, and the
			// user can fix them up. Drop entries whose provider we know we
			// cannot import.
			if ok && prefixSeen[prefix] && !prefixOK[prefix] {
				combo.SkippedModels = append(combo.SkippedModels, m)
				continue
			}
			combo.Models = append(combo.Models, m)
		}

		if len(combo.Models) == 0 {
			continue // nothing survived; importing an empty combo helps nobody
		}
		combo.Partial = len(combo.SkippedModels) > 0
		out = append(out, combo)
	}
	return out
}

func decodeModelList(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	// Normal case: a JSON array of strings.
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list
	}
	// Seen in the wild: the array stored as a JSON string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if err := json.Unmarshal([]byte(s), &list); err == nil {
			return list
		}
	}
	return nil
}

func comboPrefix(model string) (string, bool) {
	if i := strings.Index(model, "/"); i > 0 {
		return model[:i], true
	}
	return "", false
}

// --- reporting -------------------------------------------------------------

func buildWarnings(p Plan) []string {
	var w []string
	byReason := map[Portability]int{}
	for _, c := range p.Blocked {
		byReason[c.Portability]++
	}
	if n := byReason[NotPortableOAuth]; n > 0 {
		w = append(w, fmt.Sprintf(
			"%d connection(s) use OAuth login and cannot be migrated — keep using your current router for those.", n))
	}
	if n := byReason[NotPortableUnknown]; n > 0 {
		w = append(w, fmt.Sprintf(
			"%d connection(s) use a provider Lintasan has no endpoint for; add them manually if you need them.", n))
	}
	if n := len(p.Unusable); n > 0 {
		w = append(w, fmt.Sprintf(
			"%d connection(s) were already failing or disabled in the source router and are excluded by default.", n))
	}
	return w
}

// sortPlan makes output stable so preview and import always agree, and so the
// UI does not reshuffle between refreshes.
func sortPlan(p *Plan) {
	byName := func(s []Connection) {
		sort.SliceStable(s, func(i, j int) bool {
			if s[i].Name != s[j].Name {
				return s[i].Name < s[j].Name
			}
			return s[i].SourceID < s[j].SourceID
		})
	}
	byName(p.Healthy)
	byName(p.Unusable)
	byName(p.Blocked)
	sort.SliceStable(p.Combos, func(i, j int) bool { return p.Combos[i].Name < p.Combos[j].Name })
}

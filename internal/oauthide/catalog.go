// Package oauthide ports 9router OAUTH_PROVIDERS (v0.4.71) into Lintasan Go.
// Experimental lab — not all flow types are implemented yet.
package oauthide

// RiskNotice mirrors 9router RISK_NOTICE on deprecated subscription OAuth providers.
const RiskNotice = "Risk: subscription/OAuth session may not be licensed for third-party routers. Account restriction or ban possible. Personal BYO use only."

// FlowType describes how the provider authenticates.
type FlowType string

const (
	FlowPKCE         FlowType = "authorization_code_pkce"
	FlowAuthCode     FlowType = "authorization_code"
	FlowDevice       FlowType = "device_code"
	FlowImport       FlowType = "import_token"
	FlowCustomDevice FlowType = "custom_device"
	FlowLocalApp     FlowType = "local_app_callback"
)

// ImplStatus is engineering readiness (not upstream ToS).
type ImplStatus string

const (
	ImplReady     ImplStatus = "ready"      // authorize + callback + exchange (+ refresh where applicable)
	ImplPartial   ImplStatus = "partial"    // started, not e2e
	ImplPlanned   ImplStatus = "planned"    // catalog only
	ImplImportOnly ImplStatus = "import_only"
)

// Provider is one OAuth IDE entry (9router id = Lintasan slug).
type Provider struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Flow            FlowType   `json:"flow"`
	Impl            ImplStatus `json:"implementation"`
	Deprecated      bool       `json:"deprecated,omitempty"`
	DeprecationNote string     `json:"deprecation_note,omitempty"`
	RiskNotice      string     `json:"risk_notice,omitempty"`
	Notes           string     `json:"notes,omitempty"`
}

// Catalog is the full 9router OAUTH_PROVIDERS set (8 entries).
func Catalog() []Provider {
	return []Provider{
		{ID: "claude", Name: "Claude Code", Flow: FlowPKCE, Impl: ImplReady, Deprecated: true, RiskNotice: RiskNotice,
			Notes: "PKCE via claude.ai + api.anthropic.com token"},
		{ID: "antigravity", Name: "Antigravity", Flow: FlowPKCE, Impl: ImplPlanned, Deprecated: true,
			DeprecationNote: "Antigravity IDE only — proxy use may trigger bans", RiskNotice: RiskNotice},
		{ID: "codex", Name: "OpenAI Codex", Flow: FlowPKCE, Impl: ImplReady, Deprecated: true, RiskNotice: RiskNotice,
			Notes: "OpenAI auth.openai.com PKCE"},
		{ID: "github", Name: "GitHub Copilot", Flow: FlowDevice, Impl: ImplReady, Deprecated: true, RiskNotice: RiskNotice,
			Notes: "Device code + copilot_internal token (9router GITHUB_CONFIG)"},
		{ID: "cursor", Name: "Cursor IDE", Flow: FlowImport, Impl: ImplImportOnly,
			Notes: "Import from state.vscdb — no browser OAuth URL"},
		{ID: "xai", Name: "xAI (Grok)", Flow: FlowPKCE, Impl: ImplReady,
			Notes: "PKCE; public client from 9router/xAI upstream"},
		{ID: "kilocode", Name: "Kilo Code", Flow: FlowCustomDevice, Impl: ImplPlanned,
			Notes: "api.kilo.ai device-auth poll"},
		{ID: "cline", Name: "Cline", Flow: FlowLocalApp, Impl: ImplPlanned,
			Notes: "app.cline.bot callback flow"},
	}
}

// ByID returns provider metadata or nil.
func ByID(id string) *Provider {
	for _, p := range Catalog() {
		if p.ID == id {
			cp := p
			return &cp
		}
	}
	return nil
}

// IsKnownProvider reports whether id is in the 9router OAuth catalog.
func IsKnownProvider(id string) bool {
	return ByID(id) != nil
}

// CanStartAuthorize is true when dashboard may start a browser/device flow (not import-only).
func CanStartAuthorize(id string) bool {
	p := ByID(id)
	if p == nil {
		return false
	}
	return p.Impl == ImplReady || p.Impl == ImplPartial
}
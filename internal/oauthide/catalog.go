// Package oauthide ports 9router OAUTH_PROVIDERS (v0.4.71 → v0.5.104) into Lintasan Go.
// Experimental lab — not all flow types are implemented yet.
package oauthide

// RiskNotice mirrors 9router RISK_NOTICE on deprecated subscription OAuth providers.
const RiskNotice = "Risk: subscription/OAuth session may not be licensed for third-party routers. Account restriction or ban possible. Personal BYO use only."

// FlowType describes how the provider authenticates.
type FlowType string

const (
	FlowPKCE          FlowType = "authorization_code_pkce"
	FlowAuthCode      FlowType = "authorization_code"
	FlowDevice        FlowType = "device_code"
	FlowImport        FlowType = "import_token"
	FlowCustomDevice  FlowType = "custom_device"
	FlowLocalApp      FlowType = "local_app_callback"
	FlowCLIPoll       FlowType = "cli_poll"                  // Z.ai / GLM coding plan
	FlowImportStateDB FlowType = "import_state_vscdb"        // Cursor .vscdb
	FlowRSAKeyExch    FlowType = "rsa_key_exchange"          // Zed hosted LLM
	FlowFIREBASE      FlowType = "firebase_password"         // Windsurf
	FlowX25519        FlowType = "x25519_encrypted_callback" // Xiaomi MiMo
)

// ImplStatus is engineering readiness (not upstream ToS).
type ImplStatus string

const (
	ImplReady      ImplStatus = "ready"   // authorize + callback + exchange (+ refresh where applicable)
	ImplPartial    ImplStatus = "partial" // started, not e2e
	ImplPlanned    ImplStatus = "planned" // catalog only
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

// Catalog is the full 9router OAUTH_PROVIDERS set (v0.5.104, 29 entries).
// The 8 catalogued in v0.4.71 have implementation impl=Ready; the 21 new
// providers the 9router 2026-08..10 OAUTH_PROVIDERS added are all listed but
// global Impl stays truthful (ImplPlanned for anything not ported yet).
func Catalog() []Provider {
	return []Provider{
		// ── v0.4.71 base, asserted by existing unit tests ────────────────────
		{ID: "claude", Name: "Claude Code", Flow: FlowPKCE, Impl: ImplReady, Deprecated: true, RiskNotice: RiskNotice,
			Notes: "PKCE via claude.ai + api.anthropic.com token"},
		{ID: "antigravity", Name: "Antigravity", Flow: FlowAuthCode, Impl: ImplReady, Deprecated: true,
			DeprecationNote: "Antigravity IDE only — proxy use may trigger bans", RiskNotice: RiskNotice,
			Notes: "Google OAuth2 + loadCodeAssist (9router port)"},
		{ID: "codex", Name: "OpenAI Codex", Flow: FlowPKCE, Impl: ImplReady, Deprecated: true, RiskNotice: RiskNotice,
			Notes: "OpenAI auth.openai.com PKCE"},
		{ID: "github", Name: "GitHub Copilot", Flow: FlowDevice, Impl: ImplReady, Deprecated: true, RiskNotice: RiskNotice,
			Notes: "Device code + copilot_internal token (9router GITHUB_CONFIG)"},
		{ID: "cursor", Name: "Cursor IDE", Flow: FlowImportStateDB, Impl: ImplImportOnly,
			Notes: "Import from state.vscdb — no browser OAuth URL"},
		{ID: "xai", Name: "xAI (Grok)", Flow: FlowPKCE, Impl: ImplReady,
			Notes: "PKCE; public client from 9router/xAI upstream"},
		{ID: "kilocode", Name: "Kilo Code", Flow: FlowCustomDevice, Impl: ImplReady,
			Notes: "api.kilo.ai device-auth poll"},
		{ID: "cline", Name: "Cline", Flow: FlowLocalApp, Impl: ImplReady,
			Notes: "app.cline.bot callback flow"},

		// ── Added in 9router v0.5.104 — catalogued, not ported ───────────────
		{ID: "clinepass", Name: "Cline Pass", Flow: FlowLocalApp, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "app.cline.bot premium pass; shares endpoints with cline"},
		{ID: "codebuddy-cn", Name: "Codebuddy CN", Flow: FlowDevice, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "Tencent copilot project budgeted plan, china lane"},
		{ID: "codebuddy-intl", Name: "Codebuddy Intl", Flow: FlowDevice, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "international lane of codebuddy-cn"},
		{ID: "gemini-cli", Name: "Gemini CLI", Flow: FlowAuthCode, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "Google Cloud authcode — GOOGLE_OAUTH_CLIENT, requires useAuthUrl"},
		{ID: "grok-cli", Name: "Grok CLI", Flow: FlowDevice, Impl: ImplPlanned,
			Notes: "x.ai device_code for grok CLI — distinct from xai PKCE lane"},
		{ID: "grok-web", Name: "Grok Web", Flow: FlowImport, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "grok.com session import — non-OAuth, session cookie"},
		{ID: "iflow", Name: "iFlow", Flow: FlowAuthCode, Impl: ImplPlanned,
			Notes: "iflow.cn authcode, public client 10009311001"},
		{ID: "kimi", Name: "Kimi Coding", Flow: FlowDevice, Impl: ImplPlanned,
			Notes: "kimi.com coding plan device_code (KIMI_CODING_CLIENT_ID)"},
		{ID: "kimchi", Name: "Kimchi", Flow: FlowImport, Impl: ImplPlanned,
			Notes: "kimchi.dev key import"},
		{ID: "kiro", Name: "Kiro", Flow: FlowCustomDevice, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "aws-builder-id EventStream + auto-refresh"},
		{ID: "minimax-code", Name: "MiniMax Code", Flow: FlowDevice, Impl: ImplPlanned,
			Notes: "MiniMax coding-plan credits"},
		{ID: "minimax-code-global", Name: "MiniMax Code Global", Flow: FlowDevice, Impl: ImplPlanned,
			Notes: "MiniMax coding-plan credits, intl lane"},
		{ID: "muse", Name: "Muse", Flow: FlowDevice, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "muse.ai device_code"},
		{ID: "nous", Name: "Nous Research", Flow: FlowDevice, Impl: ImplPartial,
			Notes: "portal.nousresearch.com device_code (Hermes CLI-compatible) — request/poll ported in nous.go; refresh pending"},
		{ID: "qoder", Name: "Qoder", Flow: FlowCustomDevice, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "Lintasan routes qoder natively (internal/qoder, format qoder) — OAuth IDE lane not ported"},
		{ID: "qoder-cn", Name: "Qoder CN", Flow: FlowCustomDevice, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "china lane of qoder"},
		{ID: "glm", Name: "Zai GLM Coding", Flow: FlowCLIPoll, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "zcode.z.ai CLI init/poll; no refresh (re-login on expiry)"},
		{ID: "trae", Name: "Trae CN", Flow: FlowAuthCode, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "Lintasan ships Trae as in-process experimental adapter (internal/trae) — OAuth IDE lane not ported"},
		{ID: "windsurf", Name: "Windsurf", Flow: FlowFIREBASE, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "firebase password sign-in → codeium seat → GetChatMessage"},
		{ID: "zed", Name: "Zed", Flow: FlowRSAKeyExch, Impl: ImplPlanned,
			Notes: "cloud.zed.dev hosted LLM, RSA-2048 keypair callback — NOT OAuth2"},
		{ID: "xiaomi-mimo", Name: "Xiaomi MiMo", Flow: FlowX25519, Impl: ImplPlanned, RiskNotice: RiskNotice,
			Notes: "x.xiaomi.com ECDH encrypted callback"},
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

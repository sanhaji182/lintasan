package expprovider

// cohort_a.go — Cohort-A provider DESCRIPTORS (config scaffold).
//
// These are the DECLARATIVE definitions for the remaining Cohort-A ACP providers.
// They demonstrate the framework's thesis: onboarding a provider is a descriptor
// + a fixture, NOT new architecture. Each descriptor names the provider's ACP
// adapter executable, its auth env var + ACP authenticate method id, and its
// declared capabilities — nothing else.
//
// STATUS — IMPORTANT (do not misread):
//   - These descriptors are SCAFFOLD. They are NOT registered, NOT admitted, NOT
//     validated, and NOT wired anywhere. Constructing one has zero effect until a
//     future, separately-approved onboarding checkpoint calls Admit against it
//     WITH a recorded fixture (or the operator-gated live CLI).
//   - The executable names, auth method ids, and launch args below are BEST-KNOWN
//     values from each tool's public ACP surface, but they are WIRE-UNVERIFIED
//     for this codebase. The Codex lesson stands: the real wire contract
//     (authenticate ordering, session/new params, ContentBlock shape, exact
//     method ids) MUST be confirmed against the real binary during that
//     provider's own live validation before its descriptor is trusted. Treat any
//     field marked (UNVERIFIED) as a hypothesis to be checked, not a fact.
//   - Capabilities are DECLARED only (Invariant 5): display-only, never trusted
//     for routing.
//
// The point of having them now is purely architectural: to PROVE the framework
// needs no per-provider flow. See framework_generic_test.go for the executable
// proof that a brand-new provider onboards through AdmitProvider with no new
// framework code.

import "github.com/sanhaji182/lintasan-go/internal/provider"

// ClaudeCodeDescriptor — Anthropic Claude Code via the cc-acp community ACP wrapper.
// VALIDATED: cc-acp v0.1.1 (npm: claude-code-acp) wraps Claude Code SDK over ACP.
// Auth model: claude-code-subscription (OAuth/browser login via `claude auth login`).
// The underlying Claude Code binary also accepts ANTHROPIC_API_KEY env var directly,
// but cc-acp only exposes subscription auth. AuthEnvVar kept for G4 injection path.
func ClaudeCodeDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         "claude-code",
		DefaultPath:  "cc-acp",                    // VALIDATED: npm bin from claude-code-acp package
		AuthMode:     AuthAPIKey,                   // env var path (ANTHROPIC_API_KEY) works for underlying Claude Code
		AuthEnvVar:   "ANTHROPIC_API_KEY",          // VALIDATED: `claude auth status` confirms this source
		AuthMethodID: "claude-code-subscription",   // VALIDATED: only method cc-acp advertises
		Capabilities: provider.NewCapabilitySet(
			provider.CapCoding,
			provider.CapToolCalling,
			provider.CapReasoning,
			provider.CapStreaming,
		),
		ForeignAuthVars: []string{"OPENAI_API_KEY", "GEMINI_API_KEY", "GITHUB_TOKEN"},
	}
}

// GeminiCLIDescriptor — Google Gemini CLI in native ACP mode.
// VALIDATED: gemini v0.44.1 with --acp flag speaks native ACP over stdio.
// Auth model: 4 methods (oauth-personal, gemini-api-key, vertex-ai, gateway).
// Key storage: system keychain (not env var in ACP mode). Env var GEMINI_API_KEY
// not picked up by --acp mode; key must be configured via `gemini` interactive setup.
func GeminiCLIDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         "gemini-cli",
		DefaultPath:  "gemini",          // VALIDATED: @google/gemini-cli v0.44.1
		Args:         []string{"--acp"}, // VALIDATED: native ACP flag (--experimental-acp deprecated)
		AuthMode:     AuthAPIKey,
		AuthEnvVar:   "GEMINI_API_KEY",  // VALIDATED: selectedType in settings.json, but key stored in keychain
		AuthMethodID: "gemini-api-key",  // VALIDATED: one of 4 offered methods
		Capabilities: provider.NewCapabilitySet(
			provider.CapCoding,
			provider.CapToolCalling,
			provider.CapReasoning,
			provider.CapStreaming,
		),
		ForeignAuthVars: []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GITHUB_TOKEN"},
	}
}

// CopilotDescriptor — GitHub Copilot CLI in native ACP mode.
// VALIDATED: copilot v1.0.56 (@github/copilot) with --acp flag speaks native ACP.
// Auth model: copilot-login (device-auth OAuth flow via `copilot login`).
// Env vars (precedence): COPILOT_GITHUB_TOKEN > GH_TOKEN > GITHUB_TOKEN.
// Token types: fine-grained PAT with "Copilot Requests" permission, or OAuth token.
// Classic PATs (ghp_) NOT supported. session/new blocks until valid auth present.
func CopilotDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         "copilot",
		DefaultPath:  "copilot",         // VALIDATED: @github/copilot v1.0.56
		Args:         []string{"--acp"}, // VALIDATED: native ACP flag
		AuthMode:     AuthAPIKey,
		AuthEnvVar:   "COPILOT_GITHUB_TOKEN", // VALIDATED: highest precedence env var
		AuthMethodID: "copilot-login",         // VALIDATED: only method offered
		Capabilities: provider.NewCapabilitySet(
			provider.CapCoding,
			provider.CapToolCalling,
			provider.CapStreaming,
		),
		ForeignAuthVars: []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY"},
	}
}

// CohortADescriptors returns every Cohort-A descriptor (Codex + the scaffold
// providers) for diagnostics/enumeration. It REGISTERS NOTHING — it is a catalog
// listing, not an activation. A future onboarding checkpoint admits a provider
// individually (with its fixture), one approved step at a time.
func CohortADescriptors() []ProviderDescriptor {
	return []ProviderDescriptor{
		CodexDescriptor(),
		ClaudeCodeDescriptor(),
		GeminiCLIDescriptor(),
		CopilotDescriptor(),
	}
}

// CohortACredentialSource builds the staging credential mapping for Cohort-A:
// it registers, for every descriptor, the EXPLICIT provider→env-var binding the
// G4 injector uses to scope each provider's secret. It returns the SAME
// CredentialSource interface Codex already uses (EnvCredentialSource), so the
// mapping is the only per-provider config — no new credential machinery.
//
// SCOPING (Invariant 3): a lookup for "claude-code" can ONLY read
// ANTHROPIC_API_KEY, never another provider's var. Secrets are read live from
// the process env on each lookup (rotation-safe); this function holds no secret
// itself. A descriptor whose AuthMode is AuthNone contributes no mapping.
//
// IMPORTANT: registering a mapping does NOT supply a credential — the env var
// must actually be set for that provider to admit. An unset var yields
// (\"\", false), which the harness turns into a NO-GO (the provider cannot launch
// + auth). This is exactly the Codex environment-blocker posture, generalized.
func CohortACredentialSource() *EnvCredentialSource {
	src := NewEnvCredentialSource()
	for _, d := range CohortADescriptors() {
		if d.AuthMode == AuthNone || d.AuthEnvVar == "" {
			continue
		}
		src.Map(d.Name, d.AuthEnvVar)
	}
	return src
}

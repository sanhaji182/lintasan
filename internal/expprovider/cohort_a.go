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

// ClaudeCodeDescriptor — Anthropic Claude Code via its ACP adapter.
// (UNVERIFIED) adapter executable + authenticate method id are best-known public
// values; confirm against the real binary at Claude Code's live validation.
func ClaudeCodeDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         "claude-code",
		DefaultPath:  "claude-code-acp", // (UNVERIFIED) Anthropic's ACP adapter binary
		AuthMode:     AuthAPIKey,
		AuthEnvVar:   "ANTHROPIC_API_KEY",
		AuthMethodID: "anthropic-api-key", // (UNVERIFIED) confirm at live validation
		Capabilities: provider.NewCapabilitySet(
			provider.CapCoding,
			provider.CapToolCalling,
			provider.CapReasoning,
			provider.CapStreaming,
		),
		ForeignAuthVars: []string{"OPENAI_API_KEY", "GEMINI_API_KEY", "GITHUB_TOKEN"},
	}
}

// GeminiCLIDescriptor — Google Gemini CLI in ACP mode.
// (UNVERIFIED) Gemini CLI exposes an experimental ACP mode; the exact adapter
// invocation + auth method id must be confirmed against the real binary.
func GeminiCLIDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         "gemini-cli",
		DefaultPath:  "gemini", // (UNVERIFIED) launched with an ACP flag; see Args
		Args:         []string{"--experimental-acp"},
		AuthMode:     AuthAPIKey,
		AuthEnvVar:   "GEMINI_API_KEY",
		AuthMethodID: "gemini-api-key", // (UNVERIFIED) confirm at live validation
		Capabilities: provider.NewCapabilitySet(
			provider.CapCoding,
			provider.CapToolCalling,
			provider.CapReasoning,
			provider.CapStreaming,
		),
		ForeignAuthVars: []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GITHUB_TOKEN"},
	}
}

// CopilotDescriptor — GitHub Copilot CLI in ACP mode.
// (UNVERIFIED) Copilot's ACP surface + auth (it may use a GitHub token / device
// auth rather than a plain API key) must be confirmed against the real binary;
// the AuthMode below is a placeholder hypothesis.
func CopilotDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         "copilot",
		DefaultPath:  "copilot", // (UNVERIFIED) launched with an ACP flag; see Args
		Args:         []string{"--acp"},
		AuthMode:     AuthAPIKey,
		AuthEnvVar:   "GITHUB_TOKEN",
		AuthMethodID: "github-token", // (UNVERIFIED) confirm at live validation
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

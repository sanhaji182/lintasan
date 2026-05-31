package expprovider

// codex.go — Codex provider DESCRIPTOR (Cohort A, provider #1). The first
// concrete Experimental provider, now expressed as a thin descriptor over the
// Generic ACP Provider Framework (framework.go). Everything mechanical — the
// three harness probes, the admission flow, the membrane wiring, the generic
// constructor — lives in framework.go and is shared by every Cohort-A provider.
// THIS file carries only Codex's IDENTITY: its executable, auth env var + ACP
// authenticate method id, and declared capabilities.
//
// WHAT THIS IS (and is NOT):
//   - It declares CodexDescriptor + the constants that make Codex Codex, then
//     reuses the framework's Admit/harness/probes verbatim.
//   - It is ADDITIVE + DORMANT + MEMBRANE-GATED: admission registers Codex as
//     Track()==Experimental, so production routing (ResolveRoutable) can NEVER
//     select it. It is reached only via the explicit experimental door
//     (ResolveExperimental + the G3 opt-in signal). No proxy hot-path code
//     imports this; importing it changes zero production behavior.
//   - It does NOT flip a flag, deploy, or auto-activate. Onboarding ends at the
//     admission GO verdict + dormant registration. Live activation is a separate,
//     later, gated decision.
//
// ACP SHAPE 2 (no reverse engineering): Codex is driven via its OFFICIAL agent
// surface — `codex-acp` speaks spec ACP over stdio, brokered by the reconciled
// experimental.ACPClient. Lintasan is the ACP host; codex-acp is the ACP agent.
//
// The Codex Shape-1 Responses ingress (/v1/responses) is a SEPARATE, orthogonal
// path (Codex points AT Lintasan as a model endpoint). This file is Shape 2
// (Lintasan drives codex-acp as a subprocess). They share no runtime.

import (
	"context"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// CodexProviderName is the registry key + the experimental/<name> routing prefix.
const CodexProviderName = "codex"

// CodexAuthEnvVar is the environment variable codex-acp reads for the OpenAI API
// key. (codex-acp also accepts CODEX_API_KEY / a ChatGPT subscription, but the
// plain API key is the deterministic staging path — see the onboarding review.)
const CodexAuthEnvVar = "OPENAI_API_KEY"

// CodexAuthMethodID is the ACP `authenticate` method id codex-acp exposes for
// the OPENAI_API_KEY env-var auth method. The broker selects it after
// initialize and BEFORE session/new — codex-acp rejects session/new with
// "Authentication required" until an auth method is selected. (The key itself
// is delivered out-of-band via CodexAuthEnvVar / G4 env injection; this id only
// names WHICH already-present credential codex-acp should use. codex-acp also
// offers "codex-api-key" and "chatgpt"; we pin the OPENAI_API_KEY path.)
const CodexAuthMethodID = "openai-api-key"

// codexDefaultPath is the codex-acp executable, resolved on PATH at launch.
const codexDefaultPath = "codex-acp"

// CodexCapabilities is the DECLARED capability set for Codex. Per Invariant 5
// these are surfaced for display with a risk badge but NEVER trusted for Official
// routing (Codex is not in the routable pool at all). Declared from Codex's
// known surface: coding + tool calling + reasoning + streaming.
func CodexCapabilities() provider.CapabilitySet {
	return provider.NewCapabilitySet(
		provider.CapCoding,
		provider.CapToolCalling,
		provider.CapReasoning,
		provider.CapStreaming,
	)
}

// CodexDescriptor is the declarative definition of the Codex provider. It is the
// ONLY Codex-specific input the framework needs; everything else is generic.
// Onboarding another provider means writing a sibling of this — no new flow.
func CodexDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         CodexProviderName,
		DefaultPath:  codexDefaultPath,
		AuthMode:     AuthAPIKey,
		AuthEnvVar:   CodexAuthEnvVar,
		AuthMethodID: CodexAuthMethodID,
		Capabilities: CodexCapabilities(),
		// Foreign secrets the isolation gate proves do NOT leak into Codex's env.
		ForeignAuthVars: []string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GITHUB_TOKEN"},
	}
}

// --- Backward-compatible Codex-named wrappers ---------------------------------
//
// These keep the original Codex API stable (existing tests + the live test call
// them) while delegating to the generic framework. They are thin sugar: the
// engine is framework.go.

// CodexLaunchSpec returns the LaunchSpec for the Codex ACP adapter. path
// overrides the default codex-acp executable when non-empty; args overrides the
// descriptor args; baseEnv is the NON-secret child environment (the secret is
// injected by G4 at launch, never placed in baseEnv).
func CodexLaunchSpec(path string, args []string, baseEnv []string) LaunchSpec {
	return CodexDescriptor().LaunchSpec(path, args, baseEnv)
}

// NewCodexProvider builds the Codex ACPProvider from a spec + credential source.
func NewCodexProvider(spec LaunchSpec, src CredentialSource) *ACPProvider {
	return NewExperimentalProvider(spec, CodexCapabilities(), src)
}

// CodexHarness builds the admission harness for Codex (delegates to the generic
// harness with Codex's foreign-secret set).
func CodexHarness(foreignAuthVars []string) *Harness {
	return ExperimentalHarness(foreignAuthVars)
}

// AdmitCodex runs the full admission flow for Codex. It delegates to the generic
// AdmitProvider; the only Codex-specific inputs are the spec (Codex's
// LaunchSpec) and its declared capabilities. A non-GO report is NOT an error.
func AdmitCodex(ctx context.Context, reg *provider.Registry, spec LaunchSpec, src CredentialSource, foreignAuthVars []string) (*ACPProvider, *Record, Report, error) {
	return AdmitProvider(ctx, reg, spec, CodexCapabilities(), src, foreignAuthVars)
}

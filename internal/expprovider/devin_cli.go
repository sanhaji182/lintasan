package expprovider

import (
	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// devin_cli.go — Cohort-A descriptor for Devin CLI, ported from
// 9router open-sse/executors/devin-cli.js (v0.5.104). Lintasan ships Devin
// through the EXISTING ACP framework (G3) rather than a bespoke executor: the
// 9router executor's own header says "speaks the Agent Client Protocol (ACP)
// JSON-RPC 2.0 over stdio" — which is exactly what this framework drives, so
// re-implementing the translate/spawn layer would double a translator the
// substrate already covers.
//
// DEVIATIONS from the 9router executor, on purpose:
//   - Binary discovery. 9router walks CLI_DEVIN_BIN → PATH → three known
//     installer paths (LOCALAPPDATA/devin, /usr/local/bin, brew cask). The
//     Cohort-A LaunchSpec contract handles this once via LaunchSpec + a PATH
//     lookup at spawn — that machinery is already validated by Codex/Claude
//     Code/Copilot and is not re-solved per provider.
//   - MCP tool pre-seeding (DEVIN_MCP_TOOLS / DEVIN_MCP_RESULTS env, so the
//     spawned Devin can call the client's tool-call results back). Not ported
//     here: Lintasan's proxy surfaces tools natively and does not need the
//     bridged-MCP shim. When a use-case needs pre-seeded results, it goes
//     through the clientTools side out-of-band, not through this descriptor.
//   - Workspace cwd (CLI_DEVIN_WORKSPACE_CWD) → ProviderDescriptor.WorkDir is
//     per-provider only at the descriptor level; Lintasan does not advertise a
//     per-request cwd the way the 9router executor does (it remembers the cwd
//     from the message env block). WorkDir="" means "spawn in whatever the
//     caller has".

// DevinCLIDescriptor — Devin CLI via native `devin acp` mode.
// VALIDATED (from 9router source): spawn argument `acp` is the flag that puts
// Devin in Agent Client Protocol / stdio mode; `--agent-type` overrides the
// default full-tool agent (fs/shell/search).
// Auth model: none (authType "none" in the registry, authModes ["none"]) —
// Devin CLI is signed in via `devin auth login` once and the CLI carries the
// session itself, there is no env-var credential to inject. Therefore this
// descriptor uses AuthMode=AuthNone and leaves AuthEnvVar empty so the G4
// injector builds an unchanged env (see IsolationProbe, which returns the
// BaseEnv unmodified for AuthNone candidates).
func DevinCLIDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		Name:         "devin-cli",
		DefaultPath:  "devin",         // VALIDATED: `curl -fsSL https://cli.devin.ai/install.sh | bash` — the binary is `devin`
		Args:         []string{"acp"}, // VALIDATED: 9router spawns ["devin", "acp"]; --agent-type is CLI_DEVIN_AGENT_TYPE overridable, not part of the default invocation
		AuthMode:     AuthNone,
		AuthEnvVar:   "", // AuthNone: G4 requires nothing, and IsolationProbe with AuthMode=AuthNone does not check an env var
		AuthMethodID: "", // VALIDATED: none offered — authModes=["none"]; the descriptor leaves this blank so the ACP auth-method probe does not mistake it for a not-yet-authed flow
		Capabilities: provider.NewCapabilitySet(
			provider.CapCoding,
			provider.CapToolCalling,
			provider.CapStreaming,
		),
		ForeignAuthVars: []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY"}, // isolation gate proves no other provider's secret leaks in
	}
}

// SummarizerDevinCLIDescriptor is the 9router CLI_DEVIN_AGENT_TYPE=summarizer
// variant: text-only, no tools, no filesystem/shell access. Offered as its own
// descriptor so the tool-less mode is selectable in the dashboard without a
// global env override (and without shipping the DANGEROUS fs/shell variant for
// an operator who only wants a summarizer).
func SummarizerDevinCLIDescriptor() ProviderDescriptor {
	d := DevinCLIDescriptor()
	d.Name = "devin-cli-summarizer"
	d.Args = []string{"acp", "--agent-type", "summarizer"}
	// Summarizer takes no tools and writes nothing, so CapToolCalling is
	// deliberately absent (declared-caps, not trusted for routing — but the
	// CapCoding surface is also dropped because the summarizer is an OpText
	// mode, not a coding agent).
	d.Capabilities = provider.NewCapabilitySet(
		provider.CapStreaming,
	)
	return d
}

package expprovider

// codex.go — Codex onboarding (Cohort A, provider #1). The FIRST concrete
// Experimental provider built on the live foundation (reconciled ACP broker, E1
// isolation, membrane, F2.4 live, M5 acceptance principle).
//
// WHAT THIS IS (and is NOT):
//   - It assembles the Codex ACPProvider: a LaunchSpec for the `codex-acp` adapter
//     (Zed's Apache-2.0 ACP wrapper around the Codex CLI), the three real harness
//     probes (Isolation/Protocol/Acceptance), and the admission + wiring flow.
//   - It is ADDITIVE + DORMANT + MEMBRANE-GATED: registering the Codex provider
//     puts it in the registry as Track()==Experimental, so production routing
//     (ResolveRoutable) can NEVER select it. It is reached only via the explicit
//     experimental door (ResolveExperimental + the G3 opt-in signal). No proxy
//     hot-path code imports this; importing it changes zero production behavior.
//   - It does NOT flip a flag, deploy, or auto-activate. Onboarding ends at the
//     admission GO verdict + dormant registration. Live activation (opt-in routing
//     to the agent) is a separate, later, gated decision.
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
	"fmt"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/experimental"
	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// CodexProviderName is the registry key + the experimental/<name> routing prefix.
const CodexProviderName = "codex"

// CodexAuthEnvVar is the environment variable codex-acp reads for the OpenAI API
// key. (codex-acp also accepts CODEX_API_KEY / a ChatGPT subscription, but the
// plain API key is the deterministic staging path — see the onboarding review.)
const CodexAuthEnvVar = "OPENAI_API_KEY"

// CodexLaunchSpec returns the LaunchSpec for the Codex ACP adapter. baseEnv is
// the NON-secret child environment (PATH/HOME/etc.); the secret is injected by
// G4 at launch, never placed in baseEnv. path is the codex-acp executable
// (e.g. "codex-acp" on PATH, or an absolute path); args are its launch flags.
//
// Defaults chosen for an agent turn (which can run a multi-step tool loop):
// generous request/start timeouts so a real tool loop is not killed mid-flight,
// while still bounded so a hung agent is contained by E1.
func CodexLaunchSpec(path string, args []string, baseEnv []string) LaunchSpec {
	if strings.TrimSpace(path) == "" {
		path = "codex-acp" // resolved on PATH at launch
	}
	return LaunchSpec{
		Name:           CodexProviderName,
		Protocol:       ProtocolACP,
		Path:           path,
		Args:           args,
		AuthMode:       AuthAPIKey,
		AuthEnvVar:     CodexAuthEnvVar,
		BaseEnv:        baseEnv,
		StartTimeout:   30 * time.Second,
		RequestTimeout: 120 * time.Second, // a tool-loop turn can be long
		StopTimeout:    5 * time.Second,
	}
}

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

// NewCodexProvider builds the Codex ACPProvider from a spec + credential source.
// The provider is Track()==Experimental (membrane-gated) and exposes the Agent
// interface (Run) for the acceptance gate + later opt-in driving.
func NewCodexProvider(spec LaunchSpec, src CredentialSource) *ACPProvider {
	return NewACPProvider(spec, CodexCapabilities(), NewInjector(src))
}

// --- Harness probes (the three deferred gates, now implemented for Codex) -----
//
// The substrate ships Isolation/Protocol/Acceptance as fail-closed `notImplemented`
// stubs (harness.go), so no provider can be admitted before its real probes
// exist. These three close that gap for Codex. They are provider-agnostic in
// spirit (they drive the shared broker), so the same probe bodies will serve the
// rest of Cohort A — the only Codex-specific input is the LaunchSpec/credential.

// IsolationProbe verifies the containment invariants (2,3,4) for an ACP candidate:
//   - Credential scoping (Invariant 3): BuildEnv injects EXACTLY the candidate's
//     own auth var and no foreign secret. We build the env via the candidate's
//     injector and assert the auth var is present (for api_key/oauth) and that no
//     OTHER known provider auth var leaked in.
//   - No dark egress / no core-store handle (Invariant 4): the adapter holds only
//     a CredentialSource indirection + a LaunchSpec — structurally it cannot reach
//     internal/auth. We assert the spec carries no secret in BaseEnv (the G4
//     guard) — a spec that bakes a secret fails here.
//   - Process containment (Invariant 2) is provided by E1 and exercised by the
//     Protocol/Acceptance probes (a crash/hang surfaces as a contained error);
//     this probe asserts the static credential-isolation preconditions.
func IsolationProbe(foreignAuthVars []string) func(ctx context.Context, c Candidate) (GateOutcome, string) {
	return func(ctx context.Context, c Candidate) (GateOutcome, string) {
		if c.Adapter == nil {
			return GateFail, "no adapter on candidate"
		}
		spec := c.Spec
		// G4 guard: BaseEnv must not pre-set the auth var (no baked secret).
		prefix := spec.AuthEnvVar + "="
		for _, kv := range spec.BaseEnv {
			if spec.AuthEnvVar != "" && strings.HasPrefix(kv, prefix) {
				return GateFail, "BaseEnv bakes the auth secret (Invariant 3 violation)"
			}
		}
		// Build the final child env through the adapter's injector and assert
		// scoping: the candidate's own var present; no foreign provider var.
		env, err := c.Adapter.injector.BuildEnv(spec)
		if err != nil {
			return GateFail, "credential injection failed: " + err.Error()
		}
		if spec.AuthMode != AuthNone {
			found := false
			for _, kv := range env {
				if strings.HasPrefix(kv, prefix) {
					found = true
					break
				}
			}
			if !found {
				return GateFail, "injected env missing the candidate's auth var " + spec.AuthEnvVar
			}
		}
		for _, fv := range foreignAuthVars {
			if fv == "" || fv == spec.AuthEnvVar {
				continue
			}
			fprefix := fv + "="
			for _, kv := range env {
				if strings.HasPrefix(kv, fprefix) {
					return GateFail, "foreign secret leaked into child env: " + fv
				}
			}
		}
		return GatePass, "credential scoped to " + spec.AuthEnvVar + "; no foreign secret; no baked secret"
	}
}

// ProtocolProbe drives the candidate's adapter through a REAL ACP prompt turn via
// the reconciled broker and asserts handshake honesty + stream drain + permission
// round-trip + terminal honesty. It is the mechanical Protocol Gate:
//   - Run completes (initialize → session/new → session/prompt stream → stopReason).
//   - terminal honesty: a non-empty StopReason is returned (the stream did not end
//     silently — the M4/ACP "never end silently" principle).
//   - identifier fidelity: every toolCallId the agent reported is non-empty and was
//     tracked verbatim (the broker enforces this; the probe asserts the result).
//
// It uses a granting permission handler so any tool the agent reports can proceed.
// The probe is bounded by ctx so a misbehaving agent is contained.
func ProtocolProbe(ctx context.Context, c Candidate) (GateOutcome, string) {
	if c.Adapter == nil {
		return GateFail, "no adapter on candidate"
	}
	turnCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	res, err := c.Adapter.Run(turnCtx, AgentTurn{
		Prompt:       map[string]any{"text": "protocol gate: respond and (optionally) use a tool"},
		OnPermission: grantingPermission,
	})
	if err != nil {
		return GateFail, "prompt turn did not complete: " + err.Error()
	}
	if strings.TrimSpace(res.StopReason) == "" {
		return GateFail, "terminal honesty FAILED: stream ended without a stopReason"
	}
	for _, id := range res.ToolCalls {
		if strings.TrimSpace(id) == "" {
			return GateFail, "identifier fidelity FAILED: empty toolCallId tracked"
		}
	}
	return GatePass, fmt.Sprintf("handshake+stream OK; stopReason=%q; toolCalls=%d", res.StopReason, len(res.ToolCalls))
}

// AcceptanceProbe is the M5-principle gate: valid ONLY if the TOOL LOOP CLOSES.
// Stream-text-only is NOT acceptance. It drives a real turn and requires:
//   - at least one tool call was reported (the loop actually ran a tool),
//   - identifier fidelity held (every toolCallId non-empty, tracked verbatim),
//   - the turn reached a terminal stopReason (the loop closed, not hung/truncated).
//
// For Codex the live form drives the real codex-acp CLI in staging (operator-run);
// in-process it is exercised by the scripted spec-ACP agent + recorded fixtures
// (which emit a tool_call + permission round-trip), proving the code path closes
// the loop. A NO-GO here blocks admission.
func AcceptanceProbe(ctx context.Context, c Candidate) (GateOutcome, string) {
	if c.Adapter == nil {
		return GateFail, "no adapter on candidate"
	}
	turnCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	res, err := c.Adapter.Run(turnCtx, AgentTurn{
		Prompt:       map[string]any{"text": "acceptance gate: perform a task that requires a tool call"},
		OnPermission: grantingPermission,
	})
	if err != nil {
		return GateFail, "acceptance turn did not complete: " + err.Error()
	}
	if len(res.ToolCalls) == 0 {
		return GateFail, "tool loop did NOT close: no tool call observed (stream-text-only is not acceptance)"
	}
	for _, id := range res.ToolCalls {
		if strings.TrimSpace(id) == "" {
			return GateFail, "identifier fidelity FAILED in the tool loop: empty toolCallId"
		}
	}
	if strings.TrimSpace(res.StopReason) == "" {
		return GateFail, "tool loop did not reach a terminal stopReason (truncated/hung)"
	}
	return GatePass, fmt.Sprintf("tool loop closed: %d tool call(s), stopReason=%q, fidelity OK", len(res.ToolCalls), res.StopReason)
}

// grantingPermission selects an allow option (allow_once preferred) so a reported
// tool call can proceed during the gates; falls back to cancel if none offered.
func grantingPermission(_ context.Context, req experimental.PermissionRequest) experimental.PermissionOutcome {
	for _, o := range req.Options {
		if o.Kind == "allow_once" || o.Kind == "allow_always" {
			return experimental.PermissionOutcome{Outcome: "selected", OptionID: o.OptionID}
		}
	}
	if len(req.Options) > 0 {
		return experimental.PermissionOutcome{Outcome: "selected", OptionID: req.Options[0].OptionID}
	}
	return experimental.PermissionOutcome{Outcome: "cancelled"}
}

// --- Admission + wiring -------------------------------------------------------

// CodexHarness builds the admission harness for Codex with all three real probes
// wired (the membrane gate is live from the skeleton). foreignAuthVars is the set
// of OTHER providers' auth vars the isolation probe asserts do NOT leak in.
func CodexHarness(foreignAuthVars []string) *Harness {
	return NewHarness().
		WithGate(GateIsolation, IsolationProbe(foreignAuthVars)).
		WithGate(GateProtocol, ProtocolProbe).
		WithGate(GateAcceptance, AcceptanceProbe)
}

// AdmitCodex runs the full admission flow for Codex against the supplied registry:
//
//  1. Build the provider, register it (Track==Experimental → membrane-gated).
//  2. Lifecycle proposed → admitted (adapter exists).
//  3. Run the harness (Isolation/Protocol/Acceptance + always-on Membrane). The
//     MembraneCheck closure proves Codex is Experimental-track AND absent from the
//     registry's routable pool.
//  4. On a GO verdict, lifecycle admitted → active. On NO-GO, the provider stays
//     admitted (registered + membrane-gated, but NOT active) and the report says why.
//
// IMPORTANT: "active" here is the lifecycle state meaning "passed admission, may be
// reached via the explicit opt-in door". It is NOT a routing-flag flip and NOT
// production activation — production routing still cannot see Codex (membrane).
// Live opt-in activation is a separate, later, gated decision.
//
// It returns the registered provider, its lifecycle record, and the admission
// report. A non-GO report is NOT an error (it is a valid "not admitted" outcome);
// err is non-nil only for a wiring failure (registration/transition bug).
func AdmitCodex(ctx context.Context, reg *provider.Registry, spec LaunchSpec, src CredentialSource, foreignAuthVars []string) (*ACPProvider, *Record, Report, error) {
	if reg == nil {
		return nil, nil, Report{}, fmt.Errorf("expprovider: nil registry")
	}
	if spec.Name != CodexProviderName {
		return nil, nil, Report{}, fmt.Errorf("expprovider: spec name %q is not %q", spec.Name, CodexProviderName)
	}

	p := NewCodexProvider(spec, src)

	// Step 1: register (membrane-gated by Track()==Experimental).
	if err := reg.Register(p); err != nil {
		return nil, nil, Report{}, fmt.Errorf("expprovider: register codex: %w", err)
	}

	// Step 2: lifecycle proposed → admitted.
	rec := NewRecord(CodexProviderName)
	if err := rec.Transition(StateAdmitted); err != nil {
		return p, rec, Report{}, fmt.Errorf("expprovider: codex proposed→admitted: %w", err)
	}

	// Step 3: run the harness. MembraneCheck proves Experimental-track + absence
	// from the routable pool (defense-in-depth, always-on).
	cand := Candidate{
		Provider: CodexProviderName,
		Adapter:  p,
		Spec:     spec,
		MembraneCheck: func() (bool, string) {
			if p.Track() != provider.TrackExperimental {
				return false, "codex is not Experimental-track"
			}
			for _, n := range reg.RoutableProviders() {
				if n == CodexProviderName {
					return false, "codex leaked into the routable (Official) pool"
				}
			}
			return true, "Experimental-track and absent from routable pool"
		},
	}
	rep := CodexHarness(foreignAuthVars).Run(ctx, cand)

	// Step 4: admitted → active ONLY on a GO verdict.
	if rep.Go() {
		if err := rec.Transition(StateActive); err != nil {
			return p, rec, rep, fmt.Errorf("expprovider: codex admitted→active: %w", err)
		}
	}
	return p, rec, rep, nil
}

package expprovider

// framework.go — Generic ACP Provider Framework (Cohort A engine).
//
// WHY THIS EXISTS: Codex onboarding proved the full path (LaunchSpec →
// credential injection → admission harness → membrane-gated registration) end
// to end against a real ACP agent. Everything in that path EXCEPT the Codex
// identity (its executable name, auth env var, ACP authenticate method id, and
// declared capabilities) is provider-agnostic. This file extracts that engine so
// onboarding provider N+1 (Claude Code, Gemini CLI, Copilot, …) is a DESCRIPTOR
// + a fixture, NOT new architecture.
//
// THE CONTRACT FOR A NEW PROVIDER (no framework change required):
//  1. Declare a ProviderDescriptor (name, executable, auth env var + ACP method
//     id, declared capabilities, the set of OTHER providers' auth vars the
//     isolation gate must prove do NOT leak in).
//  2. Provide a fixture (recorded spec-ACP frame sequence) for the in-process
//     Protocol/Acceptance gates, OR run the operator-gated live acceptance test
//     against the real CLI.
//  3. Call ProviderDescriptor.Admit (or AdmitProvider with an explicit spec).
//
// SAFETY (unchanged): every provider admitted through this framework is
// Track()==Experimental → membrane-gated → never production-routable. Admission
// ends at a GO verdict + dormant registration. No flag flip, no deploy, no
// activation, no proxy hot-path wiring. These are structural properties of the
// substrate (membrane + lifecycle), not promises this file makes.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/experimental"
	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// Framework defaults for an agent turn (which can run a multi-step tool loop):
// generous request/start timeouts so a real tool loop is not killed mid-flight,
// while still bounded so a hung agent is contained by E1.
const (
	defaultStartTimeout   = 30 * time.Second
	defaultRequestTimeout = 120 * time.Second
	defaultStopTimeout    = 5 * time.Second
)

// ProviderDescriptor is the DECLARATIVE definition of one Experimental ACP
// provider. It is the ONLY per-provider input the framework needs: everything
// else (the broker, the gates, the admission flow, the membrane wiring) is
// shared. Adding a provider = adding one of these + a fixture.
//
// Fields map 1:1 onto the wire/admission contract Codex established:
//   - Name           → registry key + experimental/<name> routing prefix.
//   - DefaultPath     → the ACP adapter executable (resolved on PATH at launch).
//   - Args            → its launch flags (e.g. ["--acp", "--stdio"]).
//   - AuthMode        → credential injection mode (G4).
//   - AuthEnvVar      → env var the secret is injected into (e.g. OPENAI_API_KEY).
//   - AuthMethodID    → ACP `authenticate` method id selected before session/new
//     (empty = the agent needs no explicit auth selection).
//   - Capabilities    → DECLARED set (Invariant 5: display-only, never trusted
//     for routing).
//   - ForeignAuthVars → OTHER providers' auth vars the isolation gate proves do
//     NOT leak into this provider's child env.
//   - WorkDir         → cwd advertised to session/new (spec agents require it).
//   - *Timeout        → subprocess bounds; zero falls back to framework defaults.
type ProviderDescriptor struct {
	Name            string
	DefaultPath     string
	Args            []string
	AuthMode        AuthMode
	AuthEnvVar      string
	AuthMethodID    string
	Capabilities    provider.CapabilitySet
	ForeignAuthVars []string
	WorkDir         string
	StartTimeout    time.Duration
	RequestTimeout  time.Duration
	StopTimeout     time.Duration
}

// LaunchSpec builds the G2 LaunchSpec for this descriptor. path overrides
// DefaultPath when non-empty (e.g. an absolute path, or a test re-exec path);
// args overrides Args when non-nil. baseEnv is the NON-secret child environment
// (PATH/HOME/test mode); the secret is injected by G4 at launch, never here.
func (d ProviderDescriptor) LaunchSpec(path string, args []string, baseEnv []string) LaunchSpec {
	if strings.TrimSpace(path) == "" {
		path = d.DefaultPath
	}
	if args == nil {
		args = d.Args
	}
	start, request, stop := d.StartTimeout, d.RequestTimeout, d.StopTimeout
	if start == 0 {
		start = defaultStartTimeout
	}
	if request == 0 {
		request = defaultRequestTimeout
	}
	if stop == 0 {
		stop = defaultStopTimeout
	}
	return LaunchSpec{
		Name:           d.Name,
		Protocol:       ProtocolACP,
		Path:           path,
		Args:           args,
		AuthMode:       d.AuthMode,
		AuthEnvVar:     d.AuthEnvVar,
		AuthMethodID:   d.AuthMethodID,
		BaseEnv:        baseEnv,
		WorkDir:        d.WorkDir,
		StartTimeout:   start,
		RequestTimeout: request,
		StopTimeout:    stop,
	}
}

// Admit is the one-call onboarding for a descriptor: build the spec from baseEnv
// and run the full admission flow. It is sugar over AdmitProvider for the common
// case where the caller does not need to customize the spec further.
func (d ProviderDescriptor) Admit(ctx context.Context, reg *provider.Registry, baseEnv []string, src CredentialSource) (*ACPProvider, *Record, Report, error) {
	spec := d.LaunchSpec("", nil, baseEnv)
	return AdmitProvider(ctx, reg, spec, d.Capabilities, src, d.ForeignAuthVars)
}

// NewExperimentalProvider builds an ACPProvider from a spec + declared caps +
// credential source. It is the generic constructor every provider uses; the
// provider is Track()==Experimental (membrane-gated) and exposes Agent.Run.
func NewExperimentalProvider(spec LaunchSpec, caps provider.CapabilitySet, src CredentialSource) *ACPProvider {
	return NewACPProvider(spec, caps, NewInjector(src))
}

// --- Generic harness probes (provider-agnostic; they drive the shared broker) -
//
// These three close the harness's deferred gates for ANY ACP provider. They take
// no provider identity beyond what the Candidate carries (its adapter + spec),
// so the same bodies serve every Cohort-A provider. The only per-provider input
// is the LaunchSpec/credential the Candidate was built with + the foreign-secret
// set handed to IsolationProbe.

// IsolationProbe verifies the containment invariants (2,3,4) for an ACP candidate:
//   - Credential scoping (Invariant 3): BuildEnv injects EXACTLY the candidate's
//     own auth var and no foreign secret.
//   - No baked secret (Invariant 3): the spec's BaseEnv must not pre-set the auth
//     var.
//   - No dark egress / no core-store handle (Invariant 4): the adapter holds only
//     a CredentialSource indirection + a LaunchSpec — structurally it cannot reach
//     internal/auth.
//   - Process containment (Invariant 2) is provided by E1 and exercised by the
//     Protocol/Acceptance probes; this probe asserts the static preconditions.
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
//   - Run completes (initialize → authenticate → session/new → session/prompt
//     stream → stopReason).
//   - terminal honesty: a non-empty StopReason is returned (the stream did not
//     end silently — the M4/ACP "never end silently" principle).
//   - identifier fidelity: every toolCallId the agent reported is non-empty and
//     was tracked verbatim (the broker enforces this; the probe asserts it).
//
// It uses a granting permission handler so any tool the agent reports can
// proceed. The probe is bounded by ctx so a misbehaving agent is contained.
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
// The live form drives the real CLI in staging (operator-run); in-process it is
// exercised by the recorded spec-ACP fixture (which emits a tool_call +
// permission round-trip), proving the code path closes the loop. NO-GO blocks
// admission.
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

// ExperimentalHarness builds the admission harness with all three real probes
// wired (the membrane gate is live from the skeleton). foreignAuthVars is the
// set of OTHER providers' auth vars the isolation probe asserts do NOT leak in.
// This is provider-agnostic: the same harness serves every Cohort-A provider.
func ExperimentalHarness(foreignAuthVars []string) *Harness {
	return NewHarness().
		WithGate(GateIsolation, IsolationProbe(foreignAuthVars)).
		WithGate(GateProtocol, ProtocolProbe).
		WithGate(GateAcceptance, AcceptanceProbe)
}

// AdmitProvider runs the full, GENERIC admission flow for ANY ACP provider:
//
//  1. Build the provider, register it (Track==Experimental → membrane-gated).
//  2. Lifecycle proposed → admitted (adapter exists).
//  3. Run the harness (Isolation/Protocol/Acceptance + always-on Membrane). The
//     MembraneCheck closure proves the provider is Experimental-track AND absent
//     from the registry's routable pool.
//  4. On a GO verdict, lifecycle admitted → active. On NO-GO, the provider stays
//     admitted (registered + membrane-gated, but NOT active) and the report says
//     why.
//
// IMPORTANT: "active" here is the lifecycle state meaning "passed admission, may
// be reached via the explicit opt-in door". It is NOT a routing-flag flip and
// NOT production activation — production routing still cannot see the provider
// (membrane). Live opt-in activation is a separate, later, gated decision.
//
// spec.Name is the registry key. caps is the DECLARED capability set. A non-GO
// report is NOT an error (it is a valid "not admitted" outcome); err is non-nil
// only for a wiring failure (registration/transition bug).
func AdmitProvider(ctx context.Context, reg *provider.Registry, spec LaunchSpec, caps provider.CapabilitySet, src CredentialSource, foreignAuthVars []string) (*ACPProvider, *Record, Report, error) {
	if reg == nil {
		return nil, nil, Report{}, fmt.Errorf("expprovider: nil registry")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return nil, nil, Report{}, fmt.Errorf("expprovider: launch spec has empty name")
	}

	p := NewExperimentalProvider(spec, caps, src)

	// Step 1: register (membrane-gated by Track()==Experimental).
	if err := reg.Register(p); err != nil {
		return nil, nil, Report{}, fmt.Errorf("expprovider: register %s: %w", name, err)
	}

	// Step 2: lifecycle proposed → admitted.
	rec := NewRecord(name)
	if err := rec.Transition(StateAdmitted); err != nil {
		return p, rec, Report{}, fmt.Errorf("expprovider: %s proposed→admitted: %w", name, err)
	}

	// Step 3: run the harness. MembraneCheck proves Experimental-track + absence
	// from the routable pool (defense-in-depth, always-on).
	cand := Candidate{
		Provider: name,
		Adapter:  p,
		Spec:     spec,
		MembraneCheck: func() (bool, string) {
			if p.Track() != provider.TrackExperimental {
				return false, name + " is not Experimental-track"
			}
			for _, n := range reg.RoutableProviders() {
				if n == name {
					return false, name + " leaked into the routable (Official) pool"
				}
			}
			return true, "Experimental-track and absent from routable pool"
		},
	}
	rep := ExperimentalHarness(foreignAuthVars).Run(ctx, cand)

	// Step 4: admitted → active ONLY on a GO verdict.
	if rep.Go() {
		if err := rec.Transition(StateActive); err != nil {
			return p, rec, rep, fmt.Errorf("expprovider: %s admitted→active: %w", name, err)
		}
	}
	return p, rec, rep, nil
}

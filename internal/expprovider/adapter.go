package expprovider

// G1 — Experimental adapter seam.
//
// ACPProvider models an Experimental ACP agent as a provider.Provider so it is
// registry-resident and, crucially, gated by the Phase-2 membrane: its Track()
// returns TrackExperimental, so the production resolver (ResolveRoutable /
// RoutableProviders) can NEVER select it. It is reachable ONLY via the explicit
// experimental door (ResolveExperimental + the G3 opt-in signal).
//
// Two surfaces, deliberately separated:
//
//   - provider.Provider (Name/Track/Capabilities/Prepare/Translate): satisfied
//     so the adapter lives in the registry and is membrane-gated. The HTTP
//     Prepare/Translate path is NOT how an agent runs — an ACP agent is driven
//     over stdio, not a single HTTP call — so those methods return a loud
//     ErrUseAgentInterface sentinel. This makes "someone routed an agent down
//     the HTTP proxy path" a visible, contained error instead of silent wrong
//     behavior.
//
//   - Agent (Run): the REAL execution surface for an ACP agent — a prompt turn
//     with tool round-trips, brokered by experimental.ACPClient over an E1
//     experimental.Subprocess. The onboarding step (later, per provider) wires
//     a host ToolHandler and calls Run.
//
// SAFETY: this is substrate. ACPProvider holds a LaunchSpec + an Injector + a
// CredentialSource indirection; it never holds raw secrets and never self-
// egresses (Invariant 4 — the agent's upstream calls are the agent's own
// process; the host instruments the subprocess boundary). No concrete provider
// is constructed here.

import (
	"context"
	"errors"

	"github.com/sanhaji182/lintasan-go/internal/experimental"
	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// Adapter-seam errors.
var (
	// ErrUseAgentInterface is returned by Prepare/Translate to signal that an
	// ACP agent must be driven via the Agent interface (Run), not the HTTP
	// provider path. It is a contained, loud failure — never silent.
	ErrUseAgentInterface = errors.New("expprovider: ACP agent must be driven via Agent.Run, not the HTTP Prepare/Translate path")
	// ErrNotStarted is returned when Run is called before the agent subprocess
	// has been started.
	ErrNotStarted = errors.New("expprovider: agent not started")
)

// AgentTurn is a single prompt turn request to an ACP agent.
type AgentTurn struct {
	// SessionParams is passed to session/new (provider-specific; may be nil).
	SessionParams any
	// Prompt is the turn payload passed to session/prompt.
	Prompt any
	// OnPermission is the host-side permission handler. The agent reports + runs
	// its own tools (ACP Shape 2); the host only consents via
	// session/request_permission. A nil handler denies (the broker selects a
	// reject option or cancels), so the turn always terminates cleanly.
	OnPermission experimental.PermissionHandler
}

// Agent is the execution surface for an Experimental ACP provider. It is an
// OPTIONAL interface (Go-idiomatic, mirrors provider.Embedder/StreamTranslator):
// the onboarding code type-asserts a registry provider to Agent to drive it.
type Agent interface {
	// Run starts (if needed) the agent subprocess, performs the ACP lifecycle
	// (initialize -> session/new -> session/prompt with tool round-trips), and
	// returns the terminal prompt result. A crash/hang/panic in the agent is
	// contained by E1 and surfaced as an error, never a gateway panic.
	Run(ctx context.Context, turn AgentTurn) (*experimental.PromptResult, error)
	// StopAgent tears down the subprocess (idempotent).
	StopAgent() error
}

// ACPProvider is the Experimental ACP adapter. It is safe for concurrent use at
// the provider-registry level (Name/Track/Capabilities are immutable); Run
// serializes through the underlying Subprocess (one in-flight request per
// child — pool multiple ACPProviders for concurrency, per the E1 contract).
type ACPProvider struct {
	spec     LaunchSpec
	caps     provider.CapabilitySet
	injector *Injector

	// initVersion is the ACP protocol version offered at initialize.
	initVersion int

	// proc/client are lazily constructed on first Run; nil until started.
	proc   *experimental.Subprocess
	client *experimental.ACPClient
}

// NewACPProvider builds an Experimental ACP provider from a launch spec, a
// declared capability set (declaration only — never trusted for Official
// routing, Invariant 5), and a credential injector. It constructs no subprocess
// yet (lazy on first Run).
func NewACPProvider(spec LaunchSpec, caps provider.CapabilitySet, injector *Injector) *ACPProvider {
	return &ACPProvider{
		spec:        spec,
		caps:        caps,
		injector:    injector,
		initVersion: experimental.CurrentProtocolVersion,
	}
}

// --- provider.Provider implementation ---------------------------------------

// Name is the registry key (matches the LaunchSpec.Name and the
// experimental/<name> routing prefix).
func (p *ACPProvider) Name() string { return p.spec.Name }

// Track is ALWAYS Experimental. This is the load-bearing membrane property:
// production routing (ResolveRoutable) filters on TrackOfficial, so an
// ACPProvider can never be selected by the default pool.
func (p *ACPProvider) Track() provider.Track { return provider.TrackExperimental }

// Capabilities returns the DECLARED set. Per Invariant 5 these are surfaced for
// display with a risk badge but never trusted for Official routing.
func (p *ACPProvider) Capabilities() provider.CapabilitySet { return p.caps }

// Prepare is intentionally unsupported: an ACP agent is not a single HTTP call.
// Returns ErrUseAgentInterface so a mis-route is loud and contained.
func (p *ACPProvider) Prepare(ctx context.Context, req *provider.Request, conn *provider.ConnConfig) (*provider.UpstreamRequest, error) {
	return nil, ErrUseAgentInterface
}

// Translate is intentionally unsupported for the same reason as Prepare.
func (p *ACPProvider) Translate(ctx context.Context, raw []byte, req *provider.Request) (*provider.Response, error) {
	return nil, ErrUseAgentInterface
}

// --- Agent implementation ----------------------------------------------------

// start lazily launches the subprocess + ACP client with the credential-
// injected environment. Caller holds no lock; Subprocess serializes internally.
func (p *ACPProvider) start(ctx context.Context) error {
	if p.proc != nil && p.proc.Running() {
		return nil
	}
	env, err := p.injector.BuildEnv(p.spec)
	if err != nil {
		return err
	}
	proc := experimental.New(p.spec.toSubprocessConfig(env))
	client := experimental.NewACPClient(proc)
	if err := client.Start(ctx); err != nil {
		return err
	}
	if _, err := client.Initialize(ctx, experimental.InitializeParams{
		ProtocolVersion: p.initVersion,
		// Advertise NO optional client capabilities (no fs, no terminal). Per the
		// ACP spec, a conformant agent is then forbidden from calling fs/* or
		// terminal/* — which keeps client-method handling out of scope for the
		// minimum onboarding-ready broker.
		ClientCapabilities: experimental.ClientCapabilities{},
		ClientInfo:         map[string]any{"name": "lintasan", "role": "host"},
	}); err != nil {
		_ = client.Close()
		return err
	}
	p.proc = proc
	p.client = client
	return nil
}

// Run performs one prompt turn against the agent, starting it on first use.
func (p *ACPProvider) Run(ctx context.Context, turn AgentTurn) (*experimental.PromptResult, error) {
	if err := p.start(ctx); err != nil {
		return nil, err
	}
	if _, err := p.client.NewSession(ctx, turn.SessionParams); err != nil {
		return nil, err
	}
	return p.client.Prompt(ctx, experimental.PromptParams{Prompt: turn.Prompt}, turn.OnPermission)
}

// StopAgent tears down the subprocess. Idempotent; safe if never started.
func (p *ACPProvider) StopAgent() error {
	if p.client == nil {
		return nil
	}
	return p.client.Close()
}

// compile-time assertions: ACPProvider satisfies both contracts.
var (
	_ provider.Provider = (*ACPProvider)(nil)
	_ Agent             = (*ACPProvider)(nil)
)

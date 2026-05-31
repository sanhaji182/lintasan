package expprovider

// G2 — Launcher registry.
//
// A LaunchSpec is the provider-agnostic description of HOW to launch an
// Experimental agent CLI as an E1 subprocess: the executable, its args, the
// auth mode, and the protocol it speaks. It is pure data — no secrets live here
// (the actual credential is injected separately by G4, post-prepare, so this
// registry can be logged/inspected safely).
//
// The LauncherRegistry maps a provider name -> LaunchSpec. Onboarding a concrete
// provider later is "add one LaunchSpec" — no code change to the seam.

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/experimental"
)

// Protocol is how the host talks to the launched agent.
type Protocol string

const (
	// ProtocolACP — JSON-RPC 2.0 over stdio, brokered by experimental.ACPClient.
	// The Tier-1/2 path (Codex/Claude Code/Gemini CLI/Copilot).
	ProtocolACP Protocol = "acp"
)

// AuthMode describes how the agent authenticates upstream. The mode selects how
// G4 injects the credential into the child environment; the secret VALUE is
// never stored in the spec.
type AuthMode string

const (
	// AuthAPIKey — a single API key passed via an environment variable.
	AuthAPIKey AuthMode = "api_key"
	// AuthOAuth — an OAuth access token passed via an environment variable
	// (refresh is the provider's CredentialRefresher concern, not the spec's).
	AuthOAuth AuthMode = "oauth"
	// AuthNone — the agent manages its own auth out-of-band (e.g. a prior CLI
	// login). The host injects no credential.
	AuthNone AuthMode = "none"
)

// LaunchSpec is the immutable launch description for one Experimental provider.
type LaunchSpec struct {
	// Name is the registry key (e.g. "codex"). Matches the ACPProvider name and
	// the experimental/<name> routing prefix.
	Name string
	// Protocol the agent speaks (currently ACP only).
	Protocol Protocol
	// Path is the agent executable (e.g. "codex"). Resolved on PATH at launch.
	Path string
	// Args are the launch arguments (e.g. ["--acp", "--stdio"]).
	Args []string
	// AuthMode selects credential injection behavior (G4).
	AuthMode AuthMode
	// AuthEnvVar is the environment variable the credential is injected into
	// when AuthMode is api_key/oauth (e.g. "OPENAI_API_KEY"). Empty for AuthNone.
	AuthEnvVar string
	// AuthMethodID is the ACP `authenticate` method id to select after
	// initialize, BEFORE session/new (e.g. "openai-api-key" for codex-acp). A
	// spec-faithful agent rejects session/new with "Authentication required"
	// until authenticate succeeds. Empty means the agent needs no explicit auth
	// selection (the broker skips the authenticate step). The secret itself is
	// still delivered out-of-band via AuthEnvVar (G4 env injection); AuthMethodID
	// only tells the agent WHICH already-present credential to use.
	AuthMethodID string
	// BaseEnv is the NON-secret environment the child needs (e.g. PATH, HOME,
	// flags). Secrets are NEVER placed here — G4 adds the credential separately
	// at launch time. The child sees EXACTLY BaseEnv + the injected credential.
	BaseEnv []string
	// WorkDir is the working directory advertised to the agent in the spec ACP
	// session/new params (`cwd`). A spec-faithful agent (codex-acp) rejects
	// session/new with -32602 when cwd is absent. Empty means the adapter falls
	// back to the host process's current directory at launch.
	WorkDir string
	// StartTimeout / RequestTimeout / StopTimeout bound the subprocess; zero
	// uses the experimental package defaults.
	StartTimeout   time.Duration
	RequestTimeout time.Duration
	StopTimeout    time.Duration
}

// Validate checks a spec is internally consistent. It does NOT check the
// executable exists on disk (that is the harness's isolation/launch concern) —
// it validates the declaration so a malformed spec is rejected at registration.
func (s LaunchSpec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return ErrSpecNoName
	}
	if strings.TrimSpace(s.Path) == "" {
		return ErrSpecNoPath
	}
	switch s.Protocol {
	case ProtocolACP:
	default:
		return ErrSpecBadProtocol
	}
	switch s.AuthMode {
	case AuthNone:
		// no env var required
	case AuthAPIKey, AuthOAuth:
		if strings.TrimSpace(s.AuthEnvVar) == "" {
			return ErrSpecNoAuthEnvVar
		}
	default:
		return ErrSpecBadAuthMode
	}
	return nil
}

// toSubprocessConfig builds the E1 launch config from the spec plus the already
// credential-injected environment. The caller (G1, via G4) supplies the final
// env so secrets are added at the last moment, never persisted in the spec.
func (s LaunchSpec) toSubprocessConfig(finalEnv []string) experimental.Config {
	return experimental.Config{
		Name:           "exp-" + s.Name,
		Path:           s.Path,
		Args:           append([]string(nil), s.Args...),
		Env:            finalEnv,
		StartTimeout:   s.StartTimeout,
		RequestTimeout: s.RequestTimeout,
		StopTimeout:    s.StopTimeout,
	}
}

// Launcher-registry errors.
var (
	ErrSpecNoName       = errors.New("expprovider: launch spec has empty name")
	ErrSpecNoPath       = errors.New("expprovider: launch spec has empty path")
	ErrSpecBadProtocol  = errors.New("expprovider: launch spec has unknown protocol")
	ErrSpecBadAuthMode  = errors.New("expprovider: launch spec has unknown auth mode")
	ErrSpecNoAuthEnvVar = errors.New("expprovider: launch spec requires auth but has no AuthEnvVar")
	ErrSpecNotFound     = errors.New("expprovider: no launch spec registered for name")
	ErrSpecDuplicate    = errors.New("expprovider: launch spec already registered for name")
)

// LauncherRegistry maps provider name -> LaunchSpec. Safe for concurrent use.
// It is intentionally separate from provider.Registry: that one holds the
// Provider behavior; this one holds the launch data. Onboarding wires the two
// together (a provider.Provider whose Name() matches a LaunchSpec.Name).
type LauncherRegistry struct {
	mu    sync.RWMutex
	specs map[string]LaunchSpec
}

// NewLauncherRegistry returns an empty registry.
func NewLauncherRegistry() *LauncherRegistry {
	return &LauncherRegistry{specs: make(map[string]LaunchSpec)}
}

// Register validates and adds a spec. It rejects duplicates (onboarding should
// be explicit; an accidental double-register is a bug, not an overwrite).
func (r *LauncherRegistry) Register(s LaunchSpec) error {
	if err := s.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.specs[s.Name]; exists {
		return ErrSpecDuplicate
	}
	r.specs[s.Name] = s
	return nil
}

// Get returns the spec for name.
func (r *LauncherRegistry) Get(name string) (LaunchSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.specs[name]
	return s, ok
}

// Names lists registered spec names, sorted, for diagnostics/dashboard.
func (r *LauncherRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.specs))
	for n := range r.specs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Len reports how many specs are registered.
func (r *LauncherRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.specs)
}

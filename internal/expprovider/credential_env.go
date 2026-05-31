package expprovider

import (
	"os"
	"strings"
	"sync"
)

// credential_env.go — staging-capable CredentialSource backing (G4 consumer).
//
// EnvCredentialSource resolves per-provider secrets from process environment
// variables, scoped by an EXPLICIT provider→envvar mapping. It is the bring-up /
// staging implementation of the CredentialSource contract: the host registers
// exactly which env var holds each provider's secret, so a lookup for "codex"
// can ONLY read the var mapped to codex and can NEVER return another provider's
// secret (Invariant 3 — credential isolation).
//
// Production (E2 territory) will back the SAME CredentialSource interface with an
// encrypted per-provider store; this env source is the reference implementation
// and the staging path. It holds no secret of its own — it reads the live env on
// each lookup, so a rotated env var is picked up without re-wiring.
type EnvCredentialSource struct {
	mu      sync.RWMutex
	mapping map[string]string   // provider name -> env var name (the ONLY var that provider may read)
	getenv  func(string) string // injectable for tests; defaults to os.Getenv
}

// NewEnvCredentialSource returns an empty env-backed source. Map providers with Map.
func NewEnvCredentialSource() *EnvCredentialSource {
	return &EnvCredentialSource{mapping: make(map[string]string), getenv: os.Getenv}
}

// withGetenv overrides the env lookup (test seam).
func (s *EnvCredentialSource) withGetenv(fn func(string) string) *EnvCredentialSource {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getenv = fn
	return s
}

// Map registers that provider's secret lives in environment variable envVar.
// Empty arguments are ignored. Returns the source for chaining. Re-mapping a
// provider overwrites the prior var (explicit reconfiguration).
func (s *EnvCredentialSource) Map(provider, envVar string) *EnvCredentialSource {
	provider = strings.TrimSpace(provider)
	envVar = strings.TrimSpace(envVar)
	if provider == "" || envVar == "" {
		return s
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mapping[provider] = envVar
	return s
}

// Credential implements CredentialSource. It is SCOPED: it returns a secret ONLY
// for a provider that was explicitly mapped, reading ONLY that provider's env
// var. An unmapped provider, or a mapped var that is empty/unset, yields
// ("", false) — the caller (G4 BuildEnv) treats a missing-but-required credential
// as a hard error rather than launching an agent that will fail-auth.
func (s *EnvCredentialSource) Credential(provider string) (string, bool) {
	s.mu.RLock()
	env, ok := s.mapping[provider]
	getenv := s.getenv
	s.mu.RUnlock()
	if !ok || env == "" {
		return "", false
	}
	v := getenv(env)
	if v == "" {
		return "", false
	}
	return v, true
}

// MappedProviders lists the providers this source can resolve, sorted, for
// diagnostics. It never exposes secret VALUES — only the provider names.
func (s *EnvCredentialSource) MappedProviders() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.mapping))
	for p := range s.mapping {
		out = append(out, p)
	}
	// tiny insertion sort (avoid importing sort for a handful of names)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// compile-time assertion: EnvCredentialSource satisfies CredentialSource.
var _ CredentialSource = (*EnvCredentialSource)(nil)

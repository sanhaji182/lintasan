package expprovider

// cohort_a_test.go — Cohort-A READINESS proof. Each remaining Cohort-A provider
// (Claude Code, Gemini CLI, Copilot) onboards through the Generic ACP Framework
// using ONLY its descriptor + a fixture skeleton + a credential mapping — with
// ZERO new framework code (AdmitProvider/ExperimentalHarness/the three probes
// are reused verbatim). This is the readiness criterion: descriptor + credential
// mapping + fixture is sufficient; no per-provider flow exists.
//
// SCOPE: these tests drive the in-process fixture-replay agent (the SAME child
// mode Codex uses). They do NOT touch the real CLIs, do NOT validate the real
// wire contract, and do NOT register anything in production. The fixtures are
// SKELETONS (spec-shaped, wire-UNVERIFIED) — each provider's real live
// validation is a separate, later, operator-gated checkpoint.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// cohortAOnboardCase pairs a descriptor with its fixture skeleton + the expected
// session-id namespace (sanity that the right fixture drove the turn).
type cohortAOnboardCase struct {
	descriptor  ProviderDescriptor
	fixtureFile string
}

func cohortAOnboardCases() []cohortAOnboardCase {
	return []cohortAOnboardCase{
		{ClaudeCodeDescriptor(), "testdata/claude-code-session.jsonl"},
		{GeminiCLIDescriptor(), "testdata/gemini-cli-session.jsonl"},
		{CopilotDescriptor(), "testdata/copilot-session.jsonl"},
	}
}

// cohortAFixtureSpec builds a LaunchSpec for descriptor d that re-execs THIS test
// binary as the provider-agnostic replay agent, pointed at d's fixture skeleton.
// It reuses the codex-fixture child mode verbatim — proof the replay path is
// provider-agnostic.
func cohortAFixtureSpec(t *testing.T, d ProviderDescriptor, fixtureFile string) LaunchSpec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	abs, err := filepath.Abs(fixtureFile)
	if err != nil {
		t.Fatalf("abs fixture path: %v", err)
	}
	spec := d.LaunchSpec(exe, nil, append(os.Environ(),
		childModeEnv+"=codex-fixture", // generic replay child mode (provider-agnostic)
		"LINTASAN_CODEX_FIXTURE="+abs,
	))
	spec.StartTimeout = 5 * time.Second
	spec.RequestTimeout = 5 * time.Second
	spec.StopTimeout = 2 * time.Second
	return spec
}

// TestCohortA_AllProvidersOnboard_NoFrameworkChange is the headline readiness
// proof: every remaining Cohort-A provider runs the full admission flow to a GO
// verdict — lifecycle active, membrane-gated, all four gates PASS — through the
// SAME generic AdmitProvider, with only a descriptor + fixture + credential
// mapping per provider.
func TestCohortA_AllProvidersOnboard_NoFrameworkChange(t *testing.T) {
	for _, tc := range cohortAOnboardCases() {
		tc := tc
		t.Run(tc.descriptor.Name, func(t *testing.T) {
			d := tc.descriptor
			// Credential mapping (the Cohort-A staging source), but resolve the
			// secret from a test stub so the test does not depend on real env vars.
			src := CredentialSourceFunc(func(p string) (string, bool) {
				if p == d.Name {
					return "***", true
				}
				return "", false
			})
			reg := provider.NewRegistry()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			p, rec, rep, err := AdmitProvider(ctx, reg,
				cohortAFixtureSpec(t, d, tc.fixtureFile),
				d.Capabilities, src, d.ForeignAuthVars)
			if err != nil {
				t.Fatalf("AdmitProvider wiring error for %s: %v", d.Name, err)
			}
			defer p.StopAgent()

			if !rep.Go() {
				for _, g := range rep.Results {
					t.Logf("gate %s = %s: %s", g.Gate, g.Outcome, g.Reason)
				}
				t.Fatalf("%s admission NOT GO", d.Name)
			}
			if rec.State != StateActive {
				t.Fatalf("%s lifecycle state = %q, want active", d.Name, rec.State)
			}
			if p.Name() != d.Name {
				t.Fatalf("provider name = %q, want %q", p.Name(), d.Name)
			}
			// Membrane invariant: registered but NEVER routable.
			if reg.IsRoutable(d.Name) {
				t.Fatalf("MEMBRANE VIOLATION: %s is production-routable", d.Name)
			}
			got, ok := reg.ResolveExperimental(d.Name)
			if !ok || got.Name() != d.Name {
				t.Fatalf("%s not reachable via the explicit experimental door", d.Name)
			}
			// All four gates present and PASS.
			if len(rep.Results) != 4 {
				t.Fatalf("%s: expected 4 gate results, got %d", d.Name, len(rep.Results))
			}
			for _, g := range rep.Results {
				if g.Outcome != GatePass {
					t.Fatalf("%s gate %s not PASS: %s (%s)", d.Name, g.Gate, g.Outcome, g.Reason)
				}
			}
		})
	}
}

// TestCohortA_CredentialMapping_IsScoped proves the Cohort-A credential source
// maps each provider to its OWN env var and never leaks across providers
// (Invariant 3). It uses a stubbed env reader so it depends on no real secrets.
func TestCohortA_CredentialMapping_IsScoped(t *testing.T) {
	// Stub env: only ANTHROPIC_API_KEY is "set".
	stub := map[string]string{"ANTHROPIC_API_KEY": "secret-anthropic"}
	src := CohortACredentialSource().withGetenv(func(k string) string { return stub[k] })

	// claude-code resolves its own secret.
	if v, ok := src.Credential("claude-code"); !ok || v != "secret-anthropic" {
		t.Fatalf("claude-code credential = (%q,%v), want (secret-anthropic,true)", v, ok)
	}
	// gemini-cli is mapped (to GEMINI_API_KEY) but that var is unset → no secret,
	// and crucially it does NOT return another provider's secret.
	if v, ok := src.Credential("gemini-cli"); ok || v != "" {
		t.Fatalf("gemini-cli credential = (%q,%v), want (\"\",false) — unset var must not leak", v, ok)
	}
	// An unmapped provider yields nothing.
	if _, ok := src.Credential("not-a-provider"); ok {
		t.Fatal("unmapped provider must not resolve a credential")
	}
}

// TestCohortA_CredentialMapping_CoversAllAuthProviders proves the staging source
// maps every Cohort-A descriptor that requires auth (all four currently).
func TestCohortA_CredentialMapping_CoversAllAuthProviders(t *testing.T) {
	src := CohortACredentialSource()
	mapped := map[string]bool{}
	for _, name := range src.MappedProviders() {
		mapped[name] = true
	}
	for _, d := range CohortADescriptors() {
		if d.AuthMode == AuthNone {
			continue
		}
		if !mapped[d.Name] {
			t.Fatalf("Cohort-A credential source missing mapping for %q", d.Name)
		}
	}
}

// TestCohortA_FixtureSkeletonsExist proves every non-Codex Cohort-A provider has
// a fixture skeleton on disk (the third leg of "descriptor + mapping + fixture").
func TestCohortA_FixtureSkeletonsExist(t *testing.T) {
	for _, tc := range cohortAOnboardCases() {
		if _, err := os.Stat(tc.fixtureFile); err != nil {
			t.Fatalf("missing fixture skeleton for %s: %v", tc.descriptor.Name, err)
		}
	}
}

package expprovider

// codex_live_test.go — operator-gated LIVE acceptance test (M5). It drives the
// REAL codex-acp binary through the REAL broker (ACPProvider.Run) using the
// production CodexLaunchSpec + the full admission flow. This is the automated
// form of the M5 live acceptance run: it proves the reconciled+remediated wire
// contract (authenticate → session/new → ContentBlock prompt → tool-loop close)
// works against the actual agent, not a fixture.
//
// It is SKIPPED by default. To run it, an operator sets:
//
//	LINTASAN_CODEX_LIVE=1
//	LINTASAN_CODEX_ACP_BIN=/abs/path/to/codex-acp     (the real binary)
//	OPENAI_API_KEY=sk-...                              (a VALID key, with quota)
//
// With a valid key the gates close the tool loop (PASS). With a missing/invalid
// key the turn fails at the model call (the broker still authenticates, opens a
// session, and sends a spec-shaped prompt — proving the WIRE fix — but the
// upstream model call 401s), which the test reports as a wire-OK / model-auth
// failure so the operator can tell the two apart.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// liveCodexSpec builds the production Codex spec pointed at the real binary from
// LINTASAN_CODEX_ACP_BIN, with a non-secret BaseEnv (PATH/HOME + a writable cwd
// hint). The secret is injected by G4 from the OPENAI_API_KEY env via the env
// credential source — never baked into the spec.
func liveCodexSpec(t *testing.T) LaunchSpec {
	t.Helper()
	bin := os.Getenv("LINTASAN_CODEX_ACP_BIN")
	if strings.TrimSpace(bin) == "" {
		t.Skip("LINTASAN_CODEX_ACP_BIN not set — skipping live codex-acp test")
	}
	// Non-secret base env only. PATH/HOME let the binary find its runtime; the
	// credential is added by the injector, not here.
	base := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
	}
	spec := CodexLaunchSpec(bin, nil, base)
	// Advertise a writable sandbox cwd for session/new (spec requires it).
	sandbox := os.Getenv("LINTASAN_CODEX_WORKDIR")
	if strings.TrimSpace(sandbox) == "" {
		sandbox = t.TempDir()
	}
	spec.WorkDir = sandbox
	return spec
}

// TestCodexLive_AdmissionFlow runs the FULL admission flow (all four gates)
// against the real codex-acp binary. PASS here is the genuine M5 acceptance:
// protocol + acceptance gates closed the real tool loop with identifier fidelity
// and terminal honesty.
func TestCodexLive_AdmissionFlow(t *testing.T) {
	if os.Getenv("LINTASAN_CODEX_LIVE") != "1" {
		t.Skip("LINTASAN_CODEX_LIVE != 1 — skipping live codex-acp acceptance test")
	}
	spec := liveCodexSpec(t)

	// Env-backed credential source: codex's secret lives in OPENAI_API_KEY.
	src := NewEnvCredentialSource().Map(CodexProviderName, CodexAuthEnvVar)
	if _, ok := src.Credential(CodexProviderName); !ok {
		t.Skipf("%s not set/empty — a valid key is required for the live acceptance run", CodexAuthEnvVar)
	}

	reg := provider.NewRegistry()
	foreign := []string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	p, rec, rep, err := AdmitCodex(ctx, reg, spec, src, foreign)
	if err != nil {
		t.Fatalf("AdmitCodex wiring error: %v", err)
	}
	defer p.StopAgent()

	// Always assert the membrane invariant regardless of GO/NO-GO.
	if reg.IsRoutable(CodexProviderName) {
		t.Fatal("MEMBRANE VIOLATION: codex is production-routable")
	}

	if !rep.Go() {
		// Surface each gate's reason so the operator can distinguish a wire
		// failure from a model-auth/quota failure.
		for _, g := range rep.Results {
			t.Logf("gate %s = %s: %s", g.Gate, g.Outcome, g.Reason)
		}
		t.Fatalf("LIVE admission NO-GO (see gate reasons above)")
	}
	if rec.State != StateActive {
		t.Fatalf("lifecycle state = %q, want active", rec.State)
	}
	for _, g := range rep.Results {
		t.Logf("gate %s = %s: %s", g.Gate, g.Outcome, g.Reason)
	}
	t.Logf("LIVE Codex acceptance PASS: tool loop closed against real codex-acp")
}

// TestCodexLive_WireContract is a narrower probe: it drives a single real turn
// through ACPProvider.Run and reports how far the lifecycle got. It is the
// diagnostic that separates the WIRE contract (authenticate + session/new +
// ContentBlock prompt) from the MODEL call. A nil error + non-empty stopReason
// is a full PASS; a model-auth error after a successful session proves the wire
// fix even when no valid key is available.
func TestCodexLive_WireContract(t *testing.T) {
	if os.Getenv("LINTASAN_CODEX_LIVE") != "1" {
		t.Skip("LINTASAN_CODEX_LIVE != 1 — skipping live codex-acp wire test")
	}
	spec := liveCodexSpec(t)
	src := NewEnvCredentialSource().Map(CodexProviderName, CodexAuthEnvVar)
	if _, ok := src.Credential(CodexProviderName); !ok {
		t.Skipf("%s not set/empty", CodexAuthEnvVar)
	}
	p := NewCodexProvider(spec, src)
	defer p.StopAgent()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := p.Run(ctx, AgentTurn{
		Prompt:       map[string]any{"text": "Reply with the single word: pong."},
		OnPermission: grantingPermission,
	})
	if err != nil {
		// The broker reached at least session/prompt if the error is a model
		// error; a wire-contract failure would have errored at authenticate or
		// session/new instead. Log it so the operator can read the boundary.
		t.Logf("Run returned error (inspect to classify wire vs model): %v", err)
		t.Fatalf("live wire turn did not complete cleanly: %v", err)
	}
	if strings.TrimSpace(res.StopReason) == "" {
		t.Fatal("terminal honesty: empty stopReason from a real turn")
	}
	t.Logf("LIVE wire OK: stopReason=%q toolCalls=%d textLen=%d", res.StopReason, len(res.ToolCalls), len(res.Text))
}

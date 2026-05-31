package expprovider

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// codex_test.go — Codex onboarding tests (Cohort A #1). Proves the full admission
// flow on the live foundation: LaunchSpec + credential injection + the three real
// harness probes (Isolation/Protocol/Acceptance) + membrane gate + lifecycle, all
// driven by a fixture-replay agent that emits the recorded codex-acp spec-ACP
// frame sequence (testdata/codex-acp-session.jsonl).
//
// The fixture is the in-process WIRE-TRUTH ANCHOR: it is a static recording of the
// shape codex-acp emits (text chunk + tool_call notification + request_permission
// + terminal stopReason). The LIVE acceptance (M5) drives the real codex-acp CLI
// in staging (operator-run); this proves the code path closes the tool loop with
// identifier fidelity and terminal honesty.

// runCodexFixtureAgent replays testdata/codex-acp-session.jsonl as a child ACP
// agent. Each non-comment line is {"on":<method>,"emit":[<frames>]}; on receiving
// a request for <method>, it emits the frames in order, substituting:
//   - "__ID__"        → the incoming request's id (for direct responses)
//   - "__PROMPT_ID__" → the session/prompt request id (for the terminal response)
//
// Notifications (no id) and the permission request (its own id) pass through; the
// broker's reply to the permission request is read and discarded so the stream can
// continue to the terminal frame.
func runCodexFixtureAgent() {
	path := os.Getenv("LINTASAN_CODEX_FIXTURE")
	if path == "" {
		os.Exit(21) // misconfigured: no fixture path → contained child-exit error
	}
	script, err := loadFixtureScript(path)
	if err != nil {
		os.Exit(22)
	}

	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	writeRaw := func(b []byte) {
		w.Write(b)
		w.WriteByte('\n')
		w.Flush()
	}

	for {
		line, rerr := r.ReadString('\n')
		if len(line) > 0 {
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			json.Unmarshal([]byte(line), &msg)
			frames, ok := script[msg.Method]
			if ok {
				idStr := string(msg.ID)
				for _, fr := range frames {
					out := strings.ReplaceAll(string(fr), `"__ID__"`, nz(idStr))
					out = strings.ReplaceAll(out, `"__PROMPT_ID__"`, nz(idStr))
					writeRaw([]byte(out))
					// After the permission request frame, read the broker's reply
					// (so the next iteration's stream is aligned) and drop it.
					if strings.Contains(out, "session/request_permission") {
						r.ReadString('\n')
					}
				}
			}
		}
		if rerr != nil {
			return
		}
	}
}

// nz returns a JSON id token, defaulting to null when empty.
func nz(id string) string {
	if strings.TrimSpace(id) == "" {
		return "null"
	}
	return id
}

// loadFixtureScript parses the JSONL fixture into method → ordered frames. Lines
// beginning with {"_comment" are skipped.
func loadFixtureScript(path string) (map[string][]json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]json.RawMessage{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, `{"_comment"`) {
			continue
		}
		var rec struct {
			On   string            `json:"on"`
			Emit []json.RawMessage `json:"emit"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, err
		}
		if rec.On != "" {
			out[rec.On] = rec.Emit
		}
	}
	return out, sc.Err()
}

// fixtureSpec builds a Codex LaunchSpec that re-execs THIS test binary as the
// fixture-replay agent, with the fixture path + child mode in BaseEnv.
func fixtureSpec(t *testing.T) LaunchSpec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	abs, err := filepath.Abs("testdata/codex-acp-session.jsonl")
	if err != nil {
		t.Fatalf("abs fixture path: %v", err)
	}
	spec := CodexLaunchSpec(exe, nil, append(os.Environ(),
		childModeEnv+"=codex-fixture",
		"LINTASAN_CODEX_FIXTURE="+abs,
	))
	// Tighten timeouts for the test (the fixture replies instantly).
	spec.StartTimeout = 5 * time.Second
	spec.RequestTimeout = 5 * time.Second
	spec.StopTimeout = 2 * time.Second
	return spec
}

// codexCredSrc returns a source that resolves ONLY codex's secret.
func codexCredSrc() CredentialSource {
	return CredentialSourceFunc(func(p string) (string, bool) {
		if p == CodexProviderName {
			return "sk-test-codex", true
		}
		return "", false
	})
}

// TestCodex_LaunchSpecValid proves the Codex spec is internally consistent.
func TestCodex_LaunchSpecValid(t *testing.T) {
	spec := CodexLaunchSpec("", nil, nil)
	if err := spec.Validate(); err != nil {
		t.Fatalf("Codex LaunchSpec invalid: %v", err)
	}
	if spec.Name != CodexProviderName || spec.Protocol != ProtocolACP || spec.AuthEnvVar != CodexAuthEnvVar {
		t.Fatalf("unexpected spec fields: %+v", spec)
	}
	if spec.Path != "codex-acp" {
		t.Fatalf("default path = %q, want codex-acp", spec.Path)
	}
}

// TestCodex_AdmissionFlow_GO is the headline test: the full admission flow runs
// every gate against the fixture agent and reaches a GO verdict → lifecycle active.
func TestCodex_AdmissionFlow_GO(t *testing.T) {
	reg := provider.NewRegistry()
	foreign := []string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, rec, rep, err := AdmitCodex(ctx, reg, fixtureSpec(t), codexCredSrc(), foreign)
	if err != nil {
		t.Fatalf("AdmitCodex wiring error: %v", err)
	}
	defer p.StopAgent()

	if !rep.Go() {
		t.Fatalf("admission verdict NOT GO: %+v", rep.Results)
	}
	if rec.State != StateActive {
		t.Fatalf("lifecycle state = %q, want active", rec.State)
	}
	// Membrane: Codex registered but NOT in the routable pool.
	if reg.IsRoutable(CodexProviderName) {
		t.Fatal("MEMBRANE VIOLATION: codex is production-routable")
	}
	gotExp, ok := reg.ResolveExperimental(CodexProviderName)
	if !ok || gotExp.Name() != CodexProviderName {
		t.Fatal("codex not reachable via the explicit experimental door")
	}
	// Every gate PASS (no FAIL) and all four present.
	if len(rep.Results) != 4 {
		t.Fatalf("expected 4 gate results, got %d", len(rep.Results))
	}
	for _, g := range rep.Results {
		if g.Outcome != GatePass {
			t.Fatalf("gate %s not PASS: %s (%s)", g.Gate, g.Outcome, g.Reason)
		}
	}
}

// TestCodex_ProtocolGate_TerminalHonestyAndFidelity drives the protocol probe and
// asserts the turn completed with a stopReason and a verbatim toolCallId.
func TestCodex_ProtocolGate(t *testing.T) {
	p := NewCodexProvider(fixtureSpec(t), codexCredSrc())
	defer p.StopAgent()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, reason := ProtocolProbe(ctx, Candidate{Provider: CodexProviderName, Adapter: p, Spec: fixtureSpec(t)})
	if out != GatePass {
		t.Fatalf("ProtocolProbe FAIL: %s", reason)
	}
}

// TestCodex_AcceptanceGate_ToolLoopCloses is the M5-principle test: the tool loop
// MUST close (≥1 tool call observed, fidelity intact, terminal stopReason).
func TestCodex_AcceptanceGate_ToolLoopCloses(t *testing.T) {
	p := NewCodexProvider(fixtureSpec(t), codexCredSrc())
	defer p.StopAgent()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, reason := AcceptanceProbe(ctx, Candidate{Provider: CodexProviderName, Adapter: p, Spec: fixtureSpec(t)})
	if out != GatePass {
		t.Fatalf("AcceptanceProbe FAIL (tool loop did not close): %s", reason)
	}
	if !strings.Contains(reason, "tool call") {
		t.Fatalf("acceptance reason should evidence the tool loop, got %q", reason)
	}
}

// TestCodex_IsolationGate_ScopingAndForeignSecret proves the isolation probe:
// the candidate's own auth var is injected and a foreign secret is NOT.
func TestCodex_IsolationGate(t *testing.T) {
	p := NewCodexProvider(fixtureSpec(t), codexCredSrc())
	defer p.StopAgent()
	probe := IsolationProbe([]string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY"})
	out, reason := probe(context.Background(), Candidate{Provider: CodexProviderName, Adapter: p, Spec: fixtureSpec(t)})
	if out != GatePass {
		t.Fatalf("IsolationProbe FAIL: %s", reason)
	}
}

// TestCodex_IsolationGate_RejectsBakedSecret proves a spec that bakes the auth
// secret into BaseEnv is rejected (Invariant 3).
func TestCodex_IsolationGate_RejectsBakedSecret(t *testing.T) {
	spec := fixtureSpec(t)
	spec.BaseEnv = append(spec.BaseEnv, CodexAuthEnvVar+"=leaked-secret")
	p := NewCodexProvider(spec, codexCredSrc())
	defer p.StopAgent()
	probe := IsolationProbe(nil)
	out, reason := probe(context.Background(), Candidate{Provider: CodexProviderName, Adapter: p, Spec: spec})
	if out != GateFail {
		t.Fatalf("IsolationProbe should FAIL on a baked secret, got %s (%s)", out, reason)
	}
}

// TestCodex_AdmissionFlow_BlocksOnMissingCredential proves that without a
// resolvable credential, the gates fail (the agent can't launch) → NO-GO, and the
// provider stays admitted (not active) but is still membrane-gated.
func TestCodex_AdmissionFlow_BlocksOnMissingCredential(t *testing.T) {
	reg := provider.NewRegistry()
	emptySrc := CredentialSourceFunc(func(string) (string, bool) { return "", false })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	p, rec, rep, err := AdmitCodex(ctx, reg, fixtureSpec(t), emptySrc, nil)
	if err != nil {
		t.Fatalf("AdmitCodex wiring error: %v", err)
	}
	defer p.StopAgent()
	if rep.Go() {
		t.Fatal("admission should be NO-GO without a credential")
	}
	if rec.State == StateActive {
		t.Fatal("lifecycle must NOT be active on a NO-GO")
	}
	// Even on NO-GO, the provider is registered + membrane-gated (never routable).
	if reg.IsRoutable(CodexProviderName) {
		t.Fatal("MEMBRANE VIOLATION: codex routable even on NO-GO")
	}
}

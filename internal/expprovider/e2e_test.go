package expprovider

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/experimental"
	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// e2e_test.go — end-to-end proof that the G1 adapter (ACPProvider.Run) actually
// drives the ACP loop through a REAL E1 subprocess, with credential injection
// applied. The test binary re-execs itself as a scripted SPEC-ACP agent.
//
// WIRE RECONCILIATION (2026-05-31): the scripted agent now speaks SPEC ACP, not
// the old simplified dialect. A prompt turn emits a `session/update` notification
// (sessionUpdate:"tool_call", no id, no reply) + an agent→client
// `session/request_permission` REQUEST (has id, MUST be answered), then the
// terminal response to session/prompt with a stopReason. This closes the loop
// the M5 principle requires: a tool-bearing turn that COMPLETES, with the
// toolCallId observed verbatim by the host (identifier fidelity).

const childModeEnv = "LINTASAN_EXPPROV_TEST_CHILD"

// TestMain dispatches to the scripted ACP agent when launched as a child;
// otherwise it runs the normal suite.
func TestMain(m *testing.M) {
	switch os.Getenv(childModeEnv) {
	case "acp-agent":
		runScriptedACPAgent()
		return
	case "codex-fixture":
		// Replays the recorded codex-acp spec-ACP frame sequence (testdata/
		// codex-acp-session.jsonl) — the in-process wire-truth anchor for the
		// Codex Protocol + Acceptance gates. Defined in codex_test.go.
		runCodexFixtureAgent()
		return
	case "acp-agent-assert-secret":
		// Same agent, but FIRST assert the injected secret is visible to the
		// child (proves credential injection reached the process env) and that
		// no foreign secret leaked in.
		if os.Getenv("OPENAI_API_KEY") != "***" {
			os.Exit(11) // injected secret missing → contained as child-exit error
		}
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			os.Exit(12) // foreign secret leaked → fail
		}
		runScriptedACPAgent()
		return
	}
	os.Exit(m.Run())
}

// runScriptedACPAgent speaks the SPEC ACP JSON-RPC lifecycle over stdio and is
// DELIBERATELY STRICT — it enforces the same preconditions a real spec agent
// (codex-acp) enforces, so a broker regression is caught in-process:
//
//	initialize     → {protocolVersion: 1, agentInfo, authMethods:[openai-api-key]}
//	authenticate   → {} and marks the session authenticated. REQUIRED before
//	                 session/new (mirrors codex-acp's "Authentication required").
//	session/new    → if NOT authenticated → JSON-RPC error -32000; else {sessionId}.
//	session/prompt → if sessionId is missing/mismatched → error -32602; if the
//	                 prompt is NOT a spec ContentBlock array ([{type,text}]) →
//	                 error -32602; else emits session/update (tool_call) as a
//	                 NOTIFICATION, then session/request_permission as an agent→host
//	                 REQUEST, reads the host's outcome, then responds to the prompt
//	                 id with stopReason + an echo of the toolCallId (fidelity).
//	shutdown       → {}
func runScriptedACPAgent() {
	const sessionID = "sess-e2e"
	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	writeLine := func(v any) {
		b, _ := json.Marshal(v)
		w.Write(b)
		w.WriteByte('\n')
		w.Flush()
	}
	writeErr := func(id json.RawMessage, code int, msg string) {
		writeLine(map[string]any{"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": code, "message": msg}})
	}
	readMsg := func() (id json.RawMessage, method string, result json.RawMessage) {
		line, _ := r.ReadString('\n')
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}
		json.Unmarshal([]byte(line), &msg)
		return msg.ID, msg.Method, msg.Result
	}
	authed := false
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			json.Unmarshal([]byte(line), &msg)
			switch msg.Method {
			case "initialize":
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"protocolVersion": 1,
						"agentInfo": map[string]any{"name": "scripted"},
						"authMethods": []map[string]any{
							{"type": "env_var", "id": "openai-api-key", "name": "Use OPENAI_API_KEY"}}}})
			case "authenticate":
				// Mirror codex-acp: selecting the env_var method validates the
				// precondition and unlocks session creation.
				authed = true
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{}})
			case "session/new":
				if !authed {
					// EXACTLY the failure live validation found: session/new
					// before authenticate must be rejected.
					writeErr(msg.ID, -32000, "Authentication required")
					break
				}
				// Spec ACP requires session/new params {cwd, mcpServers}; a
				// spec-faithful agent (codex-acp) rejects -32602 when cwd is
				// absent. Enforce it so the broker's default params are exercised.
				var np struct {
					Cwd        string          `json:"cwd"`
					McpServers json.RawMessage `json:"mcpServers"`
				}
				json.Unmarshal(msg.Params, &np)
				if np.Cwd == "" {
					writeErr(msg.ID, -32602, "session/new requires cwd")
					break
				}
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"sessionId": sessionID}})
			case "session/prompt":
				promptID := msg.ID
				// Strict spec validation: sessionId present + matching, and the
				// prompt MUST be a ContentBlock array (not a bare {"text":...}).
				var pp struct {
					SessionID string          `json:"sessionId"`
					Prompt    json.RawMessage `json:"prompt"`
				}
				json.Unmarshal(msg.Params, &pp)
				if pp.SessionID != sessionID {
					writeErr(promptID, -32602, "invalid or missing sessionId")
					break
				}
				if !promptIsContentBlockArray(pp.Prompt) {
					writeErr(promptID, -32602, "prompt must be a ContentBlock array")
					break
				}
				// 1) Report the tool call via a session/update NOTIFICATION (no id).
				writeLine(map[string]any{"jsonrpc": "2.0", "method": "session/update",
					"params": map[string]any{"sessionId": sessionID,
						"update": map[string]any{"sessionUpdate": "tool_call",
							"toolCallId": "tc-e2e-1", "kind": "execute", "status": "pending"}}})
				// 2) Ask permission via an agent→host REQUEST (has id) and read the
				//    host's outcome on the next line.
				writeLine(map[string]any{"jsonrpc": "2.0", "id": "perm-1",
					"method": "session/request_permission",
					"params": map[string]any{"sessionId": sessionID, "toolCallId": "tc-e2e-1",
						"options": []map[string]any{
							{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
							{"optionId": "reject-once", "name": "Reject", "kind": "reject_once"}}}})
				_, _, permResult := readMsg()
				var pr struct {
					Outcome struct {
						Outcome  string `json:"outcome"`
						OptionID string `json:"optionId"`
					} `json:"outcome"`
				}
				json.Unmarshal(permResult, &pr)
				// 3) Terminal response to the prompt id, echoing the granted outcome.
				writeLine(map[string]any{"jsonrpc": "2.0", "id": promptID,
					"result": map[string]any{"stopReason": "end_turn",
						"content": json.RawMessage(`{"echoedToolCallId":"tc-e2e-1","permission":"` + pr.Outcome.OptionID + `"}`)}})
			case "shutdown":
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{}})
			}
		}
		if err != nil {
			return
		}
	}
}

// promptIsContentBlockArray reports whether raw is a spec ACP ContentBlock array
// ([{"type":"text","text":...}, ...]) with at least one text block. A bare
// object like {"text":"..."} (the pre-fix broker shape) returns false — which is
// exactly what makes the strict agent catch the encoding regression.
func promptIsContentBlockArray(raw json.RawMessage) bool {
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "text" {
			return true
		}
	}
	return false
}

// e2eSpec returns a spec that re-execs THIS test binary as the scripted agent.
func e2eSpec(t *testing.T, mode string) LaunchSpec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return LaunchSpec{
		Name:         "codex",
		Protocol:     ProtocolACP,
		Path:         exe,
		Args:         nil,
		AuthMode:     AuthAPIKey,
		AuthEnvVar:   "OPENAI_API_KEY",
		AuthMethodID: "openai-api-key", // strict agent requires authenticate before session/new
		// BaseEnv re-execs the agent mode; the secret is injected by G4 on top.
		BaseEnv:        append(os.Environ(), childModeEnv+"="+mode),
		RequestTimeout: 5 * time.Second,
		StopTimeout:    2 * time.Second,
	}
}

// TestE2E_AgentRun_ClosesToolLoopWithVerbatimID is the acceptance-shaped proof:
// Run launches the agent, drives the full SPEC-ACP lifecycle, the host permission
// handler fires for the tool call, the turn completes with stopReason, and the
// toolCallId is observed verbatim (in PromptResult.ToolCalls and the agent's echo).
func TestE2E_AgentRun_ClosesToolLoopWithVerbatimID(t *testing.T) {
	src := CredentialSourceFunc(func(p string) (string, bool) {
		if p == "codex" {
			return "***", true
		}
		return "", false
	})
	p := NewACPProvider(e2eSpec(t, "acp-agent"), provider.NewCapabilitySet(provider.CapCoding), NewInjector(src))
	defer p.StopAgent()

	var sawPermissionFor string
	turn := AgentTurn{
		Prompt: map[string]any{"text": "ping please"},
		OnPermission: func(ctx context.Context, req experimental.PermissionRequest) experimental.PermissionOutcome {
			sawPermissionFor = req.ToolCallID
			// Grant: select the allow_once option the agent offered.
			for _, o := range req.Options {
				if o.Kind == "allow_once" {
					return experimental.PermissionOutcome{Outcome: "selected", OptionID: o.OptionID}
				}
			}
			return experimental.PermissionOutcome{Outcome: "cancelled"}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	res, err := p.Run(ctx, turn)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The host permission handler saw the agent's verbatim toolCallId.
	if sawPermissionFor != "tc-e2e-1" {
		t.Fatalf("host saw permission for toolCallId %q, want tc-e2e-1", sawPermissionFor)
	}
	// The broker accumulated the tool call from the session/update stream.
	if len(res.ToolCalls) != 1 || res.ToolCalls[0] != "tc-e2e-1" {
		t.Fatalf("PromptResult.ToolCalls = %v, want [tc-e2e-1]", res.ToolCalls)
	}
	// The agent echoed the toolCallId + the granted option — proving the
	// permission outcome round-tripped and the loop closed.
	var content struct {
		Echoed     string `json:"echoedToolCallId"`
		Permission string `json:"permission"`
	}
	json.Unmarshal(res.Content, &content)
	if content.Echoed != "tc-e2e-1" {
		t.Fatalf("agent echoed toolCallId %q, want tc-e2e-1 — fidelity broken", content.Echoed)
	}
	if content.Permission != "allow-once" {
		t.Fatalf("agent saw permission %q, want allow-once — outcome did not round-trip", content.Permission)
	}
	if res.StopReason != "end_turn" {
		t.Fatalf("stopReason = %q, want end_turn", res.StopReason)
	}
}

// TestE2E_CredentialInjectionReachesChild proves G4: the injected secret is in
// the child's process env, and a foreign provider's secret is NOT. The child
// exits non-zero if either condition fails, which Run surfaces as a contained
// error (so a PASS here means both assertions held inside the real subprocess).
func TestE2E_CredentialInjectionReachesChild(t *testing.T) {
	src := CredentialSourceFunc(func(p string) (string, bool) {
		if p == "codex" {
			return "***", true
		}
		return "", false
	})
	p := NewACPProvider(e2eSpec(t, "acp-agent-assert-secret"), nil, NewInjector(src))
	defer p.StopAgent()

	turn := AgentTurn{
		Prompt: map[string]any{"text": "ping"},
		OnPermission: func(ctx context.Context, req experimental.PermissionRequest) experimental.PermissionOutcome {
			for _, o := range req.Options {
				if o.Kind == "allow_once" {
					return experimental.PermissionOutcome{Outcome: "selected", OptionID: o.OptionID}
				}
			}
			return experimental.PermissionOutcome{Outcome: "cancelled"}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := p.Run(ctx, turn); err != nil {
		t.Fatalf("Run failed — credential injection assertion likely failed inside child: %v", err)
	}
}

// --- Fix-4 regression proofs: the strict spec agent CATCHES the live bugs -----
//
// These tests drive the broker primitives directly against the strict scripted
// agent, reproducing the THREE pre-fix broker behaviors that live validation
// found, and assert the agent now REJECTS each one. Without these, "fixture
// realism" would be unverifiable — they are the proof that the fixture would
// have caught the regression.

// startStrictClient launches the strict scripted agent and returns a started,
// initialized ACPClient (no authenticate yet — the caller drives the sequence).
func startStrictClient(t *testing.T) *experimental.ACPClient {
	t.Helper()
	src := CredentialSourceFunc(func(p string) (string, bool) {
		if p == "codex" {
			return "***", true
		}
		return "", false
	})
	spec := e2eSpec(t, "acp-agent")
	env, err := NewInjector(src).BuildEnv(spec)
	if err != nil {
		t.Fatalf("BuildEnv: %v", err)
	}
	proc := experimental.New(spec.toSubprocessConfig(env))
	client := experimental.NewACPClient(proc)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("client.Start: %v", err)
	}
	if _, err := client.Initialize(ctx, experimental.InitializeParams{ProtocolVersion: experimental.CurrentProtocolVersion}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return client
}

// TestFix4_StrictAgent_RejectsSessionNewBeforeAuthenticate reproduces BUG 1
// (authenticate step missing): calling session/new WITHOUT a prior authenticate
// must be rejected by the agent — exactly the -32000 "Authentication required"
// the real codex-acp returned.
func TestFix4_StrictAgent_RejectsSessionNewBeforeAuthenticate(t *testing.T) {
	client := startStrictClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	// Skip authenticate (the OLD broker behavior) → must fail.
	if _, err := client.NewSession(ctx, map[string]any{"cwd": "/tmp", "mcpServers": []any{}}); err == nil {
		t.Fatal("BUG-1 NOT caught: session/new before authenticate should be rejected")
	}
	// With authenticate first, session/new succeeds (the fix).
	if err := client.Authenticate(ctx, experimental.AuthenticateParams{MethodID: "openai-api-key"}); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if _, err := client.NewSession(ctx, map[string]any{"cwd": "/tmp", "mcpServers": []any{}}); err != nil {
		t.Fatalf("session/new after authenticate should succeed: %v", err)
	}
}

// TestFix4_StrictAgent_RejectsMissingSessionID reproduces BUG 2 (sessionId
// propagation missing): a session/prompt with an empty/missing sessionId — what
// the OLD Run sent — must be rejected.
func TestFix4_StrictAgent_RejectsMissingSessionID(t *testing.T) {
	client := startStrictClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := client.Authenticate(ctx, experimental.AuthenticateParams{MethodID: "openai-api-key"}); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if _, err := client.NewSession(ctx, map[string]any{"cwd": "/tmp", "mcpServers": []any{}}); err != nil {
		t.Fatalf("session/new: %v", err)
	}
	// OLD behavior: prompt with NO sessionId + correct ContentBlock shape.
	_, err := client.Prompt(ctx, experimental.PromptParams{
		Prompt: []map[string]any{{"type": "text", "text": "hi"}}, // sessionId omitted
	}, nil)
	if err == nil {
		t.Fatal("BUG-2 NOT caught: session/prompt with missing sessionId should be rejected")
	}
}

// TestFix4_StrictAgent_RejectsNonContentBlockPrompt reproduces BUG 3 (prompt
// ContentBlock shape mismatch): a prompt sent as a bare {"text":...} object —
// what the OLD Run forwarded — must be rejected; only a ContentBlock array works.
func TestFix4_StrictAgent_RejectsNonContentBlockPrompt(t *testing.T) {
	client := startStrictClient(t)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := client.Authenticate(ctx, experimental.AuthenticateParams{MethodID: "openai-api-key"}); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	sid, err := client.NewSession(ctx, map[string]any{"cwd": "/tmp", "mcpServers": []any{}})
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	// OLD behavior: prompt as a bare object, NOT a ContentBlock array.
	_, err = client.Prompt(ctx, experimental.PromptParams{
		SessionID: sid,
		Prompt:    map[string]any{"text": "hi"}, // wrong shape
	}, nil)
	if err == nil {
		t.Fatal("BUG-3 NOT caught: non-ContentBlock prompt shape should be rejected")
	}
	// Correct shape (what the fixed encodePrompt produces) succeeds.
	res, err := client.Prompt(ctx, experimental.PromptParams{
		SessionID: sid,
		Prompt:    []map[string]any{{"type": "text", "text": "hi"}},
	}, func(_ context.Context, req experimental.PermissionRequest) experimental.PermissionOutcome {
		for _, o := range req.Options {
			if o.Kind == "allow_once" {
				return experimental.PermissionOutcome{Outcome: "selected", OptionID: o.OptionID}
			}
		}
		return experimental.PermissionOutcome{Outcome: "cancelled"}
	})
	if err != nil {
		t.Fatalf("correct ContentBlock prompt should succeed: %v", err)
	}
	if res.StopReason == "" {
		t.Fatal("expected a terminal stopReason on the fixed prompt shape")
	}
}

// TestFix4_EncodePrompt_NormalizesToContentBlockArray proves the encoder turns
// the legacy {"text":...} and plain-string shapes into the spec ContentBlock
// array, while passing an already-shaped array through unchanged.
func TestFix4_EncodePrompt_NormalizesToContentBlockArray(t *testing.T) {
	// legacy map shape → array with one text block
	out := encodePrompt(map[string]any{"text": "hello"})
	arr, ok := out.([]map[string]any)
	if !ok || len(arr) != 1 || arr[0]["type"] != "text" || arr[0]["text"] != "hello" {
		t.Fatalf("legacy map not normalized: %#v", out)
	}
	// plain string → array with one text block
	out = encodePrompt("hi")
	arr, ok = out.([]map[string]any)
	if !ok || len(arr) != 1 || arr[0]["text"] != "hi" {
		t.Fatalf("string not normalized: %#v", out)
	}
	// already an array → unchanged
	in := []map[string]any{{"type": "text", "text": "x"}}
	if got := encodePrompt(in); got == nil {
		t.Fatal("array passthrough returned nil")
	}
}

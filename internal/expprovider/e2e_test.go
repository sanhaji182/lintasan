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

// runScriptedACPAgent speaks the SPEC ACP JSON-RPC lifecycle over stdio:
//
//	initialize     → {protocolVersion: 1, agentInfo}
//	session/new    → {sessionId}
//	session/prompt → emits session/update (tool_call, toolCallId=tc-e2e-1) as a
//	                 NOTIFICATION, then session/request_permission as an agent→host
//	                 REQUEST, reads the host's permission outcome, then responds
//	                 to the original prompt id with stopReason + an echo of the
//	                 toolCallId it offered (so the test asserts identifier fidelity).
//	shutdown       → {}
func runScriptedACPAgent() {
	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	writeLine := func(v any) {
		b, _ := json.Marshal(v)
		w.Write(b)
		w.WriteByte('\n')
		w.Flush()
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
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			json.Unmarshal([]byte(line), &msg)
			switch msg.Method {
			case "initialize":
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"name": "scripted"}}})
			case "session/new":
				writeLine(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"result": map[string]any{"sessionId": "sess-e2e"}})
			case "session/prompt":
				promptID := msg.ID
				// 1) Report the tool call via a session/update NOTIFICATION (no id).
				writeLine(map[string]any{"jsonrpc": "2.0", "method": "session/update",
					"params": map[string]any{"sessionId": "sess-e2e",
						"update": map[string]any{"sessionUpdate": "tool_call",
							"toolCallId": "tc-e2e-1", "kind": "execute", "status": "pending"}}})
				// 2) Ask permission via an agent→host REQUEST (has id) and read the
				//    host's outcome on the next line.
				writeLine(map[string]any{"jsonrpc": "2.0", "id": "perm-1",
					"method": "session/request_permission",
					"params": map[string]any{"sessionId": "sess-e2e", "toolCallId": "tc-e2e-1",
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

// e2eSpec returns a spec that re-execs THIS test binary as the scripted agent.
func e2eSpec(t *testing.T, mode string) LaunchSpec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return LaunchSpec{
		Name:       "codex",
		Protocol:   ProtocolACP,
		Path:       exe,
		Args:       nil,
		AuthMode:   AuthAPIKey,
		AuthEnvVar: "OPENAI_API_KEY",
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

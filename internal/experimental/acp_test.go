package experimental

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// acp_test.go — ACP broker tests (post wire-reconciliation, 2026-05-31).
//
// These drive the broker against a REAL scripted SPEC-ACP agent subprocess (the
// "acp-agent" mode in TestMain), exercising the full lifecycle over the E1
// transport: initialize (integer protocolVersion) → session/new → session/prompt
// (a STREAM of session/update notifications + a session/request_permission
// agent→client request) → terminal stopReason → shutdown.
//
// The load-bearing assertions: (1) the broker DRAINS the notification stream and
// accumulates text + tool-call ids; (2) it ANSWERS the permission request and the
// outcome round-trips; (3) IDENTIFIER FIDELITY — the toolCallId the agent reports
// is observed verbatim (the M3 call_id lesson applied to ACP).

func newACPTestClient(t *testing.T) *ACPClient {
	t.Helper()
	cfg := childConfig(t, "acp-agent", 5*time.Second, 2*time.Second)
	return NewACPClient(New(cfg))
}

// grantAllowOnce is a permission handler that selects the allow_once option.
func grantAllowOnce(_ context.Context, req PermissionRequest) PermissionOutcome {
	for _, o := range req.Options {
		if o.Kind == "allow_once" {
			return PermissionOutcome{Outcome: "selected", OptionID: o.OptionID}
		}
	}
	return PermissionOutcome{Outcome: "cancelled"}
}

// TestACP_FullLifecycleStreamingTurn is the reconciled acceptance-shaped test:
// the whole spec-ACP turn completes — notifications drained, text accumulated,
// permission answered, stopReason reached, toolCallId observed verbatim.
func TestACP_FullLifecycleStreamingTurn(t *testing.T) {
	c := newACPTestClient(t)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer c.Close()

	ctx := context.Background()

	// initialize — integer protocolVersion (1), negotiated echo.
	init, err := c.Initialize(ctx, InitializeParams{
		ProtocolVersion:    CurrentProtocolVersion,
		ClientCapabilities: ClientCapabilities{}, // advertise none (no fs/terminal)
		ClientInfo:         map[string]any{"name": "lintasan"},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.ProtocolVersion != 1 {
		t.Fatalf("unexpected protocol version: %d (want 1)", init.ProtocolVersion)
	}

	// session/new
	sid, err := c.NewSession(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if sid != "sess-42" {
		t.Fatalf("unexpected session id: %q", sid)
	}

	// session/prompt — drive the streaming turn with a granting permission handler.
	res, err := c.Prompt(ctx, PromptParams{SessionID: sid, Prompt: "what time is it?"}, grantAllowOnce)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}

	// Stream drain: the text chunk notification was accumulated.
	if !strings.Contains(res.Text, "thinking") {
		t.Fatalf("broker did not accumulate streamed text, got %q", res.Text)
	}
	// Identifier fidelity: the tool_call id was tracked verbatim from the stream.
	if len(res.ToolCalls) != 1 || res.ToolCalls[0] != "tc-1" {
		t.Fatalf("ToolCalls = %v, want [tc-1]", res.ToolCalls)
	}
	// Permission round-trip: the agent echoed the toolCallId + the granted option.
	var content struct {
		EchoedToolCallID string `json:"echoedToolCallId"`
		Permission       string `json:"permission"`
	}
	if err := json.Unmarshal(res.Content, &content); err != nil {
		t.Fatalf("decode prompt result content: %v (raw=%s)", err, res.Content)
	}
	if content.EchoedToolCallID != "tc-1" {
		t.Fatalf("IDENTIFIER FIDELITY BROKEN: agent echoed %q, want tc-1", content.EchoedToolCallID)
	}
	if content.Permission != "allow-once" {
		t.Fatalf("permission did not round-trip: agent saw %q, want allow-once", content.Permission)
	}
	if res.StopReason != "end_turn" {
		t.Fatalf("unexpected stop reason: %q", res.StopReason)
	}

	// shutdown is graceful.
	if err := c.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

// TestACP_NilPermissionHandlerIsContained proves that with no permission handler
// the broker DENIES (selects a reject option or cancels) so the agent terminates
// cleanly rather than hanging. The turn still completes with a stopReason.
func TestACP_NilPermissionHandlerIsContained(t *testing.T) {
	c := newACPTestClient(t)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer c.Close()
	ctx := context.Background()
	if _, err := c.Initialize(ctx, InitializeParams{ProtocolVersion: CurrentProtocolVersion}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sid, _ := c.NewSession(ctx, map[string]any{})

	// nil handler → broker denies (selects reject-once option the agent offered);
	// the agent echoes that optionId and the loop completes without hanging.
	res, err := c.Prompt(ctx, PromptParams{SessionID: sid, Prompt: "go"}, nil)
	if err != nil {
		t.Fatalf("prompt with nil handler should still complete: %v", err)
	}
	var content struct {
		Permission string `json:"permission"`
	}
	json.Unmarshal(res.Content, &content)
	if content.Permission != "reject-once" {
		t.Fatalf("nil handler should deny via reject-once, agent saw %q", content.Permission)
	}
	if res.StopReason != "end_turn" {
		t.Fatalf("nil-handler path broke the loop, stopReason=%q", res.StopReason)
	}
}

// TestACP_ToolCallIdTrackedVerbatim proves the broker records the agent's
// toolCallId exactly, in the order it appeared, from the session/update stream.
func TestACP_ToolCallIdTrackedVerbatim(t *testing.T) {
	c := newACPTestClient(t)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer c.Close()
	ctx := context.Background()
	if _, err := c.Initialize(ctx, InitializeParams{ProtocolVersion: CurrentProtocolVersion}); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sid, err := c.NewSession(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	res, err := c.Prompt(ctx, PromptParams{SessionID: sid, Prompt: "go"}, grantAllowOnce)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0] != "tc-1" {
		t.Fatalf("broker failed to track verbatim toolCallId: got %v, want [tc-1]", res.ToolCalls)
	}
}

// TestACP_ClosedClientRejects proves operations on a closed client are contained.
func TestACP_ClosedClientRejects(t *testing.T) {
	c := newACPTestClient(t)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := c.Initialize(context.Background(), InitializeParams{}); err == nil {
		t.Fatal("expected error from a closed client")
	}
}

// TestACP_UnknownMethodReturnsRPCError proves a JSON-RPC error from the agent is
// surfaced as a typed error (not swallowed).
func TestACP_UnknownMethodReturnsRPCError(t *testing.T) {
	c := newACPTestClient(t)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer c.Close()
	// "session/new" is known; "bogus/method" is not → agent returns rpc error.
	err := c.call(context.Background(), "bogus/method", nil, nil)
	if err == nil {
		t.Fatal("expected an rpc error for an unknown method")
	}
	if !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("expected method-not-found rpc error, got %v", err)
	}
}

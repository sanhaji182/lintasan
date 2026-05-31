package experimental

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// acp.go — ACP (Agent Client Protocol) integration layer.
//
// ACP lets one host drive many agents over a single official protocol: JSON-RPC
// 2.0 over the agent's stdio. Lintasan is the ACP CLIENT/HOST; an agent CLI
// (launched as an E1 Subprocess) is the ACP AGENT. This is "Shape 2" — official
// orchestration of an official CLI/SDK, ZERO reverse-engineering.
//
// This layer is built ON TOP of the Phase-3 E1 byte transport (Subprocess): the
// Subprocess gives us isolation (timeout/crash/panic containment); this file
// adds the JSON-RPC framing + the ACP lifecycle broker.
//
// WIRE RECONCILIATION (2026-05-31): the broker speaks SPEC ACP
// (agentclientprotocol.com), NOT any single agent's dialect — that is what lets
// the SAME broker serve Codex (codex-acp), Claude Code (claude-agent-acp),
// Gemini CLI (--experimental-acp), and Copilot (copilot --acp) through one
// Protocol Gate. The reconciled facts:
//   - protocolVersion is a single INTEGER major version (current = 1), not a string.
//   - A prompt turn is a STREAM: after session/prompt the agent emits 0..many
//     `session/update` NOTIFICATIONS (no id, no reply) and 0..many agent→client
//     REQUESTS (session/request_permission, has id, MUST be answered), then a
//     terminal response to the original session/prompt carrying a stopReason.
//   - Tools are REPORTED by the agent via session/update (sessionUpdate:
//     "tool_call" / "tool_call_update"); the agent EXECUTES them itself. The host
//     only consents via session/request_permission. The host does NOT run tools.
//   - Optional client methods (fs/*, terminal/*) are gated by client capabilities
//     declared at initialize. This broker advertises NONE, so a conformant agent
//     is spec-forbidden from calling them; if one does anyway, the broker replies
//     with a method-not-found error (defense-in-depth) rather than hanging.
//
// SCOPE LOCK: provider-agnostic protocol broker ONLY. It brokers the lifecycle
// (initialize → session/new → session/prompt stream → shutdown) and carries
// identifiers VERBATIM (the M3 call_id-fidelity lesson, now applied to the
// toolCallId the agent reports). It implements NO specific provider (Codex,
// Claude Code, Gemini CLI, Copilot are later, separately-approved onboarding),
// executes NO tools, and is NOT wired into the production router (the membrane
// keeps it off the Official path).

// --- JSON-RPC 2.0 envelope (matches internal/mcp conventions) ----------------

// jsonrpcRequest is a JSON-RPC 2.0 request/notification sent to the agent.
type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"` // omitted → notification
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonrpcError is the JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonrpcError) Error() string {
	return fmt.Sprintf("acp: rpc error %d: %s", e.Code, e.Message)
}

// jsonrpcResponse is a JSON-RPC 2.0 response sent to / received from the agent.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// CurrentProtocolVersion is the ACP MAJOR protocol version this broker speaks.
// Per spec the version is a single integer, incremented only on breaking changes.
const CurrentProtocolVersion = 1

// --- Initialization payloads -------------------------------------------------

// FSCapability declares filesystem client capabilities. The minimum broker
// advertises neither (both false / nil), so a conformant agent MUST NOT call
// fs/read_text_file or fs/write_text_file.
type FSCapability struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// ClientCapabilities is what the host advertises at initialize. The spec rule:
// any capability omitted is treated as UNSUPPORTED. The minimum onboarding-ready
// broker advertises NONE (zero value → fs nil, terminal false), which is exactly
// what keeps fs/terminal handling out of scope: the agent is spec-forbidden from
// invoking those methods.
type ClientCapabilities struct {
	FS       *FSCapability `json:"fs,omitempty"`
	Terminal bool          `json:"terminal,omitempty"`
}

// InitializeParams negotiates protocol version + client capabilities.
type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         map[string]any     `json:"clientInfo,omitempty"`
}

// InitializeResult is the agent's handshake reply: the chosen protocol version,
// the agent capabilities, optional info, and the auth methods it supports.
type InitializeResult struct {
	ProtocolVersion   int             `json:"protocolVersion"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities,omitempty"`
	AgentInfo         map[string]any  `json:"agentInfo,omitempty"`
	AuthMethods       json.RawMessage `json:"authMethods,omitempty"`
}

// NewSessionResult carries the session id the agent allocated.
type NewSessionResult struct {
	SessionID string `json:"sessionId"`
}

// PromptParams sends one turn to a session. Prompt is the ContentBlock[] payload
// (text/image/resource …); kept as `any` so the broker stays content-agnostic
// (the spec restricts content types by the negotiated prompt capabilities, which
// the minimum broker leaves at the agent's default).
type PromptParams struct {
	SessionID string `json:"sessionId"`
	Prompt    any    `json:"prompt"`
}

// --- session/update notification payloads ------------------------------------

// SessionNotification is the params of a `session/update` notification.
type SessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate is the (polymorphic) update body. `SessionUpdate` is the
// discriminator; the broker reads only the fields the minimum turn needs and
// safely ignores the rest (unknown sessionUpdate kinds are dropped, per the
// spec's forward-compatibility posture).
type SessionUpdate struct {
	SessionUpdate string          `json:"sessionUpdate"`
	ToolCallID    string          `json:"toolCallId,omitempty"`
	Kind          string          `json:"kind,omitempty"`
	Status        string          `json:"status,omitempty"`
	Title         string          `json:"title,omitempty"`
	Content       json.RawMessage `json:"content,omitempty"`
}

// --- session/request_permission payloads -------------------------------------

// PermissionOption is one choice the agent offers for a permission request.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // allow_once | allow_always | reject_once | reject_always
}

// PermissionRequest is the params of an agent→client `session/request_permission`.
type PermissionRequest struct {
	SessionID  string             `json:"sessionId"`
	ToolCallID string             `json:"toolCallId"`
	Options    []PermissionOption `json:"options"`
}

// PermissionOutcome is the host's decision, sent back as the request result.
// Outcome is "selected" (with OptionID) or "cancelled".
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// PermissionHandler decides how to answer a `session/request_permission`. The
// host supplies it (prod policy is pluggable; the adapter ships a deterministic
// default). A nil handler means DENY: the broker selects a reject option if the
// agent offered one, else returns "cancelled" — so the agent can terminate
// cleanly instead of hanging.
type PermissionHandler func(ctx context.Context, req PermissionRequest) PermissionOutcome

// --- prompt turn result ------------------------------------------------------

// PromptResult is the terminal outcome of a prompt turn. StopReason + Content
// come from the agent's response to session/prompt; Text and ToolCalls are
// assembled by the broker from the session/update stream (convenience for the
// acceptance gate: Text proves streaming happened, ToolCalls proves the tool
// loop ran and lets the harness assert identifier fidelity).
type PromptResult struct {
	StopReason string          `json:"stopReason,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"`
	Text       string          `json:"-"`
	ToolCalls  []string        `json:"-"`
}

// --- protocol errors ---------------------------------------------------------

var (
	// ErrACPClosed is returned when an operation is attempted on a closed client.
	ErrACPClosed = errors.New("acp: client closed")
	// ErrACPProtocol indicates a malformed/unexpected message from the agent.
	ErrACPProtocol = errors.New("acp: protocol error")
)

// ACPClient drives an ACP agent over an E1 Subprocess. It is the protocol broker
// only: it frames JSON-RPC, sequences the lifecycle, drains the prompt-turn
// notification stream, and answers agent→client permission requests via a
// host-supplied PermissionHandler. It owns request-id allocation.
//
// CONCURRENCY: one in-flight exchange per client (the underlying Subprocess is
// single-flight). Drive Initialize/NewSession/Prompt sequentially; pool multiple
// ACPClients for concurrency, per the E1 contract.
type ACPClient struct {
	proc *Subprocess

	mu     sync.Mutex
	nextID int
	closed bool
}

// NewACPClient wraps a (not-yet-started) Subprocess as an ACP client.
func NewACPClient(proc *Subprocess) *ACPClient {
	return &ACPClient{proc: proc, nextID: 1}
}

// Start launches the underlying agent subprocess.
func (c *ACPClient) Start(ctx context.Context) error {
	if c.proc == nil {
		return errors.New("acp: nil subprocess")
	}
	return c.proc.Start(ctx)
}

// Close shuts the agent down (graceful → force-kill via the E1 harness).
func (c *ACPClient) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return c.proc.Stop()
}

// allocID returns the next JSON-RPC request id.
func (c *ACPClient) allocID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

// call sends a SYNCHRONOUS JSON-RPC request and reads exactly one response,
// decoding Result into out. It is for request/response methods only (initialize,
// session/new, shutdown) — NOT the prompt turn, which can interleave
// notifications + agent→client requests and is driven by Prompt instead.
func (c *ACPClient) call(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrACPClosed
	}

	id := c.allocID()
	reqBytes, err := json.Marshal(jsonrpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("acp: marshal %s: %w", method, err)
	}
	respBytes, err := c.proc.Request(ctx, reqBytes)
	if err != nil {
		return err // already a contained E1 error (timeout/exit/etc.)
	}
	var resp jsonrpcResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return fmt.Errorf("%w: bad response json: %v", ErrACPProtocol, err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("%w: bad result json: %v", ErrACPProtocol, err)
		}
	}
	return nil
}

// Initialize performs the ACP handshake (integer protocol version + client
// capabilities). It returns the agent's chosen version + capabilities; version
// negotiation/compatibility is the caller's (adapter's) decision.
func (c *ACPClient) Initialize(ctx context.Context, params InitializeParams) (*InitializeResult, error) {
	var res InitializeResult
	if err := c.call(ctx, "initialize", params, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// NewSession opens a new agent session and returns its id.
func (c *ACPClient) NewSession(ctx context.Context, params any) (string, error) {
	var res NewSessionResult
	if err := c.call(ctx, "session/new", params, &res); err != nil {
		return "", err
	}
	if res.SessionID == "" {
		return "", fmt.Errorf("%w: session/new returned empty sessionId", ErrACPProtocol)
	}
	return res.SessionID, nil
}

// Prompt sends one turn and drives the agent loop to a terminal PromptResult.
//
// After writing session/prompt, the broker reads frames in a loop and dispatches
// each by shape:
//   - NOTIFICATION (has method, no id) → a session/update; folded into turn state
//     (text chunks accumulated; tool_call ids tracked). NEVER replied to.
//   - AGENT→CLIENT REQUEST (has method AND id) → dispatched: session/request_permission
//     is answered via onPermission (or denied if nil); any fs/* or terminal/*
//     method gets a method-not-found error (we advertised no such capability).
//   - RESPONSE (no method) matching the prompt id → the terminal result; returns.
//
// The loop ends when the agent responds to the original session/prompt with a
// stopReason. A crash/hang is contained by E1 (ReadLine deadline) and surfaced as
// a contained error. IMPORTANT: the broker NEVER executes a tool — the agent runs
// its own tools and reports them; the host only consents.
//
// TURN BOUND: pass a ctx with a turn-level deadline so the whole turn is bounded
// (each ReadLine inherits the ctx deadline). With no ctx deadline, each frame is
// bounded by the Subprocess RequestTimeout instead.
func (c *ACPClient) Prompt(ctx context.Context, params PromptParams, onPermission PermissionHandler) (*PromptResult, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrACPClosed
	}
	c.mu.Unlock()

	promptID := c.allocID()
	reqBytes, err := json.Marshal(jsonrpcRequest{JSONRPC: "2.0", ID: promptID, Method: "session/prompt", Params: params})
	if err != nil {
		return nil, fmt.Errorf("acp: marshal prompt: %w", err)
	}
	promptIDBytes, _ := json.Marshal(promptID)

	if err := c.proc.WriteLine(ctx, reqBytes); err != nil {
		return nil, err
	}

	result := &PromptResult{}
	seen := map[string]bool{}

	for {
		frame, err := c.proc.ReadLine(ctx)
		if err != nil {
			return nil, err
		}

		var probe struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id,omitempty"`
			Method  string          `json:"method,omitempty"`
			Params  json.RawMessage `json:"params,omitempty"`
			Result  json.RawMessage `json:"result,omitempty"`
			Error   *jsonrpcError   `json:"error,omitempty"`
		}
		if err := json.Unmarshal(frame, &probe); err != nil {
			return nil, fmt.Errorf("%w: bad frame json: %v", ErrACPProtocol, err)
		}

		// Frames carrying a method are either notifications or agent→client requests.
		if probe.Method != "" {
			// Notification: no id (or null id) → fold, never reply.
			if len(probe.ID) == 0 || string(bytes.TrimSpace(probe.ID)) == "null" {
				foldNotification(probe.Method, probe.Params, result, seen)
				continue
			}
			// Agent→client request: dispatch + reply with the SAME id verbatim.
			replyResult, rpcErr := c.dispatchAgentRequest(ctx, probe.Method, probe.Params, onPermission, result, seen)
			resp := jsonrpcResponse{JSONRPC: "2.0", ID: probe.ID}
			if rpcErr != nil {
				resp.Error = rpcErr
			} else {
				resp.Result = replyResult
			}
			replyBytes, merr := json.Marshal(resp)
			if merr != nil {
				return nil, fmt.Errorf("acp: marshal reply: %w", merr)
			}
			if werr := c.proc.WriteLine(ctx, replyBytes); werr != nil {
				return nil, werr
			}
			continue
		}

		// No method → a response. If it carries an error, surface it.
		if probe.Error != nil {
			return nil, probe.Error
		}
		// Only the response to OUR prompt id is the terminal frame. Anything else
		// (a stray response in our single-flight model) is ignored defensively.
		if len(probe.ID) > 0 && !jsonValueEqual(probe.ID, promptIDBytes) {
			continue
		}
		var term PromptResult
		if len(probe.Result) > 0 {
			if err := json.Unmarshal(probe.Result, &term); err != nil {
				return nil, fmt.Errorf("%w: bad prompt result: %v", ErrACPProtocol, err)
			}
		}
		result.StopReason = term.StopReason
		result.Content = term.Content
		return result, nil
	}
}

// dispatchAgentRequest handles an agent→client request during a prompt turn. It
// returns either a result payload (to send back) or a jsonrpcError. It NEVER
// executes a tool; it consents (permission) or refuses (unadvertised method).
func (c *ACPClient) dispatchAgentRequest(ctx context.Context, method string, params json.RawMessage, onPermission PermissionHandler, result *PromptResult, seen map[string]bool) (json.RawMessage, *jsonrpcError) {
	switch method {
	case "session/request_permission":
		var req PermissionRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, &jsonrpcError{Code: -32602, Message: "invalid request_permission params"}
			}
		}
		// Track the toolCallId verbatim (identifier fidelity: the broker records
		// exactly what the agent sent; it never rewrites an id).
		if req.ToolCallID != "" && !seen[req.ToolCallID] {
			seen[req.ToolCallID] = true
			result.ToolCalls = append(result.ToolCalls, req.ToolCallID)
		}
		var outcome PermissionOutcome
		if onPermission == nil {
			outcome = denyOutcome(req)
		} else {
			outcome = onPermission(ctx, req)
		}
		return mustJSON(map[string]any{"outcome": outcome}), nil
	default:
		// fs/* and terminal/* (and anything else): we advertised NO client
		// capabilities, so a conformant agent must never call these. Reply with a
		// method-not-found error instead of hanging (defense-in-depth).
		return nil, &jsonrpcError{Code: -32601, Message: "method not supported: " + method + " (client capability not advertised)"}
	}
}

// Shutdown asks the agent to terminate the protocol session gracefully. Errors
// are non-fatal (Close still force-stops the process).
func (c *ACPClient) Shutdown(ctx context.Context) error {
	return c.call(ctx, "shutdown", nil, nil)
}

// foldNotification accumulates a session/update notification into the turn
// result. Unknown notification methods and unknown update kinds are ignored
// safely (spec forward-compat). It NEVER replies (notifications have no id).
func foldNotification(method string, params json.RawMessage, result *PromptResult, seen map[string]bool) {
	if method != "session/update" {
		return // e.g. an agent-side info notification we don't model — ignore.
	}
	var note SessionNotification
	if err := json.Unmarshal(params, &note); err != nil {
		return // malformed update: ignore rather than break the turn.
	}
	switch note.Update.SessionUpdate {
	case "agent_message_chunk", "agent_thought_chunk":
		result.Text += extractText(note.Update.Content)
	case "tool_call":
		if note.Update.ToolCallID != "" && !seen[note.Update.ToolCallID] {
			seen[note.Update.ToolCallID] = true
			result.ToolCalls = append(result.ToolCalls, note.Update.ToolCallID)
		}
	case "tool_call_update":
		// Status transitions for an already-reported tool call. Nothing to fold
		// for the minimum broker; the toolCallId was tracked on the initial
		// tool_call. (A real agent may send tool_call_update before tool_call in
		// edge cases — track defensively.)
		if note.Update.ToolCallID != "" && !seen[note.Update.ToolCallID] {
			seen[note.Update.ToolCallID] = true
			result.ToolCalls = append(result.ToolCalls, note.Update.ToolCallID)
		}
	}
}

// extractText pulls text from a content block (single object {type,text}) or an
// array of blocks. Non-text content is ignored.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var one struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &one) == nil && one.Type == "text" {
		return one.Text
	}
	var many []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &many) == nil {
		var sb strings.Builder
		for _, b := range many {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	return ""
}

// denyOutcome is the nil-handler default: pick a reject option if the agent
// offered one, else cancel. Either way the agent can terminate cleanly.
func denyOutcome(req PermissionRequest) PermissionOutcome {
	for _, o := range req.Options {
		if o.Kind == "reject_once" || o.Kind == "reject_always" {
			return PermissionOutcome{Outcome: "selected", OptionID: o.OptionID}
		}
	}
	return PermissionOutcome{Outcome: "cancelled"}
}

// jsonValueEqual reports whether two JSON byte slices encode the same value,
// independent of formatting/whitespace (used to match the prompt response id).
func jsonValueEqual(a, b []byte) bool {
	var ia, ib any
	if json.Unmarshal(a, &ia) != nil {
		return false
	}
	if json.Unmarshal(b, &ib) != nil {
		return false
	}
	na, _ := json.Marshal(ia)
	nb, _ := json.Marshal(ib)
	return bytes.Equal(na, nb)
}

// mustJSON marshals v to json.RawMessage, returning null on error (never panics).
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

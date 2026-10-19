// build_request.go — port of 9router open-sse/utils/cursorProtobuf.js
// (v0.5.104) `encodeRequest` + `buildChatRequest` + `buildToolResultRequest`
// + `generateCursorBody` + `generateToolResultBody`. These are the SHAPE
// helpers the Cursor executor calls when Lintasan has a real Cursor session
// (from a Cursor state.vscdb import via the oauthide Cursor provider).
//
// Shape contract pinned by the 9router executor, line-for-line:
//   - The StreamUnifiedChatRequestWithTools message is a SINGLE wrapped
//     field (field 1 = request) inside a ConnectRPC frame.
//   - Static wire fields (unknown1/2/4/13/19/38/47/51/53) are pinned by
//     the schema; removing them causes a Cursor 400 UNSUPPORTED_REQUEST.
//   - Every message carries an `isLast` indicator on the wire so Cursor's
//     server-side tool tracker knows when the call chain closes.
//   - Tool results travel as a SEPARATE request frame (field 2 =
//     client_side_tool_v2_result) — not inside the message loop.
package cursor

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ToolResult is the OpenAI-shaped result 9router's Cursor executor accepts.
// Mirrors the JS tool_result shape.
type ToolResult struct {
	ToolCallID    string `json:"tool_call_id"`
	ToolName      string `json:"tool_name"`
	ResultContent string `json:"result_content"`
}

// parseToolId mirrors 9router parseToolId: a "call_<uuid>_<modelCallId>"
// shape is what Cursor's server-side tracker uses; the executor splits it
// and carries the two halves as separate fields on the wire.
func parseToolID(id string) (toolCallID, modelCallID string) {
	// 9router's parseToolId splits on "_": the first segment is "call",
	// the second is the call uuid, the third is the model call id (when
	// present). No third segment = empty modelCallID.
	parts := strings.SplitN(id, "_", 3)
	if len(parts) < 2 {
		return id, ""
	}
	toolCallID = parts[1]
	if len(parts) >= 3 {
		modelCallID = parts[2]
	}
	return toolCallID, modelCallID
}

// stripMCPPrefix mirrors the 9router executor's selected_tool
// simplification (per the cursor-api Rust source McpResult.selected_tool):
// the mcp_custom_/mcp_ prefix is a server-side bucket tag, not part of
// the caller's real tool name.
func stripMCPPrefix(name string) string {
	switch {
	case strings.HasPrefix(name, "mcp_custom_"):
		return strings.TrimPrefix(name, "mcp_custom_")
	case strings.HasPrefix(name, "mcp_"):
		return strings.TrimPrefix(name, "mcp_")
	}
	return name
}

// EncodeInstruction wraps an instruction block (field 3 on
// StreamUnifiedChatRequest). Empty instruction → empty block (still
// carries the field presence marker; mirrors the JS's `encodeField(...) if
// text else empty` pattern).
func EncodeInstruction(text string) []byte {
	if text == "" {
		return nil
	}
	return EncodeLenField(FieldInstruction, []byte(text))
}

// EncodeModel wraps the model name in Cursor's two-field block:
// field 1 = name, field 4 = empty buffer (per the schema's MODEL_NAME /
// MODEL_EMPTY constants). Mirrors the JS encodeModel.
func EncodeModel(modelName string) []byte {
	return Concat(
		EncodeStringField(1, modelName),
		EncodeLenField(4, nil),
	)
}

// EncodeCursorSetting is the pinned "cursor\aisettings" body the JS
// executor emits (field 15 on StreamUnifiedChatRequest). The unknown
// sub-fields are part of the 1.1.3 schema; reordering them changes the
// wire shape Cursor's parser accepts.
func EncodeCursorSetting() []byte {
	// unknown6 = CursorSetting.Unknown6{field1: empty, field2: empty}.
	unknown6 := Concat(
		EncodeLenField(1, nil),
		EncodeLenField(2, nil),
	)
	return Concat(
		// path (literal; note the JS source escapes `cursor\aisettings`
		// which means the actual byte string is `cursor\aisettings` — a
		// single backslash, NOT an escape sequence because the JS source
		// is itself a double-escaped string).
		EncodeStringField(1, `cursor\aisettings`),
		EncodeLenField(3, nil),
		EncodeLenField(6, unknown6),
		EncodeVarintField(8, 1),
		EncodeVarintField(9, 1),
	)
}

// EncodeMetadata is the process-shape metadata the JS executor sends
// (field 26). All five fields are LEN-typed strings. Mirrors the JS's
// encodeMetadata (platform/arch/version/cwd/timestamp).
func EncodeMetadata(platform, arch, version, cwd, timestamp string) []byte {
	return Concat(
		EncodeStringField(1, defaultIfEmpty(platform, "linux")),
		EncodeStringField(2, defaultIfEmpty(arch, "x64")),
		EncodeStringField(3, defaultIfEmpty(version, "v20.0.0")),
		EncodeStringField(4, defaultIfEmpty(cwd, "/")),
		EncodeStringField(5, defaultIfEmpty(timestamp, "")),
	)
}

// EncodeMessageID is the field-30 (MESSAGE_IDS) entry: field 1 = message id,
// field 2 = summary (optional), field 3 = role varint.
func EncodeMessageID(messageID, summaryID string, role int) []byte {
	parts := [][]byte{
		EncodeStringField(1, messageID),
	}
	if summaryID != "" {
		parts = append(parts, EncodeStringField(2, summaryID))
	}
	parts = append(parts, EncodeVarintField(3, uint64(role)))
	return Concat(parts...)
}

// EncodeMCPTool is one entry in field 34 (MCP_TOOLS), shape-pinned by the
// JS encodeMcpTool: name/desc/params/server="custom".
func EncodeMCPTool(toolName, toolDesc, jsonParams string) []byte {
	var parts [][]byte
	if toolName != "" {
		parts = append(parts, EncodeStringField(1, toolName))
	}
	if toolDesc != "" {
		parts = append(parts, EncodeStringField(2, toolDesc))
	}
	if jsonParams != "" && jsonParams != "{}" {
		parts = append(parts, EncodeStringField(3, jsonParams))
	}
	parts = append(parts, EncodeStringField(4, "custom"))
	return Concat(parts...)
}

// EncodeMessage encodes ONE ConversationMessage (field 1 on
// StreamUnifiedChatRequest). Mirrors the JS encodeMessage exactly:
// content / role / id / [server_bubble_id] / [tool_results] /
// is_agentic / unified_mode / [supported_tools-if-last].
//
// toolResults are PRE-ENCODED (caller's own ClientSideToolV2Result
// blocks) — this avoids the JS source's nested encodeToolResult
// (which is itself a 3-layer wrapper). Keep the caller in charge.
func EncodeMessage(content string, role int, messageID string, isLast bool, hasTools bool, toolResults [][]byte, serverBubbleID string) []byte {
	parts := [][]byte{
		EncodeStringField(FieldMsgContent, content),
		EncodeVarintField(FieldMsgRole, uint64(role)),
		EncodeStringField(FieldMsgID, messageID),
	}
	if serverBubbleID != "" {
		parts = append(parts, EncodeStringField(FieldMsgServerBubbleID, serverBubbleID))
	}
	for _, tr := range toolResults {
		parts = append(parts, EncodeLenField(FieldMsgToolResults, tr))
	}
	agentic := uint64(0)
	mode := uint64(UnifiedModeChat)
	if hasTools {
		agentic = 1
		mode = UnifiedModeAgent
	}
	parts = append(parts, EncodeVarintField(FieldMsgIsAgentic, agentic))
	parts = append(parts, EncodeVarintField(FieldMsgUnifiedMode, mode))
	if isLast && hasTools {
		parts = append(parts, EncodeLenField(FieldMsgSupportedTools, EncodeVarint(1)))
	}
	return Concat(parts...)
}

// ChatMessage is the Lintasan caller's shape — one OpenAI-ish message.
// Mirrors the JS executor's message walk, including the mixed assistant
// guardrail (tool_calls and tool_results together on one assistant
// message must be split into TWO messages so the protobuf shape stays
// valid on the wire).
type ChatMessage struct {
	Role        string           `json:"role"`
	Content     string           `json:"content"`
	ToolCalls   []map[string]any `json:"tool_calls,omitempty"`
	ToolResults []ToolResult     `json:"tool_results,omitempty"`
}

// normalizeMixedAssistant mirrors the JS executor's guardrail: a single
// assistant message that carries BOTH tool_calls AND tool_results is
// split into TWO messages (call message + result message), unless the
// NEXT message in the array already carries the same tool_results (the
// dedupe path in the JS source).
func normalizeMixedAssistant(messages []ChatMessage) []ChatMessage {
	var out []ChatMessage
	for i, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 && len(msg.ToolResults) > 0 {
			// Push the tool-calls message (no results).
			out = append(out, ChatMessage{Role: msg.Role, Content: msg.Content, ToolCalls: msg.ToolCalls})
			// Skip the result push when the NEXT message already carries
			// the same tool_call ids (the JS source's `sameIds` check).
			if i+1 < len(messages) {
				next := messages[i+1]
				if next.Role == "assistant" && sameToolResultIDs(msg.ToolResults, next.ToolResults) {
					continue
				}
			}
			// Otherwise push a separate results message.
			out = append(out, ChatMessage{Role: "assistant", Content: "", ToolResults: msg.ToolResults})
			continue
		}
		out = append(out, msg)
	}
	return out
}

func sameToolResultIDs(a, b []ToolResult) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, tr := range a {
		seen[tr.ToolCallID] = true
	}
	for _, tr := range b {
		if !seen[tr.ToolCallID] {
			return false
		}
	}
	return true
}

// BuildChatRequest assembles the whole StreamUnifiedChatRequest
// (field 1 on StreamUnifiedChatRequestWithTools) — the same order
// the JS encodeRequest emits. Static fields are LITERAL constants from
// the 1.1.3 schema; reordering them changes the wire shape.
//
// The caller's conversationID and timestamp come from outside so tests
// can pin them without stubbing crypto/uuid.
func BuildChatRequest(
	messages []ChatMessage,
	modelName string,
	mcpTools [][]byte, // pre-encoded via EncodeMCPTool
	reasoningEffort string, // "", "medium", or "high"
	forceAgentMode bool,
	conversationID string,
	timestamp string,
) ([]byte, error) {
	hasTools := len(mcpTools) > 0
	isAgentic := hasTools || forceAgentMode

	// Reasoning effort → thinking level.
	thinkingLevel := uint64(ThinkingLevelUnspecified)
	switch reasoningEffort {
	case "medium":
		thinkingLevel = ThinkingLevelMedium
	case "high":
		thinkingLevel = ThinkingLevelHigh
	}

	// Split mixed assistant tool payloads before the message loop.
	normalized := normalizeMixedAssistant(messages)

	// Build the message blocks and their parallel message_ids list.
	var parts [][]byte
	var messageIDs [][]byte
	for i, msg := range normalized {
		role := RoleUser
		if msg.Role != "user" {
			role = RoleAssistant
		}
		messageID := newUUID()
		isLast := i == len(normalized)-1
		var toolResults [][]byte
		for _, tr := range msg.ToolResults {
			enc, err := BuildToolResultRequest(tr)
			if err != nil {
				return nil, err
			}
			// Tool results ride INSIDE the message loop as the
			// ConversationMessage.ToolResult nested field (per the JS
			// encodeMessage's `toolResults` argument), not as a top-level
			// field — the top-level field-2 path is the OUT-OF-BAND
			// frame, used when the tool result arrives as a separate
			// request (see BuildToolResultRequest).
			toolResults = append(toolResults, enc)
		}
		parts = append(parts, EncodeLenField(FieldMessages,
			EncodeMessage(msg.Content, role, messageID, isLast, hasTools, toolResults, "")))
		messageIDs = append(messageIDs,
			EncodeLenField(FieldMessageIDs, EncodeMessageID(messageID, "", role)))
	}

	// Static fields pinned by the schema; see the 9router JS source for
	// their ordering. The order in this Concat MATTERS.
	parts = append(parts,
		EncodeVarintField(2, 1),               // UNKNOWN_2
		EncodeLenField(FieldInstruction, nil), // empty instruction block
		EncodeVarintField(4, 1),               // UNKNOWN_4
		EncodeLenField(FieldModel, EncodeModel(modelName)),
		EncodeLenField(FieldWebTool, nil),
		EncodeVarintField(13, 1), // UNKNOWN_13
		EncodeLenField(15, EncodeCursorSetting()),
		EncodeVarintField(19, 1), // UNKNOWN_19
		EncodeStringField(FieldConversationID, conversationID),
		EncodeLenField(FieldMetadata, EncodeMetadata("", "", "", "", timestamp)),
		EncodeVarintField(FieldIsAgentic, bool64(isAgentic)),
	)
	if isAgentic {
		parts = append(parts, EncodeLenField(FieldSupportedTools, EncodeVarint(1)))
	}
	parts = append(parts, messageIDs...)
	for _, t := range mcpTools {
		parts = append(parts, EncodeLenField(FieldMCPTools, t))
	}
	parts = append(parts,
		EncodeVarintField(35, 0), // LARGE_CONTEXT
		EncodeVarintField(38, 0), // UNKNOWN_38
		EncodeVarintField(46, modeFor(isAgentic)),
		EncodeLenField(47, nil),                   // UNKNOWN_47
		EncodeVarintField(48, bool64(!isAgentic)), // SHOULD_DISABLE_TOOLS (1 when chat)
		EncodeVarintField(49, thinkingLevel),
		EncodeVarintField(51, 0), // UNKNOWN_51
		EncodeVarintField(53, 1), // UNKNOWN_53
		EncodeStringField(54, modeNameFor(isAgentic)),
	)
	return Concat(parts...), nil
}

// BuildToolResultRequest is the SEPARATE top-level frame for a tool
// result (field 2 on StreamUnifiedChatRequestWithTools), pinned by the
// JS buildToolResultRequest. Shape: {tool: 19 (MCP), mcp_result: {1:
// selected_tool, 2: result}, tool_call_id, [model_call_id]} — NO
// tool_index (per the Rust source it stays None).
func BuildToolResultRequest(tr ToolResult) ([]byte, error) {
	toolCallID, modelCallID := parseToolID(tr.ToolCallID)
	selectedTool := stripMCPPrefix(tr.ToolName)

	// ClientSideToolV2Result block.
	cv2 := Concat(
		EncodeVarintField(1, ClientSideToolV2MCP),
		EncodeLenField(28, EncodeMCPResult(selectedTool, tr.ResultContent)),
		EncodeStringField(35, toolCallID),
	)
	if modelCallID != "" {
		cv2 = append(cv2, EncodeStringField(48, modelCallID)...)
	}
	// Top-level StreamUnifiedChatRequestWithTools.field 2.
	return EncodeLenField(2, cv2), nil
}

// EncodeMCPResult is the McpResult block inside ClientSideToolV2Result:
// field 1 = selected_tool, field 2 = result. Mirrors the JS's
// encodeMcpResult (which nests into MCPParams/tools[0]).
func EncodeMCPResult(selectedTool, resultContent string) []byte {
	return Concat(
		EncodeStringField(1, selectedTool),
		EncodeStringField(2, resultContent),
	)
}

// WrapConnectRPCFrame is the byte-exact 9router frame wrapper — flag
// 0x00 (uncompressed) + BE-32 length + payload. Cursor doesn't support
// compressed requests on this lane (per the JS `wrapConnectRPCFrame(
// protobuf, false)` call site), so a compressed=true path is rejected.
func WrapConnectRPCFrame(payload []byte, compressed bool) ([]byte, error) {
	if compressed {
		return nil, fmt.Errorf("cursor connect-rpc: compressed frame not supported on the unified-chat lane")
	}
	return GRPCWebFrame(payload), nil
}

// GenerateCursorBody is the caller's entry point: build + wrap in the
// top-level StreamUnifiedChatRequestWithTools frame. The outer wrapper is
// field 1 (REQUEST), so a downstream decoder first unwraps it, then walks
// the StreamUnifiedChatRequest. The conversationID reaches the inner
// message as its field 23.
func GenerateCursorBody(messages []ChatMessage, modelName string, mcpTools [][]byte, reasoningEffort string, forceAgentMode bool, conversationID, timestamp string) ([]byte, error) {
	inner, err := BuildChatRequest(messages, modelName, mcpTools, reasoningEffort, forceAgentMode, conversationID, timestamp)
	if err != nil {
		return nil, fmt.Errorf("cursor build request: %w", err)
	}
	// Wrap in field 1 (REQUEST) before framing — this is the
	// buildChatRequest entry in the 9router JS source.
	protobuf := EncodeLenField(FieldRequest, inner)
	framed, err := WrapConnectRPCFrame(protobuf, false)
	if err != nil {
		return nil, err
	}
	return framed, nil
}

// GenerateToolResultBody is the separate-frame entry point for tool
// results (per the JS's generateToolResultBody contract). The tool
// result body is ALREADY wrapped as field 2 by BuildToolResultRequest —
// no additional field 1 wrapping happens in this path.
func GenerateToolResultBody(tr ToolResult) ([]byte, error) {
	protobuf, err := BuildToolResultRequest(tr)
	if err != nil {
		return nil, fmt.Errorf("cursor build tool result: %w", err)
	}
	return WrapConnectRPCFrame(protobuf, false)
}

// ─── helpers ─────────────────────────────────────────────────────────────

func bool64(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

func modeFor(isAgentic bool) uint64 {
	if isAgentic {
		return UnifiedModeAgent
	}
	return UnifiedModeChat
}

func modeNameFor(isAgentic bool) string {
	if isAgentic {
		return "Agent"
	}
	return "Ask"
}

func defaultIfEmpty(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// newUUID mint a fresh UUID v4. Mirrors the JS's uuidv4 call; the shape
// is a 36-char canonical lowercase UUID — Cursor's parser requires that
// shape (an id of any other form is rejected with a 400 UNSUPPORTED).
// Uses crypto/rand for entropy (not a counter) so a fresh process gets
// a fresh id — matching the upstream executor's per-request mint.
func newUUID() string {
	// crypto/rand via the standard library. Mirrors the JS uuidv4
	// behaviour; a fresh call gets a fresh id. The shape is a canonical
	// lowercase UUID v4: 8-4-4-4-12 hex digits.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand never fails in practice; fall back to a
		// deterministic-but-unique UUID from the current time so a
		// missing crypto source still yields a well-formed id.
		return fmt.Sprintf("00000000-0000-4000-8000-%012x", time.Now().UnixNano())
	}
	// Set version (4) and variant (10xx) bits per RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// JSON helper for callers that want to inspect the decoded response; the
// JS source does the same when it needs a look at a non-SSE body. Only
// imported here so the file's helpers keep their JSON helper surface.
var _ = json.Unmarshal

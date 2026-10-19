package cursor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Each test pins one wire-shape contract from the 9router JS source
// (utils/cursorProtobuf.js v0.5.104). Failures describe WHICH upstream
// 400/403 shape they would produce, so a future edit's blast radius stays
// legible from the failure line alone.

func TestEncodeInstructionEmptyIsEmpty(t *testing.T) {
	// 9router's encodeInstruction returns a 0-byte buffer for an empty
	// text, so the wire does NOT carry an empty Instruction block.
	if got := EncodeInstruction(""); got != nil {
		t.Fatalf("EncodeInstruction(\"\") = %v, want nil", got)
	}
	// A non-empty instruction is a single field-3 (Instruction) block
	// carrying the text bytes directly. Per the JS encodeInstruction
	// contract the text is embedded bare inside field 3, NOT nested
	// under a field 1 wrapper — the Field.INSTRUCTION_TEXT constant
	// describes the INSTRUCTION block itself.
	got := EncodeInstruction("You are an AI assistant.")
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) != 1 || fields[0].Number != FieldInstruction || string(fields[0].Bytes) != "You are an AI assistant." {
		t.Fatalf("round-trip failed: %+v", fields)
	}
}

func TestEncodeModelShape(t *testing.T) {
	// Field 1 = model name, field 4 = empty bytes.
	got := EncodeModel("claude-sonnet-4.5")
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(fields))
	}
	if fields[0].Number != 1 || string(fields[0].Bytes) != "claude-sonnet-4.5" {
		t.Fatalf("field 1 = %v", fields[0])
	}
	if fields[1].Number != 4 || len(fields[1].Bytes) != 0 {
		t.Fatalf("field 4 = %v, want empty bytes", fields[1])
	}
}

func TestEncodeCursorSettingPathIsLiteralBackslash(t *testing.T) {
	// The JS source `cursor\aisettings` is a DOUBLE-escaped string — the
	// literal byte sequence on the wire is `cursor\aisettings` (one
	// backslash). A Go raw string preserves that byte-for-byte.
	got := EncodeCursorSetting()
	if !bytes.Contains(got, []byte(`cursor\aisettings`)) {
		t.Fatalf("path literal not on the wire: %v", got)
	}
	if bytes.Contains(got, []byte(`cursor\\aisettings`)) {
		t.Fatal("double backslash found — the JS source is NOT a double-escape")
	}
}

func TestEncodeCursorSettingSubfields(t *testing.T) {
	got := EncodeCursorSetting()
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// CursorSetting has fields 1,3,6,8,9 in that order.
	if len(fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(fields))
	}
	if fields[0].Number != 1 || fields[1].Number != 3 || fields[2].Number != 6 ||
		fields[3].Number != 8 || fields[4].Number != 9 {
		t.Fatalf("field order wrong: %v", fields)
	}
	// Field 8 and 9 are varint 1.
	if fields[3].WireType != WireTypeVarint || fields[3].Varint != 1 {
		t.Fatalf("field 8 = %v, want varint 1", fields[3])
	}
	if fields[4].WireType != WireTypeVarint || fields[4].Varint != 1 {
		t.Fatalf("field 9 = %v, want varint 1", fields[4])
	}
}

func TestEncodeMetadataDefaults(t *testing.T) {
	got := EncodeMetadata("", "", "", "", "")
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(fields))
	}
	// Defaults mirror the JS executor's process.* fallbacks.
	if string(fields[0].Bytes) != "linux" || string(fields[1].Bytes) != "x64" || string(fields[2].Bytes) != "v20.0.0" || string(fields[3].Bytes) != "/" {
		t.Fatalf("defaults: %v", fields)
	}
}

func TestEncodeMessageShapeChatModeNoTools(t *testing.T) {
	got := EncodeMessage("hi there", RoleUser, "msg-1", true, false, nil, "")
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Expected fields: content(1), role(2), id(13), is_agentic(29), unified_mode(47).
	// NO supported_tools(51) — that only fires when isLast && hasTools.
	if len(fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(fields))
	}
	if fields[0].Number != 1 || string(fields[0].Bytes) != "hi there" {
		t.Fatalf("content = %v", fields[0])
	}
	if fields[1].Number != 2 || fields[1].Varint != RoleUser {
		t.Fatalf("role = %v", fields[1])
	}
	if fields[2].Number != 13 || string(fields[2].Bytes) != "msg-1" {
		t.Fatalf("msg id = %v", fields[2])
	}
	if fields[3].Number != 29 || fields[3].Varint != 0 {
		t.Fatalf("is_agentic = %v, want 0", fields[3])
	}
	if fields[4].Number != 47 || fields[4].Varint != UnifiedModeChat {
		t.Fatalf("unified_mode = %v, want chat", fields[4])
	}
	// No supported_tools field on the wire.
	for _, f := range fields {
		if f.Number == FieldMsgSupportedTools {
			t.Fatal("supported_tools fired on a chat-mode message")
		}
	}
}

func TestEncodeMessageShapeAgentModeWithTools(t *testing.T) {
	// hasTools=true, isLast=true → unified_mode=AGENT and the trailing
	// supported_tools field fires (encodeVarint(1) inside a LEN wrapper).
	tr := EncodeLenField(2, EncodeMCPResult("Write", "ok"))
	got := EncodeMessage("do work", RoleAssistant, "msg-2", true, true, [][]byte{tr}, "bubble-1")
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	sawContent, sawBubble, sawAgentic, sawToolResults, sawMode, sawSupported := false, false, false, false, false, false
	for _, f := range fields {
		switch f.Number {
		case 1:
			sawContent = true
			if string(f.Bytes) != "do work" {
				t.Errorf("content = %q", f.Bytes)
			}
		case 18:
			sawToolResults = true
			if len(f.Bytes) == 0 {
				t.Error("tool_results payload empty")
			}
		case 29:
			sawAgentic = true
			if f.Varint != 1 {
				t.Errorf("is_agentic = %d, want 1", f.Varint)
			}
		case 32:
			sawBubble = true
			if string(f.Bytes) != "bubble-1" {
				t.Errorf("server_bubble_id = %q", f.Bytes)
			}
		case 47:
			sawMode = true
			if f.Varint != UnifiedModeAgent {
				t.Errorf("unified_mode = %d, want AGENT", f.Varint)
			}
		case 51:
			sawSupported = true
			if f.WireType != WireTypeLen || len(f.Bytes) == 0 {
				t.Errorf("supported_tools = %v", f)
			}
		}
	}
	if !sawContent || !sawBubble || !sawAgentic || !sawToolResults || !sawMode || !sawSupported {
		t.Fatalf("missing fields: content=%v bubble=%v agentic=%v toolResults=%v mode=%v supported=%v",
			sawContent, sawBubble, sawAgentic, sawToolResults, sawMode, sawSupported)
	}
	// supported_tools only fires when isLast && hasTools.
	got = EncodeMessage("middle", RoleAssistant, "m", false, true, nil, "")
	fields, _ = DecodeFields(got)
	for _, f := range fields {
		if f.Number == FieldMsgSupportedTools {
			t.Fatal("supported_tools fired on a non-last message")
		}
	}
}

func TestEncodeMessageIDShape(t *testing.T) {
	got := EncodeMessageID("msg-1", "", RoleUser)
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("got %d fields, want 2 (no summary)", len(fields))
	}
	if fields[0].Number != 1 || string(fields[0].Bytes) != "msg-1" {
		t.Fatalf("field 1 = %v", fields[0])
	}
	if fields[1].Number != 3 || fields[1].Varint != RoleUser {
		t.Fatalf("field 3 = %v", fields[1])
	}
	// With summary: 3 fields.
	got = EncodeMessageID("m", "s", RoleAssistant)
	fields, _ = DecodeFields(got)
	if len(fields) != 3 {
		t.Fatalf("got %d fields with summary, want 3", len(fields))
	}
	if fields[1].Number != 2 || string(fields[1].Bytes) != "s" {
		t.Fatalf("summary = %v", fields[1])
	}
}

func TestEncodeMCPToolShape(t *testing.T) {
	got := EncodeMCPTool("bash", "run a shell command", `{"type":"object","properties":{}}`)
	fields, err := DecodeFields(got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) != 4 {
		t.Fatalf("got %d fields, want 4", len(fields))
	}
	if fields[0].Number != 1 || string(fields[0].Bytes) != "bash" {
		t.Fatalf("tool name = %v", fields[0])
	}
	if fields[1].Number != 2 || string(fields[1].Bytes) != "run a shell command" {
		t.Fatalf("tool desc = %v", fields[1])
	}
	if fields[2].Number != 3 || !json.Valid(fields[2].Bytes) {
		t.Fatalf("tool params = %v (must be valid JSON)", fields[2])
	}
	if fields[3].Number != 4 || string(fields[3].Bytes) != "custom" {
		t.Fatalf("server = %v, want 'custom'", fields[3])
	}
}

func TestNormalizeMixedAssistantSplitsAndDedupes(t *testing.T) {
	// One assistant message with both tool_calls and tool_results → TWO
	// messages on the wire (call-then-result) — that's the 9router
	// guardrail against protobuf encode errors on mixed payloads.
	input := []ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "running",
			ToolCalls: []map[string]any{{"name": "bash"}},
			ToolResults: []ToolResult{
				{ToolCallID: "call_1", ToolName: "bash", ResultContent: "ok"},
			}},
	}
	out := normalizeMixedAssistant(input)
	if len(out) != 3 {
		t.Fatalf("got %d messages, want 3 (split into call + result)", len(out))
	}
	if len(out[1].ToolResults) != 0 || len(out[1].ToolCalls) != 1 {
		t.Fatalf("call message carries wrong shape: %+v", out[1])
	}
	if len(out[2].ToolResults) != 1 || len(out[2].ToolCalls) != 0 {
		t.Fatalf("result message carries wrong shape: %+v", out[2])
	}

	// When the NEXT message already carries the same tool_call ids, the
	// dedupe path keeps the loop at 2 (call-message only + the next one).
	dup := []ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "running",
			ToolCalls:   []map[string]any{{"name": "bash"}},
			ToolResults: []ToolResult{{ToolCallID: "call_1", ToolName: "bash", ResultContent: "ok"}}},
		{Role: "assistant", Content: "",
			ToolResults: []ToolResult{{ToolCallID: "call_1", ToolName: "bash", ResultContent: "ok"}}},
	}
	out = normalizeMixedAssistant(dup)
	if len(out) != 3 {
		t.Fatalf("dedupe failed: got %d, want 3", len(out))
	}
}

func TestSameToolResultIDsEmptyIsFalse(t *testing.T) {
	// An empty `a` never counts as "same" (the JS source's `size > 0`
	// guard); an empty-against-empty pair must not silently dedupe.
	if sameToolResultIDs(nil, nil) {
		t.Fatal("empty-vs-empty counted as same")
	}
	if sameToolResultIDs(nil, []ToolResult{{ToolCallID: "x"}}) {
		t.Fatal("empty vs non-empty counted as same")
	}
}

func TestBuildChatRequestShapeEndToEnd(t *testing.T) {
	body, err := BuildChatRequest(
		[]ChatMessage{{Role: "user", Content: "hi"}},
		"claude-sonnet-4.5",
		nil,   // no tools → chat mode
		"",    // no reasoning effort
		false, // not agent
		"conv-uuid",
		"2026-10-19T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("BuildChatRequest: %v", err)
	}
	fields, err := DecodeFields(body)
	if err != nil {
		t.Fatalf("DecodeFields: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("no fields emitted")
	}
	// Field-order walk against the JS encodeRequest's wire order:
	// 1 (messages), 2, 3 (instruction), 4, 5 (model), 8 (web_tool),
	// 13, 15 (cursor_setting), 19, 23 (conversation_id), 26 (metadata),
	// 27 (is_agentic), 30 (message_ids), 35, 38, 46, 47, 48, 49, 51, 53, 54.
	wantOrder := []int{1, 2, 3, 4, 5, 8, 13, 15, 19, 23, 26, 27, 30, 35, 38, 46, 47, 48, 49, 51, 53, 54}
	gotOrder := make([]int, 0, len(fields))
	for _, f := range fields {
		gotOrder = append(gotOrder, f.Number)
	}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("field count = %d, want %d\norder=%v", len(gotOrder), len(wantOrder), gotOrder)
	}
	for i, want := range wantOrder {
		if gotOrder[i] != want {
			t.Fatalf("field[%d] = %d, want %d (full order = %v)", i, gotOrder[i], want, gotOrder)
		}
	}
	// Chat mode pins: unified_mode=CHAT, should_disable_tools=1.
	for _, f := range fields {
		if f.Number == 46 && f.Varint != UnifiedModeChat {
			t.Fatalf("unified_mode = %d, want CHAT", f.Varint)
		}
		if f.Number == 48 && f.Varint != 1 {
			t.Fatalf("should_disable_tools = %d, want 1 (chat mode disables tools)", f.Varint)
		}
	}
	// Conversation ID must be the caller-supplied one, not an internal UUID.
	for _, f := range fields {
		if f.Number == FieldConversationID && string(f.Bytes) != "conv-uuid" {
			t.Fatalf("conversation_id = %q, want 'conv-uuid'", f.Bytes)
		}
	}
	// Unified mode name is the literal "Ask" for a chat body.
	for _, f := range fields {
		if f.Number == FieldUnifiedModeName && string(f.Bytes) != "Ask" {
			t.Fatalf("unified_mode_name = %q, want 'Ask'", f.Bytes)
		}
	}
}

func TestBuildChatRequestAgentModeSetsAgentShape(t *testing.T) {
	tool := EncodeMCPTool("bash", "run a shell command", `{}`)
	body, err := BuildChatRequest(
		[]ChatMessage{{Role: "user", Content: "hi"}},
		"claude-sonnet-4.5",
		[][]byte{tool},
		"high", // reasoning effort
		false,  // not forceAgent; the presence of tools makes it agentic
		"conv-uuid",
		"2026-10-19T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("BuildChatRequest: %v", err)
	}
	fields, _ := DecodeFields(body)
	for _, f := range fields {
		if f.Number == 46 && f.Varint != UnifiedModeAgent {
			t.Fatalf("unified_mode = %d, want AGENT (tools present)", f.Varint)
		}
		if f.Number == 48 && f.Varint != 0 {
			t.Fatalf("should_disable_tools = %d, want 0 (agent mode keeps tools)", f.Varint)
		}
		if f.Number == 49 && f.Varint != ThinkingLevelHigh {
			t.Fatalf("thinking_level = %d, want HIGH", f.Varint)
		}
		if f.Number == FieldUnifiedModeName && string(f.Bytes) != "Agent" {
			t.Fatalf("unified_mode_name = %q, want 'Agent'", f.Bytes)
		}
		if f.Number == FieldMCPTools && len(f.Bytes) == 0 {
			t.Fatalf("MCP tool field is empty")
		}
	}
}

func TestBuildToolResultRequestShape(t *testing.T) {
	body, err := BuildToolResultRequest(ToolResult{
		ToolCallID:    "call_abc123",
		ToolName:      "mcp_custom_Write",
		ResultContent: "file written",
	})
	if err != nil {
		t.Fatalf("BuildToolResultRequest: %v", err)
	}
	// The result is a top-level field 2 wrapper, decode it once.
	fields, err := DecodeFields(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) != 1 || fields[0].Number != 2 {
		t.Fatalf("expected field 2 wrapper, got %v", fields)
	}
	// Walk the ClientSideToolV2Result.
	inner, err := DecodeFields(fields[0].Bytes)
	if err != nil {
		t.Fatalf("decode cv2: %v", err)
	}
	sawMCP := false
	for _, f := range inner {
		if f.Number != 1 && f.Number != 28 && f.Number != 35 && f.Number != 48 {
			t.Fatalf("unexpected ClientSideToolV2Result field %d (tool_index must be absent per the Rust source)", f.Number)
		}
		if f.Number == 1 && f.Varint != ClientSideToolV2MCP {
			t.Fatalf("tool varint = %d, want %d (MCP)", f.Varint, ClientSideToolV2MCP)
		}
		if f.Number == 28 {
			sawMCP = true
			mcpFields, err := DecodeFields(f.Bytes)
			if err != nil {
				t.Fatalf("decode mcp_result: %v", err)
			}
			// McpResult: field 1 = selected_tool (stripped), field 2 = result.
			if len(mcpFields) != 2 {
				t.Fatalf("got %d mcp_result fields, want 2", len(mcpFields))
			}
			if string(mcpFields[0].Bytes) != "Write" {
				t.Fatalf("selected_tool = %q, want 'Write' (mcp_custom_ prefix stripped)", mcpFields[0].Bytes)
			}
			if string(mcpFields[1].Bytes) != "file written" {
				t.Fatalf("result = %q", mcpFields[1].Bytes)
			}
		}
		if f.Number == 35 && string(f.Bytes) != "abc123" {
			t.Fatalf("tool_call_id = %q, want 'abc123' (split off the leading 'call_')", f.Bytes)
		}
	}
	if !sawMCP {
		t.Fatal("mcp_result field missing from the ClientSideToolV2Result block")
	}
}

func TestStripMCPPrefixAllThreeShapes(t *testing.T) {
	if got := stripMCPPrefix("mcp_custom_Write"); got != "Write" {
		t.Fatalf("mcp_custom_ = %q", got)
	}
	if got := stripMCPPrefix("mcp_Write"); got != "Write" {
		t.Fatalf("mcp_ = %q", got)
	}
	if got := stripMCPPrefix("Write"); got != "Write" {
		t.Fatalf("no prefix = %q", got)
	}
	// mcp_custom_ wins over mcp_ (the longer prefix is tested first).
	if got := stripMCPPrefix("mcp_custom_mcp_thing"); got != "mcp_thing" {
		t.Fatalf("double-prefix = %q", got)
	}
}

func TestParseToolIDShape(t *testing.T) {
	// Cursor's tool-call id shape is "call_<uuid>_<modelCallId>".
	call, model := parseToolID("call_abc123_def456")
	if call != "abc123" || model != "def456" {
		t.Fatalf("call=%q model=%q", call, model)
	}
	// A two-segment id has no model call id.
	call, model = parseToolID("call_abc123")
	if call != "abc123" || model != "" {
		t.Fatalf("call=%q model=%q", call, model)
	}
	// Malformed ids pass through as the call id (JS early-return path).
	call, model = parseToolID("weird")
	if call != "weird" || model != "" {
		t.Fatalf("call=%q model=%q", call, model)
	}
}

func TestWrapConnectRPCFrameRejectsCompressed(t *testing.T) {
	// Cursor doesn't support compressed requests on the unified-chat lane
	// (per the JS `wrapConnectRPCFrame(protobuf, false)` call site). A
	// compressed=true call must fail LOUDLY rather than produce a frame
	// the upstream would answer 400 with.
	if _, err := WrapConnectRPCFrame([]byte{0x01}, true); err == nil {
		t.Fatal("compressed frame accepted; must be rejected")
	}
	// Uncompressed works and carries the right shape.
	got, err := WrapConnectRPCFrame([]byte{0x01, 0x02}, false)
	if err != nil {
		t.Fatalf("uncompressed: %v", err)
	}
	if got[0] != 0x00 || len(got) != 7 {
		t.Fatalf("frame = %v", got)
	}
}

func TestGenerateCursorBodyEndToEndFramed(t *testing.T) {
	framed, err := GenerateCursorBody(
		[]ChatMessage{{Role: "user", Content: "hi"}},
		"claude-sonnet-4.5", nil, "", false,
		"conv-uuid", "2026-10-19T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("GenerateCursorBody: %v", err)
	}
	// The framing must be peelable back to a valid protobuf body.
	frames, err := SplitGRPCWebFrames(framed)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	fields, err := DecodeFields(frames[0])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fields) == 0 || fields[0].Number != FieldRequest {
		t.Fatalf("outer frame must carry field 1 (request), got %v", fields)
	}
	// The inner body is the StreamUnifiedChatRequest; sanity-check the
	// conversation id reached it.
	inner, err := DecodeFields(fields[0].Bytes)
	if err != nil {
		t.Fatalf("decode inner: %v", err)
	}
	sawConv := false
	for _, f := range inner {
		if f.Number == FieldConversationID && string(f.Bytes) == "conv-uuid" {
			sawConv = true
		}
	}
	if !sawConv {
		t.Fatal("conversation id did not reach the inner StreamUnifiedChatRequest")
	}
}

func TestNewUUIDShape(t *testing.T) {
	// Cursor requires canonical 8-4-4-4-12 lowercase UUID v4.
	got := newUUID()
	parts := strings.Split(got, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatalf("uuid shape wrong: %q", got)
	}
	// Version/variant bits pinned by RFC 4122.
	if got[14] != '4' {
		t.Fatalf("version nibble = %c, want '4'", got[14])
	}
	if got[19] != '8' && got[19] != '9' && got[19] != 'a' && got[19] != 'b' {
		t.Fatalf("variant nibble = %c, want 8/9/a/b", got[19])
	}
	// Two calls in a row → two different ids.
	if got == newUUID() {
		t.Fatal("two calls produced the same uuid")
	}
	// Lowercase-only.
	if got != strings.ToLower(got) {
		t.Fatalf("uuid must be lowercase: %q", got)
	}
}

func TestToolResultRidesInsideMessageLoop(t *testing.T) {
	// Per the JS encodeMessage contract, tool_results ride INSIDE the
	// ConversationMessage.ToolResult nested field (field 18), not at the
	// top level. The top-level field 2 path is the OUT-OF-BAND frame used
	// when the tool result arrives as a separate request.
	tr, err := BuildToolResultRequest(ToolResult{
		ToolCallID:    "call_x",
		ToolName:      "bash",
		ResultContent: "ok",
	})
	if err != nil {
		t.Fatalf("BuildToolResultRequest: %v", err)
	}
	msg := EncodeMessage("doing work", RoleAssistant, "m", false, true, [][]byte{tr}, "")
	fields, _ := DecodeFields(msg)
	sawField18 := false
	for _, f := range fields {
		if f.Number == 18 {
			sawField18 = true
			// The bytes ARE the top-level field-2 wrapper shape, so
			// decoding them must show a field 2 (the wrapped ClientSide
			// ToolV2Result path rides inside it).
			inner, err := DecodeFields(f.Bytes)
			if err != nil {
				t.Fatalf("decode nested tool_results: %v", err)
			}
			if len(inner) != 1 || inner[0].Number != 2 {
				t.Fatalf("nested tool_results field shape wrong: %v", inner)
			}
		}
	}
	if !sawField18 {
		t.Fatal("tool_results not carried on field 18")
	}
}

// response.go — port of 9router (open-sse/utils/cursorProtobuf.js
// v0.5.104) RESPONSE PARSING block. This is the Response-SSE demuxer
// friend of internal/cursor/build_request.go: it takes the wire payload
// of one upstream ConnectRPC body and turns it into Lintasan's
// OpenAI-ish shape fields (text / tool-call / thinking / error).
//
// 9r executor contract:
//   - StreamUnifiedChatResponseWithTools response carries
//     TOOL_CALL=1 / RESPONSE=2 (top level).
//   - ClientSideToolV2Call (field 1) shape: TOOL_ID=3, TOOL_NAME=9,
//     TOOL_RAW_ARGS=10, TOOL_IS_LAST=11, TOOL_MCP_PARAMS=27 (with a
//     nested MCPParams.Tool shape). Cursor multi-line IDs get the
//     FIRST line taken (the source says: "Cursor returns multi-line
//     ID, take first line").
//   - StreamUnifiedChatResponse (field 2) shape: RESPONSE_TEXT=1,
//     THINKING=25 (nested, carrying THINKING_TEXT=1).
//   - UNKNOWN response fields are logged (we return detection info so
//     a caller can decide; the JS source just logs).
package cursor

import (
	"fmt"
	"strings"
)

// KnownResponseField is the set of field numbers Cursor's 1.1.3 wire
// serves on the RESPONSE lane. Note that FieldToolCall and Field-
// Response both carry number values inside nested blocks (FieldToolCall
// == nested FieldResponseText == 1) — the set includes the top-level
// numbers 1 and 2 plus their nested peers 3/9/10/11/27 and the nested
// response 1/25. A nested field 1 collides with the top-level
// ClientSideToolV2Call field 1 — that's fine, both are valid response
// fields, just at a different nesting level.
var KnownResponseFields = map[int]bool{
	FieldToolCall:      true, // 1 ClientSideToolV2Call (top level)
	FieldResponse:      true, // 2 StreamUnifiedChatResponse (top level)
	FieldToolID:        true, // 3 inside ClientSideToolV2Call
	FieldToolName:      true, // 9
	FieldToolRawArgs:   true, // 10
	FieldToolIsLast:    true, // 11
	FieldToolMCPParams: true, // 27
	FieldThinking:      true, // 25 (nested in StreamUnifiedChatResponse)
}

// ToolCall is the Lintasan caller's decoded ClientSideToolV2Call shape.
// Mirrors the JS extractToolCall's return: (function.name, raw args,
// call id, isLast — plus the tool_call_id the caller should use as the
// OpenAI 'function' chat-side id).
type ToolCall struct {
	ID      string `json:"id,omitempty"`       // extracted from TOOL_ID (first line)
	Name    string `json:"name,omitempty"`     // TOOL_NAME
	RawArgs string `json:"raw_args,omitempty"` // TOOL_RAW_ARGS
	IsLast  bool   `json:"is_last,omitempty"`
}

// StreamResponse is the decoded shape of one upstream body. Exactly ONE
// of Text / ToolCall / Thinking is non-nil; the caller decides which one
// to render into the OpenAI wire. Error carries the decode failure
// (mirroring the executor's base64 fallback when the body is malformed).
type StreamResponse struct {
	Text          *string   `json:"text,omitempty"`
	ToolCall      *ToolCall `json:"tool_call,omitempty"`
	Thinking      *string   `json:"thinking,omitempty"`
	DecodeError   string    `json:"decode_error,omitempty"`
	RawBase64     string    `json:"raw_base64,omitempty"` // when decode failed
	UnknownFields []int     `json:"unknown_fields,omitempty"`
}

// ExtractTextFromResponse is the pure-function entry: take one ConnectRPC
// body payload (already unwrapped from its gRPC-web frame) and return the
// decoded shape. Mirrors the JS extractTextFromResponse.
func ExtractTextFromResponse(payload []byte) StreamResponse {
	fields, err := DecodeFields(payload)
	if err != nil {
		// Mirror the JS executor's "decode failed" base64 raw fallback so
		// callers can debug what came back.
		return StreamResponse{
			DecodeError: err.Error(),
			RawBase64:   base64(payload),
		}
	}

	// Flag unknown field numbers (they indicate a protocol update that
	// this port has not seen yet); includes them in the stats but does
	// not fail the response.
	var unknown []int
	for _, f := range fields {
		if !KnownResponseFields[f.Number] {
			unknown = append(unknown, f.Number)
		}
	}

	for _, f := range fields {
		switch f.Number {
		case FieldToolCall:
			// field 1: ClientSideToolV2Call
			if tc := extractToolCall(f.Bytes); tc != nil {
				return StreamResponse{ToolCall: tc, UnknownFields: unknown}
			}
		case FieldResponse:
			// field 2: StreamUnifiedChatResponse → text + thinking
			text, thinking := extractTextAndThinking(f.Bytes)
			if text != nil || thinking != nil {
				return StreamResponse{Text: text, Thinking: thinking, UnknownFields: unknown}
			}
		}
	}

	// A body that decoded cleanly but had neither text nor tool call is
	// an internal Cursor bookkeeping frame (e.g. an in-flight status
	// update); return the empty shape with unknown fields.
	return StreamResponse{UnknownFields: unknown}
}

// extractToolCall decodes the ClientSideToolV2Call block. Returns nil
// when no usable tool call is inside (mirroring the JS executor's
// extractToolCall null-return path when MCP_PARAMS cannot be parsed).
func extractToolCall(block []byte) *ToolCall {
	fields, err := DecodeFields(block)
	if err != nil {
		return nil
	}
	tcl := &ToolCall{}
	// TOOL_ID (field 3): a multi-line string — keep the FIRST line.
	for _, f := range fields {
		if f.Number != FieldToolID {
			continue
		}
		full := string(f.Bytes)
		if i := strings.IndexByte(full, '\n'); i >= 0 {
			full = full[:i]
		}
		tcl.ID = strings.TrimSpace(full)
		break
	}
	// TOOL_NAME (field 9).
	for _, f := range fields {
		if f.Number != FieldToolName {
			continue
		}
		tcl.Name = string(f.Bytes)
		break
	}
	// TOOL_RAW_ARGS (field 10).
	for _, f := range fields {
		if f.Number != FieldToolRawArgs {
			continue
		}
		tcl.RawArgs = string(f.Bytes)
		break
	}
	// TOOL_IS_LAST (field 11): varint 0 = false, anything else = LAST.
	for _, f := range fields {
		if f.Number != FieldToolIsLast {
			continue
		}
		tcl.IsLast = f.Varint != 0
		break
	}
	// Malformed: both ID and Name absent means the block was never a
	// usable tool call; mark nil to match the executor's null path.
	if tcl.ID == "" && tcl.Name == "" {
		return nil
	}
	return tcl
}

// extractTextAndThinking walks the StreamUnifiedChatResponse nested
// block for text (field 1) and thinking (field 25, which itself carries
// a `{field 1: text}` sub-message per the schema).
func extractTextAndThinking(block []byte) (*string, *string) {
	fields, err := DecodeFields(block)
	if err != nil {
		return nil, nil
	}
	var text, thinking *string
	for _, f := range fields {
		switch f.Number {
		case FieldResponseText:
			s := string(f.Bytes)
			text = &s
		case FieldThinking:
			// THINKING is a nested message {field 1 = text}.
			nested, err := DecodeFields(f.Bytes)
			if err != nil {
				continue
			}
			for _, sub := range nested {
				if sub.Number == FieldThinkingText {
					s := string(sub.Bytes)
					thinking = &s
					break
				}
			}
		}
	}
	return text, thinking
}

// base64 encodes buf for the RawBase64 debug field of StreamResponse.
func base64(buf []byte) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out strings.Builder
	for i := 0; i < len(buf); i += 3 {
		n := len(buf) - i
		b0 := uint32(buf[i])
		var b1, b2 uint32
		if n > 1 {
			b1 = uint32(buf[i+1])
		}
		if n > 2 {
			b2 = uint32(buf[i+2])
		}
		// Pack: 18 bits = 3 bytes, then split into 3-4 6-bit groups.
		var group [4]uint32
		group[0] = (b0 >> 2) & 0x3f
		group[1] = (b0&0x3)<<4 | (b1>>4)&0xf
		group[2] = (b1&0x0f)<<2 | (b2>>6)&0x3
		group[3] = b2 & 0x3f
		out.WriteByte(chars[group[0]])
		out.WriteByte(chars[group[1]])
		if n > 1 {
			out.WriteByte(chars[group[2]])
		} else {
			out.WriteByte('=')
		}
		if n > 2 {
			out.WriteByte(chars[group[3]])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
}

// String is for logs — no secrets.
func (s StreamResponse) String() string {
	if s.DecodeError != "" {
		return fmt.Sprintf("decode-error=%s raw=%s", s.DecodeError, s.RawBase64)
	}
	switch {
	case s.ToolCall != nil:
		return fmt.Sprintf("tool-call {id=%q name=%q is_last=%v}", s.ToolCall.ID, s.ToolCall.Name, s.ToolCall.IsLast)
	case s.Text != nil:
		return fmt.Sprintf("text=%.40q", *s.Text)
	case s.Thinking != nil:
		return fmt.Sprintf("thinking=%.40q", *s.Thinking)
	}
	return "empty"
}

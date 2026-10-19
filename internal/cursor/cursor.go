// Package cursor — port of the wire-shape contract 9router
// (open-sse/utils/cursorProtobuf.js, v0.5.104) uses to speak the
// ConnectRPC protobuf wire format to api2.cursor.sh's
// aiserver.v1.ChatService/StreamUnifiedChatWithTools endpoint.
//
// Pure-function library, mirroring internal/{opencodezen,mimofree,windsurf,
// zed,kiro,antigravity}: it encodes/decodes a cursor protobuf wire frame;
// the HTTP closure and response-stream pump live at the caller.
//
// This library only carries the PRIMITIVE encoding pieces (varint, tag,
// field). The FULL MessageShape (UnifiedChatRequestWithTools) is a much
// larger surface (~1123 lines in the JS source) and needs a purpose-built
// Cursor account pool, a client-tool v2 block type, and a ConnectRPC
// SSE-frame demuxer that are specific to a running Cursor deployment.
// Those land as follow-ups; this file provides only the shape-correct
// primitives so a caller that speaks the wire format via hand-rolled
// fields can use them without re-deriving.
package cursor

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// Wire types used by the Cursor ConnectRPC protobuf wire format.
// Kept as consts so a caller cannot pass a wire-type byte by mistake.
const (
	WireTypeVarint  = 0
	WireTypeFixed64 = 1
	WireTypeLen     = 2
	WireTypeFixed32 = 5
)

// Role / UnifiedMode / ThinkingLevel enum values from the schema. Keep
// them as exported constants so a caller cannot make a typo that would
// land as a wire value.
const (
	RoleUser      = 1
	RoleAssistant = 2

	UnifiedModeChat  = 1
	UnifiedModeAgent = 2

	ThinkingLevelUnspecified = 0
	ThinkingLevelMedium      = 1
	ThinkingLevelHigh        = 2

	ClientSideToolV2MCP = 19
)

// Field numbers from the JS source's FIELD block, restricted to the
// subset the encoder helpers touch. Wire numbers are stable in the
// 1.1.3 schema — reordering them silently changes the wire and is
// caught by the caller's own tests.
const (
	FieldRequest = 1

	// StreamUnifiedChatRequest (nested).
	FieldMessages        = 1
	FieldInstruction     = 3
	FieldModel           = 5
	FieldWebTool         = 8
	FieldConversationID  = 23
	FieldMetadata        = 26
	FieldIsAgentic       = 27
	FieldSupportedTools  = 29
	FieldMessageIDs      = 30
	FieldMCPTools        = 34
	FieldLargeContext    = 35
	FieldUnifiedMode     = 46
	FieldDisableTools    = 48
	FieldThinkingLevel   = 49
	FieldUnifiedModeName = 54

	// ConversationMessage (nested).
	FieldMsgContent        = 1
	FieldMsgRole           = 2
	FieldMsgID             = 13
	FieldMsgToolResults    = 18
	FieldMsgIsAgentic      = 29
	FieldMsgServerBubbleID = 32
	FieldMsgUnifiedMode    = 47
	FieldMsgSupportedTools = 51
)

// EncodeVarint returns the LEB128 encoding of v (the standard protobuf
// varint form, up to 10 bytes for a full uint64).
func EncodeVarint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	out = append(out, byte(v))
	return out
}

// EncodeTag returns the wire tag bytes for a (fieldNum, wireType) pair:
// a varint of `fieldNum << 3 | wireType`.
func EncodeTag(fieldNum, wireType int) []byte {
	return EncodeVarint(uint64(fieldNum<<3 | wireType))
}

// EncodeVarintField returns a full field with wire type 0: tag + varint value.
func EncodeVarintField(fieldNum int, value uint64) []byte {
	return append(EncodeTag(fieldNum, WireTypeVarint), EncodeVarint(value)...)
}

// EncodeLenField returns a full field with wire type 2 (length-delimited):
// tag + varint length + payload bytes. Payload may be nil.
func EncodeLenField(fieldNum int, payload []byte) []byte {
	out := EncodeTag(fieldNum, WireTypeLen)
	out = append(out, EncodeVarint(uint64(len(payload)))...)
	return append(out, payload...)
}

// EncodeStringField is EncodeLenField for a Go string payload.
func EncodeStringField(fieldNum int, value string) []byte {
	return EncodeLenField(fieldNum, []byte(value))
}

// EncodeFixed64Field returns a full field with wire type 1 (fixed 64-bit
// LE, IEEE 754 double). Mirrors the JS writeDoubleLE.
func EncodeFixed64Field(fieldNum int, value float64) []byte {
	out := EncodeTag(fieldNum, WireTypeFixed64)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))
	return append(out, buf[:]...)
}

// EncodeFixed32Field returns a full field with wire type 5 (fixed 32-bit
// LE, IEEE 754 float). Used for sub-precision parameters (temperature
// in Cursor's wire is Fixed32, not Fixed64).
func EncodeFixed32Field(fieldNum int, value float32) []byte {
	out := EncodeTag(fieldNum, WireTypeFixed32)
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], math.Float32bits(value))
	return append(out, buf[:]...)
}

// Concat joins byte slices into one buffer, matching the JS concatArrays.
// Preserves the order — a caller that joins fields out of order changes
// the wire (protobuf order matters for repeated fields).
func Concat(parts ...[]byte) []byte {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	out := make([]byte, 0, total)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// ─── Decoding primitives ────────────────────────────────────────────────

// DecodeVarint reads one LEB128 varint starting at offset; returns the
// value and the new offset. Error when the varint exceeds 10 bytes (a
// malformed field would otherwise loop forever) or runs past the buffer.
func DecodeVarint(buf []byte, off int) (uint64, int, error) {
	var result uint64
	var shift uint
	for {
		if off >= len(buf) {
			return 0, off, fmt.Errorf("cursor protobuf: varint truncated at offset %d", off)
		}
		if shift >= 64 {
			return 0, off, fmt.Errorf("cursor protobuf: varint overflow (more than 10 bytes) at offset %d", off)
		}
		b := buf[off]
		off++
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, off, nil
		}
		shift += 7
	}
}

// Field is one decoded protobuf field.
type Field struct {
	Number   int
	WireType int
	Varint   uint64
	Bytes    []byte
	Double   float64
}

// DecodeFields walks a protobuf message and returns every field at the
// top level, in encounter order. It does NOT recurse into nested length-
// delimited fields; the caller knows which of those are messages vs
// strings/bytes and recurses itself.
func DecodeFields(buf []byte) ([]Field, error) {
	var out []Field
	off := 0
	for off < len(buf) {
		tag, newOff, err := DecodeVarint(buf, off)
		if err != nil {
			return nil, err
		}
		off = newOff
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)
		f := Field{Number: fieldNum, WireType: wireType}
		switch wireType {
		case WireTypeVarint:
			v, newOff, err := DecodeVarint(buf, off)
			if err != nil {
				return nil, err
			}
			f.Varint = v
			off = newOff
		case WireTypeLen:
			ln, newOff, err := DecodeVarint(buf, off)
			if err != nil {
				return nil, err
			}
			if ln > uint64(len(buf)-newOff) {
				return nil, fmt.Errorf("cursor protobuf: length %d exceeds buffer at offset %d", ln, newOff)
			}
			f.Bytes = buf[newOff : newOff+int(ln)]
			off = newOff + int(ln)
		case WireTypeFixed64:
			if off+8 > len(buf) {
				return nil, fmt.Errorf("cursor protobuf: fixed64 truncated at offset %d", off)
			}
			f.Double = math.Float64frombits(binary.LittleEndian.Uint64(buf[off : off+8]))
			off += 8
		case WireTypeFixed32:
			if off+4 > len(buf) {
				return nil, fmt.Errorf("cursor protobuf: fixed32 truncated at offset %d", off)
			}
			// Fixed32 is read as a 32-bit float in the JS decoder; carry
			// it through Double (the caller can re-cast).
			f.Double = float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[off : off+4])))
			off += 4
		default:
			return nil, fmt.Errorf("cursor protobuf: unknown wire type %d at offset %d", wireType, off)
		}
		out = append(out, f)
	}
	return out, nil
}

// FormatToolName mirrors 9router's formatToolName — the tool name sent
// upstream is "<name>_<callid>" so Cursor's server-side tool tracker can
// pair a tool call with its result. The caller's tool id never reaches
// the wire un-wrapped.
func FormatToolName(name, callID string) string {
	if callID == "" {
		return name
	}
	return name + "_" + callID
}

// ParseToolName splits a formatted tool name back into (name, callID).
// Returns ("", "") for a malformed input — mirrors the JS's early-return.
func ParseToolName(formatted string) (string, string) {
	idx := strings.LastIndexByte(formatted, '_')
	if idx <= 0 || idx == len(formatted)-1 {
		return "", ""
	}
	return formatted[:idx], formatted[idx+1:]
}

// ─── connectrpc frame framing ───────────────────────────────────────────

// GRPCWebFrame is the 1-flag + BE-32 length + payload frame the ConnectRPC
// protocol uses on the wire (same shape as Windsurf's, different flag
// semantics: 0x00 = uncompressed, 0x01 = compressed).
func GRPCWebFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0x00 // uncompressed
	frame[1] = byte(len(payload) >> 24)
	frame[2] = byte(len(payload) >> 16)
	frame[3] = byte(len(payload) >> 8)
	frame[4] = byte(len(payload))
	copy(frame[5:], payload)
	return frame
}

// SplitGRPCWebFrames returns the payloads of every frame in the response
// body. Errors on a truncated frame with the offset so the caller can
// log where the response broke mid-stream.
func SplitGRPCWebFrames(body []byte) ([][]byte, error) {
	var out [][]byte
	off := 0
	for off < len(body) {
		if off+5 > len(body) {
			return out, fmt.Errorf("cursor connect-rpc: truncated frame header at offset %d", off)
		}
		length := int(body[off+1])<<24 | int(body[off+2])<<16 | int(body[off+3])<<8 | int(body[off+4])
		if length < 0 || off+5+length > len(body) {
			return out, fmt.Errorf("cursor connect-rpc: frame at offset %d claims %d bytes, only %d available", off, length, len(body)-off-5)
		}
		out = append(out, body[off+5:off+5+length])
		off += 5 + length
	}
	return out, nil
}

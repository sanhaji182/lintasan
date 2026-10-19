// Package windsurf ports the gRPC-web protobuf wire shape 9router's
// proxy/executor/windsurf.go (luqman-v1/9router-go) uses to speak to
// exa.language_server_pb.LanguageServerService.GetChatMessage — a
// hand-rolled protobuf, no third-party dependency.
//
// This is a B3/TYPO port (the MIT license both forks carry permits this),
// so the 9router-go credit is on file for the entire protobuf field-number
// structure (metadata→ikan_exp, message map, completion chunks). The
// HTTP-forward closure ForwardWindsurf itself is NOT ported — Lintasan
// keeps executors as pure-function shape encoders, the translating caller
// owns the connection and lifecycles.
package windsurf

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Endpoints / identity constants match 9router-go (same upstream names).
const (
	ChatURL  = "https://server.codeium.com/exa.language_server_pb.LanguageServerService/GetChatMessage"
	IDEName  = "windsurf"
	IDEVers  = "3.14.0"
	ExtVers  = "3.14.0"
	Locale   = "en-US"
	ReadSize = 32 << 10
)

// ModelAliases maps dashboard catalog model names to Windsurf's wire spelling
// (dashes, MODEL_X constants, etc.). Straight port of the 9router-go alias
// table — Sans' parity plan is explicit that maintaining this is
// contract-fence, not a freehand rewrite.
var ModelAliases = map[string]string{
	"swe-1.6-fast": "swe-1-6-fast", "swe-1.6": "swe-1-6",
	"swe-1.5-fast": "swe-1-5-fast", "swe-1.5": "swe-1-5",
	"claude-opus-4.7-max": "claude-opus-4-7-max", "claude-opus-4.7-xhigh": "claude-opus-4-7-xhigh",
	"claude-opus-4.7-high": "claude-opus-4-7-high", "claude-opus-4.7-medium": "claude-opus-4-7-medium",
	"claude-opus-4.7-low": "claude-opus-4-7-low", "claude-opus-4.7-review": "opus-4-7-review",
	"claude-sonnet-4.6-thinking-1m": "claude-sonnet-4-6-thinking-1m", "claude-sonnet-4.6-1m": "claude-sonnet-4-6-1m",
	"claude-sonnet-4.6-thinking": "claude-sonnet-4-6-thinking", "claude-sonnet-4.6": "claude-sonnet-4-6",
	"claude-opus-4.6-thinking": "claude-opus-4-6-thinking", "claude-opus-4.6": "claude-opus-4-6",
	"claude-opus-4.5-thinking": "MODEL_CLAUDE_4_5_OPUS_THINKING", "claude-opus-4.5": "MODEL_CLAUDE_4_5_OPUS",
	"claude-sonnet-4.5-thinking": "MODEL_PRIVATE_3", "claude-sonnet-4.5": "MODEL_PRIVATE_2",
	"claude-haiku-4.5":   "MODEL_PRIVATE_11",
	"gpt-5.5-xhigh-fast": "gpt-5-5-xhigh-priority", "gpt-5.5-high-fast": "gpt-5-5-high-priority",
	"gpt-5.5-medium-fast": "gpt-5-5-medium-priority", "gpt-5.5-low-fast": "gpt-5-5-low-priority",
	"gpt-5.5-none-fast": "gpt-5-5-none-priority", "gpt-5.5-xhigh": "gpt-5-5-xhigh",
	"gpt-5.5-high": "gpt-5-5-high", "gpt-5.5-medium": "gpt-5-5-medium",
	"gpt-5.5-low": "gpt-5-5-low", "gpt-5.5-none": "gpt-5-5-none",
	"gpt-5.5-review": "gpt-5-5-review", "gpt-5.5": "gpt-5-5-medium",
	"gpt-5.4-xhigh-fast": "gpt-5-4-xhigh-priority", "gpt-5.4-high-fast": "gpt-5-4-high-priority",
	"gpt-5.4-medium-fast": "gpt-5-4-medium-priority", "gpt-5.4-low-fast": "gpt-5-4-low-priority",
	"gpt-5.4-none-fast": "gpt-5-4-none-priority", "gpt-5.4-xhigh": "gpt-5-4-xhigh",
	"gpt-5.4-high": "gpt-5-4-high", "gpt-5.4-medium": "gpt-5-4-medium",
	"gpt-5.4-low": "gpt-5-4-low", "gpt-5.4-none": "gpt-5-4-none",
	"gpt-5.4-mini-xhigh": "gpt-5-4-mini-xhigh", "gpt-5.4-mini-high": "gpt-5-4-mini-high",
	"gpt-5.4-mini-medium": "gpt-5-4-mini-medium", "gpt-5.4-mini-low": "gpt-5-4-mini-low",
	"gpt-5.4":                  "gpt-5-4-medium",
	"gpt-5.3-codex-xhigh-fast": "gpt-5-3-codex-xhigh-priority", "gpt-5.3-codex-high-fast": "gpt-5-3-codex-high-priority",
	"gpt-5.3-codex-medium-fast": "gpt-5-3-codex-medium-priority", "gpt-5.3-codex-low-fast": "gpt-5-3-codex-low-priority",
	"gpt-5.3-codex-xhigh": "gpt-5-3-codex-xhigh", "gpt-5.3-codex-high": "gpt-5-3-codex-high",
	"gpt-5.3-codex-medium": "gpt-5-3-codex-medium", "gpt-5.3-codex-low": "gpt-5-3-codex-low",
	"gpt-5.3-codex": "gpt-5-3-codex-medium",
	"gpt-5.2-xhigh": "MODEL_GPT_5_2_XHIGH", "gpt-5.2-high": "MODEL_GPT_5_2_HIGH",
	"gpt-5.2-medium": "MODEL_GPT_5_2_MEDIUM", "gpt-5.2-low": "MODEL_GPT_5_2_LOW",
	"gpt-5.2":           "gpt-5-2-medium",
	"gpt-5.1":           "gpt-5-1-medium",
	"gpt-5":             "gpt-5-medium",
	"gpt-4.1":           "MODEL_GPT_4_1",
	"claude-3-7-sonnet": "claude-3-7-sonnet",
}

// ResolveModelID resolves a catalog model name through ModelAliases. An
// unknown id passes through unchanged so the executor still runs rather
// than refusing (mirrors 9router-go's fallback).
func ResolveModelID(model string) string {
	if v, ok := ModelAliases[model]; ok {
		return v
	}
	return model
}

// ─── Protobuf encoding primitives (hand-rolled, no third-party dep) ────────

// EncVarint is the standard LEB128 sign-extended varint encoding.
func EncVarint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	out = append(out, byte(v))
	return out
}

func Tag(fieldNum, wireType int) []byte { return EncVarint(uint64(fieldNum<<3 | wireType)) }

func Field(tag, payload []byte) []byte {
	out := append([]byte{}, tag...)
	out = append(out, EncVarint(uint64(len(payload)))...)
	return append(out, payload...)
}

func Str(fieldNum int, s string) []byte { return Field(Tag(fieldNum, 2), []byte(s)) }
func Msg(fieldNum int, msg []byte) []byte {
	return Field(Tag(fieldNum, 2), msg)
}

func VarintField(fieldNum int, v uint64) []byte {
	return append(Tag(fieldNum, 0), EncVarint(v)...)
}

// ─── Request-shape encoders ──────────────────────────────────────────────

// Metadata is the gRPC-web metadata field: apiKey/ide/ide/ext/session/locale.
func Metadata(apiKey, sessionID string) []byte {
	return bytes.Join([][]byte{
		Str(1, apiKey), Str(2, IDEName), Str(3, IDEVers),
		Str(4, ExtVers), Str(5, sessionID), Str(6, Locale),
	}, nil)
}

// ChatMessage is one message: role + content + optional tool_call_id.
func ChatMessage(role, content, toolCallID string) []byte {
	parts := [][]byte{Str(1, role), Str(2, content)}
	if toolCallID != "" {
		parts = append(parts, Str(3, toolCallID))
	}
	return bytes.Join(parts, nil)
}

// GetChatMessageRequest assembles the full GetChatMessageRequest protobuf:
// field 1 metadata, 2 cascade_id, 3 model_or_alias, 4 .messages.
//
// NOTE: model is the WINDSURF WIRE NAME — resolution via ResolveModelID
// happens at the CALLER (match 9router-go's `resolveWsModelID(oreq.Model)`
// in ForwardWindsurf), so this encoder does not silently alias an unknown
// id. Lintasan's caller is responsible for invoking ResolveModelID first.
func GetChatMessageRequest(apiKey, model string, messages [][]byte) []byte {
	parts := [][]byte{
		Msg(1, Metadata(apiKey, RandHex(16))),
		Str(2, RandHex(16)),
		Msg(3, Str(1, model)),
	}
	for _, m := range messages {
		parts = append(parts, Msg(4, m))
	}
	return bytes.Join(parts, nil)
}

// GRPCWebFrame wraps a protobuf payload in a gRPC-web frame:
// 1 flag byte (0x00 no compression) + big-endian 32-bit length + payload.
func GRPCWebFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0x00
	frame[1] = byte(len(payload) >> 24)
	frame[2] = byte(len(payload) >> 16)
	frame[3] = byte(len(payload) >> 8)
	frame[4] = byte(len(payload))
	copy(frame[5:], payload)
	return frame
}

// ─── Protobuf decoding ───────────────────────────────────────────────────

// Chunk variants decoded from one CompletionChunk message.
type Chunk struct {
	Kind             string // "content" | "done" | "error" | "unknown"
	Text             string
	PromptTokens     int
	CompletionTokens int
	Message          string
}

// ReadVarint decodes one LEB128 varint starting at offset off; returns the
// result and the new offset.
func ReadVarint(buf []byte, off int) (uint64, int) {
	var result uint64
	var shift uint
	for off < len(buf) {
		b := buf[off]
		off++
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}
	return result, off
}

// DecodeStringField walks a protobuf message looking for the first
// fieldNum == target with wire type 2 (length-delimited), returning its
// payload as a string. Empty/absent → "".
func DecodeStringField(buf []byte, target int) string {
	off := 0
	for off < len(buf) {
		tag, o := ReadVarint(buf, off)
		off = o
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)
		switch wireType {
		case 2:
			ln, o := ReadVarint(buf, off)
			off = o
			if ln > uint64(len(buf)-off) {
				return ""
			}
			if fieldNum == target {
				return string(buf[off : off+int(ln)])
			}
			off += int(ln)
		case 0:
			_, off = ReadVarint(buf, off)
		case 1:
			off += 8
		case 5:
			off += 4
		default:
			return ""
		}
	}
	return ""
}

// DecodeDoneChunk parses DoneChunk{field 1 = UsageStats{1=prompt, 2=completion}}.
func DecodeDoneChunk(buf []byte) (int, int) {
	off := 0
	var usage []byte
	for off < len(buf) {
		tag, o := ReadVarint(buf, off)
		off = o
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)
		if wireType == 2 {
			ln, o := ReadVarint(buf, off)
			off = o
			if ln > uint64(len(buf)-off) {
				break
			}
			if fieldNum == 1 {
				usage = buf[off : off+int(ln)]
			}
			off += int(ln)
		} else if wireType == 0 {
			_, off = ReadVarint(buf, off)
		} else {
			break
		}
	}
	if usage == nil {
		return 0, 0
	}
	var prompt, completion int
	off = 0
	for off < len(usage) {
		tag, o := ReadVarint(usage, off)
		off = o
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)
		if wireType == 0 {
			v, o := ReadVarint(usage, off)
			off = o
			if fieldNum == 1 {
				prompt = int(v)
			} else if fieldNum == 2 {
				completion = int(v)
			}
		} else if wireType == 2 {
			ln, o := ReadVarint(usage, off)
			off = o
			off += int(ln)
		} else {
			break
		}
	}
	return prompt, completion
}

// DecodeCompletionChunk decodes ONE CompletionChunk message: oneof
// (1 = ContentChunk{1=text}, 3 = DoneChunk{1=UsageStats}, 4 = ErrorChunk{1=message}).
// Skipped: 2 = ToolCallChunk (Lintasan proxy does not pass tool calls to this
// provider; add when a use-case needs it).
func DecodeCompletionChunk(buf []byte) Chunk {
	off := 0
	for off < len(buf) {
		tag, o := ReadVarint(buf, off)
		off = o
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)
		if wireType == 2 {
			ln, o := ReadVarint(buf, off)
			off = o
			if ln > uint64(len(buf)-off) {
				break
			}
			payload := buf[off : off+int(ln)]
			off += int(ln)
			switch fieldNum {
			case 1:
				if text := DecodeStringField(payload, 1); text != "" {
					return Chunk{Kind: "content", Text: text}
				}
			case 3:
				p, c := DecodeDoneChunk(payload)
				return Chunk{Kind: "done", PromptTokens: p, CompletionTokens: c}
			case 4:
				msg := DecodeStringField(payload, 1)
				if msg == "" {
					msg = "unknown windsurf error"
				}
				return Chunk{Kind: "error", Message: msg}
			}
		} else if wireType == 0 {
			_, off = ReadVarint(buf, off)
		} else if wireType == 1 {
			off += 8
		} else if wireType == 5 {
			off += 4
		} else {
			break
		}
	}
	return Chunk{Kind: "unknown"}
}

// RandHex returns n random bytes hex-encoded (session/cascade ids). Falls
// back to a time-based id when crypto/rand fails — unrecoverable in
// practice, but mirrors 9router-go's kindness so the executor still runs.
func RandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// JoinStringField returns a helper for tests and callers that want the
// contents of a UTF-8 field rendered for logging (truncates at 200 bytes).
func Preview(s string) string {
	if len(s) <= 200 {
		return s
	}
	return s[:200] + "…"
}

// SplitGRPCWebFrames splits a response body into the gRPC-web frames it
// contains (each: 1 flag + 4-byte BE length + payload). Returns raw
// payloads — flags investigated by the caller. Error carries the first
// malformed frame's offset (0-based) so debugging has a pointer.
func SplitGRPCWebFrames(body []byte) ([][]byte, error) {
	var out [][]byte
	off := 0
	for off < len(body) {
		if off+5 > len(body) {
			return out, fmt.Errorf("windsurf grpc-web: truncated frame header at offset %d", off)
		}
		// 4-byte BE length between `off+1` and `off+5`.
		length := int(body[off+1])<<24 | int(body[off+2])<<16 | int(body[off+3])<<8 | int(body[off+4])
		if length < 0 || off+5+length > len(body) {
			return out, fmt.Errorf("windsurf grpc-web: frame at offset %d claims %d bytes, only %d available", off, length, len(body)-off-5)
		}
		out = append(out, body[off+5:off+5+length])
		off += 5 + length
	}
	return out, nil
}

// CollectContent concatenates every "content" chunk TEXT returned by a body.
// Convenience for the non-streaming case where the caller already decoded
// frames via SplitGRPCWebFrames.
func CollectContent(chunks []Chunk) string {
	var s strings.Builder
	for _, c := range chunks {
		if c.Kind == "content" {
			s.WriteString(c.Text)
		}
	}
	return s.String()
}

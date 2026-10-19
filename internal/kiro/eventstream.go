// Package kiro ports the AWS EventStream binary framing 9router
// (open-sse/utils/awsEventStream.js, v0.5.104) uses for its Kiro /
// Amazon CodeWhisperer / Bedrock invoke-with-response-stream flows.
//
// Frame layout (per the AWS EventStream spec, mirrored verbatim):
//
//	[totalLength u32 BE][headersLength u32 BE][preludeCrc u32 BE]
//	[headers...][payload][messageCrc u32 BE]
//
// Before Lintasan can translate a Kiro wire response it has to be EXACTLY
// framed. This package is a pure-function library (encode/decode/validate)
// with no HTTP side effects and no allocator state, mirroring the executor's
// hardening against malformed frames (bounds + CRC + duplicate-header):
//
//   - parseEventFrame validates the prelude CRC (bytes 0..8) and the
//     message CRC (bytes 0..totalLength-4) — the JS version throws on a
//     mismatch; here we return an error so the caller decides.
//   - header types handled: 0/1 bool, 2 int8, 3 int16, 4 int32, 5/8
//     8-byte (int64/float64, value skipped), 6 bytes, 7 string,
//     9 16-byte (timestamp/value skipped). Unknown type is an error.
//   - duplicate header names are an error (the JS version also rejects
//     them and the JS-side behaviour is what upstream spec requires).
//   - empty or non-JSON payload → nil headers-with-no-payload; the caller
//     decides whether that is an error or a control-frame (e.g.	a
//     non-JSON payload is what a Bedrock ErrorEvent body uses).
//
// The encryption/AWS-SigV4 signing path lives outside this package (see
// Luqman's 9router-go internal/proxy/executor/bedrock.go for the SigV4
// hand-roll if we ever need it).
package kiro

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

// AWS_EVENTSTREAM limits from 9router config/awsConstants.js. Bounds cap
// yells when the prelude lies about the byte count.
const (
	// MaxTotalFrameLength is ~16 MB, keeping DoS-from-crafted-framing sane.
	MaxTotalFrameLength = 16 << 20
	// MaxHeadersLength is 256 KB.
	MaxHeadersLength = 256 << 10
	// MaxPayloadLength is 16 MB (the same cap the executor applies).
	MaxPayloadLength = 16 << 20
)

// Frame is a decoded single EventStream frame: its headers (in
// encounter order into the map) and the payload bytes.
type Frame struct {
	Headers       map[string]any // unsupported-typed entries are read raw
	Payload       []byte         // raw, undecoded (use PayloadJSON when expected)
	TotalLength   int
	HeadersLength int
}

// CRC32Table mirrors the JS Uint32Array above — Morton-Williams.
var CRC32Table [256]uint32

func init() {
	for i := 0; i < 256; i++ {
		v := uint32(i)
		for b := 0; b < 8; b++ {
			if v&1 != 0 {
				v = v>>1 ^ 0xedb88320
			} else {
				v >>= 1
			}
		}
		CRC32Table[i] = v
	}
}

// CRC32 returns the CRC32 (Morton-Williams, the AWS-EventStream variant)
// of buf. Mirrors 9router's crc32 bitwise.
func CRC32(buf []byte) uint32 {
	crc := ^uint32(0)
	for _, b := range buf {
		crc = CRC32Table[(crc^uint32(b))&0xff] ^ (crc >> 8)
	}
	return ^crc
}

// ParseEventFrame decodes ONE AWS EventStream binary frame from data. It
// validates both CRCs and the bounds; error when it does not match. The
// returned Frame's Payload is the RAW bytes (the caller can JSON-decode
// when it expects JSON — the JS executor does that itself, so the split
// prevents this package from having to know which provider's payload
// shape is arriving).
func ParseEventFrame(data []byte) (*Frame, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("AWS EventStream frame too short: %d bytes (min 16)", len(data))
	}
	totalLength := int(binary.BigEndian.Uint32(data[0:4]))
	headersLength := int(binary.BigEndian.Uint32(data[4:8]))

	if totalLength < 16 || totalLength > MaxTotalFrameLength {
		return nil, fmt.Errorf("AWS EventStream total length %d out of bounds", totalLength)
	}
	if totalLength > len(data) {
		return nil, fmt.Errorf("AWS EventStream total length %d exceeds buffer %d", totalLength, len(data))
	}
	if headersLength < 0 || headersLength > MaxHeadersLength || headersLength+12 > totalLength {
		return nil, fmt.Errorf("AWS EventStream headers length %d out of bounds", headersLength)
	}

	// Prelude CRC covers bytes 0..8 (the two u32 length fields).
	preludeCRC := binary.BigEndian.Uint32(data[8:12])
	if preludeCRC != CRC32(data[0:8]) {
		return nil, fmt.Errorf("AWS EventStream prelude CRC mismatch: got %d, want %d", preludeCRC, CRC32(data[0:8]))
	}
	// Message CRC covers everything except the last 4 bytes.
	messageCRC := binary.BigEndian.Uint32(data[totalLength-4 : totalLength])
	if messageCRC != CRC32(data[0:totalLength-4]) {
		return nil, fmt.Errorf("AWS EventStream message CRC mismatch: got %d, want %d", messageCRC, CRC32(data[0:totalLength-4]))
	}

	// Headers section: sequence of (name_len u8, name, type u8, value) —
	// the value shape is determined by type.
	headers := make(map[string]any)
	seen := make(map[string]bool)
	offset := 12
	headerEnd := offset + headersLength
	requireBytes := func(count int) error {
		if offset+count > headerEnd {
			return fmt.Errorf("AWS EventStream header exceeds its declared bounds")
		}
		return nil
	}
	for offset < headerEnd {
		if err := requireBytes(1); err != nil {
			return nil, err
		}
		nameLength := int(data[offset])
		offset++
		if err := requireBytes(nameLength + 1); err != nil {
			return nil, err
		}
		if nameLength == 0 {
			return nil, fmt.Errorf("AWS EventStream header name length 0")
		}
		name := string(data[offset : offset+nameLength])
		offset += nameLength
		if seen[name] {
			return nil, fmt.Errorf("AWS EventStream contains duplicate header: %s", name)
		}
		seen[name] = true
		if err := requireBytes(1); err != nil {
			return nil, err
		}
		// Consume the type byte once, BEFORE the switch — the switch reads
		// the wire bytes that follow it, and forgetting this advance makes
		// every numeric/string case look at the type byte as if it were
		// the length prefix (a shape-shifting bug the JS executor avoids
		// because JS has DataView.getWithCursor; a Go switch does not).
		hdrType := data[offset]
		offset++
		switch hdrType {
		case 0: // bool false
			headers[name] = false
		case 1: // bool true
			headers[name] = true
		case 2: // int8
			if err := requireBytes(1); err != nil {
				return nil, err
			}
			b := int8(data[offset])
			headers[name] = b
			offset++
		case 3: // int16 BE
			if err := requireBytes(2); err != nil {
				return nil, err
			}
			v := int16(binary.BigEndian.Uint16(data[offset : offset+2]))
			headers[name] = v
			offset += 2
		case 4: // int32 BE
			if err := requireBytes(4); err != nil {
				return nil, err
			}
			v := int32(binary.BigEndian.Uint32(data[offset : offset+4]))
			headers[name] = v
			offset += 4
		case 5, 8: // int64 / any 8-byte value — skipped for now
			if err := requireBytes(8); err != nil {
				return nil, err
			}
			offset += 8
			headers[name] = fmt.Sprintf("<int64@type-%d>", hdrType)
		case 6: // bytes
			if err := requireBytes(2); err != nil {
				return nil, err
			}
			valueLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if err := requireBytes(valueLength); err != nil {
				return nil, err
			}
			headers[name] = append([]byte(nil), data[offset:offset+valueLength]...)
			offset += valueLength
		case 7: // string
			if err := requireBytes(2); err != nil {
				return nil, err
			}
			valueLength := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if err := requireBytes(valueLength); err != nil {
				return nil, err
			}
			headers[name] = string(data[offset : offset+valueLength])
			offset += valueLength
		case 9: // 16-byte timestamp; skipped
			if err := requireBytes(16); err != nil {
				return nil, err
			}
			offset += 16
			headers[name] = fmt.Sprintf("<timestamp@type-9>")
		default:
			return nil, fmt.Errorf("AWS EventStream header %s has unknown type %d", name, hdrType)
		}
	}

	// Payload is whatever lives between headers-end and the last 4 bytes
	// reserved for CRC.
	payloadStart := offset
	payloadEnd := totalLength - 4
	if payloadStart > payloadEnd {
		// jsontext says the headers overran — we should not reach here
		// thanks to the requireBytes errors, but belt-and-braces.
		return nil, fmt.Errorf("AWS EventStream headers overrun into payload area")
	}
	payload := append([]byte(nil), data[payloadStart:payloadEnd]...)

	return &Frame{
		Headers:       headers,
		Payload:       payload,
		TotalLength:   totalLength,
		HeadersLength: headersLength,
	}, nil
}

// PayloadJSON decodes the Frame's payload if it is expected to be JSON
// payloads (the 9router executor JSON-decodes too); error on a non-JSON
// body. Empty frame payload → a nil map without an error — the JS side
// treats that as a control frame, so we mirror it.
func (f *Frame) PayloadJSON() (map[string]any, error) {
	if len(f.Payload) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(string(f.Payload)) == "" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(f.Payload, &out); err != nil {
		return nil, fmt.Errorf("AWS EventStream payload is not valid JSON (%v)", err)
	}
	return out, nil
}

// EncodeEventFrame is the byte-exact counterpart to ParseEventFrame,
// mirroring 9router's encodeEventFrame (used by test fixtures only; the
// request path does not emit EventStream, so this stays unexercised in
// Lintasan until a use-case needs it — but it lives here so encode and
// decode stay in step, sometimes the exact bug the JS file documents).
// headers must be string or []byte (written as string/bytes types 7/6).
func EncodeEventFrame(headers map[string][]byte, payload []byte) []byte {
	var hBuf []byte
	for _, name := range sortedHeaderNames(headers) {
		nameBytes := []byte(name)
		hBuf = append(hBuf, byte(len(nameBytes)))
		hBuf = append(hBuf, nameBytes...)
		hBuf = append(hBuf, 7) // type 7 = string
		value := headers[name]
		l := make([]byte, 2)
		binary.BigEndian.PutUint16(l, uint16(len(value)))
		hBuf = append(hBuf, l...)
		hBuf = append(hBuf, value...)
	}
	total := 12 + len(hBuf) + len(payload) + 4
	out := make([]byte, 0, total)
	l := make([]byte, 4)
	// totalLength
	binary.BigEndian.PutUint32(l, uint32(total))
	out = append(out, l...)
	// headersLength
	l = make([]byte, 4)
	binary.BigEndian.PutUint32(l, uint32(len(hBuf)))
	out = append(out, l...)
	// preludeCrc over the 8-byte prelude we just wrote.
	preludeCRC := CRC32(out[0:8])
	crcB := make([]byte, 4)
	binary.BigEndian.PutUint32(crcB, preludeCRC)
	out = append(out, crcB...)
	// headers
	out = append(out, hBuf...)
	// payload
	out = append(out, payload...)
	// message CRC over everything we wrote.
	msgCRC := CRC32(out)
	msgCrcB := make([]byte, 4)
	binary.BigEndian.PutUint32(msgCrcB, msgCRC)
	out = append(out, msgCrcB...)
	return out
}

func sortedHeaderNames(headers map[string][]byte) []string {
	// OrderedDict is a stable choice so tests do not depend on Go map order.
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	// insertion sort — cheap and deterministic (only used by fixtures).
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

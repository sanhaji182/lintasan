// Package opencodezen portés 9router open-sse/executors/opencode-zen.js
// (v0.5.104) into Lintasan Go — experimental-track, membrane gated, never
// auto-routable.
//
// The 9router executor does 4 things to make OpenCode accept a proxy call:
//
//  1. UA / client fingerprint — User-Agent must look like
//     opencode/1.18.31 (or any downstream "opencode/1.x"-style UA with
//     minor >= 17); x-opencode-client + x-opencode-project default to
//     "desktop"/"global".
//  2. Session/request id — x-opencode-session must be a
//     `ses_<12hex><14 base62>` id whose 12-hex prefix decodes to a
//     descending-time value with counter 1 (upstream 403 FreeTierError
//     otherwise). Translate a client-supplied session deterministically
//     (sha256("opencode\0SERVICENAME\0<session>") time-low + base62 tail);
//     mint a fresh one per request otherwise.
//  3. Tool fingerprint — free-tier model calls must carry the built-in
//     bash/glob/grep/read function tools and stream:true, otherwise
//     upstream 403s.
//  4. Free-tier models (`*-free` / fixes handled in HasFreeTierModel) get
//     Authorization "Bearer public" AND x-opencode-request mint
//     separately, regardless of the connection's actual key.
//
// Only the pieces the Lintasan proxy hot path needs live here: Header
// builders, session-id helpers, and a body-shape normalizer. The executor's
// /responses sub-lane handling is a translator concern, not a header
// builder one, and intentionally NOT here (see the A7 plan note about the
// multi-transport gap).
//
// SAFETY: this file isINTERNALLY a pure-function library. It performs no
// HTTP calls, holds no state, and never mutates shared global state except
// through the PreparedHeaders it returns. A connection that routes through
// this provider requires the caller to explicitly gate it
// (`opencode_zen_enabled` DB setting, default false, same pattern as
// `provider_sdk_enabled`) — otherwise the proxy sends the default
// executor's headers and OpenCode answers 403 FreeTierError with no
// explanation, which is the exact bug this code prevents.
package opencodezen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// Version constants and endpoint targets from the 9router executor.
const (
	ResponsesBaseURL = "https://opencode.ai/zen/v1/responses"

	// OpenCodeUA is the pinned client version this port is validated against
	// (NOT a constant to later bump silently — upstream accepts minor >= 17,
	// so a stale constant keeps working as long as the minor survives).
	OpenCodeUA = "opencode/1.18.31"

	// SessionHeader / RequestHeader downstream headers OpenCode checks.
	SessionHeader = "x-opencode-session"
	RequestHeader = "x-opencode-request"
	ClientHeader  = "x-opencode-client"
	ProjectHeader = "x-opencode-project"

	// SessionRew / RequestRew are the shapes OpenCode expects when it sees
	// x-opencode-session / x-opencode-request. Keep them here (not just
	// inline) so the doc-comment regexp is testable.
	SessionRew = `^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`
)

// FingerprintTools is the 4-file-search quartet OpenCode demands on
// free-tier requests (mirrors 9router PR #4132). Upstream 403s without
// them. It is a var rather than a const because slices have no const
// representation in Go; nothing in this package mutates it, and callers
// must not either.
var FingerprintTools = []string{"bash", "glob", "grep", "read"}

// base62 is the alphabet OpenCode uses for the session/request random tail
// (0-9A-Za-z, lexicographic by [0-9][A-Z][a-z]). Keep the ordering stable —
// the tail is what upstream's decoder reads, not what's pretty to a human.
const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var sessionRew = regexp.MustCompile(SessionRew)

// HasValidOpencodeVersion mirrors the 9router hasValidOpencodeVersion: only
// "opencode/<major>.<minor>(.<patch>)" with major > 1 or (major == 1 and
// minor >= 17) counts. Anything else — no UA, curl/..., some other tool's
// User-Agent — must fall back to the pinned OpenCodeUA because otherwise
// upstream 403s with no explanation.
func HasValidOpencodeVersion(ua string) bool {
	s := string(ua)
	if s == "" {
		return false
	}
	// Keep a regex-free match: the executor's regex is anchored to the
	// "opencode/" prefix but does not require end-of-string. A downstream
	// that pads ("opencode/1.20.0") or suffixes ("opencode/1.20.0-beta")
	// still counts because upstream reads digits after the first dot.
	idx := strings.Index(strings.ToLower(s), "opencode/")
	if idx < 0 {
		return false
	}
	rest := s[idx+len("opencode/"):]
	// Expect [<digit>+.][digits+.]digits — read the first two dotted digit
	// groups in order and compare major/minor numerically.
	major, restAfterMajor, ok := readDigitGroup(rest)
	if !ok || len(restAfterMajor) == 0 || restAfterMajor[0] != '.' {
		return false
	}
	minor, _, ok := readDigitGroup(restAfterMajor[1:])
	if !ok {
		return false
	}
	return major > 1 || (major == 1 && minor >= 17)
}

func readDigitGroup(in string) (int, string, bool) {
	if len(in) == 0 || in[0] < '0' || in[0] > '9' {
		return 0, in, false
	}
	i := 0
	n := 0
	for i < len(in) && in[i] >= '0' && in[i] <= '9' {
		n = n*10 + int(in[i]-'0')
		if n > 1<<24 { // very defensive; matches realistic opencode versions
			break
		}
		i++
	}
	return n, in[i:], true
}

// base62Random returns a string of exactly n base62 chars.
func base62Random(n int) string {
	out := make([]byte, 0, n)
	buf := make([]byte, n*2)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			// crypto/rand never fails on a real OS; belt-and-braces.
			panic(fmt.Sprintf("opencodezen: rand read: %v", err))
		}
		for _, b := range buf {
			if len(out) >= n {
				break
			}
			out = append(out, base62[int(b)%len(base62)])
		}
	}
	return string(out)
}

// maskLength48 mirrors 9router's ~((time * 0x1000 + counter) & (2^48 - 1)).
// hex format const: 12 chars, zero-padded.
const uint48Mask = (uint64(1) << 48) - 1

// DescendingTimeHex is the session-id time field: the bitwise NOT of
// unixMilli * 0x1000 + counter, truncated to 48 bits, rendered as 12
// lowercase hex. Upstream decodes the incoming prefix and 403
// FreeTierError when the decoded counter reaches a range the real CLI
// never sends (live probes: 0–16 pass, >=100 fail, but 1 is the value the
// CLI actually pins, so we do too).
func DescendingTimeHex(unixMilli int64, counter uint64) string {
	v := (^(uint64(unixMilli)*0x1000 + counter)) & uint48Mask
	// 12 hex chars of a 48-bit value; lower-case to match the 9router
	// executor's lowercase .toString(16) output.
	return fmt.Sprintf("%012x", v)
}

// NewSessionID mint a fresh session id, same shape the 9router
// generateSessionId emits. Counter pinned to 1 (what the CLI sends).
func NewSessionID(unixMilli int64) string {
	return "ses_" + DescendingTimeHex(unixMilli, 1) + base62Random(14)
}

// NewRequestID mint a fresh request id, same shape 9router
// generateRequestId emits. It is NOT a session id — the request id
// timestamps the *call*, so the time segment uses the ascending
// high→low bytes of (time * 0x1000 + 1) rather than a descending-NOT
// prefix, exactly as 9router does.
func NewRequestID(unixMilli int64) string {
	current := uint64(unixMilli)*0x1000 + 1
	timeHex := ""
	for shift := 40; shift >= 0; shift -= 8 {
		timeHex += fmt.Sprintf("%02x", (current>>uint(shift))&0xff)
	}
	return "msg_" + timeHex + base62Random(14)
}

// TranslateSessionId mirrors 9router translateSessionId: reuse a
// client-supplied session id when it already carries the OpenCode shape;
// otherwise derive one deterministically from sha256("opencode\0" +
// serviceName + "\0" + sessionId) so a re-run of the same conversation
// produces the same session id (observable, not random). The 5-byte time
// prefix is masked to 36 bits and pin counter to 1.
func TranslateSessionId(sessionId, serviceName string) string {
	if sessionId != "" && sessionRew.MatchString(strings.TrimSpace(sessionId)) {
		return strings.TrimSpace(sessionId)
	}
	digest := sha256.Sum256([]byte("opencode\x00" + serviceName + "\x00" + sessionId))
	// 5 bytes drop from the top → 36 bits of time, counter 1.
	time := int64(digest[0])<<32 | int64(digest[1])<<24 | int64(digest[2])<<16 | int64(digest[3])<<8 | int64(digest[4])
	time &= int64(uint64(1)<<36 - 1)
	timeHex := DescendingTimeHex(time, 1)
	tail := make([]byte, 0, 14)
	for i := 6; i < 20; i++ {
		tail = append(tail, base62[int(digest[i])%len(base62)])
	}
	return "ses_" + timeHex + string(tail)
}

// PrepareHeaders builds the OpenCode-shaped Authorization and
// x-opencode-* headers onto an outbound request. downstreamUA is the
// caller's User-Agent if it already carries a valid opencode version;
// apiKey is the connection's credential (the free-tier path overrides it
// with the public bearer). Rotating the api key does NOT affect the
// session/request ids — those are function of UnixMilli + scope only — so
// a key rotation mid-conversation does not orphan the session.
func PrepareHeaders(h http.Header, downstreamUA, apiKey, serviceName string, unixMilli int64, freeTier bool) http.Header {
	if h == nil {
		h = http.Header{}
	}
	if HasValidOpencodeVersion(downstreamUA) {
		h.Set("User-Agent", downstreamUA)
	} else {
		h.Set("User-Agent", OpenCodeUA)
	}
	h.Set(ClientHeader, "desktop")
	h.Set(ProjectHeader, "global")
	h.Set(SessionHeader, TranslateSessionId("", serviceName))
	h.Set(RequestHeader, NewRequestID(unixMilli))
	if freeTier {
		// Free-tier gate: upstream 403s everything else.
		h.Set("Authorization", "Bearer public")
	} else if strings.TrimSpace(apiKey) != "" {
		h.Set("Authorization", "Bearer "+apiKey)
	}
	return h
}

// EnsureFingerprintTools mirrors 9router ensureChatFingerprintTools: it
// mutates the *tools array* on a chat-completions body so the built-in
// bash/glob/grep/read quartet is always present (the exact upstream
// fingerprint models fingerprint to accept a call). idempotent: an
// existing tool is left alone, its name is the only key. tools is the
// caller's map-backed tools array; pass empty to inject the quartet into
// a body that has none.
func EnsureFingerprintTools(tools []map[string]any) []map[string]any {
	present := map[string]bool{}
	for _, t := range tools {
		if name, ok := toolNameOf(t); ok && name != "" {
			present[name] = true
		}
	}
	for _, name := range FingerprintTools {
		if present[name] {
			continue
		}
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": "OpenCode built-in " + name + " tool",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		})
	}
	return tools
}

func toolNameOf(tool map[string]any) (string, bool) {
	if len(tool) == 0 {
		return "", false
	}
	if raw, ok := tool["name"].(string); ok {
		return strings.TrimSpace(raw), true
	}
	fn, _ := tool["function"].(map[string]any)
	if fn == nil {
		return "", false
	}
	raw, _ := fn["name"].(string)
	return strings.TrimSpace(raw), true
}

// sha256Fallback exists so tests can inspect the deterministic time derivation
// without needing the full session-id plumbing.
var _ = hex.EncodeToString

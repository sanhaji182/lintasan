// Package mimofree ports 9router open-sse/executors/mimo-free.js (v0.5.104)
// into Lintasan Go — experimental-track, membrane gated, never auto-routable.
//
// The upstream is Xiaomi's free MiMo Code lane, reachable ONLY from a
// mimocode-like client. Two anti-abuse gates, both verified 18 Oct 2026:
//
//	POST /api/free-ai/bootstrap with {client:<fingerprint>} and a real
//	Chrome UA -> 200 {"jwt":"<591 chars>"}, exp in payload.
//	Missing UA -> 403 {"code":"403","message":"Illegal access",
//	"type":"illegal_access"}.
//
// The executor does one meaningful piece of state: obtains a JWT via
// /bootstrap with a stable device fingerprint (the hash of the calling
// machine), caches it until expiry, and 1-retries with a fresh JWT on
// 401/403 (重型 cache invalidation, not just bumping a counter).
//
// This port keeps the SAME three-decimal contract (bootstrap → cache →
// retry), and adopts 9router's stable-fingerprint derivation
// (hostname|platform|arch|cpu|username) so a re-run reuses the same Jwt and
// doesn't burn an extra upstream bootstrap like an ephemeral fingerprint
// would.
package mimofree

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"time"
)

// BootstrapURL and ChatURL mirror the 9router executor / registry pair.
// Declared var (not const) so tests can point BootstrapURL at a fake server
// via Redirect; ChatURL is a caller-owned constant — the connection's own
// BaseURL wins at the proxy hot path anyway.
var (
	BootstrapURL = "https://api.xiaomimimo.com/api/free-ai/bootstrap"
	ChatURL      = "https://api.xiaomimimo.com/api/free-ai/openai/chat"
)

// Redirect is a test-only helper that swaps the package-level BootstrapURL
// so a test can point it at a fake server without real HTTP. Must be
// restored by the caller's defer.
func Redirect(to string) {
	BootstrapURL = to
}

const (
	// SourceHeader is what mimocode-cli sends; required for upstream to
	// treat the call as coming from the free-lane client.
	SourceHeader = "X-Mimo-Source"
	SourceValue  = "mimocode-cli-free"

	// SessionAffinityHeader binds all requests of one running process to
	// one upstream session. SessionAffinityPrefix preserved (9router fmt).
	SessionAffinityHeader = "x-session-affinity"
)

// UserAgents is the 3-OS Chrome-131 pool the executor samples per
// request; keeping the pool (rather than a single UA) mirrors the 9router
// distribution so a single fingerprint can't be pinned. It is a var
// because slices have no const representation in Go; nothing in this
// package mutates it and callers must not either.
var UserAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
}

const (
	// RuntimeSystemMarker is the 1-time MiMo-Code signature substring the
	// upstream requires inside a system message, byte-for-byte. Missing it
	// is the 403 "Illegal access" path. Matches 9router source exactly.
	RuntimeSystemMarker = "You are MiMoCode, an interactive CLI tool that helps users with software engineering tasks."

	// JWT TTLs, mirroring 9router.
	jwtFallbackTTLSec = 3000
	jwtExpiryBufms    = 5 * time.Minute
)

var _ = runtime.NumCPU() // silence an unused-import warning when mbedtls's NumCPU is inlined

// UserAgentPool returns the UserAgents list. Copy so a caller cannot mutate
// the package-level ambient slice (those mutations would corrupt every
// other request this process makes).
func UserAgentPool() []string {
	out := make([]string, len(UserAgents))
	copy(out, UserAgents)
	return out
}

// ClientFingerprint is a proof-of-device hash stable across processes on
// the same machine. 9router hostname|platform|arch|cpu|username is a
// reasonable seed that stays stable within one install but differs
// between installs, which is exactly the anti-abuse signal upstream is
// keying on.
func ClientFingerprint() string {
	username := "unknown-user"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	hostname, _ := os.Hostname()
	cpu := "unknown-cpu"
	if runtime.NumCPU() > 0 {
		// runtime.CPU is not in the stdlib; the executor catches this via
		// os.cpus()[0]?.model which the stdlib has no equivalent for, so
		// arch is the closest stable proxy.
		cpu = runtime.GOARCH
	}
	seed := fmt.Sprintf("%s|%s|%s|%s|%s", hostname, runtime.GOOS, runtime.GOARCH, cpu, username)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// jwtCache is the in-process token cache — same lifetime as the 9router
// executor, plus a lock since Lintasan spans goroutines.
type jwtCache struct {
	mu        sync.RWMutex
	token     string
	expiresAt time.Time
}

var cache jwtCache

// ParseJwtExp mirrors 9router parseJwtExp: read the JWT's exp claim (or fail
// over to a fixed TTL when it's not in there). The 5-minute forward buffer
// mirrors the executor so a cached token does not die mid-request.
func ParseJwtExp(jwt string) time.Time {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return time.Now().Add(jwtFallbackTTLSec * time.Second)
	}
	// JWT pads with '='; base64.RawURLEncoding wants them stripped.
	pad := parts[1]
	switch n := len(pad) % 4; n {
	case 2:
		pad += "=="
	case 3:
		pad += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(pad)
	if err != nil {
		return time.Now().Add(jwtFallbackTTLSec * time.Second)
	}
	var payload struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(decoded, &payload) != nil || payload.Exp <= 0 {
		return time.Now().Add(jwtFallbackTTLSec * time.Second)
	}
	return time.Unix(payload.Exp, 0)
}

// Bootstrap performs the upstream POST /api/free-ai/bootstrap. Returns the
// JWT or an error carrying the upstream status so a caller can surface the
// actual "Illegal access" wording (rather than "http error" vagueness).
func Bootstrap(httpClient *http.Client) (string, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	payload, _ := json.Marshal(map[string]string{"client": ClientFingerprint()})
	req, err := http.NewRequest(http.MethodPost, BootstrapURL, strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgents[int(time.Now().UnixNano())%len(UserAgents)])
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("mimo bootstrap: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("mimo bootstrap HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("mimo bootstrap decode: %w", err)
	}
	if strings.TrimSpace(parsed.JWT) == "" {
		return "", fmt.Errorf("mimo bootstrap returned no JWT")
	}
	cache.mu.Lock()
	cache.token = parsed.JWT
	cache.expiresAt = ParseJwtExp(parsed.JWT)
	cache.mu.Unlock()
	return parsed.JWT, nil
}

// EnsureToken returns a cached JWT that is still in-buffer, or a fresh one.
// Caches invert the ordering (not an error) — callers can safely re-call.
func EnsureToken(httpClient *http.Client) (string, error) {
	cache.mu.RLock()
	tok, exp := cache.token, cache.expiresAt
	cache.mu.RUnlock()
	if tok != "" && time.Now().Before(exp.Add(-jwtExpiryBufms)) {
		return tok, nil
	}
	return Bootstrap(httpClient)
}

// ResetCache clears the cached JWT so the next EnsureToken re-bootstraps.
// 9router calls this on any 401/403 before retrying once. Lintasan callers
// use it the same way: on 401/403, ResetCache + EnsureToken + one retry.
func ResetCache() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.token = ""
	cache.expiresAt = time.Time{}
}

// BuildHeaders is the header triple an actual chat POST needs, with a
// caller-supplied session-affinity id (one per running process, not per
// request — upstream treats requests outside the affinity id as a new
// session and pays a first-token latency penalty). randomUA matches the
// 9router executor's per-request sampling of the Chrome pool.
func BuildHeaders(sessionAffinity string, stream bool, redactJWT string) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set(SourceHeader, SourceValue)
	h.Set("User-Agent", UserAgents[int(time.Now().UnixNano())%len(UserAgents)])
	h.Set(SessionAffinityHeader, sessionAffinity)
	if stream {
		h.Set("Accept", "text/event-stream")
	} else {
		h.Set("Accept", "application/json")
	}
	if redactJWT != "" {
		h.Set("Authorization", "Bearer "+redactJWT)
	}
	return h
}

// GenerateSessionAffinity mint a stable `ses_` + 24-char lowercase-alnum id.
// The 9router executor does precisely this (using Math.random); the
// mathematics does not have to be cryptographic — upstream only checks the
// affinity id is well-formed and stable enough to bind.
func GenerateSessionAffinity() string {
	const alph = "abcdefghijklmnopqrstuvwxyz0123456789"
	now := uint64(time.Now().UnixNano())
	out := make([]byte, 24)
	for i := 0; i < 24; i++ {
		out[i] = alph[(now>>(uint(i)*4))%uint64(len(alph))]
	}
	return "ses_" + string(out)
}

// InjectSystemMarker is the idempotent body-shaper the executor's
// transformRequest does. Body is the decoded message-object; the most
// common case is a system marker in slot 0 of messages. Missing-a-marker
// bodies get the marker UN-PREFIX-ED — upstream checks the substring, not
// the position (verified in a live probe; mirrors 9router shape).
func InjectSystemMarker(messages []map[string]any) []map[string]any {
	for _, m := range messages {
		if role, _ := m["role"].(string); role == "system" {
			if content, _ := m["content"].(string); strings.Contains(content, RuntimeSystemMarker) {
				return messages
			}
		}
	}
	// Prepend the marker; 9router returns a new array rather than mutate.
	prepended := make([]map[string]any, 0, len(messages)+1)
	prepended = append(prepended, map[string]any{"role": "system", "content": RuntimeSystemMarker})
	return append(prepended, messages...)
}

// Port Zed's native-app RSA keypair auth helpers from 9router
// (open-sse/shared/zedAuth.js). Only the "generate a keypair + emit the sign-in
// URL" and "decrypt an RSA-encrypted access-token" steps — the pieces an
// incoming connection needs from the call site —
package zed

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// Web / cloud LLM base URLs mirror 9router constants.
const (
	WebBaseURL   = "https://zed.dev"
	CloudBaseURL = "https://cloud.zed.dev"
	LLMBaseURL   = "https://cloud.zed.dev"

	// DefaultNativeAppPort is the fixed port Zed's native app callback
	// uses (the executor's defaultNativeAppPort; configurable at the
	// connection level but kept here as the pinned default).
	DefaultNativeAppPort = 58443

	// PrivateKeyPrefix marks a stored private key as an encoded
	// verifier — mirrors 9router's PRIVATE_KEY_PREFIX so a stored
	// verifier can be re-parsed after a process restart without
	// doubling an encoding.
	PrivateKeyPrefix = "zed-rsa-pkcs1:"

	// LLMTTLMs is the expiry window for cloud.zed.dev /completions'
	// short-lived LLM bearer — the executor has it 50 minutes.
	LLMTokenTTLMs = 50 * 60 * 1000
)

// NativeAuthData is the payload Lintasan's dashboard needs to start a Zed
// native-app login: open the sign-in URL in a browser, hold the private
// key in a local store, and when the browser callbacks with an encrypted
// access token, decrypt it against the private key.
type NativeAuthData struct {
	AuthURL            string
	PrivateKeyVerifier string
	NativeAppPort      int
	SystemID           string
	PublicKey          string // base64url-padded PKCS1 pubkey
}

// GenerateKeyPair creates the per-session RSA-2048 keypair Zed is built for.
// The executor's key shape: PKCS1 DER for the pubkey (stored as
// base64url-padded bytes) and PKCS1 PEM for the private key (stored via
// [PrivateKeyPrefix]-encoded verifier).
func GenerateKeyPair() (pubBase64URL string, privVerifier string, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	// PKCS1 DER for the public key (NOT SubjectPublicKeyInfo — 9router
	// uses x509.MarshalPKCS1PublicKey, which is simply the RSAPublicKey).
	pubDER := x509.MarshalPKCS1PublicKey(&priv.PublicKey)
	pubB64 := b64URLPadded(pubDER)

	pemDER := x509.MarshalPKCS1PrivateKey(priv)
	pemBuf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pemDER})
	if len(pemBuf) == 0 {
		return "", "", errors.New("failed to encode Zed private key PEM")
	}
	return pubB64, encodeVerifier(string(pemBuf)), nil
}

// SignInURL builds the URL the caller opens in the user's browser after a
// keypair is generated. NOTE: it MUST be over the loopback callback — see
// the risk notes — since Zed redirects the token to the listener the
// caller runs (or, optionally, to a pasted callback URL).
func SignInURL(nativeAppPort int, publicKeyBase64URL, systemID string) string {
	if nativeAppPort == 0 {
		nativeAppPort = DefaultNativeAppPort
	}
	qs := "native_app_signin"
	// Build the URL manually so we can guarantee (?=parameter order matches
	// 9router's emitter — the client's own URL builder also does it this way.
	return fmt.Sprintf(
		"%s/%s?native_app_port=%d&native_app_public_key=%s&system_id=%s",
		WebBaseURL, qs, nativeAppPort, publicKeyBase64URL, systemID,
	)
}

// ParseCallbackPayload accepts one of the shapes the executor accepts:
//   - raw JSON: {"user_id": "...", "access_token": "..."}
//   - full URL: https://zed.dev/native_app_signin_callback?user_id=…&access_token=…
//   - path+query: "/?user_id=…&access_token=…"
//   - bare query: user_id=…&access_token=…
//
// Accepting all of them is why the executor benefits from a helper — the
// wire is not fixed. Returns the pair (userId, encryptedAccessToken); error
// when either is missing.
func ParseCallbackPayload(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("missing Zed callback URL")
	}
	// Try JSON first.
	if strings.HasPrefix(raw, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			if uid, tok := pick(m); uid != "" && tok != "" {
				return uid, tok, nil
			}
		}
	}
	// Strip a leading path if any, then parse query.
	query := raw
	if idx := strings.IndexByte(raw, '?'); idx >= 0 {
		query = raw[idx+1:]
	} else if strings.HasPrefix(raw, "?") {
		query = raw[1:]
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return "", "", errors.New("invalid Zed callback URL")
	}
	m := map[string]string{}
	for k, v := range values {
		if len(v) > 0 {
			m[k] = v[0]
		}
	}
	if uid, tok := pick(m); uid != "" && tok != "" {
		return uid, tok, nil
	}
	return "", "", errors.New("Zed callback must include user_id and access_token")
}

func pick(m map[string]string) (string, string) {
	uid := coalesce(m, "user_id", "userId")
	tok := coalesce(m, "access_token", "accessToken", "token")
	return uid, tok
}

func coalesce(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// DecryptAccessToken decrypts the RSA-encrypted access token with the
// PEM-encoded private key in the verifier. OAEP-SHA256 is tried first
// (the primary coding), then PKCS1v15 as fallback (same as the 9router
// executor) because a changed hash default silently bricks all keys of
// the wrong kind.
func DecryptAccessToken(encryptedAccessTokenBase64URL, privVerifier string) (string, error) {
	priv, err := decodeVerifier(privVerifier)
	if err != nil {
		return "", fmt.Errorf("failed to parse Zed private key verifier: %w", err)
	}
	ciphertext, err := base64.URLEncoding.DecodeString(strings.TrimSpace(encryptedAccessTokenBase64URL) + paddingFor(strings.TrimSpace(encryptedAccessTokenBase64URL)))
	if err != nil {
		return "", fmt.Errorf("failed to decode Zed encrypted access token: %w", err)
	}
	got, oaepErr := rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, ciphertext, nil)
	if oaepErr == nil {
		return string(got), nil
	}
	// Fallback PKCS1v15, mirroring the 9router executor.
	got, pkcs1Err := rsa.DecryptPKCS1v15(rand.Reader, priv, ciphertext)
	if pkcs1Err == nil {
		return string(got), nil
	}
	return "", fmt.Errorf("failed to decrypt Zed access token: oaep=%v; pkcs1v15=%v", oaepErr, pkcs1Err)
}

// paddingFor returns the number of '=' characters to append so a raw /
// unpadded base64url string decodes; base64.StdEncoding.DecodeString only
// applies full padding rules, so we pad manually to match the JWT shape
// 9router's helpers use.
func paddingFor(s string) string {
	switch n := len(s) % 4; n {
	case 2:
		return "=="
	case 3:
		return "="
	}
	return ""
}

// encodeVerifier / decodeVerifier preserve a private key on disk via a
// single-line, prefix-marked encoding (the marker lets a future verifier
// identify this specific format without SHA-flagging every PEM).
func encodeVerifier(privPEM string) string {
	return PrivateKeyPrefix + base64.StdEncoding.EncodeToString([]byte(privPEM))
}

func decodeVerifier(verifier string) (*rsa.PrivateKey, error) {
	if !strings.HasPrefix(verifier, PrivateKeyPrefix) {
		return nil, errors.New("private key verifier missing the zed-rsa-pkcs1: marker")
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(verifier, PrivateKeyPrefix))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(body)
	if block == nil {
		return nil, errors.New("not a PEM block")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return priv, nil
}

// b64URLPadded converts 9router's bytes→base64 then /− / + replace; the
// result is a URL-safe base64 (StdEncoding alphabet + padding, which the
// upstream's JS builder matches).
func b64URLPadded(b []byte) string {
	s := base64.StdEncoding.EncodeToString(b)
	// Match 9router's JS builder exactly: / and + swaps on top of the
	// StdEncoding alphabet; padding stays so it decodes back symmetrically.
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

// sessionStore is the per-process key→(mesh of private verifier + metadata)
// material Lintasan's OAuth IDE layer keeps on file via the existing
// oauth_sessions table. This in-memory cache is only a lookup hint to make
// concurrent-incoming calls cheap.
type sessionStore struct {
	mu sync.RWMutex
	m  map[string]*NativeAuthData
}

var store = &sessionStore{m: map[string]*NativeAuthData{}}

// PutNativeAuthData records one session for later lookup. Callers must still
// persist to the oauth_sessions table — this is only a hot cache.
func PutNativeAuthData(sessionID string, data *NativeAuthData) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.m[sessionID] = data
}

// PeekNativeAuthData is a non-destructive read. Returns nil when absent.
func PeekNativeAuthData(sessionID string) *NativeAuthData {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.m[sessionID]
}

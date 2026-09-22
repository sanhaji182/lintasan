// Package qoder implements Qoder as a Lintasan provider.
//
// STATUS — EXPERIMENTAL TRACK, RE-DERIVED PROTOCOL.
//
// This package is a native Go port of the "cosy" wire protocol that the Qoder
// IDE client speaks to its own backend. It exists so Qoder can be a first-class
// entry in Lintasan's provider registry — with real pooling, failover,
// circuit-breaking and model-aware routing — instead of requiring a separate
// local bridge process per credential.
//
// IMPORTANT — READ BEFORE USING:
//
//   - Every provider here reports Track() == TrackExperimental. That is
//     load-bearing, not cosmetic: the production resolver only selects
//     TrackOfficial providers, so a Qoder connection can NEVER be reached by
//     ordinary routing. It is opt-in only.
//   - Lintasan's own README states the project does not support reverse
//     engineering commercial IDE internal endpoints. This package is exactly
//     that. Enabling it is a product decision that must be made deliberately;
//     the policy text and this code cannot both be true at once.
//   - The protocol below was derived by porting an existing third-party
//     implementation and then re-verified live against the upstream service.
//     It is not a published or stable contract. Upstream may change it without
//     notice, and when it does this package will fail loudly rather than
//     silently degrade.
//
// WHAT IS VERIFIED (live, from the Lintasan host):
//
//	PAT -> job token exchange            OK   (id, name, email, plan)
//	model list                           OK   (HTTP 200, per-account model sets)
//	chat streaming                       OK   (SSE deltas)
//
// WHAT IS KNOWN-FRAGILE (see provider.go and the notes on each):
//
//   - Dead credentials are rejected at CHAT time, not at auth time, and the
//     rejection arrives as an HTTP 200 whose first SSE frame carries an error
//     envelope. A reader that waits for data blocks forever. Handled explicitly.
//   - Upstream returns a queue state (code 10605) when a model is busy. The
//     reference implementation ignores it entirely; this port surfaces it.
//   - Accounts are NOT interchangeable: an account's model list differs per
//     account. Model-aware selection is mandatory, not an optimisation.
package qoder

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"time"
)

// Protocol constants observed from the Qoder IDE client. Version is reported to
// upstream in payloads and headers; AppCode and SecretB64 are the fixed inputs
// to the legacy request signature. These are protocol constants, not secrets —
// SecretB64 decodes to the ASCII string "war, war never changes".
const (
	// Version is the cosy protocol version reported upstream.
	Version = "1.0.10"

	// AppCode identifies the client application.
	AppCode = "cosy"

	// SecretB64 is the fixed signing constant for the legacy signature scheme.
	SecretB64 = "d2FyLCB3YXIgbmV2ZXIgY2hhbmdlcw=="

	// ServerPubKeyPEM is the upstream public key. The client RSA-encrypts a
	// per-session temp key with it; only upstream can decrypt the result.
	ServerPubKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`
)

// ============================================================================
// Custom base64 ("cosy" envelope encoding)
// ============================================================================
//
// Request and response bodies on the jobToken and model/chat endpoints are
// standard-base64-encoded JSON that is then (a) permuted and (b) transliterated
// into a private alphabet. The permutation is a three-block rotation of the
// encoded string, not a per-character shuffle, and it is its own inverse.

const (
	customAlphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"
	stdAlphabet    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	customPad      = '$'
)

// std2custom and custom2std are 128-entry transliteration tables built once.
var (
	std2custom [128]int
	custom2std [128]int
)

func init() {
	for i := range std2custom {
		std2custom[i] = -1
		custom2std[i] = -1
	}
	for i := 0; i < 64; i++ {
		std2custom[stdAlphabet[i]] = int(customAlphabet[i])
		custom2std[customAlphabet[i]] = int(stdAlphabet[i])
	}
	// Padding: the private alphabet has no '='; '$' stands in for it.
	std2custom['='] = customPad
	custom2std[customPad] = '='
}

// EncodeBase64 wraps plaintext in the cosy envelope: standard base64, three
// block rotation, private alphabet.
func EncodeBase64(plaintext []byte) (string, error) {
	std := base64.StdEncoding.EncodeToString(plaintext)
	n := len(std)
	if n == 0 {
		return "", nil
	}
	// Three-block rotation: [tail][middle][head]. Its own inverse.
	a := n / 3
	rotated := std[n-a:] + std[a:n-a] + std[:a]

	out := make([]byte, n)
	for i := 0; i < n; i++ {
		c := int(rotated[i])
		if c >= 128 || std2custom[c] < 0 {
			return "", fmt.Errorf("qoder: byte %d outside base64 alphabet", c)
		}
		out[i] = byte(std2custom[c])
	}
	return string(out), nil
}

// DecodeBase64 reverses EncodeBase64.
func DecodeBase64(encoded string) ([]byte, error) {
	n := len(encoded)
	if n == 0 {
		return nil, nil
	}
	mapped := make([]byte, n)
	for i := 0; i < n; i++ {
		c := int(encoded[i])
		if c >= 128 || custom2std[c] < 0 {
			return nil, fmt.Errorf("qoder: byte %d outside cosy alphabet", c)
		}
		mapped[i] = byte(custom2std[c])
	}
	a := n / 3
	// Same rotation applied to the mapped string restores standard order.
	std := string(mapped[n-a:]) + string(mapped[a:n-a]) + string(mapped[:a])
	return base64.StdEncoding.DecodeString(std)
}

// ============================================================================
// Request signatures
// ============================================================================

// MD5Hex returns the lowercase hex MD5 of s. MD5 is used here because it is
// what the upstream protocol specifies; it is not a security choice and this
// package never relies on it for integrity.
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", sum)
}

// CurrentDate returns the RFC1123 GMT date the jobToken handshake signs.
func CurrentDate() string {
	return time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
}

// SignLegacy produces the signature used on the jobToken handshake, which is
// bound to the request date but not to the body.
func SignLegacy(date string) string {
	return MD5Hex(AppCode + "&" + SecretB64 + "&" + date)
}

// SignRequest produces the per-request signature used on all post-auth API
// calls. Unlike SignLegacy it covers the body and the URL path (with the
// leading "/algo" stripped), so a tampered body or a replayed path will not
// verify. cosyDate is Unix seconds.
func SignRequest(payloadB64, cosyKey, cosyDate string, body []byte, pathWithoutAlgo string) string {
	return MD5Hex(payloadB64 + "\n" + cosyKey + "\n" + cosyDate + "\n" + string(body) + "\n" + pathWithoutAlgo)
}

// ============================================================================
// Device fingerprint derivation
// ============================================================================
//
// Upstream expects a stable pseudo-device identity per credential. The values
// are derived deterministically rather than generated randomly so that the same
// credential always presents as the same device across process restarts, which
// is what keeps it from tripping machine-fingerprint risk controls.
//
// Fingerprinter carries the install salt as a field rather than package-global
// state: derivations are pure functions of (salt, seed), so the type is safe for
// concurrent use and trivially testable.

// Fingerprinter derives the device identity presented to upstream.
//
// Salt is an optional per-deployment value mixed into every derived seed. It
// exists because a pure uid->fingerprint mapping means every deployment of this
// code presents identical fingerprints for a given account, which upstream can
// recognise and block globally. Changing the salt changes every fingerprint and
// is therefore operationally equivalent to changing devices — it must be set
// once and then left alone.
type Fingerprinter struct {
	salt string
}

// NewFingerprinter returns a Fingerprinter with the given install salt. An empty
// salt is the unsalted (cross-implementation compatible) mode.
func NewFingerprinter(salt string) *Fingerprinter {
	return &Fingerprinter{salt: salt}
}

// Salt reports the configured install salt.
func (f *Fingerprinter) Salt() string { return f.salt }

// saltedSeed mixes the install salt into a derivation seed.
func (f *Fingerprinter) saltedSeed(seed string) string {
	if f.salt == "" {
		return seed
	}
	return seed + "|salt:" + f.salt
}

// FingerprintSeed picks the derivation seed for a credential. The account id is
// preferred; before it is known (i.e. during the job token exchange, which is
// what discovers it) the credential itself is the seed. Both paths are stable.
func FingerprintSeed(accountID, credential string) string {
	if accountID != "" {
		return accountID
	}
	return "cred:" + credential
}

// labelID is the shared hash construction: md5(label + ":" + saltedSeed(seed)).
func (f *Fingerprinter) labelID(label, seed string) string {
	sum := md5.Sum([]byte(label + ":" + f.saltedSeed(seed)))
	return fmt.Sprintf("%x", sum)
}

// MachineID derives the 32-hex device id sent as cosy-machineid.
func (f *Fingerprinter) MachineID(seed string) string { return f.labelID("machine", seed) }

// SessionID derives a stable session identifier (32 hex).
func (f *Fingerprinter) SessionID(seed string) string { return f.labelID("session", seed) }

// MachineType derives the 18-character device type sent as cosy-machinetype.
func (f *Fingerprinter) MachineType(seed string) string { return f.labelID("machinetype", seed)[:18] }

// MachineToken derives the device token sent as cosy-machinetoken: base64url of
// sha512 truncated to 43 characters.
func (f *Fingerprinter) MachineToken(seed string) string {
	sum := sha512.Sum512([]byte("machinetoken:" + f.saltedSeed(seed)))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:43]
}

// ============================================================================
// Session establishment
// ============================================================================

// Identity is the account identity returned by the job token exchange and
// replayed to upstream inside the encrypted session blob.
type Identity struct {
	Name               string
	AccountID          string
	UID                string
	YxUID              string
	OrganizationID     string
	OrganizationName   string
	UserType           string
	SecurityOauthToken string
	RefreshToken       string
}

// Session is an authenticated upstream session: the encrypted identity blob,
// the RSA-wrapped session key, and the device identity to present alongside it.
type Session struct {
	Identity     Identity
	CosyKey      string
	Info         string
	MachineID    string
	MachineToken string
	MachineType  string
}

// NewSession builds a session for an authenticated identity. It generates a
// fresh 16-byte temp key, RSA-wraps it with the upstream public key to form
// cosy-key, and AES-CBC encrypts the identity JSON with that temp key to form
// the info blob. Both are base64 standard encoded.
func NewSession(id Identity, machineID, machineToken, machineType string) (*Session, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("qoder: session key: %w", err)
	}
	// 16 ASCII hex characters, used directly as the AES key.
	tempKey := []byte(fmt.Sprintf("%x", raw)[:16])

	rw, err := rsaEncrypt([]byte(tempKey))
	if err != nil {
		return nil, fmt.Errorf("qoder: wrap session key: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"name":                 id.Name,
		"aid":                  id.AccountID,
		"uid":                  id.UID,
		"yx_uid":               id.YxUID,
		"organization_id":      id.OrganizationID,
		"organization_name":    id.OrganizationName,
		"user_type":            id.UserType,
		"security_oauth_token": id.SecurityOauthToken,
		"refresh_token":        id.RefreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("qoder: marshal identity: %w", err)
	}

	enc, err := aesEncrypt(payload, tempKey)
	if err != nil {
		return nil, fmt.Errorf("qoder: encrypt identity: %w", err)
	}

	return &Session{
		Identity:     id,
		CosyKey:      base64.StdEncoding.EncodeToString(rw),
		Info:         base64.StdEncoding.EncodeToString(enc),
		MachineID:    machineID,
		MachineToken: machineToken,
		MachineType:  machineType,
	}, nil
}

// BuildPayload returns the base64 payload identifying the client version and
// carrying the encrypted identity blob. A fresh request id is embedded per call.
func (s *Session) BuildPayload() (string, error) {
	data, err := json.Marshal(map[string]string{
		"cosyVersion": Version,
		"ideVersion":  "",
		"info":        s.Info,
		"requestId":   NewUUID(),
		"version":     "v1",
	})
	if err != nil {
		return "", fmt.Errorf("qoder: marshal payload: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ComposeBearer builds the Authorization header value.
func ComposeBearer(payloadB64, signature string) string {
	return "Bearer COSY." + payloadB64 + "." + signature
}

// PathSig strips the leading /algo from a URL path, matching what the request
// signature covers. Upstream signs the client-visible path, not the /algo-prefixed
// routing prefix.
func PathSig(rawPath string) string {
	return strings.TrimPrefix(rawPath, "/algo")
}

// rsaEncrypt wraps plaintext with the upstream public key.
func rsaEncrypt(plaintext []byte) ([]byte, error) {
	block, _ := pem.Decode([]byte(strings.ReplaceAll(ServerPubKeyPEM, "\r", "")))
	if block == nil {
		return nil, fmt.Errorf("qoder: decode upstream public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("qoder: parse upstream public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("qoder: upstream public key is not RSA")
	}
	return rsa.EncryptPKCS1v15(rand.Reader, rsaPub, plaintext)
}

// aesEncrypt applies PKCS#7-style padding and AES-CBC with the key as its own IV.
func aesEncrypt(plain, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	pad := bs - len(plain)%bs
	padded := make([]byte, len(plain)+pad)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(pad)
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:bs]).CryptBlocks(out, padded)
	return out, nil
}

// ============================================================================
// Random identifiers
// ============================================================================

// NewUUID returns a random RFC4122-shaped identifier.
func NewUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("qoder: crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewRequestID returns a 24-character hex identifier for request-id fields.
func NewRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("qoder: crypto/rand unavailable: " + err.Error())
	}
	return fmt.Sprintf("%x", b)[:24]
}

// UnixSec returns the current Unix time in seconds (the cosy-date value).
func UnixSec() int64 { return time.Now().Unix() }

// UnixMs returns the current Unix time in milliseconds.
func UnixMs() int64 { return time.Now().UnixMilli() }

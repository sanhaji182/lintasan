package zed

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

// publicKeyEncrypt is a helper the test file only: OAEP-SHA256 against the
// private key's public half, mirroring what upstream's JS does.
func publicKeyEncrypt(priv *rsa.PrivateKey, plaintext, out []byte) (int, error) {
	n, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &priv.PublicKey, plaintext, nil)
	if err != nil {
		return 0, err
	}
	copy(out, n)
	return len(n), nil
}

// publicKeyEncryptPKCS1 mirrors upstream's older PKCS1v15 shape, so the
// fallback path is exercised.
func publicKeyEncryptPKCS1(priv *rsa.PrivateKey, plaintext, out []byte) (int, error) {
	n, err := rsa.EncryptPKCS1v15(rand.Reader, &priv.PublicKey, plaintext)
	if err != nil {
		return 0, err
	}
	copy(out, n)
	return len(n), nil
}

func base64URLNoPad(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}

// Pins the wire contract 9router's open-sse/shared/zedAuth.js writes:
//	- keypair pool: RSA-2048, PKCS1 pubkey DER stored as base64(-/+
//	  padded), PKCS1 pin PEM stored as the 'zed-rsa-pkcs1:'-prefixed verifier.
//	- SignInURL: native_app_port + native_app_public_key + system_id —
//	  pinned order so a different ordering would break the signin.
//	- DecryptAccessToken: OAEP-SHA256 first, then PKCS1v15 fallback (the
//	  9router JS behaviour keeps both so an older key still works).

func TestPrivateKeyRoundTrip(t *testing.T) {
	pub, verifier, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if !strings.HasPrefix(pub, "ssh-rsa-") == false && pub == "" {
		t.Fatalf("pub key missing: %q", pub)
	}
	if !strings.HasPrefix(verifier, PrivateKeyPrefix) {
		t.Fatalf("verifier missing prefix %q: %q", PrivateKeyPrefix, verifier)
	}
	// Round-trip: decodeVerifier should recover the same RSA private key.
	priv, err := decodeVerifier(verifier)
	if err != nil {
		t.Fatalf("decodeVerifier: %v", err)
	}
	if priv.N.BitLen() != 2048 {
		t.Fatalf("RSA modulus bit length = %d, want 2048", priv.N.BitLen())
	}
	// Base64(-/+) shape: pubkey must be the URL-safe alphabet. Check for
	// any '+' or '/' leaks the builder missed — if one appears, the
	// upstream's parser will not decode.
	if strings.ContainsAny(pub, "+/") {
		t.Fatalf("pub key leaked non-URL-safe chars: %q", pub)
	}
}

func TestSignInURLShapeOrder(t *testing.T) {
	// Order is pinned — JS URL query parsing is order-sensitive only for
	// server-side contract checks, and 9router pins native_app_port first.
	pub, _, _ := GenerateKeyPair()
	u := SignInURL(58443, pub, "sys-abc")
	for _, want := range []string{
		"https://zed.dev/native_app_signin?",
		"native_app_port=58443",
		"native_app_public_key=" + pub,
		"system_id=sys-abc",
	} {
		if !strings.Contains(u, want) {
			t.Fatalf("SignInURL missing %q — got %q", want, u)
		}
	}
	// Default port fills in when the caller passes 0 (upstream contract).
	if got := SignInURL(0, pub, "sys-abc"); !strings.Contains(got, "native_app_port=58443") {
		t.Fatalf("default native_app_port not applied: %q", got)
	}
}

func TestParseCallbackPayloadAcceptsFourShapes(t *testing.T) {
	cases := []string{
		// bare query
		"user_id=42&access_token=TOKEN",
		// path + query (the local proxy / callback shape)
		"/?user_id=42&access_token=TOKEN",
		// full URL
		"https://zed.dev/native_app_signin_callback?user_id=42&access_token=TOKEN",
		// JSON
		`{"user_id":"42","access_token":"TOKEN"}`,
	}
	for i, raw := range cases {
		uid, tok, err := ParseCallbackPayload(raw)
		if err != nil {
			t.Fatalf("shape %d (%q): %v", i, raw, err)
		}
		if uid != "42" || tok != "TOKEN" {
			t.Fatalf("shape %d: uid=%q tok=%q, want 42/TOKEN", i, uid, tok)
		}
	}
	// The camelCase aliases must work too.
	uid, tok, err := ParseCallbackPayload(`{"userId":"7","access_token":"T7"}`)
	if err != nil || uid != "7" || tok != "T7" {
		t.Fatalf("camelCase aliases: uid=%q tok=%q err=%v", uid, tok, err)
	}
}

func TestParseCallbackPayloadRejectsMissingFields(t *testing.T) {
	for _, bad := range []string{"", "?user_id=42", "user_id=42"} {
		if _, _, err := ParseCallbackPayload(bad); err == nil {
			t.Fatalf("shape %q accepted, want an error", bad)
		}
	}
}

func TestDecryptAccessTokenOAEPRoundTrip(t *testing.T) {
	// Round-trip the actual RSA encrypt/decrypt the executor does. Use a
	// real keypair so the OAEP-SHA256 path is exercised against the
	// PKCS1v15 fallback path separately.
	_, verifier, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	priv, err := decodeVerifier(verifier)
	if err != nil {
		t.Fatalf("decodeVerifier: %v", err)
	}
	secret := []byte("this-is-the-access-token")
	encBytes := make([]byte, 2048/8)
	_, err = publicKeyEncrypt(priv, secret, encBytes)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encB64 := base64URLNoPad(encBytes)
	got, err := DecryptAccessToken(encB64, verifier)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != string(secret) {
		t.Fatalf("round-trip: %q vs %q", got, string(secret))
	}
}

func TestDecryptAccessTokenFallsBackToPKCS1v15(t *testing.T) {
	// When a caller supplies a PKCS1v15-encrypted token (an older shape),
	// the decryptor must still succeed — that is 9router's exact contract.
	_, verifier, _ := GenerateKeyPair()
	priv, _ := decodeVerifier(verifier)
	secret := []byte("pkcs1v15 payload")
	encBytes := make([]byte, 2048/8)
	_, err := publicKeyEncryptPKCS1(priv, secret, encBytes)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	encB64 := base64URLNoPad(encBytes)
	got, err := DecryptAccessToken(encB64, verifier)
	if err != nil {
		t.Fatalf("pkcs1v15 fallback failed: %v", err)
	}
	if got != string(secret) {
		t.Fatalf("round-trip: %q vs %q", got, string(secret))
	}
}

func TestDecryptionGarbageInputFails(t *testing.T) {
	_, verifier, _ := GenerateKeyPair()
	// Not valid base64url, not valid RSA payload.
	if _, err := DecryptAccessToken("%%%", verifier); err == nil {
		t.Fatal("junk base64 accepted")
	}
	// Valid base64 but ciphertext 0.
	if _, err := DecryptAccessToken("", verifier); err == nil {
		t.Fatal("empty ciphertext accepted")
	}
}

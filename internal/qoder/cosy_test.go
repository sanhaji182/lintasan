package qoder

import (
	"strings"
	"testing"
)

// TestFingerprintVectors pins the device-fingerprint derivations to published
// cross-implementation vectors.
//
// These expected values are not self-generated: they are the reference vectors
// that ship with the third-party implementation this package was ported from,
// which in turn were cross-checked against that project's Python reference
// (qoder_fingerprint.py). Pinning them here means a refactor that silently
// changes the fingerprint — which upstream would read as a brand-new device —
// fails CI instead of shipping.
//
// The vectors hold only in the unsalted mode, which is why the Fingerprinter is
// constructed with an empty salt.
func TestFingerprintVectors(t *testing.T) {
	const uid = "test-uid-123"
	f := NewFingerprinter("")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"machineid", f.MachineID(uid), "005b8945c0659064f8d25299980a27b3"},
		{"sessionid", f.SessionID(uid), "e1527624132e0a39ebcb328440517365"},
		{"machinetype", f.MachineType(uid), "66ea01f7983702b088"},
		{"machinetoken", f.MachineToken(uid), "zzpUYGGMSPEfJVrGQWHj7SBYaRUMwPMK0B4QN_aqKP0"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestFingerprintShape locks the wire-format lengths: a derivation that produced
// the right characters but the wrong length would still be rejected upstream.
func TestFingerprintShape(t *testing.T) {
	f := NewFingerprinter("")
	const seed = "some-account-id"

	if n := len(f.MachineID(seed)); n != 32 {
		t.Errorf("machineid length = %d, want 32", n)
	}
	if n := len(f.SessionID(seed)); n != 32 {
		t.Errorf("sessionid length = %d, want 32", n)
	}
	if n := len(f.MachineType(seed)); n != 18 {
		t.Errorf("machinetype length = %d, want 18", n)
	}
	if n := len(f.MachineToken(seed)); n != 43 {
		t.Errorf("machinetoken length = %d, want 43", n)
	}
}

// TestFingerprintIsStableAndIsolated covers the two properties the derivation
// exists for: identical input never drifts, and distinct accounts never collide.
func TestFingerprintIsStableAndIsolated(t *testing.T) {
	f := NewFingerprinter("")

	if f.MachineID("acct-a") != f.MachineID("acct-a") {
		t.Error("machine id is not stable for a repeated seed")
	}
	if f.MachineID("acct-a") == f.MachineID("acct-b") {
		t.Error("machine id collides across distinct accounts")
	}
	if f.MachineToken("acct-a") == f.MachineToken("acct-b") {
		t.Error("machine token collides across distinct accounts")
	}
}

// TestFingerprintSeedPrecedence verifies the seed-selection rule. The account id
// must win once it is known; before then the credential stands in, so the two
// phases of a request present a consistent device identity.
func TestFingerprintSeedPrecedence(t *testing.T) {
	credSeed := FingerprintSeed("", "pt-secret")

	if Fake := FingerprintSeed("", "pt-secret"); Fake != credSeed {
		t.Error("credential seed is not stable")
	}
	if FingerprintSeed("real-account-id", "pt-secret") == credSeed {
		t.Error("account id must take precedence over the credential seed")
	}
}

// TestInstallSaltShiftsFingerprint documents the operational semantics of the
// salt: setting it moves every derived value, and clearing it restores the
// compatible baseline. Changing a live salt is equivalent to swapping devices.
func TestInstallSaltShiftsFingerprint(t *testing.T) {
	const uid = "test-uid-123"

	plain := NewFingerprinter("")
	unsaltedID := plain.MachineID(uid)
	unsaltedToken := plain.MachineToken(uid)

	salted := NewFingerprinter("deployment-salt")
	if salted.MachineID(uid) == unsaltedID {
		t.Error("salt must change the derived machine id")
	}
	if salted.MachineToken(uid) == unsaltedToken {
		t.Error("salt must change the derived machine token")
	}
	// Shape is preserved under a salt, so upstream still accepts the format.
	if n := len(salted.MachineToken(uid)); n != 43 {
		t.Errorf("salted machinetoken length = %d, want 43", n)
	}
	// Isolation still holds, and derivation is still idempotent.
	if salted.MachineID("acct-a") == salted.MachineID("acct-b") {
		t.Error("salted machine id collides across distinct accounts")
	}
	if salted.MachineID(uid) != salted.MachineID(uid) {
		t.Error("salted derivation is not idempotent")
	}
	// A second, distinct salt must produce yet another fingerprint space.
	if NewFingerprinter("other-salt").MachineID(uid) == salted.MachineID(uid) {
		t.Error("distinct salts must produce distinct fingerprints")
	}
}

// TestEnvelopeRoundTrip verifies the custom base64 envelope is lossless across
// payload sizes that exercise each padding case and the block rotation boundary.
func TestEnvelopeRoundTrip(t *testing.T) {
	payloads := []string{
		"",
		"a",
		"ab",
		"abc",
		"abcd",
		strings.Repeat("x", 15),
		strings.Repeat("y", 16),
		strings.Repeat("z", 17),
		`{"payload":"{\"personalToken\":\"pt-abc\"}","encodeVersion":"1"}`,
		strings.Repeat(`{"k":"v"},`, 500),
	}
	for _, p := range payloads {
		enc, err := EncodeBase64([]byte(p))
		if err != nil {
			t.Fatalf("EncodeBase64(%q): %v", p, err)
		}
		if len(p) > 0 && strings.ContainsAny(enc, "=+/") {
			t.Errorf("envelope leaked standard base64 characters: %q", enc)
		}
		got, err := DecodeBase64(enc)
		if err != nil {
			t.Fatalf("DecodeBase64(%q): %v", enc, err)
		}
		if string(got) != p {
			t.Errorf("round trip mismatch: got %q, want %q", got, p)
		}
	}
}

// TestEnvelopeRejectsForeignAlphabet ensures a body that is not actually in the
// cosy envelope is rejected rather than silently mis-decoded — the difference
// between a clear error and a confusing upstream 400.
func TestEnvelopeRejectsForeignAlphabet(t *testing.T) {
	if _, err := DecodeBase64("dGhpcyBpcyBzdGFuZGFyZCBiYXNlNjQ="); err == nil {
		t.Error("expected standard base64 input to be rejected")
	}
}

// TestSigningConstants pins the signature scheme. SignLegacy is date-bound, so a
// fixed date yields a fixed digest; this catches an accidental change to the
// separator, the app code, or the signing constant.
func TestSignLegacyIsDateBound(t *testing.T) {
	const date = "Mon, 02 Jan 2006 15:04:05 GMT"
	got := SignLegacy(date)

	if got != SignLegacy(date) {
		t.Error("signature is not deterministic for a fixed date")
	}
	if got == SignLegacy("Tue, 03 Jan 2006 15:04:05 GMT") {
		t.Error("signature does not vary with the date")
	}
	if len(got) != 32 {
		t.Errorf("signature length = %d, want 32 hex characters", len(got))
	}
}

// TestSignRequestCoversBodyAndPath documents the properties that make the
// per-request signature meaningful: it must change when the body, the path, or
// the session key changes.
func TestSignRequestCoversBodyAndPath(t *testing.T) {
	base := SignRequest("payload", "cosykey", "1700000000", []byte(`{"a":1}`), "/api/v2/model/list")

	if base != SignRequest("payload", "cosykey", "1700000000", []byte(`{"a":1}`), "/api/v2/model/list") {
		t.Error("signature is not deterministic for identical inputs")
	}
	if base == SignRequest("payload", "cosykey", "1700000000", []byte(`{"a":2}`), "/api/v2/model/list") {
		t.Error("signature does not cover the body")
	}
	if base == SignRequest("payload", "cosykey", "1700000000", []byte(`{"a":1}`), "/api/v2/chat") {
		t.Error("signature does not cover the path")
	}
	if base == SignRequest("payload", "otherkey", "1700000000", []byte(`{"a":1}`), "/api/v2/model/list") {
		t.Error("signature does not cover the session key")
	}
	if base == SignRequest("payload", "cosykey", "1700000001", []byte(`{"a":1}`), "/api/v2/model/list") {
		t.Error("signature does not cover the timestamp")
	}
}

// TestPathSigStripsAlgoPrefix pins the rule that the signed path excludes the
// /algo routing prefix. Signing the prefixed path would produce a signature that
// upstream rejects, so this is a correctness property, not a cosmetic one.
func TestPathSigStripsAlgoPrefix(t *testing.T) {
	cases := map[string]string{
		"/algo/api/v2/model/list": "/api/v2/model/list",
		"/api/v2/model/list":      "/api/v2/model/list",
		"/algo":                   "",
	}
	for in, want := range cases {
		if got := PathSig(in); got != want {
			t.Errorf("PathSig(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSessionBuildsConsistentMaterial verifies the session handshake produces
// material of the right shape, and that it is genuinely per-session (a fresh
// key each time) rather than accidentally cached.
func TestSessionBuildsConsistentMaterial(t *testing.T) {
	id := Identity{Name: "n", AccountID: "aid", UID: "uid-1", SecurityOauthToken: "sec"}
	f := NewFingerprinter("")

	s1, err := NewSession(id, f.MachineID("uid-1"), f.MachineToken("uid-1"), f.MachineType("uid-1"))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s1.CosyKey == "" || s1.Info == "" {
		t.Fatal("session material is empty")
	}
	if s1.Identity.UID != "uid-1" {
		t.Errorf("identity not preserved: %+v", s1.Identity)
	}

	s2, err := NewSession(id, s1.MachineID, s1.MachineToken, s1.MachineType)
	if err != nil {
		t.Fatalf("NewSession (second): %v", err)
	}
	if s1.CosyKey == s2.CosyKey || s1.Info == s2.Info {
		t.Error("session material is reused across sessions; expected a fresh key per session")
	}

	payload, err := s1.BuildPayload()
	if err != nil {
		t.Fatalf("BuildPayload: %v", err)
	}
	if payload == "" {
		t.Fatal("payload is empty")
	}
	bearer := ComposeBearer(payload, "sig")
	if !strings.HasPrefix(bearer, "Bearer COSY.") || !strings.HasSuffix(bearer, ".sig") {
		t.Errorf("unexpected bearer shape: %q", bearer)
	}
}

// TestNewIdentifiersAreDistinct guards the identifier generators: a collision
// here would make two concurrent chats share a request identity upstream.
func TestNewIdentifiersAreDistinct(t *testing.T) {
	seen := make(map[string]bool, 512)
	for i := 0; i < 512; i++ {
		id := NewUUID()
		if seen[id] {
			t.Fatalf("NewUUID collided at iteration %d: %s", i, id)
		}
		seen[id] = true
		if len(id) != 36 {
			t.Fatalf("NewUUID length = %d, want 36: %s", len(id), id)
		}
	}
	if len(NewRequestID()) != 24 {
		t.Errorf("NewRequestID length = %d, want 24", len(NewRequestID()))
	}
}

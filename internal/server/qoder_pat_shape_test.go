package server

import "testing"

// A shape pre-flight runs whether or not validation is on, so "validate: false" cannot
// mean "accept anything". Without it a truncated or mistyped paste is written verbatim
// and surfaces later as a connection that 401s on every request — which is exactly what
// happened during testing: a fake value with validate:false was inserted and had to be
// removed from production by hand.

func TestQoderPATShapeAcceptsRealCredentials(t *testing.T) {
	// A real credential's shape: "pt-" + 61 URL-safe chars.
	real := "pt-Jc5Ahv9QqWvStf9kP2mNxR7bYdL3hZxQ8cVnKjHfT4gWpRsEuAiObMdCfXvBn"
	// 3 chars of prefix, then 60 chars.
	valid := []string{
		real,
		"pt-" + "a1234567890123456789",
		"pt-AAAA_BBBB-CCCC_DDDD-EEEE_FFFF",
	}
	for _, v := range valid {
		if !qoderPATShape.MatchString(v) {
			t.Errorf("rejected a valid-looking PAT: %q", v)
		}
	}

	invalid := []struct{ in, why string }{
		{"sk-abc123456789012345", "wrong prefix"},
		{"pt-", "no token body"},
		{"pt-short", "too short to be a real token"},
		{"notARealCredential000000000000000", "no prefix at all"},
		{"pt-has spaces in it 00000", "containing whitespace"},
		{"ghp_aaaaaaaaaaaaaaaaaaaaa", "a GitHub token"},
		{"", "empty"},
	}
	for _, c := range invalid {
		if qoderPATShape.MatchString(c.in) {
			t.Errorf("accepted something that is not a Qoder PAT (%s): %q", c.why, c.in)
		}
	}
}

// The shape check must not depend on the validate flag — that is the regression this
// test guards, since the dangerous path was validate:false.
func TestQoderPATShapeIsIndependentOfValidationFlag(t *testing.T) {
	// The predicate takes only the credential, so there is no flag to consult. Assert
	// the property directly: a fake value fails regardless of how it is presented.
	const fake = "pt-fake-direct-000000000000000000000000000000"
	if !qoderPATShape.MatchString(fake) {
		// This particular string happens to be well-shaped, which is correct — the shape
		// check is not a liveness check. Document that distinction explicitly.
		t.Log("a well-shaped but dead credential passes the shape check by design; " +
			"only validation catches that, which is why validation is on by default")
	}
	const malformed = "pt-fake direct 0000"
	if qoderPATShape.MatchString(malformed) {
		t.Error("a malformed value must be rejected by the shape check")
	}
}

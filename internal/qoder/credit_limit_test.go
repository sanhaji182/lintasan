package qoder

// credit_limit_test.go — code 112 is a plan/entitlement limit, not an account fault.
//
// The distinction is load-bearing. Measured 2026-09-23: an account at used=300/300 with
// isQuotaExceeded=true still served basic models with HTTP 200 and real content, while
// premium models returned 112. So a credit-limit refusal is scoped to a MODEL, and
// treating it as "this account is dead" would take working accounts out of the pool —
// the same mistake `IsQuotaExceeded` is deliberately not used for on the request path.

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestIsCreditLimitRequiresCodeAndMarker: a bare 112 must not classify, and neither must
// the marker without the code. Misflagging an account is a wrong signal an operator acts
// on, so the classifier demands both.
func TestIsCreditLimitRequiresCodeAndMarker(t *testing.T) {
	cases := []struct {
		name string
		err  *UpstreamError
		want bool
	}{
		{"code + pricingUrl", &UpstreamError{Code: "112", Message: `{"pricingUrl":"https://qoder.com/pricing"}`}, true},
		{"code + credit wording", &UpstreamError{Code: "112", Message: "credit limit reached"}, true},
		{"code alone", &UpstreamError{Code: "112", Message: "nope"}, false},
		{"marker without code", &UpstreamError{Code: "105", Message: `{"pricingUrl":"x"}`}, false},
		{"other code", &UpstreamError{Code: "105", Message: "Login expired"}, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.IsCreditLimit(); got != tc.want {
				t.Errorf("IsCreditLimit() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCreditLimitIsNotLoginExpired keeps the two refusals apart. Folding 112 into the
// 105 family would tell the client to retry a request that cannot succeed, and would
// tell an operator to look for a credential problem that does not exist.
func TestCreditLimitIsNotLoginExpired(t *testing.T) {
	ce := &UpstreamError{Code: CreditLimitCode, Message: `{"pricingUrl":"https://qoder.com/pricing"}`}
	if ce.IsLoginExpired() {
		t.Error("112 must not be reported as an expired login")
	}
	if !ce.IsCreditLimit() {
		t.Error("112 with pricingUrl must be a credit limit")
	}

	le := &UpstreamError{Code: LoginExpiredCode, Message: "Login expired"}
	if le.IsCreditLimit() {
		t.Error("105 must not be reported as a credit limit")
	}
}

// TestDiagnosisSeparatesCreditLimitFromCooldown: the operator-facing message must not say
// "retry", because the request will fail identically forever, and must not say "discard
// the credential", because the credential is fine.
func TestDiagnosisSeparatesCreditLimitFromCooldown(t *testing.T) {
	msg, kind, retryable := DescribeStreamError(&UpstreamError{
		Status: 403, Code: CreditLimitCode,
		Message: `{"pricingUrl":"https://qoder.com/pricing?client=qoder"}`,
	})
	if kind != "credit_limit" {
		t.Fatalf("kind = %q, want credit_limit", kind)
	}
	if retryable {
		t.Error("a credit limit must not be retryable — retrying cannot change the plan")
	}
	if !strings.Contains(msg, "not entitled") {
		t.Errorf("message should name the cause, got %q", msg)
	}
	// It must say the account still works for other models, or an operator reads the
	// refusal as an account-level failure and disables a working account.
	if !strings.Contains(msg, "Other models") {
		t.Errorf("message must scope the refusal to the model, got %q", msg)
	}
}

// TestEnvelopeMessageCarriesPricingMarker: the classifier keys on the raw body, so the
// parser must put the body's fields where the classifier can see them. Without this the
// classification would degrade to matching a bare code, which the test above forbids.
func TestEnvelopeMessageCarriesPricingMarker(t *testing.T) {
	inner := `{"code":"112","message":"insufficient credits","pricingUrl":"https://qoder.com/pricing"}`
	frame, _ := json.Marshal(map[string]any{
		"statusCodeValue": 403,
		"statusCode":      "FORBIDDEN",
		"body":            inner,
	})

	e := parseEnvelopeError(frame)
	if e == nil {
		t.Fatal("expected an error from a 403 envelope with a business code")
	}
	if e.Code != CreditLimitCode {
		t.Errorf("code = %q, want 112", e.Code)
	}
	if !strings.Contains(e.Message, PricingURLMarker) {
		t.Fatalf("Message lost the pricing marker, so the classifier cannot confirm it: %q", e.Message)
	}
	if !e.IsCreditLimit() {
		t.Error("IsCreditLimit must be true for the observed shape")
	}
	// The human text must survive alongside the marker.
	if !strings.Contains(e.Message, "insufficient credits") {
		t.Errorf("Message dropped the human explanation: %q", e.Message)
	}
}

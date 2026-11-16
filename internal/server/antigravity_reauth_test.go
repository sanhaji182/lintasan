package server

// Contract tests for the Antigravity "cannot be used -> must say so and ask for a
// re-login" behaviour.
//
// The failure this pins down: production had an enabled oauth-antigravity
// connection whose sessions were all expired/revoked. Every request failed, but
// the only signal was
//   {"error":{"message":"all routes failed","details":"... has no OAuth access token"}}
// with HTTP 502 — which reads like a transient gateway fault. Nothing told the
// operator that a human sign-in was required, and nothing marked the connection
// as unhealthy.
//
// Required behaviour, and what these tests assert:
//   1. The error names the reauth code and tells the operator to sign in again.
//   2. It is NOT classified as a retryable/transient provider failure (401, not 502).
//   3. The connection row is flagged so the dashboard can surface it.
//   4. The flag clears once the connection serves a request (no stale lies).

import (
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

func TestAntigravityReauthError_SaysSignInAgain(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "client.apps.googleusercontent.com")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "secret")

	err := antigravityReauthError(&Connection{ID: "cd249bde"})
	if err == nil {
		t.Fatal("expected an error for a tokenless antigravity connection")
	}
	msg := err.Error()
	for _, want := range []string{antigravityReauthCode, "sign in again", "cd249bde"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
	if !isAntigravityReauth(err) {
		t.Error("error not recognized as a reauth signal")
	}
}

// When the OAuth client itself is unconfigured, telling the operator to sign in
// would be useless advice — that path must name the missing configuration.
func TestAntigravityReauthError_NamesUnconfiguredClient(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "")

	err := antigravityReauthError(&Connection{ID: "c1"})
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID") {
		t.Errorf("message %q must name the missing client id", msg)
	}
	if !strings.Contains(msg, antigravityReauthCode) {
		t.Errorf("message %q must carry the machine code", msg)
	}
}

// A plain upstream failure must NOT be mistaken for a reauth signal, or the
// operator would be told to sign in for an unrelated outage.
func TestIsAntigravityReauth_RejectsUnrelatedErrors(t *testing.T) {
	if isAntigravityReauth(nil) {
		t.Error("nil must not be a reauth signal")
	}
	if isAntigravityReauth(errString("antigravity: upstream error")) {
		t.Error("generic upstream error misclassified as reauth")
	}
}

func TestFlagAndClearConnectionReauth(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority, oauth_provider)
		 VALUES ('ag-1', 'oauth-antigravity', 'https://example.invalid', '', 'openai', 1, 1, 'antigravity')`,
	); err != nil {
		t.Fatal(err)
	}

	p := NewProxyHandler(&config.Config{}, database)
	p.flagConnectionReauth("ag-1", "gemini-3.1-pro-high", "oauth_reauth_required: expired")

	var code, model string
	if err := database.Conn().QueryRow(
		`SELECT COALESCE(last_error_code,''), COALESCE(last_error_model,'') FROM connections WHERE id='ag-1'`,
	).Scan(&code, &model); err != nil {
		t.Fatal(err)
	}
	if code != antigravityReauthCode {
		t.Errorf("last_error_code = %q, want %q", code, antigravityReauthCode)
	}
	if model != "gemini-3.1-pro-high" {
		t.Errorf("last_error_model = %q", model)
	}

	// Serving a request proves the account works again: the flag must go,
	// otherwise the dashboard keeps asking for a login that already happened.
	p.clearConnectionReauth("ag-1")
	if err := database.Conn().QueryRow(
		`SELECT COALESCE(last_error_code,''), COALESCE(last_error_at,'') FROM connections WHERE id='ag-1'`,
	).Scan(&code, &model); err != nil {
		t.Fatal(err)
	}
	if code != "" {
		t.Errorf("flag not cleared after success: %q", code)
	}
	if model != "" {
		t.Errorf("last_error_at not cleared after success: %q", model)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

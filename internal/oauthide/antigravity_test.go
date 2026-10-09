package oauthide

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestBuildAntigravityAuthorizeURLRejectsMissingClientID(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "")

	got, err := BuildAntigravityAuthorizeURL("https://lintasan.example/callback", "state-123")
	if got != "" {
		t.Fatalf("authorize URL = %q, want empty URL when configuration is missing", got)
	}
	var configErr *AntigravityConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("error = %v, want AntigravityConfigurationError", err)
	}
	if !errors.Is(err, ErrAntigravityClientNotConfigured) {
		t.Fatalf("error = %v, want ErrAntigravityClientNotConfigured", err)
	}
}

func TestBuildAntigravityAuthorizeURLRejectsMissingClientSecret(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "client.apps.googleusercontent.com")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_SECRET", "")

	got, err := BuildAntigravityAuthorizeURL("https://lintasan.example/callback", "state-123")
	if got != "" {
		t.Fatalf("authorize URL = %q, want empty URL when configuration is incomplete", got)
	}
	var configErr *AntigravityConfigurationError
	if !errors.As(err, &configErr) {
		t.Fatalf("error = %v, want AntigravityConfigurationError", err)
	}
	if !strings.Contains(err.Error(), "LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET") {
		t.Fatalf("error = %v, want missing secret environment variable", err)
	}
}

func TestBuildAntigravityAuthorizeURLIncludesGoogleOAuthParameters(t *testing.T) {
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID", "client.apps.googleusercontent.com")
	t.Setenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET", "secret")

	got, err := BuildAntigravityAuthorizeURL("https://lintasan.example/api/oauth/callback/antigravity", "state-123")
	if err != nil {
		t.Fatalf("BuildAntigravityAuthorizeURL() error = %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	q := parsed.Query()
	for key, want := range map[string]string{
		"response_type": "code",
		"client_id":     "client.apps.googleusercontent.com",
		"redirect_uri":  "https://lintasan.example/api/oauth/callback/antigravity",
		"state":         "state-123",
		"access_type":   "offline",
		"prompt":        "consent",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if q.Get("scope") == "" {
		t.Fatal("scope is empty")
	}
}

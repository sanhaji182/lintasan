package oauthide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OpenAI Codex OAuth (9router CODEX_CONFIG).
const (
	CodexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	CodexTokenURL     = "https://auth.openai.com/oauth/token"
	CodexScope        = "openid profile email offline_access"
	CodexPKCEBytes    = 64
)

// BuildCodexAuthorizeURL mirrors 9router codex.buildAuthUrl.
func BuildCodexAuthorizeURL(redirectURI, state, codeChallenge string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", CodexClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", CodexScope)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")
	params.Set("originator", "codex_cli_rs")
	params.Set("state", state)
	return CodexAuthorizeURL + "?" + params.Encode()
}

// ExchangeCodexToken form-encoded PKCE exchange.
func ExchangeCodexToken(code, redirectURI, codeVerifier string) (*TokenJSON, string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", CodexClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest(http.MethodPost, CodexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("codex token HTTP %d: %s", resp.StatusCode, truncateErr(string(b)))
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, "", err
	}
	out := TokenJSON{}
	if v, ok := raw["access_token"].(string); ok {
		out.AccessToken = v
	}
	if v, ok := raw["refresh_token"].(string); ok {
		out.RefreshToken = v
	}
	if v, ok := raw["expires_in"].(float64); ok {
		out.ExpiresIn = int(v)
	}
	if out.AccessToken == "" {
		return nil, "", fmt.Errorf("codex token missing access_token")
	}
	meta, _ := json.Marshal(map[string]any{
		"id_token": raw["id_token"],
	})
	return &out, string(meta), nil
}
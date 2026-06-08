package oauthide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Claude Code OAuth (9router CLAUDE_CONFIG).
const (
	ClaudeClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	ClaudeAuthorizeURL = "https://claude.ai/oauth/authorize"
	ClaudeTokenURL     = "https://api.anthropic.com/v1/oauth/token"
	ClaudeScope        = "org:create_api_key user:profile user:inference"
	ClaudePKCEBytes    = 64
)

// BuildClaudeAuthorizeURL mirrors 9router claude.buildAuthUrl.
func BuildClaudeAuthorizeURL(redirectURI, state, codeChallenge string) string {
	params := url.Values{}
	params.Set("code", "true")
	params.Set("client_id", ClaudeClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", ClaudeScope)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	return ClaudeAuthorizeURL + "?" + params.Encode()
}

// ExchangeClaudeToken JSON body PKCE exchange.
func ExchangeClaudeToken(code, redirectURI, codeVerifier, state string) (*TokenJSON, error) {
	authCode := code
	codeState := ""
	if i := strings.Index(authCode, "#"); i >= 0 {
		codeState = authCode[i+1:]
		authCode = authCode[:i]
	}
	if codeState == "" {
		codeState = state
	}
	body := map[string]string{
		"code":          authCode,
		"state":         codeState,
		"grant_type":    "authorization_code",
		"client_id":     ClaudeClientID,
		"redirect_uri":  redirectURI,
		"code_verifier": codeVerifier,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, ClaudeTokenURL, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("claude token HTTP %d: %s", resp.StatusCode, truncateErr(string(b)))
	}
	var out TokenJSON
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if out.AccessToken == "" {
		return nil, fmt.Errorf("claude token missing access_token")
	}
	return &out, nil
}
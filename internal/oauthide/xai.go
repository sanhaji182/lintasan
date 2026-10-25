package oauthide

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PKCE holds verifier/challenge for authorization_code_pkce flows.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE creates S256 PKCE pair.
func NewPKCE(nbytes int) (*PKCE, error) {
	if nbytes < 32 {
		nbytes = 32
	}
	raw := make([]byte, nbytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	verifier := base64URLEncode(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64URLEncode(sum[:])
	return &PKCE{Verifier: verifier, Challenge: challenge}, nil
}

func base64URLEncode(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// XAI public OAuth client (ported from 9router constants/xai.js).
const (
	XAIClientID     = "b1a00492-073a-47ea-816f-4c329264a828"
	XAIAuthorizeURL = "https://auth.x.ai/oauth2/authorize"
	XAITokenURL     = "https://auth.x.ai/oauth2/token"
	XAIScope        = "openid profile email offline_access grok-cli:access api:access"
	XAIPKCEBytes    = 96
)

// BuildXAIAuthorizeURL mirrors 9router xai.buildAuthUrl.
func BuildXAIAuthorizeURL(redirectURI, state, codeChallenge string) string {
	nonce := randomHex(16)
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", XAIClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", XAIScope)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	params.Set("nonce", nonce)
	params.Set("plan", "generic")
	params.Set("referrer", "cli-proxy-api")
	return XAIAuthorizeURL + "?" + params.Encode()
}

// TokenJSON is a minimal OAuth token response.
type TokenJSON struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

// ExchangeXAIToken PKCE code exchange (public client).
func ExchangeXAIToken(code, redirectURI, codeVerifier string) (*TokenJSON, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", XAIClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest(http.MethodPost, XAITokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xai token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("xai token HTTP %d: %s", resp.StatusCode, truncateErr(string(body)))
	}
	var out TokenJSON
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.AccessToken == "" {
		return nil, fmt.Errorf("xai token missing access_token")
	}
	return &out, nil
}

func truncateErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

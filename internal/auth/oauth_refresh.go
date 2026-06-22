package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

type refreshedTokens struct {
	Access  string
	Refresh string
}

func refreshOAuthProvider(provider, refreshToken string) (*refreshedTokens, time.Time, error) {
	switch provider {
	case "xai":
		return refreshFormToken(
			"https://auth.x.ai/oauth2/token",
			url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
				"client_id":     {oauthide.XAIClientID},
			},
			"",
		)
	case "claude":
		return refreshJSONToken(
			"https://api.anthropic.com/v1/oauth/token",
			map[string]string{
				"grant_type":    "refresh_token",
				"refresh_token": refreshToken,
			},
		)
	case "codex":
		return refreshFormToken(
			oauthide.CodexTokenURL,
			url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
				"client_id":     {oauthide.CodexClientID},
			},
			"",
		)
	case "antigravity":
		cid := strings.TrimSpace(os.Getenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID"))
		secret := strings.TrimSpace(os.Getenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET"))
		if secret == "" {
			secret = strings.TrimSpace(os.Getenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_SECRET"))
		}
		if cid == "" || secret == "" {
			return nil, time.Time{}, fmt.Errorf("antigravity refresh: client id/secret not configured")
		}
		return refreshFormToken(
			oauthide.AntigravityTokenURL,
			url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
				"client_id":     {cid},
				"client_secret": {secret},
			},
			"",
		)
	default:
		return nil, time.Time{}, fmt.Errorf("refresh not implemented for provider %s", provider)
	}
}

func refreshFormToken(tokenURL string, form url.Values, basicAuth string) (*refreshedTokens, time.Time, error) {
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basicAuth != "" {
		req.Header.Set("Authorization", basicAuth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, time.Time{}, fmt.Errorf("refresh HTTP %d: %s", resp.StatusCode, truncateOAuthErr(string(body)))
	}
	return parseRefreshBody(body)
}

func refreshJSONToken(tokenURL string, payload map[string]string) (*refreshedTokens, time.Time, error) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(string(b)))
	if err != nil {
		return nil, time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, time.Time{}, fmt.Errorf("refresh HTTP %d: %s", resp.StatusCode, truncateOAuthErr(string(body)))
	}
	return parseRefreshBody(body)
}

func parseRefreshBody(body []byte) (*refreshedTokens, time.Time, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, time.Time{}, err
	}
	access, _ := raw["access_token"].(string)
	refresh, _ := raw["refresh_token"].(string)
	if access == "" {
		return nil, time.Time{}, fmt.Errorf("refresh response missing access_token")
	}
	expires := time.Now().Add(24 * time.Hour)
	if v, ok := raw["expires_in"].(float64); ok && v > 0 {
		expires = time.Now().Add(time.Duration(int(v)) * time.Second)
	}
	return &refreshedTokens{Access: access, Refresh: refresh}, expires, nil
}

func truncateOAuthErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
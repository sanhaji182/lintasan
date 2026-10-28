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
	Access   string
	Refresh  string
	FlowMeta string
}

// oauthRefreshHTTPClient bounds provider refresh calls; http.DefaultClient has
// no timeout and can pin a proxy request indefinitely on a stalled provider.
var oauthRefreshHTTPClient = &http.Client{Timeout: 20 * time.Second}

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
	case "nous":
		// Nous Research portal device-key refresh. NOUS_CONFIG (oauthide.
		// DefaultNousConfig) carries the portal's OAuth token endpoint; the
		// manufacturer-of-record is portal.nousresearch.com, so the refreshed
		// access token rides the same host and scope as the original grant.
		// See internal/oauthide/nous.go for the full mint doc-comment.
		//
		// CRITICAL CORRECTION (2026-10-19): 9router's refreshNousToken
		// (open-sse/services/tokenRefresh/providers.js:772) DOES re-mint the
		// agent key on every refresh — the flow is documented verbatim as
		// "refresh_token → new access_token → mint fresh agent_key". The
		// earlier implementation assumed the agent key carries through, which
		// is NOT the case: the agent key has its own TTL in the flow_meta
		// (per NOUS_CONFIG mint on the initial device grant), so a dead-key
		// session needs a fresh mint on refresh — the access token alone
		// would 403 on the inference host. Redo to match 9router's shape.
		//
		// On a mint failure we surface the ERROR rather than silently
		// returning the bare access_token: the 9router fallback is
		// `apiKey` || access_token because ITS caller already knows a
		// Hermes agent key may still be cached per account. Lintasan's
		// caller does NOT have that fallback (a fresh session has no prior
		// key to fall back to), so pushing the bare access_token would 403
		// the inference host and look like an auth failure when it isn't.
		cfg := oauthide.DefaultNousConfig()
		tok, exp, err := refreshFormToken(
			cfg.TokenURL,
			url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {refreshToken},
				"client_id":     {cfg.ClientID},
			},
			"",
		)
		if err != nil {
			return nil, time.Time{}, err
		}
		apiKey, keyID, expiresAt, mintErr := oauthide.MintNousAgentKey(cfg, tok.Access)
		if mintErr != nil {
			return nil, time.Time{}, fmt.Errorf("nous agent-key re-mint: %w", mintErr)
		}
		meta := map[string]any{
			"agent_key":            apiKey,
			"agent_key_id":         keyID,
			"agent_key_expires_at": expiresAt,
		}
		metaJSON, _ := json.Marshal(meta)
		tok.FlowMeta = string(metaJSON)
		return tok, exp, nil
	case "cline":
		return refreshClineToken(refreshToken)
	default:
		return nil, time.Time{}, fmt.Errorf("refresh not implemented for provider %s", provider)
	}
}

func refreshClineToken(refreshToken string) (*refreshedTokens, time.Time, error) {
	payload := map[string]string{
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
		"clientType":   "extension",
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, "https://api.cline.bot/api/v1/auth/refresh", strings.NewReader(string(b)))
	if err != nil {
		return nil, time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthRefreshHTTPClient.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, time.Time{}, fmt.Errorf("cline refresh HTTP %d: %s", resp.StatusCode, truncateOAuthErr(string(body)))
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, time.Time{}, err
	}
	dataObj := raw
	if d, ok := raw["data"].(map[string]any); ok && d != nil {
		dataObj = d
	}
	access, _ := dataObj["accessToken"].(string)
	if access == "" {
		access, _ = dataObj["access_token"].(string)
	}
	if access == "" {
		return nil, time.Time{}, fmt.Errorf("cline refresh response missing accessToken")
	}
	refresh, _ := dataObj["refreshToken"].(string)
	if refresh == "" {
		refresh, _ = dataObj["refresh_token"].(string)
	}
	if refresh == "" {
		refresh = refreshToken
	}
	expires := time.Now().Add(24 * time.Hour)
	if expStr, ok := dataObj["expiresAt"].(string); ok && expStr != "" {
		if t, err := time.Parse(time.RFC3339, expStr); err == nil {
			expires = t
		}
	} else if v, ok := dataObj["expiresIn"].(float64); ok && v > 0 {
		expires = time.Now().Add(time.Duration(int(v)) * time.Second)
	}
	return &refreshedTokens{Access: access, Refresh: refresh}, expires, nil
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
	resp, err := oauthRefreshHTTPClient.Do(req)
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
	resp, err := oauthRefreshHTTPClient.Do(req)
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

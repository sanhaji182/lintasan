package oauthide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Antigravity Google OAuth (9router ANTIGRAVITY_CONFIG shape).
// Client credentials are NOT shipped in source — set env from your own Google OAuth app or lab copy of 9router constants.
const (
	AntigravityAuthorizeURL       = "https://accounts.google.com/o/oauth2/v2/auth"
	AntigravityTokenURL           = "https://oauth2.googleapis.com/token"
	AntigravityUserInfoURL        = "https://www.googleapis.com/oauth2/v1/userinfo"
	AntigravityLoadCodeAssistURL  = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	AntigravityOnboardUserURL     = "https://cloudcode-pa.googleapis.com/v1internal:onboardUser"
	AntigravityLoadUserAgent      = "google-api-nodejs-client/9.15.1"
	AntigravityLoadAPIClient      = "google-cloud-sdk vscode_cloudshelleditor/0.1"
	AntigravityLoadClientMetadata = `{"ideType":9,"platform":3,"pluginType":2}`
)

func antigravityClientID() string {
	return strings.TrimSpace(os.Getenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID"))
}

func antigravityClientSecret() string {
	if v := os.Getenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET"); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("LINTASAN_OAUTH_IDE_ANTIGRAVITY_SECRET"))
}

// BuildAntigravityAuthorizeURL standard Google OAuth2 (no PKCE).
func BuildAntigravityAuthorizeURL(redirectURI, state string) string {
	cid := antigravityClientID()
	if cid == "" {
		return AntigravityAuthorizeURL + "?error=antigravity_client_id_not_configured"
	}
	scopes := strings.Join([]string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/cclog",
		"https://www.googleapis.com/auth/experimentsandconfigs",
	}, " ")
	params := url.Values{}
	params.Set("client_id", antigravityClientID())
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", scopes)
	params.Set("state", state)
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	return AntigravityAuthorizeURL + "?" + params.Encode()
}

// ExchangeAntigravityToken form exchange + postExchange metadata JSON.
func ExchangeAntigravityToken(code, redirectURI string) (*TokenJSON, string, error) {
	cid := antigravityClientID()
	secret := antigravityClientSecret()
	if cid == "" || secret == "" {
		return nil, "", fmt.Errorf("antigravity requires LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID and LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cid)
	form.Set("client_secret", secret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest(http.MethodPost, AntigravityTokenURL, strings.NewReader(form.Encode()))
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
		return nil, "", fmt.Errorf("antigravity token HTTP %d: %s", resp.StatusCode, truncateErr(string(b)))
	}
	var out TokenJSON
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, "", err
	}
	if out.AccessToken == "" {
		return nil, "", fmt.Errorf("antigravity token missing access_token")
	}
	meta, _ := antigravityPostExchange(out.AccessToken)
	return &out, meta, nil
}

func antigravityPostExchange(accessToken string) (string, error) {
	loadHeaders := map[string]string{
		"Authorization":     "Bearer " + accessToken,
		"Content-Type":      "application/json",
		"User-Agent":        AntigravityLoadUserAgent,
		"X-Goog-Api-Client": AntigravityLoadAPIClient,
		"Client-Metadata":   AntigravityLoadClientMetadata,
		"x-request-source":  "local",
	}
	var userInfo map[string]any
	uiReq, _ := http.NewRequest(http.MethodGet, AntigravityUserInfoURL+"?alt=json", nil)
	uiReq.Header.Set("Authorization", "Bearer "+accessToken)
	uiReq.Header.Set("x-request-source", "local")
	if uiResp, err := httpClient.Do(uiReq); err == nil {
		defer uiResp.Body.Close()
		if uiResp.StatusCode >= 200 && uiResp.StatusCode < 300 {
			_ = json.NewDecoder(io.LimitReader(uiResp.Body, 1<<20)).Decode(&userInfo)
		}
	}
	metadata := map[string]int{"ideType": 9, "platform": 3, "pluginType": 2}
	projectID := ""
	tierID := "legacy-tier"
	body, _ := json.Marshal(map[string]any{"metadata": metadata})
	lreq, _ := http.NewRequest(http.MethodPost, AntigravityLoadCodeAssistURL, strings.NewReader(string(body)))
	for k, v := range loadHeaders {
		lreq.Header.Set(k, v)
	}
	if lresp, err := httpClient.Do(lreq); err == nil {
		defer lresp.Body.Close()
		if lresp.StatusCode >= 200 && lresp.StatusCode < 300 {
			var data map[string]any
			if json.NewDecoder(io.LimitReader(lresp.Body, 1<<20)).Decode(&data) == nil {
				if cap, ok := data["cloudaicompanionProject"].(map[string]any); ok {
					if id, ok := cap["id"].(string); ok {
						projectID = id
					}
				} else if id, ok := data["cloudaicompanionProject"].(string); ok {
					projectID = id
				}
				if tiers, ok := data["allowedTiers"].([]any); ok {
					for _, t := range tiers {
						tm, _ := t.(map[string]any)
						if tm != nil && tm["isDefault"] == true {
							if id, ok := tm["id"].(string); ok && id != "" {
								tierID = strings.TrimSpace(id)
								break
							}
						}
					}
				}
			}
		}
	}
	if projectID != "" {
		go antigravityOnboardAsync(accessToken, tierID, loadHeaders, metadata)
	}
	flowMeta, err := json.Marshal(map[string]any{
		"userInfo":  userInfo,
		"projectId": projectID,
		"tierId":    tierID,
	})
	return string(flowMeta), err
}

func antigravityOnboardAsync(accessToken, tierID string, loadHeaders map[string]string, metadata map[string]int) {
	body, _ := json.Marshal(map[string]any{"tierId": tierID, "metadata": metadata})
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodPost, AntigravityOnboardUserURL, strings.NewReader(string(body)))
		for k, v := range loadHeaders {
			req.Header.Set(k, v)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return
		}
		var result map[string]any
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && result["done"] == true {
			return
		}
		time.Sleep(5 * time.Second)
	}
}
package oauthide

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ClineAuthorizeURL     = "https://api.cline.bot/api/v1/auth/authorize"
	ClineTokenExchangeURL = "https://api.cline.bot/api/v1/auth/token"
)

func BuildClineAuthorizeURL(redirectURI string) string {
	params := url.Values{}
	params.Set("client_type", "extension")
	params.Set("callback_url", redirectURI)
	params.Set("redirect_uri", redirectURI)
	return ClineAuthorizeURL + "?" + params.Encode()
}

type clineTokenBundle struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	FlowMeta     string
}

func ExchangeClineToken(code, redirectURI string) (*TokenJSON, string, error) {
	bundle, err := clineDecodeOrPost(code, redirectURI)
	if err != nil {
		return nil, "", err
	}
	return &TokenJSON{
		AccessToken:  bundle.AccessToken,
		RefreshToken: bundle.RefreshToken,
		ExpiresIn:    bundle.ExpiresIn,
	}, bundle.FlowMeta, nil
}

func clineDecodeOrPost(code, redirectURI string) (*clineTokenBundle, error) {
	if tok, meta, ok := clineTryBase64Code(code); ok {
		return &clineTokenBundle{
			AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
			ExpiresIn: tok.ExpiresIn, FlowMeta: meta,
		}, nil
	}
	body := map[string]string{
		"grant_type": "authorization_code", "code": code,
		"client_type": "extension", "redirect_uri": redirectURI,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, ClineTokenExchangeURL, strings.NewReader(string(raw)))
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
		return nil, fmt.Errorf("cline token HTTP %d: %s", resp.StatusCode, truncateErr(string(b)))
	}
	var wrap struct {
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    string `json:"expiresAt"`
			UserInfo     struct {
				Email string `json:"email"`
			} `json:"userInfo"`
		} `json:"data"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    string `json:"expiresAt"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		return nil, err
	}
	access := wrap.Data.AccessToken
	refresh := wrap.Data.RefreshToken
	expAt := wrap.Data.ExpiresAt
	email := wrap.Data.UserInfo.Email
	if access == "" {
		access = wrap.AccessToken
		refresh = wrap.RefreshToken
		expAt = wrap.ExpiresAt
	}
	if access == "" {
		return nil, fmt.Errorf("cline token missing accessToken")
	}
	expIn := 3600
	if expAt != "" {
		if t, err := time.Parse(time.RFC3339, expAt); err == nil {
			expIn = int(time.Until(t).Seconds())
			if expIn < 60 {
				expIn = 3600
			}
		}
	}
	meta, _ := json.Marshal(map[string]string{"email": email})
	return &clineTokenBundle{AccessToken: access, RefreshToken: refresh, ExpiresIn: expIn, FlowMeta: string(meta)}, nil
}

func clineTryBase64Code(code string) (*TokenJSON, string, bool) {
	b64 := code
	if pad := 4 - (len(b64) % 4); pad != 4 {
		b64 += strings.Repeat("=", pad)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(b64)
		if err != nil {
			return nil, "", false
		}
	}
	s := string(decoded)
	last := strings.LastIndex(s, "}")
	if last < 0 {
		return nil, "", false
	}
	var tokenData struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Email        string `json:"email"`
		FirstName    string `json:"firstName"`
		LastName     string `json:"lastName"`
		ExpiresAt    string `json:"expiresAt"`
	}
	if json.Unmarshal([]byte(s[:last+1]), &tokenData) != nil || tokenData.AccessToken == "" {
		return nil, "", false
	}
	expIn := 3600
	if tokenData.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, tokenData.ExpiresAt); err == nil {
			expIn = int(time.Until(t).Seconds())
		}
	}
	meta, _ := json.Marshal(map[string]any{
		"email": tokenData.Email, "firstName": tokenData.FirstName, "lastName": tokenData.LastName,
	})
	return &TokenJSON{
		AccessToken: tokenData.AccessToken, RefreshToken: tokenData.RefreshToken, ExpiresIn: expIn,
	}, string(meta), true
}
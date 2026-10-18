package oauthide

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// nous.go — C2 of the 9router parity plan (docs/plans/2026-10-18-9router-parity.md).
//
// Ports 9router src/lib/oauth/providers/nous.js + NOUS_CONFIG
// (src/lib/oauth/constants/oauth.js:228) verbatim, including the two-step
// mint the 9router flow performs after the device grant returns:
//
//	Step 1 — standard RFC 8628 device grant
//	  POST /api/oauth/device/code  (client_id: hermes-cli, scope: inference:…)
//	  … user opens verification_uri_complete in a browser …
//	  POST /api/oauth/token       (grant_type: device_code)
//
//	Step 2 — Hermes-CLI agent-key mint (the token alone is half a credential)
//	  POST /api/oauth/agent-key   (Bearer access_token)
//
// The device endpoints live on portal.nousresearch.com (NOT api.nous… or
// inference-api…) so the polling host and the inference host must not be
// conflated: the emitted access key never calls the portal, and the refresh
// token never calls inference.

// NOUS_CONFIG mirrors src/lib/oauth/constants/oauth.js NOUS_CONFIG.
type NousConfig struct {
	ClientID         string // "hermes-cli"
	PortalBaseURL    string // https://portal.nousresearch.com
	DeviceCodeURL    string
	TokenURL         string
	AgentKeyURL      string
	InferenceBaseURL string // https://inference-api.nousresearch.com/v1
	Scope            string // "inference:invoke inference:mint_agent_key"
}

// DefaultNousConfig is the pinned 9router client. Hermes ships the same
// client_id, so a session minted here works with the user's Hermes CLI.
func DefaultNousConfig() NousConfig {
	return NousConfig{
		ClientID:         "hermes-cli",
		PortalBaseURL:    "https://portal.nousresearch.com",
		DeviceCodeURL:    "https://portal.nousresearch.com/api/oauth/device/code",
		TokenURL:         "https://portal.nousresearch.com/api/oauth/token",
		AgentKeyURL:      "https://portal.nousresearch.com/api/oauth/agent-key",
		InferenceBaseURL: "https://inference-api.nousresearch.com/v1",
		Scope:            "inference:invoke inference:mint_agent_key",
	}
}

// NousDeviceCode is the response of POST /api/oauth/device/code.
type NousDeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// RequestNousDeviceCode performs POST /api/oauth/device/code.
//
// Returns (*DeviceCode, nil) on success; error otherwise. HTTP error is not
// masked: the body (usually a plain-text message) is carried through so the
// dashboard can display it rather than an opaque failure.
func RequestNousDeviceCode(cfg NousConfig) (*NousDeviceCode, error) {
	form := url.Values{"client_id": {cfg.ClientID}, "scope": {cfg.Scope}}
	resp, err := http.PostForm(cfg.DeviceCodeURL, form)
	if err != nil {
		return nil, fmt.Errorf("nous device code request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("nous device code HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var dc NousDeviceCode
	if err := json.Unmarshal(body, &dc); err != nil {
		return nil, fmt.Errorf("nous device code decode: %w", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return nil, fmt.Errorf("nous device code: response missing device_code/user_code")
	}
	// 9router uses interval 5 (device-code spec minimum), so a zero must be
	// filled in rather than trusted — a hostile/buggy server otherwise spins
	// the poll loop at its own rate.
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	// Standard device grant expiry is 600s; a server that omits it must not
	// make the poller wait forever.
	if dc.ExpiresIn <= 0 {
		dc.ExpiresIn = 600
	}
	return &dc, nil
}

// NousPollDecision mirrors what 9router's pollToken returns: ok / pending /
// slow-down / hard error. The caller drives its own retry loop from this.
type NousPollDecision struct {
	OK            bool
	Pending       bool   // authorization_pending → keep polling
	SlowDown      bool   // slow_down → back off (caller raises its interval)
	HardError     bool   // access_denied / expired_token → stop
	Error         string // upstream error code, "" when ok
	Access        string
	Refresh       string
	ExpiresIn     int
	AgentKeySet   bool // step 2 result (agent_key mint succeeded)
	AgentKeyError string
}

// PollNousToken performs one POST /api/oauth/token round. Call at
// DeviceCode.Interval until OK / HardError. No retry inside — mirror of
// 9router's pollToken, which returns `pending` rather than looping itself.
func PollNousToken(cfg NousConfig, deviceCode string) NousPollDecision {
	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {cfg.ClientID},
		"device_code": {deviceCode},
	}
	resp, err := http.PostForm(cfg.TokenURL, form)
	if err != nil {
		return NousPollDecision{HardError: true, Error: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	// Body may be non-JSON on some upstream errors; degrade to a synthetic
	// invalid_response object rather than failing the invite outright.
	var raw map[string]any
	if jerr := json.Unmarshal(body, &raw); jerr != nil {
		raw = map[string]any{"error": "invalid_response", "error_description": strings.TrimSpace(string(body))}
	}
	errCode, _ := raw["error"].(string)

	switch errCode {
	case "authorization_pending":
		return NousPollDecision{Pending: true, Error: errCode}
	case "slow_down":
		return NousPollDecision{SlowDown: true, Error: errCode}
	case "access_denied", "expired_token":
		return NousPollDecision{HardError: true, Error: errCode}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		desc, _ := raw["error_description"].(string)
		return NousPollDecision{HardError: true, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, desc)}
	}

	dec := NousPollDecision{
		OK: true,
	}
	dec.Access, _ = raw["access_token"].(string)
	dec.Refresh, _ = raw["refresh_token"].(string)
	if v, ok := raw["expires_in"].(float64); ok && v > 0 {
		dec.ExpiresIn = int(v)
	}
	// An OK response with no access token is a protocol violation, not a
	// success: mark hard error so the caller does not store an empty token.
	if strings.TrimSpace(dec.Access) == "" {
		return NousPollDecision{HardError: true, Error: "token endpoint returned no access_token"}
	}
	return dec
}

// MintNousAgentKey performs POST /api/oauth/agent-key with the minted access
// token and returns the long-lived agent key. This is the Hermes-compatible
// credential: storage per 9router is `sessionKey` + the access token, the
// agent key being what actually authenticates inference calls.
func MintNousAgentKey(cfg NousConfig, accessToken string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, cfg.AgentKeyURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("nous agent-key request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("nous agent-key HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("nous agent-key decode: %w", err)
	}
	// 9router reads `key`; tolerate `agent_key` too because the field name is
	// not formally documented in a public spec.
	for _, field := range []string{"key", "agent_key"} {
		if s, ok := raw[field].(string); ok && strings.TrimSpace(s) != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("nous agent-key: response missing key field")
}

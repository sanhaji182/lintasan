package qoder

// activity.go — Cosy Activity Claim & Quota Engine ported from qoder-suite.
//
// Features:
// 1. Exchange PAT (pt-*) to short-lived Job Token (jt-*) via /api/v1/jobToken/exchange.
// 2. Dynamic MD5 signature generation (secret = "cosy&war, war never changes" & GMT Date).
// 3. Activity eligibility check via /api/v2/activity/claim/eligibility.
// 4. Activity claim execution via POST /api/v2/activity/claim?activityId={id}.
// 5. Auto-claim pipeline for single or pooled accounts.

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// CosySecret is the shared secret for Cosy dynamic request signing.
	CosySecret = "cosy&war, war never changes"
	// CosyAppCode identifies the Cosy IDE client.
	CosyAppCode = "cosy"
	// CosyVersion is the client protocol version.
	CosyVersion = "1.1.5"
	// CosyClientType is the numeric client type header.
	CosyClientType = "5"
	// CosyDefaultMachineOS is the default client OS descriptor.
	CosyDefaultMachineOS = "arm64_darwin"
	// DefaultMachineID is the fallback machine identifier.
	DefaultMachineID = "default-mid-001"
)

// JobTokenResponse is the result of exchanging a PAT for a Job Token.
type JobTokenResponse struct {
	Token                 string `json:"token"`
	RefreshToken          string `json:"refresh_token,omitempty"`
	ExpiresAt             int64  `json:"expires_at,omitempty"`
	RefreshTokenExpiresAt int64  `json:"refresh_token_expires_at,omitempty"`
}

// ActivityItem is one claimable activity / promotion entry.
type ActivityItem struct {
	ActivityID string          `json:"activity_id"`
	ID         string          `json:"id,omitempty"`
	Status     any             `json:"status"` // string, number, or boolean
	Name       string          `json:"name,omitempty"`
	Title      string          `json:"title,omitempty"`
	Credits    int             `json:"credits,omitempty"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}

// ActivityEligibilityResult is the payload from /api/v2/activity/claim/eligibility.
type ActivityEligibilityResult struct {
	Success    bool            `json:"success"`
	Code       int             `json:"code"`
	Message    string          `json:"message,omitempty"`
	Activities []ActivityItem  `json:"activities"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}

// ActivityClaimResult is the response from /api/v2/activity/claim.
type ActivityClaimResult struct {
	ActivityID string          `json:"activity_id"`
	Success    bool            `json:"success"`
	Code       int             `json:"code"`
	Message    string          `json:"message,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	Raw        string          `json:"raw,omitempty"`
}

// AutoClaimSummary is the overall result of running auto-claim for a credential.
type AutoClaimSummary struct {
	Success     bool                       `json:"success"`
	JobToken    string                     `json:"job_token,omitempty"`
	Eligibility *ActivityEligibilityResult `json:"eligibility,omitempty"`
	Claims      []ActivityClaimResult      `json:"claims"`
	Error       string                     `json:"error,omitempty"`
}

// DeviceFingerprint holds isolated hardware & client descriptors derived for an account.
type DeviceFingerprint struct {
	MachineID    string
	MachineToken string
	MachineType  string
	MachineOS    string
}

// DeriveDeviceFingerprint deterministically derives a stable, unique 1-account-to-1-device
// fingerprint matching official Cosy client expectations.
func (f *Fingerprinter) DeriveDeviceFingerprint(seed string) DeviceFingerprint {
	osList := []string{"arm64_darwin", "x86_64_linux", "windows_x86_64"}
	h := md5.Sum([]byte("os:" + f.saltedSeed(seed)))
	idx := int(h[0]) % len(osList)

	return DeviceFingerprint{
		MachineID:    f.MachineID(seed),
		MachineToken: f.MachineToken(seed),
		MachineType:  f.MachineType(seed),
		MachineOS:    osList[idx],
	}
}

// GenerateCosySignature generates the RFC1123 GMT date string and MD5 signature hex.
// Matches:
//
//	now_gmt = datetime.now(timezone.utc).strftime('%a, %d %b %Y %H:%M:%S GMT')
//	raw_str = f"{secret}&{now_gmt}"
//	sig = hashlib.md5(raw_str.encode("utf-8")).hexdigest()
func GenerateCosySignature(secret string, t time.Time) (dateStr string, sigHex string) {
	dateStr = t.UTC().Format(http.TimeFormat)
	raw := fmt.Sprintf("%s&%s", secret, dateStr)
	h := md5.Sum([]byte(raw))
	return dateStr, hex.EncodeToString(h[:])
}

// SetCosyHeaders sets the required Cosy authentication, device, and signature headers.
func SetCosyHeaders(req *http.Request, token, machineID string, now time.Time) {
	SetCosyHeadersWithDevice(req, token, DeviceFingerprint{
		MachineID: machineID,
	}, now)
}

// SetCosyHeadersWithDevice sets full Cosy authentication and isolated device headers.
func SetCosyHeadersWithDevice(req *http.Request, token string, dev DeviceFingerprint, now time.Time) {
	if dev.MachineID == "" {
		dev.MachineID = DefaultMachineID
	}
	if dev.MachineOS == "" {
		dev.MachineOS = CosyDefaultMachineOS
	}
	dateStr, sig := GenerateCosySignature(CosySecret, now)

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Cosy-Version", CosyVersion)
	req.Header.Set("Cosy-ClientType", CosyClientType)
	req.Header.Set("Cosy-MachineOS", dev.MachineOS)
	req.Header.Set("Cosy-MachineId", dev.MachineID)
	req.Header.Set("Cosy-MachineToken", dev.MachineToken)
	req.Header.Set("Cosy-MachineType", dev.MachineType)
	req.Header.Set("Cosy-MachineCode", "")
	req.Header.Set("appcode", CosyAppCode)
	req.Header.Set("login-version", "v2")
	req.Header.Set("date", dateStr)
	req.Header.Set("signature", sig)
	req.Header.Set("User-Agent", "Go-http-client/2.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}

// ExchangePATToJobToken exchanges a personal token (pt-*) for a Job Token (jt-*) via openapi.qoder.sh.
func (m *SessionManager) ExchangePATToJobToken(ctx context.Context, pat string) (*JobTokenResponse, error) {
	endpoint := CheckinHostGlobal + "/api/v1/jobToken/exchange"
	payload, err := json.Marshal(map[string]string{"personal_token": pat})
	if err != nil {
		return nil, fmt.Errorf("qoder: marshal exchange payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("qoder: build jobToken request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "qodercli/1.0.0")
	req.Header.Set("Cosy-Version", CosyVersion)
	req.Header.Set("Cosy-ClientType", CosyClientType)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: exchange job token failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("qoder: read job token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: resp.StatusCode, Message: strings.TrimSpace(string(bodyBytes))}
	}

	var res JobTokenResponse
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return nil, fmt.Errorf("qoder: parse job token response: %w", err)
	}
	return &res, nil
}

// resolveEffectiveToken returns a bearer token suitable for Cosy API calls.
// If the input starts with "pt-", it attempts to exchange it for a Job Token.
// Otherwise, it returns the input token directly.
func (m *SessionManager) resolveEffectiveToken(ctx context.Context, credential string) (string, error) {
	trimmed := strings.TrimSpace(credential)
	if strings.HasPrefix(trimmed, "pt-") {
		jt, err := m.ExchangePATToJobToken(ctx, trimmed)
		if err == nil && jt.Token != "" {
			return jt.Token, nil
		}
		// If exchange fails, fallback to session security token if possible
		sess, serr := m.session(ctx, trimmed)
		if serr == nil && strings.TrimSpace(sess.Identity.SecurityOauthToken) != "" {
			return sess.Identity.SecurityOauthToken, nil
		}
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("qoder: empty job token received")
	}
	return trimmed, nil
}

// ActivityProxyURL returns the configured proxy URL for Cosy activity operations.
// Defaults to the local rotating proxy gateway (proxyuser:proxypass123@127.0.0.1:9999).
// Can be customized via LINTASAN_QODER_PROXY_URL.
func ActivityProxyURL() string {
	if custom := strings.TrimSpace(os.Getenv("LINTASAN_QODER_PROXY_URL")); custom != "" {
		return custom
	}
	return "http://proxyuser:proxypass123@127.0.0.1:9999"
}

// newActivityHTTPClient builds a strict proxy-routed HTTP client.
// In accordance with anti-bot policy, direct connection without proxy is rejected (fail-closed).
// An optional custom transport (e.g. for testing) can be passed.
func newActivityHTTPClient(customTransport http.RoundTripper) (*http.Client, error) {
	if customTransport != nil {
		return &http.Client{
			Transport: customTransport,
			Timeout:   35 * time.Second,
		}, nil
	}

	proxyStr := ActivityProxyURL()
	parsedURL, err := url.Parse(proxyStr)
	if err != nil {
		return nil, fmt.Errorf("qoder activity proxy: invalid url %q: %w", proxyStr, err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(parsedURL),
		// Prevent lingering idle conns from tying down rotating proxy tunnels
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   35 * time.Second,
	}, nil
}

// CheckActivityEligibility checks available activity claim promotions for a token or PAT.
// Uses unique device fingerprint derived from the credential seed and routes via strict proxy.
func (m *SessionManager) CheckActivityEligibility(ctx context.Context, tokenOrPAT string) (*ActivityEligibilityResult, error) {
	seed := FingerprintSeed("", tokenOrPAT)
	dev := m.fp.DeriveDeviceFingerprint(seed)
	return m.CheckActivityEligibilityWithDevice(ctx, tokenOrPAT, dev)
}

// CheckActivityEligibilityWithDevice checks eligibility with explicit device fingerprint and proxy routing.
func (m *SessionManager) CheckActivityEligibilityWithDevice(ctx context.Context, tokenOrPAT string, dev DeviceFingerprint) (*ActivityEligibilityResult, error) {
	var customTransport http.RoundTripper
	if m.client != nil && m.client.Transport != nil {
		customTransport = m.client.Transport
	}
	client, err := newActivityHTTPClient(customTransport)
	if err != nil {
		return nil, err
	}

	effToken, err := m.resolveEffectiveToken(ctx, tokenOrPAT)
	if err != nil {
		return nil, err
	}

	endpoint := CheckinHostGlobal + "/api/v2/activity/claim/eligibility"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("qoder: build eligibility request: %w", err)
	}

	SetCosyHeadersWithDevice(req, effToken, dev, time.Now())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: eligibility request failed (proxy): %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("qoder: read eligibility response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: resp.StatusCode, Message: strings.TrimSpace(string(bodyBytes))}
	}

	var rawMap map[string]any
	if err := json.Unmarshal(bodyBytes, &rawMap); err != nil {
		return nil, fmt.Errorf("qoder: parse eligibility json: %w", err)
	}

	result := &ActivityEligibilityResult{
		Success: true,
		Raw:     bodyBytes,
	}

	if code, ok := rawMap["code"].(float64); ok {
		result.Code = int(code)
	}
	if msg, ok := rawMap["message"].(string); ok {
		result.Message = msg
	}

	// Parse activities from data.activities or activities
	var actSlice []any
	if dataObj, ok := rawMap["data"].(map[string]any); ok {
		if acts, ok := dataObj["activities"].([]any); ok {
			actSlice = acts
		}
	} else if acts, ok := rawMap["activities"].([]any); ok {
		actSlice = acts
	}

	for _, item := range actSlice {
		if itemMap, ok := item.(map[string]any); ok {
			var act ActivityItem
			if id, ok := itemMap["activityId"].(string); ok {
				act.ActivityID = id
			} else if id, ok := itemMap["id"].(string); ok {
				act.ActivityID = id
			}
			act.Status = itemMap["status"]
			if name, ok := itemMap["name"].(string); ok {
				act.Name = name
			}
			if title, ok := itemMap["title"].(string); ok {
				act.Title = title
			}
			if credits, ok := itemMap["credits"].(float64); ok {
				act.Credits = int(credits)
			}
			actBytes, _ := json.Marshal(itemMap)
			act.Raw = actBytes
			result.Activities = append(result.Activities, act)
		}
	}

	return result, nil
}

// ClaimActivity submits a claim for a specific activity ID.
// Uses unique device fingerprint derived from the credential seed and routes via strict proxy.
func (m *SessionManager) ClaimActivity(ctx context.Context, tokenOrPAT, activityID string) (*ActivityClaimResult, error) {
	seed := FingerprintSeed("", tokenOrPAT)
	dev := m.fp.DeriveDeviceFingerprint(seed)
	return m.ClaimActivityWithDevice(ctx, tokenOrPAT, dev, activityID)
}

// ClaimActivityWithDevice submits a claim with explicit device fingerprint and proxy routing.
func (m *SessionManager) ClaimActivityWithDevice(ctx context.Context, tokenOrPAT string, dev DeviceFingerprint, activityID string) (*ActivityClaimResult, error) {
	var customTransport http.RoundTripper
	if m.client != nil && m.client.Transport != nil {
		customTransport = m.client.Transport
	}
	client, err := newActivityHTTPClient(customTransport)
	if err != nil {
		return nil, err
	}

	effToken, err := m.resolveEffectiveToken(ctx, tokenOrPAT)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/api/v2/activity/claim?activityId=%s", CheckinHostGlobal, url.QueryEscape(activityID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("qoder: build claim request: %w", err)
	}

	SetCosyHeadersWithDevice(req, effToken, dev, time.Now())
	req.ContentLength = 0

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: claim request failed (proxy): %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("qoder: read claim response: %w", err)
	}

	result := &ActivityClaimResult{
		ActivityID: activityID,
		Raw:        strings.TrimSpace(string(bodyBytes)),
		Success:    resp.StatusCode == http.StatusOK,
	}

	var parsed map[string]any
	if err := json.Unmarshal(bodyBytes, &parsed); err == nil {
		if c, ok := parsed["code"].(float64); ok {
			result.Code = int(c)
		}
		if m, ok := parsed["message"].(string); ok {
			result.Message = m
		}
		if d, ok := parsed["data"]; ok {
			dB, _ := json.Marshal(d)
			result.Data = dB
		}
	}

	return result, nil
}

// isActivityClaimable checks if an activity's status signifies it can be claimed.
func isActivityClaimable(status any) bool {
	if status == nil {
		return false
	}
	switch v := status.(type) {
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "available" || s == "claimable" || s == "1" || s == "true"
	case float64:
		return v == 1
	case int:
		return v == 1
	case bool:
		return v
	default:
		return false
	}
}

// AutoClaimActivities performs exchange -> eligibility check -> claim all claimable activities
// using 1 unique machine ID / device fingerprint per account seed and routing strictly via proxy.
func (m *SessionManager) AutoClaimActivities(ctx context.Context, credential string) (*AutoClaimSummary, error) {
	summary := &AutoClaimSummary{
		Claims: []ActivityClaimResult{},
	}

	seed := FingerprintSeed("", credential)
	dev := m.fp.DeriveDeviceFingerprint(seed)

	effToken, err := m.resolveEffectiveToken(ctx, credential)
	if err != nil {
		summary.Error = err.Error()
		return summary, err
	}
	summary.JobToken = effToken

	elig, err := m.CheckActivityEligibilityWithDevice(ctx, effToken, dev)
	if err != nil {
		summary.Error = err.Error()
		return summary, err
	}
	summary.Eligibility = elig
	summary.Success = true

	for _, act := range elig.Activities {
		if isActivityClaimable(act.Status) && act.ActivityID != "" {
			cRes, cErr := m.ClaimActivityWithDevice(ctx, effToken, dev, act.ActivityID)
			if cErr != nil {
				summary.Claims = append(summary.Claims, ActivityClaimResult{
					ActivityID: act.ActivityID,
					Success:    false,
					Message:    cErr.Error(),
				})
			} else {
				summary.Claims = append(summary.Claims, *cRes)
			}
		}
	}

	return summary, nil
}

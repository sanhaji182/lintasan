package qoder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Endpoints are the per-region upstream hosts. Qoder splits its API across
// several hostnames — the auth exchange, the model catalogue and the chat
// stream do not share a host — so they are grouped here rather than derived from
// a single base URL.
type Endpoints struct {
	JobTokenURL   string
	ModelListURL  string
	ChatStreamURL string
}

// GlobalEndpoints are the endpoints for the international region.
var GlobalEndpoints = Endpoints{
	JobTokenURL:   "https://center.qoder.sh/algo/api/v3/user/jobToken?Encode=1",
	ModelListURL:  "https://api2.qoder.sh/algo/api/v2/model/list?Encode=1",
	ChatStreamURL: "https://api1.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1",
}

// CNEndpoints are the endpoints for the mainland-China region, which uses a
// different gateway host.
var CNEndpoints = Endpoints{
	JobTokenURL:   "https://gateway.qoder.com.cn/algo/api/v3/user/jobToken?Encode=1",
	ModelListURL:  "https://gateway.qoder.com.cn/algo/api/v2/model/list?Encode=1",
	ChatStreamURL: "https://gateway.qoder.com.cn/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1",
}

// EndpointsForRegion resolves a region name to its endpoint set. Anything other
// than "cn" is treated as global, which matches upstream's own default.
func EndpointsForRegion(region string) Endpoints {
	if strings.EqualFold(strings.TrimSpace(region), "cn") {
		return CNEndpoints
	}
	return GlobalEndpoints
}

// ============================================================================
// Upstream error classification
// ============================================================================

// UpstreamError is a structured failure reported by Qoder. It covers both the
// transport-level HTTP status and the protocol-level business code, because
// upstream reports most real failures inside a 200 response body.
//
// Two of these matter operationally and are called out explicitly:
//
//   - Code 105 ("Login expired") means chat was refused for this attempt. It is
//     NOT a verdict that the credential is dead, and treating it as one is
//     actively harmful: measured across five sweeps, 7 of 9 credentials returned
//     a normal completion AFTER having returned 105, and availability oscillates
//     between roughly a third and a half of the pool rather than draining. Such a
//     credential also keeps completing the job token exchange and listing models
//     the whole time.
//
//     The correct response is a short cooldown and retry. Quarantining on 105
//     would remove healthy credentials from rotation, and a failover rule that
//     reads it as "this account is gone" would walk the pool and mark all of it
//     dead.
//
//   - Code 10605 carries a queue state: the model is busy and upstream is asking
//     the client to come back later. Queued is not a failure of the account, and
//     treating it as one would wrongly penalise a healthy credential. It is
//     surfaced separately for exactly that reason.
type UpstreamError struct {
	// Status is the envelope HTTP status (e.g. 403), or the transport status.
	Status int
	// Code is the protocol business code from the inner body ("105", "10605").
	Code string
	// Message is upstream's human-readable message.
	Message string
	// Queued indicates upstream returned a queue state rather than a hard error.
	Queued bool
	// RetryAfterSeconds is upstream's requested backoff for a queue state.
	RetryAfterSeconds int
}

// Error implements error.
func (e *UpstreamError) Error() string {
	if e == nil {
		return "qoder: <nil>"
	}
	var b strings.Builder
	b.WriteString("qoder upstream error")
	if e.Status != 0 {
		fmt.Fprintf(&b, " (status %d)", e.Status)
	}
	if e.Code != "" {
		fmt.Fprintf(&b, " (code %s)", e.Code)
	}
	if e.Queued {
		fmt.Fprintf(&b, " [queued, retry after %ds]", e.RetryAfterSeconds)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	return b.String()
}

// IsLoginExpired reports whether chat was refused with the "Login expired" code.
//
// It deliberately does NOT mean "discard this credential". The condition is
// transient on a timescale of minutes — most credentials that report it serve
// normally again shortly after. Callers should apply a short cooldown before
// retrying, and must not quarantine on this signal alone.
func (e *UpstreamError) IsLoginExpired() bool {
	return e != nil && e.Code == "105"
}

// IsQueued reports whether upstream asked the client to come back later because
// the model is busy. This is not an account fault.
func (e *UpstreamError) IsQueued() bool {
	return e != nil && e.Queued
}

// QueueErrorCode is upstream's queue-state code.
const QueueErrorCode = "10605"

// LoginExpiredCode is upstream's code for a credential that is no longer valid
// for chat. It is observed alongside the message "Login expired".
const LoginExpiredCode = "105"

// CreditLimitCode is upstream's code for a request the account is not entitled to
// serve — observed on premium models (Ultimate, Kimi-K3, GLM-5.3) against an account
// whose credits are spent. The envelope carries a `pricingUrl` pointing at the vendor's
// pricing page.
//
// It is NOT an account fault, and this is the whole reason it gets its own code rather
// than folding into a generic auth failure:
//
//   - Measured 2026-09-23: an account at used=300/300, isQuotaExceeded=true still served
//     basic models with HTTP 200 and real content (~400 ms), while premium models
//     returned 112. A "credit limit" therefore means "this account cannot serve THIS
//     model", never "this account is dead".
//   - Code 112 does NOT mean the plan is over either. The same code was observed on an
//     account with a positive reported balance, because its entitlement had shrunk to
//     two models.
//
// Callers must surface it as scoped to a model, and must never use it to quarantine an
// account. Disabling an account stays a human decision.
const CreditLimitCode = "112"

// PricingURLMarker is the body field that accompanies a credit-limit refusal. It is
// used as a second signal so the classification does not rest on a bare number.
const PricingURLMarker = "pricingUrl"

// IsCreditLimit reports whether upstream refused the request because this account is
// not entitled to this model.
//
// Deliberately requires BOTH the code and a corroborating marker. A bare code match
// would misfile any future reuse of 112, and mislabelling an account as credit-limited
// is the kind of wrong flag an operator acts on.
func (e *UpstreamError) IsCreditLimit() bool {
	if e == nil || e.Code != CreditLimitCode {
		return false
	}
	return strings.Contains(e.Message, PricingURLMarker) ||
		strings.Contains(e.Message, "pricing") ||
		strings.Contains(strings.ToLower(e.Message), "credit")
}

// envelope is the wrapper upstream puts around every streamed frame and, in the
// error case, around the failure itself. Reading it first is what prevents a
// dead credential from looking like a slow-but-healthy stream.
type envelope struct {
	Body            string          `json:"body"`
	StatusCodeValue json.RawMessage `json:"statusCodeValue"`
	StatusCode      string          `json:"statusCode"`
}

// envelopeStatus coerces the several shapes upstream uses for the envelope
// status: a number, or a string containing a number.
func envelopeStatus(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// parseEnvelopeError inspects a streamed frame and, if it carries a non-200
// envelope status or a business error code, returns the structured error.
//
// This is the single most important function in the package. Upstream delivers
// failures as HTTP 200 with the real status buried in the first frame; a naive
// SSE reader waits for content that never arrives and the request appears to
// hang until the client gives up.
func parseEnvelopeError(raw []byte) *UpstreamError {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil
	}

	status, hasStatus := envelopeStatus(env.StatusCodeValue)

	// Inner business code: {"code":"105","message":"Login expired"}
	if env.Body != "" {
		var inner struct {
			Code              string `json:"code"`
			Message           string `json:"message"`
			IsQueued          bool   `json:"isQueued"`
			RetryAfterSeconds int    `json:"retryAfterSeconds"`
		}
		if err := json.Unmarshal([]byte(env.Body), &inner); err == nil && inner.Code != "" && inner.Code != "0" {
			statusOut := status
			if statusOut == 0 {
				statusOut = http.StatusOK
			}
			// Message carries BOTH the human text and the raw inner body. The raw
			// body is what the credit-limit classifier keys on: upstream puts its
			// corroborating marker (`pricingUrl`) in the body rather than in
			// `message`, and a classifier that only saw `message` would have to fall
			// back to matching a bare code — which is exactly the fragile
			// classification this avoids.
			msg := inner.Message
			if rawBody := strings.TrimSpace(env.Body); rawBody != "" && rawBody != msg {
				if msg == "" {
					msg = rawBody
				} else {
					msg = msg + ": " + rawBody
				}
			}
			return &UpstreamError{
				Status:            statusOut,
				Code:              inner.Code,
				Message:           msg,
				Queued:            inner.IsQueued || inner.Code == QueueErrorCode,
				RetryAfterSeconds: inner.RetryAfterSeconds,
			}
		}
	}

	// Envelope status only, no business code.
	if hasStatus && status != http.StatusOK {
		return &UpstreamError{
			Status:  status,
			Message: strings.TrimSpace(env.StatusCode + " " + env.Body),
		}
	}
	return nil
}

// ============================================================================
// Session manager
// ============================================================================

// defaultSessionTTL bounds how long a cached session is reused. Upstream-
// reported expiry is honoured when it is shorter than this.
const defaultSessionTTL = 30 * time.Minute

// SessionManager exchanges credentials for sessions and caches them.
//
// One instance is shared by a provider and is safe for concurrent use. Sessions
// are cached per credential because the exchange is a real network round trip
// plus an RSA+AES setup, while the credential itself is long-lived.
type SessionManager struct {
	fp        *Fingerprinter
	endpoints Endpoints
	client    *http.Client
	ttl       time.Duration

	mu    sync.Mutex
	cache map[string]*cachedSession
}

type cachedSession struct {
	session   *Session
	expiresAt time.Time
}

// NewSessionManager returns a manager for the given install salt and region.
// A nil httpClient is replaced with a default client that has a sane timeout.
func NewSessionManager(salt, region string, httpClient *http.Client) *SessionManager {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &SessionManager{
		fp:        NewFingerprinter(salt),
		endpoints: EndpointsForRegion(region),
		client:    httpClient,
		ttl:       defaultSessionTTL,
		cache:     make(map[string]*cachedSession),
	}
}

// Fingerprinter exposes the manager's fingerprinter (for diagnostics and tests).
func (m *SessionManager) Fingerprinter() *Fingerprinter { return m.fp }

// Endpoints exposes the resolved endpoint set.
func (m *SessionManager) Endpoints() Endpoints { return m.endpoints }

// session returns a live session for credential, reusing a cached one when it is
// still fresh. Call Invalidate after a login-expired error to force re-exchange.
func (m *SessionManager) session(ctx context.Context, credential string) (*Session, error) {
	if credential == "" {
		return nil, fmt.Errorf("qoder: empty credential")
	}

	m.mu.Lock()
	if c, ok := m.cache[credential]; ok && time.Now().Before(c.expiresAt) {
		m.mu.Unlock()
		return c.session, nil
	}
	m.mu.Unlock()

	sess, ttl, err := m.login(ctx, credential)
	if err != nil {
		return nil, err
	}

	if ttl <= 0 || ttl > m.ttl {
		ttl = m.ttl
	}
	m.mu.Lock()
	m.cache[credential] = &cachedSession{session: sess, expiresAt: time.Now().Add(ttl)}
	m.mu.Unlock()
	return sess, nil
}

// Invalidate drops any cached session for credential, forcing the next call to
// re-exchange it.
func (m *SessionManager) Invalidate(credential string) {
	m.mu.Lock()
	delete(m.cache, credential)
	m.mu.Unlock()
}

// login performs the credential -> session handshake.
//
// It runs in two phases because device identity depends on information the
// exchange itself returns:
//
//  1. Exchange the credential for a job token. The account id is not yet known,
//     so the device identity is derived from the credential.
//  2. Now that the account id is known, derive the device identity from it and
//     build the session.
//
// Using the credential-derived identity for phase 2 produces a signature
// upstream rejects, which is a subtle failure because the exchange in phase 1
// succeeds either way.
func (m *SessionManager) login(ctx context.Context, credential string) (*Session, time.Duration, error) {
	handshakeSeed := FingerprintSeed("", credential)

	token, err := m.exchangeJobToken(ctx, credential, handshakeSeed)
	if err != nil {
		return nil, 0, err
	}

	accountID := stringField(token, "id")
	if accountID == "" {
		return nil, 0, fmt.Errorf("qoder: job token response carried no account id")
	}

	identity := Identity{
		Name:               stringField(token, "name"),
		AccountID:          accountID,
		UID:                accountID,
		UserType:           stringFieldDefault(token, "userType", "personal_standard"),
		SecurityOauthToken: stringField(token, "securityOauthToken"),
		RefreshToken:       stringField(token, "refreshToken"),
	}

	deviceSeed := FingerprintSeed(accountID, credential)
	sess, err := NewSession(
		identity,
		m.fp.MachineID(deviceSeed),
		m.fp.MachineToken(deviceSeed),
		m.fp.MachineType(deviceSeed),
	)
	if err != nil {
		return nil, 0, err
	}
	return sess, sessionTTL(token), nil
}

// exchangeJobToken posts the credential to the job token endpoint and returns
// the decoded token object.
func (m *SessionManager) exchangeJobToken(ctx context.Context, credential, seed string) (map[string]any, error) {
	// A refresh token is a distinct credential type and is presented in a
	// different field than a personal access token.
	isRefresh := strings.HasPrefix(credential, "drt-")
	personalToken, refreshToken := credential, ""
	if isRefresh {
		personalToken, refreshToken = "", credential
	}

	inner, err := json.Marshal(map[string]any{
		"personalToken":      personalToken,
		"securityOauthToken": "",
		"refreshToken":       refreshToken,
		"needRefresh":        isRefresh,
		"authInfo":           map[string]any{},
	})
	if err != nil {
		return nil, fmt.Errorf("qoder: marshal token payload: %w", err)
	}
	outer, err := json.Marshal(map[string]any{
		"payload":       string(inner),
		"encodeVersion": "1",
	})
	if err != nil {
		return nil, fmt.Errorf("qoder: marshal token envelope: %w", err)
	}
	body, err := EncodeBase64(outer)
	if err != nil {
		return nil, err
	}

	date := CurrentDate()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoints.JobTokenURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("qoder: build token request: %w", err)
	}
	req.Header.Set("cosy-machinetoken", m.fp.MachineToken(seed))
	req.Header.Set("cosy-machinetype", m.fp.MachineType(seed))
	req.Header.Set("cosy-machineid", m.fp.MachineID(seed))
	req.Header.Set("login-version", "v2")
	req.Header.Set("appcode", AppCode)
	req.Header.Set("cosy-version", Version)
	req.Header.Set("cosy-clienttype", "5")
	req.Header.Set("date", date)
	req.Header.Set("signature", SignLegacy(date))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("accept-encoding", "identity")
	req.Header.Set("user-agent", "Go-http-client/2.0")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: token request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("qoder: read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{
			Status:  resp.StatusCode,
			Message: strings.TrimSpace(string(raw)),
		}
	}

	var token map[string]any
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, fmt.Errorf("qoder: decode token response: %w", err)
	}
	// A rejected credential can come back as a 200 with a business error inside.
	if e := parseEnvelopeError(raw); e != nil {
		return nil, e
	}
	return token, nil
}

// BuildAPIHeaders assembles the signed headers for an authenticated upstream
// call. body is signed as-is, so callers must pass the exact bytes they send.
func (m *SessionManager) BuildAPIHeaders(sess *Session, body []byte, path string, accept string, extra map[string]string) (map[string]string, error) {
	payload, err := sess.BuildPayload()
	if err != nil {
		return nil, err
	}
	date := fmt.Sprintf("%d", UnixSec())
	sig := SignRequest(payload, sess.CosyKey, date, body, PathSig(path))

	h := map[string]string{
		"content-type":          "application/json",
		"accept":                accept,
		"cosy-data-policy":      "agree",
		"cosy-machinetype":      sess.MachineType,
		"cosy-machineid":        sess.MachineID,
		"cosy-machinetoken":     sess.MachineToken,
		"cosy-clienttype":       "5",
		"cosy-date":             date,
		"cosy-key":              sess.CosyKey,
		"cosy-user":             sess.Identity.UID,
		"cosy-version":          Version,
		"cosy-scene":            "assistant",
		"cosy-business-product": "ide",
		"cosy-business-type":    "agent",
		"login-version":         "v2",
		"cache-control":         "no-cache",
		"user-agent":            "Go-http-client/2.0",
		"authorization":         ComposeBearer(payload, sig),
	}
	for k, v := range extra {
		h[k] = v
	}
	return h, nil
}

// pathOf extracts the path (without query) from a full URL, for signing.
func pathOf(rawURL string) string {
	if i := strings.Index(rawURL, "://"); i >= 0 {
		rest := rawURL[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			rawURL = rest[j:]
		} else {
			return "/"
		}
	}
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		rawURL = rawURL[:i]
	}
	return rawURL
}

// ============================================================================
// response field helpers
// ============================================================================

// stringField reads a string field that upstream may encode as a non-string
// scalar (a numeric id, for example).
func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strings.TrimSuffix(fmt.Sprintf("%f", t), ".000000")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// stringFieldDefault is stringField with a fallback for empty or missing values.
func stringFieldDefault(m map[string]any, key, def string) string {
	if s := strings.TrimSpace(stringField(m, key)); s != "" {
		return s
	}
	return def
}

// sessionTTL derives how long a session may be reused from the token's reported
// expiry, when present. A missing or unparseable expiry yields zero, which the
// caller clamps to the manager default.
func sessionTTL(token map[string]any) time.Duration {
	raw := stringField(token, "expireTime")
	if raw == "" {
		return 0
	}
	// Numeric expiry: milliseconds since epoch, or seconds if implausibly small.
	if ms, err := parseInt64(raw); err == nil && ms > 0 {
		if ms < 1e12 {
			ms *= 1000
		}
		exp := time.UnixMilli(ms)
		if d := time.Until(exp); d > 0 {
			// Renew well before actual expiry so a session never lapses mid-request.
			return d - 5*time.Minute
		}
		return 0
	}
	return 0
}

// parseInt64 parses a decimal integer, tolerating a trailing fractional part.
func parseInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// readAllLimited reads a response body with a hard cap.
func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, io.LimitReader(r, limit))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

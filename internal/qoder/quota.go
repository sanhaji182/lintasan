package qoder

// quota.go — account quota / remaining credits.
//
// Qoder exposes the account's credit state at a separate host from the chat and
// catalogue endpoints, authenticated with the session's security OAuth token
// rather than the PAT. That means a quota read costs a credential exchange, so
// results are cached: credit state changes on the order of minutes and a
// dashboard refresh loop would otherwise re-auth every account on every poll.
//
// The upstream shape carries used/total/remaining per bucket. It is NOT a single
// number: an account has a base allocation ("userQuota") plus an optional add-on
// pack, and conflating them would under-report what is actually available.
// Both are surfaced separately for exactly that reason.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// QuotaEndpoint is the upstream credit-usage endpoint (international region).
const QuotaEndpoint = "https://openapi.qoder.sh/api/v2/quota/usage"

// QuotaPlanEndpoint reports the account's plan.
const QuotaPlanEndpoint = "https://openapi.qoder.sh/api/v2/user/plan"

// QuotaBucket is one allocation of credits. Upstream reports a base allocation and
// an optional add-on pack; they are kept distinct because the totals differ and a
// caller deciding whether it can run a request needs the sum, while an operator
// looking at a purchase decision needs to know which bucket is nearly dry.
type QuotaBucket struct {
	Used      float64 `json:"used"`
	Total     float64 `json:"total"`
	Remaining float64 `json:"remaining"`
	ResetTime string  `json:"reset_time,omitempty"`
}

// Quota is an account's credit state.
type Quota struct {
	Plan            string       `json:"plan,omitempty"`
	UserQuota       *QuotaBucket `json:"user_quota,omitempty"`
	AddonQuota      *QuotaBucket `json:"addon_quota,omitempty"`
	IsQuotaExceeded bool         `json:"is_quota_exceeded"`
	ExpiresAt       int64        `json:"expires_at,omitempty"`
	// FetchedAt records when this snapshot was taken, so a dashboard can show its
	// age rather than implying it is live.
	FetchedAt time.Time `json:"fetched_at"`
}

// TotalRemaining is the credits available across both buckets.
func (q *Quota) TotalRemaining() float64 {
	if q == nil {
		return 0
	}
	var sum float64
	if q.UserQuota != nil {
		sum += q.UserQuota.Remaining
	}
	if q.AddonQuota != nil {
		sum += q.AddonQuota.Remaining
	}
	return sum
}

// TotalAllocation is the credits the account was granted in total.
func (q *Quota) TotalAllocation() float64 {
	if q == nil {
		return 0
	}
	var sum float64
	if q.UserQuota != nil {
		sum += q.UserQuota.Total
	}
	if q.AddonQuota != nil {
		sum += q.AddonQuota.Total
	}
	return sum
}

// quotaCacheTTL is how long a quota snapshot is reused. Credit state moves on the
// order of minutes, and each read costs a full credential exchange, so this trades
// a little staleness for a lot fewer auth round trips.
const quotaCacheTTL = 5 * time.Minute

type cachedQuota struct {
	quota     *Quota
	expiresAt time.Time
}

// QuotaCache stores recent quota snapshots per credential.
//
// Kept separate from the session cache because the lifetimes differ: a session is
// good for tens of minutes, while credit usage is worth refreshing much sooner.
// Sharing one cache would force one of the two to a wrong TTL.
type QuotaCache struct {
	mu    sync.Mutex
	items map[string]cachedQuota
	ttl   time.Duration
}

// NewQuotaCache returns a cache with the given TTL. Zero uses the default.
func NewQuotaCache(ttl time.Duration) *QuotaCache {
	if ttl <= 0 {
		ttl = quotaCacheTTL
	}
	return &QuotaCache{items: make(map[string]cachedQuota), ttl: ttl}
}

// Get returns a cached snapshot for credential.
func (c *QuotaCache) Get(credential string) (*Quota, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[credential]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.quota, true
}

// Put stores a snapshot.
func (c *QuotaCache) Put(credential string, q *Quota) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[credential] = cachedQuota{quota: q, expiresAt: time.Now().Add(c.ttl)}
}

// Invalidate drops one entry, for a manual refresh.
func (c *QuotaCache) Invalidate(credential string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, credential)
}

// FetchQuota reads an account's credit state.
//
// It authenticates with the session's security OAuth token, not the PAT: the quota
// host accepts the former. That is why this cannot be a bare GET with the
// connection's stored key and why it goes through the session manager.
func (m *SessionManager) FetchQuota(ctx context.Context, credential string) (*Quota, error) {
	sess, err := m.session(ctx, credential)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sess.Identity.SecurityOauthToken) == "" {
		// Without a security OAuth token the quota host has nothing to
		// authenticate with. Reported as a distinct condition rather than an empty
		// quota, so a dashboard shows "unavailable" instead of "zero credits".
		return nil, fmt.Errorf("qoder: session carries no security OAuth token; quota is unavailable for this credential")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, QuotaEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("qoder: build quota request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+sess.Identity.SecurityOauthToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "Go-http-client/2.0")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: quota request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("qoder: read quota response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
	}

	quota, err := parseQuota(raw)
	if err != nil {
		return nil, err
	}
	quota.Plan = m.fetchPlan(ctx, sess.Identity.SecurityOauthToken)
	quota.FetchedAt = time.Now()
	return quota, nil
}

// parseQuota decodes the quota response.
//
// Field names are camelCase here (userQuota, addOnQuota, isQuotaExceeded) even
// though the model list uses snake_case — the two endpoints come from different
// services. Typing these the wrong way round silently yields zero credits rather
// than an error, which is the failure mode worth guarding against.
func parseQuota(raw []byte) (*Quota, error) {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("qoder: decode quota: %w", err)
	}
	// Some responses nest the payload under "data".
	if d, ok := envelope["data"].(map[string]any); ok {
		envelope = d
	}

	q := &Quota{
		IsQuotaExceeded: envelope["isQuotaExceeded"] == true,
		ExpiresAt:       int64(numField(envelope, "expiresAt")),
		UserQuota:       bucketFrom(envelope, "userQuota"),
		AddonQuota:      bucketFrom(envelope, "addOnQuota"),
	}
	if q.UserQuota == nil && q.AddonQuota == nil {
		return nil, fmt.Errorf("qoder: quota response contained no recognizable buckets")
	}
	return q, nil
}

// bucketFrom extracts one bucket, accepting the misspelled variants upstream has
// been observed to use across versions.
func bucketFrom(m map[string]any, key string) *QuotaBucket {
	candidates := []string{key}
	switch key {
	case "addOnQuota":
		candidates = append(candidates, "addonQuota", "addon_quota")
	case "userQuota":
		candidates = append(candidates, "user_quota")
	}
	for _, c := range candidates {
		obj, ok := m[c].(map[string]any)
		if !ok {
			continue
		}
		b := &QuotaBucket{
			Used:      numField(obj, "used"),
			Total:     numField(obj, "total"),
			Remaining: numField(obj, "remaining"),
			ResetTime: strField(obj, "resetTime"),
		}
		if b.ResetTime == "" {
			b.ResetTime = strField(obj, "reset_time")
		}
		// A bucket with no total and no remaining is not a bucket; treat it as
		// absent rather than reporting a confident zero.
		if b.Total == 0 && b.Remaining == 0 && b.Used == 0 {
			continue
		}
		return b
	}
	return nil
}

// numField reads a numeric field that may arrive as a number or a numeric string.
func numField(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		if f, err := parseFloat(v); err == nil {
			return f
		}
	}
	return 0
}

// parseFloat is a small tolerant float parser.
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%f", &f)
	return f, err
}

// fetchPlan reads the plan name. Best-effort: a missing plan is cosmetic, so a
// failure here must not fail the quota read.
func (m *SessionManager) fetchPlan(ctx context.Context, token string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, QuotaPlanEndpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	raw, err := readAllLimited(resp.Body, 1<<18)
	if err != nil {
		return ""
	}
	var m2 map[string]any
	if err := json.Unmarshal(raw, &m2); err != nil {
		return ""
	}
	if d, ok := m2["data"].(map[string]any); ok {
		m2 = d
	}
	if s := strField(m2, "plan"); s != "" {
		return s
	}
	return strField(m2, "planName")
}

// QuotaForAll reads quota for several credentials.
//
// Sequential by design: these are authenticated reads against one upstream, and a
// parallel fan-out across a whole pool is what triggered the upstream's
// throttling during development. A dashboard refresh is not worth that.
func (m *SessionManager) QuotaForAll(ctx context.Context, credentials []string, cache *QuotaCache) map[string]any {
	out := make(map[string]any, len(credentials))
	for _, cred := range credentials {
		if cache != nil {
			if q, ok := cache.Get(cred); ok {
				out[cred] = q
				continue
			}
		}
		q, err := m.FetchQuota(ctx, cred)
		if err != nil {
			// The error travels with the credential so a dashboard can show which
			// account is unavailable rather than a blank column.
			out[cred] = map[string]any{"error": err.Error()}
			continue
		}
		if cache != nil {
			cache.Put(cred, q)
		}
		out[cred] = q
	}
	return out
}

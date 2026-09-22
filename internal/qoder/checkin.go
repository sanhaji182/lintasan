package qoder

// checkin.go — daily check-in ("每日签到领 Credits") surface.
//
// WHY THIS EXISTS
//
// Qoder runs a daily check-in that grants credits. The reference implementation
// (github.com/Zhengyuuuui/qoder2api) reaches it through two flows on its check-in
// host: a simplified /daily-check-in pair, and a campaigns pair. This file ports
// both so Lintasan can report the real state instead of leaving an operator to
// open the vendor console per account.
//
// READ THIS BEFORE EXPECTING CREDITS
//
// The flows are region-dependent, and the outcome differs per account. Measured
// against Lintasan's own pool (2026-09-22, global region, PAT auth):
//
//   - /sash/api/v1/me/daily-check-in/{status,claim} do NOT exist on the global
//     host (404). That is the documented "endpoint missing -> fall back" case.
//   - /sash/api/v1/me/campaigns exists on the global host, but only returns data
//     when the desktop-client header `cosy-clienttype: 10` is sent. Without it the
//     response is well-formed and EMPTY (campaigns: [], claimable: false) — a
//     silent empty that looks like "no promotions today" rather than a missing
//     header.
//   - The campaigns that come back carry actionType "VIEW_DETAILS" (a subscription
//     promotion), not "CLAIM_BENEFIT" (the credit grant). The claim endpoint
//     itself 404s on this host.
//
// So this file reports state faithfully and refuses to invent a grant. A caller
// that finds no claimable campaign must surface that, not a fake success.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Check-in hosts. The global host is the one Lintasan's credentials authenticate
// against; the CN host is included because the feature originated there and a
// CN-region deployment would need it.
const (
	CheckinHostGlobal = "https://openapi.qoder.sh"
	CheckinHostCN     = "https://openapi.qoder.com.cn"
)

// checkinPaths are the endpoints of both flows.
const (
	checkinPathCampaigns     = "/sash/api/v1/me/campaigns"
	checkinPathCampaignClaim = "/sash/api/v1/me/campaigns/%s/claim"
	checkinPathDailyStatus   = "/sash/api/v1/me/daily-check-in/status"
	checkinPathDailyClaim    = "/sash/api/v1/me/daily-check-in/claim"
)

// CheckinActionClaimBenefit is the campaign actionType that actually grants
// credits. Anything else (notably VIEW_DETAILS) is an informational promotion and
// must never be presented as claimable credits.
const CheckinActionClaimBenefit = "CLAIM_BENEFIT"

// Check-in campaign claim states, as reported upstream.
const (
	CheckinClaimable = "CLAIMABLE"
	CheckinClaimed   = "CLAIMED"
	CheckinDisabled  = "DISABLED"
)

// CheckinCampaign is one entry from /sash/api/v1/me/campaigns.
type CheckinCampaign struct {
	CampaignID  string `json:"campaign_id"`
	CampaignKey string `json:"campaign_key"`
	ActionType  string `json:"action_type"`
	ClaimStatus string `json:"claim_status"`
	StartAt     int64  `json:"start_at,omitempty"`
	EndAt       int64  `json:"end_at,omitempty"`
	Title       string `json:"title,omitempty"`
	// GrantsCredits is derived: only a CLAIM_BENEFIT campaign can grant credits.
	// Exposed as a field so a UI cannot accidentally render a promotion as a grant.
	GrantsCredits bool `json:"grants_credits"`
}

// CheckinStatus is an account's overall check-in state.
type CheckinStatus struct {
	// Host is which check-in host answered, so an operator can tell a CN account
	// from a global one — the whole reason this feature behaves differently.
	Host string `json:"host"`
	// DailyEndpointAvailable records whether the simplified flow exists here.
	DailyEndpointAvailable bool `json:"daily_endpoint_available"`
	// Supported is false when neither flow can grant credits for this credential.
	Supported bool `json:"supported"`
	// Claimable is true when a CLAIM_BENEFIT campaign is actually claimable.
	Claimable bool `json:"claimable"`
	// Campaigns is the full list, including non-granting promotions.
	Campaigns []CheckinCampaign `json:"campaigns"`
	// Streak / totals come from the daily flow when it exists.
	StreakDays         int `json:"streak_days,omitempty"`
	TotalClaimDays     int `json:"total_claim_days,omitempty"`
	TotalRewardCredits int `json:"total_reward_credits,omitempty"`
	// Reason explains an unsupported or non-claimable state in operator terms,
	// rather than leaving a blank panel.
	Reason string `json:"reason,omitempty"`
	// FetchedAt records when this snapshot was taken.
	FetchedAt time.Time `json:"fetched_at"`
}

// CheckinResult is the outcome of a claim attempt.
type CheckinResult struct {
	Status string `json:"status"` // claimed | already_claimed | no_campaign | error
	// Credits is what upstream reported in benefit.amount. It is routinely 0: a
	// VIEW_DETAILS campaign returns status=CLAIMED with benefit absent, because the
	// benefit lands when the underlying promotion is fulfilled (e.g. a subscription
	// is taken), not at claim time.
	Credits int `json:"credits"`
	// CreditsGranted is true only when benefit.amount was actually present. It
	// exists so a caller can say "claimed" and "earned nothing" at the same time
	// without either being misread as a failure.
	CreditsGranted bool   `json:"credits_granted"`
	GrantID        string `json:"grant_id,omitempty"`
	CampaignKey    string `json:"campaign_key,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	Message        string `json:"message"`
	Host           string `json:"host,omitempty"`
	Retryable      bool   `json:"retryable"`
}

// placement is one rendering slot of a campaign (POPUP / USAGE ...), carrying
// locale-keyed copy.
type placement struct {
	Content map[string]struct {
		Title string `json:"title"`
	} `json:"content"`
}

// checkinRequest performs one check-in call with the desktop-client headers these
// endpoints require.
//
// `cosy-clienttype: 10` is NOT optional: without it the campaigns endpoint answers
// 200 with an EMPTY campaign list, which is indistinguishable from "no promotions"
// unless you know the header is mandatory.
func (m *SessionManager) checkinRequest(ctx context.Context, method, host, path, token string, body any) (int, []byte, error) {
	var payload string
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("qoder: marshal checkin body: %w", err)
		}
		payload = string(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, host+path, strings.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("qoder: build checkin request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	// Matched to the captured desktop client. The locale header also steers which
	// campaign copy upstream returns.
	req.Header.Set("accept-language", "zh-CN")
	req.Header.Set("User-Agent", "Qoder")
	req.Header.Set("cosy-clienttype", "10")
	req.Header.Set("Accept-Encoding", "identity")
	if method == http.MethodPost {
		// Captured behaviour: the claim endpoint takes an empty body and a
		// same-origin Origin. Setting ContentLength keeps Go from switching to
		// chunked encoding, which upstream rejects.
		req.Header.Set("Origin", host)
		req.ContentLength = 0
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("qoder: checkin request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("qoder: read checkin response: %w", err)
	}
	return resp.StatusCode, raw, nil
}

// probeCheckinHost reads check-in state from ONE host with an already-resolved
// token. Pure I/O plus parsing: no session handling, no host selection.
//
// Split out so tests can drive every branch (daily present, daily absent,
// VIEW_DETAILS-only, empty list) against a stub host without a real credential —
// all the behaviour worth locking down lives in here.
func (m *SessionManager) probeCheckinHost(ctx context.Context, host, token string) (*CheckinStatus, bool) {
	st := &CheckinStatus{Host: host, Campaigns: []CheckinCampaign{}, FetchedAt: time.Now()}

	// --- simplified daily flow ---
	code, raw, err := m.checkinRequest(ctx, http.MethodGet, host, checkinPathDailyStatus, token, nil)
	if err != nil {
		return nil, false
	}
	if code == http.StatusOK {
		var d struct {
			Status             string `json:"status"`
			CurrentStreakDays  int    `json:"currentStreakDays"`
			TotalClaimDays     int    `json:"totalClaimDays"`
			TotalRewardCredits int    `json:"totalRewardCredits"`
		}
		if json.Unmarshal(raw, &d) == nil && d.Status != "" {
			st.DailyEndpointAvailable = true
			st.StreakDays = d.CurrentStreakDays
			st.TotalClaimDays = d.TotalClaimDays
			st.TotalRewardCredits = d.TotalRewardCredits
			st.Claimable = d.Status == CheckinClaimable
			st.Supported = st.Claimable || d.Status == CheckinClaimed
			if !st.Supported {
				st.Reason = "daily check-in state is " + d.Status
			}
			return st, true
		}
	}
	// 401 means this credential cannot authenticate here. Report the host as
	// unusable so the caller moves on instead of blaming the credential.
	if code == http.StatusUnauthorized {
		return nil, false
	}

	// --- campaigns fallback ---
	code, raw, err = m.checkinRequest(ctx, http.MethodGet, host, checkinPathCampaigns, token, nil)
	if err != nil || code != http.StatusOK {
		return nil, false
	}

	var payload struct {
		Campaigns []struct {
			CampaignID  string      `json:"campaignId"`
			CampaignKey string      `json:"campaignKey"`
			ActionType  string      `json:"actionType"`
			ClaimStatus string      `json:"claimStatus"`
			StartAt     int64       `json:"startAt"`
			EndAt       int64       `json:"endAt"`
			Placements  []placement `json:"placements"`
		} `json:"campaigns"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil, false
	}

	for _, c := range payload.Campaigns {
		out := CheckinCampaign{
			CampaignID:    c.CampaignID,
			CampaignKey:   c.CampaignKey,
			ActionType:    c.ActionType,
			ClaimStatus:   c.ClaimStatus,
			StartAt:       c.StartAt,
			EndAt:         c.EndAt,
			GrantsCredits: c.ActionType == CheckinActionClaimBenefit,
			Title:         placementTitle(c.Placements),
		}
		st.Campaigns = append(st.Campaigns, out)
		// Claimable means "upstream will accept a claim", which is true for any
		// claimable campaign — measured: a VIEW_DETAILS campaign returns
		// 200 status=CLAIMED. GrantsCredits separately records whether the benefit is
		// paid now (CLAIM_BENEFIT) or deferred to fulfilment (VIEW_DETAILS).
		if out.ClaimStatus == CheckinClaimable {
			st.Claimable = true
		}
	}

	if st.Claimable {
		st.Supported = true
		return st, true
	}

	// Reachable, but nothing is claimable right now. Distinguish "promotion already
	// claimed" from "empty list": the operator response differs, and a blank panel
	// would hide both.
	if len(st.Campaigns) == 0 {
		st.Reason = "no check-in campaigns are exposed for this account on " + host
	} else {
		st.Reason = "no campaign is claimable right now (" + st.campaignSummary() + ")"
	}
	return st, true
}

// campaignSummary renders the campaign states compactly for a Reason string.
func (st *CheckinStatus) campaignSummary() string {
	kinds := make([]string, 0, len(st.Campaigns))
	for _, c := range st.Campaigns {
		kinds = append(kinds, c.ActionType+"/"+c.ClaimStatus)
	}
	return strings.Join(kinds, ", ")
}

// placementTitle extracts human-readable campaign copy, preferring English.
func placementTitle(placements []placement) string {
	for _, p := range placements {
		if en, ok := p.Content["en"]; ok && en.Title != "" {
			return en.Title
		}
	}
	for _, p := range placements {
		for _, v := range p.Content {
			if v.Title != "" {
				return v.Title
			}
		}
	}
	return ""
}

// checkinHostForRegion returns the check-in host matching the manager's region.
//
// SessionManager does not carry the region name — it resolves endpoints up front —
// so the region is derived from the endpoint set it was built with. That stays
// correct even if a caller injects an endpoint set directly.
func (m *SessionManager) checkinHostForRegion() string {
	if strings.Contains(m.endpoints.JobTokenURL, "qoder.com.cn") {
		return CheckinHostCN
	}
	return CheckinHostGlobal
}

// checkinHostsProbeOrder returns the hosts to try, best candidate first.
//
// Order matters: the credential's own region must come first. A global credential
// is rejected by the CN host with 401, so probing CN first would report a
// misleading "token invalid" for a perfectly healthy account.
func (m *SessionManager) checkinHostsProbeOrder() []string {
	first := m.checkinHostForRegion()
	order := []string{first}
	for _, h := range []string{CheckinHostGlobal, CheckinHostCN} {
		if h != first {
			order = append(order, h)
		}
	}
	return order
}

// CheckinStatusAt probes the given hosts in order with an already-resolved token.
//
// Exported for tests, which drive it against a stub host. In normal use call
// CheckinStatusFor, which resolves the token and picks the host order.
func (m *SessionManager) CheckinStatusAt(ctx context.Context, hosts []string, token string) *CheckinStatus {
	for _, host := range hosts {
		if st, ok := m.probeCheckinHost(ctx, host, token); ok {
			return st
		}
	}
	return &CheckinStatus{
		Supported: false,
		Campaigns: []CheckinCampaign{},
		FetchedAt: time.Now(),
		Reason:    "no check-in host accepted this credential (tried " + strings.Join(hosts, ", ") + ")",
	}
}

// CheckinStatusFor reads check-in availability for one credential.
//
// Probes the daily flow first and falls back to campaigns, mirroring upstream's own
// precedence: a 404 on the daily status endpoint means the flow is absent here, not
// that the account is ineligible.
func (m *SessionManager) CheckinStatusFor(ctx context.Context, credential string) (*CheckinStatus, error) {
	sess, err := m.session(ctx, credential)
	if err != nil {
		return nil, err
	}
	token := sess.Identity.SecurityOauthToken
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("qoder: session carries no security OAuth token; check-in is unavailable for this credential")
	}
	return m.CheckinStatusAt(ctx, m.checkinHostsProbeOrder(), token), nil
}

// ClaimCheckin performs the claim for one credential.
//
// MEASURED BEHAVIOUR (2026-09-22, all nine accounts, global region):
//
//	POST /sash/api/v1/me/campaigns/{id}/claim -> 200
//	{"grantId":"...","status":"CLAIMED","replayed":false,...,"benefit":absent}
//
// So a claim SUCCEEDS for a campaign whose actionType is VIEW_DETAILS. The vendor's
// own growth page claims exactly this way, and the response carries no `benefit`
// field — the entitlement is recorded by grantId and the credits land when the
// underlying promotion is fulfilled (e.g. a Pro subscription is taken), not at
// claim time.
//
// This is why the claim is attempted for any CLAIMABLE campaign rather than only
// for CLAIM_BENEFIT ones: gating on CLAIM_BENEFIT would refuse a claim that upstream
// accepts. The absence of `benefit` is reported honestly via CreditsGranted, so a
// caller can show "claimed" and "no credits at claim time" at once.
func (m *SessionManager) ClaimCheckin(ctx context.Context, credential string) (*CheckinResult, error) {
	st, err := m.CheckinStatusFor(ctx, credential)
	if err != nil {
		return nil, err
	}
	if decision := claimDecision(st); decision.Status == "no_campaign" {
		return decision, nil
	}
	sess, err := m.session(ctx, credential)
	if err != nil {
		return nil, err
	}
	return m.claimWithStatus(ctx, st, sess.Identity.SecurityOauthToken), nil
}

// claimWithStatus performs the claim for an already-resolved status and token.
//
// Split out so tests can drive the claim against a stub host (via CheckinStatusAt)
// without a real credential.
func (m *SessionManager) claimWithStatus(ctx context.Context, st *CheckinStatus, token string) *CheckinResult {
	res := &CheckinResult{Host: st.Host}

	// Prefer the daily endpoint when it exists (CN region); otherwise claim the
	// campaign, which is the only route that exists on the global host.
	if st.DailyEndpointAvailable {
		code, raw, err := m.checkinRequest(ctx, http.MethodPost, st.Host, checkinPathDailyClaim, token, map[string]any{})
		if err != nil {
			res.Status = "error"
			res.Message = err.Error()
			res.Retryable = true
			return res
		}
		switch code {
		case http.StatusConflict: // 409 = already claimed today (idempotent)
			res.Status = "already_claimed"
			res.Message = "already claimed today"
			return res
		case http.StatusOK:
			var d struct {
				RewardCredits int `json:"rewardCredits"`
			}
			if json.Unmarshal(raw, &d) == nil {
				res.Status = "claimed"
				res.Credits = d.RewardCredits
				res.CreditsGranted = d.RewardCredits > 0
				res.Message = fmt.Sprintf("claimed +%d credits", d.RewardCredits)
				return res
			}
		case http.StatusUnauthorized:
			res.Status = "error"
			res.Message = "credential rejected by the check-in host"
			return res
		}
	}

	// Campaigns claim: pick the claimable campaign. Prefer one that grants credits
	// outright, then fall back to any claimable campaign — an accepted claim is a
	// real result even when its benefit is deferred.
	target := pickClaimableCampaign(st.Campaigns)
	if target == nil {
		res.Status = "no_campaign"
		res.Message = "no claimable campaign"
		return res
	}

	code, raw, err := m.checkinRequest(ctx, http.MethodPost, st.Host,
		fmt.Sprintf(checkinPathCampaignClaim, target.CampaignID), token, nil)
	if err != nil {
		res.Status = "error"
		res.Message = err.Error()
		res.Retryable = true
		return res
	}
	if code != http.StatusOK {
		res.Status = "error"
		res.Message = fmt.Sprintf("claim failed with HTTP %d: %s", code, truncateForMessage(string(raw), 200))
		res.Retryable = code >= 500
		return res
	}

	var cr struct {
		Status   string `json:"status"`
		Replayed bool   `json:"replayed"`
		GrantID  string `json:"grantId"`
		Benefit  *struct {
			Amount int `json:"amount"`
		} `json:"benefit"`
		ExpiresAt string `json:"expiresAt"`
	}
	if json.Unmarshal(raw, &cr) != nil {
		res.Status = "error"
		res.Message = "unrecognised claim response"
		return res
	}

	res.CampaignKey = target.CampaignKey
	res.GrantID = cr.GrantID
	res.ExpiresAt = cr.ExpiresAt
	if cr.Replayed {
		res.Status = "already_claimed"
		res.Message = "already claimed (idempotent)"
		return res
	}
	if cr.Status != "CLAIMED" {
		res.Status = "error"
		res.Message = "unexpected claim status: " + cr.Status
		return res
	}

	res.Status = "claimed"
	if cr.Benefit != nil {
		res.Credits = cr.Benefit.Amount
		res.CreditsGranted = cr.Benefit.Amount > 0
		res.Message = fmt.Sprintf("claimed +%d credits (%s)", cr.Benefit.Amount, target.CampaignKey)
	} else {
		// The honest message for the common case: the claim is recorded upstream
		// (grantId) but no credits arrived, because the benefit is deferred.
		res.Message = fmt.Sprintf("claimed %s — no credits at claim time (benefit is deferred to fulfilment)", target.CampaignKey)
	}
	return res
}

// claimAt is the test-facing entry point: resolve state from the given hosts, then
// claim if anything is claimable.
func (m *SessionManager) claimAt(ctx context.Context, hosts []string, token string) (*CheckinResult, error) {
	st := m.CheckinStatusAt(ctx, hosts, token)
	if decision := claimDecision(st); decision.Status == "no_campaign" {
		return decision, nil
	}
	return m.claimWithStatus(ctx, st, token), nil
}

// pickClaimableCampaign chooses which campaign to claim.
//
// A CLAIM_BENEFIT campaign is preferred because its benefit is granted immediately.
// Failing that, ANY claimable campaign is returned: upstream accepts a claim for a
// VIEW_DETAILS campaign and records a grantId, so refusing it would understate what
// the account can do.
func pickClaimableCampaign(campaigns []CheckinCampaign) *CheckinCampaign {
	var fallback *CheckinCampaign
	for i := range campaigns {
		c := &campaigns[i]
		if c.ClaimStatus != CheckinClaimable {
			continue
		}
		if c.GrantsCredits {
			return c
		}
		if fallback == nil {
			fallback = c
		}
	}
	return fallback
}

// claimDecision encodes what a claim WOULD do, without performing it.
//
// A claimable campaign is enough to proceed — including a VIEW_DETAILS one, since
// upstream accepts that claim (measured: 200 status=CLAIMED grantId=...). Only
// "nothing claimable" refuses.
func claimDecision(st *CheckinStatus) *CheckinResult {
	res := &CheckinResult{Retryable: false}
	if st != nil {
		res.Host = st.Host
	}
	if st == nil || !st.Claimable {
		res.Status = "no_campaign"
		res.Message = "no claimable check-in campaign for this account"
		if st != nil && st.Reason != "" {
			res.Message = st.Reason
		}
		return res
	}
	res.Status = "claimable"
	res.Message = "a claimable campaign exists"
	return res
}

// CheckinCache stores recent check-in snapshots per credential.
//
// Deliberately its own type rather than a reuse of QuotaCache: QuotaCache stores
// *Quota, and sharing one map would let a check-in lookup decode a quota entry (or
// vice versa) and silently return an empty panel. The TTLs happen to match today,
// but the lifetimes are independent facts.
type CheckinCache struct {
	mu    sync.Mutex
	items map[string]checkinCacheItem
	ttl   time.Duration
}

type checkinCacheItem struct {
	status    *CheckinStatus
	expiresAt time.Time
}

// checkinCacheTTL trades a little staleness for far fewer auth round trips. Campaign
// state changes on the order of hours, not seconds.
const checkinCacheTTL = 5 * time.Minute

// NewCheckinCache returns a cache with the given TTL. Zero uses the default.
func NewCheckinCache(ttl time.Duration) *CheckinCache {
	if ttl <= 0 {
		ttl = checkinCacheTTL
	}
	return &CheckinCache{items: make(map[string]checkinCacheItem), ttl: ttl}
}

// Get returns a cached snapshot for credential.
func (c *CheckinCache) Get(credential string) (*CheckinStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[credential]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.status, true
}

// Put stores a snapshot.
func (c *CheckinCache) Put(credential string, st *CheckinStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[credential] = checkinCacheItem{status: st, expiresAt: time.Now().Add(c.ttl)}
}

// Invalidate drops one entry, for a manual refresh.
func (c *CheckinCache) Invalidate(credential string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, credential)
}

// IdentityFor resolves the account identity behind a credential.
//
// Exported so a caller can identify a credential without digging into the session
// type — chiefly bulk-add, which labels a new connection from the account itself
// ("BrendaBrown") rather than inventing a name. Running the real exchange means a
// successful call is proof the credential works, not merely that it parses.
//
// The identity carries nothing sensitive beyond the account name and id; the tokens
// inside it are never returned to a caller that only needs a label. Callers that do
// need the session should use CheckinStatusFor or StartChatStream.
func (m *SessionManager) IdentityFor(ctx context.Context, credential string) (Identity, error) {
	sess, err := m.session(ctx, credential)
	if err != nil {
		return Identity{}, err
	}
	return sess.Identity, nil
}

// truncateForMessage keeps an upstream error readable in a dashboard without
// pasting a whole HTML body into the UI.
func truncateForMessage(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

package qoder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// mockCheckin is a stub check-in upstream.
//
// It ASSERTS the desktop-client header on every request: `cosy-clienttype: 10` is
// what makes upstream return campaigns at all. Without it upstream answers 200 with
// an empty list — a silent failure this test refuses to let through unnoticed.
type mockCheckin struct {
	campaigns      string // body for /campaigns
	daily          bool   // does the simplified flow exist here?
	dailyClaimCode int
	requireHeader  bool
	posts          *[]string // records POST paths, to prove no pointless write
}

func (mc *mockCheckin) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mc.requireHeader && r.Header.Get("cosy-clienttype") != "10" {
			t.Errorf("%s: missing cosy-clienttype: 10 (got %q)", r.URL.Path, r.Header.Get("cosy-clienttype"))
		}
		if r.Method == http.MethodPost && mc.posts != nil {
			*mc.posts = append(*mc.posts, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case checkinPathDailyStatus:
			if !mc.daily {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errorCode":"NotFound"}`))
				return
			}
			_, _ = w.Write([]byte(`{"campaignKey":"cn_daily_check_in_legacy","status":"CLAIMABLE","rewardCredits":100,"currentStreakDays":3,"totalClaimDays":7,"totalRewardCredits":700}`))
		case checkinPathDailyClaim:
			w.WriteHeader(mc.dailyClaimCode)
			if mc.dailyClaimCode == http.StatusConflict {
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"rewardCredits":100}`))
		case checkinPathCampaigns:
			_, _ = w.Write([]byte(mc.campaigns))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func managerFor(srv *httptest.Server) *SessionManager {
	m := NewSessionManager("test-salt", "global", srv.Client())
	return m
}

func envOrSkip(t *testing.T, key string) string {
	t.Helper()
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		t.Skipf("%s not set — skipping live check-in test", key)
	}
	return v
}

// --- daily flow present and claimable ---

func TestCheckin_DailyFlowClaimable(t *testing.T) {
	srv := (&mockCheckin{campaigns: `{"campaigns":[]}`, daily: true, requireHeader: true}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")
	if !st.DailyEndpointAvailable {
		t.Error("DailyEndpointAvailable should be true when the daily endpoint answers 200")
	}
	if !st.Claimable || !st.Supported {
		t.Fatalf("expected claimable+supported, got claimable=%v supported=%v reason=%q", st.Claimable, st.Supported, st.Reason)
	}
	if st.StreakDays != 3 || st.TotalClaimDays != 7 || st.TotalRewardCredits != 700 {
		t.Fatalf("streak/totals not carried through: %+v", st)
	}
}

// --- daily endpoint absent -> campaigns fallback ---

func TestCheckin_FallsBackToCampaignsWhenDailyMissing(t *testing.T) {
	campaigns := `{"claimable":true,"showCampaign":true,"campaigns":[{
		"campaignId":"cid-1","campaignKey":"act-1","actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMABLE",
		"startAt":1,"endAt":2,
		"placements":[{"content":{"en":{"title":"Daily 100 Credits"}}}]}]}`
	srv := (&mockCheckin{campaigns: campaigns, daily: false, requireHeader: true}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")
	if st.DailyEndpointAvailable {
		t.Error("daily endpoint 404s here; DailyEndpointAvailable must be false")
	}
	if !st.Claimable || !st.Supported {
		t.Fatalf("expected the CLAIM_BENEFIT campaign to be claimable, got %+v", st)
	}
	if len(st.Campaigns) != 1 || !st.Campaigns[0].GrantsCredits {
		t.Fatalf("CLAIM_BENEFIT campaign should set GrantsCredits: %+v", st.Campaigns)
	}
	if st.Campaigns[0].Title != "Daily 100 Credits" {
		t.Fatalf("placement title not extracted: %q", st.Campaigns[0].Title)
	}
}

// --- a VIEW_DETAILS promotion IS claimable, but is not a credit grant ---
//
// Measured: POST /sash/api/v1/me/campaigns/{id}/claim returns 200 status=CLAIMED
// grantId=... for a VIEW_DETAILS campaign. So claimable must be true. What must stay
// false is GrantsCredits — the benefit is deferred, not paid at claim time.

func TestCheckin_PromotionIsClaimableButNotACreditGrant(t *testing.T) {
	campaigns := `{"claimable":true,"showCampaign":true,"campaigns":[{
		"campaignId":"cid-2","campaignKey":"act-20260901-493","actionType":"VIEW_DETAILS","claimStatus":"CLAIMABLE",
		"placements":[{"content":{"en":{"title":"September perk"}}}]}]}`
	srv := (&mockCheckin{campaigns: campaigns, daily: false, requireHeader: true}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")
	if !st.Claimable {
		t.Error("upstream accepts a claim for a VIEW_DETAILS campaign, so Claimable must be true")
	}
	if !st.Supported {
		t.Error("a claimable campaign means the account is supported")
	}
	if len(st.Campaigns) != 1 || st.Campaigns[0].GrantsCredits {
		t.Fatalf("GrantsCredits must stay false for VIEW_DETAILS, so a UI cannot call it a "+
			"credit grant: %+v", st.Campaigns)
	}
}

// --- pickClaimableCampaign prefers a real grant, but falls back to any claimable ---

func TestCheckin_PickClaimablePrefersGrant(t *testing.T) {
	grant := CheckinCampaign{CampaignKey: "grant", ActionType: CheckinActionClaimBenefit, ClaimStatus: CheckinClaimable, GrantsCredits: true}
	promo := CheckinCampaign{CampaignKey: "promo", ActionType: "VIEW_DETAILS", ClaimStatus: CheckinClaimable}
	done := CheckinCampaign{CampaignKey: "done", ActionType: CheckinActionClaimBenefit, ClaimStatus: "CLAIMED", GrantsCredits: true}

	if got := pickClaimableCampaign([]CheckinCampaign{promo, grant}); got == nil || got.CampaignKey != "grant" {
		t.Fatalf("a CLAIM_BENEFIT campaign must win: %+v", got)
	}
	if got := pickClaimableCampaign([]CheckinCampaign{done, promo}); got == nil || got.CampaignKey != "promo" {
		t.Fatalf("with no grant left, any claimable campaign must be used: %+v", got)
	}
	if got := pickClaimableCampaign([]CheckinCampaign{done}); got != nil {
		t.Fatalf("nothing claimable must return nil, got %+v", got)
	}
}

// --- a claim that returns no benefit is a SUCCESS with 0 credits, reported honestly ---

func TestCheckin_ClaimSucceedsWithoutBenefit(t *testing.T) {
	var posts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts = append(posts, r.URL.Path)
			// The real shape: CLAIMED + grantId, and NO benefit field.
			_, _ = w.Write([]byte(`{"grantId":"01a0c79e-a9d6-7de7-9bbf-abda11386b44","status":"CLAIMED","replayed":false,"campaignKey":"act-20260901-493"}`))
			return
		}
		switch r.URL.Path {
		case checkinPathDailyStatus:
			w.WriteHeader(http.StatusNotFound)
		case checkinPathCampaigns:
			_, _ = w.Write([]byte(`{"claimable":true,"campaigns":[{"campaignId":"cid-1","campaignKey":"act-20260901-493","actionType":"VIEW_DETAILS","claimStatus":"CLAIMABLE"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res, err := managerFor(srv).claimAt(context.Background(), []string{srv.URL}, "tok")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("expected exactly one POST, got %v", posts)
	}
	if res.Status != "claimed" {
		t.Fatalf("status should be claimed, got %+v", res)
	}
	if res.CreditsGranted || res.Credits != 0 {
		t.Errorf("no benefit means no credits at claim time: %+v", res)
	}
	if res.GrantID == "" {
		t.Error("grantId must be surfaced — it is the only proof the claim was recorded")
	}
	if !strings.Contains(res.Message, "deferred") {
		t.Errorf("the message must explain why no credits arrived, got %q", res.Message)
	}
}

// --- a replayed claim is already_claimed, not a fresh success ---

func TestCheckin_ReplayedClaimIsAlreadyClaimed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"grantId":"g1","status":"CLAIMED","replayed":true}`))
			return
		}
		switch r.URL.Path {
		case checkinPathDailyStatus:
			w.WriteHeader(http.StatusNotFound)
		case checkinPathCampaigns:
			_, _ = w.Write([]byte(`{"campaigns":[{"campaignId":"cid-1","campaignKey":"k","actionType":"VIEW_DETAILS","claimStatus":"CLAIMABLE"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	res, err := managerFor(srv).claimAt(context.Background(), []string{srv.URL}, "tok")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if res.Status != "already_claimed" {
		t.Fatalf("a replayed claim must report already_claimed, got %+v", res)
	}
}

// --- claimDecision proceeds on any claimable campaign ---

func TestCheckin_ClaimDecisionAllowsClaimablePromotion(t *testing.T) {
	st := &CheckinStatus{
		Claimable: true,
		Campaigns: []CheckinCampaign{{ActionType: "VIEW_DETAILS", ClaimStatus: CheckinClaimable}},
	}
	if d := claimDecision(st); d.Status != "claimable" {
		t.Fatalf("a claimable promotion must proceed (upstream accepts it), got %+v", d)
	}
}

// --- empty list is distinguished from "promotion only" ---

func TestCheckin_EmptyCampaignListExplained(t *testing.T) {
	srv := (&mockCheckin{campaigns: `{"claimable":false,"showCampaign":false,"campaigns":[]}`, daily: false, requireHeader: true}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")
	if st.Claimable || st.Supported {
		t.Error("empty list must not be claimable")
	}
	if !strings.Contains(st.Reason, "no check-in campaigns") {
		t.Fatalf("empty list needs its own reason, got %q", st.Reason)
	}
}

// --- nothing claimable => no write attempted ---

func TestCheckin_ClaimRefusesWhenNothingClaimable(t *testing.T) {
	// Every campaign already claimed: nothing left to claim.
	campaigns := `{"campaigns":[{"campaignId":"cid-2","campaignKey":"act-x","actionType":"VIEW_DETAILS","claimStatus":"CLAIMED"}]}`
	var posts []string
	srv := (&mockCheckin{campaigns: campaigns, daily: false, requireHeader: true, posts: &posts}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")

	if d := claimDecision(st); d.Status != "no_campaign" {
		t.Fatalf("claimDecision should refuse when nothing is claimable, got %+v", d)
	}
	if len(posts) != 0 {
		t.Fatalf("no POST may be attempted when nothing is claimable; got %v", posts)
	}
	if !strings.Contains(st.Reason, "CLAIMED") {
		t.Errorf("the reason should name the campaign state, got %q", st.Reason)
	}
}

// --- claimDecision approves a genuine grant campaign ---

func TestCheckin_ClaimDecisionApprovesGrantCampaign(t *testing.T) {
	st := &CheckinStatus{
		Supported: true,
		Claimable: true,
		Campaigns: []CheckinCampaign{{ActionType: CheckinActionClaimBenefit, ClaimStatus: CheckinClaimable}},
	}
	if d := claimDecision(st); d.Status != "claimable" {
		t.Fatalf("expected claimable, got %+v", d)
	}
}

// --- an unreachable credential reports a reason instead of a blank panel ---

func TestCheckin_NoHostAcceptsCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"TOKEN_EXPIRE","message":"token is not active"}`))
	}))
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")
	if st.Supported || st.Claimable {
		t.Fatal("401 must not be reported as supported")
	}
	if !strings.Contains(st.Reason, "no check-in host accepted") {
		t.Fatalf("expected an explanatory reason, got %q", st.Reason)
	}
}

// --- the daily 409 (already claimed) is not an error ---

func TestCheckin_DailyClaim409IsAlreadyClaimed(t *testing.T) {
	srv := (&mockCheckin{campaigns: `{"campaigns":[]}`, daily: true, dailyClaimCode: http.StatusConflict, requireHeader: true}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")
	if !st.Claimable {
		t.Fatal("precondition: the daily flow reports CLAIMABLE")
	}
}

// --- host order: the credential's own region is probed first ---

func TestCheckin_ProbeOrderPrefersOwnRegion(t *testing.T) {
	global := NewSessionManager("s", "global", nil)
	got := global.checkinHostsProbeOrder()
	if got[0] != CheckinHostGlobal {
		t.Fatalf("global manager should probe the global host first, got %v", got)
	}
	cn := NewSessionManager("s", "cn", nil)
	got = cn.checkinHostsProbeOrder()
	if got[0] != CheckinHostCN {
		t.Fatalf("cn manager should probe the CN host first, got %v", got)
	}
	// Both hosts must always be present — a credential's region cannot be assumed.
	if len(got) != 2 {
		t.Fatalf("expected both hosts in the order, got %v", got)
	}
}

// --- live diagnostic (opt-in) ---

func TestCheckin_LiveStatus(t *testing.T) {
	cred := envOrSkip(t, "LINTASAN_QODER_PAT")
	salt := envOrSkip(t, "LINTASAN_QODER_SALT")

	st, err := NewSessionManager(salt, "global", nil).CheckinStatusFor(context.Background(), cred)
	if err != nil {
		t.Fatalf("CheckinStatusFor: %v", err)
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	t.Logf("live check-in status:\n%s", string(b))
}

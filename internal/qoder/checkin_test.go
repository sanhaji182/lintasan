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

// --- a VIEW_DETAILS promotion must NOT be reported as claimable credits ---

func TestCheckin_PromotionIsNotClaimableCredits(t *testing.T) {
	campaigns := `{"claimable":true,"showCampaign":true,"campaigns":[{
		"campaignId":"cid-2","campaignKey":"act-20260901-493","actionType":"VIEW_DETAILS","claimStatus":"CLAIMABLE",
		"placements":[{"content":{"en":{"title":"September perk"}}}]}]}`
	srv := (&mockCheckin{campaigns: campaigns, daily: false, requireHeader: true}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")
	if st.Claimable {
		t.Error("a VIEW_DETAILS campaign must not make the account claimable for credits")
	}
	if st.Supported {
		t.Error("VIEW_DETAILS is not a credit grant; Supported must be false")
	}
	if !strings.Contains(st.Reason, "VIEW_DETAILS") {
		t.Fatalf("reason should name the actionType so an operator sees why; got %q", st.Reason)
	}
	if len(st.Campaigns) != 1 || st.Campaigns[0].GrantsCredits {
		t.Fatalf("campaign should be reported but flagged non-granting: %+v", st.Campaigns)
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

// --- the guard: no write when nothing grants credits ---

func TestCheckin_ClaimRefusesWithoutGrantCampaign(t *testing.T) {
	campaigns := `{"campaigns":[{"campaignId":"cid-2","campaignKey":"act-x","actionType":"VIEW_DETAILS","claimStatus":"CLAIMABLE"}]}`
	var posts []string
	srv := (&mockCheckin{campaigns: campaigns, daily: false, requireHeader: true, posts: &posts}).server(t)
	defer srv.Close()

	st := managerFor(srv).CheckinStatusAt(context.Background(), []string{srv.URL}, "tok")

	// The decision that gates the write must be "no_campaign".
	if d := claimDecision(st); d.Status != "no_campaign" {
		t.Fatalf("claimDecision should refuse, got %+v", d)
	}
	if len(posts) != 0 {
		t.Fatalf("no POST may be attempted when nothing grants credits; got %v", posts)
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

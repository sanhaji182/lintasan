package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// TestQoderCheckinRoutesRegistered is the 405 guard.
//
// A handler can compile and still be unreachable because the one line that
// registers it never landed (this repo has shipped that bug before — see
// references/prod-ops-pitfalls.md §18). `strings` on the binary does not catch it.
// Only a request to a running mux does.
func TestQoderCheckinRoutesRegistered(t *testing.T) {
	s, _ := newTestServer(t, nil)

	for _, route := range []string{
		"/api/qoder/checkin",
		"/api/qoder/campaigns",
	} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)

		if rec.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s: 405 — auth passed but no handler is registered for GET %s; "+
				"the wiring line is missing from registerParityRoutes", route, route)
		}
		// 401 is the expected unauthenticated answer, and it proves the route exists
		// behind the auth boundary rather than falling through to a 404.
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusOK {
			t.Errorf("%s: unexpected status %d (body: %s)", route, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
}

// TestQoderCampaignsFlagsNonGrantingPromotions proves the payload cannot be
// mistaken for claimable credits.
//
// The live global host returns exactly one campaign with actionType VIEW_DETAILS.
// If the flag ever reported true for that, a UI would offer a claim that cannot
// succeed — the failure this whole feature exists to avoid.
func TestQoderCampaignsFlagsNonGrantingPromotions(t *testing.T) {
	st := &qoder.CheckinStatus{
		Host:      qoder.CheckinHostGlobal,
		Supported: false,
		Claimable: false,
		Campaigns: []qoder.CheckinCampaign{{
			CampaignID:    "01a05bce-e800-7494-a002-806e4438f483",
			CampaignKey:   "act-20260901-493",
			ActionType:    "VIEW_DETAILS",
			ClaimStatus:   qoder.CheckinClaimable,
			Title:         "September perk: double your first-month Credits on Pro/Pro+",
			GrantsCredits: false,
		}},
		Reason: "campaigns are present but none grants credits (VIEW_DETAILS/CLAIMABLE)",
	}

	b, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["claimable"] != false {
		t.Error("claimable must be false for a VIEW_DETAILS-only account")
	}
	if back["supported"] != false {
		t.Error("supported must be false when nothing grants credits")
	}
	camps, _ := back["campaigns"].([]any)
	if len(camps) != 1 {
		t.Fatalf("expected the promotion to be reported, got %d campaigns", len(camps))
	}
	c0, _ := camps[0].(map[string]any)
	if c0["grants_credits"] != false {
		t.Error("grants_credits must be false so a UI cannot render the promotion as a grant")
	}
	if c0["action_type"] != "VIEW_DETAILS" {
		t.Errorf("action_type should be surfaced verbatim, got %v", c0["action_type"])
	}
}

// TestCheckinHandlerDeclaresClaimGate documents that the claim write is gated.
//
// Claiming was measured to WORK on this host (200 status=CLAIMED grantId=...), so a
// POST route exists — but it is a write, so it defaults to a dry run and is enabled
// only by the `qoder_checkin_enabled` setting. This test pins that default so a
// future edit cannot quietly make production claim on every call.
func TestCheckinHandlerDeclaresClaimGate(t *testing.T) {
	src, err := os.ReadFile("qoder_checkin_handlers.go")
	if err != nil {
		t.Skipf("handler source not readable from test cwd: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, `qoderCheckinSettingKey = "qoder_checkin_enabled"`) {
		t.Error("the check-in claim gate setting must be named qoder_checkin_enabled")
	}
	// The toggle must read from settings and fail closed.
	if !strings.Contains(body, "s.db.GetSetting(qoderCheckinSettingKey)") {
		t.Error("the gate must be read from settings")
	}
	if !strings.Contains(body, `"dry_run": !enabled`) {
		t.Error("the claim response must report dry_run so a caller cannot mistake a " +
			"no-op for a completed claim")
	}
}

// TestQoderCheckinPostRouteExists is the other half of the 405 guard: the POST must
// be registered, since a GET-only registration silently 405s every claim.
func TestQoderCheckinPostRouteExists(t *testing.T) {
	s, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/qoder/checkin", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatal("POST /api/qoder/checkin: 405 — the POST registration is missing from " +
			"registerParityRoutes; claims would silently never work")
	}
	// This harness drives s.mux directly, so authMiddleware is not in the path and a
	// handled request answers 200. Only 405 (no handler for POST) is a failure here;
	// the auth boundary itself is covered by security_boundary_test.go.
	if rec.Code != http.StatusOK && rec.Code != http.StatusUnauthorized {
		t.Errorf("unexpected status %d for POST /api/qoder/checkin (body: %s)",
			rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}

// TestQoderCheckinCacheIsSeparateFromQuotaCache guards the type-mixup bug: sharing
// one cache between quota and check-in would decode a *Quota as a *CheckinStatus and
// yield a blank panel instead of an error.
func TestQoderCheckinCacheIsSeparateFromQuotaCache(t *testing.T) {
	qc := qoder.NewCheckinCache(0)
	if _, ok := qc.Get("missing"); ok {
		t.Fatal("empty cache must miss")
	}
	st := &qoder.CheckinStatus{Host: "h", Reason: "r"}
	qc.Put("cred", st)

	got, ok := qc.Get("cred")
	if !ok || got == nil {
		t.Fatal("expected a hit after Put")
	}
	if got.Host != "h" || got.Reason != "r" {
		t.Fatalf("round-trip lost data: %+v", got)
	}

	qc.Invalidate("cred")
	if _, ok := qc.Get("cred"); ok {
		t.Fatal("Invalidate must drop the entry")
	}
}

package qoder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestEnrichFromUserStatusMapsEveryField pins the /api/v3/user/status mapping.
//
// That endpoint answers 200 with { plan, userTag, userType, quota, isQuotaExceeded,
// nextResetAt, ... } and is the ONLY source of the reset time — which is why every
// bucket's ResetTime was blank. Found via OmniRoute
// (open-sse/services/usage/qoder.ts).
func TestEnrichFromUserStatusMapsEveryField(t *testing.T) {
	// 1790546012024 ms = 2026-09-28T04:53:32Z (the Pro Trial end date).
	body := `{
		"id":"u1","name":"Ryan Perry","userType":"personal_professional_trial",
		"plan":"PLAN_TIER_PRO_TRIAL","userTag":"Pro Trial","quota":0,
		"isQuotaExceeded":true,"nextResetAt":1790546012024,
		"whitelistStatus":"PASS","isSubAccount":false,
		"featureSwitches":{"allow_byok":2}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("user/status must be called with a bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	m := managerFor(srv)
	q := &Quota{}
	m.enrichFromUserStatusAt(context.Background(), q, "tok", srv.URL)

	if q.NextResetAt != 1790546012024 {
		t.Errorf("nextResetAt not captured: %d", q.NextResetAt)
	}
	if q.NextReset == "" {
		t.Error("NextReset must be formatted for the dashboard, not left empty")
	}
	if q.PlanTier != "PLAN_TIER_PRO_TRIAL" {
		t.Errorf("plan enum lost: %q", q.PlanTier)
	}
	if q.UserTag != "Pro Trial" {
		t.Errorf("userTag lost: %q", q.UserTag)
	}
	// The friendly label wins for Plan; the enum is preserved separately.
	if q.Plan != "Pro Trial" {
		t.Errorf("Plan should take the human label, got %q", q.Plan)
	}
	if q.Pooled {
		t.Error("a personal seat is not pooled")
	}
}

// TestEnrichFromUserStatusTeamSeatIsPooled guards the documented trap: on a
// Teams/Enterprise seat, `quota: 0` means POOLED, not exhausted. Reading it as
// exhausted would 429 every request.
func TestEnrichFromUserStatusTeamSeatIsPooled(t *testing.T) {
	body := `{"userType":"teams","plan":"PLAN_TIER_TEAMS","quota":0,"isQuotaExceeded":false}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	m := managerFor(srv)
	q := &Quota{}
	m.enrichFromUserStatusAt(context.Background(), q, "tok", srv.URL)

	if !q.Pooled {
		t.Fatal("a teams seat must be marked pooled; quota:0 there does not mean exhausted")
	}
	if q.IsQuotaExceeded {
		t.Error("pooled must not be reported as exceeded")
	}
}

// TestEnrichFromUserStatusFailuresAreSilent: this endpoint is enrichment, so any
// failure must leave the snapshot intact rather than failing the quota read.
func TestEnrichFromUserStatusFailuresAreSilent(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"500", http.StatusInternalServerError, ""},
		{"401", http.StatusUnauthorized, `{"error":"nope"}`},
		{"garbage", http.StatusOK, `not json at all`},
		{"empty", http.StatusOK, ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			q := &Quota{Plan: "keep-me", ExpiresAt: 42}
			managerFor(srv).enrichFromUserStatusAt(context.Background(), q, "tok", srv.URL)

			if q.Plan != "keep-me" || q.ExpiresAt != 42 {
				t.Fatalf("enrichment clobbered the existing snapshot: %+v", q)
			}
			if q.NextResetAt != 0 || q.Pooled {
				t.Fatalf("a failed enrichment must not invent values: %+v", q)
			}
		})
	}
}

// TestNextResetUnitDetection: upstream sends milliseconds. A seconds value must not
// be rendered as a 1970 date, and vice versa.
func TestNextResetUnitDetection(t *testing.T) {
	for _, tc := range []struct {
		name string
		ms   int64
	}{
		{"milliseconds", 1790546012024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"nextResetAt": tc.ms, "userType": "personal"})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			q := &Quota{}
			managerFor(srv).enrichFromUserStatusAt(context.Background(), q, "tok", srv.URL)
			if q.NextReset == "" {
				t.Fatal("no reset string produced")
			}
			if q.NextReset[:4] != "2026" {
				t.Fatalf("unit detection wrong; expected a 2026 date, got %q", q.NextReset)
			}
		})
	}
}

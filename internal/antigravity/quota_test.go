package antigravity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseQuotaSummary(t *testing.T) {
	raw := &RawQuotaResponse{
		Groups: []QuotaGroup{
			{
				DisplayName: "Gemini Models",
				Description: "Models within this group: Gemini Flash, Gemini Pro",
				Buckets: []QuotaBucket{
					{
						BucketID:          "gemini-weekly",
						DisplayName:       "Weekly Limit Remaining",
						Window:            "weekly",
						ResetTime:         "2026-10-26T02:36:51Z",
						Description:       "Weekly refresh",
						RemainingFraction: 0.8559182,
					},
					{
						BucketID:          "gemini-5h",
						DisplayName:       "Five Hour Limit Remaining",
						Window:            "5h",
						ResetTime:         "2026-10-21T09:32:11Z",
						Description:       "5-hour refresh",
						RemainingFraction: 0.9999188,
					},
				},
			},
			{
				DisplayName: "Claude and GPT models",
				Description: "Models within this group: Claude Opus, Claude Sonnet",
				Buckets: []QuotaBucket{
					{
						BucketID:          "3p-weekly",
						DisplayName:       "Weekly Limit Remaining",
						Window:            "weekly",
						ResetTime:         "2026-10-28T04:32:26Z",
						RemainingFraction: 0.9998672,
					},
					{
						BucketID:          "3p-5h",
						DisplayName:       "Five Hour Limit Remaining",
						Window:            "5h",
						ResetTime:         "2026-10-21T09:32:26Z",
						RemainingFraction: 0.9997344,
					},
				},
			},
		},
	}

	res := ParseQuotaSummary(raw)
	if res == nil {
		t.Fatal("expected non-nil result")
	}

	if len(res.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(res.Groups))
	}

	geminiWeekly, ok := res.Quotas["gemini_weekly"]
	if !ok {
		t.Fatal("expected gemini_weekly in quotas")
	}
	if geminiWeekly.RemainingPercentage != 85.59 {
		t.Fatalf("expected 85.59%%, got %f", geminiWeekly.RemainingPercentage)
	}
	if geminiWeekly.Used != 144 {
		t.Fatalf("expected 144 used, got %d", geminiWeekly.Used)
	}

	geminiSession, ok := res.Quotas["gemini_session"]
	if !ok {
		t.Fatal("expected gemini_session in quotas")
	}
	if geminiSession.RemainingPercentage != 99.99 {
		t.Fatalf("expected 99.99%%, got %f", geminiSession.RemainingPercentage)
	}

	claudeWeekly, ok := res.Quotas["claude_gpt_weekly"]
	if !ok {
		t.Fatal("expected claude_gpt_weekly in quotas")
	}
	if claudeWeekly.RemainingPercentage != 99.99 {
		t.Fatalf("expected 99.99%%, got %f", claudeWeekly.RemainingPercentage)
	}
}

func TestFetchQuotaLiveMock(t *testing.T) {
	ClearQuotaCache()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Client-Name") != "antigravity" {
			http.Error(w, "missing header", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"groups": [
				{
					"displayName": "Gemini Models",
					"buckets": [
						{
							"bucketId": "gemini-weekly",
							"displayName": "Weekly Limit Remaining",
							"window": "weekly",
							"resetTime": "2026-10-26T02:36:51Z",
							"remainingFraction": 0.85
						}
					]
				}
			]
		}`))
	}))
	defer srv.Close()

	oldURL := AntigravityQuotaEndpoint
	AntigravityQuotaEndpoint = srv.URL
	defer func() { AntigravityQuotaEndpoint = oldURL }()

	res, err := FetchQuota(context.Background(), srv.Client(), "test-token", true)
	if err != nil {
		t.Fatalf("FetchQuota failed: %v", err)
	}

	if res.Quotas["gemini_weekly"].RemainingPercentage != 85.0 {
		t.Fatalf("expected 85.0, got %f", res.Quotas["gemini_weekly"].RemainingPercentage)
	}

	// Verify caching
	res2, err := FetchQuota(context.Background(), srv.Client(), "test-token", false)
	if err != nil {
		t.Fatalf("FetchQuota cache read failed: %v", err)
	}
	if res2.Quotas["gemini_weekly"].RemainingPercentage != 85.0 {
		t.Fatalf("expected 85.0 from cache, got %f", res2.Quotas["gemini_weekly"].RemainingPercentage)
	}
}

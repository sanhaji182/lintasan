package qoder

import (
	"encoding/json"
	"testing"
)

// TestParseQuotaKeepsEveryReportedField is a regression guard.
//
// The Quota struct used to drop `outerProviders`, `totalUsagePercentage`,
// `usageType`, `userType` and `upgradeUrl` because nothing read them. A dropped
// field cannot be noticed when upstream starts using it — and `outerProviders` is
// exactly where per-provider bonus quota would surface. This test pins that a field
// present in the payload survives the round trip.
func TestParseQuotaKeepsEveryReportedField(t *testing.T) {
	raw := []byte(`{
		"userId":"u1",
		"userType":"personal_professional_trial",
		"usageType":"credits",
		"totalUsagePercentage":0.25,
		"isQuotaExceeded":false,
		"expiresAt":1790546012024,
		"upgradeUrl":"https://qoder.com/pricing?client=qoder",
		"isPlanQuotaProrated":false,
		"outerProviders":[{"provider":"qwen","total":1000,"remaining":800}],
		"userQuota":{"total":300,"used":75,"remaining":225,"percentage":0.25,"unit":"credits"}
	}`)

	q, err := parseQuota(raw)
	if err != nil {
		t.Fatalf("parseQuota: %v", err)
	}

	if q.AccountType != "personal_professional_trial" {
		t.Errorf("userType dropped: %q", q.AccountType)
	}
	if q.UsageType != "credits" {
		t.Errorf("usageType dropped: %q", q.UsageType)
	}
	if q.TotalUsagePercentage != 0.25 {
		t.Errorf("totalUsagePercentage dropped: %v", q.TotalUsagePercentage)
	}
	if q.UpgradeURL == "" {
		t.Error("upgradeUrl dropped; the dashboard cannot offer an upgrade path without it")
	}
	if q.ExpiresAt != 1790546012024 {
		t.Errorf("expiresAt wrong: %d", q.ExpiresAt)
	}

	// outerProviders must survive VERBATIM — shape unverified, so nothing may be
	// parsed out of it.
	if len(q.OuterProviders) == 0 {
		t.Fatal("outerProviders dropped; per-provider bonus quota would be invisible")
	}
	var ops []map[string]any
	if err := json.Unmarshal(q.OuterProviders, &ops); err != nil {
		t.Fatalf("outerProviders is not valid JSON: %v (%s)", err, q.OuterProviders)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 outer provider, got %d", len(ops))
	}
	// Every key upstream sent must still be present.
	for _, k := range []string{"provider", "total", "remaining"} {
		if _, ok := ops[0][k]; !ok {
			t.Errorf("outerProviders entry lost key %q (kept: %v)", k, ops[0])
		}
	}

	// The buckets we already understood must be unaffected.
	if q.UserQuota == nil || q.TotalRemaining() != 225 || q.TotalAllocation() != 300 {
		t.Fatalf("bucket arithmetic regressed: %+v", q.UserQuota)
	}
}

// TestParseQuotaEmptyOuterProviders covers the observed state: an empty array must
// not be turned into an error, and must not appear as a bogus entry.
func TestParseQuotaEmptyOuterProviders(t *testing.T) {
	raw := []byte(`{"isQuotaExceeded":true,"userQuota":{"total":300,"used":300,"remaining":0},"outerProviders":[]}`)
	q, err := parseQuota(raw)
	if err != nil {
		t.Fatalf("parseQuota: %v", err)
	}
	if len(q.OuterProviders) == 0 {
		t.Fatal("an empty array should still round-trip as []")
	}
	var ops []any
	if err := json.Unmarshal(q.OuterProviders, &ops); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("expected an empty list, got %v", ops)
	}
	if !q.IsQuotaExceeded {
		t.Error("isQuotaExceeded lost")
	}
}

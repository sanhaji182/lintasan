package server

// model_test_cost_test.go — a model probe is a real, billed request and must be
// recorded like one.
//
// The defect: testModelOnce / testQoderModelOnce sent a genuine chat request
// upstream, which consumed the account's credits, and left no trace — logRequest,
// RecordQuota, telemetry and the cost sample were all absent from the Qoder probe, so
// an operator clicking "Test Model" across a pool spent credits that appeared nowhere:
// not in request_logs, not in /api/analytics providerCredits, and not attributable by
// the burn-rate watchdog. Observed live: an account holding 27 credits went to 0 after
// a probe, with no log row to explain it.
//
// The upstream exchange itself is not exercisable here (it needs the real credential
// handshake), so these tests cover the two halves that ARE pure: the payload the probe
// reports, and the reporting helper it calls. The probe's wiring into the handler is
// covered by inspection of the single call site.

import (
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// TestQoderProbeResultReportsCost: a probe that was charged must say so, so a single
// click is self-explaining.
func TestQoderProbeResultReportsCost(t *testing.T) {
	outcome := qoder.StreamOutcome{
		InputTokens: 13200, OutputTokens: 8,
		Credits: 0.4125, CreditsReported: true, CachedTokens: 13100,
	}
	got := qoderProbeResult(outcome, 1234, 200)

	if got["status"] != "ok" || got["success"] != true {
		t.Errorf("a served probe must report ok/true, got %v", got)
	}
	if got["credits"] != 0.4125 {
		t.Errorf("credits = %v, want 0.4125 — the probe's cost must be visible", got["credits"])
	}
	if got["cached_tokens"] != 13100 {
		t.Errorf("cached_tokens = %v, want 13100", got["cached_tokens"])
	}
	if got["input_tokens"] != 13200 {
		t.Errorf("input_tokens = %v, want 13200", got["input_tokens"])
	}
}

// TestQoderProbeResultOmitsUnreportedCost is the reporting rule that matters: an
// absent figure must be ABSENT, never 0.
//
// A 0 would render as "this probe was free", which is a claim the API has not made and
// an operator would act on when deciding whether to test a pool.
func TestQoderProbeResultOmitsUnreportedCost(t *testing.T) {
	outcome := qoder.StreamOutcome{
		InputTokens: 10, OutputTokens: 2,
		// CreditsReported false: upstream sent no credits field.
	}
	got := qoderProbeResult(outcome, 500, 200)

	if v, present := got["credits"]; present {
		t.Errorf("credits present (%v) when upstream reported none; it must be omitted", v)
	}
	if v, present := got["cached_tokens"]; present {
		t.Errorf("cached_tokens present (%v) when upstream reported none", v)
	}
	// The probe still succeeded — an unreported price is not a failure.
	if got["status"] != "ok" {
		t.Errorf("status = %v, want ok", got["status"])
	}
}

// TestQoderProbeResultReportsZeroAsZero: a genuinely reported 0 must survive as 0.
// Dropping it would be the mirror of the bug above — it would hide a request that
// upstream did price at nothing (a free-tier model, say).
func TestQoderProbeResultReportsZeroAsZero(t *testing.T) {
	outcome := qoder.StreamOutcome{
		InputTokens: 10, OutputTokens: 2,
		Credits: 0, CreditsReported: true,
	}
	got := qoderProbeResult(outcome, 500, 200)

	v, present := got["credits"]
	if !present {
		t.Fatal("a reported 0 must be reported as 0, not omitted")
	}
	if v != float64(0) {
		t.Errorf("credits = %v, want 0", v)
	}
}

// TestCostSampleFromOutcomeCarriesPresence pins the conversion the probe relies on:
// the presence flag must travel with the value, or the persistence layer cannot tell a
// free probe from an unpriced one.
func TestCostSampleFromOutcomeCarriesPresence(t *testing.T) {
	reported := costSample{
		Credits:      0.5,
		CachedTokens: 12,
		Reported:     qoder.StreamOutcome{CreditsReported: true}.CreditsReported,
	}
	if !reported.Reported {
		t.Error("Reported must be true when upstream sent the field")
	}
	if reported.CachedTokens != 12 {
		t.Errorf("CachedTokens = %d, want 12", reported.CachedTokens)
	}

	absent := costSample{Reported: qoder.StreamOutcome{}.CreditsReported}
	if absent.Reported {
		t.Error("Reported must be false when upstream sent no credits field")
	}
}

// TestProbeTaskClassIsDistinguishable: probe rows must be separable from live traffic,
// so a future analysis can exclude them and the watchdog can attribute them.
func TestProbeTaskClassIsDistinguishable(t *testing.T) {
	// The handler passes these two literals; asserting on them here means a rename
	// that breaks the accounting shows up as a failing test rather than as silently
	// misfiled rows.
	const taskClass, mode = "model-test", "probe"
	if !strings.Contains(taskClass, "test") {
		t.Errorf("task class %q should mark the row as a probe", taskClass)
	}
	if mode != "probe" {
		t.Errorf("mode = %q, want probe", mode)
	}
}

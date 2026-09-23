package server

// log_request_cost_test.go — logRequestCost must persist what it is told.
//
// Written because a live probe logged status 200 for a request the handler had
// classified as a refusal, and the cause had to be isolated. These tests separate the
// persistence layer from the handler so a status/argument mistake cannot hide behind a
// live upstream's behaviour.

import (
	"database/sql"
	"testing"
)

type loggedRow struct {
	model, provider, err, taskClass string
	status                          int
	credits, cachedTokens           *float64
	cachedTokensInt                 *int
}

// lastLogFor returns the most recent request_logs row for a connection.
func lastLogFor(t *testing.T, h *ProxyHandler, connID string) loggedRow {
	t.Helper()
	var r loggedRow
	var errMsg sql.NullString
	var credits sql.NullFloat64
	var cachedTok sql.NullInt64
	row := h.db.Conn().QueryRow(`
		SELECT model, provider, status, COALESCE(error,''), credits, cached_tokens
		FROM request_logs WHERE connection_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1`, connID)
	if err := row.Scan(&r.model, &r.provider, &r.status, &errMsg, &credits, &cachedTok); err != nil {
		t.Fatalf("no request_logs row for %s: %v", connID, err)
	}
	r.err = errMsg.String
	if credits.Valid {
		v := credits.Float64
		r.credits = &v
	}
	if cachedTok.Valid {
		v := int(cachedTok.Int64)
		r.cachedTokensInt = &v
	}
	return r
}

// TestLogRequestCostPersistsStatusVerbatim is the isolation test: whatever status the
// caller passes is what lands. A 502 stays 502.
func TestLogRequestCostPersistsStatusVerbatim(t *testing.T) {
	h := newTestProxyHandler(t)

	h.logRequestCost("qfmodel", "c-log", "Qoder probe", 502, 373, 0, 0, false,
		"upstream refused this credential for this attempt (code 105)", "model-test", "probe", costSample{})

	r := lastLogFor(t, h, "c-log")
	if r.status != 502 {
		t.Errorf("status = %d, want 502 — the caller's verdict must survive", r.status)
	}
	if r.model != "qfmodel" || r.provider != "Qoder probe" {
		t.Errorf("row identity wrong: model=%q provider=%q", r.model, r.provider)
	}
	// A failed attempt must not claim a charge.
	if r.credits != nil {
		t.Errorf("credits = %v on a failed attempt, want NULL", *r.credits)
	}
}

// TestLogRequestCostPersistsReportedCharge covers the success shape.
func TestLogRequestCostPersistsReportedCharge(t *testing.T) {
	h := newTestProxyHandler(t)

	h.logRequestCost("qmodel_38max", "c-cost", "Qoder", 200, 1924, 27, 109, false,
		"", "model-test", "probe",
		costSample{Credits: 0.409008, CachedTokens: 13188, Reported: true})

	r := lastLogFor(t, h, "c-cost")
	if r.status != 200 {
		t.Errorf("status = %d, want 200", r.status)
	}
	if r.credits == nil || *r.credits != 0.409008 {
		t.Errorf("credits = %v, want 0.409008", r.credits)
	}
	if r.cachedTokensInt == nil || *r.cachedTokensInt != 13188 {
		t.Errorf("cached_tokens = %v, want 13188", r.cachedTokensInt)
	}
}

// TestLogRequestCostLeavesUnreportedFieldsNull: the NULL-not-zero rule, enforced at the
// persistence layer rather than trusted to every caller.
func TestLogRequestCostLeavesUnreportedFieldsNull(t *testing.T) {
	h := newTestProxyHandler(t)

	h.logRequestCost("m", "c-null", "prov", 200, 10, 0, 0, false, "", "", "", costSample{})

	r := lastLogFor(t, h, "c-null")
	if r.credits != nil {
		t.Errorf("credits = %v, want NULL when nothing was reported", *r.credits)
	}
	if r.cachedTokensInt != nil {
		t.Errorf("cached_tokens = %v, want NULL when nothing was reported", *r.cachedTokensInt)
	}
}

// TestLogRequestCostKeepsZeroChargeAsZero: a reported 0 is a fact, not an absence.
func TestLogRequestCostKeepsZeroChargeAsZero(t *testing.T) {
	h := newTestProxyHandler(t)

	h.logRequestCost("m", "c-zero", "prov", 200, 10, 5, 5, false, "", "", "",
		costSample{Credits: 0, CachedTokens: 0, Reported: true})

	r := lastLogFor(t, h, "c-zero")
	if r.credits == nil {
		t.Fatal("a reported 0 must persist as 0, not NULL")
	}
	if *r.credits != 0 {
		t.Errorf("credits = %v, want 0", *r.credits)
	}
}

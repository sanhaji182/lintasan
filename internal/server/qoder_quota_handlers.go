package server

// qoder_quota_handlers.go — Qoder credit visibility and model probing.
//
// Two dashboard-facing gaps this closes:
//
//  1. Model test. The generic probe posts an OpenAI-shaped body to the
//     connection's chat path. Qoder needs its own protocol, so a Qoder connection
//     tested through the generic path reports a misleading upstream error. The
//     probe below drives the real provider instead, so "test" means the same
//     thing for Qoder as for every other provider.
//
//  2. Credits. Qoder accounts have a credit allocation that is invisible from
//     Lintasan: the gateway's own /api/quota reports only its internal counters.
//     Without this, an operator running a Qoder pool cannot see how much headroom
//     it has without opening the vendor console per account. The handler below
//     reads the real figure per connection.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// qoderProbeMaxTokens keeps a model probe cheap. A probe only needs to prove the
// model answers, not to generate anything.
const qoderProbeMaxTokens = 16

// testQoderModelOnce probes one Qoder model through the provider protocol.
//
// It deliberately distinguishes the failure modes an operator needs to tell apart:
// a dead credential (transient, 105), a busy model (queued), a stalled stream, and
// a genuine protocol error. Collapsing them into "test failed" would make the pool
// unmanageable, because the correct response differs for each.
func (s *Server) testQoderModelOnce(conn *Connection, modelID string, start time.Time) map[string]any {
	provider := s.proxy.qoderProvider
	if provider == nil {
		return map[string]any{
			"success": false, "status": "not_enabled",
			"message": "the Qoder provider is not active; enable qoder_enabled and provision the request template",
		}
	}
	if strings.TrimSpace(conn.APIKey) == "" {
		return map[string]any{"success": false, "status": "auth_error", "message": "connection has no credential"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// logAttempt records a probe that did not succeed.
	//
	// A refused probe is the case an operator most needs to see: it means the account
	// is in cooldown, and without a row the only evidence is the absence of one.
	// Recorded with the same task class as a successful probe so both are separable
	// from live traffic.
	//
	// The stored status is 502, not the upstream's 200, for the in-stream case. Qoder
	// reports a refusal INSIDE a 200, so persisting 200 would make a refused probe
	// indistinguishable from a served one in `request_logs` — the two rows would
	// differ only in the error text, which is exactly the kind of "looks fine"
	// accounting this file has already been burned by. The real upstream status is
	// preserved in the message.
	logAttempt := func(status int, errMsg string) {
		s.proxy.logRequestCost(modelID, conn.ID, conn.Name, status, time.Since(start).Milliseconds(), 0, 0, false, errMsg, "model-test", "probe", costSample{})
	}

	body, err := qoder.BuildChatBody(qoder.ChatRequest{
		Model:     modelID,
		Messages:  []map[string]any{{"role": "user", "content": "ping"}},
		Stream:    true,
		MaxTokens: qoderProbeMaxTokens,
	})
	if err != nil {
		logAttempt(http.StatusBadRequest, err.Error())
		return map[string]any{"success": false, "status": "upstream_error", "message": err.Error()}
	}

	resp, err := provider.Sessions().StartChatStream(ctx, conn.APIKey, modelID, body, "")
	if err != nil {
		logAttempt(http.StatusBadGateway, err.Error())
		return qoderProbeFailure(err, time.Since(start).Milliseconds())
	}
	defer resp.Body.Close()

	var sawContent bool
	// Timeouts come from the handler (operator-settable), not the provider.
	outcome, err := qoder.ConsumeStreamWithIdleTimeout(ctx, resp.Body, modelID,
		s.proxy.FirstByteTimeout(), s.proxy.IdleTimeout(), func(d qoder.StreamDelta) error {
			if d.Content != "" || d.Reasoning != "" || len(d.ToolCalls) > 0 {
				sawContent = true
			}
			return nil
		})
	latency := time.Since(start).Milliseconds()

	if err != nil {
		msg, kind, _ := qoder.DescribeStreamError(err)
		// An upstream business code is what makes this classifiable; a transport error
		// has none and must not clear or set a flag.
		code := ""
		if ue, ok := err.(*qoder.UpstreamError); ok {
			code = ue.Code
		}
		s.recordAccountErrorFlag(conn.ID, code, modelID, msg)
		logAttempt(http.StatusBadGateway, msg)
		fail := qoderProbeFailure(err, latency)
		fail["http_status"] = resp.StatusCode
		// Surfaced so the dashboard can show the flag without a second lookup.
		if kind != "" {
			fail["error_kind"] = kind
		}
		return fail
	}
	if !sawContent {
		logAttempt(http.StatusBadGateway, "upstream accepted the request but produced no content within the idle window")
		return map[string]any{
			"success": false, "status": "empty_stream", "latency_ms": latency,
			"http_status": resp.StatusCode,
			"message":     "upstream accepted the request but produced no content within the idle window",
		}
	}

	// Served: the account is working for this model, so any earlier flag is stale.
	s.clearAccountErrorFlag(conn.ID)

	// A model test is a REAL request that consumes the account's credits, so it is
	// recorded like any other turn. It previously left no trace: not in
	// request_logs, not in /api/analytics, not in providerCredits — while the
	// upstream charge landed on the account all the same. An operator clicking
	// "Test Model" across a pool spent credits they could not see, and the burn-rate
	// watchdog could not attribute it either.
	//
	// The status is 200 because that is what the probe establishes: the model
	// answered. `credits` is the upstream-reported charge for the probe.
	cost := costSample{
		Credits:      outcome.Credits,
		CachedTokens: outcome.CachedTokens,
		Reported:     outcome.CreditsReported,
	}
	s.proxy.logRequestCost(modelID, conn.ID, conn.Name, resp.StatusCode, latency, outcome.InputTokens, outcome.OutputTokens, false, "", "model-test", "probe", cost)

	return qoderProbeResult(outcome, latency, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Account error flags
// ---------------------------------------------------------------------------

// recordAccountErrorFlag persists the most recent upstream-code refusal for a
// connection, plus a counter for credit-limit refusals.
//
// INFORMATIONAL ONLY. Nothing reads these to route around, skip, or disable an
// account, and that is a deliberate design decision rather than an omission:
//
//   - Measured 2026-09-23: an account at used=300/300 with isQuotaExceeded=true still
//     served basic models (HTTP 200, real content, ~400 ms) while premium models
//     returned code 112. A credit-limited account is scoped-down, not dead.
//   - Auto-skipping on this flag would therefore remove working capacity from the pool,
//     which is the same mistake `Quota.IsQuotaExceeded` is deliberately not used for.
//
// The operator sees the flag and decides, using the existing enable/disable control.
func (s *Server) recordAccountErrorFlag(connID, code, model, errMsg string) {
	if connID == "" || code == "" {
		return
	}
	hits := 0
	if code == qoder.CreditLimitCode {
		hits = 1
	}
	s.db.Conn().Exec(`
		UPDATE connections
		SET last_error_code = ?, last_error_at = datetime('now','localtime'),
		    last_error_model = ?, credit_limit_hits = credit_limit_hits + ?
		WHERE id = ?`, code, model, hits, connID)
}

// clearAccountErrorFlag removes the flag after a request the account served.
//
// A stale flag is worse than no flag: it would tell an operator to look at an account
// that has since recovered. Clearing on success is what makes the flag mean "most recent
// state" rather than "has ever failed".
func (s *Server) clearAccountErrorFlag(connID string) {
	if connID == "" {
		return
	}
	s.db.Conn().Exec(`
		UPDATE connections
		SET last_error_code = NULL, last_error_model = NULL
		WHERE id = ? AND last_error_code IS NOT NULL`, connID)
}

// qoderProbeResult builds the success payload for a model probe.
//
// Extracted from the handler so the reported-shape rules are testable without a live
// credential exchange: `credits` is an upstream figure and is OMITTED when upstream
// reported none, rather than sent as 0, which an operator would read as a free probe.
func qoderProbeResult(outcome qoder.StreamOutcome, latency int64, httpStatus int) map[string]any {
	out := map[string]any{
		"success": true, "status": "ok", "latency_ms": latency,
		"http_status": httpStatus, "message": "model responds",
		"input_tokens": outcome.InputTokens, "output_tokens": outcome.OutputTokens,
	}
	if outcome.CreditsReported {
		out["credits"] = outcome.Credits
		out["cached_tokens"] = outcome.CachedTokens
	}
	return out
}

// qoderProbeFailure maps a typed Qoder failure onto the model-test status vocabulary
// the dashboard already renders.
func qoderProbeFailure(err error, latency int64) map[string]any {
	msg, kind, retryable := qoder.DescribeStreamError(err)
	status := "upstream_error"
	switch kind {
	case "credential_cooldown":
		// Deliberately NOT "auth_error": the dashboard would show it as a broken
		// credential and an operator would replace a working one. It is transient
		// and clears on retry.
		status = "rate_limited"
	case "upstream_queue", "upstream_stalled":
		status = "rate_limited"
	}
	return map[string]any{
		"success": false, "status": status, "latency_ms": latency,
		"message": msg, "retryable": retryable, "error_kind": kind,
	}
}

// ---------------------------------------------------------------------------
// Credits
// ---------------------------------------------------------------------------

// quotaCache is process-wide. A dashboard refresh must not re-authenticate every
// account on every poll, and credit state is not worth a fresh exchange per view.
var quotaCache = qoder.NewQuotaCache(5 * time.Minute)

// handleQoderQuota reports credit state for Qoder connections.
//
// GET /api/qoder/quota                 — every active Qoder connection
// GET /api/qoder/quota/{connection_id} — one connection
//
// Rows are keyed by connection id and include the connection's name and priority so
// the response is readable on its own, without a second lookup.
func (s *Server) handleQoderQuota(w http.ResponseWriter, r *http.Request) {
	if s.proxy.qoderProvider == nil {
		writeJSON(w, map[string]any{
			"success": false, "status": "not_enabled",
			"message": "the Qoder provider is not active",
			"data":    []any{},
		})
		return
	}

	one := strings.TrimSpace(r.PathValue("connection_id"))

	rows, err := s.db.Conn().Query(`
		SELECT id, name, api_key, priority, models_count, is_active,
		       COALESCE(last_error_code,''), COALESCE(last_error_at,''),
		       COALESCE(last_error_model,''), credit_limit_hits
		FROM connections
		WHERE LOWER(format) = 'qoder' AND is_active = 1
		ORDER BY priority DESC`)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}
	defer rows.Close()

	type row struct {
		id, name, key                        string
		priority, models, active, creditHits int
		lastCode, lastAt, lastModel          string
	}
	var want []row
	for rows.Next() {
		var it row
		if err := rows.Scan(&it.id, &it.name, &it.key, &it.priority, &it.models, &it.active,
			&it.lastCode, &it.lastAt, &it.lastModel, &it.creditHits); err != nil {
			continue
		}
		if one != "" && it.id != one {
			continue
		}
		want = append(want, it)
	}

	// Force a re-read for an explicit single-connection refresh, so the dashboard's
	// refresh button does something rather than replaying the cache.
	if one != "" && r.URL.Query().Get("refresh") == "1" {
		for _, it := range want {
			quotaCache.Invalidate(it.key)
		}
	}

	sessions := s.proxy.qoderProvider.Sessions()

	type entry struct {
		ConnectionID string       `json:"connection_id"`
		Name         string       `json:"name"`
		Priority     int          `json:"priority"`
		ModelsCount  int          `json:"models_count"`
		Quota        *qoder.Quota `json:"quota,omitempty"`
		Error        string       `json:"error,omitempty"`
		// Diagnostic flag: the most recent upstream-code refusal for this account.
		// Informational — the dashboard shows it beside the enable/disable control so
		// the decision to disable stays with the operator. Nothing here routes on it.
		LastErrorCode   string `json:"last_error_code,omitempty"`
		LastErrorAt     string `json:"last_error_at,omitempty"`
		LastErrorModel  string `json:"last_error_model,omitempty"`
		CreditLimitHits int    `json:"credit_limit_hits,omitempty"`
		// CreditLimited is the readable verdict the UI acts on: the last refusal was a
		// plan/entitlement limit rather than a dead credential. Scoped to
		// LastErrorModel — the account itself still serves other models.
		CreditLimited bool `json:"credit_limited,omitempty"`
	}

	out := make([]entry, 0, len(want))
	var totalRemaining, totalAllocation float64
	available, errored := 0, 0

	for _, it := range want {
		e := entry{
			ConnectionID:    it.id,
			Name:            it.name,
			Priority:        it.priority,
			ModelsCount:     it.models,
			LastErrorCode:   it.lastCode,
			LastErrorAt:     it.lastAt,
			LastErrorModel:  it.lastModel,
			CreditLimitHits: it.creditHits,
			CreditLimited:   it.lastCode == qoder.CreditLimitCode,
		}

		q, ok := quotaCache.Get(it.key)
		if !ok {
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			fetched, ferr := sessions.FetchQuota(ctx, it.key)
			cancel()
			if ferr != nil {
				e.Error = ferr.Error()
				errored++
				out = append(out, e)
				continue
			}
			quotaCache.Put(it.key, fetched)
			q = fetched
		}
		e.Quota = q
		available++
		totalRemaining += q.TotalRemaining()
		totalAllocation += q.TotalAllocation()
		out = append(out, e)
	}

	writeJSON(w, map[string]any{
		"success": true,
		"summary": map[string]any{
			"connections":      len(want),
			"available":        available,
			"errored":          errored,
			"total_remaining":  totalRemaining,
			"total_allocation": totalAllocation,
			// Named explicitly because it is the number an operator actually cares
			// about, and a client should not have to know the bucket structure to
			// find it.
			"credits_remaining": totalRemaining,
			"fetched_at":        time.Now().Format(time.RFC3339),
		},
		"data": out,
	})
}

// handleQoderConfig reports whether the provider is active and how it is
// configured, so the dashboard can explain a disabled state instead of showing a
// silently empty list.
//
// GET /api/qoder/config
func (s *Server) handleQoderConfig(w http.ResponseWriter, r *http.Request) {
	active := s.proxy.qoderProvider != nil
	resp := map[string]any{
		"success":  true,
		"enabled":  active,
		"template": qoder.TemplateReady(),
	}
	if !active {
		resp["hint"] = "set qoder_enabled, provision " + qoder.TemplateEnvVar + ", and restart"
		// Must write before returning: an early return without writeJSON sends an
		// empty 200 body, which the dashboard cannot parse and reports as an
		// unreadable config rather than the `enabled: false` + hint it needs.
		writeJSON(w, resp)
		return
	}
	resp["region"] = s.proxy.qoderRegion()
	resp["idle_timeout_seconds"] = int(s.proxy.qoderIdleTimeout().Seconds())
	resp["first_byte_timeout_seconds"] = int(s.proxy.qoderFirstByteTimeout().Seconds())
	writeJSON(w, resp)
}

// unused guard for fmt while the failure helper stays small.
var _ = fmt.Sprintf

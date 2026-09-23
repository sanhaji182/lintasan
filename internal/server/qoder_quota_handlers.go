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

	body, err := qoder.BuildChatBody(qoder.ChatRequest{
		Model:     modelID,
		Messages:  []map[string]any{{"role": "user", "content": "ping"}},
		Stream:    true,
		MaxTokens: qoderProbeMaxTokens,
	})
	if err != nil {
		return map[string]any{"success": false, "status": "upstream_error", "message": err.Error()}
	}

	resp, err := provider.Sessions().StartChatStream(ctx, conn.APIKey, modelID, body, "")
	if err != nil {
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
		fail := qoderProbeFailure(err, latency)
		fail["http_status"] = resp.StatusCode
		return fail
	}
	if !sawContent {
		return map[string]any{
			"success": false, "status": "empty_stream", "latency_ms": latency,
			"http_status": resp.StatusCode,
			"message":     "upstream accepted the request but produced no content within the idle window",
		}
	}

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
		SELECT id, name, api_key, priority, models_count
		FROM connections
		WHERE LOWER(format) = 'qoder' AND is_active = 1
		ORDER BY priority DESC`)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}
	defer rows.Close()

	type row struct {
		id, name, key string
		priority      int
		models        int
	}
	var want []row
	for rows.Next() {
		var it row
		if err := rows.Scan(&it.id, &it.name, &it.key, &it.priority, &it.models); err != nil {
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
	}

	out := make([]entry, 0, len(want))
	var totalRemaining, totalAllocation float64
	available, errored := 0, 0

	for _, it := range want {
		e := entry{ConnectionID: it.id, Name: it.name, Priority: it.priority, ModelsCount: it.models}

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

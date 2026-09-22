package server

// qoder_checkin_handlers.go — check-in / campaign visibility and claiming.
//
// MEASURED BEHAVIOUR (2026-09-22, all nine accounts, global region going through
// openapi.qoder.sh):
//
//	GET  /sash/api/v1/me/campaigns            -> 200 (needs `cosy-clienttype: 10`;
//	                                             without it the list comes back empty)
//	POST /sash/api/v1/me/campaigns/{id}/claim -> 200 {"status":"CLAIMED","grantId":...}
//
// So claiming WORKS on this host. What it does not do is pay out credits for the
// campaign currently on offer: the response carries no `benefit`, because
// act-20260901-493 is a VIEW_DETAILS subscription promotion whose credits land when
// a Pro/Pro+ subscription is taken.
//
// Endpoints:
//
//	GET  /api/qoder/checkin    — check-in availability per Qoder connection
//	GET  /api/qoder/campaigns  — campaigns, flagged by whether they grant credits
//	POST /api/qoder/checkin    — claim; DRY-RUN unless explicitly enabled
//
// The claim is guarded by the `qoder_checkin_enabled` setting. A POST returns the
// decision it would take plus, when enabled, the real upstream result. Claiming only
// ever RECEIVES credits and upstream is idempotent, but it is still a write, so the
// default is off and the reason is returned rather than a silent no-op.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// qoderCheckinCache is process-wide, mirroring quotaCache: a dashboard refresh must
// not re-authenticate every account on every poll, and campaign state moves slowly.
var qoderCheckinCache = qoder.NewCheckinCache(5 * time.Minute)

// qoderCheckinSettingKey gates the claim write. Absent/false => dry-run.
const qoderCheckinSettingKey = "qoder_checkin_enabled"

// qoderCheckinEnabled reports whether real claims are permitted.
func (s *Server) qoderCheckinEnabled() bool {
	// A read failure (including "no such row", which is the normal first-run state)
	// is treated as disabled — the safe direction for a write toggle.
	v, err := s.db.GetSetting(qoderCheckinSettingKey)
	if err != nil {
		return false
	}
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "1" || v == "true" || v == "yes"
}

// qoderCredentialRow is the minimal connection projection these handlers need.
type qoderCredentialRow struct {
	id, name, key string
	priority      int
}

// qoderQueryCredentials returns the active Qoder connections, optionally filtered
// to a single connection id.
func (s *Server) qoderQueryCredentials(one string) ([]qoderCredentialRow, error) {
	rows, err := s.db.Conn().Query(`
		SELECT id, name, api_key, priority
		FROM connections
		WHERE LOWER(format) = 'qoder' AND is_active = 1
		ORDER BY priority DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []qoderCredentialRow
	for rows.Next() {
		var it qoderCredentialRow
		if err := rows.Scan(&it.id, &it.name, &it.key, &it.priority); err != nil {
			continue
		}
		if one != "" && it.id != one {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

// handleQoderCheckin reports check-in availability, and claims when asked.
//
// GET  /api/qoder/checkin  — availability for every active Qoder connection
// POST /api/qoder/checkin  — claim (dry-run unless `qoder_checkin_enabled`)
//
// The GET response shape mirrors /api/qoder/quota so the dashboard can reuse its table.
func (s *Server) handleQoderCheckin(w http.ResponseWriter, r *http.Request) {
	if s.proxy.qoderProvider == nil {
		writeJSON(w, map[string]any{
			"success": false, "status": "not_enabled",
			"message": "the Qoder provider is not active",
			"data":    []any{},
		})
		return
	}

	one := strings.TrimSpace(r.PathValue("connection_id"))
	if r.Method == http.MethodPost {
		s.handleQoderCheckinClaim(w, r, one)
		return
	}

	want, err := s.qoderQueryCredentials(one)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}

	// An explicit refresh must actually refetch, so the dashboard's refresh button
	// does something instead of replaying the cache.
	if one != "" && r.URL.Query().Get("refresh") == "1" {
		for _, it := range want {
			qoderCheckinCache.Invalidate(it.key)
		}
	}

	sessions := s.proxy.qoderProvider.Sessions()

	type entry struct {
		ConnectionID string               `json:"connection_id"`
		Name         string               `json:"name"`
		Priority     int                  `json:"priority"`
		Checkin      *qoder.CheckinStatus `json:"checkin,omitempty"`
		Error        string               `json:"error,omitempty"`
	}

	out := make([]entry, 0, len(want))
	claimable, errored := 0, 0

	for _, it := range want {
		e := entry{ConnectionID: it.id, Name: it.name, Priority: it.priority}

		st, ok := qoderCheckinCache.Get(it.key)
		if !ok {
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			fetched, ferr := sessions.CheckinStatusFor(ctx, it.key)
			cancel()
			if ferr != nil {
				e.Error = ferr.Error()
				errored++
				out = append(out, e)
				continue
			}
			qoderCheckinCache.Put(it.key, fetched)
			st = fetched
		}
		e.Checkin = st
		if st.Claimable {
			claimable++
		}
		out = append(out, e)
	}

	writeJSON(w, map[string]any{
		"success": true,
		// Reports whether a REAL claim would be performed by POST, so a UI can label
		// the button honestly instead of implying a write that will not happen.
		"claim_enabled": s.qoderCheckinEnabled(),
		"summary": map[string]any{
			"connections": len(want),
			"claimable":   claimable,
			"errored":     errored,
			"fetched_at":  time.Now().Format(time.RFC3339),
		},
		"data": out,
	})
}

// handleQoderCheckinClaim performs (or dry-runs) a claim.
//
// POST /api/qoder/checkin            — every connection with a claimable campaign
// POST /api/qoder/checkin/{id}       — one connection
//
// Body/behaviour: claims are idempotent upstream, so a repeat is harmless. The
// `qoder_checkin_enabled` setting gates the actual write; when off, the decision is
// returned with `dry_run: true` and nothing is sent.
func (s *Server) handleQoderCheckinClaim(w http.ResponseWriter, r *http.Request, one string) {
	want, err := s.qoderQueryCredentials(one)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}
	enabled := s.qoderCheckinEnabled()
	sessions := s.proxy.qoderProvider.Sessions()

	type result struct {
		ConnectionID string               `json:"connection_id"`
		Name         string               `json:"name"`
		Result       *qoder.CheckinResult `json:"result"`
	}

	out := make([]result, 0, len(want))
	claimed, already, noCampaign, failed := 0, 0, 0, 0

	for _, it := range want {
		res := &qoder.CheckinResult{Host: qoder.CheckinHostGlobal}

		// Always read state first: it decides whether a claim is attempted at all, and
		// it supplies the reason when it is not.
		statusCtx, statusCancel := context.WithTimeout(r.Context(), 45*time.Second)
		st, serr := sessions.CheckinStatusFor(statusCtx, it.key)
		statusCancel()

		switch {
		case serr != nil:
			res.Status = "error"
			res.Message = serr.Error()
			failed++
		case st == nil || !st.Claimable:
			res.Status = "no_campaign"
			res.Message = "no claimable campaign for this account"
			if st != nil && st.Reason != "" {
				res.Message = st.Reason
			}
			noCampaign++
		case !enabled:
			// Dry run: report what WOULD happen, write nothing.
			res.Status = "dry_run"
			res.Host = st.Host
			res.Message = "claiming is disabled; set " + qoderCheckinSettingKey + "=true to enable"
			noCampaign++
		default:
			claimCtx, claimCancel := context.WithTimeout(r.Context(), 60*time.Second)
			cr, cerr := sessions.ClaimCheckin(claimCtx, it.key)
			claimCancel()
			if cerr != nil {
				res.Status = "error"
				res.Message = cerr.Error()
				failed++
			} else {
				cr.Host = st.Host
				res = cr
				switch cr.Status {
				case "claimed":
					claimed++
				case "already_claimed":
					already++
				case "no_campaign":
					noCampaign++
				default:
					failed++
				}
				// A claim changes campaign state, so the cached snapshot is stale.
				qoderCheckinCache.Invalidate(it.key)
			}
		}

		out = append(out, result{ConnectionID: it.id, Name: it.name, Result: res})
	}

	writeJSON(w, map[string]any{
		"success": true,
		"dry_run": !enabled,
		"summary": map[string]any{
			"connections": len(want),
			"claimed":     claimed,
			"already":     already,
			"no_campaign": noCampaign,
			"failed":      failed,
		},
		"data": out,
	})
}

// handleQoderCampaigns lists the campaigns upstream exposes, flagging which would
// actually grant credits and which are informational promotions.
//
// GET /api/qoder/campaigns
func (s *Server) handleQoderCampaigns(w http.ResponseWriter, r *http.Request) {
	if s.proxy.qoderProvider == nil {
		writeJSON(w, map[string]any{
			"success": false, "status": "not_enabled",
			"message": "the Qoder provider is not active",
			"data":    []any{},
		})
		return
	}

	want, err := s.qoderQueryCredentials("")
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}
	sessions := s.proxy.qoderProvider.Sessions()

	type entry struct {
		ConnectionID string                  `json:"connection_id"`
		Name         string                  `json:"name"`
		Host         string                  `json:"host,omitempty"`
		Campaigns    []qoder.CheckinCampaign `json:"campaigns"`
		Error        string                  `json:"error,omitempty"`
	}

	out := make([]entry, 0, len(want))
	for _, it := range want {
		e := entry{ConnectionID: it.id, Name: it.name, Campaigns: []qoder.CheckinCampaign{}}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		fetched, ferr := sessions.CheckinStatusFor(ctx, it.key)
		cancel()
		if ferr != nil {
			e.Error = ferr.Error()
		} else {
			e.Host = fetched.Host
			e.Campaigns = fetched.Campaigns
		}
		out = append(out, e)
	}

	writeJSON(w, map[string]any{
		"success": true,
		"note": "grants_credits reflects action_type==" + qoder.CheckinActionClaimBenefit +
			"; a campaign with grants_credits=false is still claimable but its benefit " +
			"(e.g. subscription credits) lands on fulfilment, not at claim time",
		"data": out,
	})
}

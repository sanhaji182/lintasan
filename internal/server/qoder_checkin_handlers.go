package server

// qoder_checkin_handlers.go — read-only check-in / campaign visibility.
//
// A DAILY "claim +100 credits" BUTTON IS DELIBERATELY NOT EXPOSED.
//
// On the global region — which is what this deployment's Qoder accounts are — the
// check-in endpoints that grant credits do not exist, and the one campaign upstream
// does return is a VIEW_DETAILS subscription promotion rather than a CLAIM_BENEFIT
// grant. Shipping a claim button would give an operator a control that can only
// ever fail or, worse, appear to succeed. See internal/qoder/checkin.go for the
// measurements behind that call.
//
// These endpoints therefore REPORT state:
//
//	GET /api/qoder/checkin    — check-in availability per Qoder connection
//	GET /api/qoder/campaigns  — campaigns upstream exposes, flagged by whether they
//	                            would actually grant credits
//
// If a CLAIM_BENEFIT campaign ever appears (e.g. a CN-region account is added),
// these endpoints report it without a code change. Enabling an actual claim is a
// separate, deliberate decision.

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

// handleQoderCheckin reports daily-check-in availability for Qoder connections.
//
// GET /api/qoder/checkin
//
// The response shape mirrors /api/qoder/quota so the dashboard can reuse its table.
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
	supported, claimable, errored := 0, 0, 0

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

		if st.Supported {
			supported++
		}
		if st.Claimable {
			claimable++
		}
		out = append(out, e)
	}

	writeJSON(w, map[string]any{
		"success": true,
		// Stated explicitly so a client need not infer it: this deployment cannot
		// claim credits, and the UI must not offer to.
		"claim_endpoint_enabled": false,
		"summary": map[string]any{
			"connections": len(want),
			"supported":   supported,
			"claimable":   claimable,
			"errored":     errored,
			"fetched_at":  time.Now().Format(time.RFC3339),
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
		"note": "campaigns with grants_credits=false are informational promotions; " +
			"only action_type=" + qoder.CheckinActionClaimBenefit + " campaigns grant credits",
		"data": out,
	})
}

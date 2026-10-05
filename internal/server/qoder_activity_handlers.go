package server

// qoder_activity_handlers.go — Cosy activity claim & eligibility endpoints (ported from qoder-suite).
//
// Endpoints:
//   GET  /api/qoder/activity/eligibility                  — Check eligibility for all Qoder accounts
//   GET  /api/qoder/activity/eligibility/{connection_id}  — Check eligibility for one account
//   POST /api/qoder/activity/claim                        — Claim specific activity ID
//   POST /api/qoder/activity/claim/{connection_id}        — Claim specific activity ID on an account
//   POST /api/qoder/activity/auto-claim                   — Run auto-claim across accounts
//   POST /api/qoder/activity/auto-claim/{connection_id}   — Run auto-claim for one account

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

type qoderActivityEligibilityEntry struct {
	ConnectionID string                           `json:"connection_id"`
	Name         string                           `json:"name"`
	Priority     int                              `json:"priority"`
	Eligibility  *qoder.ActivityEligibilityResult `json:"eligibility,omitempty"`
	Error        string                           `json:"error,omitempty"`
}

// handleQoderActivityEligibility handles GET /api/qoder/activity/eligibility.
func (s *Server) handleQoderActivityEligibility(w http.ResponseWriter, r *http.Request) {
	if s.proxy.qoderProvider == nil {
		writeJSON(w, map[string]any{
			"success": false,
			"message": "the Qoder provider is not active",
			"data":    []any{},
		})
		return
	}

	one := strings.TrimSpace(r.PathValue("connection_id"))
	if one == "" {
		one = strings.TrimSpace(r.URL.Query().Get("connection_id"))
	}

	want, err := s.qoderQueryCredentials(one)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	sessions := s.proxy.qoderProvider.Sessions()
	out := make([]qoderActivityEligibilityEntry, 0, len(want))
	totalClaimable := 0

	for _, it := range want {
		e := qoderActivityEligibilityEntry{
			ConnectionID: it.id,
			Name:         it.name,
			Priority:     it.priority,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		elig, ferr := sessions.CheckActivityEligibility(ctx, it.key, "")
		cancel()

		if ferr != nil {
			e.Error = ferr.Error()
		} else {
			e.Eligibility = elig
			for _, act := range elig.Activities {
				if s := act.Status; s == "available" || s == "claimable" || s == 1 || s == true {
					totalClaimable++
				}
			}
		}
		out = append(out, e)
	}

	writeJSON(w, map[string]any{
		"success": true,
		"data":    out,
		"summary": map[string]any{
			"total_accounts":  len(want),
			"claimable_count": totalClaimable,
		},
	})
}

// handleQoderActivityClaim handles POST /api/qoder/activity/claim.
func (s *Server) handleQoderActivityClaim(w http.ResponseWriter, r *http.Request) {
	if s.proxy.qoderProvider == nil {
		writeJSON(w, map[string]any{
			"success": false,
			"message": "the Qoder provider is not active",
		})
		return
	}

	one := strings.TrimSpace(r.PathValue("connection_id"))
	var reqBody struct {
		ConnectionID string `json:"connection_id"`
		ActivityID   string `json:"activity_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&reqBody)

	if one == "" {
		one = reqBody.ConnectionID
	}
	activityID := strings.TrimSpace(reqBody.ActivityID)
	if activityID == "" {
		activityID = strings.TrimSpace(r.URL.Query().Get("activity_id"))
	}

	if one == "" || activityID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "both connection_id and activity_id are required",
		})
		return
	}

	want, err := s.qoderQueryCredentials(one)
	if err != nil || len(want) == 0 {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": "qoder connection not found or inactive",
		})
		return
	}

	sessions := s.proxy.qoderProvider.Sessions()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	claimRes, claimErr := sessions.ClaimActivity(ctx, want[0].key, "", activityID)
	if claimErr != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"message": claimErr.Error(),
		})
		return
	}

	writeJSON(w, map[string]any{
		"success":       claimRes.Success,
		"connection_id": one,
		"activity_id":   activityID,
		"result":        claimRes,
	})
}

// handleQoderActivityAutoClaim handles POST /api/qoder/activity/auto-claim.
func (s *Server) handleQoderActivityAutoClaim(w http.ResponseWriter, r *http.Request) {
	if s.proxy.qoderProvider == nil {
		writeJSON(w, map[string]any{
			"success": false,
			"message": "the Qoder provider is not active",
		})
		return
	}

	one := strings.TrimSpace(r.PathValue("connection_id"))
	var reqBody struct {
		ConnectionID string `json:"connection_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&reqBody)
	if one == "" {
		one = reqBody.ConnectionID
	}

	want, err := s.qoderQueryCredentials(one)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	sessions := s.proxy.qoderProvider.Sessions()
	type autoEntry struct {
		ConnectionID string                  `json:"connection_id"`
		Name         string                  `json:"name"`
		Summary      *qoder.AutoClaimSummary `json:"summary,omitempty"`
		Error        string                  `json:"error,omitempty"`
	}

	results := make([]autoEntry, 0, len(want))
	totalClaimed := 0

	for _, it := range want {
		entry := autoEntry{
			ConnectionID: it.id,
			Name:         it.name,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		sum, ferr := sessions.AutoClaimActivities(ctx, it.key, "")
		cancel()

		if ferr != nil {
			entry.Error = ferr.Error()
		} else {
			entry.Summary = sum
			for _, c := range sum.Claims {
				if c.Success {
					totalClaimed++
				}
			}
		}
		results = append(results, entry)
	}

	writeJSON(w, map[string]any{
		"success": true,
		"data":    results,
		"summary": map[string]any{
			"accounts_processed": len(want),
			"claims_performed":   totalClaimed,
		},
	})
}

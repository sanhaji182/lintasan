package server

// qoder_models_handler.go — per-model Credit cost for Qoder.
//
// Qoder prices Credits with a per-model "reference factor", and applies a second
// multiplier during Off-Peak hours. Both are published by the vendor:
//
//	docs.qoder.com/release-notes/model-consumption-reference-update.md
//	docs.qoder.com/events/offpeakrate.md
//
// A raw factor is hard to act on ("0.5x of what?"), so this endpoint returns what an
// operator actually needs: the multiplier in force RIGHT NOW, how many effective
// units a credit balance buys at that multiplier, and when the window flips.
//
// GET /api/qoder/models          — factors for every model on every Qoder connection
// GET /api/qoder/models/{conn}   — one connection
//
// The window is evaluated per request in UTC, never cached, because a stale window is
// exactly the kind of error that silently doubles a bill.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// qoderModelCost is one model row.
type qoderModelCost struct {
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
	// Factor is the multiplier in force right now.
	Factor float64 `json:"factor"`
	// StandardFactor and OffPeakFactor are the published pair, so a caller can show
	// "0.2x now, 0.5x at 07:00" instead of only the current number.
	StandardFactor float64 `json:"standard_factor"`
	OffPeakFactor  float64 `json:"off_peak_factor"`
	// FactorSource says where the number came from: "vendor_table" (published discount
	// table), "discovered" (the upstream model list's own price_factor), or "unknown"
	// (nothing reported it).
	FactorSource string `json:"factor_source"`
	// OffPeakActive is whether the discounted window is the one in force right now.
	OffPeakActive bool `json:"off_peak_active"`
	// EffectiveCredits is how many units of this model the given credit balance buys
	// at Factor. This is the number that makes a factor actionable.
	EffectiveCredits float64 `json:"effective_credits,omitempty"`
	// MaxInputTokens / MaxOutputTokens are carried for context. Zero means upstream did
	// not report it — NOT that the model accepts no output.
	MaxInputTokens  int `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
	// PromoNote / PromoUntil record a temporary promotion overriding the factors.
	PromoNote  string `json:"promo_note,omitempty"`
	PromoUntil string `json:"promo_until,omitempty"`
	// FreeNow is true only when the model costs nothing at this moment (a 0.0x
	// promotion). Derived, never inferred from a missing factor.
	FreeNow bool `json:"free_now,omitempty"`
}

// handleQoderModels reports per-model Credit cost for Qoder connections.
func (s *Server) handleQoderModels(w http.ResponseWriter, r *http.Request) {
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

	// The window is computed once per request from the clock — never from a cache.
	now := time.Now()
	offPeak := qoder.IsOffPeak(now)
	nextChange := qoder.NextWindowChange(now)
	wib := time.FixedZone("WIB", 7*3600)

	type connEntry struct {
		ConnectionID string           `json:"connection_id"`
		Name         string           `json:"name"`
		CreditsLeft  float64          `json:"credits_left"`
		Models       []qoderModelCost `json:"models"`
		Error        string           `json:"error,omitempty"`
	}

	out := make([]connEntry, 0, len(want))

	for _, it := range want {
		e := connEntry{ConnectionID: it.id, Name: it.name, Models: []qoderModelCost{}}

		// Credits for the "what does this buy me" number. A snapshot read is enough —
		// the cache TTL exists precisely so a dashboard refresh is not 9 auth calls.
		credits := 0.0
		if q, ok := quotaCache.Get(it.key); ok && q != nil {
			credits = q.TotalRemaining()
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			if q, ferr := s.proxy.qoderProvider.Sessions().FetchQuota(ctx, it.key); ferr == nil && q != nil {
				quotaCache.Put(it.key, q)
				credits = q.TotalRemaining()
			} else if ferr != nil {
				e.Error = ferr.Error()
			}
			cancel()
		}
		e.CreditsLeft = credits

		// Per-account model catalogue. Entitlements differ per account, so this is not
		// a fixed list.
		rows, qerr := s.db.Conn().Query(
			`SELECT model_id, COALESCE(model_name, model_id), price_factor
			 FROM discovered_models
			 WHERE connection_id = ? AND is_active = 1
			 ORDER BY model_id`, it.id)
		if qerr != nil {
			e.Error = qerr.Error()
			out = append(out, e)
			continue
		}
		for rows.Next() {
			var id, name string
			var pf *float64
			if err := rows.Scan(&id, &name, &pf); err != nil {
				continue
			}

			row := qoderModelCost{ModelID: id, DisplayName: name, FactorSource: "unknown"}

			disc, published := qoder.FactorForModel(id)
			switch {
			case published:
				row.StandardFactor = disc.StandardFactor
				row.OffPeakFactor = disc.OffPeakFactor
				row.Factor = qoder.EffectiveFactor(id, disc.StandardFactor, now)
				row.FactorSource = "vendor_table"
				row.PromoNote = disc.PromoNote
				row.PromoUntil = disc.PromoUntil
				row.FreeNow = qoder.IsPromoFree(disc, now)
			case pf != nil && *pf > 0:
				// No published discount: the discovered factor is the factor, around
				// the clock. Carried into both fields so a UI shows one number, not a
				// blank discount column.
				row.StandardFactor = *pf
				row.OffPeakFactor = *pf
				row.Factor = *pf
				row.FactorSource = "discovered"
			}

			if credits > 0 && row.Factor > 0 {
				row.EffectiveCredits = qoder.EffectiveCredits(credits, row.Factor)
			}
			row.OffPeakActive = offPeak
			e.Models = append(e.Models, row)
		}
		rows.Close()

		out = append(out, e)
	}

	writeJSON(w, map[string]any{
		"success": true,
		"window": map[string]any{
			"off_peak_now":      offPeak,
			"off_peak_utc":      qoder.DiscountWindowUTC(),
			"off_peak_local":    qoder.DiscountWindowLocal(wib),
			"next_change":       nextChange.Format(time.RFC3339),
			"next_change_local": nextChange.In(wib).Format("15:04 WIB"),
			"server_time_local": now.In(wib).Format(time.RFC3339),
		},
		"note": "factor is the Credits multiplier in force now. effective_credits = " +
			"credits_left / factor — how many model units the balance still buys. " +
			"A missing factor is reported as unknown, never as free.",
		"data": out,
	})
}

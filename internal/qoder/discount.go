package qoder

// discount.go — Qoder's published time-of-day Credit multipliers.
//
// Qoder prices Credits per model with a relative "reference factor", and applies a
// second multiplier during Off-Peak hours. Both are published by the vendor:
//
//	docs.qoder.com/release-notes/model-consumption-reference-update.md  (factors)
//	docs.qoder.com/events/offpeakrate.md                                (windows)
//
// The windows are the operationally interesting part, because they are a 2.5x swing
// on the most-used premium model and nobody notices a factor is time-dependent:
//
//	Qwen3.8-Max  0.5x standard  ->  0.2x off-peak (60% off)
//	Qwen3.7-Max  0.5x standard  ->  0.1x off-peak (80% off)
//	Qwen3.7-Plus 0.1x standard  -> 0.04x off-peak (60% off)
//
// The canonical window is defined in UTC and is exact: Off-Peak is 14:00-00:00 UTC,
// Regular is 00:00-14:00 UTC. The vendor publishes local-time tables too, but those
// shift with DST, so this file only ever uses UTC and derives local times for display.
//
// Both the discount and the "free for a limited time" promos reduce the multiplier
// only — they do NOT grant bonus Credits, and usage still counts against the plan
// allowance. A vendor FAQ states the discount is unavailable once Credits are
// exhausted, which is why an exceeded account sees no benefit from timing.

import (
	"fmt"
	"time"
)

// Off-peak window bounds in UTC. The vendor defines the day as
// [regularStart, offPeakStart) = regular and [offPeakStart, 24:00) = off-peak.
const (
	OffPeakStartUTCHour = 14 // 14:00 UTC
	RegularStartUTCHour = 0  // 00:00 UTC
)

// ModelDiscount describes a time-dependent multiplier for one model.
type ModelDiscount struct {
	// ModelKey is the upstream key we receive in the model list (e.g. "qmodel_38max").
	ModelKey string `json:"model_key"`
	// DisplayName is the vendor's name for the model.
	DisplayName string `json:"display_name"`
	// StandardFactor is the published reference factor outside any promotion.
	StandardFactor float64 `json:"standard_factor"`
	// OffPeakFactor is the multiplier during Off-Peak hours. Equal to StandardFactor
	// when the model has no off-peak discount (rather than zero, which would read as
	// "free").
	OffPeakFactor float64 `json:"off_peak_factor"`
	// PromoNote records a temporary promotion that overrides the factors, if any.
	PromoNote string `json:"promo_note,omitempty"`
	// PromoUntil is when that promotion ends, RFC3339, empty if none.
	PromoUntil string `json:"promo_until,omitempty"`
}

// OffPeakDiscounts is the published table.
//
// Keys are the upstream model keys Lintasan receives, not the vendor's display names,
// so a caller can join straight onto a discovered model. Where a temporary promotion
// zeroes the factor (Qwen3.8-Flash is free until 2026-09-30 23:59 SGT) that is
// recorded as a PromoNote rather than baked into OffPeakFactor — when the promo ends,
// only the note is stale, not the arithmetic.
var OffPeakDiscounts = []ModelDiscount{
	{
		ModelKey: "qmodel_38max", DisplayName: "Qwen3.8-Max",
		StandardFactor: 0.50, OffPeakFactor: 0.20,
	},
	{
		ModelKey: "qfmodel", DisplayName: "Qwen3.8-Flash",
		StandardFactor: 0.10, OffPeakFactor: 0.04,
		PromoNote:  "free (0.0x) for a limited time; no claim required, and accounts with a zero balance are eligible",
		PromoUntil: "2026-09-30T23:59:59+08:00",
	},
	{
		ModelKey: "qmodel_37max", DisplayName: "Qwen3.7-Max",
		StandardFactor: 0.50, OffPeakFactor: 0.10,
	},
	{
		ModelKey: "qmodel_37plus", DisplayName: "Qwen3.7-Plus",
		StandardFactor: 0.10, OffPeakFactor: 0.04,
	},
}

// FactorForModel returns the published discount entry for an upstream model key.
func FactorForModel(key string) (ModelDiscount, bool) {
	for _, d := range OffPeakDiscounts {
		if d.ModelKey == key {
			return d, true
		}
	}
	return ModelDiscount{}, false
}

// IsOffPeak reports whether t falls in the Off-Peak window, evaluated in UTC.
func IsOffPeak(t time.Time) bool {
	h := t.UTC().Hour()
	return h >= OffPeakStartUTCHour || h < RegularStartUTCHour
}

// EffectiveFactor returns the multiplier that applies to a model at time t.
//
// When the model is not in the published table, fallback is returned unchanged — the
// caller's own discovered factor. That matters: the fallback for a non-Qwen model
// must not be silently rewritten to 1.0.
func EffectiveFactor(key string, fallback float64, t time.Time) float64 {
	d, ok := FactorForModel(key)
	if !ok {
		return fallback
	}
	if IsOffPeak(t) {
		return d.OffPeakFactor
	}
	return d.StandardFactor
}

// NextWindowChange returns when the current window ends, so a UI can say "off-peak in
// 2h 14m" instead of leaving an operator to work it out. Returns t itself if the
// clock is exactly on a boundary.
func NextWindowChange(t time.Time) time.Time {
	u := t.UTC()
	if IsOffPeak(t) {
		// Ends at 00:00 UTC the next day.
		next := time.Date(u.Year(), u.Month(), u.Day(), RegularStartUTCHour, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
		return next
	}
	return time.Date(u.Year(), u.Month(), u.Day(), OffPeakStartUTCHour, 0, 0, 0, time.UTC)
}

// EffectiveCredits converts a credit balance into the number of model units it buys
// at the factor in force at time t.
//
// This is the number an operator actually wants: credits are abstract, "how many
// requests of this model can I still make" is not. Returns 0 for a factor of 0 or
// less, because an unset factor must not be read as unlimited.
func EffectiveCredits(credits, factor float64) float64 {
	if factor <= 0 {
		return 0
	}
	return credits / factor
}

// IsPromoFree reports whether a model is currently free because of a recorded
// promotion.
//
// Derived, never inferred from a missing factor: an unset factor means "not reported"
// and must not be presented as free. Only a promo that is active AND has not passed its
// end date counts.
func IsPromoFree(d ModelDiscount, t time.Time) bool {
	if d.PromoNote == "" {
		return false
	}
	// Qwen3.8-Flash is the only free promo on record: 0.0x until its PromoUntil.
	if d.PromoUntil == "" {
		return false
	}
	until, err := time.Parse(time.RFC3339, d.PromoUntil)
	if err != nil {
		return false
	}
	return t.Before(until)
}

// DiscountWindowUTC renders the window in UTC, which is the vendor's canonical form.
func DiscountWindowUTC() string {
	return fmt.Sprintf("%02d:00-%02d:00 UTC", OffPeakStartUTCHour, RegularStartUTCHour)
}

// DiscountWindowLocal renders the window in a given location, for display only.
func DiscountWindowLocal(loc *time.Location) string {
	// Use an arbitrary date; only the clock times matter and both bounds are the same
	// UTC day, so no date arithmetic is needed.
	base := time.Date(2026, 1, 1, OffPeakStartUTCHour, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 2, RegularStartUTCHour, 0, 0, 0, time.UTC)
	return fmt.Sprintf("%s-%s %s",
		base.In(loc).Format("15:04"), end.In(loc).Format("15:04"), loc.String())
}

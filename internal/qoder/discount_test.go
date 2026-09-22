package qoder

import (
	"testing"
	"time"
)

// The off-peak window is defined in UTC and does NOT wrap midnight:
// Off-Peak 14:00–24:00, Regular 00:00–14:00. Verified against the vendor's own local
// table ("Singapore/HK/Beijing (UTC+8): Off-Peak 22:00–08:00" — i.e. 14:00 UTC →
// 00:00 UTC). Getting this backwards is a silent 2.5x cost error, so both edges and
// both halves of the day are pinned.

func TestIsOffPeakBoundaries(t *testing.T) {
	utc := func(h, m int) time.Time {
		return time.Date(2026, 9, 22, h, m, 0, 0, time.UTC)
	}
	cases := []struct {
		at   time.Time
		want bool
		why  string
	}{
		{utc(0, 0), false, "00:00 UTC starts REGULAR hours, not Off-Peak"},
		{utc(0, 1), false, "just after midnight is Regular"},
		{utc(7, 30), false, "mid Regular (this is 14:30 WIB — a working hour)"},
		{utc(13, 59), false, "last minute of Regular"},
		{utc(14, 0), true, "14:00 UTC is the Off-Peak start"},
		{utc(14, 1), true, "just after 14:00 is Off-Peak"},
		{utc(20, 0), true, "mid Off-Peak (this is 03:00 WIB)"},
		{utc(23, 59), true, "last minute of the UTC day is Off-Peak"},
	}
	for _, c := range cases {
		if got := IsOffPeak(c.at); got != c.want {
			t.Errorf("IsOffPeak(%s UTC) = %v, want %v (%s)",
				c.at.Format("15:04"), got, c.want, c.why)
		}
	}
}

// A WIB operator should see the window as 21:00–07:00: Off-Peak runs overnight, not
// during their working day.
func TestIsOffPeakInWIB(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	// 12:00 WIB = 05:00 UTC = Regular.
	if IsOffPeak(time.Date(2026, 9, 22, 12, 0, 0, 0, wib)) {
		t.Error("12:00 WIB (05:00 UTC) should be Regular")
	}
	// 22:00 WIB = 15:00 UTC = Off-Peak.
	if !IsOffPeak(time.Date(2026, 9, 22, 22, 0, 0, 0, wib)) {
		t.Error("22:00 WIB (15:00 UTC) should be Off-Peak")
	}
}

// EffectiveFactor must apply the off-peak multiplier inside the window and the
// standard one outside it — for Qwen3.8-Max that is 0.2 vs 0.5, a 2.5x swing.
func TestEffectiveFactorQwen38Max(t *testing.T) {
	peak := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC) // Regular hours
	off := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)  // Off-Peak hours

	if got := EffectiveFactor("qmodel_38max", 0.5, peak); got != 0.50 {
		t.Errorf("regular hours factor = %v, want 0.50", got)
	}
	if got := EffectiveFactor("qmodel_38max", 0.5, off); got != 0.20 {
		t.Errorf("off-peak factor = %v, want 0.20 (60%% off)", got)
	}
}

// An unknown model must be returned untouched. Silently rewriting a caller's
// discovered factor to 1.0 would misprice every non-Qwen model.
func TestEffectiveFactorUnknownModelPassesThrough(t *testing.T) {
	off := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	if got := EffectiveFactor("some-other-model", 1.7, off); got != 1.7 {
		t.Fatalf("unknown model factor was rewritten: got %v, want 1.7", got)
	}
	// And a model with no published discount keeps its standard factor off-peak.
	if got := EffectiveFactor("qmodel_38max", 0.5, off); got == 0.5 {
		t.Error("Qwen3.8-Max must get the off-peak discount")
	}
}

// EffectiveCredits is the number an operator wants: credits are abstract, "how many
// units of this model can I still run" is not.
func TestEffectiveCredits(t *testing.T) {
	// 1260 credits, off-peak Qwen3.8-Max at 0.2x = 6300 units.
	if got := EffectiveCredits(1260, 0.2); got != 6300 {
		t.Errorf("EffectiveCredits(1260, 0.2) = %v, want 6300", got)
	}
	// Same credits at the standard 0.5x = 2520 units.
	if got := EffectiveCredits(1260, 0.5); got != 2520 {
		t.Errorf("EffectiveCredits(1260, 0.5) = %v, want 2520", got)
	}
	// A zero or unset factor must NOT be read as unlimited.
	if got := EffectiveCredits(1260, 0); got != 0 {
		t.Errorf("a zero factor must yield 0 units, not %v", got)
	}
	if got := EffectiveCredits(1260, -1); got != 0 {
		t.Errorf("a negative factor must yield 0 units, not %v", got)
	}
}

// NextWindowChange drives a "off-peak in 2h 14m" label.
func TestNextWindowChange(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{
			"regular -> next change is 14:00 UTC the same day",
			time.Date(2026, 9, 22, 9, 30, 0, 0, time.UTC),
			time.Date(2026, 9, 22, 14, 0, 0, 0, time.UTC),
		},
		{
			"regular just after midnight -> 14:00 UTC the same day",
			time.Date(2026, 9, 22, 3, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 22, 14, 0, 0, 0, time.UTC),
		},
		{
			"off-peak -> 00:00 UTC the NEXT day",
			time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC),
		},
		{
			"off-peak late at night -> 00:00 UTC the next day",
			time.Date(2026, 9, 22, 23, 59, 0, 0, time.UTC),
			time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NextWindowChange(c.at)
			if !got.Equal(c.want) {
				t.Errorf("NextWindowChange(%s) = %s, want %s",
					c.at.Format(time.RFC3339), got.Format(time.RFC3339), c.want.Format(time.RFC3339))
			}
		})
	}
}

// The published table must stay consistent with the vendor's own numbers.
func TestOffPeakDiscountTableMatchesPublishedRates(t *testing.T) {
	want := map[string]struct{ std, off float64 }{
		"qmodel_38max":  {0.50, 0.20}, // Qwen3.8-Max: 60% off
		"qfmodel":       {0.10, 0.04}, // Qwen3.8-Flash: 60% off (and free-promo until 09-30)
		"qmodel_37max":  {0.50, 0.10}, // Qwen3.7-Max: 80% off
		"qmodel_37plus": {0.10, 0.04}, // Qwen3.7-Plus: 60% off
	}
	for key, w := range want {
		d, ok := FactorForModel(key)
		if !ok {
			t.Errorf("%s missing from the discount table", key)
			continue
		}
		if d.StandardFactor != w.std {
			t.Errorf("%s standard = %v, want %v", key, d.StandardFactor, w.std)
		}
		if d.OffPeakFactor != w.off {
			t.Errorf("%s off-peak = %v, want %v", key, d.OffPeakFactor, w.off)
		}
		if d.OffPeakFactor > d.StandardFactor {
			t.Errorf("%s off-peak factor is HIGHER than standard — that is a bug, not a discount", key)
		}
	}
	// The free promotion must be recorded as a note, not as a zeroed factor, so the
	// arithmetic stays right after it ends.
	qf, _ := FactorForModel("qfmodel")
	if qf.PromoNote == "" || qf.PromoUntil == "" {
		t.Error("the Qwen3.8-Flash free promotion must be recorded with its end date")
	}
}

// The window must be rendered for a local timezone without hand-maintained tables.
func TestDiscountWindowLocal(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	got := DiscountWindowLocal(wib)
	// 14:00 UTC = 21:00 WIB, 00:00 UTC = 07:00 WIB.
	if got != "21:00-07:00 WIB" {
		t.Errorf("WIB window = %q, want \"21:00-07:00 WIB\"", got)
	}
}

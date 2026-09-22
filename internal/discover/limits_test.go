package discover

import "testing"

// The two mappers encode "not reported" as SQL NULL, which is the whole point: a stored
// zero is a claim about the model, not a missing measurement. Qoder sends
// max_output_tokens as 0 for models that plainly produce output, and a 0.0x price factor
// for models the vendor's own table prices at 0.1x.

func TestPositiveOrNilMapsUnreportedToNull(t *testing.T) {
	if got := positiveOrNil(180000); got != 180000 {
		t.Errorf("a real limit must pass through, got %v", got)
	}
	for _, bad := range []int{0, -1} {
		if got := positiveOrNil(bad); got != nil {
			t.Errorf("positiveOrNil(%d) = %v, want nil (0 must not be stored as a limit — "+
				"it would render as \"0 tokens\", i.e. a claim the model accepts none)", bad, got)
		}
	}
}

func TestPriceFactorOrNilMapsUnreportedToNull(t *testing.T) {
	if got := priceFactorOrNil(0.5); got != 0.5 {
		t.Errorf("a real factor must pass through, got %v", got)
	}
	for _, bad := range []float64{0, -1, 0.0} {
		if got := priceFactorOrNil(bad); got != nil {
			t.Errorf("priceFactorOrNil(%v) = %v, want nil (a stored 0 would read as free)", bad, got)
		}
	}
}

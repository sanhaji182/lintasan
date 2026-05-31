package provider

import (
	"reflect"
	"testing"
)

// capability_enforce_test.go — F2.4 enforcement facade tests.
//
// These pin the load-bearing F2.4 invariants:
//   - PARITY (R4): EnforceEligibility's Dropped set == ShadowEvaluateIdentity's
//     WouldExclude set for the same input — the SAME evaluator, no drift.
//   - FAIL-OPEN: a default-tier (missing-data) candidate is NEVER dropped, even
//     when its conservative caps fail the request.
//   - EMPTY-POOL (R3): a mask that would empty the pool reverts to keep-all with
//     Aborted=true; enforcement narrows, never zeroes.
//   - PURITY: inputs are not mutated.

// realModelIdentity returns an identity that resolves data-backed (Tier model)
// — a known catalog model — so its capabilities are real, not fail-open.
func knownModelIdentity(model string) CandidateIdentity {
	return CandidateIdentity{Model: model, OwnedBy: "openai", BaseURL: "https://api.openai.com/v1"}
}

// TestEnforce_ParityWithShadow is the R4 anchor: the dropped set must equal the
// shadow would_exclude set for identical input.
func TestEnforce_ParityWithShadow(t *testing.T) {
	// A vision request against a mix: a data-backed provider that lacks vision
	// should be droppable; a default-tier one must be kept (fail-open).
	signals := RequestSignals{HasVision: true, Stream: true}
	identities := []CandidateIdentity{
		{Format: "openai", Model: "gpt-4o", OwnedBy: "openai", BaseURL: "https://api.openai.com/v1"},
		{Format: "openai", Model: "some-unknown-model", BaseURL: "https://self-hosted.example.com/v1"},
	}

	shadow := ShadowEvaluateIdentity(signals, identities)
	enf := EnforceEligibility(signals, identities)

	if !enf.Aborted && !reflect.DeepEqual(enf.Dropped, shadow.WouldExclude) {
		t.Fatalf("R4 parity broken: enforce.Dropped=%v shadow.WouldExclude=%v", enf.Dropped, shadow.WouldExclude)
	}
	// The embedded shadow record must be the same observation.
	if !reflect.DeepEqual(enf.Shadow.WouldExclude, shadow.WouldExclude) {
		t.Fatalf("embedded shadow diverged: %v vs %v", enf.Shadow.WouldExclude, shadow.WouldExclude)
	}
}

// TestEnforce_FailOpenNeverDrops: a candidate that resolves at the default tier
// (no model match, no derivable provider) must be KEPT even for a capability it
// conservatively lacks.
func TestEnforce_FailOpenNeverDrops(t *testing.T) {
	signals := RequestSignals{HasVision: true}
	identities := []CandidateIdentity{
		{Format: "openai", Model: "totally-unknown", BaseURL: "https://mystery.invalid/v1"},
	}
	enf := EnforceEligibility(signals, identities)
	if len(enf.Keep) != 1 || !enf.Keep[0] {
		t.Fatalf("fail-open candidate must be kept, Keep=%v", enf.Keep)
	}
	if len(enf.Dropped) != 0 {
		t.Fatalf("fail-open candidate must never be dropped, Dropped=%v", enf.Dropped)
	}
}

// TestEnforce_DropsDataBackedFailure: a data-backed candidate that positively
// lacks a required capability IS dropped (the whole point of enforcement), as
// long as the pool does not empty.
func TestEnforce_DropsDataBackedFailure(t *testing.T) {
	signals := RequestSignals{HasVision: true}
	identities := []CandidateIdentity{
		knownModelIdentity("gpt-4o"), // vision-capable → keep
		{Format: "groq", Model: "", OwnedBy: "groq", BaseURL: "https://api.groq.com/openai/v1"}, // data-backed, no vision → drop
	}
	enf := EnforceEligibility(signals, identities)
	if enf.Aborted {
		t.Fatal("pool should not abort: gpt-4o survives as a vision-capable keep")
	}
	if enf.Keep[0] != true {
		t.Fatalf("vision-capable gpt-4o must be kept, Keep=%v", enf.Keep)
	}
	if enf.Keep[1] != false {
		t.Fatalf("data-backed non-vision groq must be dropped, Keep=%v", enf.Keep)
	}
	if len(enf.Dropped) != 1 {
		t.Fatalf("expected exactly 1 drop, Dropped=%v", enf.Dropped)
	}
}

// TestEnforce_EmptyPoolGuard: if EVERY candidate is a data-backed failure, the
// R3 guard reverts to keep-all and sets Aborted. Uses two groq identities —
// both resolve data-backed (Tier provider) to caps [streaming, tool_calling],
// neither has vision — so a vision request would drop both, emptying the pool.
func TestEnforce_EmptyPoolGuard(t *testing.T) {
	signals := RequestSignals{HasVision: true}
	identities := []CandidateIdentity{
		{Format: "groq", OwnedBy: "groq", BaseURL: "https://api.groq.com/openai/v1"},
		{Format: "groq", OwnedBy: "groq", BaseURL: "https://api.groq.com/openai/v1"},
	}
	// Precondition: both must be data-backed failures (else the test proves nothing).
	shadow := ShadowEvaluateIdentity(signals, identities)
	if len(shadow.WouldExclude) != len(identities) {
		t.Fatalf("precondition: expected ALL %d candidates data-backed-failing, got would_exclude=%v",
			len(identities), shadow.WouldExclude)
	}
	enf := EnforceEligibility(signals, identities)
	if !enf.Aborted {
		t.Fatalf("empty-pool must abort, Aborted=false Keep=%v", enf.Keep)
	}
	for i, k := range enf.Keep {
		if !k {
			t.Fatalf("aborted enforcement must keep ALL, Keep[%d]=false", i)
		}
	}
	if len(enf.Dropped) != 0 {
		t.Fatalf("aborted enforcement must report no drops, Dropped=%v", enf.Dropped)
	}
}

// TestEnforce_EmptyInput: no candidates → empty result, not aborted, no panic.
func TestEnforce_EmptyInput(t *testing.T) {
	enf := EnforceEligibility(RequestSignals{Stream: true}, nil)
	if len(enf.Keep) != 0 || enf.Aborted || len(enf.Dropped) != 0 {
		t.Fatalf("empty input must yield empty non-aborted result, got %+v", enf)
	}
}

// TestEnforce_NoRequiredCapsKeepsAll: a request needing nothing keeps everyone.
func TestEnforce_NoRequiredCapsKeepsAll(t *testing.T) {
	identities := []CandidateIdentity{
		knownModelIdentity("gpt-4o"),
		{Format: "groq", OwnedBy: "groq", BaseURL: "https://api.groq.com/openai/v1"},
	}
	enf := EnforceEligibility(RequestSignals{}, identities)
	for i, k := range enf.Keep {
		if !k {
			t.Fatalf("no required caps must keep all, Keep[%d]=false", i)
		}
	}
	if len(enf.Dropped) != 0 {
		t.Fatalf("no required caps must drop none, Dropped=%v", enf.Dropped)
	}
}

// TestEnforce_DoesNotMutateInput: the identities slice is untouched.
func TestEnforce_DoesNotMutateInput(t *testing.T) {
	identities := []CandidateIdentity{
		knownModelIdentity("gpt-4o"),
		{Format: "groq", OwnedBy: "groq", BaseURL: "https://api.groq.com/openai/v1"},
	}
	before := make([]CandidateIdentity, len(identities))
	copy(before, identities)
	_ = EnforceEligibility(RequestSignals{HasVision: true}, identities)
	if !reflect.DeepEqual(before, identities) {
		t.Fatal("EnforceEligibility must not mutate its input identities")
	}
}

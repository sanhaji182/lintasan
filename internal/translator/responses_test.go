package translator

import (
	"errors"
	"testing"
)

// responses_test.go — Codex M0 scaffolding tests for the translator layer.
//
// M0 locks ONLY that the scaffolding exists with the right shape and that it is
// honestly unimplemented (returns ErrResponsesNotImplemented, never a fake
// translation). Real translation assertions arrive in M1. Critically, these
// tests also pin that FormatResponses is NOT yet wired into the existing
// 5-format machinery, so the live translator behavior is unchanged.

// TestResponsesFormatConstant: the 6th format constant exists with the expected
// wire value ("responses"), matching Codex's wire_api value.
func TestResponsesFormatConstant(t *testing.T) {
	if FormatResponses != "responses" {
		t.Errorf("FormatResponses must be \"responses\", got %q", FormatResponses)
	}
}

// TestResponsesStubsUnimplemented: M0 translation entry points return
// ErrResponsesNotImplemented (no fake/no-op translation). Replaced in M1+.
func TestResponsesStubsUnimplemented(t *testing.T) {
	if _, err := ResponsesToOpenAIRequest(map[string]any{"model": "gpt-5"}); !errors.Is(err, ErrResponsesNotImplemented) {
		t.Errorf("ResponsesToOpenAIRequest M0 stub must return ErrResponsesNotImplemented, got %v", err)
	}
	if _, err := OpenAIResponseToResponses(map[string]any{"object": "chat.completion"}); !errors.Is(err, ErrResponsesNotImplemented) {
		t.Errorf("OpenAIResponseToResponses M0 stub must return ErrResponsesNotImplemented, got %v", err)
	}
}

// TestResponsesNotWiredIntoExistingFormats: M0 must NOT add FormatResponses to
// AllFormats() — the existing 5-format translator behavior stays byte-identical
// until M1 implements the mapping. This guards against premature wiring.
func TestResponsesNotWiredIntoExistingFormats(t *testing.T) {
	for _, f := range AllFormats() {
		if f == FormatResponses {
			t.Fatal("FormatResponses must NOT be in AllFormats() during M0 (would alter existing translator behavior before M1 implements it)")
		}
	}
	if len(AllFormats()) != 5 {
		t.Errorf("expected 5 existing formats in M0, got %d", len(AllFormats()))
	}
}

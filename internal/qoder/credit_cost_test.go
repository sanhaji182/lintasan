package qoder

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestParseStreamFrameCapturesCreditCost pins that the upstream charge reaches the
// delta instead of being dropped.
//
// Why this test exists: Qoder's usage frame carries the only authoritative price
// for a turn, and the vendor states consumption follows "task complexity" rather
// than token count — so the figure cannot be reconstructed from prompt_tokens /
// completion_tokens. Measured 2026-09-23, the same model and the same prompt cost
// 4.617 credits with a cold prompt prefix and 0.449 with it cached. Dropping the
// field (as the parser did before this change) left the dashboard able to show
// tokens but never cost.
func TestParseStreamFrameCapturesCreditCost(t *testing.T) {
	inner := `{"choices":[{"delta":{"content":"OK"},"index":0}],` +
		`"usage":{"billable":true,"completion_tokens":52,` +
		`"credits":4.617181250000001,"original_credits":4.617181250000001,` +
		`"prompt_tokens":13225,"prompt_tokens_details":{"cached_tokens":0},` +
		`"total_tokens":13277}}`
	frame, _ := json.Marshal(map[string]any{"body": inner})

	d, uerr := ParseStreamFrame(frame)
	if uerr != nil {
		t.Fatalf("unexpected upstream error: %v", uerr)
	}
	if d.Credits == 0 {
		t.Fatal("credits dropped: a 4.617-credit turn would log as free")
	}
	if d.Credits != 4.617181250000001 {
		t.Errorf("credits = %v, want 4.617181250000001", d.Credits)
	}
	if !d.Billable {
		t.Error("billable flag dropped")
	}
	if d.CachedTokens != 0 {
		t.Errorf("cached_tokens = %d, want 0 for a cold prefix", d.CachedTokens)
	}
	if d.InputTokens != 13225 || d.OutputTokens != 52 {
		t.Errorf("tokens regressed: in=%d out=%d", d.InputTokens, d.OutputTokens)
	}
}

// TestParseStreamFrameCapturesCachedPrefix covers the dominant cost variable: the
// same prompt with a warm prefix is ~10x cheaper, so cached_tokens must survive.
func TestParseStreamFrameCapturesCachedPrefix(t *testing.T) {
	inner := `{"usage":{"billable":false,"completion_tokens":55,` +
		`"credits":0.45060125000000006,"prompt_tokens":13225,` +
		`"prompt_tokens_details":{"cached_tokens":13188},"total_tokens":13280}}`
	frame, _ := json.Marshal(map[string]any{"body": inner})

	d, uerr := ParseStreamFrame(frame)
	if uerr != nil {
		t.Fatalf("unexpected upstream error: %v", uerr)
	}
	if d.CachedTokens != 13188 {
		t.Errorf("cached_tokens = %d, want 13188", d.CachedTokens)
	}
	if d.Billable {
		t.Error("billable = true, want false (a cached turn reported billable=false)")
	}
	if d.Credits != 0.45060125000000006 {
		t.Errorf("credits = %v, want 0.45060125000000006", d.Credits)
	}
}

// TestConsumeStreamSeparatesZeroFromAbsent is the reason `CreditsReported` exists.
//
// A usage frame that carries no `credits` field must NOT be recorded as a turn
// that cost nothing — that would render as a free request on the dashboard and
// hide a genuine reporting gap. A frame that reports 0 is a different fact and
// must still count as reported.
func TestConsumeStreamSeparatesZeroFromAbsent(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		// Usage present, but no credits field at all.
		stream := `data: {"body":"{\"choices\":[{\"delta\":{\"content\":\"hi\"},\"index\":0}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2}}"}` + "\n\n"
		outcome, err := ConsumeStream(context.Background(), strings.NewReader(stream), "auto", func(StreamDelta) error { return nil })
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if outcome.CreditsReported {
			t.Error("CreditsReported = true for a frame with no credits field; a missing figure is not a free turn")
		}
		if outcome.Credits != 0 {
			t.Errorf("Credits = %v, want 0", outcome.Credits)
		}
	})

	t.Run("reported zero", func(t *testing.T) {
		stream := `data: {"body":"{\"choices\":[{\"delta\":{\"content\":\"hi\"},\"index\":0}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"credits\":0,\"billable\":false}}"}` + "\n\n"
		outcome, err := ConsumeStream(context.Background(), strings.NewReader(stream), "auto", func(StreamDelta) error { return nil })
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if !outcome.CreditsReported {
			t.Error("a usage frame carrying credits=0 must count as reported, not absent")
		}
	})

	t.Run("sums and keeps the cached count", func(t *testing.T) {
		stream := `data: {"body":"{\"choices\":[{\"delta\":{\"content\":\"a\"},\"index\":0}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":5,\"credits\":1.5,\"billable\":true,\"prompt_tokens_details\":{\"cached_tokens\":64}}}"}` + "\n\n"
		outcome, err := ConsumeStream(context.Background(), strings.NewReader(stream), "auto", func(StreamDelta) error { return nil })
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if outcome.Credits != 1.5 {
			t.Errorf("Credits = %v, want 1.5", outcome.Credits)
		}
		if outcome.CachedTokens != 64 {
			t.Errorf("CachedTokens = %d, want 64", outcome.CachedTokens)
		}
		if !outcome.Billable {
			t.Error("Billable = false, want true")
		}
	})
}

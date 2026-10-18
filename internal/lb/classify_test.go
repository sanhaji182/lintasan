package lb

import (
	"testing"
	"time"
)

// Tests pin the classification contract from the 9router parity plan B1.
// Each rule is asserted via ClassifyUpstreamError, not by duplicating the
// table — so a future edit that changes order or scope breaks exactly the
// behaviour it changes.

func TestClassifyStatusRulesBeatNoText(t *testing.T) {
	// No text, 403 → long fixed cooldown.
	got := ClassifyUpstreamError("anything-provider", 403, "", 0)
	if !got.ShouldFallback || got.Backoff || got.Cooldown != CooldownLong {
		t.Fatalf("403 no-text: got %+v, want fixed CooldownLong fallback", got)
	}
}

func TestClassify429Backoff(t *testing.T) {
	got := ClassifyUpstreamError("p", 429, "", 0)
	if !got.ShouldFallback || !got.Backoff || got.NewBackoffLevel != 1 {
		t.Fatalf("429 bare: got %+v, want backoff level 1", got)
	}
	if got.Cooldown != BackoffBase {
		t.Fatalf("429 bare cooldown = %s, want %s", got.Cooldown, BackoffBase)
	}
}

func TestClassifyRateLimitTextBeatsStatus(t *testing.T) {
	// Upstream returns 403 but body says "rate limit exceeded" — text rule
	// wins over the status rule (order contract), so this backs off
	// exponentially instead of sitting out the long 403 cooldown.
	got := ClassifyUpstreamError("p", 403, "Rate Limit Exceeded", 3)
	if !got.ShouldFallback || !got.Backoff || got.NewBackoffLevel != 4 {
		t.Fatalf("text-beats-status: got %+v, want backoff level 4", got)
	}
	if want := 16 * time.Second; got.Cooldown != want {
		t.Fatalf("backoff level 4 cooldown = %s, want %s", got.Cooldown, want)
	}
}

func TestClassifyProviderScopedCodexRule(t *testing.T) {
	txt := "model is not supported when using codex with a chatgpt account"
	// On codex → 30 min hard cooldown.
	got := ClassifyUpstreamError("codex", 400, txt, 0)
	if !got.ShouldFallback || got.Cooldown != MaxRateLimitCool {
		t.Fatalf("codex scoped: got %+v, want %s", got, MaxRateLimitCool)
	}
	// On another provider, same text → the rule must NOT fire (falls through
	// to status, none matches here → no fallback).
	other := ClassifyUpstreamError("not-codex", 400, txt, 0)
	if other.ShouldFallback {
		t.Fatalf("codex rule leaked to another provider: %+v", other)
	}
}

func TestClassifyTransientTexts(t *testing.T) {
	cases := []struct {
		text     string
		cooldown time.Duration
	}{
		{"no credentials — supply an API key", CooldownLong},
		{"request not allowed for this endpoint", CooldownShort},
		{"improperly formed request body", CooldownLong},
		{"upstream capacity exhausted", BackoffBase},      // backoff → level 1
		{"service overloaded, try again", BackoffBase},    // backoff → level 1
		{"quota exceeded for today", BackoffBase},         // backoff → level 1
		{"too many requests right now", BackoffBase},      // backoff → level 1
		{"insufficient_quota (openai wording)", BackoffBase}, // backoff → level 1
	}
	for _, tc := range cases {
		got := ClassifyUpstreamError("p", 0, tc.text, 0)
		if !got.ShouldFallback {
			t.Fatalf("text %q: no fallback, want one", tc.text)
		}
		if got.Cooldown != tc.cooldown {
			t.Fatalf("text %q: cooldown %s, want %s", tc.text, got.Cooldown, tc.cooldown)
		}
	}
}

func TestClassifyNoMatchReturnsZero(t *testing.T) {
	got := ClassifyUpstreamError("p", 503, "connection reset by peer", 0)
	if got.ShouldFallback {
		t.Fatalf("503 no-rule: got %+v, want no fallback (caller default)", got)
	}
}

func TestBackoffCooldownCapsAndFloors(t *testing.T) {
	tests := []struct {
		level int
		want  time.Duration
	}{
		{0, BackoffBase},
		{-3, BackoffBase},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{16, BackoffMax},
		{9999, BackoffMax},
	}
	for _, tc := range tests {
		if got := BackoffCooldown(tc.level); got != tc.want {
			t.Fatalf("BackoffCooldown(%d) = %s, want %s", tc.level, got, tc.want)
		}
	}
}

func TestClassifyBackoffCapRespectStillExponential(t *testing.T) {
	// A caller at level 15 that hits another backoff rule stays at 15
	// (capped) with the max cooldown.
	got := ClassifyUpstreamError("p", 429, "", BackoffMaxLevel)
	if got.NewBackoffLevel != BackoffMaxLevel {
		t.Fatalf("level cap: got %d, want %d", got.NewBackoffLevel, BackoffMaxLevel)
	}
	if got.Cooldown != BackoffMax {
		t.Fatalf("level cap cooldown = %s, want %s", got.Cooldown, BackoffMax)
	}
}

func TestClassifyUpstreamErrorString(t *testing.T) {
	if s := ClassifyUpstreamError("p", 0, "", 0).String(); s != "no-fallback" {
		t.Fatalf("String() no-match = %q", s)
	}
	if s := ClassifyUpstreamError("p", 401, "", 0).String(); s != "cooldown 2m0s" {
		t.Fatalf("String() 401 = %q", s)
	}
}

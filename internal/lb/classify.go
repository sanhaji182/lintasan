package lb

import (
	"fmt"
	"strings"
	"time"
)

// classify.go — B1 of the 9router parity plan (docs/plans/2026-10-18-9router-parity.md).
//
// Ports open-sse/config/errorConfig.js ERROR_RULES (9router v0.5.104) from a
// status-code-only cooldown classification into a text-first + status fallback
// classification, matching the REAL differentiation upstreams actually signal.
//
// WHY THIS EXISTS: the existing pool rejection path (oauth_proxy_wire.go
// markPoolAccountAuthFailed / proxy.go recordMultiAccountResult) only reacts to
// HTTP status. Upstreams signal far richer failure classes in the BODY — a 401
// with "no credentials" is a different recovery posture than a 403 with
// "request not allowed" (transient, clear in seconds), and a 429 with
// "quota exceeded" should back off exponentially rather than sit out a fixed
// short window. This file gives those callers one classify step so a provider's
// own wording, not just its numeric status, drives the cooldown.
//
// CONTRACT (mirrors 9router):
//   - Rules are checked TOP-TO-BOTTOM: text rules first (in listed order =
//     priority), then status rules.
//   - A text rule matches when the lowercased error text CONTAINS the rule's
//     substring.
//   - A provider-scoped rule (rule.Provider != nil) only applies to that
//     provider; nil matches every provider.
//   - Backoff rules return an exponential cooldown computed from the caller's
//     current backoff level (level+1); everything else returns its fixed
//     cooldown. TRANSIENT_COOLDOWN is the caller's business for unmatched
//     failures — Classify returns ShouldFallback=false and Cooldown=0 so the
//     caller keeps its existing default behaviour instead of inheriting
//     9router's notion of "transient" (which would silently change prod timing).

// Cooldown durations (B1 constants mirror errorConfig.js).
const (
	CooldownLong     = 2 * time.Minute
	CooldownShort    = 5 * time.Second
	BackoffBase      = 2 * time.Second
	BackoffMax       = 5 * time.Minute
	BackoffMaxLevel  = 15
	MaxRateLimitCool = 30 * time.Minute
)

// ErrorRule is one classification rule.
type ErrorRule struct {
	Provider string // empty = applies to all providers
	Text     string // substring match on lowercased error text
	Status   int    // HTTP status match (fallback when text rules miss)
	Cooldown time.Duration
	Backoff  bool // expiry uses exponential backoff, not a fixed cooldown
}

// TextRules and StatusRules are separate so the hot path scans text first
// (the actual 9router ordering) without mixing kinds in one slice.
var textRules = []ErrorRule{
	// --- Text rules (checked first; order = priority) ---
	{Provider: "codex", Text: "model is not supported when using codex with a chatgpt account", Cooldown: MaxRateLimitCool},
	{Text: "no credentials", Cooldown: CooldownLong},
	{Text: "request not allowed", Cooldown: CooldownShort},
	{Text: "improperly formed request", Cooldown: CooldownLong},
	{Text: "rate limit", Backoff: true},
	{Text: "too many requests", Backoff: true},
	{Text: "quota exceeded", Backoff: true},
	{Text: "insufficient_quota", Backoff: true},
	{Text: "capacity", Backoff: true},
	{Text: "overloaded", Backoff: true},
	{Text: "resource_exhausted", Backoff: true},
	{Text: "resource has been exhausted", Backoff: true},
	{Text: "model_capacity_exhausted", Backoff: true},
	{Text: "server is temporarily unavailable", Backoff: true},

	// --- Status rules (fallback when text doesn't match) ---
	{Status: 401, Cooldown: CooldownLong},
	{Status: 402, Cooldown: CooldownLong},
	{Status: 403, Cooldown: CooldownLong},
	{Status: 404, Cooldown: CooldownLong},
	{Status: 429, Backoff: true},
	{Status: 502, Backoff: true},
	{Status: 503, Backoff: true},
	{Status: 504, Backoff: true},
}

// Classification is the outcome of one ClassifyUpstreamError call.
//
// ShouldFallback=true always accompanies either a fixed Cooldown (retry after
// this long) or a Backoff flag + NewBackoffLevel (compute with BackoffCooldown).
// ShouldFallback=false means no rule matched — the caller keeps its existing
// default handling, this function does NOT invent a default.
type Classification struct {
	ShouldFallback  bool
	Cooldown        time.Duration
	Backoff         bool
	NewBackoffLevel int
}

// BackoffCooldown computes the exponential cooldown for a backoff level
// (level 1 → base, doubles per level, capped at BackoffMax).
func BackoffCooldown(level int) time.Duration {
	if level < 1 {
		level = 1
	}
	d := BackoffBase
	for i := 1; i < level; i++ {
		d *= 2
		if d >= BackoffMax {
			return BackoffMax
		}
	}
	if d > BackoffMax {
		return BackoffMax
	}
	return d
}

// ClassifyUpstreamError returns the recovery classification for one upstream
// failure. provider and errText may be empty; backoffLevel is the current
// exponential level (0 = no backoff recorded yet) and is only read by Backoff
// rules.
func ClassifyUpstreamError(provider string, status int, errText string, backoffLevel int) Classification {
	lower := strings.ToLower(strings.TrimSpace(errText))
	for i := range textRules {
		r := &textRules[i]
		if !ruleApplies(r, provider, status, lower) {
			continue
		}
		if r.Backoff {
			newLevel := backoffLevel + 1
			if newLevel > BackoffMaxLevel {
				newLevel = BackoffMaxLevel
			}
			return Classification{
				ShouldFallback:  true,
				Backoff:         true,
				Cooldown:        BackoffCooldown(newLevel),
				NewBackoffLevel: newLevel,
			}
		}
		return Classification{ShouldFallback: true, Cooldown: r.Cooldown}
	}
	return Classification{ShouldFallback: false}
}

// ruleApplies is the single match predicate so tests can assert order and
// scoping against the real table instead of a duplicate of it.
func ruleApplies(r *ErrorRule, provider string, status int, lowerErr string) bool {
	// Text rules: provider-scoped first, then substring.
	if r.Text != "" {
		if r.Provider != "" && !strings.EqualFold(r.Provider, provider) {
			return false
		}
		return lowerErr != "" && strings.Contains(lowerErr, r.Text)
	}
	// Status rules: match exactly; ignore provider scoping (none declared).
	return r.Status != 0 && r.Status == status
}

// String is for logs — keep it short, no secrets.
func (c Classification) String() string {
	if !c.ShouldFallback {
		return "no-fallback"
	}
	if c.Backoff {
		return fmt.Sprintf("backoff level %d for %s", c.NewBackoffLevel, c.Cooldown)
	}
	return fmt.Sprintf("cooldown %s", c.Cooldown)
}

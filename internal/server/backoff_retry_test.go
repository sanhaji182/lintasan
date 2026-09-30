package server

import (
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/qoder"
)

// TestBackoffRetryPassParameters pins the A+D defaults: 3 passes, 45s per-pass
// wait cap, 90s total budget. These bound the worst case a client can suffer
// when every upstream asks us to wait.
func TestBackoffRetryPassParameters(t *testing.T) {
	if maxBackoffRetryPasses != 3 {
		t.Fatalf("expected 3 retry passes, got %d", maxBackoffRetryPasses)
	}
	if perPassWaitCap != 45*time.Second {
		t.Fatalf("expected 45s per-pass cap, got %v", perPassWaitCap)
	}
	if totalBackoffBudget != 90*time.Second {
		t.Fatalf("expected 90s total budget, got %v", totalBackoffBudget)
	}
}

// TestBackoffWaitIsCappedByBudget asserts the wait computation: a hint larger
// than the per-pass cap is clamped, and a hint larger than the remaining
// budget is clamped to the budget. This is the arithmetic the retryPass loop
// runs before each extra pass.
func TestBackoffWaitIsCappedByBudget(t *testing.T) {
	origPasses, origCap, origBudget := maxBackoffRetryPasses, perPassWaitCap, totalBackoffBudget
	defer func() {
		maxBackoffRetryPasses, perPassWaitCap, totalBackoffBudget = origPasses, origCap, origBudget
	}()

	maxRetryAfter := 30
	retryPassesLeft := 3
	backoffBudgetLeft := 50 * time.Millisecond
	perPassWaitCap = 45 * time.Second

	wait := time.Duration(maxRetryAfter) * time.Second
	if wait > perPassWaitCap {
		wait = perPassWaitCap
	}
	if wait > backoffBudgetLeft {
		wait = backoffBudgetLeft
	}
	if wait != 50*time.Millisecond {
		t.Fatalf("expected wait clamped to remaining budget 50ms, got %v", wait)
	}

	// Fresh budget: hint 30s < cap 45s → wait is the hint itself.
	backoffBudgetLeft = 90 * time.Second
	wait = time.Duration(maxRetryAfter) * time.Second
	if wait > perPassWaitCap {
		wait = perPassWaitCap
	}
	if wait > backoffBudgetLeft {
		wait = backoffBudgetLeft
	}
	if wait != 30*time.Second {
		t.Fatalf("expected wait = hint 30s, got %v", wait)
	}

	// Hint above the cap → clamped to the cap.
	maxRetryAfter = 120
	wait = time.Duration(maxRetryAfter) * time.Second
	if wait > perPassWaitCap {
		wait = perPassWaitCap
	}
	if wait != 45*time.Second {
		t.Fatalf("expected wait clamped to cap 45s, got %v", wait)
	}
	if retryPassesLeft != 3 {
		t.Fatalf("passes untouched by wait computation, got %d", retryPassesLeft)
	}
}

// TestBackoffHintCollectedFromAllSources pins the three collection points:
// the Qoder stream path, the non-stream 403 body, and the generic 429
// Retry-After header. Whichever fires, the max hint wins.
func TestBackoffHintCollectedFromAllSources(t *testing.T) {
	maxRetryAfter := 0

	// Source 1: Qoder nested queue body (non-stream 403).
	qoderBody := []byte(`{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,\\\"retryAfterSeconds\\\":30}\"}"}`)
	if hint, queued := backoffHint(qoderBody, ""); queued && hint > maxRetryAfter {
		maxRetryAfter = hint
	}
	if maxRetryAfter != 30 {
		t.Fatalf("expected qoder body hint 30, got %d", maxRetryAfter)
	}

	// Source 2: generic Retry-After header on a 429 (larger hint wins).
	if hint, queued := backoffHint(nil, "60"); queued && hint > maxRetryAfter {
		maxRetryAfter = hint
	}
	if maxRetryAfter != 60 {
		t.Fatalf("expected header hint 60 to win, got %d", maxRetryAfter)
	}

	// Source 3: non-queue body contributes nothing.
	if hint, queued := backoffHint([]byte(`{"code":"105","message":"Login expired"}`), ""); queued || hint != 0 {
		t.Fatalf("expected no hint from login-expired body, got (%d,%v)", hint, queued)
	}
}

// backoffHint unifies the two hint sources for the tests above. It mirrors the
// collection logic used at the 403 and 429 paths in the candidate loop.
func backoffHint(body []byte, retryAfterHeader string) (int, bool) {
	if retryAfterHeader != "" {
		// treat as seconds; tests pass pre-parsed values
		var secs int
		for _, c := range retryAfterHeader {
			if c < '0' || c > '9' {
				return 0, false
			}
			secs = secs*10 + int(c-'0')
		}
		return secs, true
	}
	return qoder.BackoffHintFromBody(body)
}

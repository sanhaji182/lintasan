package server

// The dashboard Settings → "Rate limiting" toggle wrote settings.rate_limit_enabled
// and NOTHING in Go ever read it. Consequence measured in production on
// 2026-11-18: five concurrent conversations (~72 requests) were cut off with
//
//   429 {"error":{"message":"rate limit exceeded","type":"rate_limit"}}
//
// while the setting said false. These tests pin the setting to behaviour in both
// directions, and pin that flipping it takes effect WITHOUT a restart.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

func newRateLimitProxy(t *testing.T, setting string) (*ProxyHandler, *db.DB) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if setting != "" {
		if err := database.SetSetting(rateLimitEnabledSettingKey, setting); err != nil {
			t.Fatalf("set %s: %v", rateLimitEnabledSettingKey, err)
		}
	}
	return NewProxyHandler(&config.Config{}, database), database
}

// The toggle must be read at request time, not only at construction.
func TestRateLimitSettingIsHonoured(t *testing.T) {
	h, database := newRateLimitProxy(t, "false")
	if h.rlEnabled {
		// rlEnabled reflects the startup decision; with the setting false it must
		// be false already.
		t.Error("limiter armed despite rate_limit_enabled=false")
	}

	// Flip it back on at runtime: requests must start being limited again.
	if err := database.SetSetting(rateLimitEnabledSettingKey, "true"); err != nil {
		t.Fatal(err)
	}
	if !h.rateLimitSettingEnabled() {
		t.Error("turning the toggle ON had no effect")
	}
	if err := database.SetSetting(rateLimitEnabledSettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	if h.rateLimitSettingEnabled() {
		t.Error("turning the toggle OFF had no effect")
	}
}

// Unset keeps the historic default (limiter on) so an existing deployment does
// not silently lose the protection.
func TestRateLimitSettingDefaultsToEnabled(t *testing.T) {
	h, _ := newRateLimitProxy(t, "")
	if !h.rlEnabled {
		t.Fatal("limiter off with no setting: historic default is ON")
	}
	if !h.rateLimitSettingEnabled() {
		t.Fatal("rateLimitSettingEnabled() must default to true")
	}
}

// An unparseable value is not permission to disable a protection.
func TestRateLimitSettingIgnoresGarbage(t *testing.T) {
	h, _ := newRateLimitProxy(t, "maybe")
	if !h.rateLimitSettingEnabled() {
		t.Error("garbage setting disabled the limiter; default must stay enabled")
	}
}

// End-to-end: with the limiter ON, a burst beyond the window must actually be
// rejected; with it OFF, the same burst must pass. This is what the concurrent
// conversation test tripped over in production.
func TestRateLimitBurstRespectsSetting(t *testing.T) {
	if testing.Short() {
		t.Skip("burst test")
	}

	burst := func(setting string) int {
		h, database := newRateLimitProxy(t, setting)
		var rejected int
		for i := 0; i < 200; i++ {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
				strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer sk-burst")
			h.applyRateLimit(rec, req)
			if rec.Code == http.StatusTooManyRequests {
				rejected++
			}
		}
		_ = database
		return rejected
	}

	on := burst("true")
	off := burst("false")

	if on == 0 {
		t.Error("limiter ON rejected nothing: the limiter is not armed")
	}
	if off != 0 {
		t.Errorf("limiter OFF still rejected %d requests: the toggle is ignored", off)
	}
	t.Logf("rejected with limiter on=%d, off=%d", on, off)
}

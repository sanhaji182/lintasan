package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// handlers_responses_test.go — Codex M0 scaffolding tests.
//
// These lock the M0 contract: the route exists, is flag-gated (default OFF →
// 404), returns an honest 501 when ON (no fake success), and inherits the
// /v1/* auth boundary. Streaming + tool-loop behavior is M2/M3 and is NOT
// asserted here. See proposals/codex-build-plan.md.

// buildHandlerResponsesFlag builds a ProxyHandler after writing a raw value for
// responses_api_enabled, so we can exercise the flag-parsing contract. Mirrors
// buildHandlerWithFlag (proxy_provider_parity_test.go) but for the Codex flag.
func buildHandlerResponsesFlag(t *testing.T, raw string) *ProxyHandler {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if raw != "" {
		if err := database.SetSetting("responses_api_enabled", raw); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	return NewProxyHandler(&config.Config{}, database)
}

// TestResponsesFlagDefaultOff: with no setting, the Codex flag is false and the
// handler returns 404 (route inert — prod byte-identical to no-Responses build).
func TestResponsesFlagDefaultOff(t *testing.T) {
	p := buildHandlerResponsesFlag(t, "")
	if p.responsesAPI {
		t.Fatal("responsesAPI must default to false when setting is absent")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	p.HandleResponses(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("flag OFF must return 404, got %d", rec.Code)
	}
}

// TestResponsesFlagOnReturns501: with the flag ON, M0 returns 501 Not
// Implemented — an HONEST scaffolding response, never a fake 200. (Locked
// exclusion: no fake/no-op success.)
func TestResponsesFlagOnReturns501(t *testing.T) {
	p := buildHandlerResponsesFlag(t, "true")
	if !p.responsesAPI {
		t.Fatal("responsesAPI must be true when responses_api_enabled=true")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	p.HandleResponses(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("flag ON (M0) must return 501, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Lintasan-Ingress"); got != "responses" {
		t.Errorf("expected X-Lintasan-Ingress=responses, got %q", got)
	}
}

// TestResponsesFlagParsing checks the accepted truthy spellings and that
// everything else stays off (fail-safe), matching the established flag contract
// used by provider_sdk_enabled / capability_shadow_enabled / embedder_sdk_enabled.
func TestResponsesFlagParsing(t *testing.T) {
	truthy := []string{"true", "1", "on", "yes", "TRUE", "On", " yes "}
	for _, v := range truthy {
		t.Run("on/"+strings.TrimSpace(v), func(t *testing.T) {
			p := buildHandlerResponsesFlag(t, v)
			if !p.responsesAPI {
				t.Errorf("value %q should enable responses API", v)
			}
		})
	}
	falsy := []string{"", "false", "0", "off", "no", "nope", "enabled?"}
	for _, v := range falsy {
		t.Run("off/"+v, func(t *testing.T) {
			p := buildHandlerResponsesFlag(t, v)
			if p.responsesAPI {
				t.Errorf("value %q must NOT enable responses API", v)
			}
		})
	}
}

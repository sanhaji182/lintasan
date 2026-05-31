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

// TestResponsesFlagOnTranslatesAndValidates: M2 changed the flag-ON behavior.
// M0 returned 501 (scaffolding); M2 reaches the M1 request translator. With an
// empty body `{}` the translator rejects it (no model) → 400 with the sentinel
// message. This proves the ingress→translate wiring is live when the flag is ON
// (the streaming re-framing itself is covered by the emitter + adapter tests).
func TestResponsesFlagOnTranslatesAndValidates(t *testing.T) {
	p := buildHandlerResponsesFlag(t, "true")
	if !p.responsesAPI {
		t.Fatal("responsesAPI must be true when responses_api_enabled=true")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	p.HandleResponses(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("flag ON + invalid body must return 400 (translation reached), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "model") {
		t.Errorf("expected the no-model validation message, got %s", rec.Body.String())
	}
}

// TestResponsesFlagOnMalformedJSON: flag ON + non-JSON body → 400 before
// translation (parse error), never a panic or fake success.
func TestResponsesFlagOnMalformedJSON(t *testing.T) {
	p := buildHandlerResponsesFlag(t, "true")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`not json`))
	p.HandleResponses(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("flag ON + malformed JSON must return 400, got %d", rec.Code)
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

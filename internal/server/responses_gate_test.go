package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// responses_gate_test.go — locks the LIVE-READ contract for the Codex
// Responses kill-switch.
//
// The point of the gate is operational: an admin flipping the toggle in
// Dashboard → Settings → Experimental must take effect on the NEXT request,
// with no server restart, so an accidental enable is one click to undo.
// These tests fail if someone reverts the handler to the boot-time latch.

func newGateHandler(t *testing.T) (*ProxyHandler, *db.DB) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return NewProxyHandler(&config.Config{}, database), database
}

// responsesGateOpen reports whether the gate let the request through. It sends
// a request that is deliberately INVALID at the translation layer (no model),
// so an open gate answers 400 from the translator while a closed gate answers
// 404 from the kill-switch. That keeps the assertion about the gate alone and
// never reaches routing/upstream, where a 404 would be ambiguous.
func responsesGateOpen(t *testing.T, p *ProxyHandler) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"input":"hi"}`))
	rec := httptest.NewRecorder()
	p.HandleResponses(rec, req)
	if rec.Code == http.StatusNotFound {
		return false
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("gate open but unexpected status %d (want 400 from translator validation); body=%s",
			rec.Code, rec.Body.String())
	}
	return true
}

// TestResponsesGateLiveEnable: the surface is inert by default, and flipping the
// setting to true takes effect WITHOUT rebuilding the handler (no restart).
func TestResponsesGateLiveEnable(t *testing.T) {
	p, database := newGateHandler(t)

	if responsesGateOpen(t, p) {
		t.Fatal("default: gate is open — want inert (404)")
	}

	if err := database.SetSetting(responsesAPISettingKey, "true"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	// Same handler instance — no restart, no re-init.
	if !responsesGateOpen(t, p) {
		t.Fatal("after enabling: still inert — gate is not live-reading the setting")
	}
}

// TestResponsesGateLiveDisable: the critical direction — an admin turning the
// toggle OFF must immediately make the surface inert again.
func TestResponsesGateLiveDisable(t *testing.T) {
	p, database := newGateHandler(t)

	if err := database.SetSetting(responsesAPISettingKey, "true"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if !responsesGateOpen(t, p) {
		t.Fatal("enabled: gate unexpectedly inert")
	}

	if err := database.SetSetting(responsesAPISettingKey, "false"); err != nil {
		t.Fatalf("unset setting: %v", err)
	}
	if responsesGateOpen(t, p) {
		t.Fatal("after disabling: gate still open — want inert again")
	}
}

// TestResponsesGateParsing: the gate accepts the same boolean spellings as the
// other kill-switches, and an unparseable value falls back to the startup latch
// (which defaults false) rather than failing open.
func TestResponsesGateParsing(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"true", true}, {"TRUE", true}, {"1", true}, {"on", true}, {"yes", true},
		{"false", false}, {"0", false}, {"off", false}, {"no", false},
		{"banana", false}, {"", false},
	}
	for _, c := range cases {
		p, database := newGateHandler(t)
		if c.raw != "" {
			if err := database.SetSetting(responsesAPISettingKey, c.raw); err != nil {
				t.Fatalf("set %q: %v", c.raw, err)
			}
		}
		if got := p.responsesAPIEnabled(); got != c.want {
			t.Errorf("raw %q: want enabled=%v, got %v", c.raw, c.want, got)
		}
	}
}

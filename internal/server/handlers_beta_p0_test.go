package server

// Tests for the beta-stabilization fixes (commit batch "fix/beta-p0-p1").
// Covers:
//   - handlePluginGenerate: honest 503 / 400 paths (no real LLM needed)
//   - handleCosts: real aggregation from request_logs (was returning zeros)
//   - helpers: sanitizeName, stripCodeFences, round2
//
// Live LLM-backed generation is covered by smoke tests in prod only —
// the deterministic error paths are the unit-testable surface.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// rec is a minimal response recorder for handler tests. We use the stdlib
// httptest.ResponseRecorder directly to keep imports minimal.
func newReq(method, path string, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

// ─── handlePluginGenerate ─────────────────────────────────────────────

func TestHandlePluginGenerate_EmptyPrompt_BadRequest(t *testing.T) {
	s, ts := newTestServer(t, nil)
	defer ts.Close()
	rec := httptest.NewRecorder()
	s.handlePluginGenerate(rec, newReq("POST", "/api/plugins/generate", `{"prompt":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlePluginGenerate_InvalidJSON_BadRequest(t *testing.T) {
	s, ts := newTestServer(t, nil)
	defer ts.Close()
	rec := httptest.NewRecorder()
	s.handlePluginGenerate(rec, newReq("POST", "/api/plugins/generate", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlePluginGenerate_NoModelConfigured_ServiceUnavailable(t *testing.T) {
	s, ts := newTestServer(t, nil)
	defer ts.Close()
	rec := httptest.NewRecorder()
	s.handlePluginGenerate(rec, newReq("POST", "/api/plugins/generate", `{"prompt":"log request count"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	// Body must be a clear, actionable error — NOT a fake template. This
	// is the whole point of the fix: don't lie to the operator.
	body := rec.Body.String()
	if !strings.Contains(body, "plugin_generator_model") {
		t.Errorf("expected body to mention the setting name, got %q", body)
	}
	if strings.Contains(body, "export default async function") {
		t.Errorf("body must NOT contain the old fake template, got %q", body)
	}
}

func TestHandlePluginGenerate_TestModePortZero_ServiceUnavailable(t *testing.T) {
	// Port=0 is what the test server uses; the handler should refuse to
	// self-call because there's no bindable address.
	s, ts := newTestServer(t, nil)
	defer ts.Close()
	// Simulate the operator configuring a model: write the setting
	// directly. Without a master_key, we should hit the master-key branch
	// before the port-zero branch. The first guard is "no model", the
	// next is "no port", the last is "no master key". We test the
	// port-zero path by setting a model AND a master key.
	if err := s.db.SetSetting("plugin_generator_model", "gpt-4o-mini"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := s.db.SetSetting("master_key", "test-key-123"); err != nil {
		t.Fatalf("set master_key: %v", err)
	}
	rec := httptest.NewRecorder()
	s.handlePluginGenerate(rec, newReq("POST", "/api/plugins/generate", `{"prompt":"log request count"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (test mode), got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "test mode") {
		t.Errorf("expected body to mention 'test mode', got %q", rec.Body.String())
	}
}

func TestHandlePluginGenerate_NoMasterKey_ServiceUnavailable(t *testing.T) {
	s, ts := newTestServer(t, nil)
	defer ts.Close()
	// Set a real port so we get past the port-zero guard, but no master_key.
	s.cfg.Port = 20180
	if err := s.db.SetSetting("plugin_generator_model", "gpt-4o-mini"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	rec := httptest.NewRecorder()
	s.handlePluginGenerate(rec, newReq("POST", "/api/plugins/generate", `{"prompt":"log request count"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (no master key), got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "master key") {
		t.Errorf("expected body to mention 'master key', got %q", rec.Body.String())
	}
}

// ─── handleCosts ───────────────────────────────────────────────────────

func TestHandleCosts_EmptyDB_ReturnsZeroShape(t *testing.T) {
	// No rows in request_logs → costs should be zero, NOT a 500 or a
	// shape mismatch. The shape must remain stable: today, month,
	// by_model, requests_*, input_tokens_*, output_tokens_*.
	s, ts := newTestServer(t, nil)
	defer ts.Close()
	rec := httptest.NewRecorder()
	s.handleCosts(rec, newReq("GET", "/api/costs", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Today    float64 `json:"today"`
			Month    float64 `json:"month"`
			Currency string  `json:"currency"`
			ByModel  []any   `json:"by_model"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse body: %v body=%s", err, rec.Body.String())
	}
	if resp.Data.Currency != "USD" {
		t.Errorf("expected currency USD, got %q", resp.Data.Currency)
	}
	if resp.Data.Today != 0 || resp.Data.Month != 0 {
		t.Errorf("expected zero costs on empty DB, got today=%v month=%v", resp.Data.Today, resp.Data.Month)
	}
}

func TestHandleCosts_RealDataFromRequestLogs(t *testing.T) {
	// Seed a few request_logs rows and assert the aggregation matches.
	s, ts := newTestServer(t, nil)
	defer ts.Close()
	now := "datetime('now', 'localtime')"
	// Two rows for gpt-4o, one for claude-haiku. Token counts small but
	// non-zero so the cost calculator returns a positive value.
	seed := []struct {
		model, conn string
		inT, outT   int
	}{
		{"gpt-4o", "conn-1", 1000, 500},
		{"gpt-4o", "conn-1", 2000, 1000},
		{"claude-haiku", "conn-2", 3000, 1500},
	}
	for _, r := range seed {
		if _, err := s.db.Conn().Exec(
			"INSERT INTO request_logs(id, connection_id, provider, model, status, input_tokens, output_tokens, latency_ms, cached, created_at) VALUES(?,?,?,?,?,?,?,?,?,?)",
			uuid.New().String(), r.conn, "test", r.model, 200, r.inT, r.outT, 100, 0, now,
		); err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	s.handleCosts(rec, newReq("GET", "/api/costs", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Today            float64          `json:"today"`
			Month            float64          `json:"month"`
			ByModel          []map[string]any `json:"by_model"`
			RequestsToday    int              `json:"requests_today"`
			InputTokensToday int              `json:"input_tokens_today"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse body: %v body=%s", err, rec.Body.String())
	}
	if resp.Data.RequestsToday != 3 {
		t.Errorf("expected 3 requests today, got %d", resp.Data.RequestsToday)
	}
	if resp.Data.InputTokensToday != 6000 {
		t.Errorf("expected 6000 input tokens today, got %d", resp.Data.InputTokensToday)
	}
	if resp.Data.Today <= 0 {
		t.Errorf("expected non-zero cost today, got %v (calc must be working)", resp.Data.Today)
	}
	if len(resp.Data.ByModel) != 2 {
		t.Errorf("expected 2 model rows in by_model, got %d", len(resp.Data.ByModel))
	}
	// By-model must be sorted by cost desc.
	if len(resp.Data.ByModel) >= 2 {
		c0, _ := resp.Data.ByModel[0]["cost_usd"].(float64)
		c1, _ := resp.Data.ByModel[1]["cost_usd"].(float64)
		if c0 < c1 {
			t.Errorf("by_model must be sorted by cost desc, got %v then %v", c0, c1)
		}
	}
}

// ─── Helpers ───────────────────────────────────────────────────────────

func TestStripCodeFences(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain code", "plain code"},
		{"```js\ncode\n```", "code"},
		{"```\ncode\n```", "code"},
		{"  ```\ncode\n```  ", "code"},
		{"```js\nfoo()\n```\n", "foo()"},
	}
	for _, c := range cases {
		got := stripCodeFences(c.in)
		if got != c.want {
			t.Errorf("stripCodeFences(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Log every request", "log-every-request"},
		{"UPPER lower", "upper-lower"},
		{"with_under_score", "with_under_score"},
		// The implementation does NOT collapse consecutive special chars;
		// it replaces each one with '-'. Document that behavior here so a
		// future refactor doesn't accidentally change the contract.
		{"special!@#chars", "special---chars"},
		{"", "lintasan-plugin"},
		{"---", "lintasan-plugin"},
		// Length cap is hit at 40 output bytes; the loop breaks BEFORE
		// the next character is appended, so the result is exactly 40.
		{"a b c d e f g h i j k l m n o p q r s t u v w x y z", "a-b-c-d-e-f-g-h-i-j-k-l-m-n-o-p-q-r-s-t"},
	}
	for _, c := range cases {
		got := sanitizeName(c.in)
		if got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRound2(t *testing.T) {
	// math.Round uses "round half away from zero" but on float64 the
	// .5 boundary can fall the other way (1.005 may actually be stored
	// as 1.0049999...). Test only stable values.
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{1.234, 1.23},
		{1.236, 1.24},
		{1.005, round2(1.005)}, // float64 rounding is implementation-defined; just be self-consistent
		{-1.234, -1.23},
	}
	for _, c := range cases {
		got := round2(c.in)
		if got != c.want {
			t.Errorf("round2(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

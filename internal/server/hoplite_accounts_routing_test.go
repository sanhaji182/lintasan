package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestHopliteSecondAccountModelCatalogUsesQualifiedIDs(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"shared","name":"Shared"}]}`))
	}))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	token := makeKnownAdmin(t, s, "catalog-accounts-admin", "correct horse battery")
	_ = s.credStore().SetCredential(context.Background(), "hoplite", "hop_default")
	create := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Second","credential":"hop_second"}`, token)
	account := decodeEnvelope(t, create)["data"].(map[string]any)
	id := account["id"].(string)
	resp := hopliteRequest(t, ts, http.MethodGet, "/v1/models", "", token)
	body := mustJSON(t, decodeEnvelope(t, resp))
	if !strings.Contains(body, `"id":"hoplite-agent/shared"`) {
		t.Fatal("legacy model ID missing")
	}
	qualified := hopliteProjectModelIDForAccount(id, "shared")
	if !strings.Contains(body, `"id":"`+qualified+`"`) || !strings.Contains(body, "Second · Shared") {
		t.Fatalf("qualified model missing: %s", body)
	}
}

func TestHopliteComboSkipsUnhealthyAccountBeforeCreate(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	token := makeKnownAdmin(t, s, "unhealthy-combo-admin", "correct horse battery")
	create := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Bad","credential":"hop_bad"}`, token)
	id := decodeEnvelope(t, create)["data"].(map[string]any)["id"].(string)
	_, _ = s.db.Conn().Exec(`UPDATE hoplite_accounts SET health_status='unhealthy',last_tested_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
	model := hopliteProjectModelIDForAccount(id, "proj")
	combo := []any{map[string]any{"id": "bad-combo", "name": "bad-combo", "strategy": "priority", "entries": []any{map[string]any{"model": model, "connection_ids": []any{id}}}}}
	s.setJSONSetting("combos", combo)
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"bad-combo","messages":[{"role":"user","content":"work"}]}`, token)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if calls != 0 {
		t.Fatalf("unhealthy account contacted upstream %d times", calls)
	}
}

func TestHopliteExhaustedAndExpiredAccountsAreRejectedBeforeCreate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		update string
	}{
		{name: "exhausted", update: `credits_remaining=0`},
		{name: "expired", update: `expires_at='2020-01-01T00:00:00Z'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
			defer upstream.Close()
			s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
			s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
			token := makeKnownAdmin(t, s, "eligibility-"+tc.name, "correct horse battery")
			created := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Limited","credential":"hop_limited"}`, token)
			id := decodeEnvelope(t, created)["data"].(map[string]any)["id"].(string)
			_, _ = s.db.Conn().Exec(`UPDATE hoplite_accounts SET `+tc.update+` WHERE id=?`, id)
			model := hopliteProjectModelIDForAccount(id, "proj")
			resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"work"}]}`, token)
			resp.Body.Close()
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("status=%d", resp.StatusCode)
			}
			if calls != 0 {
				t.Fatalf("ineligible account contacted upstream %d times", calls)
			}
		})
	}
}

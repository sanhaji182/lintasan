package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestHopliteModelTestValidatesDefaultAndSecondaryCatalogWithoutCreatingThread(t *testing.T) {
	var projectCalls, threadCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/projects":
			projectCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "projects": []map[string]any{{"id": "shared", "name": "Shared"}}})
		default:
			threadCalls++
			http.Error(w, "unexpected mutating call", http.StatusInternalServerError)
		}
	}))
	defer upstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	token := makeKnownAdmin(t, s, "model-test-admin", "correct horse battery")
	if err := s.credStore().SetCredential(context.Background(), hopliteCredentialName, "hop_default"); err != nil {
		t.Fatal(err)
	}
	created := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Second","credential":"hop_second"}`, token)
	secondaryID := decodeEnvelope(t, created)["data"].(map[string]any)["id"].(string)

	for _, tc := range []struct {
		name, accountID, modelID string
	}{
		{name: "default", accountID: hopliteConnectionID, modelID: hopliteProjectModelIDForAccount(hopliteConnectionID, "shared")},
		{name: "secondary", accountID: secondaryID, modelID: hopliteSelectedModelIDForAccount(secondaryID, "shared", "meta/muse-spark-1.3")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"connection_id":"` + tc.accountID + `","model_id":"` + tc.modelID + `"}`
			resp := hopliteRequest(t, ts, http.MethodPost, "/api/models/test", body, token)
			result := decodeEnvelope(t, resp)
			if resp.StatusCode != http.StatusOK || result["success"] != true || result["status"] != "ok" {
				t.Fatalf("status=%d result=%#v", resp.StatusCode, result)
			}
			if result["thread_create"] != "not_tested" || !strings.Contains(result["message"].(string), "catalog") {
				t.Fatalf("unsafe or unclear result: %#v", result)
			}
		})
	}
	if projectCalls != 2 || threadCalls != 0 {
		t.Fatalf("project calls=%d thread/mutating calls=%d", projectCalls, threadCalls)
	}
}

func TestHopliteModelTestRejectsAccountMismatchWithoutUpstreamCall(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusOK) }))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	token := makeKnownAdmin(t, s, "model-mismatch-admin", "correct horse battery")
	if err := s.credStore().SetCredential(context.Background(), hopliteCredentialName, "hop_default"); err != nil {
		t.Fatal(err)
	}
	created := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Second","credential":"hop_second"}`, token)
	secondaryID := decodeEnvelope(t, created)["data"].(map[string]any)["id"].(string)

	modelID := hopliteProjectModelIDForAccount(secondaryID, "shared")
	resp := hopliteRequest(t, ts, http.MethodPost, "/api/models/test", `{"connection_id":"`+hopliteConnectionID+`","model_id":"`+modelID+`"}`, token)
	result := decodeEnvelope(t, resp)
	if result["success"] != false || result["status"] != "account_mismatch" {
		t.Fatalf("result=%#v", result)
	}
	if calls != 0 {
		t.Fatalf("mismatch contacted upstream %d times", calls)
	}
}

func TestHopliteModelTestPreservesAccountEligibilityWithoutUpstreamCall(t *testing.T) {
	for _, tc := range []struct {
		name, update, wantStatus string
	}{
		{name: "inactive", update: `is_active=0`, wantStatus: "account_unavailable"},
		{name: "unhealthy", update: `health_status='unhealthy'`, wantStatus: "account_unhealthy"},
		{name: "exhausted", update: `credits_remaining=0`, wantStatus: "account_exhausted"},
		{name: "expired", update: `expires_at='2020-01-01T00:00:00Z'`, wantStatus: "account_expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusOK) }))
			defer upstream.Close()
			s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
			s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
			token := makeKnownAdmin(t, s, "model-eligibility-"+tc.name, "correct horse battery")
			created := hopliteRequest(t, ts, http.MethodPost, "/api/experimental/cloud-agents/hoplite/accounts", `{"name":"Limited","credential":"hop_limited"}`, token)
			accountID := decodeEnvelope(t, created)["data"].(map[string]any)["id"].(string)
			if _, err := s.db.Conn().Exec(`UPDATE hoplite_accounts SET `+tc.update+`,updated_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), accountID); err != nil {
				t.Fatal(err)
			}
			modelID := hopliteProjectModelIDForAccount(accountID, "shared")
			resp := hopliteRequest(t, ts, http.MethodPost, "/api/models/test", `{"connection_id":"`+accountID+`","model_id":"`+modelID+`"}`, token)
			result := decodeEnvelope(t, resp)
			if result["success"] != false || result["status"] != tc.wantStatus {
				t.Fatalf("result=%#v", result)
			}
			if calls != 0 {
				t.Fatalf("ineligible account contacted upstream %d times", calls)
			}
		})
	}
}

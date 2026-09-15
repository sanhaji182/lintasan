package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/hoplite"
)

func TestConnectionsRepresentHopliteAsMaskedCloudAgent(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "connections-cloud-admin", "correct horse battery")
	const secret = "hop_org_live_secret_1234"
	if err := s.credStore().SetCredential(context.Background(), "hoplite", secret); err != nil {
		t.Fatal(err)
	}

	resp := hopliteRequest(t, ts, http.MethodGet, "/api/connections", "", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connections status=%d", resp.StatusCode)
	}
	encoded := mustJSON(t, decodeEnvelope(t, resp))
	for _, want := range []string{"hoplite-cloud-agent", `"provider_kind":"cloud_agent"`, `"credential_label":"Organization API Key"`, `"supports_streaming":false`, `"long_running":true`, `"project_scoped":true`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("connections response missing %q: %s", want, encoded)
		}
	}
	if strings.Contains(encoded, secret) {
		t.Fatal("connections response leaked plaintext Hoplite credential")
	}
}

func TestConnectionTestDispatchesHopliteVirtualConnection(t *testing.T) {
	var projectCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectCalls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/projects" {
			t.Fatalf("unexpected Hoplite probe: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"proj_1","name":"Acme"}]}`))
	}))
	defer upstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	token := makeKnownAdmin(t, s, "connection-test-cloud-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/api/connections/test", `{"id":"hoplite-cloud-agent"}`, token)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Hoplite virtual connection test status=%d body=%s", resp.StatusCode, body)
	}
	if projectCalls.Load() != 1 {
		t.Fatalf("Hoplite project probe calls=%d want=1", projectCalls.Load())
	}
	for _, want := range []string{`"success":true`, `"models_count":21`, `"project_count":1`, `"thread_create":"not_tested"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("Hoplite connection test response missing %s: %s", want, body)
		}
	}
}

func TestHopliteDiscoveredModelsExposeVirtualCatalog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"proj_1","name":"Acme"}]}`))
	}))
	defer upstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	token := makeKnownAdmin(t, s, "discovered-cloud-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodGet, "/api/models/discovered?connection_id=hoplite-cloud-agent", "", token)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Hoplite discovered models status=%d body=%s", resp.StatusCode, body)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	items, _ := envelope["data"].([]any)
	if len(items) != len(hoplite.Models())+1 {
		t.Fatalf("Hoplite discovered model count=%d want=%d body=%s", len(items), len(hoplite.Models())+1, body)
	}
	encoded := string(body)
	for _, want := range []string{"hoplite-agent/proj_1", "hoplite-model/v1/", `"provider_kind":"cloud_agent"`, `"supports_streaming":false`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("Hoplite discovered response missing %q: %s", want, body)
		}
	}
}

func TestHopliteBalanceExposesCreditsLimitAndDaysRemaining(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/billing/summary":
			_, _ = w.Write([]byte(`{"ok":true,"billing":{"available":true,"grantedCredits":300,"usedCredits":12.5,"remainingCredits":287.5,"nextResetAt":"2099-01-20T00:00:00Z"}}`))
		case "/api/billing/plan":
			_, _ = w.Write([]byte(`{"ok":true,"plan":{"plan":"pro","billingInterval":"annual","seatCount":3}}`))
		case "/api/billing/subscription":
			_, _ = w.Write([]byte(`{"ok":true,"subscription":{"status":"trialing","trialExpiresAt":"2099-01-20T00:00:00Z"}}`))
		default:
			t.Fatalf("unexpected billing path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	token := makeKnownAdmin(t, s, "balance-cloud-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodGet, "/api/connections/balances", "", token)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("balance status=%d body=%s", resp.StatusCode, body)
	}
	encoded := string(body)
	for _, want := range []string{`"id":"hoplite-cloud-agent"`, `"balance":"287.50 credits"`, `"total_used":"12.50 credits"`, `"plan_type":"pro · trialing"`, `"days_remaining":`, `"expires_at":"2099-01-20T00:00:00Z"`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("Hoplite balance missing %q: %s", want, body)
		}
	}
}

func TestHopliteMaskedCredentialPlaceholderNeverOverwritesStoredSecret(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "masked-cloud-admin", "correct horse battery")
	const secret = "hop_org_live_secret_1234"
	if err := s.credStore().SetCredential(context.Background(), "hoplite", secret); err != nil {
		t.Fatal(err)
	}
	resp := hopliteRequest(t, ts, http.MethodPut, "/api/experimental/credentials/hoplite", `{"credential":"hop_or**********1234"}`, token)
	if resp.StatusCode != http.StatusBadRequest {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("masked placeholder status=%d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
	if got, ok := s.credStore().GetCredential(context.Background(), "hoplite"); !ok || got != secret {
		t.Fatalf("stored credential changed: got=%q ok=%v", got, ok)
	}
}

func TestCloudAgentComboRejectsRoundRobinOnCreateAndPatch(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "combo-strategy-admin", "correct horse battery")
	create := `{"name":"cloud-mix","strategy":"round-robin","entries":[{"model":"hoplite-agent/proj_1","connection_ids":["hoplite-cloud-agent"]}]}`
	resp := hopliteRequest(t, ts, http.MethodPost, "/api/combos", create, token)
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("round-robin cloud combo create=%d want=400", resp.StatusCode)
	}
	resp.Body.Close()

	s.setJSONSetting("combos", []any{map[string]any{"id": "cloud-1", "name": "cloud-mix", "strategy": "priority", "entries": []any{map[string]any{"model": "hoplite-agent/proj_1", "connection_ids": []any{"hoplite-cloud-agent"}}}}})
	resp = hopliteRequest(t, ts, http.MethodPatch, "/api/routing/combos/cloud-1", `{"strategy":"round-robin"}`, token)
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("round-robin cloud combo patch=%d want=400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCloudAgentComboPreCreateFailureFallsBack(t *testing.T) {
	var creates atomic.Int32
	hopliteUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		creates.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ok":false,"error":{"code":"unavailable"}}`))
	}))
	defer hopliteUpstream.Close()
	llmUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fallback-ok","choices":[{"message":{"role":"assistant","content":"fallback worked"},"finish_reason":"stop"}]}`))
	}))
	defer llmUpstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = hopliteUpstream.URL, hopliteUpstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	_, err := s.db.Conn().Exec(`INSERT INTO connections(id,name,base_url,api_key,format,chat_path,is_active,priority,provider_kind) VALUES('llm-fallback','LLM fallback',?,'key','openai','/v1/chat/completions',1,10,'llm')`, llmUpstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Conn().Exec(`INSERT INTO discovered_models(id,connection_id,model_id,is_active) VALUES('fallback-model','llm-fallback','fallback-model',1)`)
	if err != nil {
		t.Fatal(err)
	}
	s.setJSONSetting("combos", []any{map[string]any{"id": "cloud-fallback", "name": "cloud-fallback", "strategy": "priority", "entries": []any{
		map[string]any{"model": "hoplite-agent/proj_1", "connection_ids": []any{"hoplite-cloud-agent"}},
		map[string]any{"model": "fallback-model", "connection_ids": []any{"llm-fallback"}},
	}}})
	s.proxy.cmb.LoadFromSettings(mustJSON(t, s.getJSONSetting("combos", []any{})))
	token := makeKnownAdmin(t, s, "combo-fallback-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"cloud-fallback","messages":[{"role":"user","content":"do it"}]}`, token)
	body := decodeEnvelope(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(mustJSON(t, body), "fallback worked") {
		t.Fatalf("fallback status=%d body=%#v", resp.StatusCode, body)
	}
	if creates.Load() == 0 {
		t.Fatal("Hoplite create was not attempted")
	}
}

func TestCloudAgentComboPostCreateFailureNeverFallsBack(t *testing.T) {
	var llmCalls atomic.Int32
	hopliteUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_accepted","projectId":"proj_1","status":"queued"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"thread":{"id":"thr_accepted","projectId":"proj_1","status":"failed"}}`))
	}))
	defer hopliteUpstream.Close()
	llmUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { llmCalls.Add(1); w.WriteHeader(200) }))
	defer llmUpstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = hopliteUpstream.URL, hopliteUpstream.Client()
	s.hoplitePollInterval, s.hopliteProxyTimeout = time.Millisecond, time.Second
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	_, _ = s.db.Conn().Exec(`INSERT INTO connections(id,name,base_url,api_key,format,chat_path,is_active,priority,provider_kind) VALUES('llm-never','LLM never',?,'key','openai','/v1/chat/completions',1,10,'llm')`, llmUpstream.URL)
	_, _ = s.db.Conn().Exec(`INSERT INTO discovered_models(id,connection_id,model_id,is_active) VALUES('never-model','llm-never','never-model',1)`)
	combo := []any{map[string]any{"id": "cloud-no-fallback", "name": "cloud-no-fallback", "strategy": "priority", "entries": []any{
		map[string]any{"model": "hoplite-agent/proj_1", "connection_ids": []any{"hoplite-cloud-agent"}},
		map[string]any{"model": "never-model", "connection_ids": []any{"llm-never"}},
	}}}
	s.setJSONSetting("combos", combo)
	s.proxy.cmb.LoadFromSettings(mustJSON(t, combo))
	token := makeKnownAdmin(t, s, "combo-no-fallback-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"cloud-no-fallback","messages":[{"role":"user","content":"do it"}]}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("post-create status=%d want=502", resp.StatusCode)
	}
	if llmCalls.Load() != 0 {
		t.Fatalf("post-create failure launched fallback %d times", llmCalls.Load())
	}
}

func TestStandardComboPathRemainsProxyOwned(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	combo := map[string]any{"name": "standard-only", "strategy": "round-robin", "entries": []any{map[string]any{"model": "plain-model", "connection_ids": []any{"plain"}}}}
	if comboContainsCloudAgent(s.db.Conn(), combo) {
		t.Fatal("standard combo incorrectly classified as cloud agent")
	}
}

func TestHopliteProjectModelsCarryCloudAgentCapabilities(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"projects":[{"id":"proj_1","name":"Acme"}]}`))
	}))
	defer upstream.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = upstream.URL, upstream.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	token := makeKnownAdmin(t, s, "catalog-cap-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodGet, "/v1/models", "", token)
	var envelope map[string]any
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	encoded := mustJSON(t, envelope)
	for _, want := range []string{`"provider_kind":"cloud_agent"`, `"supports_streaming":false`, `"long_running":true`, `"project_scoped":true`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("model catalog missing %q: %s", want, encoded)
		}
	}
}

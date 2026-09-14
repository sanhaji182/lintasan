package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestCloudAgentComboUsesConfiguredLLMConnectionID(t *testing.T) {
	var preferredCalls, otherCalls atomic.Int32
	preferred := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		preferredCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"preferred"},"finish_reason":"stop"}]}`))
	}))
	defer preferred.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherCalls.Add(1); w.WriteHeader(200) }))
	defer other.Close()
	hoplite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer hoplite.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = hoplite.URL, hoplite.Client()
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id, name, url string
		priority      int
	}{{"preferred", "Preferred", preferred.URL, 1}, {"other", "Other", other.URL, 100}} {
		_, err := s.db.Conn().Exec(`INSERT INTO connections(id,name,base_url,api_key,format,chat_path,is_active,priority,provider_kind) VALUES(?,?,?,'key','openai','/v1/chat/completions',1,?,'llm')`, row.id, row.name, row.url, row.priority)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.db.Conn().Exec(`INSERT INTO discovered_models(id,connection_id,model_id,is_active) VALUES(?,?,?,1)`, row.id+"-model", row.id, "fallback-model")
		if err != nil {
			t.Fatal(err)
		}
	}
	combo := []any{map[string]any{"name": "configured-fallback", "strategy": "priority", "entries": []any{
		map[string]any{"model": "hoplite-agent/proj_1", "connection_ids": []any{"hoplite-cloud-agent"}},
		map[string]any{"model": "fallback-model", "connection_ids": []any{"preferred"}},
	}}}
	s.setJSONSetting("combos", combo)
	token := makeKnownAdmin(t, s, "configured-fallback-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"configured-fallback","messages":[{"role":"user","content":"do it"}]}`, token)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || preferredCalls.Load() != 1 || otherCalls.Load() != 0 {
		t.Fatalf("status=%d preferred=%d other=%d", resp.StatusCode, preferredCalls.Load(), otherCalls.Load())
	}
}

func TestCloudAgentComboAmbiguousCreateFailureNeverFallsBack(t *testing.T) {
	var fallbackCalls atomic.Int32
	hoplite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("no hijacker")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}))
	defer hoplite.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fallbackCalls.Add(1); w.WriteHeader(200) }))
	defer fallback.Close()
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	s.hopliteBaseURL, s.hopliteHTTPClient = hoplite.URL, hoplite.Client()
	s.hopliteProxyTimeout = time.Second
	if err := s.credStore().SetCredential(context.Background(), "hoplite", "hop_test"); err != nil {
		t.Fatal(err)
	}
	_, _ = s.db.Conn().Exec(`INSERT INTO connections(id,name,base_url,api_key,format,chat_path,is_active,provider_kind) VALUES('fallback','Fallback',?,'key','openai','/v1/chat/completions',1,'llm')`, fallback.URL)
	combo := []any{map[string]any{"name": "ambiguous", "strategy": "priority", "entries": []any{
		map[string]any{"model": "hoplite-agent/proj_1", "connection_ids": []any{"hoplite-cloud-agent"}},
		map[string]any{"model": "fallback-model", "connection_ids": []any{"fallback"}},
	}}}
	s.setJSONSetting("combos", combo)
	token := makeKnownAdmin(t, s, "ambiguous-admin", "correct horse battery")
	resp := hopliteRequest(t, ts, http.MethodPost, "/v1/chat/completions", `{"model":"ambiguous","messages":[{"role":"user","content":"do it"}]}`, token)
	resp.Body.Close()
	if fallbackCalls.Load() != 0 {
		t.Fatalf("ambiguous acceptance launched fallback %d times", fallbackCalls.Load())
	}
}

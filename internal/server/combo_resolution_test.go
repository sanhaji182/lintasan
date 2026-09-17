package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestComboWithoutConnectionIDsResolvesAndWorks(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		model, _ := req["model"].(string)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 123456789,
			"model":   model,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "answer from " + model,
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "combo-user", "correct horse battery")

	// Register upstream connection and model
	_, err := s.db.Conn().Exec(`INSERT INTO connections(id,name,base_url,api_key,format,chat_path,is_active,priority,provider_kind) VALUES('conn-test','Test Conn',?,'testkey','openai','/v1/chat/completions',1,10,'llm')`, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Conn().Exec(`INSERT INTO discovered_models(id,connection_id,model_id,is_active) VALUES('m1','conn-test','model-alpha',1)`)
	if err != nil {
		t.Fatal(err)
	}

	// Configure combo "combo-fast" using simple models array (no explicit connection IDs)
	comboJSON := `[{"id":"c1","name":"combo-fast","strategy":"priority","models":["model-alpha"]}]`
	s.setJSONSetting("combos", []any{
		map[string]any{
			"id":       "c1",
			"name":     "combo-fast",
			"strategy": "priority",
			"models":   []any{"model-alpha"},
		},
	})
	_ = s.proxy.cmb.LoadFromSettings(comboJSON)

	// Call /v1/chat/completions with model="combo-fast"
	reqBody := `{"model":"combo-fast","messages":[{"role":"user","content":"ping"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, errBody)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	choices := data["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	content := msg["content"].(string)
	if !strings.Contains(content, "model-alpha") {
		t.Errorf("expected answer from model-alpha, got %q", content)
	}
}

func TestModelsEndpointIncludesCombos(t *testing.T) {
	s := newRESTTestServer(t)
	s.setJSONSetting("combos", []any{
		map[string]any{"name": "hemat", "strategy": "priority", "models": []any{"m1"}},
		map[string]any{"name": "cepat", "strategy": "round-robin", "models": []any{"m2"}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/models", nil)
	s.handleModels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	data := resp["data"].([]any)

	foundHemat := false
	foundCepat := false
	for _, item := range data {
		m := asMap(item)
		if m["id"] == "hemat" && m["source"] == "combo" {
			foundHemat = true
		}
		if m["id"] == "cepat" && m["source"] == "combo" {
			foundCepat = true
		}
	}

	if !foundHemat || !foundCepat {
		t.Errorf("expected hemat and cepat combos in /v1/models: hemat=%v, cepat=%v", foundHemat, foundCepat)
	}
}

func TestMotivationalQuoteBlockedByGuard(t *testing.T) {
	dummyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Simulates fake quote from broken upstream like bandelbanget
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-fake",
			"object":  "chat.completion",
			"created": 123456789,
			"model":   "claude-opus-5",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Actions speak louder than words.",
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer dummyUpstream.Close()

	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	token := makeKnownAdmin(t, s, "guard-user", "correct horse battery")

	_, err := s.db.Conn().Exec(`INSERT INTO connections(id,name,base_url,api_key,format,chat_path,is_active,priority,provider_kind) VALUES('conn-dummy','Dummy',?,'key','openai','/v1/chat/completions',1,10,'llm')`, dummyUpstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Conn().Exec(`INSERT INTO discovered_models(id,connection_id,model_id,is_active) VALUES('m-dummy','conn-dummy','fake-opus',1)`)
	if err != nil {
		t.Fatal(err)
	}

	reqBody := `{"model":"fake-opus","messages":[{"role":"user","content":"Berapa 7x8?"}]}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should NOT return HTTP 200 with motivational quote — must fail closed!
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("guard failed: dummy motivational quote was returned with HTTP 200")
	}
	if resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 502 or 503 for all routes failed, got %d", resp.StatusCode)
	}
}

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// TestCacheStale_ArithmeticDifferentPromptsNotCached proves that different
// arithmetic questions (e.g. 89*4 vs 94*9) do NOT collide on stale cache responses.
func TestCacheStale_ArithmeticDifferentPromptsNotCached(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer database.Close()

	// Upstream stub that returns the prompt echoed as answer
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		upstreamCalls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		msgs, _ := body["messages"].([]any)
		content := ""
		if len(msgs) > 0 {
			if lastMsg, ok := msgs[len(msgs)-1].(map[string]any); ok {
				content, _ = lastMsg["content"].(string)
			}
		}

		ans := "unknown"
		if strings.Contains(content, "89*4") {
			ans = "356"
		} else if strings.Contains(content, "94*9") {
			ans = "846"
		}

		resp := fmt.Sprintf(`{"choices":[{"message":{"content":"%s"}}]}`, ans)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(resp))
	}))
	defer upstream.Close()

	// Add connection and model
	addAuthConnection(t, database, "test-upstream", upstream.URL, 100)
	addAuthModel(t, database, "test-upstream", "test-model")

	p := NewProxyHandler(&config.Config{}, database)

	// 1. Request 1: 89*4
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"test-model","messages":[{"role":"user","content":"Berapa 89*4? Jawab hanya angkanya saja."}],"stream":false}`,
	))
	rec1 := httptest.NewRecorder()
	p.HandleChatCompletions(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("req1 status: %d body: %s", rec1.Code, rec1.Body.String())
	}
	if !strings.Contains(rec1.Body.String(), "356") {
		t.Errorf("expected 356 in req1, got: %s", rec1.Body.String())
	}

	// 2. Request 2: 94*9 (different numbers, similar sentence structure)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"test-model","messages":[{"role":"user","content":"Berapa 94*9? Jawab hanya angkanya saja."}],"stream":false}`,
	))
	rec2 := httptest.NewRecorder()
	p.HandleChatCompletions(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("req2 status: %d body: %s", rec2.Code, rec2.Body.String())
	}
	if strings.Contains(rec2.Body.String(), "356") {
		t.Fatalf("BUG REGRESSION: req2 returned stale response '356' instead of '846'!")
	}
	if !strings.Contains(rec2.Body.String(), "846") {
		t.Errorf("expected 846 in req2, got: %s", rec2.Body.String())
	}
	if rec2.Header().Get("X-Cache") == "HIT" {
		t.Errorf("expected req2 to be cache MISS, got HIT")
	}

	// 3. Request 3: EXACT same prompt as Request 1 -> should hit exact cache
	req3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"test-model","messages":[{"role":"user","content":"Berapa 89*4? Jawab hanya angkanya saja."}],"stream":false}`,
	))
	rec3 := httptest.NewRecorder()
	p.HandleChatCompletions(rec3, req3)

	if rec3.Code != http.StatusOK {
		t.Fatalf("req3 status: %d body: %s", rec3.Code, rec3.Body.String())
	}
	if !strings.Contains(rec3.Body.String(), "356") {
		t.Errorf("expected 356 in req3, got: %s", rec3.Body.String())
	}
	if rec3.Header().Get("X-Lintasan-Cache") != "EXACT-HIT" {
		t.Errorf("expected EXACT-HIT for req3, got: %s", rec3.Header().Get("X-Lintasan-Cache"))
	}

	// 4. Request 4: EXACT same prompt but with Cache-Control: no-cache -> should bypass cache
	req4 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
		`{"model":"test-model","messages":[{"role":"user","content":"Berapa 89*4? Jawab hanya angkanya saja."}],"stream":false}`,
	))
	req4.Header.Set("Cache-Control", "no-cache")
	rec4 := httptest.NewRecorder()
	p.HandleChatCompletions(rec4, req4)

	if rec4.Header().Get("X-Cache") != "BYPASS" {
		t.Errorf("expected BYPASS for req4 with Cache-Control: no-cache, got: %s", rec4.Header().Get("X-Cache"))
	}

	// Total upstream calls should be exactly 3 (req1, req2, req4; req3 was exact cache hit)
	if upstreamCalls != 3 {
		t.Errorf("expected 3 upstream calls, got %d", upstreamCalls)
	}
}

// TestCacheStale_GlobalDisabledPreventsAllCaching verifies that setting cache_enabled=false
// bypasses all cache lookups and writes.
func TestCacheStale_GlobalDisabledPreventsAllCaching(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer database.Close()

	_ = database.SetSetting("cache_enabled", "false")

	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()

	addAuthConnection(t, database, "test-conn", upstream.URL, 100)
	addAuthModel(t, database, "test-conn", "m")

	p := NewProxyHandler(&config.Config{}, database)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(
			`{"model":"m","messages":[{"role":"user","content":"identical prompt"}],"stream":false}`,
		))
		rec := httptest.NewRecorder()
		p.HandleChatCompletions(rec, req)

		if rec.Header().Get("X-Cache") == "HIT" {
			t.Fatalf("iteration %d: expected no cache hit when cache_enabled=false", i)
		}
	}

	if upstreamCalls != 3 {
		t.Errorf("expected 3 upstream calls when cache disabled, got %d", upstreamCalls)
	}
}

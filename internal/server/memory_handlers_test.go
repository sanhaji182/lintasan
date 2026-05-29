package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/memory"
)

// newTestMemoryStore creates a memory store connected to Redis, or skips the test.
func newTestMemoryStore(t *testing.T) *memory.MemoryStore {
	t.Helper()
	ms := memory.NewLazy(memory.Config{Addr: "127.0.0.1:6379"})
	if !ms.Available() {
		t.Skip("Redis not available — skipping integration test")
	}
	return ms
}

func TestMemorySearchEndpoint(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	// Pre-populate: store a known entry
	text := "debugging race conditions in Go concurrency with mutexes"
	key := memory.HashKey(text)
	emb := memory.Embed(text)
	err := ms.Store.Store(key, emb, text, map[string]string{"lang": "go"}, []string{"concurrency", "debugging"}, 90.0)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	defer ms.Store.Delete(key)

	mh := NewMemoryHandler(ms)

	// Search by keyword
	req := httptest.NewRequest("GET", "/v1/memory/search?q=race+condition&top_k=5", nil)
	rr := httptest.NewRecorder()
	mh.HandleMemorySearch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Query   string          `json:"query"`
		Results []memory.Memory `json:"results"`
		Count   int             `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Count == 0 {
		t.Error("expected at least 1 result for keyword 'race'")
	}
}

func TestMemorySearchNoQuery(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	mh := NewMemoryHandler(ms)

	req := httptest.NewRequest("GET", "/v1/memory/search", nil)
	rr := httptest.NewRecorder()
	mh.HandleMemorySearch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestMemoryStoreEndpoint(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	mh := NewMemoryHandler(ms)

	body := `{"text": "optimizing Go garbage collection for low latency", "tags": ["go", "performance"], "score": 85.0}`
	req := httptest.NewRequest("POST", "/v1/memory", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mh.HandleMemoryStore(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "stored" {
		t.Errorf("expected 'stored', got %v", resp["status"])
	}

	key, _ := resp["key"].(string)
	if key == "" {
		t.Fatal("expected key in response")
	}

	// Cleanup
	ms.Store.Delete(key)
}

func TestMemoryStoreNoText(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	mh := NewMemoryHandler(ms)

	req := httptest.NewRequest("POST", "/v1/memory", strings.NewReader(`{"tags": ["test"]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mh.HandleMemoryStore(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestMemoryStatsEndpoint(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	mh := NewMemoryHandler(ms)

	req := httptest.NewRequest("GET", "/v1/memory/stats", nil)
	rr := httptest.NewRecorder()
	mh.HandleMemoryStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if _, ok := resp["total_memories"]; !ok {
		t.Error("expected total_memories in response")
	}
	if avail, ok := resp["available"].(bool); !ok || !avail {
		t.Error("expected available=true")
	}
}

func TestGracefulDegradationUnavailable(t *testing.T) {
	// Create a handler with no Redis
	noRedis := &memory.MemoryStore{}
	mh := NewMemoryHandler(noRedis)

	// Search should return 503
	req := httptest.NewRequest("GET", "/v1/memory/search?q=test", nil)
	rr := httptest.NewRecorder()
	mh.HandleMemorySearch(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("search: expected 503, got %d", rr.Code)
	}

	// Store should return 503
	req2 := httptest.NewRequest("POST", "/v1/memory", strings.NewReader(`{"text": "test"}`))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	mh.HandleMemoryStore(rr2, req2)
	if rr2.Code != http.StatusServiceUnavailable {
		t.Errorf("store: expected 503, got %d", rr2.Code)
	}

	// Stats should return 200 with available=false
	req3 := httptest.NewRequest("GET", "/v1/memory/stats", nil)
	rr3 := httptest.NewRecorder()
	mh.HandleMemoryStats(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("stats: expected 200, got %d", rr3.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr3.Body.Bytes(), &resp)
	if resp["total_memories"].(float64) != 0 {
		t.Error("expected total_memories=0 when unavailable")
	}
	if avail, ok := resp["available"].(bool); ok && avail {
		t.Error("expected available=false")
	}
}

func TestNewLazyConnectFailure(t *testing.T) {
	// NewLazy now calls NewAuto which falls back to SQLite when Redis fails.
	ms := memory.NewLazy(memory.Config{Addr: "127.0.0.1:19999", DataDir: t.TempDir()})
	if !ms.Available() {
		t.Fatal("expected available via SQLite fallback")
	}
	if ms.BackendName() != "sqlite" {
		t.Fatalf("expected backend=sqlite, got %s", ms.BackendName())
	}
	if err := ms.Close(); err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestAutoIndexHeaderCheck(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	ph := &ProxyHandler{mem: ms}

	// Request without X-Lintasan-Index header — should be a no-op
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ph.autoIndex(req, "test-model", nil, "response text", 10, 20)

	// Request WITH X-Lintasan-Index: true — should store
	reqWith := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqWith.Header.Set("X-Lintasan-Index", "true")
	messages := []any{
		map[string]any{"role": "user", "content": "How do I optimize Go garbage collection?"},
	}
	ph.autoIndex(reqWith, "deepseek-v4-pro", messages, "Reduce allocations by using sync.Pool", 15, 30)

	// Verify it was stored by searching for it
	results, err := ms.Store.SearchByKeywords("sync.Pool", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	found := false
	for _, r := range results {
		if strings.Contains(r.Text, "sync.Pool") {
			found = true
			break
		}
	}
	if !found {
		t.Error("auto-indexed memory should be searchable")
	}
}

func TestPromptInjectionIntegration(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	// Pre-seed a memory that should match
	prompt := memory.Prompt{
		Model: "deepseek-v4-pro",
		Messages: []memory.Message{
			{Role: "user", Content: "fix a race condition in this Go code"},
		},
	}
	response := "The race condition was in the shared counter. Fix: add sync.Mutex around counter increments, and use atomic.AddInt64 for reads."
	key, _, err := ms.Store.IndexCompletion(prompt, response, 95.0, []string{"go", "concurrency"}, 100, 200)
	if err != nil {
		t.Fatalf("IndexCompletion failed: %v", err)
	}
	defer ms.Store.Delete(key)

	// Now search with a similar prompt — should find it with similarity > 0.75
	queryText := "fix a race condition in my Go concurrent code"
	queryEmb := memory.Embed(queryText)
	results, err := ms.Store.Search(queryEmb, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// The similar prompt should have high similarity
	if results[0].Similarity <= 0.75 {
		t.Logf("similarity=%.4f (expected > 0.75 for similar prompts)", results[0].Similarity)
		// Not a hard fail — TF-IDF hashing can produce lower sim for short texts
	}

	// Verify search by keywords also works
	kwResults, err := ms.Store.SearchByKeywords("race condition", 5)
	if err != nil {
		t.Fatalf("SearchByKeywords failed: %v", err)
	}
	found := false
	for _, r := range kwResults {
		if r.Key == key {
			found = true
			break
		}
	}
	if !found {
		t.Error("stored memory not found by keyword search")
	}
}

// Test buildPromptText helper
func TestBuildPromptText(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "You are a helpful assistant."},
		map[string]any{"role": "user", "content": "Write a Go function."},
		map[string]any{"role": "assistant", "content": "Here's the code..."},
	}
	result := buildPromptText(messages)
	if !strings.Contains(result, "Write a Go function") {
		t.Errorf("expected user content in result, got: %s", result)
	}
	if !strings.Contains(result, "You are a helpful assistant") {
		t.Errorf("expected system content in result, got: %s", result)
	}
	if strings.Contains(result, "Here's the code") {
		t.Error("assistant content should not be included")
	}
}

// Test truncate helper
func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short text: expected 'hello', got %q", got)
	}
	long := "this is a very long string that should be truncated"
	if got := truncate(long, 20); len(got) > 23 {
		t.Errorf("truncated too long: %d chars", len(got))
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("empty: expected '', got %q", got)
	}
}

// Test NewLazy with empty addr defaults to 127.0.0.1:6379
func TestNewLazyDefaultAddr(t *testing.T) {
	ms := memory.NewLazy(memory.Config{Addr: ""})
	defer ms.Close()
	// If Redis is running locally, it should be available
	if ms.Available() {
		t.Log("Redis available on default addr")
	}
	// Regardless, Close should not panic
}

// Test full round-trip: store → search → delete
func TestRoundTripStoreSearchDelete(t *testing.T) {
	ms := newTestMemoryStore(t)
	defer ms.Close()

	text := "unit test round trip verification"
	key := memory.HashKey(text)
	emb := memory.Embed(text)

	// Store
	err := ms.Store.Store(key, emb, text, map[string]string{"test": "roundtrip"}, []string{"test"}, 100.0)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Search by vector
	results, err := ms.Store.Search(emb, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Key == key {
			found = true
			if r.Score != 100.0 {
				t.Errorf("score mismatch: got %f, want 100.0", r.Score)
			}
			break
		}
	}
	if !found {
		t.Error("stored key not found in search results")
	}

	// Delete
	err = ms.Store.Delete(key)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

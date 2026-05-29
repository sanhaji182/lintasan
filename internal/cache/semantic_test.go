package cache

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestSemantic_InitCreatesTable(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := InitSemanticCache(db); err != nil {
		t.Fatalf("InitSemanticCache: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM semantic_cache").Scan(&count); err != nil {
		t.Fatalf("table not created: %v", err)
	}
}

func TestSemantic_SaveAndExactHit(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	messages := []any{
		map[string]any{"role": "system", "content": "You are a helpful assistant."},
		map[string]any{"role": "user", "content": "What's the capital of France?"},
	}
	response := `{"choices":[{"message":{"content":"Paris"}}]}`

	SaveSemanticMatch(db, "gpt-4", messages, response, 3600)

	resp, score, found := GetSemanticMatch(db, "gpt-4", messages, 0.75)
	if !found {
		t.Fatal("expected exact match hit")
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %.3f", score)
	}
	if resp != response {
		t.Errorf("response mismatch: got %s", resp)
	}
}

func TestSemantic_SimilarMatch(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	msgs1 := []any{
		map[string]any{"role": "user", "content": "build me a REST API with user authentication middleware"},
	}
	resp1 := `{"choices":[{"message":{"content":"Here's a FastAPI auth middleware example..."}}]}`

	SaveSemanticMatch(db, "gpt-4", msgs1, resp1, 3600)

	// Similar query — slightly different wording
	msgs2 := []any{
		map[string]any{"role": "user", "content": "build a REST API with auth middleware for users"},
	}

	resp, score, found := GetSemanticMatch(db, "gpt-4", msgs2, 0.75)
	if !found {
		t.Fatalf("expected semantic match, score=%.3f", score)
	}
	if score >= 1.0 {
		t.Error("should not be exact match for reworded query")
	}
	if score < 0.75 {
		t.Errorf("score too low: %.3f", score)
	}
	if resp != resp1 {
		t.Errorf("wrong response returned")
	}
}

func TestSemantic_NoMatchDifferentTopic(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	msgs1 := []any{
		map[string]any{"role": "user", "content": "build me a REST API with user authentication middleware"},
	}
	SaveSemanticMatch(db, "gpt-4", msgs1, "api response", 3600)

	msgs2 := []any{
		map[string]any{"role": "user", "content": "what is the best pizza place in Rome Italy"},
	}

	_, score, found := GetSemanticMatch(db, "gpt-4", msgs2, 0.75)
	if found {
		t.Error("should NOT match completely different topics")
	}
	if score > 0.75 {
		t.Errorf("score should be low for different topics: %.3f", score)
	}
}

func TestSemantic_ExpiredEntry(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	messages := []any{
		map[string]any{"role": "user", "content": "hello"},
	}
	SaveSemanticMatch(db, "gpt-4", messages, "hi there", 1) // 1 second TTL

	// Manually expire (SQLite datetime is UTC)
	db.Exec("UPDATE semantic_cache SET expires_at = datetime('now', '-10 seconds')")

	_, _, found := GetSemanticMatch(db, "gpt-4", messages, 0.75)
	if found {
		t.Error("should not find expired entry")
	}
}

func TestSemantic_HitsCounter(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	messages := []any{
		map[string]any{"role": "user", "content": "test counter"},
	}
	SaveSemanticMatch(db, "gpt-4", messages, "test response", 3600)

	// Hit it 3 times
	for i := 0; i < 3; i++ {
		GetSemanticMatch(db, "gpt-4", messages, 0.75)
	}

	var hits int
	db.QueryRow("SELECT hits FROM semantic_cache LIMIT 1").Scan(&hits)
	if hits != 3 {
		t.Errorf("expected 3 hits, got %d", hits)
	}
}

func TestSemantic_ModelIsolation(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	messages := []any{
		map[string]any{"role": "user", "content": "hello world"},
	}

	SaveSemanticMatch(db, "gpt-4", messages, "response from gpt-4", 3600)

	// Query with different model should NOT match
	_, _, found := GetSemanticMatch(db, "claude-3", messages, 0.75)
	if found {
		t.Error("should not match different model's cache")
	}
}

func TestSemantic_ClearExpired(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	// Fresh entry
	SaveSemanticMatch(db, "gpt-4", []any{
		map[string]any{"role": "user", "content": "fresh"},
	}, "fresh response", 3600)

	// Add entry then expire it
	SaveSemanticMatch(db, "gpt-4", []any{
		map[string]any{"role": "user", "content": "stale"},
	}, "stale response", 1)
	db.Exec("UPDATE semantic_cache SET expires_at = datetime('now', '-1 hour') WHERE response = 'stale response'")

	n, err := ClearExpiredSemantic(db)
	if err != nil {
		t.Fatalf("ClearExpiredSemantic: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 cleared, got %d", n)
	}

	// Fresh should still be there
	var count int
	db.QueryRow("SELECT COUNT(*) FROM semantic_cache").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 remaining, got %d", count)
	}
}

func TestSemantic_CosineSimilarity(t *testing.T) {
	a := map[string]int{"hello": 1, "world": 1}
	b := map[string]int{"hello": 1, "world": 1}
	s := cosineSimilarity(a, b)
	if s != 1.0 {
		t.Errorf("identical vectors: got %.3f", s)
	}

	c := map[string]int{"hello": 1, "there": 1}
	s = cosineSimilarity(a, c)
	if s <= 0 || s >= 1 {
		t.Errorf("partial overlap: expected 0<s<1, got %.3f", s)
	}

	d := map[string]int{"completely": 1, "unrelated": 1}
	s = cosineSimilarity(a, d)
	if s != 0 {
		t.Errorf("no overlap: expected 0, got %.3f", s)
	}
}

func TestGetSemanticMatch_EmptyMessages(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	_, _, found := GetSemanticMatch(db, "gpt-4", nil, 0.75)
	if found {
		t.Error("should not find anything with nil messages")
	}
}

func TestSaveSemanticMatch_EmptyResponse(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	SaveSemanticMatch(db, "gpt-4", []any{
		map[string]any{"role": "user", "content": "test"},
	}, "", 3600)

	var count int
	db.QueryRow("SELECT COUNT(*) FROM semantic_cache").Scan(&count)
	if count != 0 {
		t.Error("should not save empty response")
	}
}

func TestSaveSemanticMatch_DefaultTTL(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	db.SetMaxOpenConns(1)
	InitSemanticCache(db)

	messages := []any{
		map[string]any{"role": "user", "content": "default ttl test"},
	}
	SaveSemanticMatch(db, "gpt-4", messages, "response", 0) // ttl=0 → default 3600

	var expiresAt string
	db.QueryRow("SELECT expires_at FROM semantic_cache LIMIT 1").Scan(&expiresAt)
	if expiresAt == "" {
		t.Error("expires_at should be set")
	}
}

func TestBuildTF_Stopwords(t *testing.T) {
	tf := buildTF("the quick brown fox is jumping over the lazy dog")
	// "the", "is", "over" should be removed
	if _, ok := tf["the"]; ok {
		t.Error("stopword 'the' should be excluded")
	}
	if _, ok := tf["is"]; ok {
		t.Error("stopword 'is' should be excluded")
	}
	if v := tf["quick"]; v != 1 {
		t.Errorf("'quick' should be present: got %d", v)
	}
}

func TestStem_Ing(t *testing.T) {
	s := stem("running")
	if s != "runn" {
		t.Errorf("'running' should stem to 'runn', got '%s'", s)
	}
}

func TestStem_Tion(t *testing.T) {
	s := stem("authentication")
	if s != "authentic" {
		t.Errorf("'authentication' should stem to 'authentic', got '%s'", s)
	}
}

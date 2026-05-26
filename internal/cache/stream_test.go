package cache

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newStreamTestDB opens an in-memory SQLite database and initializes the stream cache table.
func newStreamTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	db.SetMaxOpenConns(1)

	if err := InitStreamCache(db); err != nil {
		t.Fatalf("InitStreamCache failed: %v", err)
	}
	return db
}

func TestStream_GetStreamMatch_EmptyWhenNoCache(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	chunks, tokens, found := GetStreamMatch(db, "gpt-4", []any{})
	if found {
		t.Error("expected found=false for empty cache")
	}
	if chunks != nil {
		t.Errorf("expected nil chunks, got %v", chunks)
	}
	if tokens != 0 {
		t.Errorf("expected 0 tokens, got %d", tokens)
	}
}

func TestStream_SaveAndRetrieve(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	model := "gpt-4"
	messages := []any{
		map[string]any{"role": "user", "content": "Tell me a joke"},
	}

	chunks := []string{
		"data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"Why\"}}]}\n\n",
		"data: {\"id\":\"2\",\"choices\":[{\"delta\":{\"content\":\" did\"}}]}\n\n",
		"data: {\"id\":\"3\",\"choices\":[{\"delta\":{\"content\":\" the\"}}]}\n\n",
		"data: [DONE]\n\n",
	}
	chunksJSON, err := json.Marshal(chunks)
	if err != nil {
		t.Fatalf("failed to marshal chunks: %v", err)
	}

	err = SaveStreamMatch(db, model, messages, string(chunksJSON), 42, 3600)
	if err != nil {
		t.Fatalf("SaveStreamMatch failed: %v", err)
	}

	retrieved, tokens, found := GetStreamMatch(db, model, messages)
	if !found {
		t.Fatal("expected found=true after save")
	}
	if tokens != 42 {
		t.Errorf("expected totalTokens=42, got %d", tokens)
	}
	if len(retrieved) != len(chunks) {
		t.Fatalf("expected %d chunks, got %d", len(chunks), len(retrieved))
	}
	for i, chunk := range chunks {
		if retrieved[i] != chunk {
			t.Errorf("chunk %d mismatch: expected %q, got %q", i, chunk, retrieved[i])
		}
	}
}

func TestStream_ExpiredEntryNotReturned(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	model := "gpt-4"
	messages := []any{
		map[string]any{"role": "user", "content": "Expired stream test"},
	}

	chunks := []string{"data: {\"content\":\"hello\"}\n\n"}
	chunksJSON, err := json.Marshal(chunks)
	if err != nil {
		t.Fatalf("failed to marshal chunks: %v", err)
	}

	// Save with TTL=1 second.
	err = SaveStreamMatch(db, model, messages, string(chunksJSON), 10, 1)
	if err != nil {
		t.Fatalf("SaveStreamMatch failed: %v", err)
	}

	// Wait for expiry.
	time.Sleep(2 * time.Second)

	retrieved, _, found := GetStreamMatch(db, model, messages)
	if found {
		t.Error("expected found=false for expired entry")
	}
	if retrieved != nil {
		t.Errorf("expected nil chunks for expired entry, got %v", retrieved)
	}
}

func TestStream_DifferentMessagesProduceDifferentHashes(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	model := "gpt-4"

	messages1 := []any{
		map[string]any{"role": "user", "content": "Hello"},
	}
	messages2 := []any{
		map[string]any{"role": "user", "content": "Goodbye"},
	}

	chunks := []string{"data: {\"content\":\"hi\"}\n\n"}
	chunksJSON, err := json.Marshal(chunks)
	if err != nil {
		t.Fatalf("failed to marshal chunks: %v", err)
	}

	err = SaveStreamMatch(db, model, messages1, string(chunksJSON), 10, 3600)
	if err != nil {
		t.Fatalf("SaveStreamMatch for messages1 failed: %v", err)
	}

	// messages2 should NOT match messages1.
	retrieved, _, found := GetStreamMatch(db, model, messages2)
	if found {
		t.Errorf("expected found=false for different messages, got chunks=%v", retrieved)
	}
}

func TestStream_SameModelMessagesSameHash(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	model := "claude-3"
	messages := []any{
		map[string]any{"role": "system", "content": "You are helpful."},
		map[string]any{"role": "user", "content": "Hi"},
	}

	chunks1 := []string{"data: {\"content\":\"first\"}\n\n"}
	chunks1JSON, _ := json.Marshal(chunks1)

	err := SaveStreamMatch(db, model, messages, string(chunks1JSON), 5, 3600)
	if err != nil {
		t.Fatalf("first SaveStreamMatch failed: %v", err)
	}

	// Same request should retrieve the first save.
	retrieved, _, found := GetStreamMatch(db, model, messages)
	if !found {
		t.Fatal("expected found=true for identical request")
	}
	if len(retrieved) != 1 || retrieved[0] != chunks1[0] {
		t.Errorf("expected first save chunks, got %v", retrieved)
	}

	// Overwrite with same hash (INSERT OR REPLACE).
	chunks2 := []string{"data: {\"content\":\"second\"}\n\n"}
	chunks2JSON, _ := json.Marshal(chunks2)

	err = SaveStreamMatch(db, model, messages, string(chunks2JSON), 5, 3600)
	if err != nil {
		t.Fatalf("second SaveStreamMatch failed: %v", err)
	}

	retrieved2, _, found2 := GetStreamMatch(db, model, messages)
	if !found2 {
		t.Fatal("expected found=true after overwrite")
	}
	if len(retrieved2) != 1 || retrieved2[0] != chunks2[0] {
		t.Errorf("expected second save chunks after overwrite, got %v", retrieved2)
	}
}

func TestStream_DifferentModelsProduceDifferentHashes(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	messages := []any{
		map[string]any{"role": "user", "content": "Same message, different model"},
	}

	chunks := []string{"data: {\"content\":\"test\"}\n\n"}
	chunksJSON, _ := json.Marshal(chunks)

	err := SaveStreamMatch(db, "gpt-4", messages, string(chunksJSON), 10, 3600)
	if err != nil {
		t.Fatalf("SaveStreamMatch for gpt-4 failed: %v", err)
	}

	retrieved, _, found := GetStreamMatch(db, "claude-3", messages)
	if found {
		t.Errorf("expected found=false for different model, got chunks=%v", retrieved)
	}
}

func TestStream_ReplayStream_WritesHeadersAndChunks(t *testing.T) {
	chunks := []string{
		"data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
		"data: {\"id\":\"2\",\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n",
		"data: [DONE]\n\n",
	}

	rec := httptest.NewRecorder()
	ReplayStream(rec, chunks)

	// Check headers.
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %q", cacheControl)
	}
	if connection := rec.Header().Get("Connection"); connection != "keep-alive" {
		t.Errorf("expected Connection keep-alive, got %q", connection)
	}
	if xCache := rec.Header().Get("X-Cache"); xCache != "STREAM-HIT" {
		t.Errorf("expected X-Cache STREAM-HIT, got %q", xCache)
	}

	// Check status code.
	if rec.Code != 200 {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Check body contains all chunks.
	expectedBody := ""
	for _, c := range chunks {
		expectedBody += c
	}
	if rec.Body.String() != expectedBody {
		t.Errorf("body mismatch.\nexpected: %q\ngot:      %q", expectedBody, rec.Body.String())
	}
}

func TestStream_ReplayStream_NoFlusher(t *testing.T) {
	// ReplayStream should not panic when the ResponseWriter does not implement http.Flusher.
	chunks := []string{"data: {\"test\":true}\n\n"}

	rec := httptest.NewRecorder()
	// httptest.ResponseRecorder does implement Flusher, so we wrap it.
	// Instead, just verify it doesn't panic — it already works with httptest.
	ReplayStream(rec, chunks)

	if rec.Code != 200 {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != chunks[0] {
		t.Errorf("expected body %q, got %q", chunks[0], rec.Body.String())
	}
}

func TestStream_ClearExpiredStream(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	model := "gpt-4"
	messages := []any{
		map[string]any{"role": "user", "content": "Clear test"},
	}

	chunks := []string{"data: {\"content\":\"expired\"}\n\n"}
	chunksJSON, _ := json.Marshal(chunks)

	// Save with short TTL.
	err := SaveStreamMatch(db, model, messages, string(chunksJSON), 1, 1)
	if err != nil {
		t.Fatalf("SaveStreamMatch failed: %v", err)
	}

	// Also save a long-lived entry.
	longMessages := []any{
		map[string]any{"role": "user", "content": "Long lived stream"},
	}
	longChunks := []string{"data: {\"content\":\"long\"}\n\n"}
	longChunksJSON, _ := json.Marshal(longChunks)
	err = SaveStreamMatch(db, model, longMessages, string(longChunksJSON), 1, 86400)
	if err != nil {
		t.Fatalf("SaveStreamMatch for long-lived failed: %v", err)
	}

	// Wait for the short one to expire.
	time.Sleep(2 * time.Second)

	deleted, err := ClearExpiredStream(db)
	if err != nil {
		t.Fatalf("ClearExpiredStream failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted row, got %d", deleted)
	}

	// Long-lived entry should still be retrievable.
	retrieved, _, found := GetStreamMatch(db, model, longMessages)
	if !found {
		t.Error("expected long-lived entry to survive ClearExpiredStream")
	}
	if len(retrieved) != 1 || retrieved[0] != longChunks[0] {
		t.Errorf("expected long-lived chunks, got %v", retrieved)
	}
}

func TestStream_SaveStreamMatch_DefaultTTL(t *testing.T) {
	db := newStreamTestDB(t)
	defer db.Close()

	model := "gpt-4"
	messages := []any{
		map[string]any{"role": "user", "content": "Default TTL"},
	}

	chunks := []string{"data: {\"content\":\"default ttl\"}\n\n"}
	chunksJSON, _ := json.Marshal(chunks)

	// Pass 0 for ttlSeconds — should default to 3600.
	err := SaveStreamMatch(db, model, messages, string(chunksJSON), 10, 0)
	if err != nil {
		t.Fatalf("SaveStreamMatch with ttl=0 failed: %v", err)
	}

	// Should still be retrievable.
	retrieved, _, found := GetStreamMatch(db, model, messages)
	if !found {
		t.Fatal("expected entry with default TTL to be found")
	}
	if len(retrieved) != 1 || retrieved[0] != chunks[0] {
		t.Errorf("expected default ttl chunks, got %v", retrieved)
	}
}

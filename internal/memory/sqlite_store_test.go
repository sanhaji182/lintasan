package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_memory.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	emb := Embed("hello world test")
	if err := store.Store("key1", emb, "hello world test", map[string]string{"source": "test"}, []string{"greeting", "test"}, 0.9); err != nil {
		t.Fatalf("Store key1: %v", err)
	}

	emb2 := Embed("Go programming language is fast")
	if err := store.Store("key2", emb2, "Go programming language is fast", nil, []string{"golang"}, 0.8); err != nil {
		t.Fatalf("Store key2: %v", err)
	}

	stats := store.Stats()
	if got := stats["backend"]; got != "sqlite" {
		t.Fatalf("backend = %v, want sqlite", got)
	}
	if got := stats["total_memories"]; got != 2 {
		t.Fatalf("total_memories = %v, want 2", got)
	}

	memories, total, err := store.List(10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(memories) != 2 {
		t.Fatalf("List count=%d len=%d, want 2/2", total, len(memories))
	}
	if memories[0].Key != "key2" {
		t.Fatalf("List order = %s, want key2 first", memories[0].Key)
	}

	results, err := store.SearchByKeywords("hello", 5)
	if err != nil {
		t.Fatalf("SearchByKeywords hello: %v", err)
	}
	if len(results) != 1 || results[0].Key != "key1" {
		t.Fatalf("SearchByKeywords hello got %+v", results)
	}

	results, err = store.Search(Embed("hello world"), 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 || results[0].Key != "key1" {
		t.Fatalf("Search got %+v", results)
	}

	if err := store.Delete("key1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := store.Stats()["total_memories"]; got != 1 {
		t.Fatalf("after delete total_memories = %v, want 1", got)
	}

	prompt := Prompt{Model: "gpt-4", Messages: []Message{{Role: "user", Content: "What is Go?"}}}
	key, emb3, err := store.IndexCompletion(prompt, "Go is a programming language", 0.95, []string{"gpt-4"}, 10, 20)
	if err != nil {
		t.Fatalf("IndexCompletion: %v", err)
	}
	if key == "" || len(emb3) != EmbeddingDim {
		t.Fatalf("IndexCompletion key=%q embLen=%d", key, len(emb3))
	}

	oldEmb := Embed("old memory")
	oldEmbJSON, _ := json.Marshal(oldEmb)
	if _, err := store.db.Exec(`INSERT OR REPLACE INTO memories (key, text, embedding, metadata, tags, score, hits, created_at) VALUES (?, ?, ?, '{}', '[]', 0, 0, ?)`,
		"old_key", "old memory", oldEmbJSON, time.Now().Add(-100*24*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("insert old memory: %v", err)
	}
	removed, err := store.Cleanup(90 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if removed != 1 {
		t.Fatalf("Cleanup removed=%d, want 1", removed)
	}

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("db file missing: %v", err)
	}
}

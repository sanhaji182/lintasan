package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements memory storage backed by SQLite.
type SQLiteStore struct {
	db        *sql.DB
	ftsEnabled bool
}

// NewSQLiteStore opens (or creates) a SQLite memory database at the given path.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			key        TEXT PRIMARY KEY,
			text       TEXT NOT NULL,
			embedding  BLOB,
			metadata   TEXT DEFAULT '{}',
			tags       TEXT DEFAULT '[]',
			score      REAL DEFAULT 0,
			hits       INTEGER DEFAULT 0,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at);
		CREATE INDEX IF NOT EXISTS idx_memories_score ON memories(score);
	`); err != nil {
		return err
	}

	// FTS5 is optional — not all SQLite builds include it.
	if _, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			key UNINDEXED,
			text,
			tags,
			content=memories,
			content_rowid=rowid
		);
	`); err == nil {
		s.ftsEnabled = true
		_, _ = s.db.Exec(`
			CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
				INSERT INTO memories_fts(rowid, key, text, tags) VALUES (new.rowid, new.key, new.text, new.tags);
			END;
		`)
		_, _ = s.db.Exec(`
			CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
				INSERT INTO memories_fts(memories_fts, rowid, key, text, tags) VALUES ('delete', old.rowid, old.key, old.text, old.tags);
			END;
		`)
		_, _ = s.db.Exec(`
			CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
				INSERT INTO memories_fts(memories_fts, rowid, key, text, tags) VALUES ('delete', old.rowid, old.key, old.text, old.tags);
				INSERT INTO memories_fts(rowid, key, text, tags) VALUES (new.rowid, new.key, new.text, new.tags);
			END;
		`)
		return nil
	}

	// Fallback: no FTS available, use LIKE queries.
	s.ftsEnabled = false
	return nil
}

// Store persists a memory entry into SQLite.
func (s *SQLiteStore) Store(key string, embedding []float64, text string, metadata map[string]string, tags []string, score float64) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	if len(embedding) == 0 {
		return fmt.Errorf("embedding must not be empty")
	}

	embBytes, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metaBytes, _ := json.Marshal(metadata)
	if tags == nil {
		tags = []string{}
	}
	tagsBytes, _ := json.Marshal(tags)

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO memories (key, text, embedding, metadata, tags, score, hits, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?)
	`, key, text, embBytes, string(metaBytes), string(tagsBytes), score, time.Now().UTC().Format(time.RFC3339))
	return err
}

// Search performs vector similarity search using cosine similarity.
func (s *SQLiteStore) Search(embedding []float64, topK int) ([]Memory, error) {
	if topK <= 0 {
		topK = 5
	}
	if len(embedding) != EmbeddingDim {
		return nil, fmt.Errorf("embedding must have %d dimensions, got %d", EmbeddingDim, len(embedding))
	}

	rows, err := s.db.Query(`SELECT key, text, embedding, metadata, tags, score, hits, created_at FROM memories`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct {
		mem        Memory
		similarity float64
	}
	var candidates []candidate

	for rows.Next() {
		var m Memory
		var embBytes []byte
		var metaStr, tagsStr, createdStr string

		if err := rows.Scan(&m.Key, &m.Text, &embBytes, &metaStr, &tagsStr, &m.Score, &m.Hits, &createdStr); err != nil {
			continue
		}

		var emb []float64
		if err := json.Unmarshal(embBytes, &emb); err != nil || len(emb) != EmbeddingDim {
			continue
		}

		sim := CosineSimilarity(embedding, emb)
		json.Unmarshal([]byte(metaStr), &m.Metadata)
		json.Unmarshal([]byte(tagsStr), &m.Tags)
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		m.Similarity = sim
		candidates = append(candidates, candidate{mem: m, similarity: sim})
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].similarity > candidates[j].similarity })
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	memories := make([]Memory, len(candidates))
	for i, c := range candidates {
		memories[i] = c.mem
	}
	return memories, nil
}

// SearchByKeywords performs keyword search on stored memories.
func (s *SQLiteStore) SearchByKeywords(query string, topK int) ([]Memory, error) {
	if topK <= 0 {
		topK = 5
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	if s.ftsEnabled {
		ftsQuery := strings.Join(strings.Fields(query), " OR ")
		rows, err := s.db.Query(`
			SELECT m.key, m.text, m.metadata, m.tags, m.score, m.hits, m.created_at
			FROM memories_fts f
			JOIN memories m ON f.key = m.key
			WHERE memories_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		`, ftsQuery, topK)
		if err == nil {
			defer rows.Close()
			var memories []Memory
			for rows.Next() {
				var m Memory
				var metaStr, tagsStr, createdStr string
				if err := rows.Scan(&m.Key, &m.Text, &metaStr, &tagsStr, &m.Score, &m.Hits, &createdStr); err != nil {
					continue
				}
				json.Unmarshal([]byte(metaStr), &m.Metadata)
				json.Unmarshal([]byte(tagsStr), &m.Tags)
				m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
				m.Similarity = 1.0
				memories = append(memories, m)
			}
			if len(memories) > 0 {
				return memories, nil
			}
		}
	}

	return s.searchByLike(query, topK)
}

func (s *SQLiteStore) searchByLike(query string, topK int) ([]Memory, error) {
	rows, err := s.db.Query(`
		SELECT key, text, metadata, tags, score, hits, created_at
		FROM memories
		WHERE lower(text) LIKE ? OR lower(tags) LIKE ?
		ORDER BY created_at DESC
		LIMIT ?
	`, "%"+strings.ToLower(query)+"%", "%"+strings.ToLower(query)+"%", topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var metaStr, tagsStr, createdStr string
		if err := rows.Scan(&m.Key, &m.Text, &metaStr, &tagsStr, &m.Score, &m.Hits, &createdStr); err != nil {
			continue
		}
		json.Unmarshal([]byte(metaStr), &m.Metadata)
		json.Unmarshal([]byte(tagsStr), &m.Tags)
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		m.Similarity = 1.0
		memories = append(memories, m)
	}
	return memories, nil
}

// IndexCompletion indexes a completed prompt→response pair.
func (s *SQLiteStore) IndexCompletion(prompt Prompt, response string, score float64, tags []string, promptTokens, completionTokens int) (string, []float64, error) {
	text := buildIndexText(prompt, response)
	key := HashKey(text)
	embedding := Embed(text)

	metadata := map[string]string{
		"text":              text,
		"response":          response,
		"prompt_text":       buildPromptText(prompt),
		"prompt_tokens":     fmt.Sprintf("%d", promptTokens),
		"completion_tokens": fmt.Sprintf("%d", completionTokens),
		"score":             fmt.Sprintf("%.2f", score),
	}

	if err := s.Store(key, embedding, text, metadata, tags, score); err != nil {
		return "", nil, fmt.Errorf("store: %w", err)
	}
	return key, embedding, nil
}

// Stats returns basic statistics about the memory store.
func (s *SQLiteStore) Stats() map[string]interface{} {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&count)
	var avgScore float64
	_ = s.db.QueryRow(`SELECT COALESCE(AVG(score), 0) FROM memories`).Scan(&avgScore)
	return map[string]interface{}{"total_memories": count, "avg_score": avgScore, "backend": "sqlite"}
}

// Delete removes a memory by key.
func (s *SQLiteStore) Delete(key string) error {
	_, err := s.db.Exec(`DELETE FROM memories WHERE key = ?`, key)
	return err
}

// List returns memories ordered by created_at desc, with pagination.
func (s *SQLiteStore) List(limit, offset int) ([]Memory, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&total)

	rows, err := s.db.Query(`
		SELECT key, text, metadata, tags, score, hits, created_at
		FROM memories
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var metaStr, tagsStr, createdStr string
		if err := rows.Scan(&m.Key, &m.Text, &metaStr, &tagsStr, &m.Score, &m.Hits, &createdStr); err != nil {
			continue
		}
		json.Unmarshal([]byte(metaStr), &m.Metadata)
		json.Unmarshal([]byte(tagsStr), &m.Tags)
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		memories = append(memories, m)
	}
	return memories, total, nil
}

// EnsureIndex is a no-op for SQLite.
func (s *SQLiteStore) EnsureIndex() error { return nil }

// Cleanup removes memories older than the given duration.
func (s *SQLiteStore) Cleanup(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`DELETE FROM memories WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// Close closes the SQLite database.
func (s *SQLiteStore) Close() error { return s.db.Close() }

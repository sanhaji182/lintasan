package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// InitStreamCache creates the stream_response_cache table if it does not already exist.
func InitStreamCache(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS stream_response_cache (
		hash TEXT PRIMARY KEY,
		model TEXT NOT NULL,
		chunks TEXT NOT NULL,
		total_tokens INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL
	)`)
	return err
}

// buildStreamHash computes a SHA-256 hash of the model + messages (no params, since streams don't have params).
func buildStreamHash(model string, messages []any) string {
	payload := map[string]any{
		"model":    model,
		"messages": messages,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		// Fallback: just hash model + raw messages if full marshaling fails.
		b, _ = json.Marshal(map[string]any{"model": model, "messages": messages})
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// GetStreamMatch returns stored chunks if an exact hash match exists and hasn't expired.
// Returns (chunks []string, totalTokens int, found bool).
func GetStreamMatch(db *sql.DB, model string, messages []any) ([]string, int, bool) {
	hash := buildStreamHash(model, messages)

	var chunksJSON string
	var totalTokens int
	err := db.QueryRow(
		"SELECT chunks, total_tokens FROM stream_response_cache WHERE hash=? AND expires_at > datetime('now')",
		hash,
	).Scan(&chunksJSON, &totalTokens)

	if err != nil {
		return nil, 0, false
	}

	var chunks []string
	if err := json.Unmarshal([]byte(chunksJSON), &chunks); err != nil {
		return nil, 0, false
	}

	return chunks, totalTokens, true
}

// SaveStreamMatch stores stream chunks in the cache.
// chunksJSON is a JSON array of SSE data lines.
func SaveStreamMatch(db *sql.DB, model string, messages []any, chunksJSON string, totalTokens int, ttlSeconds int) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}

	hash := buildStreamHash(model, messages)

	expiresAt := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second).Format("2006-01-02 15:04:05")

	_, err := db.Exec(
		`INSERT OR REPLACE INTO stream_response_cache
		 (hash, model, chunks, total_tokens, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		hash, model, chunksJSON, totalTokens, expiresAt,
	)
	return err
}

// ClearExpiredStream removes all expired entries from the stream_response_cache table.
// Returns the number of rows deleted.
func ClearExpiredStream(db *sql.DB) (int64, error) {
	result, err := db.Exec("DELETE FROM stream_response_cache WHERE expires_at <= datetime('now')")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ReplayStream writes cached SSE chunks to an http.ResponseWriter.
// Each chunk is a full "data: {...}\n\n" line.
func ReplayStream(w http.ResponseWriter, chunks []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Cache", "STREAM-HIT")
	w.WriteHeader(200)

	flusher, _ := w.(http.Flusher)
	for _, chunk := range chunks {
		w.Write([]byte(chunk))
		if flusher != nil {
			flusher.Flush()
		}
	}
}

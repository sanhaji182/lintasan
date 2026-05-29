package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true, "may": true,
	"might": true, "shall": true, "can": true, "in": true, "for": true, "on": true, "with": true,
	"at": true, "by": true, "from": true, "as": true, "to": true, "of": true, "and": true,
	"or": true, "if": true, "what": true, "which": true, "who": true, "this": true, "that": true,
	"it": true, "my": true, "your": true, "not": true, "but": true, "so": true, "just": true,
	"very": true, "also": true, "only": true, "then": true, "now": true, "all": true,
}

// InitSemanticCache creates the semantic_cache table if it does not already exist.
func InitSemanticCache(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS semantic_cache (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		model TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		messages_hash TEXT NOT NULL,
		response TEXT NOT NULL,
		hits INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL
	)`)
	if err != nil {
		return err
	}
	// Create index for fast model+expiry lookups
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_semantic_cache_model_expires 
		ON semantic_cache(model, expires_at)`)
	return nil
}

func stem(w string) string {
	if len(w) <= 4 {
		return w
	}
	suffixes := []struct{ old, new string }{
		{"ation", ""}, {"ment", ""}, {"ness", ""}, {"tion", ""}, {"sion", ""},
		{"able", ""}, {"ible", ""}, {"ally", ""}, {"less", ""},
		{"ious", "y"}, {"ous", ""}, {"ive", ""}, {"ful", ""},
		{"ies", "y"}, {"ing", ""}, {"ly", ""},
		{"er", ""}, {"ed", ""}, {"es", ""},
	}
	for _, s := range suffixes {
		if strings.HasSuffix(w, s.old) {
			result := w[:len(w)-len(s.old)] + s.new
			if len(result) >= 3 {
				return result
			}
			return w
		}
	}
	if strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3 {
		return w[:len(w)-1]
	}
	return w
}

func tokenize(text string) []string {
	clean := regexp.MustCompile(`[^\w\s]`).ReplaceAllString(strings.ToLower(text), " ")
	words := strings.Fields(clean)
	tokens := make([]string, 0, len(words))
	for _, w := range words {
		w = stem(w)
		if len(w) > 2 && !stopwords[w] {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

func buildTF(text string) map[string]int {
	tf := make(map[string]int)
	for _, t := range tokenize(text) {
		tf[t]++
	}
	return tf
}

// hashMessages computes SHA-256 of the full request (model + all messages).
func hashMessages(model string, messages []any) string {
	b, _ := json.Marshal(map[string]any{"model": model, "messages": messages})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// cosineSimilarity computes cosine similarity between two TF maps.
func cosineSimilarity(a, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	dot := 0.0
	magA := 0.0
	magB := 0.0

	for k, vA := range a {
		magA += float64(vA * vA)
		if vB, ok := b[k]; ok {
			dot += float64(vA * vB)
		}
	}
	for _, vB := range b {
		magB += float64(vB * vB)
	}

	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / math.Sqrt(magA*magB)
}

// GetSemanticMatch finds the best matching cached response using:
// 1. Exact hash match (full request identical) — instant return
// 2. TF-IDF cosine similarity on the last user message — threshold-based return
//
// threshold: cosine similarity threshold (0.75+ recommended, 0.92 = very strict)
func GetSemanticMatch(db *sql.DB, model string, messages []any, threshold float64) (string, float64, bool) {
	if len(messages) == 0 {
		return "", 0, false
	}

	// Extract last message content for fingerprinting
	lastMsg, ok := messages[len(messages)-1].(map[string]any)
	if !ok {
		return "", 0, false
	}
	content, _ := lastMsg["content"].(string)
	if content == "" {
		return "", 0, false
	}

	// Build TF fingerprint from last message
	tfQuery := buildTF(content)
	if len(tfQuery) == 0 {
		return "", 0, false
	}

	// Compute exact hash of entire request
	exactHash := hashMessages(model, messages)

	rows, err := db.Query(
		`SELECT id, fingerprint, messages_hash, response 
		 FROM semantic_cache 
		 WHERE model=? AND expires_at > datetime('now')`,
		model,
	)
	if err != nil {
		return "", 0, false
	}

	type candidate struct {
		id    int
		resp  string
		score float64
	}
	var matches []candidate
	var exactMatch string

	for rows.Next() {
		var id int
		var fp, hash, resp string
		if err := rows.Scan(&id, &fp, &hash, &resp); err != nil {
			continue
		}

		// Exact match: same model + same messages
		if hash == exactHash {
			exactMatch = resp
			matches = append(matches, candidate{id, resp, 1.0})
			break
		}

		// Semantic match: compare TF-IDF fingerprints
		var tfDoc map[string]int
		if json.Unmarshal([]byte(fp), &tfDoc) != nil {
			continue
		}

		score := cosineSimilarity(tfQuery, tfDoc)
		matches = append(matches, candidate{id, resp, score})
	}
	rows.Close()

	// Exact hit — update counter and return
	if exactMatch != "" {
		db.Exec("UPDATE semantic_cache SET hits=hits+1 WHERE id=?", matches[0].id)
		return exactMatch, 1.0, true
	}

	// Find best semantic match
	var best candidate
	for _, m := range matches {
		if m.score > best.score {
			best = m
		}
	}

	if best.score >= threshold {
		db.Exec("UPDATE semantic_cache SET hits=hits+1 WHERE id=?", best.id)
		return best.resp, best.score, true
	}
	return "", best.score, false
}

// SaveSemanticMatch saves a response to the semantic cache.
// ttlSecs: time-to-live in seconds (default 3600 = 1 hour).
func SaveSemanticMatch(db *sql.DB, model string, messages []any, response string, ttlSecs int) {
	if len(messages) == 0 || response == "" {
		return
	}

	// Build fingerprint from last message
	lastMsg, ok := messages[len(messages)-1].(map[string]any)
	if !ok {
		return
	}
	content, _ := lastMsg["content"].(string)
	tf := buildTF(content)
	if len(tf) == 0 {
		return
	}

	fpBytes, _ := json.Marshal(tf)
	msgHash := hashMessages(model, messages)

	if ttlSecs <= 0 {
		ttlSecs = 3600
	}

	db.Exec(
		`INSERT INTO semantic_cache (model, fingerprint, messages_hash, response, expires_at) 
		 VALUES (?, ?, ?, ?, datetime('now', ?))`,
		model, string(fpBytes), msgHash, response, fmt.Sprintf("+%d seconds", ttlSecs),
	)
}

// ClearExpiredSemantic removes all expired entries from the semantic_cache table.
func ClearExpiredSemantic(db *sql.DB) (int64, error) {
	result, err := db.Exec("DELETE FROM semantic_cache WHERE expires_at <= datetime('now')")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

package memory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// StoreManager handles persisting and querying memories via Redis.
// It implements the Backend interface.
type StoreManager struct {
	client *Client
}

// Ensure StoreManager implements Backend at compile time.
var _ Backend = (*StoreManager)(nil)

// NewStoreManager creates a new StoreManager.
func NewStoreManager(client *Client) *StoreManager {
	return &StoreManager{client: client}
}

// Store persists a memory entry as a Redis hash and adds it to the VSS index.
func (s *StoreManager) Store(key string, embedding []float64, text string, metadata map[string]string, tags []string, score float64) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	if len(embedding) == 0 {
		return fmt.Errorf("embedding must not be empty")
	}

	hashKey := "lintasan:mem:" + key

	embBytes, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	if metadata == nil {
		metadata = make(map[string]string)
	}

	metaBytes, _ := json.Marshal(metadata)
	tagsBytes, _ := json.Marshal(tags)

	_, err = s.client.Do("HSET", hashKey,
		"key", key,
		"embedding", string(embBytes),
		"text", text,
		"metadata", string(metaBytes),
		"tags", string(tagsBytes),
		"score", formatFloat(score),
		"hits", "0",
		"created_at", time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("HSET: %w", err)
	}

	// Set TTL: 90 days
	s.client.Do("EXPIRE", hashKey, "7776000")

	// VSET.ADD for vector similarity search (best effort)
	vsetArgs := []interface{}{"lintasan:memories", key, "VALUES"}
	for _, v := range embedding {
		vsetArgs = append(vsetArgs, formatFloat(v))
	}
	s.client.Do("VSET.ADD", vsetArgs...)

	return nil
}

// Search performs vector similarity search using VSET.SIM, falling back to local scan.
func (s *StoreManager) Search(embedding []float64, topK int) ([]Memory, error) {
	if topK <= 0 {
		topK = 5
	}
	if len(embedding) != EmbeddingDim {
		return nil, fmt.Errorf("embedding must have %d dimensions, got %d", EmbeddingDim, len(embedding))
	}

	recordSearchCall()

	// Try VSET.SIM first
	memories, err := s.searchViaVSet(embedding, topK)
	if err == nil && len(memories) > 0 {
		recordSearchHit()
		return memories, nil
	}

	// Fall back to local similarity scan
	memories, err = s.localSimilaritySearch(embedding, topK)
	if err == nil && len(memories) > 0 {
		recordSearchHit()
	}
	return memories, err
}

func (s *StoreManager) searchViaVSet(embedding []float64, topK int) ([]Memory, error) {
	args := []interface{}{"VSET.SIM", "lintasan:memories", "VALUES"}
	for _, v := range embedding {
		args = append(args, formatFloat(v))
	}
	args = append(args, "COUNT", topK, "WITHSCORES", "WITHVALUES")

	result, err := s.client.Do(args[0].(string), args[1:]...)
	if err != nil {
		return nil, fmt.Errorf("VSET.SIM: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected VSET.SIM response")
	}

	var memories []Memory
	for i := 0; i < len(resultSlice); i += 3 {
		if i+2 >= len(resultSlice) {
			break
		}
		key := respToString(resultSlice[i])
		scoreStr := respToString(resultSlice[i+1])

		m := Memory{Key: key}
		if sim, err := strconv.ParseFloat(scoreStr, 64); err == nil {
			m.Similarity = sim
		}

		// Fetch full data from hash
		hashKey := "lintasan:mem:" + key
		s.populateMemory(&m, hashKey)

		memories = append(memories, m)
		if len(memories) >= topK {
			break
		}
	}
	return memories, nil
}

// localSimilaritySearch scans all memory keys and computes cosine similarity locally.
//
// H3 hardening: this brute-force SCAN+cosine is O(n) over every stored key. We
// cap the scan at MaxScanRows and record counters so an oversized store can't
// silently dominate request latency. The VSET.SIM fast path in Search() is
// preferred; this only runs as a fallback when VSET is unavailable.
func (s *StoreManager) localSimilaritySearch(queryEmb []float64, topK int) ([]Memory, error) {
	var candidates []memCandidate
	var cursor uint64
	scanned := 0
	max := MaxScanRows()

	for {
		result, err := s.client.Do("SCAN", strconv.FormatUint(cursor, 10), "MATCH", "lintasan:mem:*", "COUNT", "100")
		if err != nil {
			return nil, err
		}
		parts, ok := result.([]interface{})
		if !ok || len(parts) != 2 {
			return nil, fmt.Errorf("unexpected SCAN response")
		}
		cursorStr := respToString(parts[0])
		cursor, _ = strconv.ParseUint(cursorStr, 10, 64)
		keys, _ := parts[1].([]interface{})

		for _, k := range keys {
			hashKey := respToString(k)
			key := strings.TrimPrefix(hashKey, "lintasan:mem:")

			// Fetch embedding
			embRaw, err := s.client.Do("HGET", hashKey, "embedding")
			if err != nil {
				continue
			}
			embBytes, ok := embRaw.([]byte)
			if !ok {
				continue
			}
			var emb []float64
			if err := json.Unmarshal(embBytes, &emb); err != nil {
				continue
			}
			if len(emb) != EmbeddingDim {
				continue
			}

			sim := CosineSimilarity(queryEmb, emb)
			candidates = append(candidates, memCandidate{key: key, similarity: sim, hashKey: hashKey})
			scanned++
		}

		// (3) hard cap: stop scanning once we've seen MaxScanRows keys.
		if max > 0 && scanned >= max {
			recordSearchCapped()
			break
		}

		if cursor == 0 {
			break
		}
	}
	recordSearchScanned(scanned)

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})

	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	var memories []Memory
	for _, c := range candidates {
		m := Memory{Key: c.key, Similarity: c.similarity}
		s.populateMemory(&m, c.hashKey)
		memories = append(memories, m)
	}

	return memories, nil
}

type memCandidate struct {
	key        string
	similarity float64
	hashKey    string
}

func (s *StoreManager) populateMemory(m *Memory, hashKey string) {
	allRaw, err := s.client.Do("HGETALL", hashKey)
	if err != nil {
		return
	}
	fieldPairs, ok := allRaw.([]interface{})
	if !ok {
		return
	}
	fieldMap := make(map[string]string)
	for j := 0; j < len(fieldPairs)-1; j += 2 {
		fieldMap[respToString(fieldPairs[j])] = respToString(fieldPairs[j+1])
	}

	m.Text = fieldMap["text"]
	if v, ok := fieldMap["key"]; ok && m.Key == "" {
		m.Key = v
	}
	json.Unmarshal([]byte(fieldMap["metadata"]), &m.Metadata)
	json.Unmarshal([]byte(fieldMap["embedding"]), &m.Embedding)
	if tagsStr := fieldMap["tags"]; tagsStr != "" {
		json.Unmarshal([]byte(tagsStr), &m.Tags)
	}
	if scoreStr := fieldMap["score"]; scoreStr != "" {
		fmt.Sscanf(scoreStr, "%f", &m.Score)
	}
	if hitsStr := fieldMap["hits"]; hitsStr != "" {
		fmt.Sscanf(hitsStr, "%d", &m.Hits)
	}
	if createdAtStr := fieldMap["created_at"]; createdAtStr != "" {
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}
}

// SearchByKeywords performs keyword-based text search on stored memories.
func (s *StoreManager) SearchByKeywords(query string, topK int) ([]Memory, error) {
	if topK <= 0 {
		topK = 5
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	queryLower := strings.ToLower(query)
	var memories []Memory
	var cursor uint64

	for {
		result, err := s.client.Do("SCAN", strconv.FormatUint(cursor, 10), "MATCH", "lintasan:mem:*", "COUNT", "100")
		if err != nil {
			return nil, err
		}
		parts, ok := result.([]interface{})
		if !ok || len(parts) != 2 {
			return nil, fmt.Errorf("unexpected SCAN response")
		}
		cursorStr := respToString(parts[0])
		cursor, _ = strconv.ParseUint(cursorStr, 10, 64)
		keys, _ := parts[1].([]interface{})

		for _, k := range keys {
			hashKey := respToString(k)
			key := strings.TrimPrefix(hashKey, "lintasan:mem:")

			allRaw, err := s.client.Do("HGETALL", hashKey)
			if err != nil {
				continue
			}
			fieldPairs, ok := allRaw.([]interface{})
			if !ok {
				continue
			}
			fieldMap := make(map[string]string)
			for j := 0; j < len(fieldPairs)-1; j += 2 {
				fieldMap[respToString(fieldPairs[j])] = respToString(fieldPairs[j+1])
			}

			text := strings.ToLower(fieldMap["text"])
			if !strings.Contains(text, queryLower) {
				continue
			}

			m := Memory{
				Key:  key,
				Text: fieldMap["text"],
			}
			if v, ok := fieldMap["key"]; ok && m.Key == "" {
				m.Key = v
			}
			json.Unmarshal([]byte(fieldMap["metadata"]), &m.Metadata)
			json.Unmarshal([]byte(fieldMap["embedding"]), &m.Embedding)
			if tagsStr := fieldMap["tags"]; tagsStr != "" {
				json.Unmarshal([]byte(tagsStr), &m.Tags)
			}
			if scoreStr := fieldMap["score"]; scoreStr != "" {
				fmt.Sscanf(scoreStr, "%f", &m.Score)
			}
			if hitsStr := fieldMap["hits"]; hitsStr != "" {
				fmt.Sscanf(hitsStr, "%d", &m.Hits)
			}
			if createdAtStr := fieldMap["created_at"]; createdAtStr != "" {
				m.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
			}
			m.Similarity = 1.0

			memories = append(memories, m)
			if len(memories) >= topK {
				return memories, nil
			}
		}

		if cursor == 0 {
			break
		}
	}

	return memories, nil
}

// IndexCompletion indexes a completed prompt→response pair.
func (s *StoreManager) IndexCompletion(prompt Prompt, response string, score float64, tags []string, promptTokens, completionTokens int) (string, []float64, error) {
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

func buildIndexText(prompt Prompt, response string) string {
	var b strings.Builder
	b.WriteString(prompt.Model)
	for _, msg := range prompt.Messages {
		b.WriteString(" ")
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Content)
	}
	if response != "" {
		b.WriteString(" response: ")
		b.WriteString(response)
	}
	return b.String()
}

func buildPromptText(prompt Prompt) string {
	var b strings.Builder
	for _, msg := range prompt.Messages {
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// Stats returns basic statistics about the memory store.
func (s *StoreManager) Stats() map[string]interface{} {
	var cursor uint64
	count := 0

	for {
		result, err := s.client.Do("SCAN", strconv.FormatUint(cursor, 10), "MATCH", "lintasan:mem:*", "COUNT", "100")
		if err != nil {
			break
		}
		parts, ok := result.([]interface{})
		if !ok || len(parts) != 2 {
			break
		}
		cursorStr := respToString(parts[0])
		cursor, _ = strconv.ParseUint(cursorStr, 10, 64)
		keys, _ := parts[1].([]interface{})
		count += len(keys)
		if cursor == 0 {
			break
		}
	}

	return map[string]interface{}{
		"total_memories": count,
	}
}

// Delete removes a memory by key from Redis and the VSS index.
func (s *StoreManager) Delete(key string) error {
	hashKey := "lintasan:mem:" + key
	s.client.Do("VSET.DEL", "lintasan:memories", key)
	_, err := s.client.Do("DEL", hashKey)
	return err
}

// List returns all memories ordered by created_at desc, with pagination.
func (s *StoreManager) List(limit, offset int) ([]Memory, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	// Scan all keys
	var allKeys []string
	var cursor uint64
	for {
		result, err := s.client.Do("SCAN", strconv.FormatUint(cursor, 10), "MATCH", "lintasan:mem:*", "COUNT", "200")
		if err != nil {
			return nil, 0, err
		}
		parts, ok := result.([]interface{})
		if !ok || len(parts) != 2 {
			break
		}
		cursorStr := respToString(parts[0])
		cursor, _ = strconv.ParseUint(cursorStr, 10, 64)
		keys, _ := parts[1].([]interface{})
		for _, k := range keys {
			allKeys = append(allKeys, respToString(k))
		}
		if cursor == 0 {
			break
		}
	}

	total := len(allKeys)

	// Fetch all memories to sort by created_at
	var all []Memory
	for _, hashKey := range allKeys {
		m := Memory{Key: strings.TrimPrefix(hashKey, "lintasan:mem:")}
		s.populateMemory(&m, hashKey)
		all = append(all, m)
	}

	// Sort by created_at desc
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	// Paginate
	if offset >= len(all) {
		return []Memory{}, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

// EnsureIndex is a no-op stub (index creation is not required when using VSET).
func (s *StoreManager) EnsureIndex() error {
	return nil
}

// HashKey generates a deterministic SHA-256 key from text.
func HashKey(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

// IndexScore computes a normalized score: (passed / total) * 100.
func IndexScore(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return (float64(passed) / float64(total)) * 100.0
}

// formatFloat formats a float64 with minimal trailing zeros.
func formatFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
}

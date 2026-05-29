package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/memory"
)

// MemoryHandler holds a reference to the memory store for HTTP handler methods.
type MemoryHandler struct {
	mem *memory.MemoryStore
}

// NewMemoryHandler creates a new MemoryHandler.
func NewMemoryHandler(mem *memory.MemoryStore) *MemoryHandler {
	return &MemoryHandler{mem: mem}
}

// HandleMemorySearch handles GET /v1/memory/search?q=...&top_k=5
// Searches stored memories by keyword using string matching on text field.
func (mh *MemoryHandler) HandleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if mh.mem == nil || !mh.mem.Available() {
		writeMemoryJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "memory service unavailable — Redis not connected",
		})
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeMemoryJSON(w, http.StatusBadRequest, map[string]any{
			"error": "query parameter 'q' is required",
		})
		return
	}

	topK := 5
	if tk := r.URL.Query().Get("top_k"); tk != "" {
		if n, err := strconv.Atoi(tk); err == nil && n > 0 && n <= 50 {
			topK = n
		}
	}

	results, err := mh.mem.Store.SearchByKeywords(q, topK)
	if err != nil {
		writeMemoryJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("search failed: %v", err),
		})
		return
	}

	if results == nil {
		results = []memory.Memory{}
	}

	// Strip embeddings from response
	clean := make([]memory.Memory, len(results))
	for i, r := range results {
		clean[i] = r.WithoutEmbedding()
	}

	writeMemoryJSON(w, http.StatusOK, map[string]any{
		"query":   q,
		"results": clean,
		"count":   len(clean),
	})
}

// HandleMemoryStore handles POST /v1/memory
// Manually stores a text entry with optional metadata and tags.
func (mh *MemoryHandler) HandleMemoryStore(w http.ResponseWriter, r *http.Request) {
	if mh.mem == nil || !mh.mem.Available() {
		writeMemoryJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "memory service unavailable — Redis not connected",
		})
		return
	}

	var req struct {
		Text     string            `json:"text"`
		Metadata map[string]string `json:"metadata,omitempty"`
		Tags     []string          `json:"tags,omitempty"`
		Score    float64           `json:"score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMemoryJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid JSON body",
		})
		return
	}
	if req.Text == "" {
		writeMemoryJSON(w, http.StatusBadRequest, map[string]any{
			"error": "field 'text' is required",
		})
		return
	}

	embedding := memory.Embed(req.Text)
	key := memory.HashKey(req.Text)

	err := mh.mem.Store.Store(key, embedding, req.Text, req.Metadata, req.Tags, req.Score)
	if err != nil {
		writeMemoryJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("store failed: %v", err),
		})
		return
	}

	writeMemoryJSON(w, http.StatusCreated, map[string]any{
		"key":    key,
		"status": "stored",
	})
}

// HandleMemoryStats handles GET /v1/memory/stats
// Returns index statistics: total entries, avg score, breakdown by tag.
func (mh *MemoryHandler) HandleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if mh.mem == nil || !mh.mem.Available() {
		writeMemoryJSON(w, http.StatusOK, map[string]any{
			"total_memories": 0,
			"available":      false,
			"backend":        "none",
		})
		return
	}

	baseStats := mh.mem.Store.Stats()
	total, _ := baseStats["total_memories"].(int)
	avgScore, _ := baseStats["avg_score"].(float64)

	stats := map[string]any{
		"total_memories": total,
		"available":      true,
		"backend":        mh.mem.BackendName(),
		"avg_score":      avgScore,
	}

	writeMemoryJSON(w, http.StatusOK, stats)
}

// writeMemoryJSON writes a JSON response with standard headers.
func writeMemoryJSON(w http.ResponseWriter, status int, data map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// HandleMemoryDelete handles DELETE /v1/memory/{key}
func (mh *MemoryHandler) HandleMemoryDelete(w http.ResponseWriter, r *http.Request) {
	if mh.mem == nil || !mh.mem.Available() {
		writeMemoryJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "memory service unavailable",
		})
		return
	}

	key := r.PathValue("key")
	if key == "" {
		writeMemoryJSON(w, http.StatusBadRequest, map[string]any{
			"error": "key is required",
		})
		return
	}

	if err := mh.mem.Store.Delete(key); err != nil {
		writeMemoryJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("delete failed: %v", err),
		})
		return
	}

	writeMemoryJSON(w, http.StatusOK, map[string]any{
		"key":    key,
		"status": "deleted",
	})
}

// HandleMemoryList handles GET /v1/memory?limit=20&offset=0
func (mh *MemoryHandler) HandleMemoryList(w http.ResponseWriter, r *http.Request) {
	if mh.mem == nil || !mh.mem.Available() {
		writeMemoryJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "memory service unavailable",
		})
		return
	}

	limit := 20
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	memories, total, err := mh.mem.Store.List(limit, offset)
	if err != nil {
		writeMemoryJSON(w, http.StatusInternalServerError, map[string]any{
			"error": fmt.Sprintf("list failed: %v", err),
		})
		return
	}
	if memories == nil {
		memories = []memory.Memory{}
	}
	// Strip embeddings from response
	clean := make([]memory.Memory, len(memories))
	for i, m := range memories {
		clean[i] = m.WithoutEmbedding()
	}
	writeMemoryJSON(w, http.StatusOK, map[string]any{
		"memories": clean,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"backend":  mh.mem.BackendName(),
	})
}

package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// GET /api/dashboard/stats
func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	var totalRequests, activeConnections int
	var avgLatency sql.NullFloat64

	s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&totalRequests)
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM connections WHERE is_active = 1").Scan(&activeConnections)
	s.db.Conn().QueryRow("SELECT AVG(latency_ms) FROM request_logs WHERE status = 200").Scan(&avgLatency)

	// Cache hit rate
	var cachedHits int
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs WHERE cached = 1").Scan(&cachedHits)
	cacheRate := 0.0
	if totalRequests > 0 {
		cacheRate = float64(cachedHits) / float64(totalRequests) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total_requests":     totalRequests,
		"active_connections": activeConnections,
		"cache_hit_rate":     cacheRate,
		"avg_latency":        avgLatency.Float64,
		"uptime":             time.Since(s.startTime).String(),
	})
}

// GET /api/dashboard/connections
func (s *Server) handleDashboardConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query(`
		SELECT id, name, base_url, format, is_active, priority, models_count
		FROM connections
		ORDER BY priority DESC, created_at DESC
	`)
	if err != nil {
		http.Error(w, `{"error":"failed to query connections"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ConnInfo struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		BaseURL     string `json:"base_url"`
		Format      string `json:"format"`
		IsActive    bool   `json:"is_active"`
		Priority    int    `json:"priority"`
		ModelsCount int    `json:"models_count"`
		LatencyEWMA *int   `json:"latency_ewma,omitempty"`
	}

	var conns []ConnInfo
	for rows.Next() {
		var c ConnInfo
		var isActive int
		rows.Scan(&c.ID, &c.Name, &c.BaseURL, &c.Format, &isActive, &c.Priority, &c.ModelsCount)
		c.IsActive = isActive == 1
		conns = append(conns, c)
	}

	if conns == nil {
		conns = []ConnInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conns)
}

// GET /api/dashboard/cache
func (s *Server) handleDashboardCache(w http.ResponseWriter, r *http.Request) {
	// Count from request_logs by cache type
	var exactHits, streamHits, semanticHits, misses int

	// exact cache hits: cached=1 in request_logs
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs WHERE cached = 1").Scan(&exactHits)

	// stream cache: count from embedding_cache hits
	var streamCount int
	s.db.Conn().QueryRow("SELECT COALESCE(SUM(hits), 0) FROM embedding_cache").Scan(&streamCount)

	// semantic cache hits
	var semCount int
	s.db.Conn().QueryRow("SELECT COALESCE(SUM(hits), 0) FROM semantic_cache").Scan(&semCount)

	// For now: exact = cached request_logs entries, stream = embedding_cache hits, semantic = semantic_cache hits
	// Misses = total requests - cached (approximation)
	var totalRequests int
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&totalRequests)

	// exactHits was already computed above from cached request_logs entries.
	streamHits = streamCount
	semanticHits = semCount
	misses = totalRequests - exactHits
	if misses < 0 {
		misses = 0
	}

	total := float64(exactHits + streamHits + semanticHits + misses)
	hitRate := "0.0%"
	if total > 0 {
		rate := float64(exactHits+streamHits+semanticHits) / total * 100
		hitRate = fmt.Sprintf("%.1f%%", rate)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"exact_hits":    exactHits,
		"stream_hits":   streamHits,
		"semantic_hits": semanticHits,
		"misses":        misses,
		"hit_rate":      hitRate,
	})
}

// GET /api/dashboard/combo
func (s *Server) handleDashboardCombo(w http.ResponseWriter, r *http.Request) {
	// Read combos from settings (Node.js stores as JSON array)
	combosJSON, err := s.db.GetSetting("combos")
	if err != nil || combosJSON == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}

	var combos []map[string]any
	if err := json.Unmarshal([]byte(combosJSON), &combos); err != nil {
		// Try single object
		var single map[string]any
		if err2 := json.Unmarshal([]byte(combosJSON), &single); err2 == nil {
			combos = []map[string]any{single}
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]any{})
			return
		}
	}

	// Also check combo_keys setting for key rotation state
	comboKeysJSON, _ := s.db.GetSetting("combo_keys")
	var comboKeys []map[string]any
	if comboKeysJSON != "" {
		json.Unmarshal([]byte(comboKeysJSON), &comboKeys)
	}

	// Enrich combos with key info
	type ComboResponse struct {
		Provider string           `json:"provider"`
		Strategy string           `json:"strategy"`
		Keys     []map[string]any `json:"keys"`
	}

	var result []ComboResponse
	for _, c := range combos {
		provider, _ := c["name"].(string)
		if provider == "" {
			provider, _ = c["provider"].(string)
		}
		strategy, _ := c["strategy"].(string)
		if strategy == "" {
			strategy = "first"
		}

		keys := []map[string]any{}
		// Try to get keys from the combo itself
		if k, ok := c["keys"].([]any); ok {
			for i, kk := range k {
				km, _ := kk.(map[string]any)
				if km == nil {
					km = map[string]any{}
				}
				if km["id"] == nil {
					km["id"] = fmt.Sprintf("%s-key-%d", provider, i+1)
				}
				if km["active"] == nil {
					km["active"] = true
				}
				if km["priority"] == nil {
					km["priority"] = 0
				}
				if km["request_count"] == nil {
					km["request_count"] = 0
				}
				keys = append(keys, km)
			}
		}
		// If no keys in combo, check combo_keys setting
		if len(keys) == 0 && len(comboKeys) > 0 {
			for _, ck := range comboKeys {
				ckProvider, _ := ck["provider"].(string)
				if ckProvider == provider || ckProvider == "" {
					if ck["id"] == nil {
						ck["id"] = uuid.New().String()
					}
					if ck["active"] == nil {
						ck["active"] = true
					}
					if ck["priority"] == nil {
						ck["priority"] = 0
					}
					if ck["request_count"] == nil {
						ck["request_count"] = 0
					}
					keys = append(keys, ck)
				}
			}
		}

		result = append(result, ComboResponse{
			Provider: provider,
			Strategy: strategy,
			Keys:     keys,
		})
	}

	if result == nil {
		result = []ComboResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /api/dashboard/logs?limit=50
func (s *Server) handleDashboardLogs(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "50"
	}

	rows, err := s.db.Conn().Query(`
		SELECT model, status, input_tokens, output_tokens, latency_ms, cached, created_at
		FROM request_logs
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		http.Error(w, `{"error":"failed to query logs"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type LogEntry struct {
		Timestamp   string `json:"timestamp"`
		Model       string `json:"model"`
		LatencyMs   int    `json:"latency_ms"`
		Status      int    `json:"status"`
		CacheHit    bool   `json:"cache_hit"`
		TokensIn    int    `json:"tokens_in"`
		TokensOut   int    `json:"tokens_out"`
	}

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		var cached int
		rows.Scan(&l.Model, &l.Status, &l.TokensIn, &l.TokensOut, &l.LatencyMs, &cached, &l.Timestamp)
		l.CacheHit = cached == 1
		logs = append(logs, l)
	}

	if logs == nil {
		logs = []LogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// PUT /api/dashboard/settings
func (s *Server) handleDashboardSettings(w http.ResponseWriter, r *http.Request) {
	var input map[string]string
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	for k, v := range input {
		// Map dashboard setting keys to DB setting keys
		dbKey := k
		switch k {
		case "lb_strategy":
			dbKey = "load_balancer_strategy"
		case "combo_strategy":
			dbKey = "combo_strategy"
		}
		if err := s.db.SetSetting(dbKey, v); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to set %s"}`, k), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

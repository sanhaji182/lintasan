package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Models endpoint - OpenAI compatible
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query(`
		SELECT m.model_id, c.name as connection_name, m.owned_by
		FROM discovered_models m
		JOIN connections c ON m.connection_id = c.id
		WHERE m.is_active = 1 AND c.is_active = 1
		ORDER BY m.model_id
	`)
	if err != nil {
		http.Error(w, `{"error":"failed to query models"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}

	var models []Model
	for rows.Next() {
		var modelID, connName string
		var ownedBy sql.NullString
		rows.Scan(&modelID, &connName, &ownedBy)
		owner := connName
		if ownedBy.Valid && ownedBy.String != "" {
			owner = ownedBy.String
		}
		models = append(models, Model{
			ID:      modelID,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: owner,
		})
	}

	if models == nil {
		models = []Model{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": models})
}

// Connections CRUD
func (s *Server) handleGetConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query(`SELECT id, name, base_url, api_key, format, is_active, priority, models_count, created_at FROM connections ORDER BY priority DESC, created_at DESC`)
	if err != nil {
		http.Error(w, `{"error":"failed to query connections"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ConnResponse struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		BaseURL     string `json:"base_url"`
		APIKey      string `json:"api_key"`
		Format      string `json:"format"`
		IsActive    int    `json:"is_active"`
		Priority    int    `json:"priority"`
		ModelsCount int    `json:"models_count"`
		CreatedAt   string `json:"created_at"`
	}

	var conns []ConnResponse
	for rows.Next() {
		var c ConnResponse
		rows.Scan(&c.ID, &c.Name, &c.BaseURL, &c.APIKey, &c.Format, &c.IsActive, &c.Priority, &c.ModelsCount, &c.CreatedAt)
		// Mask API key
		if len(c.APIKey) > 8 {
			c.APIKey = c.APIKey[:4] + "..." + c.APIKey[len(c.APIKey)-4:]
		}
		conns = append(conns, c)
	}

	if conns == nil {
		conns = []ConnResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": conns})
}

func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name       string `json:"name"`
		BaseURL    string `json:"base_url"`
		BaseURL2   string `json:"baseUrl"`
		APIKey     string `json:"api_key"`
		APIKey2    string `json:"apiKey"`
		Format     string `json:"format"`
		Priority   int    `json:"priority"`
		ChatPath   string `json:"chatPath"`
		ModelsPath string `json:"modelsPath"`
		AuthHeader string `json:"authHeader"`
		AuthPrefix string `json:"authPrefix"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if input.BaseURL == "" { input.BaseURL = input.BaseURL2 }
	if input.APIKey == "" { input.APIKey = input.APIKey2 }
	if input.Name == "" || input.BaseURL == "" {
		http.Error(w, `{"error":{"message":"name and baseUrl are required"}}`, http.StatusBadRequest)
		return
	}
	if input.Format == "" { input.Format = "openai" }
	if input.ChatPath == "" { input.ChatPath = "/v1/chat/completions" }
	if input.ModelsPath == "" { input.ModelsPath = "/v1/models" }
	if input.AuthHeader == "" { input.AuthHeader = "Authorization" }
	if input.AuthPrefix == "" { input.AuthPrefix = "Bearer " }

	id := uuid.New().String()
	_, err := s.db.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, format, priority, chat_path, models_path, auth_header, auth_prefix) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.BaseURL, input.APIKey, input.Format, input.Priority, input.ChatPath, input.ModelsPath, input.AuthHeader, input.AuthPrefix,
	)
	if err != nil {
		http.Error(w, `{"error":"failed to create connection"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id}, "id": id, "status": "created"})
}

func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" { id = r.URL.Query().Get("id") }
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	_, err := s.db.Conn().Exec("DELETE FROM connections WHERE id = ?", id)
	if err != nil {
		http.Error(w, `{"error":"failed to delete connection"}`, http.StatusInternalServerError)
		return
	}

	// Also delete associated models
	s.db.Conn().Exec("DELETE FROM discovered_models WHERE connection_id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (s *Server) handlePatchConnection(w http.ResponseWriter, r *http.Request) {
	var input struct { ID string `json:"id"`; IsActive *int `json:"is_active"` }
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.ID == "" {
		http.Error(w, `{"error":{"message":"id is required"}}`, http.StatusBadRequest); return
	}
	if input.IsActive != nil { s.db.Conn().Exec("UPDATE connections SET is_active=?, updated_at=datetime('now') WHERE id=?", *input.IsActive, input.ID) }
	w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode(map[string]any{"success":true,"data":map[string]any{"id":input.ID}})
}

// Combos - read from settings (Node.js stores combos in settings as JSON)
func (s *Server) handleGetCombos(w http.ResponseWriter, r *http.Request) {
	combosJSON, err := s.db.GetSetting("combos")
	if err != nil || combosJSON == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var combos any
	if json.Unmarshal([]byte(combosJSON), &combos) == nil {
		json.NewEncoder(w).Encode(map[string]any{"data": combos})
		return
	}
	w.Write([]byte(combosJSON))
}

func (s *Server) handleCreateCombo(w http.ResponseWriter, r *http.Request) {
	var input map[string]any
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Get existing combos
	combosJSON, _ := s.db.GetSetting("combos")
	var combos []map[string]any
	if combosJSON != "" {
		json.Unmarshal([]byte(combosJSON), &combos)
	}

	input["id"] = uuid.New().String()
	combos = append(combos, input)

	newJSON, _ := json.Marshal(combos)
	s.db.SetSetting("combos", string(newJSON))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"data": input, "id": input["id"].(string), "status": "created"})
}

// Stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var totalRequests, cachedRequests int
	var avgLatency sql.NullFloat64

	s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&totalRequests)
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs WHERE cached = 1").Scan(&cachedRequests)
	s.db.Conn().QueryRow("SELECT AVG(latency_ms) FROM request_logs WHERE status = 200").Scan(&avgLatency)

	var modelCount int
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM discovered_models WHERE is_active = 1").Scan(&modelCount)

	var connCount int
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM connections WHERE is_active = 1").Scan(&connCount)

	cacheRate := 0.0
	if totalRequests > 0 {
		cacheRate = float64(cachedRequests) / float64(totalRequests) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"total_requests":     totalRequests,
		"cached_requests":    cachedRequests,
		"cache_hit_rate":     fmt.Sprintf("%.1f%%", cacheRate),
		"avg_latency_ms":     avgLatency.Float64,
		"active_models":      modelCount,
		"active_connections": connCount,
	}})
}

// Logs
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "20"
	}

	rows, err := s.db.Conn().Query(`
		SELECT id, connection_id, provider, model, status, input_tokens, output_tokens, latency_ms, cached, error, created_at
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
		ID           string `json:"id"`
		ConnectionID string `json:"connection_id"`
		Provider     string `json:"provider"`
		Model        string `json:"model"`
		Status       int    `json:"status"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
		LatencyMs    int    `json:"latency_ms"`
		Cached       int    `json:"cached"`
		Error        string `json:"error"`
		CreatedAt    string `json:"created_at"`
	}

	var logs []LogEntry
	for rows.Next() {
		var l LogEntry
		var errStr sql.NullString
		rows.Scan(&l.ID, &l.ConnectionID, &l.Provider, &l.Model, &l.Status, &l.InputTokens, &l.OutputTokens, &l.LatencyMs, &l.Cached, &errStr, &l.CreatedAt)
		if errStr.Valid {
			l.Error = errStr.String
		}
		logs = append(logs, l)
	}

	if logs == nil {
		logs = []LogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": logs})
}

// Settings
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query("SELECT key, value FROM settings")
	if err != nil {
		http.Error(w, `{"error":"failed to query settings"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		// Mask sensitive values
		if k == "master_key" && len(v) > 8 {
			v = v[:4] + "..." + v[len(v)-4:]
		}
		settings[k] = v
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": settings})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var input map[string]string
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	for k, v := range input {
		if err := s.db.SetSetting(k, v); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to set %s"}`, k), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

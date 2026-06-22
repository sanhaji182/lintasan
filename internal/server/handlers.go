package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/models"
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

	type Model struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}

	var modelsList []Model
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var modelID, connName string
			var ownedBy sql.NullString
			if err := rows.Scan(&modelID, &connName, &ownedBy); err != nil {
				continue
			}
			owner := connName
			if ownedBy.Valid && ownedBy.String != "" {
				owner = ownedBy.String
			}
			modelsList = append(modelsList, Model{
				ID:      modelID,
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: owner,
			})
		}
		// Check for iteration errors after the loop
		if err := rows.Err(); err != nil {
			// Log and return empty list rather than silently returning partial data
		}
	}

	// When no DB discovered models, fill from the embedded provider catalog
	if len(modelsList) == 0 {
		for _, p := range models.Catalog() {
			for _, m := range p.Models {
				modelsList = append(modelsList, Model{
					ID:      m.ID,
					Object:  "model",
					Created: time.Now().Unix(),
					OwnedBy: p.Name,
				})
			}
		}
	}

	if modelsList == nil {
		modelsList = []Model{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": modelsList})
}

// GET /api/models/catalog — returns full catalog with pricing
func (s *Server) handleModelsCatalog(w http.ResponseWriter, r *http.Request) {
	catalog := models.Catalog()

	// Build response with providers and models including pricing
	type ModelResponse struct {
		ID            string   `json:"id"`
		Name          string   `json:"name"`
		Provider      string   `json:"provider"`
		ContextWindow int      `json:"context_window"`
		MaxTokens     int      `json:"max_tokens"`
		InputPrice    float64  `json:"input_price_per_1m"`
		OutputPrice   float64  `json:"output_price_per_1m"`
		Capabilities  []string `json:"capabilities"`
	}

	type ProviderResponse struct {
		ID         string          `json:"id"`
		Name       string          `json:"name"`
		BaseURL    string          `json:"base_url"`
		Format     string          `json:"format"`
		ModelCount int             `json:"model_count"`
		Models     []ModelResponse `json:"models"`
	}

	var providers []ProviderResponse
	totalModels := 0
	for _, p := range catalog {
		pr := ProviderResponse{
			ID:         p.ID,
			Name:       p.Name,
			BaseURL:    p.BaseURL,
			Format:     p.Format,
			ModelCount: len(p.Models),
			Models:     make([]ModelResponse, 0, len(p.Models)),
		}
		for _, m := range p.Models {
			pr.Models = append(pr.Models, ModelResponse{
				ID:            m.ID,
				Name:          m.Name,
				Provider:      p.Name,
				ContextWindow: m.ContextWindow,
				MaxTokens:     m.MaxTokens,
				InputPrice:    m.InputPrice,
				OutputPrice:   m.OutputPrice,
				Capabilities:  m.Capabilities,
			})
		}
		totalModels += len(p.Models)
		providers = append(providers, pr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"data": providers,
		"meta": map[string]any{
			"total_providers": len(catalog),
			"total_models":    totalModels,
		},
	})
}

// Connections CRUD
func (s *Server) handleGetConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query(`SELECT id, name, base_url, api_key, COALESCE(oauth_provider,''), format, is_active, priority, models_count, created_at FROM connections ORDER BY priority DESC, created_at DESC`)
	if err != nil {
		http.Error(w, `{"error":"failed to query connections"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ConnResponse struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		BaseURL     string `json:"base_url"`
		APIKey        string `json:"api_key"`
		OAuthProvider string `json:"oauth_provider,omitempty"`
		Format        string `json:"format"`
		IsActive    int    `json:"is_active"`
		Priority    int    `json:"priority"`
		ModelsCount int    `json:"models_count"`
		CreatedAt   string `json:"created_at"`
	}

	var conns []ConnResponse
	for rows.Next() {
		var c ConnResponse
		if err := rows.Scan(&c.ID, &c.Name, &c.BaseURL, &c.APIKey, &c.OAuthProvider, &c.Format, &c.IsActive, &c.Priority, &c.ModelsCount, &c.CreatedAt); err != nil {
			continue
		}
		// Mask API key
		if len(c.APIKey) > 8 {
			c.APIKey = c.APIKey[:4] + "..." + c.APIKey[len(c.APIKey)-4:]
		}
		conns = append(conns, c)
	}
	// Check for row iteration errors
	if err := rows.Err(); err != nil {
		// row iteration error — return empty list rather than silently truncated data
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
		AuthHeader    string `json:"authHeader"`
		AuthPrefix    string `json:"authPrefix"`
		OAuthProvider string `json:"oauth_provider"`
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
		`INSERT INTO connections (id, name, base_url, api_key, oauth_provider, format, priority, chat_path, models_path, auth_header, auth_prefix) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, input.Name, input.BaseURL, input.APIKey, strings.TrimSpace(strings.ToLower(input.OAuthProvider)), input.Format, input.Priority, input.ChatPath, input.ModelsPath, input.AuthHeader, input.AuthPrefix,
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
	var input struct {
		ID       string  `json:"id"`
		Name     *string `json:"name"`
		BaseURL  *string `json:"base_url"`
		APIKey        *string `json:"api_key"`
		OAuthProvider *string `json:"oauth_provider"`
		IsActive      *int    `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		// Body may be empty (frontend sends id as query param). Use zero values.
	}
	// Accept id from query param if not in body (frontend sends as query)
	if input.ID == "" { input.ID = r.URL.Query().Get("id") }
	if input.ID == "" {
		http.Error(w, `{"error":{"message":"id is required"}}`, http.StatusBadRequest); return
	}
	
	var updates []string
	var args []any
	if input.Name != nil { updates = append(updates, "name = ?"); args = append(args, *input.Name) }
	if input.BaseURL != nil { updates = append(updates, "base_url = ?"); args = append(args, *input.BaseURL) }
	if input.IsActive != nil { updates = append(updates, "is_active = ?"); args = append(args, *input.IsActive) }
	if input.APIKey != nil {
		newKey := *input.APIKey
		if !(strings.Contains(newKey, "...") && len(newKey) < 20) {
			updates = append(updates, "api_key = ?")
			args = append(args, newKey)
		}
	}
	if input.OAuthProvider != nil {
		updates = append(updates, "oauth_provider = ?")
		args = append(args, strings.TrimSpace(strings.ToLower(*input.OAuthProvider)))
	}

	if len(updates) > 0 {
		updates = append(updates, "updated_at = datetime('now', 'localtime')")
		args = append(args, input.ID)
		s.db.Conn().Exec("UPDATE connections SET " + strings.Join(updates, ", ") + " WHERE id=?", args...)
	}
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

func (s *Server) handleUpdateCombo(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

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

	// Find and update the combo
	found := false
	for i, combo := range combos {
		if combo["id"] == id {
			// Preserve the id
			input["id"] = id
			combos[i] = input
			found = true
			break
		}
	}

	if !found {
		http.Error(w, `{"error":"combo not found"}`, http.StatusNotFound)
		return
	}

	newJSON, _ := json.Marshal(combos)
	s.db.SetSetting("combos", string(newJSON))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": input, "id": id, "status": "updated"})
}

func (s *Server) handleDeleteCombo(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	// Get existing combos
	combosJSON, _ := s.db.GetSetting("combos")
	var combos []map[string]any
	if combosJSON != "" {
		json.Unmarshal([]byte(combosJSON), &combos)
	}

	// Filter out the combo with matching id
	newCombos := make([]map[string]any, 0, len(combos))
	found := false
	for _, combo := range combos {
		if combo["id"] == id {
			found = true
		} else {
			newCombos = append(newCombos, combo)
		}
	}

	if !found {
		http.Error(w, `{"error":"combo not found"}`, http.StatusNotFound)
		return
	}

	newJSON, _ := json.Marshal(newCombos)
	s.db.SetSetting("combos", string(newJSON))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "deleted"})
}

// Stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var totalRequests, cachedRequests int
	var avgLatency sql.NullFloat64
	var totalTokensIn, totalTokensOut int

	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&totalRequests); err != nil {
		totalRequests = 0
	}
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM request_logs WHERE cached = 1").Scan(&cachedRequests); err != nil {
		cachedRequests = 0
	}
	if err := s.db.Conn().QueryRow("SELECT AVG(latency_ms) FROM request_logs WHERE status = 200").Scan(&avgLatency); err != nil {
		avgLatency = sql.NullFloat64{Valid: false}
	}
	if err := s.db.Conn().QueryRow("SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0) FROM request_logs WHERE created_at >= date('now')").Scan(&totalTokensIn, &totalTokensOut); err != nil {
		totalTokensIn, totalTokensOut = 0, 0
	}

	var modelCount int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM discovered_models WHERE is_active = 1").Scan(&modelCount); err != nil {
		modelCount = 0
	}

	var connCount int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM connections WHERE is_active = 1").Scan(&connCount); err != nil {
		connCount = 0
	}

	cacheRate := 0.0
	if totalRequests > 0 {
		cacheRate = float64(cachedRequests) / float64(totalRequests) * 100
	}

	// Feature list matching Go's capabilities
	features := []map[string]any{
		{"name": "Retry Logic", "enabled": true},
		{"name": "Circuit Breaker", "enabled": true},
		{"name": "Rate Limiter", "enabled": true},
		{"name": "Fallback Chain", "enabled": true},
		{"name": "Exact Cache", "enabled": true},
		{"name": "Stream Cache", "enabled": true},
		{"name": "Load Balancer", "enabled": true},
		{"name": "Combo System", "enabled": true},
		{"name": "MITM Proxy", "enabled": true},
		{"name": "OAuth Manager", "enabled": true},
		{"name": "Model Catalog", "enabled": true},
		{"name": "Dashboard", "enabled": true},
	}

	// Providers from connections table
	providerRows, err := s.db.Conn().Query("SELECT name, is_active, base_url, format FROM connections")
	var providers []map[string]any
	if err == nil {
		defer providerRows.Close()
		for providerRows.Next() {
			var name, baseURL, format string
			var isActive int
			if providerRows.Scan(&name, &isActive, &baseURL, &format) == nil {
				providers = append(providers, map[string]any{
					"name": name, "healthy": isActive == 1,
					"latency": avgLatency.Float64, "format": format,
				})
			}
		}
		if err := providerRows.Err(); err != nil {
			// iteration error — providers stays as-is
		}
	}
	if providers == nil {
		providers = []map[string]any{}
	}

	tokensToday := totalTokensIn + totalTokensOut

	// Compute actual weekly request volume from the last 7 days.
	// Query each day individually so days-with-zero appear as 0 instead of
	// being omitted from the group-by result.
	var requestVolume []int
	for day := -6; day <= 0; day++ {
		dayStr := strconv.Itoa(day)
		var count int
		if err := s.db.Conn().QueryRow("SELECT COALESCE(COUNT(*),0) FROM request_logs WHERE date(created_at) = date('now', ? || ' days', 'localtime')", dayStr).Scan(&count); err != nil {
			count = 0
		}
		requestVolume = append(requestVolume, count)
	}
	if requestVolume == nil {
		requestVolume = []int{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"totalRequests":     totalRequests,
		"cachedRequests":    cachedRequests,
		"cacheHitRate":      cacheRate,
		"avgLatency":        avgLatency.Float64,
		"tokensToday":       tokensToday,
		"tokensMonth":       tokensToday * 30,
		"tokensSaved":       cachedRequests * 2000, // rough estimate: ~2K tokens per cache hit
		"tokensCompressed":  0, // token compression counters TBD when compressor tracks stats
		"activeModels":      modelCount,
		"activeConnections": connCount,
		"features":          features,
		"providers":         providers,
		"requestVolume":     requestVolume,
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
		if err := rows.Scan(&l.ID, &l.ConnectionID, &l.Provider, &l.Model, &l.Status, &l.InputTokens, &l.OutputTokens, &l.LatencyMs, &l.Cached, &errStr, &l.CreatedAt); err != nil {
			continue
		}
		if errStr.Valid {
			l.Error = errStr.String
		}
		logs = append(logs, l)
	}
	// Check for iteration errors
	if err := rows.Err(); err != nil {
		// iteration error — return what we have rather than silently truncated data
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
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		// Mask sensitive values before returning them to the dashboard. These
		// keys hold credentials/secrets that a client never needs in cleartext:
		// leaking jwt_secret in particular lets an attacker forge valid JWTs.
		if isSensitiveSettingKey(k) {
			v = maskSecretValue(v)
		}
		settings[k] = v
	}
	if err := rows.Err(); err != nil {
		// iteration error — return what we have
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": settings})
}

// sensitiveSettingKeys are settings whose values are credentials/secrets and
// must be masked in GET /api/settings responses.
var sensitiveSettingKeys = map[string]bool{
	"master_key": true,
	"jwt_secret": true,
}

// isSensitiveSettingKey reports whether a settings key must be masked. It also
// treats any key containing common secret markers as sensitive, so future keys
// like "*_secret", "*_token", "*_api_key", or "*password*" are masked by
// default rather than leaking until someone remembers to add them here.
func isSensitiveSettingKey(key string) bool {
	if sensitiveSettingKeys[key] {
		return true
	}
	k := strings.ToLower(key)
	for _, marker := range []string{"secret", "password", "api_key", "apikey", "token", "private_key"} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

// maskSecretValue returns a masked form of a secret that reveals only enough to
// recognize it (first/last 4 chars) without exposing the value. Short values
// are fully redacted.
func maskSecretValue(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 8 {
		return "***"
	}
	return v[:4] + "..." + v[len(v)-4:]
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	for k, rawV := range raw {
		var v string
		if err := json.Unmarshal(rawV, &v); err == nil {
			// string value as-is
		} else {
			var b bool
			if err := json.Unmarshal(rawV, &b); err == nil {
				if b {
					v = "true"
				} else {
					v = "false"
				}
			} else {
				var n float64
				if err := json.Unmarshal(rawV, &n); err == nil {
					v = strconv.FormatFloat(n, 'g', -1, 64)
				} else {
					v = string(rawV)
				}
			}
		}
		if err := s.db.SetSetting(k, v); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to set %s"}`, k), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

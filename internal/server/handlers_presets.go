package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

// Preset represents a provider preset in the catalog
type Preset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Domain    string `json:"domain"`
	BaseURL   string `json:"base_url"`
	Format    string `json:"format"`
	KeyLabel  string `json:"key_label"`
	Category  string `json:"category"`
	IsBuiltin int    `json:"is_builtin"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// handleGetPresets returns all provider presets (built-in + custom)
func (s *Server) handleGetPresets(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	rows, err := s.db.Conn().Query(`
		SELECT id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at
		FROM provider_presets
		ORDER BY is_builtin DESC, name ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	presets := []Preset{}
	for rows.Next() {
		var p Preset
		if err := rows.Scan(&p.ID, &p.Name, &p.Domain, &p.BaseURL, &p.Format, &p.KeyLabel, &p.Category, &p.IsBuiltin, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		presets = append(presets, p)
	}

	writeData(w, presets)
}

// handleCreatePreset creates a new custom provider preset
func (s *Server) handleCreatePreset(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		BaseURL  string `json:"base_url"`
		Format   string `json:"format"`
		KeyLabel string `json:"key_label"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Domain == "" || req.BaseURL == "" {
		http.Error(w, "name, domain, and base_url are required", http.StatusBadRequest)
		return
	}
	if req.Format == "" {
		req.Format = "openai"
	}
	if req.KeyLabel == "" {
		req.KeyLabel = "API Key"
	}
	if req.Category == "" {
		req.Category = "foundation"
	}

	id, _ := generatePresetID()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	_, err := s.db.Conn().Exec(`
		INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
	`, id, req.Name, req.Domain, req.BaseURL, req.Format, req.KeyLabel, req.Category, now, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeData(w, Preset{
		ID:        id,
		Name:      req.Name,
		Domain:    req.Domain,
		BaseURL:   req.BaseURL,
		Format:    req.Format,
		KeyLabel:  req.KeyLabel,
		Category:  req.Category,
		IsBuiltin: 0,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// handleUpdatePreset updates a custom provider preset (built-in presets are read-only)
func (s *Server) handleUpdatePreset(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "preset id required", http.StatusBadRequest)
		return
	}

	var req struct {
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		BaseURL  string `json:"base_url"`
		Format   string `json:"format"`
		KeyLabel string `json:"key_label"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Check if preset exists and is not built-in
	var isBuiltin int
	err := s.db.Conn().QueryRow("SELECT is_builtin FROM provider_presets WHERE id = ?", id).Scan(&isBuiltin)
	if err != nil {
		http.Error(w, "preset not found", http.StatusNotFound)
		return
	}
	if isBuiltin == 1 {
		http.Error(w, "built-in presets are read-only", http.StatusForbidden)
		return
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = s.db.Conn().Exec(`
		UPDATE provider_presets
		SET name = ?, domain = ?, base_url = ?, format = ?, key_label = ?, category = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Domain, req.BaseURL, req.Format, req.KeyLabel, req.Category, now, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"success": true})
}

// handleDeletePreset deletes a custom provider preset (built-in presets are protected)
func (s *Server) handleDeletePreset(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "preset id required", http.StatusBadRequest)
		return
	}

	// Check if preset is built-in
	var isBuiltin int
	err := s.db.Conn().QueryRow("SELECT is_builtin FROM provider_presets WHERE id = ?", id).Scan(&isBuiltin)
	if err != nil {
		http.Error(w, "preset not found", http.StatusNotFound)
		return
	}
	if isBuiltin == 1 {
		http.Error(w, "built-in presets cannot be deleted", http.StatusForbidden)
		return
	}

	_, err = s.db.Conn().Exec("DELETE FROM provider_presets WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"success": true})
}

// handleSeedBuiltinPresets inserts built-in presets if they don't exist
func (s *Server) handleSeedBuiltinPresets(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	// Seeding is additive: we insert any built-in that is missing rather than
	// bailing out when some already exist. Bailing out meant an install that
	// seeded once could never receive presets added in a later release.
	seededBefore := 0
	_ = s.db.Conn().QueryRow("SELECT COUNT(*) FROM provider_presets WHERE is_builtin = 1").Scan(&seededBefore)

	builtins := []Preset{
		{Name: "OpenAI", Domain: "openai.com", BaseURL: "https://api.openai.com/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Anthropic", Domain: "anthropic.com", BaseURL: "https://api.anthropic.com/v1", Format: "anthropic", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Google AI", Domain: "ai.google.dev", BaseURL: "https://generativelanguage.googleapis.com/v1beta", Format: "gemini", KeyLabel: "API Key", Category: "foundation"},
		{Name: "xAI", Domain: "x.ai", BaseURL: "https://api.x.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Mistral", Domain: "mistral.ai", BaseURL: "https://api.mistral.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "DeepSeek", Domain: "deepseek.com", BaseURL: "https://api.deepseek.com/v1", Format: "openai", KeyLabel: "API Key", Category: "open"},
		{Name: "Qwen", Domain: "dashscope.aliyuncs.com", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Format: "openai", KeyLabel: "API Key", Category: "open"},
		{Name: "Moonshot", Domain: "moonshot.cn", BaseURL: "https://api.moonshot.cn/v1", Format: "openai", KeyLabel: "API Key", Category: "open"},
		{Name: "Zhipu", Domain: "zhipuai.cn", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Format: "openai", KeyLabel: "API Key", Category: "open"},
		{Name: "AI21", Domain: "ai21.com", BaseURL: "https://api.ai21.com/studio/v1", Format: "openai", KeyLabel: "API Key", Category: "open"},
		{Name: "Cohere", Domain: "cohere.com", BaseURL: "https://api.cohere.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "open"},
		{Name: "Groq", Domain: "groq.com", BaseURL: "https://api.groq.com/openai/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Together", Domain: "together.ai", BaseURL: "https://api.together.xyz/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Fireworks", Domain: "fireworks.ai", BaseURL: "https://api.fireworks.ai/inference/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "OpenRouter", Domain: "openrouter.ai", BaseURL: "https://openrouter.ai/api/v1", Format: "openai", KeyLabel: "API Key", Category: "aggregator"},
		{Name: "Replicate", Domain: "replicate.com", BaseURL: "https://api.replicate.com/v1", Format: "openai", KeyLabel: "API Key", Category: "aggregator"},
		{Name: "Perplexity", Domain: "perplexity.ai", BaseURL: "https://api.perplexity.ai", Format: "openai", KeyLabel: "API Key", Category: "search"},
		{Name: "Ollama", Domain: "ollama.com", BaseURL: "http://localhost:11434/v1", Format: "openai", KeyLabel: "API Key (optional)", Category: "local"},
		// Endpoints below were confirmed by a live GET /v1/models against the
		// provider before being added here, so the importer can restore
		// connections that a competitor's export leaves without a base URL.
		{Name: "Xiaomi MiMo", Domain: "xiaomimimo.com", BaseURL: "https://api.xiaomimimo.com/v1", Format: "openai", KeyLabel: "API Key", Category: "open"},
		{Name: "NVIDIA", Domain: "nvidia.com", BaseURL: "https://integrate.api.nvidia.com/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Qoder", Domain: "qoder.com", BaseURL: "https://api.qoder.com/v1", Format: "openai", KeyLabel: "API Key", Category: "coding"},
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	inserted := 0
	skipped := 0
	for _, p := range builtins {
		// Skip presets already present so repeated seeding stays idempotent.
		var exists int
		if err := s.db.Conn().QueryRow(
			"SELECT COUNT(*) FROM provider_presets WHERE lower(name) = lower(?)", p.Name,
		).Scan(&exists); err == nil && exists > 0 {
			skipped++
			continue
		}
		id, _ := generatePresetID()
		_, err := s.db.Conn().Exec(`
			INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		`, id, p.Name, p.Domain, p.BaseURL, p.Format, p.KeyLabel, p.Category, now, now)
		if err == nil {
			inserted++
		}
	}

	writeJSON(w, map[string]any{
		"success":  true,
		"inserted": inserted,
		"skipped":  skipped,
		"total":    seededBefore + inserted,
	})
}

// generatePresetID generates a random ID
func generatePresetID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "preset_" + hex.EncodeToString(b), nil
}

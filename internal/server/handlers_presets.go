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

// seedCatalogue is the list of built-in provider presets. It lives in its own
// function so tests can assert on the catalogue without needing a database.
func seedCatalogue() []Preset {
	return []Preset{
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
		// Providers below are cross-referenced from the 9router registry
		// (github.com/decolua/9router, open-sse/providers/registry) so a user
		// migrating from it lands on a catalogue that already knows their
		// endpoints. Only OpenAI-compatible chat providers are listed, and each
		// base URL was probed with GET /models before inclusion: 200, 401, or
		// 403 means the endpoint is real and merely auth-gated, which is what we
		// want from a preset. Providers using a bespoke wire format (Anthropic,
		// Gemini, Vertex), an OAuth-only login, or a non-HTTP transport are
		// deliberately excluded rather than guessed at.
		{Name: "Alibaba Coding", Domain: "aliyuncs.com", BaseURL: "https://coding.dashscope.aliyuncs.com/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Alibaba Coding (Intl)", Domain: "aliyuncs.com", BaseURL: "https://coding-intl.dashscope.aliyuncs.com/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Alibaba Model Studio Intl", Domain: "aliyuncs.com", BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "API.airforce", Domain: "api.airforce", BaseURL: "https://api.airforce/v1", Format: "openai", KeyLabel: "API Key", Category: "free-tier"},
		{Name: "Baidu Qianfan", Domain: "baidubce.com", BaseURL: "https://qianfan.baidubce.com/v2", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Bazaarlink", Domain: "bazaarlink.ai", BaseURL: "https://bazaarlink.ai/api/v1", Format: "openai", KeyLabel: "API Key", Category: "free-tier"},
		{Name: "Blackbox AI", Domain: "blackbox.ai", BaseURL: "https://api.blackbox.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "BluesMinds", Domain: "bluesminds.com", BaseURL: "https://api.bluesminds.com/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "BytePlus ModelArk", Domain: "bytepluses.com", BaseURL: "https://ark.ap-southeast.bytepluses.com/api/coding/v3", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Cerebras", Domain: "cerebras.ai", BaseURL: "https://api.cerebras.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Chutes AI", Domain: "chutes.ai", BaseURL: "https://llm.chutes.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Featherless", Domain: "featherless.ai", BaseURL: "https://api.featherless.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "free-tier"},
		{Name: "GLM China", Domain: "bigmodel.cn", BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Hyperbolic", Domain: "hyperbolic.xyz", BaseURL: "https://api.hyperbolic.xyz/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Kilo Gateway", Domain: "kilo.ai", BaseURL: "https://api.kilo.ai/api/gateway", Format: "openai", KeyLabel: "API Key", Category: "free-tier"},
		{Name: "Kimchi", Domain: "kimchi.dev", BaseURL: "https://llm.kimchi.dev/openai/v1", Format: "openai", KeyLabel: "API Key", Category: "free-tier"},
		{Name: "LLM7", Domain: "llm7.io", BaseURL: "https://api.llm7.io/v1", Format: "openai", KeyLabel: "API Key", Category: "free-tier"},
		{Name: "Morph", Domain: "morphllm.com", BaseURL: "https://api.morphllm.com/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Nebius AI", Domain: "nebius.ai", BaseURL: "https://api.studio.nebius.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "OpenCode Go", Domain: "opencode.ai", BaseURL: "https://opencode.ai/zen/go/v1", Format: "openai", KeyLabel: "API Key", Category: "coding"},
		{Name: "Poolside", Domain: "poolside.ai", BaseURL: "https://inference.poolside.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "free-tier"},
		{Name: "SambaNova", Domain: "sambanova.ai", BaseURL: "https://api.sambanova.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "SiliconFlow", Domain: "siliconflow.com", BaseURL: "https://api.siliconflow.com/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "Tencent Hunyuan", Domain: "tencent.com", BaseURL: "https://api.hunyuan.cloud.tencent.com/v1", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		{Name: "TokenRouter", Domain: "tokenrouter.com", BaseURL: "https://api.tokenrouter.com/v1", Format: "openai", KeyLabel: "API Key", Category: "aggregator"},
		{Name: "Venice AI", Domain: "venice.ai", BaseURL: "https://api.venice.ai/api/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Vercel AI Gateway", Domain: "vercel.sh", BaseURL: "https://ai-gateway.vercel.sh/v1", Format: "openai", KeyLabel: "API Key", Category: "aggregator"},
		{Name: "Volcengine Ark", Domain: "volces.com", BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", Format: "openai", KeyLabel: "API Key", Category: "foundation"},
		// Named "Xiaomi TokenPlan" (no space before Plan) because the alias
		// generator turns spaces into hyphens: this is the only natural
		// spelling whose aliases include "xiaomi-tokenplan", the id a 9router
		// export uses for this endpoint.
		{Name: "Xiaomi TokenPlan", Domain: "xiaomimimo.com", BaseURL: "https://token-plan-sgp.xiaomimimo.com/v1", Format: "openai", KeyLabel: "API Key", Category: "open"},
		// Providers below were added for the NON-CHAT endpoints (embeddings,
		// images, audio). Two probes gate inclusion, because per-model routing
		// on those endpoints needs both to hold:
		//   1. the non-chat path itself answers (200/400/401/403, not 404), and
		//   2. GET /models answers, so discovery can populate discovered_models
		//      — without it the model is unknown and the request silently falls
		//      back to the first connection, which is the bug we just fixed.
		// Voyage AI was probed and deliberately EXCLUDED: /v1/embeddings is real
		// (400) but /v1/models is 404, so its models can never be discovered and
		// a preset would route by accident rather than by model.
		{Name: "DeepInfra", Domain: "deepinfra.com", BaseURL: "https://api.deepinfra.com/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Jina AI", Domain: "jina.ai", BaseURL: "https://api.jina.ai/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Baseten", Domain: "baseten.co", BaseURL: "https://inference.baseten.co/v1", Format: "openai", KeyLabel: "API Key", Category: "inference"},
		{Name: "Novita AI", Domain: "novita.ai", BaseURL: "https://api.novita.ai/v3/openai", Format: "openai", KeyLabel: "API Key", Category: "inference"},
	}
}

// handleSeedBuiltinPresets inserts missing built-in presets AND refreshes the
// name/category of ones already present, keyed on base_url.
//
// Two behaviours in one endpoint because both answer "bring this install up to
// the current catalogue":
//   - Insert: a preset added in a later release must reach installs that seeded
//     before it existed (the old bail-out froze them).
//   - Refresh: a renamed or recategorised preset must update in place. The key
//     is base_url, not name — a rename changes the name, so matching on name
//     would insert a duplicate rather than update. Manual (non-builtin)
//     presets are never touched, so a user's own edits are safe.
func (s *Server) handleSeedBuiltinPresets(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	builtins := seedCatalogue()

	// oldName -> currentName for presets that have been renamed. Matching the
	// old name first is what lets a rename update the existing row instead of
	// inserting a second one under the new name.
	renames := map[string]string{
		"gitlawb":                     "GitLawb",
		"Alibaba Model Studio (Intl)": "Alibaba Model Studio Intl",
		"GLM (China)":                 "GLM China",
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	inserted := 0
	updated := 0
	skipped := 0
	for _, p := range builtins {
		// Find the existing row, preferring the current name, then a prior
		// name, then the endpoint. base_url is the stable identity.
		var id string
		var isBuiltin int
		row := s.db.Conn().QueryRow(
			`SELECT id, is_builtin FROM provider_presets
			   WHERE lower(name) = lower(?)
			      OR lower(name) = lower(?)
			      OR lower(rtrim(base_url, '/')) = lower(rtrim(?, '/'))
			   LIMIT 1`,
			p.Name, oldNameFor(renames, p.Name), p.BaseURL,
		)
		if err := row.Scan(&id, &isBuiltin); err == nil && id != "" {
			if isBuiltin == 1 {
				// Refresh a built-in in place: name and category only. Never
				// the base_url — that is the identity we matched on — and
				// never a manual preset.
				if _, err := s.db.Conn().Exec(
					`UPDATE provider_presets SET name = ?, category = ?, updated_at = ? WHERE id = ? AND is_builtin = 1`,
					p.Name, p.Category, now, id,
				); err == nil {
					updated++
				}
			} else {
				skipped++ // manual preset shares the endpoint; leave it alone
			}
			continue
		}

		newID, _ := generatePresetID()
		_, err := s.db.Conn().Exec(`
			INSERT INTO provider_presets (id, name, domain, base_url, format, key_label, category, is_builtin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		`, newID, p.Name, p.Domain, p.BaseURL, p.Format, p.KeyLabel, p.Category, now, now)
		if err == nil {
			inserted++
		}
	}

	writeJSON(w, map[string]any{
		"success":  true,
		"inserted": inserted,
		"updated":  updated,
		"skipped":  skipped,
	})
}

// oldNameFor returns the previous name of a preset if it was renamed, so the
// seeder can find and update the existing row. Returns the current name when
// there is no rename, which makes the OR clause a harmless no-op.
func oldNameFor(renames map[string]string, current string) string {
	for old, cur := range renames {
		if cur == current {
			return old
		}
	}
	return current
}

// generatePresetID generates a random ID
func generatePresetID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "preset_" + hex.EncodeToString(b), nil
}

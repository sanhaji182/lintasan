package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

// oauthIdeConnectionPreset mirrors frontend oauthIdePresets.ts
type oauthIdeConnectionPreset struct {
	OAuthProvider string
	Name          string
	BaseURL       string
	Format        string
	ChatPath      string
	ModelsPath    string
	AuthHeader    string
	AuthPrefix    string
}

func oauthIdePresets() []oauthIdeConnectionPreset {
	return []oauthIdeConnectionPreset{
		{"xai", "oauth-xai", "https://api.x.ai/v1", "openai", "/v1/chat/completions", "/v1/models", "Authorization", "Bearer "},
		{"claude", "oauth-claude", "https://api.anthropic.com/v1", "anthropic", "/messages", "", "x-api-key", ""},
		{"github", "oauth-github-copilot", "https://api.githubcopilot.com", "openai", "/v1/chat/completions", "/v1/models", "Authorization", "Bearer "},
		{"codex", "oauth-codex", "https://api.openai.com/v1", "openai", "/v1/chat/completions", "/v1/models", "Authorization", "Bearer "},
		{"cursor", "oauth-cursor", "https://api2.cursor.sh", "openai", "/v1/chat/completions", "/v1/models", "Authorization", "Bearer "},
		{"kilocode", "oauth-kilocode", "https://api.kilo.ai", "openai", "/v1/chat/completions", "/v1/models", "Authorization", "Bearer "},
		{"cline", "oauth-cline", "https://api.cline.bot", "openai", "/v1/chat/completions", "/v1/models", "Authorization", "Bearer "},
		{"antigravity", "oauth-antigravity", "https://cloudcode-pa.googleapis.com", "openai", "/v1/chat/completions", "/v1/models", "Authorization", "Bearer "},
	}
}

func presetForOAuthProvider(id string) *oauthIdeConnectionPreset {
	id = strings.TrimSpace(strings.ToLower(id))
	for _, p := range oauthIdePresets() {
		if p.OAuthProvider == id {
			cp := p
			return &cp
		}
	}
	return nil
}

func (s *Server) handleOAuthProvisionConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.oauthIdeEnabled() {
		writeJSONStatus(w, http.StatusForbidden, map[string]string{"error": "oauth_ide_disabled"})
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	var in struct {
		Provider string `json:"provider"`
		Name     string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	provider := strings.TrimSpace(strings.ToLower(in.Provider))
	if provider == "" || !auth.IsIdeOAuthProvider(provider) {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "unknown or missing provider"})
		return
	}
	if !oauthide.CanStartAuthorize(provider) && provider != "cursor" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "provider not ready for proxy wiring"})
		return
	}

	preset := presetForOAuthProvider(provider)
	if preset == nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "no preset for provider"})
		return
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = preset.Name
	}

	// Upsert by oauth_provider (one lab connection per IDE provider)
	var existingID string
	_ = s.db.Conn().QueryRow(
		`SELECT id FROM connections WHERE oauth_provider = ? LIMIT 1`, provider,
	).Scan(&existingID)

	if existingID != "" {
		_, err := s.db.Conn().Exec(
			`UPDATE connections SET name=?, base_url=?, api_key='', format=?, chat_path=?, models_path=?, auth_header=?, auth_prefix=?, is_active=1, updated_at=datetime('now','localtime') WHERE id=?`,
			name, preset.BaseURL, preset.Format, preset.ChatPath, preset.ModelsPath, preset.AuthHeader, preset.AuthPrefix, existingID,
		)
		if err != nil {
			writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
		writeJSON(w, map[string]any{
			"success":        true,
			"action":         "updated",
			"id":             existingID,
			"oauth_provider": provider,
			"name":           name,
		})
		return
	}

	id := uuid.New().String()
	_, err := s.db.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, oauth_provider, format, priority, chat_path, models_path, auth_header, auth_prefix, is_active)
		 VALUES (?, ?, ?, '', ?, ?, 5, ?, ?, ?, ?, 1)`,
		id, name, preset.BaseURL, provider, preset.Format, preset.ChatPath, preset.ModelsPath, preset.AuthHeader, preset.AuthPrefix,
	)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "create failed"})
		return
	}

	writeJSON(w, map[string]any{
		"success":        true,
		"action":         "created",
		"id":             id,
		"oauth_provider": provider,
		"name":           name,
		"hint":           "Sync models after an active OAuth session exists",
	})
}

func (s *Server) handleOAuthIdePresets(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]string, 0, len(oauthIdePresets()))
	for _, p := range oauthIdePresets() {
		out = append(out, map[string]string{
			"oauth_provider": p.OAuthProvider,
			"name":           p.Name,
			"base_url":       p.BaseURL,
			"format":         p.Format,
		})
	}
	writeJSON(w, map[string]any{"presets": out})
}
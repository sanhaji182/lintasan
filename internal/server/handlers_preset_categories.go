package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"time"
)

// PresetCategory represents a category for organizing provider presets
type PresetCategory struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
	IsBuiltin int    `json:"is_builtin"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// validCategoryKey enforces safe key names (slug-style)
var validCategoryKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,30}$`)

// handleGetPresetCategories returns all categories ordered by sort_order, then label
func (s *Server) handleGetPresetCategories(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	rows, err := s.db.Conn().Query(`
		SELECT key, label, icon, color, sort_order, is_builtin, created_at, updated_at
		FROM preset_categories
		ORDER BY sort_order ASC, label ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	categories := []PresetCategory{}
	for rows.Next() {
		var c PresetCategory
		if err := rows.Scan(&c.Key, &c.Label, &c.Icon, &c.Color, &c.SortOrder, &c.IsBuiltin, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		categories = append(categories, c)
	}

	writeData(w, categories)
}

// handleCreatePresetCategory creates a new custom category
func (s *Server) handleCreatePresetCategory(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Key       string `json:"key"`
		Label     string `json:"label"`
		Icon      string `json:"icon"`
		Color     string `json:"color"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate key
	if req.Key == "" || !validCategoryKey.MatchString(req.Key) {
		http.Error(w, "key must be lowercase slug (a-z, 0-9, hyphens, underscores, max 31 chars)", http.StatusBadRequest)
		return
	}
	if req.Label == "" {
		http.Error(w, "label is required", http.StatusBadRequest)
		return
	}
	if req.Icon == "" {
		req.Icon = "📦"
	}
	if req.Color == "" {
		req.Color = "#8b5cf6"
	}

	// Check for duplicate key
	var existing int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM preset_categories WHERE key = ?", req.Key).Scan(&existing); err == nil && existing > 0 {
		http.Error(w, "category key already exists", http.StatusConflict)
		return
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.Conn().Exec(`
		INSERT INTO preset_categories (key, label, icon, color, sort_order, is_builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
	`, req.Key, req.Label, req.Icon, req.Color, req.SortOrder, now, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeData(w, PresetCategory{
		Key:       req.Key,
		Label:     req.Label,
		Icon:      req.Icon,
		Color:     req.Color,
		SortOrder: req.SortOrder,
		IsBuiltin: 0,
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// handleUpdatePresetCategory updates a custom category (label, icon, color, sort_order)
func (s *Server) handleUpdatePresetCategory(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "category key required", http.StatusBadRequest)
		return
	}

	var req struct {
		Label     string `json:"label"`
		Icon      string `json:"icon"`
		Color     string `json:"color"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Check if category exists
	var isBuiltin int
	err := s.db.Conn().QueryRow("SELECT is_builtin FROM preset_categories WHERE key = ?", key).Scan(&isBuiltin)
	if err != nil {
		http.Error(w, "category not found", http.StatusNotFound)
		return
	}
	if req.Label == "" {
		http.Error(w, "label is required", http.StatusBadRequest)
		return
	}
	if req.Icon == "" {
		req.Icon = "📦"
	}
	if req.Color == "" {
		req.Color = "#8b5cf6"
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = s.db.Conn().Exec(`
		UPDATE preset_categories
		SET label = ?, icon = ?, color = ?, sort_order = ?, updated_at = ?
		WHERE key = ?
	`, req.Label, req.Icon, req.Color, req.SortOrder, now, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = isBuiltin // built-in categories can still be edited (label/icon/color/order), just not deleted
	writeJSON(w, map[string]any{"success": true})
}

// handleDeletePresetCategory deletes a custom category (built-in categories are protected)
func (s *Server) handleDeletePresetCategory(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "category key required", http.StatusBadRequest)
		return
	}

	// Check if category is built-in
	var isBuiltin int
	err := s.db.Conn().QueryRow("SELECT is_builtin FROM preset_categories WHERE key = ?", key).Scan(&isBuiltin)
	if err != nil {
		http.Error(w, "category not found", http.StatusNotFound)
		return
	}
	if isBuiltin == 1 {
		http.Error(w, "built-in categories cannot be deleted", http.StatusForbidden)
		return
	}

	// Check if any preset still references this category
	var refCount int
	if err := s.db.Conn().QueryRow("SELECT COUNT(*) FROM provider_presets WHERE category = ?", key).Scan(&refCount); err == nil && refCount > 0 {
		http.Error(w, "category is still in use by presets", http.StatusConflict)
		return
	}

	_, err = s.db.Conn().Exec("DELETE FROM preset_categories WHERE key = ?", key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"success": true})
}

// handleSeedBuiltinCategories inserts any built-in category that is missing.
//
// Seeding is additive, not bail-out: an install that seeded once before a new
// category existed must still receive it. The earlier version returned early
// whenever any built-in was present, which froze such installs at the old set.
func (s *Server) handleSeedBuiltinCategories(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	builtins := []PresetCategory{
		{Key: "foundation", Label: "Foundation Models", Icon: "🧠", Color: "#8b5cf6", SortOrder: 10},
		{Key: "open", Label: "Open Models", Icon: "🌐", Color: "#10b981", SortOrder: 20},
		{Key: "inference", Label: "Inference Cloud", Icon: "⚡", Color: "#f59e0b", SortOrder: 30},
		{Key: "aggregator", Label: "Aggregators", Icon: "🔀", Color: "#3b82f6", SortOrder: 40},
		{Key: "search", Label: "Search & Augment", Icon: "🔍", Color: "#ec4899", SortOrder: 50},
		{Key: "local", Label: "Local & Self-Host", Icon: "🏠", Color: "#6366f1", SortOrder: 60},
		// free-tier and coding exist because presets already reference them, but
		// they were never registered as categories — leaving eight presets
		// pointing at keys the UI cannot label.
		{Key: "free-tier", Label: "Free & Open", Icon: "🆓", Color: "#22c55e", SortOrder: 70},
		{Key: "coding", Label: "Coding & Agents", Icon: "💻", Color: "#06b6d4", SortOrder: 80},
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	inserted := 0
	updated := 0
	for _, c := range builtins {
		var exists int
		if err := s.db.Conn().QueryRow(
			"SELECT COUNT(*) FROM preset_categories WHERE key = ?", c.Key,
		).Scan(&exists); err == nil && exists > 0 {
			// Refresh the label/icon/colour of an existing built-in so a renamed
			// category reaches installs that seeded the old label. The key is
			// the identity and never changes; manual categories are not in this
			// list, so only built-ins are ever updated.
			if _, err := s.db.Conn().Exec(
				`UPDATE preset_categories SET label = ?, icon = ?, color = ?, sort_order = ?, updated_at = ?
				   WHERE key = ? AND is_builtin = 1`,
				c.Label, c.Icon, c.Color, c.SortOrder, now, c.Key,
			); err == nil {
				updated++
			}
			continue
		}
		_, err := s.db.Conn().Exec(`
			INSERT INTO preset_categories (key, label, icon, color, sort_order, is_builtin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?)
		`, c.Key, c.Label, c.Icon, c.Color, c.SortOrder, now, now)
		if err == nil {
			inserted++
		}
	}

	writeJSON(w, map[string]any{"success": true, "inserted": inserted, "updated": updated})
}

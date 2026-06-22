package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sanhaji182/lintasan-go/internal/guard"
)

func (s *Server) handleAccessLogs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	results := s.accessLogStore.Search(query, limit)
	writeJSON(w, map[string]any{"data": results})
}

func (s *Server) handleAccessLogStats(w http.ResponseWriter, r *http.Request) {
	stats := s.accessLogStore.Stats()
	writeJSON(w, map[string]any{"data": stats})
}

func (s *Server) handleGuardCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text   string            `json:"text"`
		Config guard.GuardConfig `json:"config,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]any{"error": "invalid request body"})
		return
	}

	if req.Text == "" {
		writeJSON(w, map[string]any{"error": "text field required"})
		return
	}

	// Use default config if none provided
	cfg := req.Config
	if !cfg.PIIEnabled && !cfg.InjectionEnabled && !cfg.ContentEnabled {
		cfg = guard.DefaultGuardConfig()
	}

	violations := guard.Check(req.Text, cfg)
	writeJSON(w, map[string]any{
		"data": map[string]any{
			"clean":      len(violations) == 0,
			"violations": violations,
		},
	})
}

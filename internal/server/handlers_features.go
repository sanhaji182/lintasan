package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/sanhaji182/lintasan-go/internal/translator"
)

// handleMCPTools returns list of registered MCP tools
func (s *Server) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tools": []map[string]any{
			{"name": "lintasan.health", "description": "Check server health"},
			{"name": "lintasan.models", "description": "List available models"},
			{"name": "lintasan.providers", "description": "List configured providers"},
			{"name": "lintasan.stats", "description": "Get usage statistics"},
			{"name": "lintasan.compress", "description": "Compress text"},
			{"name": "lintasan.memory.store", "description": "Store memory"},
			{"name": "lintasan.memory.search", "description": "Search memory"},
			{"name": "lintasan.memory.delete", "description": "Delete memory"},
			{"name": "lintasan.guardrails.check", "description": "Check for PII/injection"},
			{"name": "lintasan.health.providers", "description": "Check provider health"},
			{"name": "lintasan.discover", "description": "Discover free providers"},
			{"name": "lintasan.routing", "description": "Get routing config"},
			{"name": "lintasan.savings", "description": "Get cost savings"},
			{"name": "lintasan.tools", "description": "List all tools"},
		},
		"total": 14,
	})
}

// handleSavingsSummary returns cost savings summary
func (s *Server) handleSavingsSummary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get stats from database
	var totalRequests int
	var totalTokens int64
	s.db.Conn().QueryRow("SELECT COUNT(*), COALESCE(SUM(input_tokens + output_tokens), 0) FROM access_logs").Scan(&totalRequests, &totalTokens)

	// Calculate estimated savings (simplified)
	estimatedSavings := float64(totalTokens) * 0.000002 // $2 per 1M tokens average

	json.NewEncoder(w).Encode(map[string]any{
		"total_savings":  estimatedSavings,
		"total_requests": totalRequests,
		"total_tokens":   totalTokens,
		"breakdown": map[string]any{
			"compression": estimatedSavings * 0.4,
			"routing":     estimatedSavings * 0.3,
			"cache":       estimatedSavings * 0.2,
			"free_tier":   estimatedSavings * 0.1,
		},
	})
}

// handleSavingsHistory returns savings history
func (s *Server) handleSavingsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rows, err := s.db.Conn().Query(`
		SELECT DATE(timestamp) as date, COUNT(*) as requests, SUM(input_tokens + output_tokens) as tokens
		FROM access_logs
		GROUP BY DATE(timestamp)
		ORDER BY date DESC
		LIMIT 30
	`)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"history": []any{}})
		return
	}
	defer rows.Close()

	var history []map[string]any
	for rows.Next() {
		var date string
		var requests int
		var tokens int64
		rows.Scan(&date, &requests, &tokens)
		history = append(history, map[string]any{
			"date":     date,
			"requests": requests,
			"tokens":   tokens,
			"savings":  float64(tokens) * 0.000002,
		})
	}

	if history == nil {
		history = []map[string]any{}
	}

	json.NewEncoder(w).Encode(map[string]any{"history": history})
}

// handleTranslate translates between API formats
func (s *Server) handleTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Get target format from query
	targetFormat := r.URL.Query().Get("to")
	if targetFormat == "" {
		targetFormat = "openai"
	}

	// Detect source format
	srcFormat := translator.DetectFormat(body)

	// Translate
	result, err := translator.Translate(body, srcFormat, translator.Format(targetFormat), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}

// handleTranslateFormats returns supported formats
func (s *Server) handleTranslateFormats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"formats": []string{"openai", "anthropic", "gemini", "cohere", "mistral"},
		"auto_detect": true,
	})
}

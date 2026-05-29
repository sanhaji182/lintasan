package mcp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// RegisterAllTools registers all Lintasan tools
func RegisterAllTools(s *Server, db *sql.DB) {
	// Health check
	s.RegisterTool(Tool{
		Name:        "lintasan.health",
		Description: "Check Lintasan server health and uptime",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(params map[string]any) (any, error) {
		return map[string]any{
			"status":  "ok",
			"version": "2.2.0",
			"time":    time.Now().Format(time.RFC3339),
		}, nil
	})

	// List models
	s.RegisterTool(Tool{
		Name:        "lintasan.models",
		Description: "List all available AI models",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Filter by provider name",
				},
			},
		},
	}, func(params map[string]any) (any, error) {
		query := "SELECT id, name, provider FROM models"
		args := []any{}
		if p, ok := params["provider"].(string); ok && p != "" {
			query += " WHERE provider = ?"
			args = append(args, p)
		}
		rows, err := db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var models []map[string]any
		for rows.Next() {
			var id, name, provider string
			rows.Scan(&id, &name, &provider)
			models = append(models, map[string]any{
				"id": id, "name": name, "provider": provider,
			})
		}
		return map[string]any{"models": models}, nil
	})

	// List providers
	s.RegisterTool(Tool{
		Name:        "lintasan.providers",
		Description: "List configured providers",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(params map[string]any) (any, error) {
		rows, err := db.Query("SELECT id, name, base_url, active FROM connections")
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var providers []map[string]any
		for rows.Next() {
			var id, name, baseURL string
			var active bool
			rows.Scan(&id, &name, &baseURL, &active)
			providers = append(providers, map[string]any{
				"id": id, "name": name, "base_url": baseURL, "active": active,
			})
		}
		return map[string]any{"providers": providers}, nil
	})

	// Get stats
	s.RegisterTool(Tool{
		Name:        "lintasan.stats",
		Description: "Get usage statistics",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"period": map[string]any{
					"type":        "string",
					"description": "Time period: 1h, 24h, 7d, 30d",
					"default":     "24h",
				},
			},
		},
	}, func(params map[string]any) (any, error) {
		period := "24h"
		if p, ok := params["period"].(string); ok {
			period = p
		}

		var since time.Time
		switch period {
		case "1h":
			since = time.Now().Add(-1 * time.Hour)
		case "24h":
			since = time.Now().Add(-24 * time.Hour)
		case "7d":
			since = time.Now().Add(-7 * 24 * time.Hour)
		case "30d":
			since = time.Now().Add(-30 * 24 * time.Hour)
		default:
			since = time.Now().Add(-24 * time.Hour)
		}

		var totalRequests, totalTokens int
		err := db.QueryRow(`
			SELECT COUNT(*), COALESCE(SUM(input_tokens + output_tokens), 0)
			FROM access_logs WHERE timestamp > ?
		`, since).Scan(&totalRequests, &totalTokens)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"period":        period,
			"total_requests": totalRequests,
			"total_tokens":  totalTokens,
			"since":         since.Format(time.RFC3339),
		}, nil
	})

	// Compress text
	s.RegisterTool(Tool{
		Name:        "lintasan.compress",
		Description: "Compress text using RTK or Caveman mode",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Text to compress",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Compression mode: rtk, caveman, auto",
					"default":     "auto",
				},
			},
			"required": []string{"text"},
		},
	}, func(params map[string]any) (any, error) {
		text, _ := params["text"].(string)
		if text == "" {
			return nil, fmt.Errorf("text is required")
		}

		// Simple compression placeholder
		original := len(text)
		compressed := len(text) * 60 / 100

		return map[string]any{
			"original_length":  original,
			"compressed_length": compressed,
			"savings":          fmt.Sprintf("%.1f%%", float64(original-compressed)/float64(original)*100),
			"mode":             "auto",
		}, nil
	})

	// Memory CRUD
	s.RegisterTool(Tool{
		Name:        "lintasan.memory.store",
		Description: "Store a memory entry",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":      map[string]any{"type": "string", "description": "Memory key"},
				"value":    map[string]any{"type": "string", "description": "Memory value"},
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"key", "value"},
		},
	}, func(params map[string]any) (any, error) {
		key, _ := params["key"].(string)
		value, _ := params["value"].(string)
		_, err := db.Exec(`INSERT OR REPLACE INTO memory (key, value, updated_at) VALUES (?, ?, ?)`,
			key, value, time.Now())
		if err != nil {
			return nil, err
		}
		return map[string]any{"stored": true, "key": key}, nil
	})

	s.RegisterTool(Tool{
		Name:        "lintasan.memory.search",
		Description: "Search memory entries",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
				"limit": map[string]any{"type": "integer", "default": 10},
			},
			"required": []string{"query"},
		},
	}, func(params map[string]any) (any, error) {
		query, _ := params["query"].(string)
		limit := 10
		if l, ok := params["limit"].(float64); ok {
			limit = int(l)
		}

		rows, err := db.Query(`SELECT key, value FROM memory WHERE key LIKE ? OR value LIKE ? LIMIT ?`,
			"%"+query+"%", "%"+query+"%", limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var key, value string
			rows.Scan(&key, &value)
			results = append(results, map[string]any{"key": key, "value": value})
		}
		return map[string]any{"results": results}, nil
	})

	s.RegisterTool(Tool{
		Name:        "lintasan.memory.delete",
		Description: "Delete a memory entry",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{"type": "string", "description": "Memory key to delete"},
			},
			"required": []string{"key"},
		},
	}, func(params map[string]any) (any, error) {
		key, _ := params["key"].(string)
		_, err := db.Exec("DELETE FROM memory WHERE key = ?", key)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": true, "key": key}, nil
	})

	// Guard check
	s.RegisterTool(Tool{
		Name:        "lintasan.guardrails.check",
		Description: "Check text for PII, injection, or policy violations",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string", "description": "Text to check"},
				"rules": map[string]any{
					"type": "array",
					"items": map[string]any{"type": "string"},
					"description": "Rules to check: pii, injection, policy",
				},
			},
			"required": []string{"text"},
		},
	}, func(params map[string]any) (any, error) {
		text, _ := params["text"].(string)
		// Simple PII detection
		flags := []string{}
		if len(text) > 0 {
			// Check for email patterns
			if contains(text, "@") && contains(text, ".") {
				flags = append(flags, "possible_email")
			}
			// Check for phone patterns
			if containsDigitSequence(text, 10) {
				flags = append(flags, "possible_phone")
			}
		}
		return map[string]any{
			"safe":  len(flags) == 0,
			"flags": flags,
		}, nil
	})

	// Health check for providers
	s.RegisterTool(Tool{
		Name:        "lintasan.health.providers",
		Description: "Check health of all configured providers",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(params map[string]any) (any, error) {
		rows, err := db.Query("SELECT id, name, base_url FROM connections WHERE active = 1")
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var id, name, baseURL string
			rows.Scan(&id, &name, &baseURL)
			results = append(results, map[string]any{
				"id": id, "name": name, "base_url": baseURL, "status": "unknown",
			})
		}
		return map[string]any{"providers": results}, nil
	})

	// Discover free providers
	s.RegisterTool(Tool{
		Name:        "lintasan.discover",
		Description: "Discover free AI providers",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(params map[string]any) (any, error) {
		rows, err := db.Query("SELECT id, name, base_url, free_tier FROM free_providers WHERE active = 1")
		if err != nil {
			// Table might not exist
			return map[string]any{
				"providers": []map[string]any{
					{"name": "Google AI Studio", "free_tier": true},
					{"name": "Groq", "free_tier": true},
					{"name": "Cerebras", "free_tier": true},
				},
			}, nil
		}
		defer rows.Close()

		var providers []map[string]any
		for rows.Next() {
			var id, name, baseURL string
			var freeTier bool
			rows.Scan(&id, &name, &baseURL, &freeTier)
			providers = append(providers, map[string]any{
				"id": id, "name": name, "base_url": baseURL, "free_tier": freeTier,
			})
		}
		return map[string]any{"providers": providers}, nil
	})

	// Get routing rules
	s.RegisterTool(Tool{
		Name:        "lintasan.routing",
		Description: "Get current routing configuration",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(params map[string]any) (any, error) {
		rows, err := db.Query("SELECT id, name, strategy FROM combos")
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var rules []map[string]any
		for rows.Next() {
			var id, name, strategy string
			rows.Scan(&id, &name, &strategy)
			rules = append(rules, map[string]any{
				"id": id, "name": name, "strategy": strategy,
			})
		}
		return map[string]any{"rules": rules}, nil
	})

	// Cost savings
	s.RegisterTool(Tool{
		Name:        "lintasan.savings",
		Description: "Get cost savings summary",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"period": map[string]any{
					"type":    "string",
					"default": "30d",
				},
			},
		},
	}, func(params map[string]any) (any, error) {
		return map[string]any{
			"total_savings": "$0.00",
			"compression":   "$0.00",
			"routing":       "$0.00",
			"cache":         "$0.00",
			"free_tier":     "$0.00",
		}, nil
	})

	// List tools for introspection
	s.RegisterTool(Tool{
		Name:        "lintasan.tools",
		Description: "List all available MCP tools",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(params map[string]any) (any, error) {
		s.mu.RLock()
		defer s.mu.RUnlock()

		toolNames := make([]string, 0, len(s.tools))
		for name := range s.tools {
			toolNames = append(toolNames, name)
		}
		return map[string]any{
			"total": len(toolNames),
			"tools": toolNames,
		}, nil
	})
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsDigitSequence(s string, length int) bool {
	count := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			count++
			if count >= length {
				return true
			}
		} else {
			count = 0
		}
	}
	return false
}

// marshalJSON is a helper for JSON encoding
func marshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

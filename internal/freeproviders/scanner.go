// Package freeproviders provides local provider discovery by scanning
// known ports for running LLM inference servers (Ollama, LM Studio,
// GPT4All, LocalAI, etc.) and auto-registering them as connections.
package freeproviders

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ProviderTarget represents a known local provider configuration to probe.
type ProviderTarget struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`
	Port        int    `json:"port"`
	DetectPath  string `json:"detect_path"`  // path to probe for discovery
	ModelsPath  string `json:"models_path"`  // path to fetch models list
	Format      string `json:"format"`       // "openai" or "ollama"
	ChatPath    string `json:"chat_path"`
	Type        string `json:"type"`         // "local"
	Description string `json:"description"`
}

// DiscoveryResult holds the outcome of probing a single provider.
type DiscoveryResult struct {
	Provider   string   `json:"provider"`
	Port       int      `json:"port"`
	ModelsFound []string `json:"models_found"`
	ModelCount int      `json:"model_count"`
	Status     string   `json:"status"`   // "available", "unreachable", "error"
	Error      string   `json:"error,omitempty"`
	Registered bool     `json:"registered"` // true if auto-registered as connection
}

// DiscoverResponse is the top-level response from a discovery scan.
type DiscoverResponse struct {
	Results       []DiscoveryResult `json:"results"`
	TotalFound    int               `json:"total_found"`
	NewRegistered int               `json:"new_registered"`
	ScannedAt     string            `json:"scanned_at"`
}

// DefaultTargets returns the standard set of local providers to scan.
func DefaultTargets() []ProviderTarget {
	return []ProviderTarget{
		{
			ID:          "ollama-local",
			Name:        "Ollama (Local)",
			Port:        11434,
			DetectPath:  "/api/tags",
			ModelsPath:  "/api/tags",
			Format:      "ollama",
			ChatPath:    "/api/chat",
			Type:        "local",
			Description: "Local LLM inference via Ollama",
		},
		{
			ID:          "lmstudio-local",
			Name:        "LM Studio (Local)",
			Port:        1234,
			DetectPath:  "/v1/models",
			ModelsPath:  "/v1/models",
			Format:      "openai",
			ChatPath:    "/v1/chat/completions",
			Type:        "local",
			Description: "Local LLM inference via LM Studio",
		},
		{
			ID:          "gpt4all-local",
			Name:        "GPT4All (Local)",
			Port:        4891,
			DetectPath:  "/v1/models",
			ModelsPath:  "/v1/models",
			Format:      "openai",
			ChatPath:    "/v1/chat/completions",
			Type:        "local",
			Description: "Local LLM inference via GPT4All",
		},
		{
			ID:          "localai-local",
			Name:        "LocalAI (Local)",
			Port:        8080,
			DetectPath:  "/v1/models",
			ModelsPath:  "/v1/models",
			Format:      "openai",
			ChatPath:    "/v1/chat/completions",
			Type:        "local",
			Description: "Local LLM inference via LocalAI / llama.cpp",
		},
	}
}

// Scanner discovers and auto-registers local LLM providers.
type Scanner struct {
	db      *sql.DB
	client  *http.Client
	mu      sync.Mutex
}

// New creates a new Scanner backed by the given database.
func New(db *sql.DB) *Scanner {
	return &Scanner{
		db: db,
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// probeEndpoint sends a GET request to detectPath on localhost:port and
// returns the response body and status.
func (s *Scanner) probeEndpoint(port int, detectPath string) ([]byte, int, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, detectPath)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Lintasan-Scanner/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// extractModels parses the response body according to format to extract model names.
func (s *Scanner) extractModels(format string, body []byte) []string {
	switch format {
	case "ollama":
		var resp struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil
		}
		models := make([]string, 0, len(resp.Models))
		for _, m := range resp.Models {
			if m.Name != "" {
				models = append(models, m.Name)
			}
		}
		return models

	case "openai":
		var resp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil
		}
		models := make([]string, 0, len(resp.Data))
		for _, m := range resp.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		return models

	default:
		return nil
	}
}

// registerProvider inserts or finds an existing connection for a discovered provider.
// Returns the connection ID and whether it was newly created.
func (s *Scanner) registerProvider(target ProviderTarget, models []string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", target.Port)

	// Check if connection already exists by base_url
	var existingID string
	err := s.db.QueryRow(`SELECT id FROM connections WHERE base_url = ? LIMIT 1`, baseURL).Scan(&existingID)
	if err == nil && existingID != "" {
		// Already registered — update models
		s.syncDiscoveredModels(existingID, models)
		return existingID, false, nil
	}

	// Create new connection
	id := uuid.New().String()
	_, err = s.db.Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, chat_path, models_path, is_active, priority)
		VALUES (?, ?, ?, '', ?, ?, ?, 1, 0)
	`, id, target.Name, baseURL, target.Format, target.ChatPath, target.ModelsPath)
	if err != nil {
		return "", false, fmt.Errorf("insert connection: %w", err)
	}

	// Insert discovered models
	s.syncDiscoveredModels(id, models)

	return id, true, nil
}

// syncDiscoveredModels replaces the discovered models for a connection.
func (s *Scanner) syncDiscoveredModels(connID string, models []string) {
	// Delete old models
	s.db.Exec(`DELETE FROM discovered_models WHERE connection_id = ?`, connID)

	// Insert new models
	for _, modelName := range models {
		mid := uuid.New().String()
		s.db.Exec(`
			INSERT INTO discovered_models (id, connection_id, model_id, model_name, owned_by, is_active)
			VALUES (?, ?, ?, ?, 'local', 1)
		`, mid, connID, modelName, modelName)
	}

	// Update models_count on the connection
	s.db.Exec(`UPDATE connections SET models_count = ?, updated_at = datetime('now', 'localtime') WHERE id = ?`,
		len(models), connID)
}

// Discover scans all default targets and auto-registers any found providers.
func (s *Scanner) Discover() DiscoverResponse {
	return s.DiscoverTargets(DefaultTargets())
}

// DiscoverTargets scans the provided list of targets and auto-registers
// any that respond successfully.
func (s *Scanner) DiscoverTargets(targets []ProviderTarget) DiscoverResponse {
	resp := DiscoverResponse{
		Results:   make([]DiscoveryResult, 0, len(targets)),
		ScannedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for _, t := range targets {
		result := DiscoveryResult{
			Provider: t.Name,
			Port:     t.Port,
			Status:   "unreachable",
		}

		body, status, err := s.probeEndpoint(t.Port, t.DetectPath)
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
			resp.Results = append(resp.Results, result)
			continue
		}

		if status != http.StatusOK {
			result.Status = "unreachable"
			result.Error = fmt.Sprintf("HTTP %d", status)
			resp.Results = append(resp.Results, result)
			continue
		}

		// Provider is reachable — extract models
		models := s.extractModels(t.Format, body)
		result.ModelsFound = models
		result.ModelCount = len(models)
		result.Status = "available"
		resp.TotalFound++

		// Auto-register
		_, registered, regErr := s.registerProvider(t, models)
		if regErr != nil {
			result.Error = fmt.Sprintf("registration failed: %v", regErr)
		}
		result.Registered = registered
		if registered {
			resp.NewRegistered++
		}

		resp.Results = append(resp.Results, result)
	}

	return resp
}

// IsSameBaseURL checks if a base URL is already known (used for dedup).
func IsSameBaseURL(existingURL, newURL string) bool {
	return strings.TrimRight(existingURL, "/") == strings.TrimRight(newURL, "/")
}

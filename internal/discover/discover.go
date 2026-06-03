package discover

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// ModelInfo is a lightweight model descriptor returned by provider /v1/models.
type ModelInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnedBy string `json:"owned_by"`
}

// SyncResult reports the outcome of syncing models for a single connection.
type SyncResult struct {
	ConnectionID string `json:"connection_id"`
	Name         string `json:"name"`
	Status       string `json:"status"` // "ok" or "error"
	ModelsCount  int    `json:"models_count"`
	Error        string `json:"error,omitempty"`
}

// Discoverer fetches models from provider /v1/models and persists them.
type Discoverer struct {
	db          *db.DB
	httpClient  *http.Client
	knownModels map[string][]ModelInfo
}

// NewDiscoverer creates a Discoverer wired to the Lintasan database.
func NewDiscoverer(database *db.DB) *Discoverer {
	return &Discoverer{
		db: database,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		knownModels: knownModels(),
	}
}

// SyncConnection fetches models from a single active connection, persists them,
// and updates the connection row metadata.
func (d *Discoverer) SyncConnection(connID string) (*SyncResult, error) {
	conn, err := d.loadConnection(connID)
	if err != nil {
		return nil, fmt.Errorf("load connection %s: %w", connID, err)
	}
	return d.syncOne(conn)
}

// SyncAll fetches models from every active connection.
func (d *Discoverer) SyncAll() ([]*SyncResult, error) {
	conns, err := d.loadActiveConnections()
	if err != nil {
		return nil, fmt.Errorf("load active connections: %w", err)
	}

	results := make([]*SyncResult, 0, len(conns))
	for _, conn := range conns {
		res, err := d.syncOne(conn)
		if err != nil {
			// Per-connection failure shouldn't abort the entire batch;
			// record it and continue.
			name, _ := conn["name"].(string)
			cid, _ := conn["id"].(string)
			results = append(results, &SyncResult{
				ConnectionID: cid,
				Name:         name,
				Status:       "error",
				Error:        err.Error(),
			})
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// loadConnection reads a single active connection row as a map.
func (d *Discoverer) loadConnection(connID string) (map[string]any, error) {
	row := d.db.Conn().QueryRow(
		`SELECT id, name, base_url, api_key, format, models_path, auth_header, auth_prefix, extra_headers
		 FROM connections WHERE id = ? AND is_active = 1`, connID)

	conn, err := scanConnection(row)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// loadActiveConnections returns every active connection.
func (d *Discoverer) loadActiveConnections() ([]map[string]any, error) {
	rows, err := d.db.Conn().Query(
		`SELECT id, name, base_url, api_key, format, models_path, auth_header, auth_prefix, extra_headers
		 FROM connections WHERE is_active = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []map[string]any
	for rows.Next() {
		conn, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		conns = append(conns, conn)
	}
	return conns, rows.Err()
}

// scanConnection scans a single row (sql.Row or sql.Rows) into a map.
func scanConnection(scanner interface {
	Scan(dest ...any) error
}) (map[string]any, error) {
	var id, name, baseURL, apiKey, format string
	var modelsPath, authHeader, authPrefix, extraHeaders sql.NullString

	err := scanner.Scan(&id, &name, &baseURL, &apiKey, &format,
		&modelsPath, &authHeader, &authPrefix, &extraHeaders)
	if err != nil {
		return nil, err
	}

	conn := map[string]any{
		"id":       id,
		"name":     name,
		"base_url": baseURL,
		"api_key":  apiKey,
		"format":   format,
	}
	if modelsPath.Valid {
		conn["models_path"] = modelsPath.String
	}
	if authHeader.Valid {
		conn["auth_header"] = authHeader.String
	}
	if authPrefix.Valid {
		conn["auth_prefix"] = authPrefix.String
	}
	if extraHeaders.Valid {
		conn["extra_headers"] = extraHeaders.String
	}
	return conn, nil
}

// syncOne runs the full sync cycle for one connection map.
func (d *Discoverer) syncOne(conn map[string]any) (*SyncResult, error) {
	cid, _ := conn["id"].(string)
	cname, _ := conn["name"].(string)

	models, err := d.fetchModelsFromProvider(conn)
	if err != nil {
		return &SyncResult{
			ConnectionID: cid,
			Name:         cname,
			Status:       "error",
			Error:        err.Error(),
		}, nil
	}

	// Persist: delete old auto-discovered rows for this connection, then insert.
	if _, err := d.db.Conn().Exec(
		`DELETE FROM discovered_models WHERE connection_id = ? AND owned_by != 'manual'`, cid,
	); err != nil {
		return nil, fmt.Errorf("delete old discovered_models: %w", err)
	}

	count := 0
	for _, m := range models {
		if m.ID == "" {
			continue
		}
		displayName := m.Name
		if displayName == "" {
			displayName = m.ID
		}
		_, err := d.db.Conn().Exec(
			`INSERT OR REPLACE INTO discovered_models
			 (id, connection_id, model_id, model_name, owned_by, is_active)
			 VALUES (?, ?, ?, ?, ?, 1)`,
			uuid.New().String(), cid, m.ID, displayName, m.OwnedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("insert discovered model %q: %w", m.ID, err)
		}
		count++
	}

	// Update connection metadata.
	if _, err := d.db.Conn().Exec(
		`UPDATE connections SET models_count = ?, last_sync = datetime('now', 'localtime') WHERE id = ?`,
		count, cid,
	); err != nil {
		return nil, fmt.Errorf("update connection metadata: %w", err)
	}

	return &SyncResult{
		ConnectionID: cid,
		Name:         cname,
		Status:       "ok",
		ModelsCount:  count,
	}, nil
}

// fetchModelsFromProvider calls GET {base_url}{models_path}, parses the
// response, and falls back to known models when the API returns nothing.
func (d *Discoverer) fetchModelsFromProvider(conn map[string]any) ([]ModelInfo, error) {
	baseURL, _ := conn["base_url"].(string)
	if baseURL == "" {
		return d.fallbackModels(conn)
	}

	modelsPath, _ := conn["models_path"].(string)
	if modelsPath == "" {
		modelsPath = "/v1/models"
	}

	url := strings.TrimRight(baseURL, "/") + modelsPath

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return d.fallbackModels(conn)
	}

	// Extra headers.
	if eh, ok := conn["extra_headers"].(string); ok && eh != "" {
		var extra map[string]string
		if json.Unmarshal([]byte(eh), &extra) == nil {
			for k, v := range extra {
				req.Header.Set(k, v)
			}
		}
	}

	// Auth.
	apiKey, _ := conn["api_key"].(string)
	if apiKey != "" {
		authHeader, _ := conn["auth_header"].(string)
		if authHeader == "" {
			authHeader = "Authorization"
		}
		authPrefix, _ := conn["auth_prefix"].(string)
		if authPrefix == "" {
			authPrefix = "Bearer "
		}
		req.Header.Set(authHeader, authPrefix+apiKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return d.fallbackModels(conn)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB max

	if resp.StatusCode >= 400 {
		return d.fallbackModels(conn)
	}

	models := parseModelsResponse(body)
	if len(models) == 0 {
		return d.fallbackModels(conn)
	}

	return models, nil
}

// parseModelsResponse normalises common /v1/models shapes.
func parseModelsResponse(body []byte) []ModelInfo {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}

	var items []any
	switch v := raw.(type) {
	case []any:
		items = v
	case map[string]any:
		if arr, ok := v["data"].([]any); ok {
			items = arr
		} else if arr, ok := v["models"].([]any); ok {
			items = arr
		}
	}

	var models []ModelInfo
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		info := ModelInfo{}
		if id, _ := m["id"].(string); id != "" {
			info.ID = id
		} else if id, _ := m["model"].(string); id != "" {
			info.ID = id
		} else if id, _ := m["name"].(string); id != "" {
			info.ID = id
		}

		if name, _ := m["name"].(string); name != "" {
			info.Name = name
		} else {
			info.Name = info.ID
		}

		if ob, _ := m["owned_by"].(string); ob != "" {
			info.OwnedBy = ob
		} else if ob, _ := m["owner"].(string); ob != "" {
			info.OwnedBy = ob
		}

		if info.ID != "" {
			models = append(models, info)
		}
	}

	return models
}

// fallbackModels returns known models for the connection's format (or empty).
func (d *Discoverer) fallbackModels(conn map[string]any) ([]ModelInfo, error) {
	format, _ := conn["format"].(string)
	if known, ok := d.knownModels[format]; ok {
		return known, nil
	}
	return nil, nil
}

// knownModels returns the hardcoded fallback map keyed by format.
func knownModels() map[string][]ModelInfo {
	return map[string][]ModelInfo{
		"commandcode": {
			{ID: "deepseek/deepseek-v4-pro", Name: "DeepSeek V4 Pro", OwnedBy: "deepseek"},
			{ID: "deepseek/deepseek-v3", Name: "DeepSeek V3", OwnedBy: "deepseek"},
			{ID: "deepseek/deepseek-r1", Name: "DeepSeek R1", OwnedBy: "deepseek"},
			{ID: "kimi/kimi-k2.6", Name: "Kimi K2.6", OwnedBy: "moonshot"},
			{ID: "glm/glm-4.7", Name: "GLM 4.7", OwnedBy: "zhipu"},
			{ID: "minimax/minimax-m2.7", Name: "MiniMax M2.7", OwnedBy: "minimax"},
			{ID: "qwen/qwen3-coder", Name: "Qwen3 Coder", OwnedBy: "alibaba"},
		},
		"openai": {
			{ID: "gpt-4o", Name: "GPT-4o", OwnedBy: "openai"},
			{ID: "gpt-4o-mini", Name: "GPT-4o Mini", OwnedBy: "openai"},
			{ID: "o3-mini", Name: "o3-mini", OwnedBy: "openai"},
			{ID: "o1", Name: "o1", OwnedBy: "openai"},
			{ID: "o1-mini", Name: "o1-mini", OwnedBy: "openai"},
		},
		"anthropic": {
			{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", OwnedBy: "anthropic"},
			{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", OwnedBy: "anthropic"},
			{ID: "claude-haiku-3-5", Name: "Claude 3.5 Haiku", OwnedBy: "anthropic"},
		},
	}
}

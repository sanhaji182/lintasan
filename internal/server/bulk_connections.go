package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

type BulkTestRequest struct {
	IDs     []string `json:"ids"`
	BaseURL string   `json:"base_url"`
	All     bool     `json:"all"`
}

type BulkTestItemResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`
	Status      string `json:"status"` // "ok" | "error"
	ModelsCount int    `json:"models_count,omitempty"`
	LatencyMs   int64  `json:"latency_ms"`
	Error       string `json:"error,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

type BulkTestResponse struct {
	Success   bool                 `json:"success"`
	Total     int                  `json:"total"`
	Healthy   int                  `json:"healthy"`
	Failed    int                  `json:"failed"`
	Results   []BulkTestItemResult `json:"results"`
	FailedIDs []string             `json:"failed_ids"`
}

type BulkActionRequest struct {
	IDs []string `json:"ids"`
}

type connToTest struct {
	id         string
	name       string
	baseURL    string
	apiKey     string
	oauthProv  string
	modelsPath string
	authHeader string
	authPrefix string
}

func (s *Server) handleBulkTestConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		http.Error(w, `{"error":"database unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req BulkTestRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	// 1. Fetch connections to test
	var query string
	var args []any

	if len(req.IDs) > 0 {
		query = "SELECT id, name, base_url, api_key, COALESCE(oauth_provider,''), COALESCE(models_path,'/v1/models'), COALESCE(auth_header,'Authorization'), COALESCE(auth_prefix,'Bearer ') FROM connections WHERE id = ?"
	} else if strings.TrimSpace(req.BaseURL) != "" {
		query = "SELECT id, name, base_url, api_key, COALESCE(oauth_provider,''), COALESCE(models_path,'/v1/models'), COALESCE(auth_header,'Authorization'), COALESCE(auth_prefix,'Bearer ') FROM connections WHERE base_url = ?"
		args = append(args, strings.TrimSpace(req.BaseURL))
	} else {
		// All connections
		query = "SELECT id, name, base_url, api_key, COALESCE(oauth_provider,''), COALESCE(models_path,'/v1/models'), COALESCE(auth_header,'Authorization'), COALESCE(auth_prefix,'Bearer ') FROM connections"
	}

	var targets []connToTest
	if len(req.IDs) > 0 {
		stmt, err := s.db.Conn().Prepare(query)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"prepare failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		defer stmt.Close()
		for _, id := range req.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			var c connToTest
			if err := stmt.QueryRow(id).Scan(&c.id, &c.name, &c.baseURL, &c.apiKey, &c.oauthProv, &c.modelsPath, &c.authHeader, &c.authPrefix); err == nil {
				targets = append(targets, c)
			}
		}
	} else {
		rows, err := s.db.Conn().Query(query, args...)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"query failed: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var c connToTest
			if err := rows.Scan(&c.id, &c.name, &c.baseURL, &c.apiKey, &c.oauthProv, &c.modelsPath, &c.authHeader, &c.authPrefix); err == nil {
				targets = append(targets, c)
			}
		}
	}

	if len(targets) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BulkTestResponse{
			Success:   true,
			Total:     0,
			Healthy:   0,
			Failed:    0,
			Results:   []BulkTestItemResult{},
			FailedIDs: []string{},
		})
		return
	}

	// 2. Concurrently test targets using worker pool
	concurrency := 15
	if len(targets) < concurrency {
		concurrency = len(targets)
	}

	jobs := make(chan connToTest, len(targets))
	resultsCh := make(chan BulkTestItemResult, len(targets))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				resultsCh <- s.executeSingleConnTest(target)
			}
		}()
	}

	for _, target := range targets {
		jobs <- target
	}
	close(jobs)

	wg.Wait()
	close(resultsCh)

	// 3. Assemble response
	res := BulkTestResponse{
		Success:   true,
		Total:     len(targets),
		Results:   make([]BulkTestItemResult, 0, len(targets)),
		FailedIDs: make([]string, 0),
	}

	for item := range resultsCh {
		res.Results = append(res.Results, item)
		if item.Status == "ok" {
			res.Healthy++
		} else {
			res.Failed++
			res.FailedIDs = append(res.FailedIDs, item.ID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) executeSingleConnTest(c connToTest) BulkTestItemResult {
	key := c.apiKey
	if c.oauthProv != "" && s.oauthMgr != nil {
		if tok, err := s.oauthMgr.GetActiveToken(c.oauthProv); err == nil && tok != "" {
			key = tok
		}
	}

	path := c.modelsPath
	if path == "" {
		path = "/v1/models"
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	models, status, _, err := fetchModelsWithContext(ctx, c.baseURL, path, key, c.authHeader, c.authPrefix)
	latency := time.Since(start).Milliseconds()

	if err == nil {
		return BulkTestItemResult{
			ID:          c.id,
			Name:        c.name,
			BaseURL:     c.baseURL,
			Status:      "ok",
			ModelsCount: len(models),
			LatencyMs:   latency,
		}
	}

	// If 5xx, try pingChat
	if key != "" && status >= 500 && status <= 599 {
		pingStatus, _, pingErr := pingChat(c.baseURL, key, c.authHeader, c.authPrefix)
		if pingErr == nil && pingStatus < 400 {
			return BulkTestItemResult{
				ID:        c.id,
				Name:      c.name,
				BaseURL:   c.baseURL,
				Status:    "ok",
				LatencyMs: time.Since(start).Milliseconds(),
			}
		}
	}

	errMsg := err.Error()
	if status > 0 {
		errMsg = fmt.Sprintf("HTTP %d: %s", status, errMsg)
	}

	return BulkTestItemResult{
		ID:        c.id,
		Name:      c.name,
		BaseURL:   c.baseURL,
		Status:    "error",
		LatencyMs: latency,
		Error:     errMsg,
		ErrorCode: status,
	}
}

func fetchModelsWithContext(ctx context.Context, base, path, key, h, prefix string) ([]any, int, []byte, error) {
	if base == "" {
		return nil, 0, nil, fmt.Errorf("base_url required")
	}
	url := provider.JoinUpstreamPath(base, path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	if key != "" {
		if h == "" {
			h = "Authorization"
		}
		req.Header.Set(h, prefix+key)
	}
	c := &http.Client{Timeout: 8 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, b, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	var data map[string]any
	json.Unmarshal(b, &data)
	if arr, ok := data["data"].([]any); ok {
		return arr, resp.StatusCode, b, nil
	}
	if arr, ok := data["models"].([]any); ok {
		return arr, resp.StatusCode, b, nil
	}
	return []any{}, resp.StatusCode, b, nil
}

func (s *Server) handleBulkDeleteConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		http.Error(w, `{"error":"database unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req BulkActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if len(req.IDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true, "deleted_count": 0})
		return
	}

	tx, err := s.db.Conn().Begin()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tx begin: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	delConn, err := tx.Prepare("DELETE FROM connections WHERE id = ?")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"prepare: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer delConn.Close()

	delModels, _ := tx.Prepare("DELETE FROM discovered_models WHERE connection_id = ?")
	if delModels != nil {
		defer delModels.Close()
	}

	deletedCount := 0
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		res, err := delConn.Exec(id)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				deletedCount += int(n)
			}
		}
		if delModels != nil {
			_, _ = delModels.Exec(id)
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tx commit: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if s.proxy != nil {
		s.proxy.RefreshMultiAccountPools()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"deleted_count": deletedCount,
	})
}

func (s *Server) handleBulkDisableConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		http.Error(w, `{"error":"database unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req BulkActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if len(req.IDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true, "disabled_count": 0})
		return
	}

	tx, err := s.db.Conn().Begin()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tx begin: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE connections SET is_active = 0 WHERE id = ?")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"prepare: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	disabledCount := 0
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		res, err := stmt.Exec(id)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				disabledCount += int(n)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tx commit: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if s.proxy != nil {
		s.proxy.RefreshMultiAccountPools()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":        true,
		"disabled_count": disabledCount,
	})
}

func (s *Server) handleBulkEnableConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		http.Error(w, `{"error":"database unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req BulkActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if len(req.IDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true, "enabled_count": 0})
		return
	}

	tx, err := s.db.Conn().Begin()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tx begin: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE connections SET is_active = 1 WHERE id = ?")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"prepare: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	enabledCount := 0
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		res, err := stmt.Exec(id)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				enabledCount += int(n)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"tx commit: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if s.proxy != nil {
		s.proxy.RefreshMultiAccountPools()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"enabled_count": enabledCount,
	})
}

package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/hoplite"
)

const (
	hopliteCredentialName = "hoplite"
	hopliteCredentialEnv  = "HOPLITE_API_KEY"
)

func (s *Server) registerHopliteRoutes() {
	s.mux.HandleFunc("GET /api/experimental/cloud-agents/hoplite/accounts", s.handleHopliteAccounts)
	s.mux.HandleFunc("POST /api/experimental/cloud-agents/hoplite/accounts", s.handleHopliteAccountCreate)
	s.mux.HandleFunc("PATCH /api/experimental/cloud-agents/hoplite/accounts/{id}", s.handleHopliteAccountPatch)
	s.mux.HandleFunc("DELETE /api/experimental/cloud-agents/hoplite/accounts/{id}", s.handleHopliteAccountDelete)
	s.mux.HandleFunc("GET /api/experimental/cloud-agents/hoplite/status", s.handleHopliteStatus)
	s.mux.HandleFunc("POST /api/experimental/cloud-agents/hoplite/test", s.handleHopliteTest)
	s.mux.HandleFunc("GET /api/experimental/cloud-agents/hoplite/projects", s.handleHopliteProjects)
	s.mux.HandleFunc("GET /api/experimental/cloud-agents/hoplite/threads", s.handleHopliteThreads)
	s.mux.HandleFunc("POST /api/experimental/cloud-agents/hoplite/threads", s.handleHopliteCreateThread)
	s.mux.HandleFunc("GET /api/experimental/cloud-agents/hoplite/threads/{id}", s.handleHopliteThread)
	s.mux.HandleFunc("GET /api/experimental/cloud-agents/hoplite/threads/{id}/messages", s.handleHopliteMessages)
}

func (s *Server) handleHopliteStatus(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" || accountID == hopliteConnectionID {
		status := s.credStore().GetStatus(r.Context(), hopliteCredentialName, hopliteCredentialEnv)
		writeData(w, map[string]any{
			"account_id": accountID, "configured": status.Configured, "source": status.Source,
			"masked_value": status.MaskedValue, "env_var": status.EnvVar, "updated_at": status.UpdatedAt,
			"mode": "cloud-agent", "routing": "isolated",
		})
		return
	}
	account, exists := s.hopliteAccountByID(r.Context(), accountID)
	if !exists {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "Hoplite account not found"})
		return
	}
	status := s.credStore().GetStatus(r.Context(), account.CredentialName, "")
	writeData(w, map[string]any{
		"account_id": account.ID, "name": account.Name, "is_active": account.IsActive,
		"health_status": account.HealthStatus, "last_tested_at": account.LastTestedAt,
		"configured": status.Configured, "source": status.Source, "masked_value": status.MaskedValue,
		"updated_at": status.UpdatedAt, "mode": "cloud-agent", "routing": "isolated",
	})
}

func (s *Server) handleHopliteTest(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	client, ok := s.hopliteClient(w, r)
	if !ok {
		return
	}
	started := time.Now()
	projects, meta, err := client.ListProjects(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		accountID = hopliteConnectionID
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		_, _ = s.db.Conn().ExecContext(r.Context(), `UPDATE hoplite_accounts SET health_status='unhealthy',last_tested_at=?,last_error=?,updated_at=? WHERE id=?`, now, "connection test failed", now, accountID)
		writeHopliteError(w, err)
		return
	}
	_, _ = s.db.Conn().ExecContext(r.Context(), `UPDATE hoplite_accounts SET health_status='healthy',last_tested_at=?,last_error='',updated_at=? WHERE id=?`, now, now, accountID)
	writeData(w, map[string]any{
		"ok":                  true,
		"account_id":          accountID,
		"checked_at":          time.Now().UTC().Format(time.RFC3339),
		"latency_ms":          time.Since(started).Milliseconds(),
		"project_count":       len(projects),
		"verified_operations": []string{"project:list"},
		"thread_create":       "not_tested",
		"meta":                meta,
	})
}

func (s *Server) handleHopliteProjects(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	client, ok := s.hopliteClient(w, r)
	if !ok {
		return
	}
	projects, meta, err := client.ListProjects(r.Context())
	if err != nil {
		writeHopliteError(w, err)
		return
	}
	writeData(w, map[string]any{"projects": projects, "meta": meta})
}

func (s *Server) handleHopliteThreads(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))
	if projectID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "projectId is required"})
		return
	}
	client, ok := s.hopliteClient(w, r)
	if !ok {
		return
	}
	threads, meta, err := client.ListThreads(r.Context(), projectID)
	if err != nil {
		writeHopliteError(w, err)
		return
	}
	writeData(w, map[string]any{"threads": threads, "meta": meta})
}

type hopliteCreateThreadBody struct {
	ProjectID         string         `json:"projectId"`
	Prompt            string         `json:"prompt"`
	Title             string         `json:"title"`
	Model             string         `json:"model"`
	Speed             string         `json:"speed"`
	AutoFix           bool           `json:"autoFix"`
	AutoMerge         bool           `json:"autoMerge"`
	ClientOperationID string         `json:"clientOperationId"`
	Repos             []hoplite.Repo `json:"repos"`
}

func (s *Server) handleHopliteCreateThread(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	var body hopliteCreateThreadBody
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	body.ProjectID = strings.TrimSpace(body.ProjectID)
	body.Prompt = strings.TrimSpace(body.Prompt)
	body.Title = strings.TrimSpace(body.Title)
	body.Model = strings.TrimSpace(body.Model)
	body.Speed = strings.TrimSpace(body.Speed)
	body.ClientOperationID = strings.TrimSpace(body.ClientOperationID)
	if body.ProjectID == "" || body.Prompt == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "projectId and prompt are required"})
		return
	}
	if len(body.Prompt) > 100_000 || len(body.Title) > 500 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "thread input exceeds Lintasan limits"})
		return
	}
	if body.Speed != "" && body.Speed != "standard" && body.Speed != "fast" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "speed must be standard or fast"})
		return
	}
	if body.ClientOperationID == "" {
		body.ClientOperationID = newHopliteOperationID()
	}
	if len(body.ClientOperationID) > 64 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "clientOperationId exceeds 64 characters"})
		return
	}

	client, ok := s.hopliteClient(w, r)
	if !ok {
		return
	}
	result, meta, err := client.CreateThread(r.Context(), hoplite.CreateThreadRequest{
		ProjectID:         body.ProjectID,
		Prompt:            body.Prompt,
		Title:             body.Title,
		Model:             body.Model,
		Speed:             body.Speed,
		AutoFix:           body.AutoFix,
		AutoMerge:         body.AutoMerge,
		ClientOperationID: body.ClientOperationID,
		Repos:             body.Repos,
	})
	if err != nil {
		writeHopliteError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusCreated, map[string]any{
		"data": map[string]any{"result": result, "meta": meta},
	})
}

func (s *Server) handleHopliteThread(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	threadID := strings.TrimSpace(r.PathValue("id"))
	if threadID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "thread id is required"})
		return
	}
	client, ok := s.hopliteClient(w, r)
	if !ok {
		return
	}
	thread, meta, err := client.GetThread(r.Context(), threadID)
	if err != nil {
		writeHopliteError(w, err)
		return
	}
	writeData(w, map[string]any{"thread": thread, "meta": meta})
}

func (s *Server) handleHopliteMessages(w http.ResponseWriter, r *http.Request) {
	if !requireCredentialManager(w, r) {
		return
	}
	threadID := strings.TrimSpace(r.PathValue("id"))
	if threadID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "thread id is required"})
		return
	}
	client, ok := s.hopliteClient(w, r)
	if !ok {
		return
	}
	messages, meta, err := client.ListMessages(r.Context(), threadID)
	if err != nil {
		writeHopliteError(w, err)
		return
	}
	writeData(w, map[string]any{"messages": messages, "meta": meta})
}

func (s *Server) hopliteClient(w http.ResponseWriter, r *http.Request) (*hoplite.Client, bool) {
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		accountID = hopliteConnectionID
	}
	account, exists := s.hopliteAccountByID(r.Context(), accountID)
	if !exists && accountID != hopliteConnectionID {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "Hoplite account not found"})
		return nil, false
	}
	if exists && account.IsActive != 1 {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"error": "Hoplite account is inactive"})
		return nil, false
	}
	key, ok := s.hopliteCredentialForAccount(r.Context(), accountID)
	if !ok {
		writeJSONStatus(w, http.StatusPreconditionFailed, map[string]any{
			"error":   "hoplite credential not configured",
			"env_var": hopliteCredentialEnv,
		})
		return nil, false
	}
	return s.newHopliteClient(key, 0), true
}

func (s *Server) hopliteCredential(ctx context.Context) (string, bool) {
	return s.hopliteCredentialForAccount(ctx, hopliteConnectionID)
}

func newHopliteOperationID() string {
	var random [12]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "lintasan-" + hex.EncodeToString(random[:])
	}
	return "lintasan-" + time.Now().UTC().Format("20060102T150405.000000000")
}

func writeHopliteError(w http.ResponseWriter, err error) {
	var upstream *hoplite.UpstreamError
	if errors.As(err, &upstream) {
		payload := map[string]any{"error": upstream.Code}
		if upstream.RequestID != "" {
			payload["request_id"] = upstream.RequestID
		}
		if upstream.RetryAfter != "" {
			payload["retry_after"] = upstream.RetryAfter
			w.Header().Set("Retry-After", upstream.RetryAfter)
		}
		status := upstream.StatusCode
		// 401/403 describe Hoplite credentials or organization membership, not
		// the caller's Lintasan session. Never trigger the dashboard's global
		// auth-expiry handler for an upstream dependency failure.
		if status == http.StatusUnauthorized || status == http.StatusForbidden || status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		writeJSONStatus(w, status, payload)
		return
	}
	writeJSONStatus(w, http.StatusBadGateway, map[string]any{"error": "hoplite upstream unavailable"})
}

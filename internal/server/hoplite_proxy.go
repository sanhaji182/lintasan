package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/hoplite"
)

type hopliteChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// handleChatCompletions preserves normal Lintasan routing while reserving the
// explicit hoplite-agent/<project-id> namespace for long-running coding agents.
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var envelope struct {
		Model    string               `json:"model"`
		Stream   bool                 `json:"stream"`
		Messages []hopliteChatMessage `json:"messages"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "request body exceeds 1 MiB")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if json.Unmarshal(body, &envelope) != nil {
		s.proxy.HandleChatCompletions(w, r)
		return
	}
	if s.handleCloudAgentCombo(w, r, envelope.Model, envelope.Stream, envelope.Messages, body) {
		return
	}
	if !isCloudAgentModel(envelope.Model) {
		s.proxy.HandleChatCompletions(w, r)
		return
	}
	s.handleHopliteCompletion(w, r, envelope.Model, envelope.Stream, envelope.Messages)
}

func (s *Server) handleHopliteCompletion(w http.ResponseWriter, r *http.Request, model string, stream bool, messages []hopliteChatMessage) {
	accountID, projectID, selectedModel, _, validModelID := parseHopliteRoutedModelID(model)
	if !validModelID {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid Hoplite model ID")
		return
	}
	if stream {
		writeOpenAIError(w, http.StatusBadRequest, "unsupported_parameter", "Hoplite agent adapter does not support streaming")
		return
	}
	prompt := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			prompt = strings.TrimSpace(messages[i].Content)
			break
		}
	}
	if prompt == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "a non-empty user message is required")
		return
	}
	account, exists := s.hopliteAccountByID(r.Context(), accountID)
	if !exists || account.IsActive != 1 {
		writeOpenAIError(w, http.StatusServiceUnavailable, "hoplite_account_unavailable", "Hoplite account is inactive or unavailable")
		return
	}
	if account.HealthStatus == "unhealthy" {
		writeOpenAIError(w, http.StatusServiceUnavailable, "hoplite_account_unhealthy", "Hoplite account failed its latest health check")
		return
	}
	if account.CreditsRemaining != nil && *account.CreditsRemaining <= 0 {
		writeOpenAIError(w, http.StatusServiceUnavailable, "hoplite_account_exhausted", "Hoplite account has no remaining credits")
		return
	}
	if account.ExpiresAt != "" {
		if expiry, err := time.Parse(time.RFC3339, account.ExpiresAt); err == nil && !expiry.After(time.Now()) {
			writeOpenAIError(w, http.StatusServiceUnavailable, "hoplite_account_expired", "Hoplite account entitlement has expired")
			return
		}
	}
	key, ok := s.hopliteCredentialForAccount(r.Context(), accountID)
	if !ok {
		writeOpenAIError(w, http.StatusPreconditionFailed, "hoplite_not_configured", "Hoplite credential is not configured")
		return
	}
	timeout := s.hopliteProxyTimeout
	if timeout <= 0 {
		timeout = 4 * time.Minute
	}
	poll := s.hoplitePollInterval
	if poll <= 0 {
		poll = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	client := s.newHopliteClient(key, timeout)
	opID := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if opID == "" {
		opID = newHopliteOperationID()
	}
	if len(opID) > 64 {
		digest := sha256.Sum256([]byte(opID))
		opID = "lintasan-" + hex.EncodeToString(digest[:24])
	}
	created, meta, err := client.CreateThread(ctx, hoplite.CreateThreadRequest{
		ProjectID: projectID, Prompt: prompt, Model: selectedModel, AutoFix: false, AutoMerge: false, ClientOperationID: opID,
	})
	// Idempotency alone cannot prove whether a network timeout happened before
	// or after upstream acceptance; the combo dispatcher handles that ambiguity
	// fail-closed and will not start another autonomous target.
	if err != nil {
		// A transport failure has ambiguous acceptance: the upstream may have
		// created the thread before the response was lost. Never fan out a second
		// agent in that case. Typed HTTP errors are definitive pre-create rejects.
		var upstream *hoplite.UpstreamError
		if !errors.As(err, &upstream) {
			w.Header().Set("X-Lintasan-Agent-Acceptance-Uncertain", "true")
		}
		writeHopliteOpenAIError(w, err)
		return
	}
	thread := created.Thread
	if thread.ID == "" {
		writeOpenAIError(w, http.StatusBadGateway, "invalid_upstream_response", "Hoplite did not return a thread ID")
		return
	}
	// This header is internal dispatch state until copied to the client. From
	// this point onward fallback is forbidden: a real agent job now exists.
	w.Header().Set("X-Lintasan-Agent-Accepted", "true")
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for !hopliteTerminal(thread.Status) {
		select {
		case <-ctx.Done():
			writeOpenAIError(w, http.StatusGatewayTimeout, "agent_timeout", "Hoplite agent did not reach a terminal state before the adapter timeout")
			return
		case <-ticker.C:
			thread, meta, err = client.GetThread(ctx, thread.ID)
			if err != nil {
				writeHopliteOpenAIError(w, err)
				return
			}
		}
	}
	if thread.Status != "ready" && thread.Status != "succeeded" {
		writeOpenAIError(w, http.StatusBadGateway, "agent_failed", "Hoplite agent ended with status "+thread.Status)
		return
	}
	history, _, err := client.ListMessages(ctx, thread.ID)
	if err != nil {
		writeHopliteOpenAIError(w, err)
		return
	}
	answer := ""
	for _, message := range history {
		if message.Role == "assistant" && (message.Kind == "" || message.Kind == "chat") && strings.TrimSpace(message.Content) != "" {
			answer = message.Content
		}
	}
	if answer == "" {
		writeOpenAIError(w, http.StatusBadGateway, "empty_agent_result", "Hoplite agent completed without an assistant chat result")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Lintasan-Provider", "hoplite-agent")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "hoplite-" + thread.ID, "object": "chat.completion", "created": time.Now().Unix(), "model": model,
		"choices":    []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": answer}, "finish_reason": "stop"}},
		"usage":      map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		"x_lintasan": map[string]any{"provider": "hoplite-agent", "agent_semantics": true, "thread_id": thread.ID, "status": thread.Status, "pull_requests": thread.PullRequests, "request_id": meta.RequestID},
	})
}

func hopliteSelectedModelID(projectID, modelID string) string {
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	return "hoplite-model/v1/" + encode(projectID) + "/" + encode(modelID)
}

func hopliteProjectModelIDForAccount(accountID, projectID string) string {
	if accountID == "" || accountID == hopliteConnectionID {
		return "hoplite-agent/" + projectID
	}
	return "hoplite-agent/v2/" + encodeHoplitePart(accountID) + "/" + encodeHoplitePart(projectID)
}

func hopliteSelectedModelIDForAccount(accountID, projectID, modelID string) string {
	if accountID == "" || accountID == hopliteConnectionID {
		return hopliteSelectedModelID(projectID, modelID)
	}
	return "hoplite-model/v2/" + encodeHoplitePart(accountID) + "/" + encodeHoplitePart(projectID) + "/" + encodeHoplitePart(modelID)
}

func parseHopliteRoutedModelID(id string) (accountID, projectID, modelID string, selected, ok bool) {
	if strings.HasPrefix(id, "hoplite-agent/v2/") {
		parts := strings.Split(strings.TrimPrefix(id, "hoplite-agent/v2/"), "/")
		if len(parts) != 2 {
			return "", "", "", false, false
		}
		account, aok := decodeHoplitePart(parts[0])
		project, pok := decodeHoplitePart(parts[1])
		return account, project, "", false, aok && pok
	}
	if strings.HasPrefix(id, "hoplite-model/v2/") {
		parts := strings.Split(strings.TrimPrefix(id, "hoplite-model/v2/"), "/")
		if len(parts) != 3 {
			return "", "", "", false, false
		}
		account, aok := decodeHoplitePart(parts[0])
		project, pok := decodeHoplitePart(parts[1])
		model, mok := decodeHoplitePart(parts[2])
		return account, project, model, true, aok && pok && mok && hoplite.IsKnownModel(model)
	}
	project, model, selected, ok := parseHopliteModelID(id)
	return hopliteConnectionID, project, model, selected, ok
}

func parseHopliteModelID(id string) (projectID, modelID string, selected, ok bool) {
	const aliasPrefix = "hoplite-agent/"
	const selectedPrefix = "hoplite-model/v1/"
	if strings.HasPrefix(id, aliasPrefix) {
		projectID = strings.TrimSpace(strings.TrimPrefix(id, aliasPrefix))
		return projectID, "", false, projectID != ""
	}
	if !strings.HasPrefix(id, selectedPrefix) {
		return "", "", false, false
	}
	parts := strings.Split(strings.TrimPrefix(id, selectedPrefix), "/")
	if len(parts) != 2 {
		return "", "", false, false
	}
	decode := func(value string) (string, bool) {
		decoded, err := base64.RawURLEncoding.DecodeString(value)
		return string(decoded), err == nil && len(decoded) > 0
	}
	var projectOK, modelOK bool
	projectID, projectOK = decode(parts[0])
	modelID, modelOK = decode(parts[1])
	if !projectOK || !modelOK || !hoplite.IsKnownModel(modelID) {
		return "", "", false, false
	}
	return projectID, modelID, true, true
}

func hopliteTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ready", "succeeded", "failed", "blocked", "archived", "cancelled", "canceled":
		return true
	}
	return false
}

func writeOpenAIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": message, "type": "hoplite_agent_error", "code": code}})
}

func writeHopliteOpenAIError(w http.ResponseWriter, err error) {
	var upstream *hoplite.UpstreamError
	if errors.As(err, &upstream) {
		status := upstream.StatusCode
		if status == http.StatusUnauthorized || status == http.StatusForbidden || status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		writeOpenAIError(w, status, upstream.Code, "Hoplite upstream request failed")
		return
	}
	writeOpenAIError(w, http.StatusBadGateway, "upstream_unavailable", "Hoplite upstream is unavailable")
}

func (s *Server) newHopliteClient(key string, timeout time.Duration) *hoplite.Client {
	baseURL := s.hopliteBaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = hoplite.DefaultBaseURL
	}
	httpClient := s.hopliteHTTPClient
	if timeout > 0 {
		if httpClient == nil {
			httpClient = &http.Client{Timeout: timeout}
		} else {
			clone := *httpClient
			clone.Timeout = timeout
			httpClient = &clone
		}
	}
	return hoplite.NewClient(baseURL, key, httpClient)
}

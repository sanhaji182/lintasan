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
	if json.Unmarshal(body, &envelope) != nil || (!strings.HasPrefix(envelope.Model, "hoplite-agent/") && !strings.HasPrefix(envelope.Model, "hoplite-model/v1/")) {
		s.proxy.HandleChatCompletions(w, r)
		return
	}
	s.handleHopliteCompletion(w, r, envelope.Model, envelope.Stream, envelope.Messages)
}

func (s *Server) handleHopliteCompletion(w http.ResponseWriter, r *http.Request, model string, stream bool, messages []hopliteChatMessage) {
	projectID, selectedModel, _, validModelID := parseHopliteModelID(model)
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
	key, ok := s.hopliteCredential(r.Context())
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
	if err != nil {
		writeHopliteOpenAIError(w, err)
		return
	}
	thread := created.Thread
	if thread.ID == "" {
		writeOpenAIError(w, http.StatusBadGateway, "invalid_upstream_response", "Hoplite did not return a thread ID")
		return
	}
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

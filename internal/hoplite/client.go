// Package hoplite implements the server-side Hoplite Cloud Agent REST client.
package hoplite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL  = "https://api.hoplite.sh"
	defaultTimeout  = 25 * time.Second
	maxErrorBody    = 32 << 10
	maxResponseBody = 10 << 20
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type ResponseMeta struct {
	StatusCode int    `json:"status_code"`
	RequestID  string `json:"request_id,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
	RateLimit  string `json:"rate_limit,omitempty"`
	Policy     string `json:"rate_limit_policy,omitempty"`
}

type UpstreamError struct {
	StatusCode int
	Code       string
	RequestID  string
	RetryAfter string
}

func (e *UpstreamError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("hoplite request failed: status=%d code=%s request_id=%s", e.StatusCode, e.Code, e.RequestID)
	}
	return fmt.Sprintf("hoplite request failed: status=%d code=%s", e.StatusCode, e.Code)
}

type Repo struct {
	RepoFullName string `json:"repoFullName"`
	RepositoryID string `json:"repositoryId,omitempty"`
	Branch       string `json:"branch,omitempty"`
}

type Project struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   *string `json:"description,omitempty"`
	DefaultBranch string  `json:"defaultBranch,omitempty"`
	PreviewPort   int     `json:"previewPort,omitempty"`
	Repos         []Repo  `json:"repos,omitempty"`
}

type PullRequest struct {
	URL    string `json:"url,omitempty"`
	Number int    `json:"number,omitempty"`
	State  string `json:"state,omitempty"`
}

type Thread struct {
	ID           string        `json:"id"`
	ProjectID    string        `json:"projectId"`
	Title        string        `json:"title,omitempty"`
	Status       string        `json:"status"`
	WaitingOn    []any         `json:"waitingOn,omitempty"`
	BlockedOn    []any         `json:"blockedOn,omitempty"`
	PullRequests []PullRequest `json:"pullRequests,omitempty"`
	CreatedAt    string        `json:"createdAt,omitempty"`
	UpdatedAt    string        `json:"updatedAt,omitempty"`
}

type RunSummary struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Message struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type CreateThreadRequest struct {
	ProjectID         string `json:"projectId"`
	Prompt            string `json:"prompt"`
	Title             string `json:"title,omitempty"`
	Model             string `json:"model,omitempty"`
	Speed             string `json:"speed,omitempty"`
	AutoFix           bool   `json:"autoFix"`
	AutoMerge         bool   `json:"autoMerge"`
	ClientOperationID string `json:"clientOperationId"`
	Repos             []Repo `json:"repos,omitempty"`
}

type CreateThreadResult struct {
	Thread  Thread      `json:"thread"`
	Created bool        `json:"created,omitempty"`
	Message *Message    `json:"message,omitempty"`
	Run     *RunSummary `json:"run,omitempty"`
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	} else if httpClient.Timeout <= 0 {
		clone := *httpClient
		clone.Timeout = defaultTimeout
		httpClient = &clone
	}
	return &Client{baseURL: baseURL, apiKey: strings.TrimSpace(apiKey), httpClient: httpClient}
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, ResponseMeta, error) {
	var envelope struct {
		Projects []Project `json:"projects"`
	}
	meta, err := c.do(ctx, http.MethodGet, "/api/projects", nil, &envelope)
	return envelope.Projects, meta, err
}

func (c *Client) ListThreads(ctx context.Context, projectID string) ([]Thread, ResponseMeta, error) {
	path := "/api/threads"
	if strings.TrimSpace(projectID) != "" {
		path += "?projectId=" + url.QueryEscape(strings.TrimSpace(projectID))
	}
	var envelope struct {
		Threads []Thread `json:"threads"`
	}
	meta, err := c.do(ctx, http.MethodGet, path, nil, &envelope)
	return envelope.Threads, meta, err
}

func (c *Client) CreateThread(ctx context.Context, request CreateThreadRequest) (CreateThreadResult, ResponseMeta, error) {
	var result CreateThreadResult
	meta, err := c.do(ctx, http.MethodPost, "/api/threads", request, &result)
	return result, meta, err
}

func (c *Client) GetThread(ctx context.Context, threadID string) (Thread, ResponseMeta, error) {
	var envelope struct {
		Thread Thread `json:"thread"`
	}
	meta, err := c.do(ctx, http.MethodGet, "/api/threads/"+url.PathEscape(threadID), nil, &envelope)
	return envelope.Thread, meta, err
}

func (c *Client) ListMessages(ctx context.Context, threadID string) ([]Message, ResponseMeta, error) {
	var envelope struct {
		Messages []Message `json:"messages"`
	}
	meta, err := c.do(ctx, http.MethodGet, "/api/threads/"+url.PathEscape(threadID)+"/messages", nil, &envelope)
	return envelope.Messages, meta, err
}

func (c *Client) do(ctx context.Context, method, path string, body any, target any) (ResponseMeta, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return ResponseMeta{}, fmt.Errorf("encode hoplite request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return ResponseMeta{}, fmt.Errorf("build hoplite request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return ResponseMeta{}, fmt.Errorf("hoplite request timed out: %w", err)
		}
		return ResponseMeta{}, fmt.Errorf("hoplite request failed: %w", err)
	}
	defer resp.Body.Close()
	meta := responseMeta(resp)
	limit := int64(maxResponseBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limit = maxErrorBody
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return meta, fmt.Errorf("read hoplite response: %w", err)
	}
	if int64(len(payload)) > limit {
		return meta, fmt.Errorf("hoplite response exceeds %d bytes", limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return meta, parseUpstreamError(meta, payload)
	}
	var statusEnvelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &statusEnvelope); err != nil {
		return meta, fmt.Errorf("decode hoplite response: %w", err)
	}
	if statusEnvelope.OK != nil && !*statusEnvelope.OK {
		code := strings.TrimSpace(statusEnvelope.Error)
		if code == "" {
			code = "semantic_failure"
		}
		return meta, &UpstreamError{StatusCode: http.StatusBadGateway, Code: code, RequestID: meta.RequestID, RetryAfter: meta.RetryAfter}
	}
	if target == nil || len(bytes.TrimSpace(payload)) == 0 {
		return meta, nil
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return meta, fmt.Errorf("decode hoplite response: %w", err)
	}
	return meta, nil
}

func responseMeta(resp *http.Response) ResponseMeta {
	return ResponseMeta{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("X-Request-Id"),
		RetryAfter: resp.Header.Get("Retry-After"),
		RateLimit:  resp.Header.Get("RateLimit"),
		Policy:     resp.Header.Get("RateLimit-Policy"),
	}
}

func parseUpstreamError(meta ResponseMeta, payload []byte) error {
	var decoded struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(payload, &decoded)
	code := strings.TrimSpace(decoded.Error)
	if code == "" {
		code = http.StatusText(meta.StatusCode)
	}
	return &UpstreamError{
		StatusCode: meta.StatusCode,
		Code:       code,
		RequestID:  meta.RequestID,
		RetryAfter: meta.RetryAfter,
	}
}

package hoplite

import (
	"context"
	"net/http"
	"net/url"
)

// AppendMessageRequest represents the request body for appending a message to an existing thread.
type AppendMessageRequest struct {
	Content     string          `json:"content"`
	Metadata    MessageMetadata `json:"metadata,omitempty"`
	Attachments []Attachment    `json:"attachments,omitempty"`
}

// MessageMetadata contains optional metadata for a message append operation.
type MessageMetadata struct {
	Model string `json:"model,omitempty"` // override model for this message run
}

// Attachment represents a previously uploaded artifact to attach to a message.
type Attachment struct {
	ID string `json:"id"`
}

// AppendMessageResult wraps the response from the AppendThreadMessage endpoint.
type AppendMessageResult struct {
	Message    *Message    `json:"message,omitempty"`
	Queued     bool        `json:"queued,omitempty"`
	IDempotent bool        `json:"idempotent,omitempty"`
	Run        *RunSummary `json:"run,omitempty"`
}

// AppendMessage sends a new user message to an existing thread and queues its agent run.
// The thread must already exist; this is different from CreateThread which starts a fresh workspace.
// Returns the accepted message object and queued flag. The actual result requires polling GetThread.
func (c *Client) AppendMessage(ctx context.Context, threadID string, req AppendMessageRequest) (*AppendMessageResult, ResponseMeta, error) {
	var result AppendMessageResult
	path := "/api/threads/" + url.PathEscape(threadID) + "/messages"
	meta, err := c.do(ctx, http.MethodPost, path, req, &result)
	if err != nil {
		return nil, meta, err
	}
	return &result, meta, nil
}

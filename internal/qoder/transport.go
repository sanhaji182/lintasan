package qoder

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// StartChatStream opens a chat completion stream and returns the live response.
//
// It is a real method rather than a test helper because the gateway integration
// needs exactly this: the credential-to-session resolution, the request signing
// and the model-header pairing are all protocol-required and easy to get subtly
// wrong, so they belong in one place instead of being repeated by each caller.
//
// The caller owns the returned body and must close it. A non-2xx status is
// returned as an *UpstreamError so the caller can classify it (dead credential,
// throttled model, moderation) rather than pattern-matching on a status code.
func (m *SessionManager) StartChatStream(ctx context.Context, credential, model string, body []byte, userType string) (*http.Response, error) {
	sess, err := m.session(ctx, credential)
	if err != nil {
		return nil, err
	}

	url := m.endpoints.ChatStreamURL
	if userType == "" {
		userType = sess.Identity.UserType
	}

	// The chat endpoint is an Encode=1 endpoint: the request body must be wrapped
	// in the cosy base64 envelope, exactly as the job-token handshake is. Sending
	// the plain JSON fails upstream with "Illegal base64 character 20" — the space
	// in the JSON — deep inside its decoder, which reads as a server fault rather
	// than a client mistake.
	//
	// The envelope is also what the signature covers, so encoding has to happen
	// before signing, not after.
	encoded, err := EncodeBase64(body)
	if err != nil {
		return nil, err
	}

	// The model key is sent in the signed body and mirrored in two headers.
	// Upstream validates the pairing, so the source must match what the body
	// declares rather than being a free choice.
	headers, err := m.BuildAPIHeaders(sess, []byte(encoded), pathOf(url), "text/event-stream", map[string]string{
		"x-model-key":    model,
		"x-model-source": "system",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("qoder: build chat request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: chat request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := readAllLimited(resp.Body, 64*1024)
		return nil, &UpstreamError{
			Status:  resp.StatusCode,
			Message: strings.TrimSpace(string(raw)),
		}
	}
	return resp, nil
}

// doStreamRequest issues a signed POST and returns the raw response, for callers
// that manage the session themselves.
func doStreamRequest(ctx context.Context, url string, headers map[string]string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return http.DefaultClient.Do(req)
}

package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// Model is one model advertised by an account.
//
// The wire names here are snake_case because that is what upstream emits. An
// earlier iteration of this package guessed camelCase (displayName, isDefault)
// and silently produced zero models — the mismatch is worth remembering.
type Model struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Format      string `json:"format"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enable"`
	IsDefault   bool   `json:"is_default"`
	IsReasoning bool   `json:"is_reasoning"`
	IsVision    bool   `json:"is_vl"`
	Sensitive   bool   `json:"is_sensitive"`
	// PriceFactor is a relative cost multiplier. It is a float on the wire, not
	// a string: an earlier iteration typed it as a string and failed to decode
	// the entire model list, which took out discovery for every model at once.
	// One wrong field type in this struct is a total-failure mode, so the types
	// here are pinned by a test against captured upstream JSON.
	PriceFactor     float64 `json:"price_factor"`
	MaxInputTokens  int     `json:"max_input_tokens"`
	MaxOutputTokens int     `json:"max_output_tokens"`
}

// Name returns the model's display name, falling back to its key.
func (m Model) Name() string {
	if strings.TrimSpace(m.DisplayName) != "" {
		return m.DisplayName
	}
	return m.Key
}

// modelListEnvelope is the top-level shape of the model list response.
//
// Upstream groups models by the surface that can use them. Lintasan proxies
// chat, so the `assistant` group is the one that matters; `chat`, `inline` and
// `experts` are other surfaces and are intentionally not treated as equivalents.
type modelListEnvelope struct {
	Assistant []Model `json:"assistant"`
	Chat      []Model `json:"chat"`
	Inline    []Model `json:"inline"`
	Experts   []Model `json:"experts"`
}

// parseModelList extracts the chat-capable models from a model list response.
//
// It prefers the `assistant` group. If that group is absent it falls back to
// `chat`, because an account with no assistant-surface models should still be
// usable rather than appearing empty. Only enabled models are returned: a
// disabled model would be rejected at request time, so advertising it would
// produce failures that look like routing bugs.
func parseModelList(raw []byte) ([]Model, error) {
	var env modelListEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("qoder: decode model list: %w", err)
	}

	group := env.Assistant
	if len(group) == 0 {
		group = env.Chat
	}

	out := make([]Model, 0, len(group))
	for _, m := range group {
		if strings.TrimSpace(m.Key) == "" {
			continue
		}
		if !m.Enabled {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ListModels fetches the models available to one credential.
//
// This is a per-credential operation by necessity, not convenience: accounts on
// the same upstream carry different entitlements, so a single account's list
// cannot be assumed true for another. Callers that pool credentials should
// either call this per credential or route on the narrowest common set.
func (m *SessionManager) ListModels(ctx context.Context, credential string) ([]Model, error) {
	sess, err := m.session(ctx, credential)
	if err != nil {
		return nil, err
	}

	url := m.endpoints.ModelListURL
	// A GET carries no body, so the signature covers an empty body.
	headers, err := m.BuildAPIHeaders(sess, nil, pathOf(url), "application/json", nil)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("qoder: build model list request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: model list request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("qoder: read model list: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
	}
	if e := parseEnvelopeError(raw); e != nil {
		return nil, e
	}

	models, err := parseModelList(raw)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		// Distinguish "authenticated but entitled to nothing" from a parse
		// failure, since the two need different responses from an operator.
		return nil, fmt.Errorf("qoder: account advertises no enabled chat models")
	}
	return models, nil
}

// ModelKeys returns just the model keys, for logging and comparison.
func ModelKeys(models []Model) []string {
	keys := make([]string, 0, len(models))
	for _, m := range models {
		keys = append(keys, m.Key)
	}
	return keys
}

// inferSurface guesses which upstream agent surface a model belongs to. The
// protocol requires naming a surface (agent id) alongside the model, and the
// family is the only signal available for it.
func inferSurface(model string) string {
	low := strings.ToLower(model)
	switch {
	case strings.Contains(low, "gemini"):
		return "gemini"
	case strings.Contains(low, "claude"), strings.Contains(low, "sonnet"),
		strings.Contains(low, "opus"), strings.Contains(low, "haiku"):
		return "claude"
	default:
		return "codex"
	}
}

// truncate returns at most n runes of s, used for the business-name field which
// upstream populates with a short prompt preview.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// readJSONBody is a small helper for reading a bounded JSON body.
func readJSONBody(r io.Reader) (map[string]any, error) {
	raw, err := readAllLimited(r, 1<<22)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

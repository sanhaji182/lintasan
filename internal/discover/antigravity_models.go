package discover

// antigravity_models.go — live model discovery for Antigravity connections.
//
// WHY THIS EXISTS
//
// Discovery used to return a hardcoded catalogue (AntigravityModels) for every
// Antigravity connection. That list went stale against the real backend: of 25
// advertised ids, 12 were no longer served (gemini-3.1-pro-high,
// gemini-3.7-flash-*, claude-sonnet-5-5*, claude-opus-5-5*, gemini-pro-agent).
// Every request to them answered 404 "Requested entity was not found", which
// reads like a broken proxy rather than a stale catalogue.
//
// Cloud Code exposes the real entitlement at v1internal:fetchAvailableModels,
// and the answer is per-account: it depends on the tier the signed-in Google
// account landed on. So discovery has to ask with that account's token, exactly
// like the Qoder path does, instead of asserting a vendor-wide list.
//
// The static list stays as the fallback for a deployment with no usable session:
// it is what can honestly be claimed without asking upstream, and reporting it
// is better than reporting zero models — but it must never outrank a live answer.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/antigravity"
)

// antigravityAvailableModelsURL is the entitlement endpoint the Antigravity IDE
// itself calls. It lives on the generation host, not cloudcode-pa.
const antigravityAvailableModelsPath = "/v1internal:fetchAvailableModels"

// fetchAntigravityModels asks Cloud Code which models this account may use.
//
// It returns an error (never a partial list) whenever the live answer cannot be
// trusted, so the caller falls back to the static catalogue instead of
// overwriting a good catalogue with an empty or half-parsed one.
func (d *Discoverer) fetchAntigravityModels(conn map[string]any) ([]ModelInfo, error) {
	// The connection row carries an EMPTY api_key for an OAuth-wired provider —
	// the live token lives in oauth_sessions. Reading only conn["api_key"] made
	// this function always fail on real connections and silently fall back to
	// the stale static catalogue, which is exactly the bug being fixed. Use the
	// same resolver the generic /models path uses.
	apiKey, _ := conn["api_key"].(string)
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" && d.oauthCredential != nil {
		if cred, err := d.oauthCredential("antigravity"); err == nil && cred != nil {
			apiKey = strings.TrimSpace(cred.Token)
		}
	}
	if apiKey == "" {
		return nil, fmt.Errorf("antigravity: connection has no OAuth access token")
	}

	baseURL, _ := conn["base_url"].(string)
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = antigravity.AntigravityIDEBaseURL
	}

	// The project id is part of the entitlement question. A blank project is
	// accepted by the endpoint and scopes the answer to the free tier, which is
	// the tier most connections land on.
	payload, err := json.Marshal(map[string]any{"project": ""})
	if err != nil {
		return nil, fmt.Errorf("antigravity: encode request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+antigravityAvailableModelsPath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("antigravity: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", antigravity.AntigravityIDEUserAgent)
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("antigravity: fetch available models: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("antigravity: fetch available models: HTTP %d: %s", resp.StatusCode, truncateForError(body))
	}

	// Shape: {"models": {"<wire id>": {...}, ...}}
	var parsed struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("antigravity: decode available models: %w", err)
	}
	if len(parsed.Models) == 0 {
		return nil, fmt.Errorf("antigravity: upstream returned no models")
	}

	// Upstream also advertises internal ids the chat path cannot serve. Keep the
	// ones the executor's own map knows about, plus anything clearly chat-shaped,
	// so the catalogue does not fill with tab_/chat_ internals that 404 later.
	out := make([]ModelInfo, 0, len(parsed.Models))
	for id := range parsed.Models {
		if !isServableAntigravityModel(id) {
			continue
		}
		out = append(out, ModelInfo{
			ID:              id,
			Name:            antigravityDisplayName(id),
			OwnedBy:         "google-antigravity",
			MaxInputTokens:  antigravityContextWindow(id),
			MaxOutputTokens: 8192,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("antigravity: no servable models in upstream answer")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// isServableAntigravityModel filters out ids that exist in the entitlement
// answer but are internal to the IDE (editor tabs, chat scaffolding).
func isServableAntigravityModel(id string) bool {
	lower := strings.ToLower(id)
	for _, prefix := range []string{"tab_", "chat_"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "lite_preview") {
		// Image/tab specialists are not chat-completion targets on this path.
		return false
	}
	return true
}

// antigravityContextWindow reports the input window Google documents per family.
func antigravityContextWindow(id string) int {
	if strings.HasPrefix(strings.ToLower(id), "claude") {
		return 200000
	}
	if strings.HasPrefix(strings.ToLower(id), "gpt-oss") {
		return 128000
	}
	return 1048576
}

// antigravityDisplayName humanises a wire id for the dashboard.
func antigravityDisplayName(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func truncateForError(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	providerModelsTTL  = 5 * time.Minute
	providerModelsMax  = 500
	providerModelsBody = 2 << 20
)

type providerModelsCacheEntry struct {
	models    []string
	fetchedAt time.Time
}

var providerModelsCache = struct {
	sync.Mutex
	entries map[string]providerModelsCacheEntry
}{entries: make(map[string]providerModelsCacheEntry)}

// ProviderModelsError carries the upstream status so the HTTP layer can tell
// "connect with an API key first" apart from a real outage.
type ProviderModelsError struct {
	Status  int
	Message string
}

func (e *ProviderModelsError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("models endpoint returned HTTP %d: %s", e.Status, e.Message)
	}
	return e.Message
}

// FetchFreeProviderModels reads a provider's /models without a stored
// connection. Public catalogues (Pollinations, OpenRouter) need no key; key
// providers return 401/403 and the caller should ask the user to connect first.
func FetchFreeProviderModels(ctx context.Context, p FreeProvider) ([]string, error) {
	if p.NoModelFetch {
		return nil, &ProviderModelsError{Message: p.Name + " does not expose /models on its OpenAI-compatible endpoint"}
	}
	cacheKey := strings.ToLower(p.Name)
	providerModelsCache.Lock()
	if entry, ok := providerModelsCache.entries[cacheKey]; ok && time.Since(entry.fetchedAt) < providerModelsTTL {
		models := append([]string(nil), entry.models...)
		providerModelsCache.Unlock()
		return models, nil
	}
	providerModelsCache.Unlock()

	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	path := strings.TrimSpace(p.ModelsPath)
	if path == "" {
		if strings.HasSuffix(strings.ToLower(base), "/v1") {
			path = "/models"
		} else {
			path = "/v1/models"
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Lintasan/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &ProviderModelsError{Message: err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, providerModelsBody))
	if err != nil {
		return nil, &ProviderModelsError{Message: "read models response: " + err.Error()}
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}
		return nil, &ProviderModelsError{Status: resp.StatusCode, Message: msg}
	}

	models, err := parseProviderModels(body)
	if err != nil {
		return nil, &ProviderModelsError{Message: err.Error()}
	}
	if p.FreeOnly {
		free := models[:0]
		for _, id := range models {
			lower := strings.ToLower(id)
			if strings.HasSuffix(lower, ":free") || strings.Contains(lower, "free") {
				free = append(free, id)
			}
		}
		models = free
	}
	if len(models) == 0 {
		return nil, &ProviderModelsError{Message: "no models returned" + freeOnlyHint(p)}
	}
	providerModelsCache.Lock()
	providerModelsCache.entries[cacheKey] = providerModelsCacheEntry{
		models:    append([]string(nil), models...),
		fetchedAt: time.Now(),
	}
	providerModelsCache.Unlock()
	return models, nil
}

func freeOnlyHint(p FreeProvider) string {
	if p.FreeOnly {
		return " (free models only)"
	}
	return ""
}

func parseProviderModels(body []byte) ([]string, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	var items []any
	switch v := raw.(type) {
	case []any:
		items = v
	case map[string]any:
		if data, ok := v["data"].([]any); ok {
			items = data
		}
	}
	seen := make(map[string]bool, len(items))
	models := make([]string, 0, len(items))
	for _, item := range items {
		obj, _ := item.(map[string]any)
		id, _ := obj["id"].(string)
		if id == "" {
			id, _ = obj["name"].(string)
		}
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		models = append(models, id)
	}
	if len(models) > providerModelsMax {
		models = models[:providerModelsMax]
	}
	sort.Strings(models)
	return models, nil
}

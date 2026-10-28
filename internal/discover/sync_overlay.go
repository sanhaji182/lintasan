package discover

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultSyncURL = "http://127.0.0.1:8080/api/freemodels/catalog"
	syncOKTTL      = 5 * time.Minute
	syncErrTTL     = 30 * time.Second
)

type syncRef struct {
	source string // "fcm" or "freellm"
	key    string
}

// syncRefs maps Lintasan provider prefixes to the upstream sync keys. Only
// providers with a known OpenAI-compatible connection are overlaid; new
// providers from the source feeds remain candidates for manual review.
var syncRefs = map[string][]syncRef{
	"pol/":     {{"fcm", "pollinations"}},
	"nvidia/":  {{"fcm", "nvidia"}, {"freellm", "nvidia"}},
	"groq/":    {{"fcm", "groq"}, {"freellm", "groq"}},
	"mistral/": {{"fcm", "mistral"}, {"freellm", "mistral"}},
	"samba/":   {{"fcm", "sambanova"}},
	"google/":  {{"fcm", "googleai"}, {"freellm", "google"}},
	"zai/":     {{"freellm", "zhipu"}},
	"or/":      {{"fcm", "openrouter"}, {"freellm", "openrouter"}},
}

type syncModel struct {
	ID string `json:"id"`
}

type syncProvider struct {
	Key    string      `json:"key"`
	Quota  string      `json:"quota"`
	Models []syncModel `json:"models"`
}

type syncCatalog struct {
	LastRunAt string `json:"last_run_at"`
	FCM       struct {
		Providers []syncProvider `json:"providers"`
	} `json:"fcm"`
	FreeLLM struct {
		Providers []syncProvider `json:"providers"`
	} `json:"freellm"`
}

type syncCacheState struct {
	mu        sync.Mutex
	loadedAt  time.Time
	ok        bool
	catalog   syncCatalog
	loadedURL string
}

var syncCache syncCacheState

func syncURL() string {
	if v := strings.TrimSpace(os.Getenv("LINTASAN_FREE_CATALOG_URL")); v != "" {
		return v
	}
	return defaultSyncURL
}

func syncEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LINTASAN_FREE_CATALOG_SYNC"))) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

func loadSyncCatalog(ctx context.Context) (syncCatalog, bool) {
	url := syncURL()
	if url == "" || !syncEnabled() {
		return syncCatalog{}, false
	}
	syncCache.mu.Lock()
	if !syncCache.loadedAt.IsZero() && syncCache.loadedURL == url {
		ttl := syncErrTTL
		if syncCache.ok {
			ttl = syncOKTTL
		}
		if time.Since(syncCache.loadedAt) < ttl {
			catalog, ok := syncCache.catalog, syncCache.ok
			syncCache.mu.Unlock()
			return catalog, ok
		}
	}
	syncCache.mu.Unlock()

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return syncCatalog{}, false
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		rememberSyncFailure(url)
		return syncCatalog{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		rememberSyncFailure(url)
		return syncCatalog{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		rememberSyncFailure(url)
		return syncCatalog{}, false
	}
	var catalog syncCatalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		rememberSyncFailure(url)
		return syncCatalog{}, false
	}
	syncCache.mu.Lock()
	syncCache.loadedAt = time.Now()
	syncCache.ok = true
	syncCache.catalog = catalog
	syncCache.loadedURL = url
	syncCache.mu.Unlock()
	return catalog, true
}

func rememberSyncFailure(url string) {
	syncCache.mu.Lock()
	syncCache.loadedAt = time.Now()
	syncCache.ok = false
	syncCache.catalog = syncCatalog{}
	syncCache.loadedURL = url
	syncCache.mu.Unlock()
}

// ApplySyncOverlay replaces the static model lists of known providers with the
// latest Catalog Sync state. It never adds providers or changes connection
// metadata; if the sync service is unavailable the static registry is returned.
func ApplySyncOverlay(ctx context.Context, providers []FreeProvider) []FreeProvider {
	catalog, ok := loadSyncCatalog(ctx)
	if !ok {
		return providers
	}
	fcm := make(map[string]syncProvider, len(catalog.FCM.Providers))
	for _, provider := range catalog.FCM.Providers {
		fcm[provider.Key] = provider
	}
	freellm := make(map[string]syncProvider, len(catalog.FreeLLM.Providers))
	for _, provider := range catalog.FreeLLM.Providers {
		freellm[provider.Key] = provider
	}

	out := make([]FreeProvider, len(providers))
	copy(out, providers)
	updatedAt := catalog.LastRunAt
	for i := range out {
		if !out[i].Enabled {
			continue
		}
		refs := syncRefs[out[i].Prefix]
		if len(refs) == 0 {
			continue
		}
		for _, ref := range refs {
			var source syncProvider
			var label string
			switch ref.source {
			case "fcm":
				source = fcm[ref.key]
				label = "free-coding-models"
			case "freellm":
				source = freellm[ref.key]
				label = "FreeLLMAPI"
			}
			models := overlayModels(out[i].Prefix, source.Models)
			if len(models) == 0 {
				continue
			}
			out[i].Models = models
			if ref.source == "fcm" && strings.TrimSpace(source.Quota) != "" && out[i].Prefix != "pol/" {
				out[i].QuotaInfo = source.Quota
			}
			out[i].SyncSource = "Catalog Sync · " + label
			out[i].SyncUpdatedAt = updatedAt
			break
		}
	}
	return out
}

func overlayModels(prefix string, models []syncModel) []string {
	seen := make(map[string]bool, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" || seen[id] {
			continue
		}
		if prefix == "or/" && !strings.HasSuffix(strings.ToLower(id), ":free") {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

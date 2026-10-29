package discover

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const syncFixture = `{
  "last_run_at": "2026-10-29T05:19:25+07:00",
  "fcm": {
    "providers": [
      {"key":"nvidia","quota":"Free 1000 req/month","models":[{"id":"nvidia/a"},{"id":"nvidia/b"}]},
      {"key":"openrouter","quota":"Free :free","models":[{"id":"vendor/paid"},{"id":"vendor/free:free"}]},
      {"key":"googleai","quota":"Free tier","models":[{"id":"gemini-sync"}]},
      {"key":"kilo","name":"Kilo","quota":"Free · no key needed","url":"https://api.kilo.ai/api/gateway/chat/completions","models":[{"id":"kilo/free"}]}
    ]
  },
  "freellm": {
    "providers": [
      {"key":"zhipu","models":[{"id":"glm-4.7-flash"}]}
    ]
  }
}`

func resetSyncCache() {
	syncCache = syncCacheState{}
}

func TestApplySyncOverlayReplacesKnownProviders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(syncFixture))
	}))
	defer srv.Close()
	t.Setenv("LINTASAN_FREE_CATALOG_URL", srv.URL)
	t.Setenv("LINTASAN_FREE_CATALOG_SYNC", "1")
	resetSyncCache()

	providers := ApplySyncOverlay(context.Background(), GetFreeProviders())
	byPrefix := map[string]FreeProvider{}
	for _, p := range providers {
		byPrefix[p.Prefix] = p
	}

	nvidia := byPrefix["nvidia/"]
	if len(nvidia.Models) != 2 || nvidia.Models[0] != "nvidia/a" || nvidia.SyncSource == "" {
		t.Fatalf("nvidia overlay = %+v", nvidia)
	}
	if nvidia.QuotaInfo != "Free 1000 req/month" {
		t.Fatalf("nvidia quota = %q", nvidia.QuotaInfo)
	}
	router := byPrefix["or/"]
	if len(router.Models) != 1 || router.Models[0] != "vendor/free:free" {
		t.Fatalf("openrouter should keep only :free models, got %v", router.Models)
	}
	zai := byPrefix["zai/"]
	if len(zai.Models) != 1 || zai.Models[0] != "glm-4.7-flash" {
		t.Fatalf("zai overlay = %v", zai.Models)
	}
	if zai.SyncSource != "Catalog Sync · FreeLLMAPI" {
		t.Fatalf("zai source = %q", zai.SyncSource)
	}
	kilo := byPrefix["kilo/"]
	if kilo.AuthType != "none" || kilo.BaseURL != "https://api.kilo.ai/api/gateway" || len(kilo.Models) != 1 {
		t.Fatalf("kilo auto-discovered provider = %+v", kilo)
	}
	if !kilo.NoModelFetch || kilo.SyncSource == "" {
		t.Fatalf("kilo should be a sync-only card, got %+v", kilo)
	}
}

func TestApplySyncOverlayFallsBackWhenSyncUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	t.Setenv("LINTASAN_FREE_CATALOG_URL", srv.URL)
	t.Setenv("LINTASAN_FREE_CATALOG_SYNC", "1")
	resetSyncCache()

	static := GetFreeProviders()
	providers := ApplySyncOverlay(context.Background(), static)
	for _, p := range providers {
		if p.Prefix == "nvidia/" && len(p.Models) == 0 {
			t.Fatal("static NVIDIA models must survive a sync outage")
		}
		if p.SyncSource != "" {
			t.Fatalf("unexpected sync metadata on %s during outage", p.Name)
		}
	}
}

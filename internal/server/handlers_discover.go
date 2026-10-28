package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/discover"
)

// handleDiscoverFreeProviders handles GET /api/discover/free-providers
func (s *Server) handleDiscoverFreeProviders(w http.ResponseWriter, r *http.Request) {
	providers := discover.ApplySyncOverlay(r.Context(), discover.GetFreeProviders())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"providers": providers,
		"total":     len(providers),
	})
}

// handleDiscoverProviderModels handles GET /api/discover/free-providers/{id}/models.
// It previews a provider's live catalogue so the dashboard can refresh free
// model IDs without creating a connection first.
func (s *Server) handleDiscoverProviderModels(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimSpace(r.PathValue("id")), "/")
	provider := discover.GetFreeProviderByPrefix(id + "/")
	if provider == nil {
		provider = discover.GetFreeProviderByName(id)
	}
	if provider == nil || !provider.Enabled {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "unknown or disabled free provider"})
		return
	}

	models, err := discover.FetchFreeProviderModels(r.Context(), *provider)
	if err != nil {
		var perr *discover.ProviderModelsError
		if errors.As(err, &perr) && (perr.Status == http.StatusUnauthorized || perr.Status == http.StatusForbidden) {
			writeJSONStatus(w, http.StatusConflict, map[string]any{
				"error":           "api_key_required",
				"needs_key":       true,
				"provider":        provider.Name,
				"fallback_models": provider.Models,
				"upstream_status": perr.Status,
				"hint":            "Connect this provider with an API key first, then refresh its models.",
			})
			return
		}
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error":           "failed to fetch models",
			"detail":          err.Error(),
			"fallback_models": provider.Models,
		})
		return
	}

	writeJSON(w, map[string]any{
		"provider":  provider.Name,
		"models":    models,
		"total":     len(models),
		"free_only": provider.FreeOnly,
		"source":    "live",
	})
}

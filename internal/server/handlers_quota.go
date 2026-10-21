package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/antigravity"
)

// handleConnectionUsage returns live quota/usage for a single connection (9router parity).
// GET /api/usage/{connection_id}
func (s *Server) handleConnectionUsage(w http.ResponseWriter, r *http.Request) {
	connID := strings.TrimSpace(r.PathValue("connection_id"))
	if connID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "connection_id is required"})
		return
	}

	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("refresh") == "1"

	var name, format, oauthProv, apiKey string
	var isActive int
	err := s.db.Conn().QueryRow(`
		SELECT name, format, COALESCE(oauth_provider, ''), COALESCE(api_key, ''), is_active
		FROM connections WHERE id = ?`, connID).Scan(&name, &format, &oauthProv, &apiKey, &isActive)
	if err != nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "Connection not found"})
		return
	}

	formatLower := strings.ToLower(format)
	oauthLower := strings.ToLower(oauthProv)

	// Case 1: Antigravity
	if oauthLower == "antigravity" || formatLower == "antigravity" {
		token := apiKey
		if s.oauthMgr != nil {
			tok, errTok := s.oauthMgr.GetActiveToken("antigravity")
			if errTok == nil && tok != "" {
				token = tok
			}
		}

		if token == "" {
			writeJSONStatus(w, http.StatusUnauthorized, map[string]any{
				"error": "Antigravity access token expired or not found",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		client := &http.Client{Timeout: 12 * time.Second}
		res, err := antigravity.FetchQuota(ctx, client, token, force)
		if err != nil {
			writeJSONStatus(w, http.StatusBadGateway, map[string]any{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, map[string]any{
			"connection_id": connID,
			"name":          name,
			"provider":      "antigravity",
			"plan":          res.Plan,
			"quotas":        res.Quotas,
			"groups":        res.Groups,
			"fetched_at":    res.FetchedAt,
		})
		return
	}

	// Case 2: Qoder
	if formatLower == "qoder" {
		writeJSON(w, map[string]any{
			"connection_id": connID,
			"name":          name,
			"provider":      "qoder",
			"message":       "Qoder quota is tracked via /api/qoder/quota",
		})
		return
	}

	// Case 3: Other
	writeJSON(w, map[string]any{
		"connection_id": connID,
		"name":          name,
		"provider":      format,
		"message":       "Quota tracking not available for this connection",
	})
}

// handleQuotaProviders returns list of all connections supporting live quota tracking.
// GET /api/quota/providers
func (s *Server) handleQuotaProviders(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("refresh") == "1"

	rows, err := s.db.Conn().Query(`
		SELECT id, name, format, COALESCE(oauth_provider, ''), COALESCE(api_key, ''), is_active
		FROM connections
		WHERE is_active = 1 AND (
			LOWER(COALESCE(oauth_provider, '')) = 'antigravity' OR
			LOWER(format) IN ('antigravity')
		)
		ORDER BY priority DESC, name ASC`)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	var providers []map[string]any
	client := &http.Client{Timeout: 12 * time.Second}

	for rows.Next() {
		var id, name, format, oauthProv, apiKey string
		var isActive int
		if err := rows.Scan(&id, &name, &format, &oauthProv, &apiKey, &isActive); err != nil {
			continue
		}

		if strings.EqualFold(oauthProv, "antigravity") || strings.EqualFold(format, "antigravity") {
			token := apiKey
			if s.oauthMgr != nil {
				tok, errTok := s.oauthMgr.GetActiveToken("antigravity")
				if errTok == nil && tok != "" {
					token = tok
				}
			}

			if token == "" {
				providers = append(providers, map[string]any{
					"connection_id": id,
					"name":          name,
					"provider":      "antigravity",
					"error":         "OAuth token unavailable",
				})
				continue
			}

			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			res, err := antigravity.FetchQuota(ctx, client, token, force)
			cancel()

			if err != nil {
				providers = append(providers, map[string]any{
					"connection_id": id,
					"name":          name,
					"provider":      "antigravity",
					"error":         err.Error(),
				})
			} else {
				providers = append(providers, map[string]any{
					"connection_id": id,
					"name":          name,
					"provider":      "antigravity",
					"plan":          res.Plan,
					"quotas":        res.Quotas,
					"groups":        res.Groups,
					"fetched_at":    res.FetchedAt,
				})
			}
		}
	}

	writeJSON(w, map[string]any{
		"success":   true,
		"providers": providers,
	})
}

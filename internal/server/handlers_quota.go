package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/antigravity"
)

// buildConnectionQuota compiles normalized quota/usage data for any supported connection.
func (s *Server) buildConnectionQuota(ctx context.Context, id, name, format, oauthProv, apiKey, baseURL string, force bool) map[string]any {
	formatLower := strings.ToLower(format)
	oauthLower := strings.ToLower(oauthProv)
	baseLower := strings.ToLower(baseURL)

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
			return map[string]any{
				"connection_id": id,
				"name":          name,
				"provider":      "antigravity",
				"error":         "Antigravity access token expired or not configured",
			}
		}

		client := &http.Client{Timeout: 12 * time.Second}
		res, err := antigravity.FetchQuota(ctx, client, token, force)
		if err != nil {
			return map[string]any{
				"connection_id": id,
				"name":          name,
				"provider":      "antigravity",
				"error":         err.Error(),
			}
		}

		return map[string]any{
			"connection_id": id,
			"name":          name,
			"provider":      "antigravity",
			"plan":          res.Plan,
			"quotas":        res.Quotas,
			"groups":        res.Groups,
			"fetched_at":    res.FetchedAt,
		}
	}

	// Case 2: CommandCode
	if formatLower == "commandcode" || strings.Contains(baseLower, "commandcode") {
		if apiKey == "" {
			return map[string]any{
				"connection_id": id,
				"name":          name,
				"provider":      "commandcode",
				"error":         "No API key provided for Command Code",
			}
		}

		info := fetchCommandCodeBalance(apiKey)
		if info.Error != "" {
			return map[string]any{
				"connection_id": id,
				"name":          name,
				"provider":      "commandcode",
				"error":         info.Error,
			}
		}

		quotas := make(map[string]any)
		var limits []map[string]any

		for _, win := range info.RateWindows {
			remPct := 100.0
			if win.Cap > 0 {
				remPct = (win.Cap - win.Used) / win.Cap * 100.0
				if remPct < 0 {
					remPct = 0
				}
			}
			dispName := fmt.Sprintf("%s Limit", strings.Title(win.Name))
			limitItem := map[string]any{
				"displayName":         dispName,
				"window":              win.Name,
				"used":                win.Used,
				"total":               win.Cap,
				"remainingPercentage": remPct,
				"description":         fmt.Sprintf("Window: %s (used %.1f of %.0f)", win.Name, win.Used, win.Cap),
			}
			if info.BillingReset != "" {
				limitItem["resetAt"] = info.BillingReset
			}
			limits = append(limits, limitItem)
			quotas[win.Name] = limitItem
		}

		if info.Balance != "" {
			limits = append(limits, map[string]any{
				"displayName":         fmt.Sprintf("Credits Balance (%s)", info.Currency),
				"window":              "credits",
				"used":                0,
				"total":               0,
				"remainingPercentage": 100.0,
				"description":         fmt.Sprintf("Balance: %s (Total Used: %s) · %s", info.Balance, info.TotalUsed, info.RateInfo),
				"resetAt":             info.BillingReset,
			})
		}

		group := map[string]any{
			"name":        "Command Code Rate Windows & Credits",
			"description": fmt.Sprintf("Subscription reset: %s · %s", info.BillingReset, info.RateInfo),
			"limits":      limits,
		}

		planName := info.PlanType
		if planName == "" {
			planName = "Command Code"
		}

		return map[string]any{
			"connection_id": id,
			"name":          name,
			"provider":      "commandcode",
			"plan":          planName,
			"quotas":        quotas,
			"groups":        []map[string]any{group},
			"fetched_at":    info.UpdatedAt,
		}
	}

	// Case 3: Qoder
	if formatLower == "qoder" {
		if s.proxy.qoderProvider == nil {
			return map[string]any{
				"connection_id": id,
				"name":          name,
				"provider":      "qoder",
				"error":         "Qoder provider not active",
			}
		}

		if force {
			quotaCache.Invalidate(apiKey)
		}

		q, ok := quotaCache.Get(apiKey)
		if !ok {
			sessions := s.proxy.qoderProvider.Sessions()
			qFetch, ferr := sessions.FetchQuota(ctx, apiKey)
			if ferr != nil {
				return map[string]any{
					"connection_id": id,
					"name":          name,
					"provider":      "qoder",
					"error":         ferr.Error(),
				}
			}
			q = qFetch
			quotaCache.Put(apiKey, q)
		}

		var limits []map[string]any
		quotas := make(map[string]any)

		if q.UserQuota != nil {
			remPct := 100.0
			if q.UserQuota.Total > 0 {
				remPct = (q.UserQuota.Total - q.UserQuota.Used) / q.UserQuota.Total * 100.0
				if remPct < 0 {
					remPct = 0
				}
			}
			lim := map[string]any{
				"displayName":         "Personal Credits",
				"window":              "credits",
				"used":                q.UserQuota.Used,
				"total":               q.UserQuota.Total,
				"remainingPercentage": remPct,
				"resetAt":             q.UserQuota.ResetTime,
				"description":         fmt.Sprintf("%.1f / %.0f credits used", q.UserQuota.Used, q.UserQuota.Total),
			}
			limits = append(limits, lim)
			quotas["user"] = lim
		}

		if q.AddonQuota != nil && q.AddonQuota.Total > 0 {
			remPct := 100.0
			if q.AddonQuota.Total > 0 {
				remPct = (q.AddonQuota.Total - q.AddonQuota.Used) / q.AddonQuota.Total * 100.0
				if remPct < 0 {
					remPct = 0
				}
			}
			lim := map[string]any{
				"displayName":         "Add-on Promo Credits",
				"window":              "addon",
				"used":                q.AddonQuota.Used,
				"total":               q.AddonQuota.Total,
				"remainingPercentage": remPct,
				"resetAt":             q.AddonQuota.ResetTime,
				"description":         fmt.Sprintf("%.1f / %.0f promo credits used", q.AddonQuota.Used, q.AddonQuota.Total),
			}
			limits = append(limits, lim)
			quotas["addon"] = lim
		}

		group := map[string]any{
			"name":        "Qoder Account Credits",
			"description": fmt.Sprintf("Plan: %s · Exceeded: %v", q.Plan, q.IsQuotaExceeded),
			"limits":      limits,
		}

		planName := q.Plan
		if planName == "" {
			planName = "Qoder"
		}

		return map[string]any{
			"connection_id": id,
			"name":          name,
			"provider":      "qoder",
			"plan":          planName,
			"quotas":        quotas,
			"groups":        []map[string]any{group},
			"fetched_at":    time.Now().UTC().Format(time.RFC3339),
		}
	}

	// Case 4: Other known providers (OpenRouter, DeepSeek, Groq, Kilo, etc.)
	if strings.Contains(baseLower, "openrouter") ||
		strings.Contains(baseLower, "deepseek") ||
		strings.Contains(baseLower, "kilo") ||
		strings.Contains(baseLower, "groq") ||
		strings.Contains(baseLower, "openai") {

		info := fetchProviderBalance(baseURL, apiKey, format)
		if info.Error != "" {
			return map[string]any{
				"connection_id": id,
				"name":          name,
				"provider":      info.ProviderType,
				"error":         info.Error,
			}
		}

		var limits []map[string]any
		if info.Balance != "" || info.RateInfo != "" || info.TotalUsed != "" {
			limits = append(limits, map[string]any{
				"displayName":         fmt.Sprintf("Balance (%s)", info.Currency),
				"window":              "balance",
				"used":                0,
				"total":               0,
				"remainingPercentage": 100.0,
				"description":         fmt.Sprintf("Current: %s · Used: %s · %s", info.Balance, info.TotalUsed, info.RateInfo),
			})
		}

		for _, win := range info.RateWindows {
			remPct := 100.0
			if win.Cap > 0 {
				remPct = (win.Cap - win.Used) / win.Cap * 100.0
				if remPct < 0 {
					remPct = 0
				}
			}
			limits = append(limits, map[string]any{
				"displayName":         fmt.Sprintf("%s Limit", win.Name),
				"window":              win.Name,
				"used":                win.Used,
				"total":               win.Cap,
				"remainingPercentage": remPct,
				"description":         fmt.Sprintf("Window %s: %.0f/%.0f used", win.Name, win.Used, win.Cap),
			})
		}

		group := map[string]any{
			"name":        fmt.Sprintf("%s Balance & Limits", strings.ToUpper(info.ProviderType)),
			"description": info.RateInfo,
			"limits":      limits,
		}

		planName := info.PlanType
		if planName == "" {
			planName = info.ProviderType
		}

		return map[string]any{
			"connection_id": id,
			"name":          name,
			"provider":      info.ProviderType,
			"plan":          planName,
			"quotas":        map[string]any{},
			"groups":        []map[string]any{group},
			"fetched_at":    info.UpdatedAt,
		}
	}

	return map[string]any{
		"connection_id": id,
		"name":          name,
		"provider":      format,
		"message":       "Usage/quota tracking not supported for this provider",
	}
}

// handleConnectionUsage returns live quota/usage for a single connection (9router parity).
// GET /api/usage/{connection_id}
func (s *Server) handleConnectionUsage(w http.ResponseWriter, r *http.Request) {
	connID := strings.TrimSpace(r.PathValue("connection_id"))
	if connID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": "connection_id is required"})
		return
	}

	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("refresh") == "1"

	var name, format, oauthProv, apiKey, baseURL string
	var isActive int
	err := s.db.Conn().QueryRow(`
		SELECT name, format, COALESCE(oauth_provider, ''), COALESCE(api_key, ''), COALESCE(base_url, ''), is_active
		FROM connections WHERE id = ?`, connID).Scan(&name, &format, &oauthProv, &apiKey, &baseURL, &isActive)
	if err != nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]any{"error": "Connection not found"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()

	result := s.buildConnectionQuota(ctx, connID, name, format, oauthProv, apiKey, baseURL, force)
	writeJSON(w, result)
}

// handleQuotaProviders returns list of all connections supporting live quota tracking.
// GET /api/quota/providers
func (s *Server) handleQuotaProviders(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("refresh") == "1"

	rows, err := s.db.Conn().Query(`
		SELECT id, name, format, COALESCE(oauth_provider, ''), COALESCE(api_key, ''), COALESCE(base_url, ''), is_active
		FROM connections
		WHERE is_active = 1 AND (
			LOWER(COALESCE(oauth_provider, '')) IN ('antigravity') OR
			LOWER(format) IN ('antigravity', 'commandcode', 'qoder') OR
			LOWER(base_url) LIKE '%commandcode%' OR
			LOWER(base_url) LIKE '%openrouter%' OR
			LOWER(base_url) LIKE '%deepseek%' OR
			LOWER(base_url) LIKE '%groq%' OR
			LOWER(base_url) LIKE '%kilo%'
		)
		ORDER BY priority DESC, name ASC`)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	type connTarget struct {
		id, name, format, oauthProv, apiKey, baseURL string
	}
	var targets []connTarget
	for rows.Next() {
		var it connTarget
		var isActive int
		if err := rows.Scan(&it.id, &it.name, &it.format, &it.oauthProv, &it.apiKey, &it.baseURL, &isActive); err == nil {
			targets = append(targets, it)
		}
	}
	rows.Close()

	var providers []map[string]any

	for _, it := range targets {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		quotaData := s.buildConnectionQuota(ctx, it.id, it.name, it.format, it.oauthProv, it.apiKey, it.baseURL, force)
		cancel()

		if quotaData != nil {
			providers = append(providers, quotaData)
		}
	}

	writeJSON(w, map[string]any{
		"success":   true,
		"providers": providers,
	})
}

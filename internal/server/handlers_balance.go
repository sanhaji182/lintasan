package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateWindow represents a usage window (5h, weekly, daily, etc.)
type RateWindow struct {
	Name      string  `json:"name"`       // e.g., "5-hour", "weekly", "daily"
	Used      float64 `json:"used"`       // Requests used in window
	Cap       float64 `json:"cap"`        // Max requests in window
	Exceeded  bool    `json:"exceeded"`   // Whether window limit was hit
}

// UsageStats represents request-level usage statistics.
type UsageStats struct {
	TotalRequests int     `json:"total_requests"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCredits  float64 `json:"total_credits"` // Dollar amount used
	SuccessRate   float64 `json:"success_rate"`  // 0-100
}

// BalanceInfo represents the credit/usage information from a provider.
type BalanceInfo struct {
	Balance      string       `json:"balance"`       // Current balance (e.g., "$95.20" or "9520 credits")
	TotalUsed    string       `json:"total_used"`    // Total amount used
	Currency     string       `json:"currency"`      // USD, credits, etc.
	PlanType     string       `json:"plan_type"`     // prepaid, subscription, free, etc.
	RateInfo     string       `json:"rate_info"`     // Legacy: flat rate info string (for non-CC providers)
	ProviderType string       `json:"provider_type"` // deepseek, openai, commandcode, etc.
	UpdatedAt    string       `json:"updated_at"`    // When this info was fetched
	Error        string       `json:"error,omitempty"` // Error message if fetch failed

	// Structured fields (populated for CommandCode, empty for others)
	RateWindows  []RateWindow `json:"rate_windows,omitempty"`
	Usage        *UsageStats  `json:"usage,omitempty"`
	BillingReset string       `json:"billing_reset,omitempty"` // e.g., "Jul 28"
}

// balanceCache caches balance info per connection ID to avoid spamming providers.
var balanceCache = struct {
	sync.RWMutex
	data map[string]cachedBalance
}{data: make(map[string]cachedBalance)}

type cachedBalance struct {
	info      BalanceInfo
	fetchedAt time.Time
}

const balanceCacheTTL = 5 * time.Minute

// handleGetConnectionBalance fetches credit/usage info from the provider for a specific connection.
// GET /api/connections/{id}/balance
func (s *Server) handleGetConnectionBalance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeBalanceJSON(w, http.StatusBadRequest, map[string]any{"error": "connection id required"})
		return
	}

	// Check cache first
	balanceCache.RLock()
	if cached, ok := balanceCache.data[id]; ok && time.Since(cached.fetchedAt) < balanceCacheTTL {
		balanceCache.RUnlock()
		writeBalanceJSON(w, http.StatusOK, map[string]any{"data": cached.info})
		return
	}
	balanceCache.RUnlock()

	// Load connection from DB
	var baseURL, apiKey, format, oauthProv string
	err := s.db.Conn().QueryRow(
		"SELECT base_url, api_key, format, COALESCE(oauth_provider,'') FROM connections WHERE id=?", id,
	).Scan(&baseURL, &apiKey, &format, &oauthProv)
	if err != nil {
		writeBalanceJSON(w, http.StatusNotFound, map[string]any{"error": "connection not found"})
		return
	}

	// Resolve OAuth token if needed
	if strings.TrimSpace(oauthProv) != "" {
		if tok, errTok := s.oauthMgr.GetActiveToken(oauthProv); errTok == nil {
			apiKey = tok
		}
	}

	if apiKey == "" {
		writeBalanceJSON(w, http.StatusBadRequest, map[string]any{"error": "no API key available for balance check"})
		return
	}

	// Detect provider and fetch balance
	info := fetchProviderBalance(baseURL, apiKey, format)
	info.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// Cache result
	balanceCache.Lock()
	balanceCache.data[id] = cachedBalance{info: info, fetchedAt: time.Now()}
	balanceCache.Unlock()

	writeBalanceJSON(w, http.StatusOK, map[string]any{"data": info})
}

// handleGetAllBalances fetches balance for all connections at once (parallel).
// GET /api/connections/balances
func (s *Server) handleGetAllBalances(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query("SELECT id, base_url, api_key, format, COALESCE(oauth_provider,'') FROM connections WHERE is_active=1")
	if err != nil {
		writeBalanceJSON(w, http.StatusInternalServerError, map[string]any{"error": "query failed"})
		return
	}
	defer rows.Close()

	type connInfo struct {
		ID       string
		BaseURL  string
		APIKey   string
		Format   string
		OAuthProv string
	}
	var conns []connInfo
	for rows.Next() {
		var c connInfo
		if err := rows.Scan(&c.ID, &c.BaseURL, &c.APIKey, &c.Format, &c.OAuthProv); err != nil {
			continue
		}
		conns = append(conns, c)
	}

	// Fetch balances in parallel
	type result struct {
		ID   string      `json:"id"`
		Data BalanceInfo `json:"balance"`
	}
	results := make([]result, len(conns))
	var wg sync.WaitGroup
	for i, c := range conns {
		wg.Add(1)
		go func(idx int, conn connInfo) {
			defer wg.Done()

			// Check cache
			balanceCache.RLock()
			if cached, ok := balanceCache.data[conn.ID]; ok && time.Since(cached.fetchedAt) < balanceCacheTTL {
				balanceCache.RUnlock()
				results[idx] = result{ID: conn.ID, Data: cached.info}
				return
			}
			balanceCache.RUnlock()

			key := conn.APIKey
			if strings.TrimSpace(conn.OAuthProv) != "" {
				if tok, errTok := s.oauthMgr.GetActiveToken(conn.OAuthProv); errTok == nil {
					key = tok
				}
			}
			if key == "" {
				results[idx] = result{ID: conn.ID, Data: BalanceInfo{Error: "no API key"}}
				return
			}

			info := fetchProviderBalance(conn.BaseURL, key, conn.Format)
			info.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

			balanceCache.Lock()
			balanceCache.data[conn.ID] = cachedBalance{info: info, fetchedAt: time.Now()}
			balanceCache.Unlock()

			results[idx] = result{ID: conn.ID, Data: info}
		}(i, c)
	}
	wg.Wait()

	if results == nil {
		results = []result{}
	}
	writeBalanceJSON(w, http.StatusOK, map[string]any{"data": results})
}

// fetchProviderBalance detects the provider from the base URL and fetches balance info.
func fetchProviderBalance(baseURL, apiKey, format string) BalanceInfo {
	host := extractHost(baseURL)

	// Route to provider-specific balance checker
	switch {
	case strings.Contains(host, "commandcode"):
		return fetchCommandCodeBalance(apiKey)
	case strings.Contains(host, "deepseek"):
		return fetchDeepseekBalance(apiKey)
	case strings.Contains(host, "openai"):
		return fetchOpenAIBalance(apiKey)
	case strings.Contains(host, "groq"):
		return fetchGroqBalance(apiKey, baseURL)
	case strings.Contains(host, "cerebras"):
		return fetchCerebrasBalance(apiKey, baseURL)
	case strings.Contains(host, "x.ai") || strings.Contains(host, "xai"):
		return fetchXAIBalance(apiKey, baseURL)
	default:
		// Generic OpenAI-compatible: try to extract rate limits from models endpoint
		return fetchGenericBalance(apiKey, baseURL, format)
	}
}

func fetchCommandCodeBalance(apiKey string) BalanceInfo {
	client := &http.Client{Timeout: 7 * time.Second}

	type ccResult struct {
		usage  []byte
		credit []byte
		sub    []byte
		err    error
	}

	base := "https://api.commandcode.ai"
	fetch := func(path string) []byte {
		req, _ := http.NewRequest("GET", base+path, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := client.Do(req)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			return nil
		}
		return b
	}

	// Fetch all three in parallel
	ch := make(chan ccResult, 1)
	go func() {
		u := fetch("/alpha/usage/summary")
		c := fetch("/alpha/billing/credits")
		s := fetch("/alpha/billing/subscriptions")
		ch <- ccResult{usage: u, credit: c, sub: s}
	}()

	select {
	case r := <-ch:
		info := BalanceInfo{ProviderType: "commandcode", PlanType: "subscription"}

		// Parse subscription → billing reset
		if r.sub != nil {
			var sub struct {
				Data struct {
					PlanID           string `json:"planId"`
					Status           string `json:"status"`
					CurrentPeriodEnd string `json:"currentPeriodEnd"`
				} `json:"data"`
			}
			if json.Unmarshal(r.sub, &sub) == nil {
				info.PlanType = sub.Data.PlanID
				if sub.Data.CurrentPeriodEnd != "" {
					if t, err := time.Parse(time.RFC3339, sub.Data.CurrentPeriodEnd); err == nil {
						info.BillingReset = t.Format("Jan 2")
						info.RateInfo = "Resets " + info.BillingReset
					}
				}
			}
		}

		// Parse credits → balance + rate windows
		if r.credit != nil {
			var cred struct {
				Credits struct {
					MonthlyCredits   float64 `json:"monthlyCredits"`
					PurchasedCredits float64 `json:"purchasedCredits"`
					FreeCredits      float64 `json:"freeCredits"`
				} `json:"credits"`
				WindowLimits struct {
					Limited  bool `json:"limited"`
					FiveHour struct {
						Used     float64 `json:"used"`
						Cap      float64 `json:"cap"`
						Exceeded bool    `json:"exceeded"`
					} `json:"fiveHour"`
					Weekly struct {
						Used     float64 `json:"used"`
						Cap      float64 `json:"cap"`
						Exceeded bool    `json:"exceeded"`
					} `json:"weekly"`
				} `json:"windowLimits"`
			}
			if err := json.Unmarshal(r.credit, &cred); err == nil {
				total := cred.Credits.MonthlyCredits + cred.Credits.PurchasedCredits + cred.Credits.FreeCredits
				info.Balance = fmt.Sprintf("$%.2f", total)
				info.Currency = "USD"

				// Structured rate windows
				wl := cred.WindowLimits
				if wl.Limited {
					info.RateWindows = []RateWindow{
						{Name: "5-hour", Used: wl.FiveHour.Used, Cap: wl.FiveHour.Cap, Exceeded: wl.FiveHour.Exceeded},
						{Name: "weekly", Used: wl.Weekly.Used, Cap: wl.Weekly.Cap, Exceeded: wl.Weekly.Exceeded},
					}
					// Legacy flat string (kept for backward compat)
					info.RateInfo += fmt.Sprintf(" · 5h: %d/%d · week: %d/%d",
						int(wl.FiveHour.Used), int(wl.FiveHour.Cap),
						int(wl.Weekly.Used), int(wl.Weekly.Cap))
					if wl.FiveHour.Exceeded || wl.Weekly.Exceeded {
						info.RateInfo += " ⚠️ limited"
					}
				}
			} else {
				info.Error = "credits parse error: " + err.Error()
			}
		}

		// Parse usage → structured stats
		if r.usage != nil {
			var usage struct {
				TotalCredits float64 `json:"totalCredits"`
				TotalCount   int     `json:"totalCount"`
				TotalTokens  int64   `json:"totalTokens"`
				SuccessRate  float64 `json:"successRate"`
			}
			if json.Unmarshal(r.usage, &usage) == nil {
				info.TotalUsed = fmt.Sprintf("$%.4f", usage.TotalCredits)
				info.Usage = &UsageStats{
					TotalRequests: usage.TotalCount,
					TotalTokens:   usage.TotalTokens,
					TotalCredits:  usage.TotalCredits,
					SuccessRate:   usage.SuccessRate,
				}
				if usage.TotalCount > 0 {
					tokensStr := fmt.Sprintf("%d", usage.TotalTokens)
					if usage.TotalTokens > 1e6 {
						tokensStr = fmt.Sprintf("%.1fM", float64(usage.TotalTokens)/1e6)
					}
					info.RateInfo += fmt.Sprintf(" · %d req · %s tok · %.0f%% ok",
						usage.TotalCount, tokensStr, usage.SuccessRate)
				}
			}
		}

		return info

	case <-time.After(7 * time.Second):
		return BalanceInfo{ProviderType: "commandcode", Error: "timeout"}
	}
}

func fetchDeepseekBalance(apiKey string) BalanceInfo {
	req, _ := http.NewRequest("GET", "https://api.deepseek.com/api/user/balance", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return BalanceInfo{ProviderType: "deepseek", Error: err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return BalanceInfo{ProviderType: "deepseek", Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 200))}
	}

	var data struct {
		Balance         string `json:"balance"`
		TotalUsed       string `json:"total_used"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
		IsAvailable     bool   `json:"is_available"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return BalanceInfo{ProviderType: "deepseek", Error: "parse error: " + err.Error()}
	}

	return BalanceInfo{
		Balance:      fmt.Sprintf("$%s", data.Balance),
		TotalUsed:    fmt.Sprintf("$%s", data.TotalUsed),
		Currency:     "USD",
		PlanType:     "prepaid",
		ProviderType: "deepseek",
	}
}

func fetchOpenAIBalance(apiKey string) BalanceInfo {
	// OpenAI doesn't have a simple balance API via API key.
	// Try to get usage from /v1/organization/usage (may not work with all keys)
	// Fall back to rate limit headers from a models request
	req, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return BalanceInfo{ProviderType: "openai", Error: err.Error()}
	}
	defer resp.Body.Close()
	resp.Body.Close()

	info := BalanceInfo{ProviderType: "openai", PlanType: "api"}

	// Extract rate limit headers
	if remaining := resp.Header.Get("x-ratelimit-remaining-requests"); remaining != "" {
		info.RateInfo = fmt.Sprintf("Requests remaining: %s", remaining)
	}
	if remaining := resp.Header.Get("x-ratelimit-remaining-tokens"); remaining != "" {
		if info.RateInfo != "" {
			info.RateInfo += " · "
		}
		info.RateInfo += fmt.Sprintf("Tokens remaining: %s", remaining)
	}

	if info.RateInfo == "" {
		info.RateInfo = "Rate limit info not available"
	}

	return info
}

func fetchGroqBalance(apiKey, baseURL string) BalanceInfo {
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(base, "/openai") {
		base += "/openai"
	}

	req, _ := http.NewRequest("GET", base+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return BalanceInfo{ProviderType: "groq", Error: err.Error()}
	}
	defer resp.Body.Close()
	resp.Body.Close()

	info := BalanceInfo{ProviderType: "groq", PlanType: "free_tier"}

	if remaining := resp.Header.Get("x-ratelimit-remaining-requests"); remaining != "" {
		info.RateInfo = fmt.Sprintf("Requests: %s remaining", remaining)
	}
	if remaining := resp.Header.Get("x-ratelimit-remaining-tokens"); remaining != "" {
		if info.RateInfo != "" {
			info.RateInfo += " · "
		}
		info.RateInfo += fmt.Sprintf("Tokens: %s remaining", remaining)
	}

	if info.RateInfo == "" {
		info.RateInfo = "Free tier (rate limits apply)"
	}

	return info
}

func fetchCerebrasBalance(apiKey, baseURL string) BalanceInfo {
	return fetchGenericRateLimits(apiKey, baseURL, "cerebras")
}

func fetchXAIBalance(apiKey, baseURL string) BalanceInfo {
	return fetchGenericRateLimits(apiKey, baseURL, "xai")
}

func fetchGenericBalance(apiKey, baseURL, format string) BalanceInfo {
	// For unknown providers, try the models endpoint and extract rate limit info
	return fetchGenericRateLimits(apiKey, baseURL, format)
}

func fetchGenericRateLimits(apiKey, baseURL, providerType string) BalanceInfo {
	base := strings.TrimRight(baseURL, "/")

	// Try /v1/models endpoint
	modelsURL := base
	if !strings.HasSuffix(base, "/v1/models") && !strings.HasSuffix(base, "/models") {
		modelsURL = base + "/v1/models"
	}

	req, _ := http.NewRequest("GET", modelsURL, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return BalanceInfo{ProviderType: providerType, Error: err.Error()}
	}
	defer resp.Body.Close()
	resp.Body.Close()

	info := BalanceInfo{ProviderType: providerType, PlanType: "api"}

	// Extract rate limit headers (standard across many OpenAI-compatible APIs)
	if remaining := resp.Header.Get("x-ratelimit-remaining-requests"); remaining != "" {
		info.RateInfo = fmt.Sprintf("Requests: %s remaining", remaining)
	}
	if remaining := resp.Header.Get("x-ratelimit-remaining-tokens"); remaining != "" {
		if info.RateInfo != "" {
			info.RateInfo += " · "
		}
		info.RateInfo += fmt.Sprintf("Tokens: %s remaining", remaining)
	}
	// Some providers use different header names
	if remaining := resp.Header.Get("x-ratelimit-remaining"); remaining != "" {
		info.RateInfo = fmt.Sprintf("Rate limit: %s remaining", remaining)
	}

	if resp.StatusCode == 200 && info.RateInfo == "" {
		info.RateInfo = "Connected · usage details not available from this provider"
	} else if resp.StatusCode != 200 {
		info.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return info
}

func extractHost(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rawURL = rawURL[idx+3:]
	}
	if idx := strings.Index(rawURL, "/"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	if idx := strings.Index(rawURL, ":"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	return strings.ToLower(rawURL)
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func writeBalanceJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

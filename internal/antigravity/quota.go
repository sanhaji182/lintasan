package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// QuotaBucket represents a single window bucket returned by Google Cloud Code Assist.
type QuotaBucket struct {
	BucketID          string  `json:"bucketId"`
	DisplayName       string  `json:"displayName"`
	Window            string  `json:"window"`
	ResetTime         string  `json:"resetTime"`
	Description       string  `json:"description"`
	RemainingFraction float64 `json:"remainingFraction"`
}

// QuotaGroup represents a logical group of models (Gemini or Claude/GPT).
type QuotaGroup struct {
	DisplayName string        `json:"displayName"`
	Description string        `json:"description"`
	Buckets     []QuotaBucket `json:"buckets"`
}

// RawQuotaResponse is the raw payload from /v1internal:retrieveUserQuotaSummary.
type RawQuotaResponse struct {
	Groups      []QuotaGroup `json:"groups"`
	Description string       `json:"description"`
}

// NormalizedQuotaItem is compatible with 9router's parsed quota dictionary.
type NormalizedQuotaItem struct {
	DisplayName         string  `json:"displayName"`
	Window              string  `json:"window"`
	Used                int     `json:"used"`
	Total               int     `json:"total"`
	RemainingPercentage float64 `json:"remainingPercentage"`
	ResetAt             string  `json:"resetAt"`
	Description         string  `json:"description"`
}

// QuotaGroupDisplay is an UI-ready structure for rendering quota cards.
type QuotaGroupDisplay struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Limits      []NormalizedQuotaItem `json:"limits"`
}

// AntigravityQuotaResult is the unified quota response for Google Antigravity.
type AntigravityQuotaResult struct {
	Plan      string                         `json:"plan"`
	Provider  string                         `json:"provider"`
	Quotas    map[string]NormalizedQuotaItem `json:"quotas"`
	Groups    []QuotaGroupDisplay            `json:"groups"`
	FetchedAt string                         `json:"fetched_at"`
}

type quotaCacheEntry struct {
	result    *AntigravityQuotaResult
	expiresAt time.Time
}

var (
	quotaCacheMu sync.RWMutex
	quotaCache   = make(map[string]quotaCacheEntry)
)

const quotaCacheTTL = 3 * time.Minute

// AntigravityQuotaEndpoint is the endpoint used to retrieve quota summaries.
var AntigravityQuotaEndpoint = fmt.Sprintf("%s/v1internal:retrieveUserQuotaSummary", AntigravityIDEBaseURL)

// ClearQuotaCache clears the in-memory cache (useful for testing).
func ClearQuotaCache() {
	quotaCacheMu.Lock()
	defer quotaCacheMu.Unlock()
	quotaCache = make(map[string]quotaCacheEntry)
}

// ParseQuotaSummary converts raw Google quota response into 9router-compatible normalized structure.
func ParseQuotaSummary(raw *RawQuotaResponse) *AntigravityQuotaResult {
	if raw == nil {
		return &AntigravityQuotaResult{
			Plan:      "free-tier",
			Provider:  "antigravity",
			Quotas:    make(map[string]NormalizedQuotaItem),
			Groups:    nil,
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		}
	}

	quotas := make(map[string]NormalizedQuotaItem)
	var groups []QuotaGroupDisplay

	for _, g := range raw.Groups {
		var limits []NormalizedQuotaItem
		isGemini := strings.Contains(strings.ToLower(g.DisplayName), "gemini")
		isClaude := strings.Contains(strings.ToLower(g.DisplayName), "claude") || strings.Contains(strings.ToLower(g.DisplayName), "gpt")

		for _, b := range g.Buckets {
			remPct := math.Round(b.RemainingFraction*10000) / 100 // e.g. 85.59%
			total := 1000
			remTokens := int(math.Round(float64(total) * b.RemainingFraction))
			used := total - remTokens
			if used < 0 {
				used = 0
			}

			item := NormalizedQuotaItem{
				DisplayName:         b.DisplayName,
				Window:              b.Window,
				Used:                used,
				Total:               total,
				RemainingPercentage: remPct,
				ResetAt:             b.ResetTime,
				Description:         b.Description,
			}
			limits = append(limits, item)

			// Map to stable 9router keys: gemini_weekly, gemini_session, claude_gpt_weekly, claude_gpt_session
			if isGemini {
				if b.Window == "weekly" {
					item.DisplayName = "Gemini (Weekly)"
					quotas["gemini_weekly"] = item
				} else if b.Window == "5h" {
					item.DisplayName = "Gemini (5h)"
					quotas["gemini_session"] = item
				}
			} else if isClaude {
				if b.Window == "weekly" {
					item.DisplayName = "Claude & GPT (Weekly)"
					quotas["claude_gpt_weekly"] = item
				} else if b.Window == "5h" {
					item.DisplayName = "Claude & GPT (5h)"
					quotas["claude_gpt_session"] = item
				}
			}
		}

		groups = append(groups, QuotaGroupDisplay{
			Name:        g.DisplayName,
			Description: g.Description,
			Limits:      limits,
		})
	}

	return &AntigravityQuotaResult{
		Plan:      "free-tier",
		Provider:  "antigravity",
		Quotas:    quotas,
		Groups:    groups,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// FetchQuota retrieves the current quota summary from Google Cloud Code Assist for Antigravity.
func FetchQuota(ctx context.Context, client *http.Client, accessToken string, bypassCache bool) (*AntigravityQuotaResult, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("antigravity quota: missing access token")
	}

	cacheKey := accessToken
	if len(cacheKey) > 32 {
		cacheKey = cacheKey[:32]
	}

	if !bypassCache {
		quotaCacheMu.RLock()
		entry, ok := quotaCache[cacheKey]
		quotaCacheMu.RUnlock()
		if ok && time.Now().Before(entry.expiresAt) {
			return entry.result, nil
		}
	}

	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	reqURL := AntigravityQuotaEndpoint
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewBufferString("{}"))
	if err != nil {
		return nil, fmt.Errorf("antigravity quota: create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", AntigravityIDEUserAgent)
	req.Header.Set("X-Client-Name", "antigravity")
	req.Header.Set("X-Client-Version", AntigravityIDEVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("antigravity quota: request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("antigravity quota: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("antigravity quota: upstream status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var raw RawQuotaResponse
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, fmt.Errorf("antigravity quota: decode json: %w", err)
	}

	res := ParseQuotaSummary(&raw)

	quotaCacheMu.Lock()
	quotaCache[cacheKey] = quotaCacheEntry{
		result:    res,
		expiresAt: time.Now().Add(quotaCacheTTL),
	}
	quotaCacheMu.Unlock()

	return res, nil
}

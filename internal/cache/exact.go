package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// InitExactCache creates the response_cache table if it does not already exist.
func InitExactCache(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS response_cache (
		hash TEXT PRIMARY KEY,
		provider TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL,
		request_body TEXT NOT NULL,
		response_body TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		hit_count INTEGER DEFAULT 0
	)`)
	return err
}

// strictCacheParams are the params that make fuzzy/semantic matching unsafe.
// A semantic hit is keyed on the last user message only, so two requests that
// differ only in these params are indistinguishable to it. Serving the earlier
// response would drop the caller's declared tools (or JSON-mode contract).
var strictCacheParams = []string{
	"tools", "tool_choice", "parallel_tool_calls",
	"functions", "function_call",
	"response_format",
}

// requestNeedsStrictCacheKey reports whether the request carries a param that
// requires an exact-match cache key, never a semantic/fuzzy one.
func requestNeedsStrictCacheKey(params map[string]any) bool {
	for _, key := range strictCacheParams {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

// RequestNeedsStrictKey is the exported form used by the server's cache-write
// guard, so the eligibility rule lives in exactly one place.
func RequestNeedsStrictKey(params map[string]any) bool {
	return requestNeedsStrictCacheKey(params)
}

// responseAffectingParams is the allow-list of request params that change the
// response body and therefore MUST participate in the cache key.
//
// This list previously held only temperature/max_tokens/top_p, which meant a
// request carrying `tools` hashed identically to the same request without it.
// The cached (tool-less) body was then replayed for a tool-enabled request, so
// the model appeared to "not have" the tool. Same class of bug for
// response_format (JSON mode returned prose), stop, seed, reasoning effort and
// n/ presence_penalty/frequency_penalty.
//
// Rule: if a param can change the completion, it belongs here. Anything added
// to the passthrough in future must be added here too.
var responseAffectingParams = []string{
	"temperature", "max_tokens", "top_p", "top_k",
	"tools", "tool_choice", "parallel_tool_calls",
	"functions", "function_call",
	"response_format",
	"stop", "seed", "n",
	"presence_penalty", "frequency_penalty", "logit_bias", "logprobs", "top_logprobs",
	"reasoning_effort", "reasoning", "thinking",
	"modalities", "audio", "prediction",
}

// buildExactHash computes a SHA-256 hash of the full request (model + messages + params).
func buildExactHash(model string, messages []any, params map[string]any) string {
	request := map[string]any{
		"model": model,
	}

	// Only include recognized params that affect the response.
	trimmed := map[string]any{}
	for _, key := range responseAffectingParams {
		if v, ok := params[key]; ok {
			trimmed[key] = v
		}
	}
	request["params"] = trimmed
	request["messages"] = messages

	b, err := json.Marshal(request)
	if err != nil {
		// Fallback: just hash model + messages if full marshaling fails.
		b, _ = json.Marshal(map[string]any{"model": model, "messages": messages})
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// GetExactMatch returns the cached response_body if an exact hash match exists and hasn't expired.
// Returns (responseBody string, found bool). Increments hit_count on each retrieval.
func GetExactMatch(db *sql.DB, model string, messages []any, params map[string]any) (string, bool) {
	hash := buildExactHash(model, messages, params)

	var responseBody string
	err := db.QueryRow(
		"SELECT response_body FROM response_cache WHERE hash=? AND expires_at > datetime('now')",
		hash,
	).Scan(&responseBody)

	if err != nil {
		return "", false
	}

	// Increment hit count (best-effort; don't fail the retrieval if this errors).
	_, _ = db.Exec("UPDATE response_cache SET hit_count = hit_count + 1 WHERE hash=?", hash)

	return responseBody, true
}

// SaveExactMatch saves a response to the exact hash cache.
// ttlSeconds: how long the entry should live (default 3600 = 1 hour if 0 is passed).
func SaveExactMatch(db *sql.DB, model string, messages []any, params map[string]any, responseBody string, inputTokens, outputTokens int, ttlSeconds int) error {
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}

	hash := buildExactHash(model, messages, params)

	requestBodyBytes, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"params":   params,
	})
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	expiresAt := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second).Format("2006-01-02 15:04:05")

	_, err = db.Exec(
		`INSERT OR REPLACE INTO response_cache
		 (hash, provider, model, request_body, response_body, input_tokens, output_tokens, expires_at)
		 VALUES (?, '', ?, ?, ?, ?, ?, ?)`,
		hash, model, string(requestBodyBytes), responseBody, inputTokens, outputTokens, expiresAt,
	)
	return err
}

// ClearExpiredExact removes all expired entries from the response_cache table.
// Returns the number of rows deleted.
func ClearExpiredExact(db *sql.DB) (int64, error) {
	result, err := db.Exec("DELETE FROM response_cache WHERE expires_at <= datetime('now')")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

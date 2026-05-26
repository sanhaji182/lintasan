package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/cache"
	"github.com/sanhaji182/lintasan-go/internal/circuit"
	"github.com/sanhaji182/lintasan-go/internal/combo"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/fallback"
	"github.com/sanhaji182/lintasan-go/internal/lb"
	"github.com/sanhaji182/lintasan-go/internal/optimizer"
	"github.com/sanhaji182/lintasan-go/internal/plugin"
	"github.com/sanhaji182/lintasan-go/internal/quota"
	"github.com/sanhaji182/lintasan-go/internal/ratelimit"
	"github.com/sanhaji182/lintasan-go/internal/reasoning"
	"github.com/sanhaji182/lintasan-go/internal/retry"
	"github.com/sanhaji182/lintasan-go/internal/webhook"
)

type ProxyHandler struct {
	cfg    *config.Config
	db     *db.DB
	pm     *plugin.Manager
	wm     *webhook.Manager
	client *http.Client

	rl        *ratelimit.Limiter          // rate limiter
	fb        *fallback.Engine            // fallback chain engine
	cmb       *combo.Engine               // hybrid combo engine
	lb        *lb.LoadBalancer            // load balancer
	breakers  map[string]*circuit.Breaker // per-connection circuit breakers
	breakerMu sync.RWMutex               // protects breakers map
}

func NewProxyHandler(cfg *config.Config, database *db.DB) *ProxyHandler {
	ph := &ProxyHandler{
		cfg: cfg,
		db:  database,
		pm:  plugin.NewManager(database.Conn()),
		wm:  webhook.NewManager(database.Conn()),
		client: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
			},
		},
	}
	ph.rl = ratelimit.New(60, 30)
	ph.fb = fallback.New(database)
	ph.fb.LoadChains()
	ph.cmb = combo.New()
	if cbJSON, err := database.GetSetting("combos"); err == nil && cbJSON != "" {
		ph.cmb.LoadFromSettings(cbJSON)
	}
	ph.breakers = make(map[string]*circuit.Breaker)
	ph.lb = ph.initLoadBalancer()
	return ph
}

type Connection struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Format     string `json:"format"`
	ChatPath   string `json:"chat_path"`
	AuthHeader string `json:"auth_header"`
	AuthPrefix string `json:"auth_prefix"`
	IsActive   int    `json:"is_active"`
	Priority   int    `json:"priority"`
}

func (p *ProxyHandler) getSetting(key, def string) string {
	val, err := p.db.GetSetting(key)
	if err != nil || val == "" { return def }
	return val
}

func (p *ProxyHandler) getBreaker(connID string) *circuit.Breaker {
	p.breakerMu.RLock()
	if b, ok := p.breakers[connID]; ok {
		p.breakerMu.RUnlock()
		return b
	}
	p.breakerMu.RUnlock()

	p.breakerMu.Lock()
	defer p.breakerMu.Unlock()
	if b, ok := p.breakers[connID]; ok {
		return b
	}
	b := circuit.New(3, 30*time.Second)
	p.breakers[connID] = b
	return b
}

// initLoadBalancer loads all active connections from the DB and creates a load balancer.
// Default strategy is Priority; can be changed via the "lb_strategy" setting.
func (p *ProxyHandler) initLoadBalancer() *lb.LoadBalancer {
	rows, err := p.db.Conn().Query(`
		SELECT id, priority, is_active
		FROM connections
		WHERE is_active = 1
		ORDER BY priority DESC
	`)
	if err != nil {
		// DB not ready yet, return empty LB
		return lb.New(lb.Priority, nil)
	}
	defer rows.Close()

	var conns []lb.Connection
	for rows.Next() {
		var c lb.Connection
		var isActive int
		if err := rows.Scan(&c.ID, &c.Priority, &isActive); err != nil {
			continue
		}
		c.Active = isActive == 1
		c.Weight = 1 // default weight; override if weight column exists
		conns = append(conns, c)
	}

	// Determine strategy from settings
	strategy := lb.Priority
	if s, err := p.db.GetSetting("lb_strategy"); err == nil && s != "" {
		switch lb.Strategy(s) {
		case lb.Priority, lb.RoundRobin, lb.LeastLatency, lb.Weighted, lb.Random:
			strategy = lb.Strategy(s)
		}
	}

	return lb.New(strategy, conns)
}

// UpdateLoadBalancerConnections refreshes the LB connection list from the DB.
// Call this after connections are added/removed/modified.
func (p *ProxyHandler) UpdateLoadBalancerConnections() {
	conns, err := p.loadLBConnections()
	if err != nil || len(conns) == 0 {
		return
	}
	p.lb.UpdateConnections(conns)
}

// loadLBConnections loads connections from the DB for the load balancer.
func (p *ProxyHandler) loadLBConnections() ([]lb.Connection, error) {
	rows, err := p.db.Conn().Query(`
		SELECT id, priority, is_active
		FROM connections
		WHERE is_active = 1
		ORDER BY priority DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []lb.Connection
	for rows.Next() {
		var c lb.Connection
		var isActive int
		if err := rows.Scan(&c.ID, &c.Priority, &isActive); err != nil {
			continue
		}
		c.Active = isActive == 1
		c.Weight = 1
		conns = append(conns, c)
	}
	return conns, nil
}

func (p *ProxyHandler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Fprintf(os.Stderr, "PANIC in HandleChatCompletions: %v\n", rec)
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", buf[:n])
			http.Error(w, fmt.Sprintf(`{"error":"internal server error: %v"}`, rec), http.StatusInternalServerError)
		}
	}()
	start := time.Now()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	model, _ := req["model"].(string)
	if model == "" {
		http.Error(w, `{"error":"model is required"}`, http.StatusBadRequest)
		return
	}
	
	stream, _ := req["stream"].(bool)
	messages, _ := req["messages"].([]any)

	// Rate Limiter
	if p.rl != nil {
		apiKey := r.Header.Get("Authorization")
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		// Check per-key
		allowed, remaining := p.rl.AllowKey(apiKey, 0)
		if !allowed {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":{"message":"rate limit exceeded","type":"rate_limit"}}`, http.StatusTooManyRequests)
			return
		}
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		// Check per-IP
		if !p.rl.AllowIP(clientIP) {
			http.Error(w, `{"error":{"message":"rate limit exceeded","type":"rate_limit"}}`, http.StatusTooManyRequests)
			return
		}
	}
	
	// Global max_tokens floor — reasoning models need room after system prompt
	if mt, ok := req["max_tokens"].(float64); !ok || mt < 8192 {
		req["max_tokens"] = 16384.0
	}
	
	// Optimizer
	if p.getSetting("prompt_optimizer_enabled", "false") == "true" {
		msgs, saved := optimizer.OptimizeMessages(messages)
		req["messages"] = msgs
		_ = saved
	}
	
	// Exact Hash Cache (fastest — check before semantic)
	exactCacheEnabled := p.getSetting("exact_cache_enabled", "true") == "true"
	if exactCacheEnabled {
		params := map[string]any{
			"temperature": req["temperature"],
			"max_tokens":  req["max_tokens"],
			"top_p":       req["top_p"],
		}
		if respBody, ok := cache.GetExactMatch(p.db.Conn(), model, messages, params); ok {
			p.logRequest(model, "exact-cache", "cache", 200, time.Since(start).Milliseconds(), 0, 0, true, "")
			if stream {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("X-Cache", "EXACT-HIT")
				w.WriteHeader(200)
				w.Write([]byte("data: " + respBody + "\n\n"))
				w.Write([]byte("data: [DONE]\n\n"))
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "EXACT-HIT")
				w.Write([]byte(respBody))
			}
			return
		}
	}

	// Stream Cache (check before semantic for streaming requests)
	streamCacheEnabled := p.getSetting("stream_cache_enabled", "true") == "true"
	if stream && streamCacheEnabled {
		if chunks, totalTokens, ok := cache.GetStreamMatch(p.db.Conn(), model, messages); ok {
			p.logRequest(model, "stream-cache", "cache", 200, time.Since(start).Milliseconds(), 0, totalTokens, true, "")
			cache.ReplayStream(w, chunks)
			return
		}
	}

	// Semantic Cache
	semanticEnabled := p.getSetting("semantic_cache_enabled", "true") == "true"
	if semanticEnabled {
		if respBody, ok := cache.GetSemanticMatch(p.db.Conn(), model, messages, 0.92); ok {
			p.logRequest(model, "cache", "cache", 200, time.Since(start).Milliseconds(), 0, 0, true, "")
			
			if stream {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				w.WriteHeader(200)
				
				// SSE cache replay format
				// In Node.js version we replay full SSE events, here we send as one big event
				// For real UI parsing we need to structure this similar to OpenAI chunks
				
				w.Write([]byte("data: " + respBody + "\n\n"))
				w.Write([]byte("data: [DONE]\n\n"))
				
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(respBody))
			}
			return
		}
	}

	candidates, resolvedModel, comboName, err := p.resolveRoute(model)
	if err != nil || len(candidates) == 0 {
		http.Error(w, fmt.Sprintf(`{"error":"no route found for model %s"}`, model), http.StatusNotFound)
		return
	}

	// Use load balancer to pick best connection from candidates
	if lbConn, lbErr := p.lb.Pick(); lbErr == nil && lbConn != nil {
		for i, c := range candidates {
			if c.ID == lbConn.ID {
				if i > 0 {
					candidates[0], candidates[i] = candidates[i], candidates[0]
				}
				break
			}
		}
	}
	req["model"] = resolvedModel
	body, _ = json.Marshal(req)

	var lastErr string
	var lastStatusCode int
	for i, conn := range candidates {
		// Circuit breaker check
		breaker := p.getBreaker(conn.ID)
		if !breaker.Allow() {
			lastErr = fmt.Sprintf("circuit breaker open for %s", conn.ID)
			lastStatusCode = 503
			p.logRequest(resolvedModel, conn.ID, conn.Name, 503, time.Since(start).Milliseconds(), 0, 0, false, lastErr)
			if p.fb != nil {
				p.fb.RecordEvent(resolvedModel, "", fallback.ReasonCircuit, 503)
			}
			// Try fallback connections
			if fallbackConns := p.fb.GetConnFallback(conn.ID); len(fallbackConns) > 0 {
				for _, fbID := range fallbackConns {
					fbConn, err := p.findConnectionByID(fbID)
					if err != nil {
						continue
					}
					candidates = append(candidates, fbConn)
				}
			}
			continue
		}

		// Retry wrapper around upstream call
		var resp *http.Response
		retryErr := retry.Do(r.Context(), retry.DefaultConfig(), func() (bool, error) {
			var err error
			resp, err = p.doUpstream(r, conn, body)
			if err != nil {
				return true, err // retry on connection errors
			}
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				resp.Body.Close()
				return true, fmt.Errorf("upstream returned %d", resp.StatusCode)
			}
			return false, nil // success, don't retry
		})

		if retryErr != nil {
			lastErr = retryErr.Error()
			breaker.Failure()
			p.logRequest(resolvedModel, conn.ID, conn.Name, 502, time.Since(start).Milliseconds(), 0, 0, false, lastErr)
			if p.fb != nil {
				if should, reason := fallback.ShouldTriggerFallback(502, false, false); should {
					p.fb.RecordEvent(resolvedModel, "", reason, 502)
				}
			}
			continue
		}

		defer resp.Body.Close()
		lastStatusCode = resp.StatusCode

		// On 5xx with more candidates: fallback
		if resp.StatusCode >= 500 && i < len(candidates)-1 {
			b, _ := io.ReadAll(resp.Body)
			lastErr = string(b)
			breaker.Failure()
			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), 0, 0, false, lastErr)
			if p.fb != nil {
				if should, reason := fallback.ShouldTriggerFallback(resp.StatusCode, false, false); should {
					p.fb.RecordEvent(resolvedModel, "", reason, resp.StatusCode)
				}
			}
			continue
		}

		// On 429: circuit breaker + fallback
		if resp.StatusCode == 429 {
			b, _ := io.ReadAll(resp.Body)
			lastErr = string(b)
			breaker.Failure()
			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), 0, 0, false, lastErr)
			if p.fb != nil {
				p.fb.RecordEvent(resolvedModel, "", fallback.Reason429, resp.StatusCode)
			}
			continue
		}

		// Success! Record to breaker
		breaker.Success()

		// Record latency for load balancer
		latencyMs := float64(time.Since(start).Milliseconds())
		p.lb.RecordLatency(conn.ID, latencyMs)

		// Token counting from response headers
		var tokensIn, tokensOut int
		if ct := resp.Header.Get("x-tokens-input"); ct != "" {
			fmt.Sscanf(ct, "%d", &tokensIn)
		}
		if ct := resp.Header.Get("x-tokens-output"); ct != "" {
			fmt.Sscanf(ct, "%d", &tokensOut)
		}

		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}

		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(resp.StatusCode)
			flusher, ok := w.(http.Flusher)

			var streamBuffer []byte
			if !ok {
				b, _ := io.ReadAll(resp.Body)
				w.Write(b)
				streamBuffer = b
			} else {
				buf := make([]byte, 4096)
				for {
					n, er := resp.Body.Read(buf)
					if n > 0 {
						w.Write(buf[:n])
						flusher.Flush()
						streamBuffer = append(streamBuffer, buf[:n]...)
					}
					if er != nil {
						break
					}
				}
			}

			// Approximate stream token counts
			if tokensOut == 0 {
				tokensOut = len(streamBuffer) / 4
			}
			if tokensIn == 0 {
				tokensIn = len(body) / 4
			}

			// Quota recording with actual tokens
			if resp.StatusCode == 200 {
				quota.RecordQuota(p.db.Conn(), conn.ID, tokensIn+tokensOut)
			}

			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), tokensIn, tokensOut, false, "")

			if comboName != "" && resp.StatusCode == 200 {
				p.cmb.RecordSuccess(comboName)
			}

			if semanticEnabled && resp.StatusCode == 200 {
				cache.SaveSemanticMatch(p.db.Conn(), model, messages, string(streamBuffer), 3600)
			}
			return
		}

		b, _ := io.ReadAll(resp.Body)

		// Translate CC Alpha SSE → OpenAI JSON
		if conn.Format == "commandcode" {
			b = translateCCAlphaToOpenAI(b)
		}

		// Reasoning extraction: DeepSeek V4 Pro puts answer in reasoning_content not content
		b = reasoning.ExtractReasoningContent(b)

		// Approximate token counts for non-stream
		if tokensOut == 0 {
			tokensOut = len(b) / 4
		}
		if tokensIn == 0 {
			tokensIn = len(body) / 4
		}

		// Quota recording with actual tokens
		if resp.StatusCode == 200 {
			quota.RecordQuota(p.db.Conn(), conn.ID, tokensIn+tokensOut)
		}

		w.WriteHeader(resp.StatusCode)
		w.Write(b)
		p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), tokensIn, tokensOut, false, "")

		if comboName != "" && resp.StatusCode == 200 {
			p.cmb.RecordSuccess(comboName)
		}

		if p.wm != nil {
			p.wm.Fire("request.success", map[string]interface{}{
				"model": resolvedModel,
				"status": resp.StatusCode,
			})
		}

		if semanticEnabled && resp.StatusCode == 200 {
			cache.SaveSemanticMatch(p.db.Conn(), model, messages, string(b), 3600)
		}
		return
	}

	if p.wm != nil {
		p.wm.Fire("request.error", map[string]interface{}{
			"model": model,
			"error": lastErr,
		})
	}

	_ = lastStatusCode
	http.Error(w, fmt.Sprintf(`{"error":{"message":"all routes failed","details":%q}}`, lastErr), http.StatusBadGateway)
}

func (p *ProxyHandler) findConnectionByID(id string) (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT id, name, base_url, api_key, format, chat_path, auth_header, auth_prefix, is_active, priority
		FROM connections
		WHERE id = ? AND is_active = 1
		LIMIT 1
	`, id)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority)
	if err != nil {
		return nil, fmt.Errorf("connection not found: %s", id)
	}
	return &conn, nil
}

func (p *ProxyHandler) doUpstream(r *http.Request, conn *Connection, body []byte) (*http.Response, error) {
	chatPath := conn.ChatPath
	if chatPath == "" { chatPath = "/v1/chat/completions" }
	upstreamURL := strings.TrimRight(conn.BaseURL, "/") + chatPath

	// Format translation: OpenAI body → commandcode body
	requestBody := body
	if conn.Format == "commandcode" {
		// Read thinking mode setting
		thinkingMode := "auto"
		if v, err := p.db.GetSetting("thinking_mode"); err == nil && v != "" {
			thinkingMode = v
		}
		requestBody = transformForCommandCode(body, thinkingMode)
	}

	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, strings.NewReader(string(requestBody)))
	if err != nil { return nil, err }
	upReq.Header.Set("Content-Type", "application/json")
	authHeader := conn.AuthHeader; if authHeader == "" { authHeader = "Authorization" }
	authPrefix := conn.AuthPrefix; if authPrefix == "" { authPrefix = "Bearer " }
	if conn.APIKey != "" { upReq.Header.Set(authHeader, authPrefix+conn.APIKey) }
	if xcc := r.Header.Get("X-Command-Code-Version"); xcc != "" {
		upReq.Header.Set("X-Command-Code-Version", xcc)
	}
	// CommandCode Alpha requires version header
	if conn.Format == "commandcode" {
		upReq.Header.Set("x-command-code-version", "0.26.25")
	}
	return p.client.Do(upReq)
}

// transformForCommandCode converts an OpenAI-format chat completions body
// into the CommandCode Alpha format (threadId/config/params).
// thinkingMode: "auto" (default), "enabled" (force reasoning), "disabled" (no reasoning)
func transformForCommandCode(body []byte, thinkingMode string) []byte {
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}

	// Extract model — CC Alpha needs provider prefix for DeepSeek models
	model, _ := req["model"].(string)
	// CC Alpha recognizes "deepseek/deepseek-v4-pro" but not bare "deepseek-v4-pro"
	if strings.HasPrefix(model, "deepseek-v4") && !strings.Contains(model, "/") {
		model = "deepseek/" + model
	}

	// Detect DeepSeek reasoning models for thinking control
	isDeepSeekReasoning := strings.Contains(model, "deepseek-v4-pro") || strings.Contains(model, "deepseek-r1")

	// Extract system prompt from messages
	var systemPrompt string
	var userMessages []map[string]any
	if msgs, ok := req["messages"].([]any); ok {
		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			role, _ := msg["role"].(string)
			if role == "system" || role == "developer" {
				if content, ok := msg["content"].(string); ok {
					systemPrompt += content + "\n\n"
				}
			} else {
				userMessages = append(userMessages, msg)
			}
		}
	}
	systemPrompt = strings.TrimSpace(systemPrompt)

	// Max tokens — dynamic floor based on thinking mode
	// When thinking is disabled, lower floor since no reasoning tokens needed
	floorTokens := 8192.0
	if isDeepSeekReasoning && thinkingMode == "disabled" {
		floorTokens = 1024
	}
	maxTokens := 16384.0
	if mt, ok := req["max_tokens"].(float64); ok && mt > 0 {
		if mt > 200000 {
			maxTokens = 200000
		} else if mt < floorTokens {
			maxTokens = floorTokens
		} else {
			maxTokens = mt
		}
	}

	// Stream: CC Alpha only supports streaming. Always send stream=true;
	// the handler will convert SSE → JSON for non-streaming clients.
	stream := true

	params := map[string]any{
		"model":      model,
		"messages":   userMessages,
		"system":     systemPrompt,
		"max_tokens": int(maxTokens),
		"stream":     stream,
	}

	// Thinking mode control for DeepSeek models (mirrors 9router's thinking.type injection)
	if isDeepSeekReasoning && thinkingMode != "auto" {
		params["thinking"] = map[string]any{"type": thinkingMode}
		if thinkingMode == "enabled" {
			params["reasoning_effort"] = "max"
		}
	}

	out := map[string]any{
		"threadId": uuid.New().String(),
		"memory":   "",
		"config": map[string]any{
			"workingDir":    "/tmp",
			"date":          time.Now().Format("2006-01-02"),
			"environment":   "linux",
			"structure":     []any{},
			"isGitRepo":     false,
			"currentBranch": "",
			"mainBranch":    "",
			"gitStatus":     "",
			"recentCommits": []any{},
		},
		"params": params,
	}

	result, _ := json.Marshal(out)
	return result
}

// translateCCAlphaToOpenAI converts CC Alpha SSE events into a standard OpenAI
// JSON chat completion response. CC Alpha streams line-delimited JSON events with
// types like "text-delta", "reasoning-delta", "finish", "provider-metadata", etc.
func translateCCAlphaToOpenAI(raw []byte) []byte {
	lines := strings.Split(string(raw), "\n")
	var contentText, reasoningText strings.Builder
	var finishReason string
	var usage map[string]any
	var modelUsed string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "null" || !strings.HasPrefix(line, "{") {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		typ, _ := event["type"].(string)
		switch typ {
		case "text-delta":
			if t, ok := event["text"].(string); ok {
				contentText.WriteString(t)
			}
		case "reasoning-start":
			// Just an event marker — no text
		case "reasoning-delta":
			if t, ok := event["text"].(string); ok {
				reasoningText.WriteString(t)
			}
		case "reasoning-end":
			// Marker event
		case "finish", "finish-step":
			if fr, ok := event["finishReason"].(string); ok && finishReason == "" {
				finishReason = fr
			}
			if u, ok := event["totalUsage"].(map[string]any); ok && usage == nil {
				usage = u
			}
			if u, ok := event["usage"].(map[string]any); ok && usage == nil {
				usage = u
			}
		case "provider-metadata":
			// Extract model from provider metadata
			if pm, ok := event["providerMetadata"].(map[string]any); ok {
				if g, ok := pm["gateway"].(map[string]any); ok {
					if r, ok := g["routing"].(map[string]any); ok {
						if slug, ok := r["canonicalSlug"].(string); ok && modelUsed == "" {
							modelUsed = slug
						}
					}
				}
			}
		case "start-step":
			// Extract model from request body
			if req, ok := event["request"].(map[string]any); ok {
				if body, ok := req["body"].(map[string]any); ok {
					if opts, ok := body["providerOptions"].(map[string]any); ok {
						if gw, ok := opts["gateway"].(map[string]any); ok {
							if only, ok := gw["only"].([]any); ok && len(only) > 0 && modelUsed == "" {
								if s, ok := only[0].(string); ok {
									modelUsed = s
								}
							}
						}
					}
				}
			}
		}
	}

	content := strings.TrimSpace(contentText.String())
	reasoning := strings.TrimSpace(reasoningText.String())

	if modelUsed == "" {
		modelUsed = "deepseek-v4-pro"
	}

	// Build OpenAI-compatible response
	message := map[string]any{
		"role": "assistant",
	}
	if content != "" {
		message["content"] = content
	}
	if reasoning != "" {
		message["reasoning_content"] = reasoning
	}

	resp := map[string]any{
		"id":      "cc-alpha-" + uuid.New().String()[:8],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelUsed,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
	}
	if usage != nil {
		resp["usage"] = usage
	}

	out, _ := json.Marshal(resp)
	return out
}

func (p *ProxyHandler) resolveRoute(model string) ([]*Connection, string, string, error) {
	model = p.resolveAlias(model)
	if conns, resolved, ok := p.resolveCombo(model); ok && len(conns) > 0 { return conns, resolved, model, nil }
	conn, err := p.findConnectionForModel(model)
	if err != nil { return nil, model, "", err }
	return []*Connection{conn}, model, "", nil
}

func (p *ProxyHandler) resolveAlias(model string) string {
	v, _ := p.db.GetSetting("aliases")
	if v == "" { return model }
	var m map[string]any
	if json.Unmarshal([]byte(v), &m) != nil { return model }
	if raw, ok := m[model]; ok {
		switch t := raw.(type) {
		case string: return t
		case map[string]any: if s, _ := t["model"].(string); s != "" { return s }
		}
	}
	return model
}

func (p *ProxyHandler) resolveCombo(name string) ([]*Connection, string, bool) {
	resolved, err := p.cmb.Resolve(name)
	if err != nil {
		return nil, name, false
	}

	// Determine the resolved model from the first entry
	resolvedModel := resolved[0].Model

	var out []*Connection
	for _, entry := range resolved {
		conns := p.connectionsForModelAndIDs(entry.Model, []string{entry.ConnectionID})
		// If the combo entry specifies an API key, override the connection's key
		if entry.APIKey != "" {
			for _, c := range conns {
				c.APIKey = entry.APIKey
			}
		}
		out = append(out, conns...)
	}
	if len(out) == 0 {
		return nil, name, false
	}
	return out, resolvedModel, true
}

func stringSlice(v any) []string { arr, _ := v.([]any); out := []string{}; for _, x := range arr { if s, ok := x.(string); ok { out = append(out, s) } }; return out }

func (p *ProxyHandler) connectionsForModelAndIDs(model string, ids []string) []*Connection {
	query := `SELECT c.id, c.name, c.base_url, c.api_key, c.format, c.chat_path, c.auth_header, c.auth_prefix, c.is_active, c.priority FROM discovered_models m JOIN connections c ON m.connection_id=c.id WHERE m.model_id=? AND m.is_active=1 AND c.is_active=1`
	args := []any{model}
	if len(ids) > 0 { ph:=make([]string,len(ids)); for i,id:=range ids{ph[i]="?"; args=append(args,id)}; query += " AND c.id IN ("+strings.Join(ph,",")+")" }
	query += " ORDER BY c.priority DESC"
	rows, err := p.db.Conn().Query(query, args...); if err != nil { return nil }; defer rows.Close()
	var out []*Connection
	for rows.Next(){ var c Connection; if rows.Scan(&c.ID,&c.Name,&c.BaseURL,&c.APIKey,&c.Format,&c.ChatPath,&c.AuthHeader,&c.AuthPrefix,&c.IsActive,&c.Priority)==nil{ out=append(out,&c) } }
	return out
}

func (p *ProxyHandler) findConnectionForModel(model string) (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT c.id, c.name, c.base_url, c.api_key, c.format, c.chat_path, c.auth_header, c.auth_prefix, c.is_active, c.priority
		FROM discovered_models m
		JOIN connections c ON m.connection_id = c.id
		WHERE m.model_id = ? AND m.is_active = 1 AND c.is_active = 1
		ORDER BY c.priority DESC
		LIMIT 1
	`, model)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority)
	if err != nil {
		return nil, fmt.Errorf("model not found: %s", model)
	}
	return &conn, nil
}

func (p *ProxyHandler) getFirstConnection() (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT id, name, base_url, api_key, format, chat_path, auth_header, auth_prefix, is_active, priority
		FROM connections
		WHERE is_active = 1
		ORDER BY priority DESC
		LIMIT 1
	`)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority)
	if err != nil {
		return nil, fmt.Errorf("no active connections")
	}
	return &conn, nil
}

func (p *ProxyHandler) logRequest(model, connID, provider string, status int, latencyMs int64, tokensIn, tokensOut int, cached bool, errMsg string) {
	cachedInt := 0; if cached { cachedInt = 1 }
	id := uuid.New().String()
	p.db.Conn().Exec(`
		INSERT INTO request_logs (id, connection_id, provider, model, status, input_tokens, output_tokens, latency_ms, cached, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`, id, connID, provider, model, status, tokensIn, tokensOut, latencyMs, cachedInt, errMsg)
}

func (p *ProxyHandler) HandleEmbeddings(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil { http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest); return }
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil { http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest); return }

	model, _ := req["model"].(string)
	if model == "" { model = "text-embedding-3-small" }

	conn, err := p.findConnectionForModel(model)
	if err != nil {
		conn, err = p.getFirstConnection()
		if err != nil { http.Error(w, fmt.Sprintf(`{"error":"no connection found for model %s"}`, model), http.StatusNotFound); return }
	}

	upstreamURL := strings.TrimRight(conn.BaseURL, "/") + "/v1/embeddings"
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, strings.NewReader(string(body)))
	if err != nil { http.Error(w, `{"error":"failed to create upstream request"}`, http.StatusInternalServerError); return }

	upReq.Header.Set("Content-Type", "application/json")
	authHeader := conn.AuthHeader; if authHeader == "" { authHeader = "Authorization" }
	authPrefix := conn.AuthPrefix; if authPrefix == "" { authPrefix = "Bearer " }
	if conn.APIKey != "" { upReq.Header.Set(authHeader, authPrefix+conn.APIKey) }

	resp, err := p.client.Do(upReq)
	if err != nil { http.Error(w, fmt.Sprintf(`{"error":"upstream error: %s"}`, err.Error()), http.StatusBadGateway); return }
	defer resp.Body.Close()

	for k, v := range resp.Header { for _, vv := range v { w.Header().Add(k, vv) } }
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *ProxyHandler) proxyPath(w http.ResponseWriter, r *http.Request, upstreamPath string){
    body,_:=io.ReadAll(r.Body); var reqBody io.Reader=bytes.NewReader(body)
    conn,err:=p.getFirstConnection(); if err!=nil{http.Error(w,`{"error":"no active connections"}`,404);return}
    upReq,err:=http.NewRequestWithContext(r.Context(),"POST",strings.TrimRight(conn.BaseURL,"/")+upstreamPath,reqBody); if err!=nil{http.Error(w,err.Error(),500);return}
    upReq.Header.Set("Content-Type", r.Header.Get("Content-Type")); if upReq.Header.Get("Content-Type")==""{upReq.Header.Set("Content-Type","application/json")}
    if conn.APIKey!=""{h:=conn.AuthHeader; if h==""{h="Authorization"}; pfx:=conn.AuthPrefix; if pfx==""{pfx="Bearer "}; upReq.Header.Set(h,pfx+conn.APIKey)}
    resp,err:=p.client.Do(upReq); if err!=nil{http.Error(w,fmt.Sprintf(`{"error":"upstream error: %s"}`,err.Error()),502);return}; defer resp.Body.Close()
    for k,v:=range resp.Header{for _,vv:=range v{w.Header().Add(k,vv)}}; w.WriteHeader(resp.StatusCode); io.Copy(w,resp.Body)
}
func (p *ProxyHandler) HandleImages(w http.ResponseWriter,r *http.Request){ p.proxyPath(w,r,"/v1/images/generations") }
func (p *ProxyHandler) HandleAudioSpeech(w http.ResponseWriter,r *http.Request){ p.proxyPath(w,r,"/v1/audio/speech") }
func (p *ProxyHandler) HandleAudioTranscriptions(w http.ResponseWriter,r *http.Request){ p.proxyPath(w,r,"/v1/audio/transcriptions") }

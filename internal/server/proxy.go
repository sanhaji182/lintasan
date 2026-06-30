package server

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/cache"
	"github.com/sanhaji182/lintasan-go/internal/circuit"
	"github.com/sanhaji182/lintasan-go/internal/combo"
	"github.com/sanhaji182/lintasan-go/internal/compress"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/cost"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/fallback"
	"github.com/sanhaji182/lintasan-go/internal/lb"
	"github.com/sanhaji182/lintasan-go/internal/memory"
	"github.com/sanhaji182/lintasan-go/internal/metrics"
	"github.com/sanhaji182/lintasan-go/internal/mlrouter"
	"github.com/sanhaji182/lintasan-go/internal/optimizer"
	"github.com/sanhaji182/lintasan-go/internal/plugin"
	"github.com/sanhaji182/lintasan-go/internal/provider"
	"github.com/sanhaji182/lintasan-go/internal/quality"
	"github.com/sanhaji182/lintasan-go/internal/quota"
	"github.com/sanhaji182/lintasan-go/internal/ratelimit"
	"github.com/sanhaji182/lintasan-go/internal/reasoning"
	"github.com/sanhaji182/lintasan-go/internal/reflect"
	"github.com/sanhaji182/lintasan-go/internal/retry"
	"github.com/sanhaji182/lintasan-go/internal/tokencount"
	"github.com/sanhaji182/lintasan-go/internal/webhook"
)

type ProxyHandler struct {
	cfg    *config.Config
	db     *db.DB
	pm     *plugin.Manager
	wm     *webhook.Manager
	quota  *quota.QuotaTracker
	client *http.Client

	rl            *ratelimit.Limiter          // rate limiter
	rlEnabled     bool                        // false = limiter bypassed
	fb            *fallback.Engine            // fallback chain engine
	cmb           *combo.Engine               // hybrid combo engine
	lb            *lb.LoadBalancer            // load balancer
	breakers      map[string]*circuit.Breaker // per-connection circuit breakers
	breakerMu     sync.RWMutex                // protects breakers map
	mem           *memory.MemoryStore         // vector memory (nil if Redis unavailable)
	compressor    *compress.Compressor        // context compression
	qf            *quality.Filter             // quality filter for multi-shot
	mlr           mlrouter.ModelPair          // ML router model pair config
	costCalc      *cost.Calculator            // cost calculator for cost-based routing
	telemetry     *proxyTelemetry             // structured telemetry counters
	qualityMu     sync.RWMutex                // protects qualityScores
	qualityScores map[string]connQualityScore // quality feedback loop per connection

	// --- Provider SDK (F1) ----------------------------------------------------
	// providerReg holds Official-track providers; defaultProvider is the generic
	// OpenAI-compatible fallback used for any unmigrated Format. providerSDK is
	// the kill-switch flag (default false): when false, doUpstream takes the
	// untouched legacy path. See provider_bootstrap.go.
	providerReg     *provider.Registry
	defaultProvider provider.Provider
	providerSDK     bool

	// capabilityShadow is the F2.3 kill-switch (default false): when true, the
	// chat router evaluates candidate capability eligibility in OBSERVE-ONLY
	// mode (records would-exclude, never actually excludes). Read once at
	// startup in initProviderSDK. See provider_bootstrap.go + capability_shadow.go.
	capabilityShadow bool
	// capabilityEnforce is the F2.4 kill-switch (default false): when true, the
	// chat router ACTS on capability eligibility — it DROPS candidates that
	// positively fail the request's required caps (data-backed only; fail-open
	// for missing data; never empties the pool). Reuses the exact F2.3 resolver,
	// so enforce acts on precisely what shadow observed (zero drift). Read once
	// at startup in initProviderSDK. See provider_bootstrap.go + capability_enforce.go.
	capabilityEnforce bool
	// embedderSDK is the F2.5 kill-switch (default false): when false,
	// HandleEmbeddings takes the untouched inline path. When true, the upstream
	// /v1/embeddings request is BUILT by the provider Embedder and executed by
	// the handler, byte-for-byte identical to the inline path. Read once at
	// startup in initProviderSDK. See provider_bootstrap.go.
	embedderSDK bool
	// shadowStats aggregates F2.3 shadow evidence across requests (Option A,
	// observe-only). Written AFTER selection, never read by the routing path.
	// Surfaced read-only via GET /api/capabilities/shadow.
	shadowStats *provider.ShadowAggregator

	oauthMgr *auth.OAuthManager // IDE OAuth session → upstream bearer overlay

	// Multi-account pools: pool_id → MultiAccountLB
	mabMu           sync.RWMutex
	multiAccountLBs map[string]*lb.MultiAccountLB
}

func NewProxyHandler(cfg *config.Config, database *db.DB) *ProxyHandler {
	ph := &ProxyHandler{
		cfg:   cfg,
		db:    database,
		pm:    plugin.NewManager(database.Conn()),
		wm:    webhook.NewManager(database.Conn()),
		quota: quota.NewTracker(database.Conn()),
		client: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:          256,
				MaxIdleConnsPerHost:   64,
				MaxConnsPerHost:       128,
				IdleConnTimeout:       180 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				DisableCompression:    true, // prevent Go from auto-decompressing upstream responses
				TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
			},
		},
	}
	// Rate limiter: per-key requests/min, burst. Defaults 60/30 (unchanged).
	// Override via env for benchmarking / tuning; 0 disables the limiter.
	rlPerMin, rlBurst := 60, 30
	if ph.cfg.RateLimitPerMin > 0 {
		rlPerMin = ph.cfg.RateLimitPerMin
	}
	if ph.cfg.RateLimitBurst > 0 {
		rlBurst = ph.cfg.RateLimitBurst
	}
	ph.rlEnabled = rlPerMin > 0
	if ph.rlEnabled {
		ph.rl = ratelimit.New(rlPerMin, rlBurst)
	}
	ph.fb = fallback.New(database)
	ph.fb.LoadChains()
	ph.cmb = combo.New()
	if cbJSON, err := database.GetSetting("combos"); err == nil && cbJSON != "" {
		ph.cmb.LoadFromSettings(cbJSON)
	}
	ph.breakers = make(map[string]*circuit.Breaker)
	ph.lb = ph.initLoadBalancer()
	ph.mem = memory.NewAuto(memory.Config{Addr: "", DataDir: "data"})

	// Initialize context compressor with default settings
	// These can be overridden via settings if needed
	ph.compressor = compress.New(8000, 6, 8000)

	// Initialize quality filter for multi-shot routing
	qfThreshold := 0.4
	if v, err := database.GetSetting("quality_filter_threshold"); err == nil && v != "" {
		if t, err := strconv.ParseFloat(v, 64); err == nil && t > 0 && t < 1 {
			qfThreshold = t
		}
	}
	ph.qf = quality.New(qfThreshold, quality.Weights{})

	// ML router model pair from settings
	ph.mlr = mlrouter.ModelPair{
		CheapModel:     ph.getSettingFromDB(database, "ml_router_cheap_model", "gpt-4o-mini"),
		ExpensiveModel: ph.getSettingFromDB(database, "ml_router_expensive_model", "gpt-4o"),
		Threshold:      0.5,
	}
	if v, err := database.GetSetting("ml_router_threshold"); err == nil && v != "" {
		if t, err := strconv.ParseFloat(v, 64); err == nil && t > 0 && t < 1 {
			ph.mlr.Threshold = t
		}
	}

	ph.telemetry = newProxyTelemetry()
	ph.qualityScores = map[string]connQualityScore{}
	ph.costCalc = cost.NewCalculator()
	ph.loadQuotaLimits(database)
	ph.initProviderSDK(database)
	ph.hydrateExperimentalProviders()
	ph.initMultiAccountPools()
	go ph.prewarmConnectionPool()

	return ph
}

// loadQuotaLimits reads the operator-configured "quota_limits" setting and
// installs per-connection daily token limits into the quota tracker. Without
// this, QuotaTracker.Allow() always returns true (no limit configured) and the
// quota gate in the request loop is a silent no-op. Format (JSON object):
//
//	{ "<connID>": { "max_tokens_per_day": 1000000, "max_tokens_per_month": 0,
//	                "max_requests_per_day": 0, "max_requests_per_month": 0 }, ... }
func (p *ProxyHandler) loadQuotaLimits(database *db.DB) {
	if p.quota == nil {
		return
	}
	raw, err := database.GetSetting("quota_limits")
	if err != nil || raw == "" {
		return
	}
	var limits map[string]struct {
		MaxTokensPerDay     int64 `json:"max_tokens_per_day"`
		MaxTokensPerMonth   int64 `json:"max_tokens_per_month"`
		MaxRequestsPerDay   int64 `json:"max_requests_per_day"`
		MaxRequestsPerMonth int64 `json:"max_requests_per_month"`
	}
	if err := json.Unmarshal([]byte(raw), &limits); err != nil {
		return
	}
	for connID, l := range limits {
		p.quota.SetLimit(connID, &quota.QuotaLimit{
			MaxTokensPerDay:     l.MaxTokensPerDay,
			MaxTokensPerMonth:   l.MaxTokensPerMonth,
			MaxRequestsPerDay:   l.MaxRequestsPerDay,
			MaxRequestsPerMonth: l.MaxRequestsPerMonth,
		})
	}
}

type Connection struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	OAuthProvider string `json:"oauth_provider,omitempty"` // 9router OAuth id: xai, claude, github, ...
	PoolID        string `json:"pool_id,omitempty"`        // multi-account pool group
	Format        string `json:"format"`
	ChatPath      string `json:"chat_path"`
	AuthHeader    string `json:"auth_header"`
	AuthPrefix    string `json:"auth_prefix"`
	IsActive      int    `json:"is_active"`
	Priority      int    `json:"priority"`
}

func (p *ProxyHandler) getSetting(key, def string) string {
	val, err := p.db.GetSetting(key)
	if err != nil || val == "" {
		return def
	}
	return val
}

// getSettingFromDB is a static helper that reads a setting from a DB instance.
// Used during initialization before ProxyHandler is fully constructed.
func (p *ProxyHandler) getSettingFromDB(database *db.DB, key, def string) string {
	val, err := database.GetSetting(key)
	if err != nil || val == "" {
		return def
	}
	return val
}

// ReloadSmartRoutingConfig re-reads the smart-routing settings into in-memory
// state so dashboard edits take effect WITHOUT a process restart. It refreshes
// the ML router model pair + threshold and reinstalls quota limits.
func (p *ProxyHandler) ReloadSmartRoutingConfig() {
	p.mlr.CheapModel = p.getSetting("ml_router_cheap_model", "gpt-4o-mini")
	p.mlr.ExpensiveModel = p.getSetting("ml_router_expensive_model", "gpt-4o")
	p.mlr.Threshold = 0.5
	if v, err := p.db.GetSetting("ml_router_threshold"); err == nil && v != "" {
		if t, err := strconv.ParseFloat(v, 64); err == nil && t > 0 && t < 1 {
			p.mlr.Threshold = t
		}
	}
	p.loadQuotaLimits(p.db)
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
	taskClass := classifyTask(model, messages)
	routeProfile := pickRouteProfile(taskClass, r.Header.Get("User-Agent"))

	// ML routing (Gap #1): when the caller targets the ML sentinel model
	// ("ml-auto"/"smart") or the operator enables ml_router_enabled globally,
	// pick cheap-vs-expensive from the 15-feature complexity score instead of
	// the literal model. Header X-Lintasan-ML-Tier records the decision.
	if mlModel, tier, ok := p.applyMLRouting(model, messages); ok {
		w.Header().Set("X-Lintasan-ML-Tier", tier)
		w.Header().Set("X-Lintasan-ML-Model", mlModel)
		model = mlModel
		req["model"] = mlModel
	}

	directMode := p.isDirectEquivalentMode(r)
	modeLabel := "intelligent"
	if directMode {
		modeLabel = "direct-equivalent"
	}
	w.Header().Set("X-Lintasan-Task-Class", taskClass)
	w.Header().Set("X-Lintasan-Route-Profile", routeProfile)
	w.Header().Set("X-Lintasan-Mode", modeLabel)

	if deduped, removed := dedupMessages(messages); removed > 0 {
		messages = deduped
		req["messages"] = deduped
		w.Header().Set("X-Lintasan-Deduped", fmt.Sprintf("%d", removed))
	}
	p.applyTaskBudgetGuardrail(req, taskClass)

	// P7.4 Self-Review Loop — triggered by X-Lintasan-Reflect header
	// Must be BEFORE any upstream call so the reflect loop owns the full request flow
	if reflectHeader := r.Header.Get("X-Lintasan-Reflect"); reflectHeader != "" {
		maxTokens := 4096
		if mt, ok := req["max_tokens"].(float64); ok && mt > 0 {
			maxTokens = int(mt)
		}
		p.handleReflectLoop(r, model, messages, maxTokens, w)
		return
	}

	// Rate Limiter
	if p.rl != nil {
		apiKey := r.Header.Get("Authorization")
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}

		// Check per-key (skip entirely when limiter disabled)
		if p.rlEnabled {
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
	}

	// Global max_tokens floor — only when not in direct-equivalent mode.
	if !directMode {
		if mt, ok := req["max_tokens"].(float64); !ok || mt < 1024 {
			req["max_tokens"] = 1024.0
		}
	}

	// Optimizer
	if !directMode && p.getSetting("prompt_optimizer_enabled", "false") == "true" {
		msgs, saved := optimizer.OptimizeMessages(messages)
		req["messages"] = msgs
		_ = saved
	}

	// Vector Memory — Prompt Injection
	// Search for similar past successes and inject as system context.
	// Gated by memory_injection_enabled (default false): the search runs on
	// EVERY request when the store is reachable and is a measured hot-path cost,
	// so it must be opt-in rather than implicitly on whenever Redis is up.
	var injectedMemories []memory.Memory
	if !directMode && p.getSetting("memory_injection_enabled", "false") == "true" && p.mem != nil && p.mem.Available() {
		promptText := buildPromptText(messages)
		if promptText != "" {
			queryEmb := memory.Embed(promptText)
			results, searchErr := p.mem.Store.Search(queryEmb, 3)
			if searchErr == nil {
				for _, m := range results {
					if m.Similarity > 0.75 {
						injectedMemories = append(injectedMemories, m)
					}
				}
				if len(injectedMemories) > 0 {
					// Inject context as a system message
					ctxLines := []string{"[Memory context from past Lintasan completions]"}
					for _, m := range injectedMemories {
						ctxLines = append(ctxLines, fmt.Sprintf("- %s (similarity: %.2f)", truncate(m.Text, 200), m.Similarity))
					}
					ctxMsg := map[string]any{"role": "system", "content": strings.Join(ctxLines, "\n")}
					msgs, ok := req["messages"].([]any)
					if !ok {
						msgs = []any{}
					}
					// Insert at position 0 so it comes before user messages
					req["messages"] = append([]any{ctxMsg}, msgs...)
				}
			}
		}
	}

	// Exact Hash Cache (fastest — check before semantic)
	exactCacheEnabled := !directMode && p.getSetting("exact_cache_enabled", "true") == "true"
	if exactCacheEnabled {
		params := map[string]any{
			"temperature": req["temperature"],
			"max_tokens":  req["max_tokens"],
			"top_p":       req["top_p"],
		}
		if respBody, ok := cache.GetExactMatch(p.db.Conn(), model, messages, params); ok {
			metrics.CacheHit()
			p.logRequest(model, "exact-cache", "cache", 200, time.Since(start).Milliseconds(), 0, 0, true, "", taskClass, modeLabel)
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
	streamCacheEnabled := !directMode && p.getSetting("stream_cache_enabled", "true") == "true"
	if stream && streamCacheEnabled {
		if chunks, totalTokens, ok := cache.GetStreamMatch(p.db.Conn(), model, messages); ok {
			p.logRequest(model, "stream-cache", "cache", 200, time.Since(start).Milliseconds(), 0, totalTokens, true, "", taskClass, modeLabel)
			cache.ReplayStream(w, chunks)
			return
		}
	}

	// Semantic Cache
	semanticEnabled := !directMode && p.getSetting("semantic_cache_enabled", "true") == "true"
	if semanticEnabled {
		if respBody, score, ok := cache.GetSemanticMatch(p.db.Conn(), model, messages, 0.75); ok {
			metrics.CacheHit()
			p.logRequest(model, "semantic-cache", "cache", 200, time.Since(start).Milliseconds(), 0, 0, true, fmt.Sprintf("score=%.3f", score), taskClass, modeLabel)

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

	// Cache miss: the request was cache-eligible (exact and/or semantic cache
	// was enabled) but fell through every cache check to an upstream call.
	// Recorded exactly once here, the single point all cache-eligible misses
	// funnel through. A request that bypassed caching entirely (direct mode or
	// both caches disabled) is NOT a miss and must not inflate the denominator.
	if exactCacheEnabled || semanticEnabled {
		metrics.CacheMiss()
	}

	// --- Experimental Provider interception (R2) ------------------------------
	// BEFORE normal routing: check for an explicit experimental opt-in signal.
	// If present, resolve + execute via the Agent interface and return. The
	// normal resolveRoute path is NEVER reached for experimental requests.
	// Membrane: the handler uses the explicit experimental door, never
	// ResolveRoutable. Official routing is byte-for-byte unchanged.
	if p.handleExperimentalRoute(w, r, model, req, body) {
		return
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
	candidates = p.reorderCandidatesForTask(candidates, taskClass, routeProfile)

	// F2.3 capability shadow routing (observe-only, flag-gated, default OFF).
	// When enabled, evaluate whether each candidate would satisfy the request's
	// required capabilities and RECORD the result. This NEVER mutates, reorders,
	// or filters `candidates` — selection below is byte-identical regardless.
	// Enforcement (dropping non-satisfying candidates) is F2.4, gated separately.
	p.runCapabilityShadow(w, req, resolvedModel, stream, candidates)

	// F2.4 capability ENFORCEMENT (act, flag-gated, default OFF). When enabled,
	// drop candidates that positively fail the request's required capabilities
	// (data-backed only; fail-open for missing data; never empties the pool).
	// Reuses the EXACT F2.3 resolver, so it acts on precisely what the shadow
	// hook above observed. Flag OFF → returns `candidates` unchanged (byte-
	// identical legacy/shadow). Enforcement only narrows + preserves order.
	candidates = p.applyCapabilityEnforcement(w, req, resolvedModel, stream, candidates)
	if len(candidates) == 0 {
		// Defensive: the facade's R3 guard guarantees this never happens, but if
		// a future change ever broke that invariant, fail-open to a no-route error
		// rather than panic on an empty pool below.
		http.Error(w, fmt.Sprintf(`{"error":"no eligible route for model %s"}`, model), http.StatusNotFound)
		return
	}

	req["model"] = resolvedModel
	body, _ = json.Marshal(req)

	// Plugin pre-request hook: transform request body before upstream
	if p.pm != nil {
		if transformedBody, err := p.pm.ExecuteRequestHook(r.Context(), comboName, resolvedModel, body); err != nil {
			fmt.Fprintf(os.Stderr, "plugin pre-request error: %v\n", err)
		} else {
			body = transformedBody
		}
	}

	// Context compression: compress messages if token count exceeds threshold
	var compressionStats compress.Stats
	compressionEnabled := p.getSetting("context_compression_enabled", "true") == "true"
	if compressionEnabled && p.compressor != nil {
		// Re-parse body to get messages
		var compressReq map[string]any
		if err := json.Unmarshal(body, &compressReq); err == nil {
			if messages, ok := compressReq["messages"].([]any); ok {
				// Convert []any to []map[string]any
				msgs := make([]map[string]any, 0, len(messages))
				for _, m := range messages {
					if msgMap, ok := m.(map[string]any); ok {
						msgs = append(msgs, msgMap)
					}
				}

				compressed, stats := p.compressor.Compress(msgs)
				compressionStats = stats

				if stats.WasCompressed {
					// Update messages in request
					anyMsgs := make([]any, len(compressed))
					for i, m := range compressed {
						anyMsgs[i] = m
					}
					compressReq["messages"] = anyMsgs
					body, _ = json.Marshal(compressReq)

					fmt.Fprintf(os.Stderr, "Context compressed: %d -> %d tokens (%.1f%% reduction, %d -> %d messages)\n",
						stats.OriginalTokens, stats.CompressedTokens,
						stats.CompressionRatio*100, stats.MessagesBefore, stats.MessagesAfter)
				}
			}
		}
	}

	if p.shouldHedge(stream, directMode, candidates) {
		if hedgeConn, hedgeResp, hedgeErr := p.doHedgedUpstream(r, candidates, body); hedgeErr == nil && hedgeResp != nil {
			defer hedgeResp.Body.Close()
			w.Header().Set("X-Lintasan-Hedge", "hit")
			for k, v := range hedgeResp.Header {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
			if compressionStats.WasCompressed {
				w.Header().Set("X-Lintasan-Compressed", "true")
				w.Header().Set("X-Lintasan-Original-Tokens", fmt.Sprintf("%d", compressionStats.OriginalTokens))
				w.Header().Set("X-Lintasan-Compressed-Tokens", fmt.Sprintf("%d", compressionStats.CompressedTokens))
				w.Header().Set("X-Lintasan-Compression-Ratio", fmt.Sprintf("%.2f", compressionStats.CompressionRatio))
				w.Header().Set("X-Lintasan-Messages-Before", fmt.Sprintf("%d", compressionStats.MessagesBefore))
				w.Header().Set("X-Lintasan-Messages-After", fmt.Sprintf("%d", compressionStats.MessagesAfter))
			}
			var tokensIn, tokensOut int
			if ct := hedgeResp.Header.Get("x-tokens-input"); ct != "" {
				fmt.Sscanf(ct, "%d", &tokensIn)
			}
			if ct := hedgeResp.Header.Get("x-tokens-output"); ct != "" {
				fmt.Sscanf(ct, "%d", &tokensOut)
			}
			b, _ := io.ReadAll(hedgeResp.Body)
			if p.pm != nil {
				if transformedResp, err := p.pm.ExecuteResponseHook(r.Context(), hedgeConn.ID, resolvedModel, b); err == nil {
					b = transformedResp
				}
			}
			if hedgeConn.Format == "commandcode" {
				b = translateCCAlphaToOpenAI(b)
			}
			b = reasoning.ExtractReasoningContent(b)
			b = normalizeOpenAIResponseBody(b)
			if tokensOut == 0 {
				tokensOut = len(b) / 4
			}
			if tokensIn == 0 {
				tokensIn = len(body) / 4
			}
			if hedgeResp.StatusCode == 200 {
				quota.RecordQuota(p.db.Conn(), hedgeConn.ID, tokensIn+tokensOut)
			}
			w.WriteHeader(hedgeResp.StatusCode)
			w.Write(b)
			p.logRequest(resolvedModel, hedgeConn.ID, hedgeConn.Name, hedgeResp.StatusCode, time.Since(start).Milliseconds(), tokensIn, tokensOut, false, "", taskClass, modeLabel)
			if semanticEnabled && hedgeResp.StatusCode == 200 {
				cache.SaveSemanticMatch(p.db.Conn(), model, messages, string(b), 3600)
			}
			p.autoIndex(r, model, messages, string(b), tokensIn, tokensOut)
			return
		} else if hedgeErr != nil {
			w.Header().Set("X-Lintasan-Hedge", "miss")
		}
	}

	var lastErr string
	var lastStatusCode int
	for i, conn := range candidates {
		// Circuit breaker check
		breaker := p.getBreaker(conn.ID)
		if !breaker.Allow() {
			lastErr = fmt.Sprintf("circuit breaker open for %s", conn.ID)
			lastStatusCode = 503
			p.logRequest(resolvedModel, conn.ID, conn.Name, 503, time.Since(start).Milliseconds(), 0, 0, false, lastErr, taskClass, modeLabel)
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

		// Quota gating (Gap #3): skip a connection whose daily token budget is
		// already exhausted, BEFORE spending an upstream call on it. Allow()
		// returns true when the connection has no configured limit, so this is a
		// no-op until an operator sets a quota limit for the connection.
		// estTokens uses the real cl100k tokenizer (char/4 fallback) rather than
		// a raw byte/4 guess, so the gate matches actual usage more closely.
		estTokens := tokencount.CountMessages(messages)
		if estTokens == 0 {
			estTokens = len(body) / 4
		}
		if p.quota != nil && !p.quota.Allow(conn.ID, estTokens) {
			lastErr = fmt.Sprintf("quota exhausted for %s", conn.ID)
			lastStatusCode = 429
			p.logRequest(resolvedModel, conn.ID, conn.Name, 429, time.Since(start).Milliseconds(), 0, 0, false, lastErr, taskClass, modeLabel)
			if p.fb != nil {
				p.fb.RecordEvent(resolvedModel, "", fallback.Reason429, 429)
			}
			w.Header().Set("X-Lintasan-Quota-Skip", conn.ID)
			continue
		}

		// Retry wrapper around upstream call
		var resp *http.Response
		var poolAccountID string
		retryErr := retry.Do(r.Context(), retry.DefaultConfig(), func() (bool, error) {
			var err error
			// Multi-account pool: pick account and inject into a copy
			retryConn := conn
			if conn.PoolID != "" {
				var pickedKey string
				pickedKey, poolAccountID = p.pickMultiAccountAPIKey(conn.PoolID, retryConn.APIKey)
				cpy := *retryConn
				cpy.APIKey = pickedKey
				retryConn = &cpy
			}
			resp, err = p.doUpstream(r, retryConn, body)
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
			p.recordMultiAccountResult(conn.PoolID, poolAccountID, false, false)
			p.logRequest(resolvedModel, conn.ID, conn.Name, 502, time.Since(start).Milliseconds(), 0, 0, false, lastErr, taskClass, modeLabel)
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
			p.recordMultiAccountResult(conn.PoolID, poolAccountID, false, false)
			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), 0, 0, false, lastErr, taskClass, modeLabel)
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
			p.recordMultiAccountResult(conn.PoolID, poolAccountID, false, true)
			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), 0, 0, false, lastErr, taskClass, modeLabel)
			if p.fb != nil {
				p.fb.RecordEvent(resolvedModel, "", fallback.Reason429, resp.StatusCode)
			}
			continue
		}

		// Success! Record to breaker
		breaker.Success()
		p.recordMultiAccountResult(conn.PoolID, poolAccountID, true, false)

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
			// Skip hop-by-hop and transport headers that conflict with the
			// downstream response writer. Go's http.Transport auto-decompresses
			// gzip and clears Content-Length; forwarding a stale or zero value
			// causes the client to see an empty body even when w.Write() has
			// data. Content-Encoding is likewise handled by the transport.
			switch strings.ToLower(k) {
			case "content-length", "content-encoding", "transfer-encoding":
				continue
			}
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}

		// Add context compression headers if compression was applied
		if compressionStats.WasCompressed {
			w.Header().Set("X-Lintasan-Compressed", "true")
			w.Header().Set("X-Lintasan-Original-Tokens", fmt.Sprintf("%d", compressionStats.OriginalTokens))
			w.Header().Set("X-Lintasan-Compressed-Tokens", fmt.Sprintf("%d", compressionStats.CompressedTokens))
			w.Header().Set("X-Lintasan-Compression-Ratio", fmt.Sprintf("%.2f", compressionStats.CompressionRatio))
			w.Header().Set("X-Lintasan-Messages-Before", fmt.Sprintf("%d", compressionStats.MessagesBefore))
			w.Header().Set("X-Lintasan-Messages-After", fmt.Sprintf("%d", compressionStats.MessagesAfter))
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

			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), tokensIn, tokensOut, false, "", taskClass, modeLabel)

			if comboName != "" && resp.StatusCode == 200 {
				p.cmb.RecordSuccess(comboName)
				if combo.AutoAliasExists(comboName) {
					combo.RecordAutoSuccess(comboName, conn.ID)
				}
			}

			if semanticEnabled && resp.StatusCode == 200 {
				cache.SaveSemanticMatch(p.db.Conn(), model, messages, string(streamBuffer), 3600)
			}

			// Auto-Indexing: embed and store completion if header set
			p.autoIndex(r, model, messages, string(streamBuffer), tokensIn, tokensOut)

			// Plugin post-response hook (stream already sent, side-effects only)
			if p.pm != nil {
				p.pm.ExecuteResponseHook(r.Context(), conn.ID, resolvedModel, streamBuffer)
			}

			return
		}

		b, _ := io.ReadAll(resp.Body)

		// Plugin post-response hook: transform response body
		if p.pm != nil {
			if transformedResp, err := p.pm.ExecuteResponseHook(r.Context(), conn.ID, resolvedModel, b); err != nil {
				fmt.Fprintf(os.Stderr, "plugin post-response error: %v\n", err)
			} else {
				b = transformedResp
			}
		}

		// Translate CC Alpha SSE → OpenAI JSON
		if conn.Format == "commandcode" {
			b = translateCCAlphaToOpenAI(b)
		}

		// Reasoning extraction: DeepSeek V4 Pro puts answer in reasoning_content not content
		b = reasoning.ExtractReasoningContent(b)
		b = normalizeOpenAIResponseBody(b)

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
		p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), tokensIn, tokensOut, false, "", taskClass, modeLabel)

		if comboName != "" && resp.StatusCode == 200 {
			p.cmb.RecordSuccess(comboName)
			if combo.AutoAliasExists(comboName) {
				combo.RecordAutoSuccess(comboName, conn.ID)
			}
		}

		if p.wm != nil {
			p.wm.Fire("request.success", map[string]interface{}{
				"model":  resolvedModel,
				"status": resp.StatusCode,
			})
		}

		if semanticEnabled && resp.StatusCode == 200 {
			cache.SaveSemanticMatch(p.db.Conn(), model, messages, string(b), 3600)
		}

		// Auto-Indexing: embed and store completion if header set
		p.autoIndex(r, model, messages, string(b), tokensIn, tokensOut)

		return
	}

	if p.wm != nil {
		p.wm.Fire("request.error", map[string]interface{}{
			"model": model,
			"error": lastErr,
		})
	}

	// Use the last established status code from the fallback chain rather than
	// always returning 502. If a specific route returned 429 (rate limit), 503
	// (circuit breaker), or another meaningful code, propagate it. Fall back to
	// 502 when lastStatusCode is 0 (no route was tried at all).
	errCode := lastStatusCode
	if errCode == 0 {
		errCode = http.StatusBadGateway
	}
	http.Error(w, fmt.Sprintf(`{"error":{"message":"all routes failed","details":%q}}`, lastErr), errCode)
}

func (p *ProxyHandler) findConnectionByID(id string) (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT id, name, base_url, api_key, COALESCE(oauth_provider,''), format, chat_path, auth_header, auth_prefix, is_active, priority, COALESCE(pool_id,'')
		FROM connections
		WHERE id = ? AND is_active = 1
		LIMIT 1
	`, id)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.OAuthProvider, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority, &conn.PoolID)
	if err != nil {
		return nil, fmt.Errorf("connection not found: %s", id)
	}
	p.applyConnectionAuth(&conn)
	return &conn, nil
}

func (p *ProxyHandler) doUpstream(r *http.Request, conn *Connection, body []byte) (*http.Response, error) {
	conn = p.connForUpstream(conn)
	// --- Provider SDK seam (F1) ----------------------------------------------
	// When the kill-switch flag is on AND this is not a commandcode connection,
	// build the upstream request via the Provider SDK (Prepare-only). The HTTP
	// call itself stays here (p.client.Do), so reliability wrapping and the
	// streaming response path in the caller are untouched. Flag off => legacy
	// path below runs verbatim (bit-for-bit identical to pre-F1).
	if p.providerSDKEligible(conn) {
		upReq, err := p.buildUpstreamViaSDK(r.Context(), conn, body, r.Header)
		if err != nil {
			return nil, err
		}
		// X-Command-Code-Version passthrough is commandcode-only and is excluded
		// by providerSDKEligible, so it is intentionally not replayed here.
		upReq.Header.Set("Accept-Encoding", "identity") // prevent upstream gzip issues
		return p.client.Do(upReq)
	}

	// --- Legacy path (unchanged) ---------------------------------------------
	chatPath := conn.ChatPath
	if chatPath == "" {
		chatPath = "/v1/chat/completions"
	}
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
	if err != nil {
		return nil, err
	}
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept-Encoding", "identity") // prevent upstream from gzipping if body isn't actually compressed
	authHeader := conn.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authPrefix := conn.AuthPrefix
	if authPrefix == "" {
		authPrefix = "Bearer "
	}
	if conn.APIKey != "" {
		upReq.Header.Set(authHeader, authPrefix+conn.APIKey)
	}
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

	// Auto-combo (Gap #2): "auto", "auto/coding", "auto/fast", "auto/cheap"
	// resolve to the live connection ranked best-first by the 6-factor scorer
	// (health, quota, cost, latency, success, freshness). Falls through to the
	// normal path if no providers can be assembled.
	if combo.AutoAliasExists(model) {
		if conns, resolved, ok := p.resolveAutoCombo(model); ok && len(conns) > 0 {
			return conns, resolved, model, nil
		}
	}

	// Tiered fallback (Gap #5): the "tiered" alias orders candidates by tier —
	// subscription → api_key → cheap → free — and auto-downgrades when a tier is
	// exhausted. Built from live providers grouped by cost.
	if model == "tiered" || model == "tier" {
		if conns, resolved, ok := p.resolveTieredCombo(); ok && len(conns) > 0 {
			return conns, resolved, model, nil
		}
	}

	if conns, resolved, ok := p.resolveCombo(model); ok && len(conns) > 0 {
		return conns, resolved, model, nil
	}
	conn, err := p.findConnectionForModel(model)
	if err != nil {
		return nil, model, "", err
	}
	candidates := []*Connection{conn}

	// Model-level fallback (Gap #6): append connections that serve the
	// configured fallback models for this model. When every connection for the
	// primary model fails, the request loop continues onto these instead of
	// giving up — fallback across MODELS, not just across connections of the
	// same model. Deduplicated by connection ID.
	candidates = p.appendModelFallbacks(candidates, model)
	// Deduplicate by pool: keep only one connection per pool_id
	candidates = p.dedupCandidatesByPool(candidates)
	return candidates, model, "", nil
}

// dedupCandidatesByPool returns a deduplicated slice where at most one
// connection per non-empty pool_id is kept (any extra connections from
// the same pool are dropped). Connections with empty pool_id are kept.
func (p *ProxyHandler) dedupCandidatesByPool(candidates []*Connection) []*Connection {
	seenPools := make(map[string]bool)
	out := make([]*Connection, 0, len(candidates))
	for _, c := range candidates {
		if c == nil {
			continue
		}
		if c.PoolID != "" {
			if seenPools[c.PoolID] {
				continue // already have a connection from this pool
			}
			seenPools[c.PoolID] = true
		}
		out = append(out, c)
	}
	return out
}

// appendModelFallbacks expands a candidate list with connections serving the
// fallback models configured for `model` (via the fallback engine's model
// chains). Existing connection IDs are not duplicated.
func (p *ProxyHandler) appendModelFallbacks(candidates []*Connection, model string) []*Connection {
	if p.fb == nil {
		return candidates
	}
	fallbackModels := p.fb.GetModelFallback(model)
	if len(fallbackModels) == 0 {
		return candidates
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		seen[c.ID] = true
	}
	for _, fm := range fallbackModels {
		if fm == "" || fm == model {
			continue
		}
		for _, c := range p.connectionsForModelAndIDs(fm, nil) {
			if c == nil || seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			candidates = append(candidates, c)
		}
	}
	return candidates
}

// resolveAutoCombo builds a Provider list from all active connections that
// expose at least one model, scores them with combo.ResolveAuto for the given
// auto mode, and returns the connections ordered best-first. The resolved model
// is the top provider's model so the upstream request targets a concrete model.
func (p *ProxyHandler) resolveAutoCombo(mode string) ([]*Connection, string, bool) {
	providers, connByID, modelByID := p.buildAutoProviders()
	if len(providers) == 0 {
		return nil, mode, false
	}
	ranked := combo.ResolveAuto(mode, providers)
	if len(ranked) == 0 {
		return nil, mode, false
	}
	var out []*Connection
	for _, pr := range ranked {
		if c, ok := connByID[pr.ID]; ok {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, mode, false
	}
	resolvedModel := modelByID[ranked[0].ID]
	if resolvedModel == "" {
		resolvedModel = ranked[0].Model
	}
	return out, resolvedModel, true
}

// resolveTieredCombo orders candidates across tiers (subscription → api_key →
// cheap → free) using combo.AutoBuildCombo, which groups live providers by
// cost-per-token. The current (highest-priority non-exhausted) tier's providers
// come first, then remaining tiers as fallback. This gives the request loop a
// tier-ordered candidate list so it naturally downgrades on failure.
func (p *ProxyHandler) resolveTieredCombo() ([]*Connection, string, bool) {
	providers, connByID, modelByID := p.buildAutoProviders()
	if len(providers) == 0 {
		return nil, "tiered", false
	}
	tc := combo.AutoBuildCombo(providers)
	if tc == nil {
		return nil, "tiered", false
	}

	// Current active tier first.
	ordered := tc.Resolve()
	// Then append providers from any remaining tiers as fallback, deduped.
	seen := map[string]bool{}
	var out []*Connection
	appendProvider := func(pr combo.Provider) {
		if seen[pr.ID] {
			return
		}
		if c, ok := connByID[pr.ID]; ok {
			seen[pr.ID] = true
			out = append(out, c)
		}
	}
	for _, pr := range ordered {
		appendProvider(pr)
	}
	// Walk through remaining tiers for fallback coverage.
	for _, pr := range providers {
		appendProvider(pr)
	}
	if len(out) == 0 {
		return nil, "tiered", false
	}
	resolvedModel := modelByID[out[0].ID]
	return out, resolvedModel, true
}

// buildAutoProviders enumerates active connections (with a usable model) into
// combo.Provider structs enriched with live quality/latency metrics, plus
// lookup maps from provider ID back to the connection and its chosen model.
func (p *ProxyHandler) buildAutoProviders() ([]combo.Provider, map[string]*Connection, map[string]string) {
	rows, err := p.db.Conn().Query(`
		SELECT c.id, c.name, c.base_url, c.api_key, COALESCE(c.oauth_provider,''), c.format, c.chat_path,
		       c.auth_header, c.auth_prefix, c.is_active, c.priority,
		       (SELECT m.model_id FROM discovered_models m
		         WHERE m.connection_id = c.id AND m.is_active = 1
		         ORDER BY m.model_id LIMIT 1) AS model_id
		FROM connections c
		WHERE c.is_active = 1
		ORDER BY c.priority DESC`)
	if err != nil {
		return nil, nil, nil
	}
	defer rows.Close()

	var providers []combo.Provider
	connByID := map[string]*Connection{}
	modelByID := map[string]string{}

	p.qualityMu.RLock()
	defer p.qualityMu.RUnlock()

	for rows.Next() {
		var c Connection
		var modelID sql.NullString
		if rows.Scan(&c.ID, &c.Name, &c.BaseURL, &c.APIKey, &c.OAuthProvider, &c.Format, &c.ChatPath,
			&c.AuthHeader, &c.AuthPrefix, &c.IsActive, &c.Priority, &modelID) != nil {
			continue
		}
		p.applyConnectionAuth(&c)
		if !modelID.Valid || modelID.String == "" {
			continue // no usable model on this connection
		}
		cc := c
		connByID[c.ID] = &cc
		modelByID[c.ID] = modelID.String

		// Live metrics: default to neutral, override from the quality feedback loop.
		health := 80
		successRate := 0.8
		latency := 500.0
		var lastUsed time.Time
		if q, ok := p.qualityScores[c.ID]; ok {
			successRate = q.SuccessEWMA
			health = int(q.SuccessEWMA * 100)
			if q.LatencyEWMA > 0 {
				latency = q.LatencyEWMA
			}
			lastUsed = q.UpdatedAt
		}
		// Quota remaining as a 0-1 fraction; 1.0 (full) when no limit configured.
		quotaRemaining := p.quotaRemainingFraction(c.ID)
		// Cost hint: "free"-named connections are treated as zero cost.
		costPerToken := 1.0
		if strings.Contains(strings.ToLower(c.Name), "free") {
			costPerToken = 0.0
		}

		providers = append(providers, combo.Provider{
			ID:             c.ID,
			Name:           c.Name,
			Model:          modelID.String,
			ConnectionID:   c.ID,
			APIKey:         c.APIKey,
			Health:         health,
			QuotaRemaining: quotaRemaining,
			CostPerToken:   costPerToken,
			Latency:        latency,
			SuccessRate:    successRate,
			LastUsed:       lastUsed,
		})
	}
	return providers, connByID, modelByID
}

// quotaRemainingFraction returns how much of a connection's daily token budget
// is left, as a 0-1 fraction. Returns 1.0 when no limit is configured.
func (p *ProxyHandler) quotaRemainingFraction(connID string) float64 {
	if p.quota == nil {
		return 1.0
	}
	limit := p.quota.GetLimit(connID)
	if limit == nil || limit.MaxTokensPerDay <= 0 {
		return 1.0
	}
	usage := quota.GetQuota(p.db.Conn(), connID)
	tokensToday, _ := usage["tokens_today"].(int)
	remaining := float64(limit.MaxTokensPerDay-int64(tokensToday)) / float64(limit.MaxTokensPerDay)
	if remaining < 0 {
		return 0
	}
	if remaining > 1 {
		return 1
	}
	return remaining
}

func (p *ProxyHandler) resolveAlias(model string) string {
	v, _ := p.db.GetSetting("aliases")
	if v == "" {
		return model
	}

	// Try array format: [{name: "X", target: "Y"}, ...]
	var arr []any
	if json.Unmarshal([]byte(v), &arr) == nil {
		for _, item := range arr {
			m, _ := item.(map[string]any)
			if m == nil {
				continue
			}
			name, _ := m["name"].(string)
			alias, _ := m["alias"].(string)
			if name == model || alias == model {
				if target, _ := m["target"].(string); target != "" {
					return target
				}
				if mod, _ := m["model"].(string); mod != "" {
					return mod
				}
			}
		}
		return model
	}

	// Try map format: {"alias": {"model": "..."}, ...}
	var m map[string]any
	if json.Unmarshal([]byte(v), &m) == nil {
		if raw, ok := m[model]; ok {
			switch t := raw.(type) {
			case string:
				return t
			case map[string]any:
				if s, _ := t["model"].(string); s != "" {
					return s
				}
			}
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

func stringSlice(v any) []string {
	arr, _ := v.([]any)
	out := []string{}
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (p *ProxyHandler) connectionsForModelAndIDs(model string, ids []string) []*Connection {
	query := `SELECT c.id, c.name, c.base_url, c.api_key, COALESCE(c.oauth_provider,''), c.format, c.chat_path, c.auth_header, c.auth_prefix, c.is_active, c.priority FROM discovered_models m JOIN connections c ON m.connection_id=c.id WHERE m.model_id=? AND m.is_active=1 AND c.is_active=1`
	args := []any{model}
	if len(ids) > 0 {
		ph := make([]string, len(ids))
		for i, id := range ids {
			ph[i] = "?"
			args = append(args, id)
		}
		query += " AND c.id IN (" + strings.Join(ph, ",") + ")"
	}
	query += " ORDER BY c.priority DESC"
	rows, err := p.db.Conn().Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Connection
	for rows.Next() {
		var c Connection
		if rows.Scan(&c.ID, &c.Name, &c.BaseURL, &c.APIKey, &c.OAuthProvider, &c.Format, &c.ChatPath, &c.AuthHeader, &c.AuthPrefix, &c.IsActive, &c.Priority) == nil {
			p.applyConnectionAuth(&c)
			out = append(out, &c)
		}
	}
	return out
}

func (p *ProxyHandler) findConnectionForModel(model string) (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT c.id, c.name, c.base_url, c.api_key, COALESCE(c.oauth_provider,''), c.format, c.chat_path, c.auth_header, c.auth_prefix, c.is_active, c.priority, COALESCE(c.pool_id,'')
		FROM discovered_models m
		JOIN connections c ON m.connection_id = c.id
		WHERE m.model_id = ? AND m.is_active = 1 AND c.is_active = 1
		ORDER BY c.priority DESC
		LIMIT 1
	`, model)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.OAuthProvider, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority, &conn.PoolID)
	if err != nil {
		return nil, fmt.Errorf("model not found: %s", model)
	}
	p.applyConnectionAuth(&conn)
	return &conn, nil
}

func (p *ProxyHandler) getFirstConnection() (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT id, name, base_url, api_key, COALESCE(oauth_provider,''), format, chat_path, auth_header, auth_prefix, is_active, priority, COALESCE(pool_id,'')
		FROM connections
		WHERE is_active = 1
		ORDER BY priority DESC
		LIMIT 1
	`)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.OAuthProvider, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority, &conn.PoolID)
	if err != nil {
		return nil, fmt.Errorf("no active connections")
	}
	p.applyConnectionAuth(&conn)
	return &conn, nil
}

func (p *ProxyHandler) logRequest(model, connID, provider string, status int, latencyMs int64, tokensIn, tokensOut int, cached bool, errMsg, taskClass, mode string) {
	cachedInt := 0
	if cached {
		cachedInt = 1
	}
	id := uuid.New().String()
	p.db.Conn().Exec(`
		INSERT INTO request_logs (id, connection_id, provider, model, status, input_tokens, output_tokens, latency_ms, cached, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', 'localtime'))
	`, id, connID, provider, model, status, tokensIn, tokensOut, latencyMs, cachedInt, errMsg)
	if p.telemetry != nil {
		p.telemetry.Observe(provider, taskClass, mode, latencyMs, status, cached)
	}
	if connID != "" {
		p.observeQuality(connID, status, latencyMs)
	}
}

func (p *ProxyHandler) HandleEmbeddings(w http.ResponseWriter, r *http.Request) {
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
		model = "text-embedding-3-small"
	}

	conn, err := p.findConnectionForModel(model)
	if err != nil {
		conn, err = p.getFirstConnection()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"no connection found for model %s"}`, model), http.StatusNotFound)
			return
		}
	}

	var upReq *http.Request
	if p.embedderSDK {
		// F2.5 SDK path: build the upstream request via the provider Embedder.
		// This produces a byte-for-byte identical request to the inline path
		// below (same URL, POST, Content-Type, faithful auth, body passthrough).
		upReq, err = p.buildEmbeddingsViaSDK(r.Context(), conn, body)
		if err != nil {
			http.Error(w, `{"error":"failed to create upstream request"}`, http.StatusInternalServerError)
			return
		}
	} else {
		// Legacy inline path (UNCHANGED): default OFF, zero behavior change.
		upstreamURL := strings.TrimRight(conn.BaseURL, "/") + "/v1/embeddings"
		upReq, err = http.NewRequestWithContext(r.Context(), "POST", upstreamURL, strings.NewReader(string(body)))
		if err != nil {
			http.Error(w, `{"error":"failed to create upstream request"}`, http.StatusInternalServerError)
			return
		}

		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("Accept-Encoding", "identity")
		authHeader := conn.AuthHeader
		if authHeader == "" {
			authHeader = "Authorization"
		}
		authPrefix := conn.AuthPrefix
		if authPrefix == "" {
			authPrefix = "Bearer "
		}
		if conn.APIKey != "" {
			upReq.Header.Set(authHeader, authPrefix+conn.APIKey)
		}
	}

	resp, err := p.client.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"upstream error: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		// Skip hop-by-hop and transport headers that conflict with the
		// downstream response writer. Go's http.Transport auto-decompresses
		// gzip and clears Content-Length; forwarding a stale or zero value
		// causes the client to see an empty body even when w.Write() has
		// data. Content-Encoding is likewise handled by the transport.
		switch strings.ToLower(k) {
		case "content-length", "content-encoding", "transfer-encoding":
			continue
		}
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *ProxyHandler) proxyPath(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	body, _ := io.ReadAll(r.Body)
	var reqBody io.Reader = bytes.NewReader(body)
	conn, err := p.getFirstConnection()
	if err != nil {
		http.Error(w, `{"error":"no active connections"}`, 404)
		return
	}
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", strings.TrimRight(conn.BaseURL, "/")+upstreamPath, reqBody)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	upReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if upReq.Header.Get("Content-Type") == "" {
		upReq.Header.Set("Content-Type", "application/json")
	}
	upReq.Header.Set("Accept-Encoding", "identity")
	if conn.APIKey != "" {
		h := conn.AuthHeader
		if h == "" {
			h = "Authorization"
		}
		pfx := conn.AuthPrefix
		if pfx == "" {
			pfx = "Bearer "
		}
		upReq.Header.Set(h, pfx+conn.APIKey)
	}
	resp, err := p.client.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"upstream error: %s"}`, err.Error()), 502)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		// Skip hop-by-hop and transport headers that conflict with the
		// downstream response writer. Go's http.Transport auto-decompresses
		// gzip and clears Content-Length; forwarding a stale or zero value
		// causes the client to see an empty body even when w.Write() has
		// data. Content-Encoding is likewise handled by the transport.
		switch strings.ToLower(k) {
		case "content-length", "content-encoding", "transfer-encoding":
			continue
		}
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *ProxyHandler) HandleImages(w http.ResponseWriter, r *http.Request) {
	p.proxyPath(w, r, "/v1/images/generations")
}
func (p *ProxyHandler) HandleAudioSpeech(w http.ResponseWriter, r *http.Request) {
	p.proxyPath(w, r, "/v1/audio/speech")
}
func (p *ProxyHandler) HandleAudioTranscriptions(w http.ResponseWriter, r *http.Request) {
	p.proxyPath(w, r, "/v1/audio/transcriptions")
}

// buildPromptText extracts the user-facing text from chat messages for memory embedding.
func buildPromptText(messages []any) string {
	var b strings.Builder
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if content != "" && (role == "user" || role == "system") {
			b.WriteString(content)
			b.WriteString(" ")
		}
	}
	return strings.TrimSpace(b.String())
}

// truncate shortens text to maxLen characters.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// handleReflectLoop runs the self-review auto-fix loop triggered by X-Lintasan-Reflect header.
// Must be called BEFORE any upstream response is written — it owns the full request flow.
func (p *ProxyHandler) handleReflectLoop(r *http.Request, model string, messages []any, maxTokens int, w http.ResponseWriter) {
	reflectHeader := r.Header.Get("X-Lintasan-Reflect")
	maxIter := 3
	if n, err := strconv.Atoi(reflectHeader); err == nil && n >= 1 && n <= 5 {
		maxIter = n
	}

	verifyRaw := r.Header.Get("X-Lintasan-Reflect-Verify")
	if verifyRaw == "" {
		verifyRaw = "text"
	}

	// Parse verify mode: "pytest:/tmp/test_X.py:/tmp/buggy_X.py"
	var verifyMode string
	var testFile, codeModule string
	if strings.HasPrefix(verifyRaw, "pytest:") {
		verifyMode = "pytest"
		parts := strings.Split(verifyRaw, ":")
		if len(parts) >= 3 {
			testFile = parts[1]
			codeModule = parts[2]
		} else {
			testFile = "/tmp/reflect_test.py"
			codeModule = "/tmp/reflect_module.py"
		}
	} else {
		verifyMode = verifyRaw
	}

	// Build initial prompt from the last user message
	initialPrompt := ""
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "user" {
			if content, ok := msg["content"].(string); ok {
				initialPrompt = content
			}
			break
		}
	}
	if initialPrompt == "" {
		http.Error(w, `{"error":"no user message found for reflect loop"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Create LLM generator that resolves connection + calls upstream with format translation
	generator := func(prompt string, prevErrors []string) (string, error) {
		conn, err := p.findConnectionForModel(model)
		if err != nil {
			return "", fmt.Errorf("no connection for model %s: %w", model, err)
		}

		perIterTokens := maxTokens / maxIter
		if perIterTokens < 256 {
			perIterTokens = 256
		}

		reqBody := map[string]any{
			"model": model,
			"messages": []map[string]any{
				{"role": "user", "content": prompt},
			},
			"max_tokens": float64(perIterTokens),
			"stream":     false,
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return "", err
		}

		resp, err := p.doUpstream(r, conn, bodyBytes)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		// CC Alpha format translation
		if conn.Format == "commandcode" {
			respBytes = translateCCAlphaToOpenAI(respBytes)
		}

		// Extract text from OpenAI response
		var respObj map[string]any
		if err := json.Unmarshal(respBytes, &respObj); err != nil {
			return string(respBytes), nil
		}
		if choices, ok := respObj["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if msg, ok := choice["message"].(map[string]any); ok {
					if content, ok := msg["content"].(string); ok {
						return content, nil
					}
					// DeepSeek: content might be empty, reasoning_content has answer
					if rc, ok := msg["reasoning_content"].(string); ok && rc != "" {
						return rc, nil
					}
				}
			}
		}
		return string(respBytes), nil
	}

	// Create verifier
	var verifier reflect.Verifier
	switch verifyMode {
	case "pytest":
		verifier = reflect.NewPytestVerifier(testFile, codeModule)
	case "syntax":
		verifier = reflect.NewSyntaxVerifier()
	default:
		// Text verifier: just checks response is non-empty
		verifier = func(output string) reflect.VerifyResult {
			if output == "" {
				return reflect.VerifyResult{Score: 0, Errors: []string{"empty response"}}
			}
			return reflect.VerifyResult{Score: 1.0, Passed: 1, Total: 1}
		}
	}

	result, err := reflect.Reflect(ctx, maxIter, generator, verifier, initialPrompt, true)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"reflect loop failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Build OpenAI-compatible response with best result
	resp := map[string]any{
		"id":      "reflect-" + uuid.New().String()[:8],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": result.BestResponse,
				},
				"finish_reason": "stop",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Lintasan-Reflect-Iterations", fmt.Sprintf("%d", result.Iterations))
	w.Header().Set("X-Lintasan-Reflect-Score", fmt.Sprintf("%.0f", result.BestScore*100))
	json.NewEncoder(w).Encode(resp)

	fmt.Fprintf(os.Stderr, "reflect: %d iterations, best=%.0f%%, duration=%v\n",
		result.Iterations, result.BestScore*100, result.Duration)
}

// autoIndex stores a completion in vector memory when X-Lintasan-Index header is "true".
func (p *ProxyHandler) autoIndex(r *http.Request, model string, messages []any, response string, promptTokens, completionTokens int) {
	if r.Header.Get("X-Lintasan-Index") != "true" {
		return
	}
	if p.mem == nil || !p.mem.Available() {
		return
	}

	// Build prompt from messages
	prompt := memory.Prompt{Model: model}
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		prompt.Messages = append(prompt.Messages, memory.Message{Role: role, Content: content})
	}

	tags := []string{model, time.Now().UTC().Format("2006-01-02")}
	_, _, err := p.mem.Store.IndexCompletion(prompt, response, 0, tags, promptTokens, completionTokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memory: auto-index failed: %v\n", err)
	}
}

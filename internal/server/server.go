package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/discover"
	"github.com/sanhaji182/lintasan-go/internal/freeproviders"
	"github.com/sanhaji182/lintasan-go/internal/mcp"
	"github.com/sanhaji182/lintasan-go/internal/metrics"
	"github.com/sanhaji182/lintasan-go/internal/mitm"
	"github.com/sanhaji182/lintasan-go/internal/plugin"
	"github.com/sanhaji182/lintasan-go/internal/rtk"
	"github.com/sanhaji182/lintasan-go/internal/websearch"
)

type Server struct {
	cfg         *config.Config
	db          *db.DB
	mux         *http.ServeMux
	proxy       *ProxyHandler
	memHandler  *MemoryHandler         // vector memory API handler
	mitmProxy   *mitm.MITMProxy        // MITM bridge for IDE interception
	oauthMgr    *auth.OAuthManager     // OAuth session manager
	userMgr     *auth.UserManager      // Dashboard user manager
	authHandler *auth.AuthHandler      // HTTP auth handlers
	pluginMgr   *plugin.Manager        // JS plugin engine (also in proxy.pm)
	discoverer  *discover.Discoverer   // auto model discovery
	fpScanner   *freeproviders.Scanner // free provider scanner
	rtkComp     *rtk.Compressor        // RTK token compressor
	webSearch   *websearch.Engine      // web search engine
	mcpServer   *mcp.Server            // MCP protocol server
	mitmOnce    sync.Once              // ensures MITM starts exactly once
	mitmSecret  string                 // random per-boot MITM bypass secret (empty = disabled)
	setup       setupState             // bootstrap/active one-way latch
	metrics     *metrics.Registry      // Prometheus metrics registry (/metrics)
}

func New(cfg *config.Config, database *db.DB) *Server {
	s := &Server{
		cfg: cfg,
		db:  database,
		mux: http.NewServeMux(),
		metrics: metrics.NewRegistry(),
	}
	// Register pull-based metric collectors. These run on every /metrics scrape
	// and emit only numeric counters/gauges + bounded labels — no secrets.
	s.metrics.RegisterCollector(buildInfoCollector)
	s.metrics.RegisterCollector(memorySearchCollector)
	s.metrics.RegisterCollector(cacheCollector)
	s.metrics.RegisterCollector(metrics.RuntimeCollector)
	s.proxy = NewProxyHandler(cfg, database)
	s.memHandler = NewMemoryHandler(s.proxy.mem)

	// Wire OAuth manager (reuse the one from proxy or create standalone)
	s.oauthMgr = auth.NewOAuthManager(database)

	// Wire dashboard auth (JWT token-based)
	jwtSecret := os.Getenv("LINTASAN_JWT_SECRET")
	if jwtSecret == "" {
		// Generate a random secret if not set (survives restarts because stored in DB)
		jwtSecret, _ = database.GetSetting("jwt_secret")
		if jwtSecret == "" {
			jwtSecret = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("lintasan-%d", time.Now().UnixNano()))))
			database.SetSetting("jwt_secret", jwtSecret)
		}
	}
	s.userMgr = auth.NewUserManager(database.Conn(), jwtSecret)
	s.authHandler = auth.NewAuthHandler(s.userMgr)

	// Seed default admin account if no users exist. The password is RANDOM
	// (never hardcoded) and the account is flagged must_change_password. The
	// generated password is printed once to stderr for first-run setup only.
	if pw, err := s.userMgr.SeedAdmin("admin"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: admin seed failed: %v\n", err)
	} else if pw != "" {
		fmt.Fprintf(os.Stderr, "\n========================================================\n"+
			"  FIRST-RUN: seeded admin account\n"+
			"    username: admin\n"+
			"    password: %s\n"+
			"  You MUST change this password on first login.\n"+
			"========================================================\n\n", pw)
	}

	// Wire plugin manager (shared with proxy so both have access)
	s.pluginMgr = s.proxy.pm

	// Wire model discoverer
	s.discoverer = discover.NewDiscoverer(database)

	// Wire free provider scanner
	s.fpScanner = freeproviders.New(database.Conn())

	// Wire RTK compressor with default config
	s.rtkComp = rtk.New(rtk.DefaultConfig())

	// Wire web search engine (SerpAPI key from settings)
	serpKey, _ := database.GetSetting("serpapi_key")
	s.webSearch = websearch.New(serpKey)

	// Wire MCP server with all tools
	s.mcpServer = mcp.NewServer("lintasan", "2.2.0")
	mcp.RegisterAllTools(s.mcpServer, database.Conn())

	// Wire MITM proxy ONLY when explicitly enabled. Default is disabled.
	// When enabled, generate a random per-boot bypass secret — there is no
	// static, source-guessable bypass value anymore.
	if cfg.MITMEnabled {
		secret := make([]byte, 24)
		if _, err := rand.Read(secret); err == nil {
			s.mitmSecret = base64.RawURLEncoding.EncodeToString(secret)
		}
		s.mitmProxy = mitm.New(cfg.MITMPort, cfg.Port, database, s.mitmSecret)
		// Persist so a standalone `lintasan mitm start` bridge can read the
		// same per-boot secret. Rotated on every server boot.
		database.SetSetting("mitm_secret", s.mitmSecret)
		fmt.Fprintf(os.Stderr, "⚠️  MITM bridge ENABLED on :%d (per-boot bypass secret active; IDE bridge only — do NOT expose publicly)\n", cfg.MITMPort)
	}

	s.routes()
	return s
}

func (s *Server) Start() error {
	// Start MITM bridge if configured
	if s.mitmProxy != nil {
		go func() {
			fmt.Printf("🔒 MITM bridge listening on :%d → forwarding to Lintasan on :%d\n", s.mitmProxy.GetListenPort(), s.cfg.Port)
			if err := s.mitmProxy.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "MITM proxy error: %v\n", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Port),
		Handler:      s.corsMiddleware(s.metricsMiddleware(s.authMiddleware(s.mux))),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long for streaming
		IdleTimeout:  120 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) routes() {
	// Register Node.js feature-parity management routes
	s.registerParityRoutes()

	// Register RESTful resource routes (DELETE/PATCH /{id} endpoints the
	// SvelteKit dashboard calls — previously unregistered, causing silent 405s)
	s.registerRESTRoutes()

	// Register OAuth routes
	s.registerOAuthRoutes()

	// Register Auth routes (JWT-based dashboard login)
	s.mux.HandleFunc("POST /api/auth/login", s.authHandler.HandleLogin())
	s.mux.HandleFunc("GET /api/auth/me", s.authHandler.HandleMe())
	s.mux.HandleFunc("POST /api/auth/logout", s.authHandler.HandleLogout())
	s.mux.HandleFunc("POST /api/auth/change-password", s.authHandler.HandleChangePassword())
	s.mux.HandleFunc("GET /api/auth/users", s.authHandler.HandleListUsers())
	s.mux.HandleFunc("POST /api/auth/users", s.authHandler.HandleCreateUser())

	// First-run setup (bootstrap/active state machine)
	s.mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("POST /api/setup", s.handleSetupComplete)

	// Health
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Prometheus metrics (read-only counters/gauges; gated by
	// LINTASAN_METRICS_ENABLED, default on). No secrets, bounded labels.
	s.mux.HandleFunc("GET /metrics", s.HandleMetrics)

	// OpenAI-compatible API
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	s.mux.HandleFunc("GET /api/models/catalog", s.handleModelsCatalog)
	s.mux.HandleFunc("POST /v1/chat/completions", s.proxy.HandleChatCompletions)
	s.mux.HandleFunc("POST /v1/embeddings", s.proxy.HandleEmbeddings)

	// Free Provider Scanner (P10c)
	s.mux.HandleFunc("GET /api/providers/discover", s.handleProviderDiscover)

	// Web Search (P10c)
	s.mux.HandleFunc("POST /v1/web/search", s.handleWebSearch)

	// Vector Memory API
	s.mux.HandleFunc("GET /v1/memory/search", s.memHandler.HandleMemorySearch)
	s.mux.HandleFunc("POST /v1/memory", s.memHandler.HandleMemoryStore)
	s.mux.HandleFunc("GET /v1/memory/stats", s.memHandler.HandleMemoryStats)
	s.mux.HandleFunc("DELETE /v1/memory/{key}", s.memHandler.HandleMemoryDelete)
	s.mux.HandleFunc("GET /v1/memory", s.memHandler.HandleMemoryList)

	// MCP Protocol (JSON-RPC 2.0)
	s.mux.HandleFunc("POST /mcp", s.mcpServer.HandleHTTP)
	s.mux.HandleFunc("GET /mcp/sse", s.mcpServer.HandleSSE)
	s.mux.HandleFunc("GET /api/mcp/tools", s.handleMCPTools)

	// Cost Savings API
	s.mux.HandleFunc("GET /api/savings/summary", s.handleSavingsSummary)
	s.mux.HandleFunc("GET /api/savings/history", s.handleSavingsHistory)

	// Translator API
	s.mux.HandleFunc("POST /api/translate", s.handleTranslate)
	s.mux.HandleFunc("GET /api/translate/formats", s.handleTranslateFormats)

	// OpenAI-compatible media endpoints (Node.js exposes these through API routes; Go exposes root /v1 too)
	s.mux.HandleFunc("POST /v1/images/generations", s.proxy.HandleImages)
	s.mux.HandleFunc("POST /v1/audio/speech", s.proxy.HandleAudioSpeech)
	s.mux.HandleFunc("POST /v1/audio/transcriptions", s.proxy.HandleAudioTranscriptions)

	// Management API
	s.mux.HandleFunc("GET /api/connections", s.handleGetConnections)
	s.mux.HandleFunc("POST /api/connections", s.handleCreateConnection)
	s.mux.HandleFunc("PATCH /api/connections", s.handlePatchConnection)
	s.mux.HandleFunc("DELETE /api/connections", s.handleDeleteConnection)
	s.mux.HandleFunc("DELETE /api/connections/{id}", s.handleDeleteConnection)
	s.mux.HandleFunc("GET /api/combos", s.handleGetCombos)
	s.mux.HandleFunc("POST /api/combos", s.handleCreateCombo)
	s.mux.HandleFunc("PUT /api/combos", s.handleUpdateCombo)
	s.mux.HandleFunc("DELETE /api/combos", s.handleDeleteCombo)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/logs", s.handleLogs)
	s.mux.HandleFunc("GET /api/analytics", s.handleAnalytics)
	s.mux.HandleFunc("GET /api/telemetry", s.handleTelemetry)
	s.mux.HandleFunc("GET /api/usage", s.handleUsage)
	s.mux.HandleFunc("GET /api/backup", s.handleBackup)
	s.mux.HandleFunc("POST /api/backup", s.handleBackupAction)
	s.mux.HandleFunc("GET /api/fallback", s.handleFallback)
	s.mux.HandleFunc("POST /api/fallback", s.handleFallbackAction)
	s.mux.HandleFunc("DELETE /api/fallback", s.handleFallbackDelete)
	s.mux.HandleFunc("GET /api/keys", s.handleKeys)
	s.mux.HandleFunc("POST /api/keys", s.handleKeysAction)
	s.mux.HandleFunc("GET /api/load-balancer", s.handleLoadBalancer)
	s.mux.HandleFunc("POST /api/load-balancer", s.handleLoadBalancerAction)
	s.mux.HandleFunc("GET /api/aliases", s.handleAliases)
	s.mux.HandleFunc("POST /api/aliases", s.handleAliasesAction)
	s.mux.HandleFunc("DELETE /api/aliases", s.handleAliasesDelete)
	s.mux.HandleFunc("GET /api/smart-routing", s.handleSmartRouting)
	s.mux.HandleFunc("POST /api/smart-routing", s.handleSmartRoutingAction)
	s.mux.HandleFunc("GET /api/plugins", s.handlePlugins)
	s.mux.HandleFunc("POST /api/plugins", s.handlePluginsAction)
	s.mux.HandleFunc("GET /api/plugins/store", s.handlePluginStore)
	s.mux.HandleFunc("POST /api/plugins/store", s.handlePluginStoreAction)
	s.mux.HandleFunc("POST /api/plugins/generate", s.handlePluginGenerate)
	s.mux.HandleFunc("GET /api/teams", s.handleTeams)
	s.mux.HandleFunc("POST /api/teams", s.handleTeamsAction)
	s.mux.HandleFunc("GET /api/users", s.handleUsers)
	s.mux.HandleFunc("POST /api/users", s.handleUsersAction)
	s.mux.HandleFunc("GET /api/webhooks", s.handleWebhooks)
	s.mux.HandleFunc("POST /api/webhooks", s.handleWebhooksAction)
	s.mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	s.mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)

	// Access Logs (in-memory ring buffer)
	s.mux.HandleFunc("GET /api/access-logs", s.handleAccessLogs)
	s.mux.HandleFunc("GET /api/access-logs/stats", s.handleAccessLogStats)

	// Guard — content safety check
	s.mux.HandleFunc("POST /api/guard/check", s.handleGuardCheck)

	// Free Provider Discovery
	s.mux.HandleFunc("GET /api/discover/free-providers", s.handleDiscoverFreeProviders)

	// Dashboard API + root redirect
	s.registerDashboardRoutes()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"version": "2.0.0",
		"uptime":  time.Since(startTime).String(),
	})
}

var startTime = time.Now()

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Lintasan-MITM")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestUser resolves the authenticated user from a request, checking the
// JWT cookie first then the Authorization: Bearer header. Returns nil if no
// valid user token is present. Does NOT consult master key / dashboard API keys.
func (s *Server) requestUser(r *http.Request) *auth.User {
	if s.userMgr == nil {
		return nil
	}
	if cookie, err := r.Cookie("lintasan_token"); err == nil && cookie.Value != "" {
		if user, err := s.userMgr.ValidateToken(cookie.Value); err == nil {
			return user
		}
	}
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		if user, err := s.userMgr.ValidateToken(strings.TrimPrefix(authHeader, "Bearer ")); err == nil {
			return user
		}
	}
	return nil
}

// authMiddleware enforces the bootstrap/active state machine and fail-CLOSED auth.
//
// Invariants (asserted by security_boundary_test.go):
//   - BOOTSTRAP: only setup-path endpoints are reachable; everything else → 503.
//   - ACTIVE:    no request reaches a management/proxy endpoint without a valid
//                JWT, master key, or dashboard API key. There is NO fail-open.
//   - There is no path-prefix whitelist (e.g. /api/dashboard/*) that bypasses auth.
//   - The MITM bypass requires a per-boot random secret, never a static value.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Health, root, metrics, and setup-status are always open (no secrets,
		// no mutation). /metrics serves read-only numeric counters (bounded
		// labels, no master_key / API keys / prompt content) so it's safe to
		// expose unauthenticated for a localhost Prometheus scraper, matching
		// /health. Exposure can still be turned off entirely via
		// LINTASAN_METRICS_ENABLED (handled in HandleMetrics). setup-status must
		// be readable in BOTH states so the login UI can render first-run vs
		// normal login.
		if path == "/health" || path == "/" || path == "/api/setup/status" || path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// --- BOOTSTRAP state: only setup endpoints are reachable. ---
		if !s.isActive() {
			if isSetupPath(path, r.Method) {
				// Attach user if a token is present (setup-complete needs admin).
				if user := s.requestUser(r); user != nil {
					next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, user)))
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			// Everything else is locked until setup completes. No fail-open.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "setup_required",
				"hint":  "complete first-run setup: log in as admin, rotate the password, and set a master key via POST /api/setup",
			})
			return
		}

		// --- ACTIVE state: fail-CLOSED. ---

		// Login is always reachable so users can obtain a token.
		if path == "/api/auth/login" && r.Method == http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		// MITM bridge bypass: ONLY with the per-boot random secret. Never a
		// static, source-guessable value. Disabled entirely if no secret set.
		if s.mitmSecret != "" {
			if hdr := r.Header.Get("X-Lintasan-MITM"); hdr != "" &&
				subtle.ConstantTimeCompare([]byte(hdr), []byte(s.mitmSecret)) == 1 {
				s.audit("mitm.bypass", "ide-bridge", path, map[string]any{"method": r.Method, "remote": r.RemoteAddr})
				next.ServeHTTP(w, r)
				return
			}
		}

		// 1) JWT (cookie or Bearer) — highest priority, carries user identity.
		if user := s.requestUser(r); user != nil {
			// Enforce password rotation: a user flagged must_change_password may
			// only reach the change-password endpoint until they rotate.
			if user.MustChangePassword &&
				!(path == "/api/auth/change-password" && r.Method == http.MethodPost) &&
				path != "/api/auth/me" && path != "/api/auth/logout" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "password_change_required",
					"hint":  "rotate your password via POST /api/auth/change-password",
				})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, user)))
			return
		}

		// 2) Master key (DB setting or config) via Bearer.
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if dbKey, _ := s.db.GetSetting("master_key"); dbKey != "" &&
				subtle.ConstantTimeCompare([]byte(token), []byte(dbKey)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			if s.cfg.MasterKey != "" &&
				subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.MasterKey)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			// 3) Dashboard-issued API keys.
			if s.validDashboardAPIKey(token) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Fail closed.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
	})
}

package server

import (
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
	"github.com/sanhaji182/lintasan-go/internal/mitm"
	"github.com/sanhaji182/lintasan-go/internal/plugin"
	"github.com/sanhaji182/lintasan-go/internal/rtk"
	"github.com/sanhaji182/lintasan-go/internal/websearch"
)

type Server struct {
	cfg        *config.Config
	db         *db.DB
	mux        *http.ServeMux
	proxy      *ProxyHandler
	memHandler *MemoryHandler         // vector memory API handler
	mitmProxy  *mitm.MITMProxy        // MITM bridge for IDE interception
	oauthMgr   *auth.OAuthManager     // OAuth session manager
	pluginMgr  *plugin.Manager        // JS plugin engine (also in proxy.pm)
	discoverer *discover.Discoverer   // auto model discovery
	fpScanner  *freeproviders.Scanner // free provider scanner
	rtkComp    *rtk.Compressor        // RTK token compressor
	webSearch  *websearch.Engine      // web search engine
	mcpServer  *mcp.Server            // MCP protocol server
	mitmOnce   sync.Once              // ensures MITM starts exactly once
}

func New(cfg *config.Config, database *db.DB) *Server {
	s := &Server{
		cfg: cfg,
		db:  database,
		mux: http.NewServeMux(),
	}
	s.proxy = NewProxyHandler(cfg, database)
	s.memHandler = NewMemoryHandler(s.proxy.mem)

	// Wire OAuth manager (reuse the one from proxy or create standalone)
	s.oauthMgr = auth.NewOAuthManager(database)

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

	// Wire MITM proxy if MITM_PORT env set
	if port := os.Getenv("MITM_PORT"); port != "" {
		s.mitmProxy = mitm.New(cfg.MITMPort, cfg.Port, database)
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
		Handler:      s.corsMiddleware(s.authMiddleware(s.mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // Long for streaming
		IdleTimeout:  120 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) routes() {
	// Register Node.js feature-parity management routes
	s.registerParityRoutes()

	// Register OAuth routes
	s.registerOAuthRoutes()

	// Health
	s.mux.HandleFunc("GET /health", s.handleHealth)

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

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health, dashboard, auth, oauth, and dashboard API
		if r.URL.Path == "/health" || r.URL.Path == "/" ||
			strings.HasPrefix(r.URL.Path, "/api/dashboard/") ||
			strings.HasPrefix(r.URL.Path, "/api/auth/") ||
			strings.HasPrefix(r.URL.Path, "/api/oauth/") {
			next.ServeHTTP(w, r)
			return
		}

		// MITM bridge bypass: requests from IDE proxy
		if r.Header.Get("X-Lintasan-MITM") == "true" {
			next.ServeHTTP(w, r)
			return
		}

		// Check master key
		masterKey, _ := s.db.GetSetting("master_key")
		if masterKey == "" {
			// No key set, allow all (first-run)
			next.ServeHTTP(w, r)
			return
		}

		// Validate Bearer token
		auth := r.Header.Get("Authorization")
		if auth == "Bearer "+masterKey {
			next.ServeHTTP(w, r)
			return
		}

		// Also check config master key
		if s.cfg.MasterKey != "" && auth == "Bearer "+s.cfg.MasterKey {
			next.ServeHTTP(w, r)
			return
		}

		// Check API keys created from dashboard (/api/keys)
		if strings.HasPrefix(auth, "Bearer ") && s.validDashboardAPIKey(strings.TrimPrefix(auth, "Bearer ")) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
	})
}

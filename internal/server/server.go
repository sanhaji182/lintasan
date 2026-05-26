package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/dashboard"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

type Server struct {
	cfg    *config.Config
	db     *db.DB
	mux    *http.ServeMux
	proxy  *ProxyHandler
}

func New(cfg *config.Config, database *db.DB) *Server {
	s := &Server{
		cfg: cfg,
		db:  database,
		mux: http.NewServeMux(),
	}
	s.proxy = NewProxyHandler(cfg, database)
	s.routes()
	return s
}

func (s *Server) Start() error {
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

	// Dashboard API endpoints
	s.mux.HandleFunc("GET /api/dashboard/stats", s.handleDashboardStats)
	s.mux.HandleFunc("GET /api/dashboard/connections", s.handleDashboardConnections)
	s.mux.HandleFunc("GET /api/dashboard/cache", s.handleDashboardCache)
	s.mux.HandleFunc("GET /api/dashboard/combo", s.handleDashboardCombo)
	s.mux.HandleFunc("GET /api/dashboard/logs", s.handleDashboardLogs)
	s.mux.HandleFunc("PUT /api/dashboard/settings", s.handleDashboardSettings)

	// Dashboard (embedded SPA) — serve index.html for all non-API paths
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// API routes already matched above; this catches browser requests
		f, err := dashboard.Assets.ReadFile("index.html")
		if err != nil {
			http.Error(w, "dashboard not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(f)
	})
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
		// Skip auth for health, dashboard, auth, and dashboard API
		if r.URL.Path == "/health" || r.URL.Path == "/" ||
			strings.HasPrefix(r.URL.Path, "/api/dashboard/") ||
			strings.HasPrefix(r.URL.Path, "/api/auth/") {
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

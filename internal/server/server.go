package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
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

	// Health
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// OpenAI-compatible API
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	s.mux.HandleFunc("POST /v1/chat/completions", s.proxy.HandleChatCompletions)
	s.mux.HandleFunc("POST /v1/embeddings", s.proxy.HandleEmbeddings)

	// Management API
	s.mux.HandleFunc("GET /api/connections", s.handleGetConnections)
	s.mux.HandleFunc("POST /api/connections", s.handleCreateConnection)
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

	// Dashboard (embedded SPA)
	dashFS, _ := fs.Sub(dashboard.Assets, "dist")
	s.mux.HandleFunc("/dashboard/", func(w http.ResponseWriter, r *http.Request) {
		// Strip /dashboard prefix for file lookup
		path := strings.TrimPrefix(r.URL.Path, "/dashboard/")
		if path == "" {
			path = "index.html"
		}
		// Try to serve the file, fallback to index.html for SPA routing
		f, err := dashFS.(fs.ReadFileFS).ReadFile(path)
		if err != nil {
			f, _ = dashFS.(fs.ReadFileFS).ReadFile("index.html")
			path = "index.html"
		}
		// Set content type
		switch {
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css")
		case strings.HasSuffix(path, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		case strings.HasSuffix(path, ".json"):
			w.Header().Set("Content-Type", "application/json")
		}
		w.Write(f)
	})

	// Redirect /dashboard to /dashboard/
	s.mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/", http.StatusMovedPermanently)
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
		// Skip auth for health and dashboard
		if r.URL.Path == "/health" || r.URL.Path == "/dashboard" || r.URL.Path == "/dashboard/" {
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

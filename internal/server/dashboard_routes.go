package server

import (
	"net/http"

	"github.com/sanhaji182/lintasan-go/internal/web"
)

func (s *Server) registerDashboardRoutes() {
	// Dashboard API — used by SvelteKit frontend
	mux := s.mux
	mux.HandleFunc("GET /api/dashboard/stats", s.handleDashboardStats)
	mux.HandleFunc("GET /api/dashboard/connections", s.handleDashboardConnections)
	mux.HandleFunc("GET /api/dashboard/cache", s.handleDashboardCache)
	mux.HandleFunc("GET /api/dashboard/combo", s.handleDashboardCombo)
	mux.HandleFunc("GET /api/dashboard/logs", s.handleDashboardLogs)
	mux.HandleFunc("PUT /api/dashboard/settings", s.handleDashboardSettings)

	// Retired dashboard routes: explicit HTTP redirect to canonical destinations
	// so bookmarks and deep links do not become unexplained 404s.
	mux.HandleFunc("GET /dashboard/providers", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/connections", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /dashboard/providers/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard/connections", http.StatusMovedPermanently)
	})

	// Embedded SPA dashboard. Mounted at "GET /" — Go 1.22 ServeMux gives the
	// specific /api, /v1, /health, /metrics, /mcp patterns priority, so this
	// catch-all only handles UI routes + static assets. Serves index.html as a
	// fallback for client-side routing (/dashboard, /login, /change-password).
	// When no UI is embedded (API-only build), falls back to the old redirect.
	if web.Available() {
		mux.Handle("GET /", web.Handler())
	} else {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, "/dashboard", http.StatusFound)
		})
	}
}

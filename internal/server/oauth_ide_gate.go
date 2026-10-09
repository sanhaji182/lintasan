package server

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

// oauthIdeDisabledJSON is returned when the OAuth IDE lab is off (dashboard Settings or env).
func oauthIdeDisabledJSON(w http.ResponseWriter) {
	writeJSONStatus(w, http.StatusNotFound, map[string]any{
		"error":   "oauth_ide_disabled",
		"hint":    "Enable **OAuth IDE (experimental)** in Dashboard → Settings (admin). Env LINTASAN_OAUTH_IDE_ENABLED still applies if the setting was never saved.",
		"enabled": false,
	})
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user := auth.GetUser(r)
	if user == nil || user.Role != "admin" {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "admin access required"})
		return nil, false
	}
	return user, true
}

const oauthIdeSettingKey = "oauth_ide_enabled"

func parseBoolSetting(v string) (bool, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

// resolveOAuthIdeEnabled is the SINGLE source of truth for the OAuth IDE lab
// gate: dashboard setting `oauth_ide_enabled` wins when set to a recognizable
// boolean; otherwise the env-derived value latched at startup.
//
// Both surfaces MUST call this. They previously each had their own check —
// (*Server).oauthIdeEnabled read the DB setting while the proxy hot path
// (oauth_proxy_wire.go) read only cfg.OAuthIDEEnabled, which is latched from
// the environment at BOOT. With the setting saved as true and the env var
// unset, the dashboard reported the lab enabled and the "Wire" button created a
// connection, but the proxy never attached the OAuth token (it fell through to
// the empty static api_key) — so requests failed on a connection that looked
// wired. That divergence is what this function exists to make impossible.
func resolveOAuthIdeEnabled(database *db.DB, cfg *config.Config) bool {
	if database != nil {
		if v, err := database.GetSetting(oauthIdeSettingKey); err == nil && strings.TrimSpace(v) != "" {
			if b, ok := parseBoolSetting(v); ok {
				return b
			}
		}
	}
	return cfg != nil && cfg.OAuthIDEEnabled
}

// oauthIdeEnabled: dashboard setting oauth_ide_enabled wins when set; else LINTASAN_OAUTH_IDE_ENABLED env.
func (s *Server) oauthIdeEnabled() bool {
	return resolveOAuthIdeEnabled(s.db, s.cfg)
}

// oauthIdeEnabled (ProxyHandler) is the same gate the dashboard uses. The proxy
// hot path MUST NOT re-derive it from cfg alone — see resolveOAuthIdeEnabled.
func (p *ProxyHandler) oauthIdeEnabled() bool {
	return resolveOAuthIdeEnabled(p.db, p.cfg)
}

func (s *Server) oauthPublicBaseURL() string {
	if s.cfg != nil && s.cfg.OAuthPublicBaseURL != "" {
		return s.cfg.OAuthPublicBaseURL
	}
	if v := strings.TrimRight(os.Getenv("LINTASAN_OAUTH_PUBLIC_BASE_URL"), "/"); v != "" {
		return v
	}
	return "http://localhost:20180"
}

// isOAuthIdeCallback reports public OAuth redirect handlers (no JWT).
func isOAuthIdeCallback(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	return strings.HasPrefix(path, "/api/oauth/callback/")
}

// oauthIdeProviderIDs lists the catalog ids in catalog order, so the accounts
// view groups providers in the same order the dashboard shows them.
func oauthIdeProviderIDs() []string {
	catalog := oauthide.Catalog()
	out := make([]string, 0, len(catalog))
	for _, p := range catalog {
		out = append(out, p.ID)
	}
	return out
}

// oauthProviderDisplayName returns the human name for a catalog id, or "".
func oauthProviderDisplayName(id string) string {
	if p := oauthide.ByID(id); p != nil {
		return p.Name
	}
	return ""
}

// auth_IdeOAuthDisclaimer exposes the shared ToS disclaimer for API payloads
// without leaking the auth package into the view layer everywhere.
func auth_IdeOAuthDisclaimer() string { return auth.IdeOAuthDisclaimer }

// oauthRefreshSkewSeconds mirrors the auth package's refresh window so the
// dashboard can warn "expiring" at the same threshold the proxy refreshes at.
const oauthRefreshSkewSeconds = 300

// parseExpiryUntil parses an RFC3339 expiry and returns whole seconds until it.
func parseExpiryUntil(expiresAt string) (int, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(expiresAt))
	if err != nil {
		return 0, false
	}
	return int(time.Until(t).Seconds()), true
}

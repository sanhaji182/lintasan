package server

import (
	"net/http"
	"os"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/auth"
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

// oauthIdeEnabled: dashboard setting oauth_ide_enabled wins when set; else LINTASAN_OAUTH_IDE_ENABLED env.
func (s *Server) oauthIdeEnabled() bool {
	if s.db != nil {
		if v, err := s.db.GetSetting(oauthIdeSettingKey); err == nil && strings.TrimSpace(v) != "" {
			if b, ok := parseBoolSetting(v); ok {
				return b
			}
		}
	}
	return s.cfg != nil && s.cfg.OAuthIDEEnabled
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
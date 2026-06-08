package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

// AuthorizeResult is returned from startOAuthAuthorize.
type AuthorizeResult struct {
	Flow        string         // browser_redirect | device_code
	RedirectURL string
	Device      *oauthide.DeviceStart
}

func startOAuthAuthorizeFull(s *Server, provider, sessionID, publicBase string) (*AuthorizeResult, error) {
	meta := oauthide.ByID(provider)
	if meta == nil {
		return nil, fmt.Errorf("unknown provider %s", provider)
	}
	switch provider {
	case "xai":
		pkce, err := oauthide.NewPKCE(oauthide.XAIPKCEBytes)
		if err != nil {
			return nil, err
		}
		if err := s.oauthMgr.SetSessionPKCE(sessionID, pkce.Verifier); err != nil {
			return nil, err
		}
		redirect := publicBase + "/api/oauth/callback/xai"
		return &AuthorizeResult{
			Flow:        "browser_redirect",
			RedirectURL: oauthide.BuildXAIAuthorizeURL(redirect, sessionID, pkce.Challenge),
		}, nil
	case "claude":
		url, err := oauthide.StartBrowserPKCE(oauthide.ClaudePKCEBytes, oauthide.BuildClaudeAuthorizeURL, publicBase, "claude", sessionID,
			func(v string) error { return s.oauthMgr.SetSessionPKCE(sessionID, v) })
		if err != nil {
			return nil, err
		}
		return &AuthorizeResult{Flow: "browser_redirect", RedirectURL: url}, nil
	case "codex":
		url, err := oauthide.StartBrowserPKCE(oauthide.CodexPKCEBytes, oauthide.BuildCodexAuthorizeURL, publicBase, "codex", sessionID,
			func(v string) error { return s.oauthMgr.SetSessionPKCE(sessionID, v) })
		if err != nil {
			return nil, err
		}
		return &AuthorizeResult{Flow: "browser_redirect", RedirectURL: url}, nil
	case "github":
		dev, err := oauthide.StartGitHubDevice()
		if err != nil {
			return nil, err
		}
		uiMeta, _ := json.Marshal(map[string]any{
			"user_code":                 dev.UserCode,
			"verification_uri":          dev.VerificationURI,
			"verification_uri_complete": dev.VerificationURIComplete,
			"expires_in":                dev.ExpiresIn,
			"interval":                  dev.Interval,
		})
		if err := s.oauthMgr.SetSessionDevice(sessionID, dev.DeviceCode, string(uiMeta)); err != nil {
			return nil, err
		}
		return &AuthorizeResult{Flow: "device_code", Device: dev}, nil
	default:
		return nil, fmt.Errorf("provider %s not implemented yet (flow=%s). Catalog lists all 8; port in progress.", provider, meta.Flow)
	}
}

func (s *Server) handleOAuthDevicePoll(w http.ResponseWriter, r *http.Request) {
	if !s.oauthIdeEnabled() {
		oauthIdeDisabledJSON(w)
		return
	}
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "session_id required"})
		return
	}
	sess, err := s.oauthMgr.GetSessionByID(sessionID)
	if err != nil || sess == nil {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if sess.Status == "active" {
		writeJSON(w, map[string]any{"status": "active", "provider": sess.Provider})
		return
	}
	if sess.Provider != "github" || sess.DeviceCode == "" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "not a pending device session"})
		return
	}
	res, err := oauthide.PollGitHubDeviceOnce(sess.DeviceCode)
	if err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if res.Pending {
		writeJSON(w, map[string]any{"status": "pending", "hint": "complete GitHub device login"})
		return
	}
	expires := time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
	if res.ExpiresIn <= 0 {
		expires = time.Now().Add(8 * time.Hour)
	}
	if err := s.oauthMgr.UpdateSessionTokensWithMeta(sessionID, res.AccessToken, res.RefreshToken, expires, res.FlowMeta); err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "store tokens failed"})
		return
	}
	s.audit("oauth.ide.device_ok", "github", sessionID, map[string]any{"provider": "github"})
	writeJSON(w, map[string]any{"status": "active", "provider": "github"})
}
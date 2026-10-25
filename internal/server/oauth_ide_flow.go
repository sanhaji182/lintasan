package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

func (s *Server) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	catalog := oauthide.Catalog()
	enabled := s.oauthIdeEnabled()
	out := map[string]any{
		"enabled":      enabled,
		"experimental": true,
		"catalog":      catalog,
		"disclaimer":   auth.IdeOAuthDisclaimer,
		"proxy_wired":  false,
		"source":       "9router OAUTH_PROVIDERS v0.5.104 (Go rewrite)",
	}
	if enabled {
		out["public_base"] = s.oauthPublicBaseURL()
		out["hint"] = "ready: xai, claude, codex, antigravity, cline (browser); github, kilocode, nous (device + poll); cursor (import)."
		out["xai_redirect_uri"] = oauthide.XAILoopbackRedirect
		out["xai_note"] = "xAI redirect is fixed loopback only. Authorize always opens xAI. If callback hits this server on :56121, login completes automatically; otherwise paste the callback URL in the dashboard."
	}
	writeJSON(w, out)
}

func startOAuthAuthorize(s *Server, provider, sessionID, publicBase string) (redirectURL string, err error) {
	res, err := startOAuthAuthorizeFull(s, provider, sessionID, publicBase)
	if err != nil {
		return "", err
	}
	if res.Flow != "browser_redirect" {
		return "", fmt.Errorf("use device_code response for %s", provider)
	}
	return res.RedirectURL, nil
}

func exchangeOAuthCallback(provider, code, redirectURI, pkceVerifier, oauthState string) (access, refresh string, expiresIn int, flowMeta string, err error) {
	switch provider {
	case "xai":
		tok, err := oauthide.ExchangeXAIToken(code, redirectURI, pkceVerifier)
		if err != nil {
			return "", "", 0, "", err
		}
		return tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, oauthIDTokenMeta(tok.IDToken), nil
	case "claude":
		tok, err := oauthide.ExchangeClaudeToken(code, redirectURI, pkceVerifier, oauthState)
		if err != nil {
			return "", "", 0, "", err
		}
		return tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, oauthIDTokenMeta(tok.IDToken), nil
	case "codex":
		tok, meta, err := oauthide.ExchangeCodexToken(code, redirectURI, pkceVerifier)
		if err != nil {
			return "", "", 0, "", err
		}
		return tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, meta, nil
	case "antigravity":
		tok, meta, err := oauthide.ExchangeAntigravityToken(code, redirectURI)
		if err != nil {
			return "", "", 0, "", err
		}
		return tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, meta, nil
	case "cline":
		tok, meta, err := oauthide.ExchangeClineToken(code, redirectURI)
		if err != nil {
			return "", "", 0, "", err
		}
		return tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, meta, nil
	default:
		a, r, e, err := exchangeIdeOAuthCodeLegacy(provider, code, redirectURI)
		return a, r, e, "", err
	}
}

// oauthIDTokenMeta keeps the id_token so the accounts API can surface the
// signed-in identity (email/name) without another provider round-trip.
func oauthIDTokenMeta(idToken string) string {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return ""
	}
	meta, _ := json.Marshal(map[string]string{"id_token": idToken})
	return string(meta)
}

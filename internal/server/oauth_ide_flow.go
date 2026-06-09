package server

import (
	"fmt"
	"net/http"

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
		"source":       "9router OAUTH_PROVIDERS v0.4.71 (Go rewrite)",
	}
	if enabled {
		out["public_base"] = s.oauthPublicBaseURL()
		out["hint"] = "ready: xai, claude, codex, antigravity, cline (browser); github, kilocode (device + poll); cursor (import)."
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
		return tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, "", nil
	case "claude":
		tok, err := oauthide.ExchangeClaudeToken(code, redirectURI, pkceVerifier, oauthState)
		if err != nil {
			return "", "", 0, "", err
		}
		return tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, "", nil
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
package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/oauthide"
)

const antigravityProxyTimeout = 5 * time.Minute

type antigravityLoopbackProxy struct {
	mu          sync.Mutex
	server      *http.Server
	ln          net.Listener
	lastPending string // session id of most recent Antigravity authorize (for raw-code paste)
}

var globalAntigravityProxy antigravityLoopbackProxy

// tryStartAntigravityLoopbackProxy binds 127.0.0.1:51121 for the Antigravity callback. Best-effort — never blocks authorize.
func tryStartAntigravityLoopbackProxy(s *Server, sessionID string) (started bool, warn string) {
	globalAntigravityProxy.mu.Lock()
	globalAntigravityProxy.lastPending = sessionID
	if globalAntigravityProxy.server != nil {
		globalAntigravityProxy.mu.Unlock()
		return true, ""
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", oauthide.AntigravityLoopbackPort))
	if err != nil {
		globalAntigravityProxy.mu.Unlock()
		return false, fmt.Sprintf(
			"Could not listen on 127.0.0.1:%d (%v). After Google redirects, copy the full URL (http://127.0.0.1:51121/callback?code=...&state=...) or just the code itself and use 'Complete Antigravity login' below.",
			oauthide.AntigravityLoopbackPort, err,
		)
	}

	mux := http.NewServeMux()
	handleCallback := func(w http.ResponseWriter, r *http.Request) {
		serveAntigravityLoopbackCallback(s, w, r)
	}
	mux.HandleFunc(oauthide.AntigravityLoopbackPath, handleCallback)
	mux.HandleFunc("/oauth-callback", handleCallback)
	mux.HandleFunc("/auth/callback", handleCallback)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	globalAntigravityProxy.ln = ln
	globalAntigravityProxy.server = srv
	globalAntigravityProxy.mu.Unlock()

	go func() {
		_ = srv.Serve(ln)
	}()

	time.AfterFunc(antigravityProxyTimeout, func() { stopAntigravityLoopbackProxy() })
	return true, ""
}

func serveAntigravityLoopbackCallback(s *Server, w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")
	if errParam != "" {
		desc := r.URL.Query().Get("error_description")
		if desc == "" {
			desc = errParam
		}
		writeAntigravityCallbackHTML(w, false, desc)
		stopAntigravityLoopbackProxy()
		return
	}
	if state == "" {
		globalAntigravityProxy.mu.Lock()
		state = globalAntigravityProxy.lastPending
		globalAntigravityProxy.mu.Unlock()
	}
	if code == "" || state == "" {
		writeAntigravityCallbackHTML(w, false, "Missing code or state from Google Antigravity.")
		stopAntigravityLoopbackProxy()
		return
	}
	if err := completeAntigravityOAuthCode(s, code, state); err != nil {
		writeAntigravityCallbackHTML(w, false, err.Error())
		stopAntigravityLoopbackProxy()
		return
	}
	writeAntigravityCallbackHTML(w, true, "Google Antigravity connected. Return to Lintasan and refresh sessions.")
	stopAntigravityLoopbackProxy()
}

// completeAntigravityOAuthCode exchanges authorization code for tokens and stores session (state = session id).
func completeAntigravityOAuthCode(s *Server, code, state string) error {
	pending, err := s.oauthMgr.GetPendingSession(state)
	if err != nil || pending == nil {
		return fmt.Errorf("invalid or expired session — Authorize first, then complete within 5 minutes")
	}
	if pending.Provider != "antigravity" {
		return fmt.Errorf("provider mismatch (expected antigravity)")
	}
	redirectURI := oauthide.AntigravityRedirectURI()
	access, refresh, expIn, flowMeta, exchErr := exchangeOAuthCallback("antigravity", code, redirectURI, pending.PKCEVerifier, state)
	if exchErr != nil {
		s.audit("oauth.ide.callback_failed", "antigravity", state, map[string]any{"error": exchErr.Error()})
		return fmt.Errorf("token exchange failed: %s", exchErr.Error())
	}
	expires := time.Now().Add(time.Duration(expIn) * time.Second)
	if expIn <= 0 {
		expires = time.Now().Add(24 * time.Hour)
	}
	if flowMeta != "" {
		err = s.oauthMgr.UpdateSessionTokensWithMeta(state, access, refresh, expires, flowMeta)
	} else {
		err = s.oauthMgr.UpdateSessionTokens(state, access, refresh, expires)
	}
	if err != nil {
		return fmt.Errorf("failed to store tokens: %v", err)
	}
	s.audit("oauth.ide.callback_ok", "antigravity", state, map[string]any{"via": "manual"})
	return nil
}

func stopAntigravityLoopbackProxy() {
	globalAntigravityProxy.mu.Lock()
	defer globalAntigravityProxy.mu.Unlock()
	if globalAntigravityProxy.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = globalAntigravityProxy.server.Shutdown(ctx)
		cancel()
		globalAntigravityProxy.server = nil
	}
	if globalAntigravityProxy.ln != nil {
		_ = globalAntigravityProxy.ln.Close()
		globalAntigravityProxy.ln = nil
	}
}

func writeAntigravityCallbackHTML(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title := "Lintasan Antigravity OAuth"
	color := "#3fb950"
	if !ok {
		color = "#f85149"
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = io.WriteString(w, fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family:system-ui;background:#0d1117;color:#c9d1d9;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0">
<div style="max-width:420px;padding:2rem;background:#161b22;border:1px solid #30363d;border-radius:12px;text-align:center">
<h2 style="color:%s">%s</h2><p>%s</p></div></body></html>`, title, color, title, msg))
}

func parseAntigravityCallbackInput(raw string) (code, state string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("paste the full callback URL or the authorization code")
	}

	if strings.Contains(raw, "://") {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("invalid URL: %w", perr)
		}
		code = u.Query().Get("code")
		state = u.Query().Get("state")
		if code == "" {
			return "", "", fmt.Errorf("URL must include the code parameter")
		}
		if state == "" {
			globalAntigravityProxy.mu.Lock()
			state = globalAntigravityProxy.lastPending
			globalAntigravityProxy.mu.Unlock()
		}
		if state == "" {
			return "", "", fmt.Errorf("URL must include the state parameter")
		}
		return code, state, nil
	}

	// Raw code
	globalAntigravityProxy.mu.Lock()
	pending := globalAntigravityProxy.lastPending
	globalAntigravityProxy.mu.Unlock()

	if pending == "" {
		return "", "", fmt.Errorf("no pending Antigravity session. Authorize first, then paste the complete callback URL (http://127.0.0.1:51121/callback?code=...&state=...)")
	}

	if strings.ContainsAny(raw, " \t\n\r") {
		return "", "", fmt.Errorf("raw code must not contain whitespace — paste only the authorization code value")
	}
	if len(raw) < 10 {
		return "", "", fmt.Errorf("authorization code seems too short (got %d chars)", len(raw))
	}

	return raw, pending, nil
}

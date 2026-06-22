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

const xaiProxyTimeout = 5 * time.Minute

type xaiLoopbackProxy struct {
	mu          sync.Mutex
	server      *http.Server
	ln          net.Listener
	lastPending string // session id of most recent xAI authorize (for raw-code paste)
}

var globalXaiProxy xaiLoopbackProxy

// tryStartXAILoopbackProxy binds 127.0.0.1:56121 for the xAI callback. Best-effort — never blocks authorize.
func tryStartXAILoopbackProxy(s *Server, sessionID string) (started bool, warn string) {
	globalXaiProxy.mu.Lock()
	globalXaiProxy.lastPending = sessionID
	if globalXaiProxy.server != nil {
		globalXaiProxy.mu.Unlock()
		return true, ""
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", oauthide.XAILoopbackPort))
	if err != nil {
		globalXaiProxy.mu.Unlock()
		return false, fmt.Sprintf(
			"Could not listen on 127.0.0.1:%d (%v). After xAI redirects, copy the full URL (http://127.0.0.1:56121/callback?code=...&state=...) or just the code itself and use 'Complete xAI login' below.",
			oauthide.XAILoopbackPort, err,
		)
	}

	mux := http.NewServeMux()
	handleXaiCallback := func(w http.ResponseWriter, r *http.Request) {
		serveXAILoopbackCallback(s, w, r)
	}
	mux.HandleFunc(oauthide.XAILoopbackPath, handleXaiCallback)
	mux.HandleFunc("/auth/callback", handleXaiCallback)

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	globalXaiProxy.ln = ln
	globalXaiProxy.server = srv
	globalXaiProxy.mu.Unlock()

	go func() {
		_ = srv.Serve(ln)
	}()

	time.AfterFunc(xaiProxyTimeout, func() { stopXAILoopbackProxy() })
	return true, ""
}

func serveXAILoopbackCallback(s *Server, w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")
	if errParam != "" {
		desc := r.URL.Query().Get("error_description")
		if desc == "" {
			desc = errParam
		}
		writeXaiCallbackHTML(w, false, desc)
		stopXAILoopbackProxy()
		return
	}
	if code == "" || state == "" {
		writeXaiCallbackHTML(w, false, "Missing code or state from xAI.")
		stopXAILoopbackProxy()
		return
	}
	if err := completeXAIOAuthCode(s, code, state); err != nil {
		writeXaiCallbackHTML(w, false, err.Error())
		stopXAILoopbackProxy()
		return
	}
	writeXaiCallbackHTML(w, true, "xAI connected. Return to Lintasan OAuth IDE and refresh sessions.")
	stopXAILoopbackProxy()
}

// completeXAIOAuthCode exchanges authorization code for tokens and stores session (state = session id).
func completeXAIOAuthCode(s *Server, code, state string) error {
	pending, err := s.oauthMgr.GetPendingSession(state)
	if err != nil || pending == nil {
		return fmt.Errorf("invalid or expired session — Authorize first, then complete within 5 minutes")
	}
	if pending.Provider != "xai" {
		return fmt.Errorf("provider mismatch (expected xai)")
	}
	redirectURI := oauthide.XAILoopbackRedirect
	access, refresh, expIn, flowMeta, exchErr := exchangeOAuthCallback("xai", code, redirectURI, pending.PKCEVerifier, state)
	if exchErr != nil {
		s.audit("oauth.ide.callback_failed", "xai", state, map[string]any{"error": exchErr.Error()})
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
	s.audit("oauth.ide.callback_ok", "xai", state, map[string]any{"via": "manual"})
	return nil
}

func stopXAILoopbackProxy() {
	globalXaiProxy.mu.Lock()
	defer globalXaiProxy.mu.Unlock()
	if globalXaiProxy.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = globalXaiProxy.server.Shutdown(ctx)
		cancel()
		globalXaiProxy.server = nil
	}
	if globalXaiProxy.ln != nil {
		_ = globalXaiProxy.ln.Close()
		globalXaiProxy.ln = nil
	}
}

func writeXaiCallbackHTML(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title := "Lintasan xAI OAuth"
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

// parseXaiCallbackInput accepts any of:
//   - Full URL: http://127.0.0.1:56121/callback?code=XYZ&state=ABC
//   - Raw authorization code (no "://") — state resolved from most recent pending session.
func parseXaiCallbackInput(raw string) (code, state string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("paste the full callback URL or the authorization code")
	}

	// If it has ://, treat as URL and extract query params.
	if strings.Contains(raw, "://") {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("invalid URL: %w", perr)
		}
		code = u.Query().Get("code")
		state = u.Query().Get("state")
		if code == "" || state == "" {
			// Try without query string — maybe ? is missing
			code = u.Query().Get("code")
			state = u.Query().Get("state")
			if code == "" || state == "" {
				return "", "", fmt.Errorf("URL must include code and state query parameters")
			}
		}
		return code, state, nil
	}

	// Raw code — pull state from most recent pending xAI session.
	globalXaiProxy.mu.Lock()
	pending := globalXaiProxy.lastPending
	globalXaiProxy.mu.Unlock()

	if pending == "" {
		return "", "", fmt.Errorf("no pending xAI session. Authorize first, then paste the complete callback URL (http://127.0.0.1:56121/callback?code=...&state=...)")
	}

	// Sanity: the raw value looks like an authorization code (no spaces, reasonable length).
	if strings.ContainsAny(raw, " \t\n\r") {
		return "", "", fmt.Errorf("raw code must not contain whitespace — paste only the authorization code value")
	}
	if len(raw) < 20 {
		return "", "", fmt.Errorf("authorization code seems too short (got %d chars)", len(raw))
	}

	return raw, pending, nil
}

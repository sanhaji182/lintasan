package server

// muse_handlers.go — admin-only proxy for the muse2api sidecar.
//
// WHY THIS EXISTS
//
// muse2api owns the muse.ai browser session and its admin credential. The
// dashboard must not see that credential, so Lintasan proxies a fixed allowlist
// of admin endpoints and injects `Authorization: Bearer <admin key>` server-side.
//
// SECURITY POSTURE
//
//   - Every route calls requireAdmin; the global /api middleware already rejects
//     anonymous calls, this is the second, route-level gate.
//   - The upstream base URL comes only from configuration; no request value can
//     change host, scheme, or path prefix (no open proxy / SSRF).
//   - Account ids are validated before they are placed in the upstream path.
//   - Request and response bodies are bounded; redirects are not followed so
//     the admin key can never be forwarded to another host.
//   - Upstream 401/403 is mapped to 502: returning 401 would make the Lintasan
//     API client clear the operator's dashboard token.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	museMaxBodyBytes    = 2 << 20 // 2 MiB
	museImportMaxBytes  = 128 << 10
	museImportTokenTTL  = 10 * time.Minute
	museImportTokenPref = "museimp_"
	museSetupHint       = "Set LINTASAN_MUSE2API_ADMIN_KEY (and optionally LINTASAN_MUSE2API_URL) on the Lintasan server, then restart it."
)

var museAccountIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// museConfigured reports whether the proxy has everything it needs. The base
// URL is validated here so a malformed value fails closed as "not configured"
// instead of producing an open or broken proxy.
func (s *Server) museConfigured() bool {
	if s == nil || strings.TrimSpace(s.museAdminKey) == "" {
		return false
	}
	base := strings.TrimSpace(s.museBaseURL)
	if base == "" {
		return false
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return false
	}
	// Reject credentials in the URL: they would end up in error strings and are
	// never a valid configuration here.
	if strings.Contains(strings.SplitN(base, "://", 2)[1], "@") {
		return false
	}
	return true
}

func (s *Server) museBase() string {
	return strings.TrimRight(strings.TrimSpace(s.museBaseURL), "/")
}

func (s *Server) museHTTPClient() *http.Client {
	if s.museClient != nil {
		return s.museClient
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func museError(w http.ResponseWriter, status int, message string) {
	writeJSONStatus(w, status, map[string]any{
		"error": message,
		"hint":  museSetupHint,
	})
}

// setMuseImportCORS is applied to the extension import endpoint and its
// preflight. It is token-authenticated and does not use cookies, so a wildcard
// origin is safe here; credentials are deliberately not allowed.
func setMuseImportCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.Header().Set("Cache-Control", "no-store")
}

func bearerFromHeader(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func (s *Server) pruneMuseImportTokensLocked(now time.Time) {
	for token, expires := range s.museImportTokens {
		if !now.Before(expires) {
			delete(s.museImportTokens, token)
		}
	}
}

func (s *Server) newMuseImportToken() (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token := museImportTokenPref + hex.EncodeToString(b)
	expires := time.Now().Add(museImportTokenTTL)

	s.museImportMu.Lock()
	defer s.museImportMu.Unlock()
	if s.museImportTokens == nil {
		s.museImportTokens = make(map[string]time.Time)
	}
	s.pruneMuseImportTokensLocked(time.Now())
	s.museImportTokens[token] = expires
	return token, expires, nil
}

// museImportTokenValid peeks at a token without consuming it. The middleware
// uses it so the handler remains the single place that consumes the token.
func (s *Server) museImportTokenValid(token string) bool {
	if token == "" {
		return false
	}
	s.museImportMu.Lock()
	defer s.museImportMu.Unlock()
	expires, ok := s.museImportTokens[token]
	if !ok {
		return false
	}
	now := time.Now()
	if !now.Before(expires) {
		delete(s.museImportTokens, token)
		return false
	}
	return true
}

func (s *Server) consumeMuseImportToken(token string) bool {
	if token == "" {
		return false
	}
	s.museImportMu.Lock()
	defer s.museImportMu.Unlock()
	expires, ok := s.museImportTokens[token]
	delete(s.museImportTokens, token)
	return ok && time.Now().Before(expires)
}

// handleMuseImportToken mints a short-lived, single-use token for the browser
// extension. The token cannot call any other endpoint.
func (s *Server) handleMuseImportToken(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if !s.museConfigured() {
		museError(w, http.StatusServiceUnavailable, "muse sidecar is not configured")
		return
	}
	token, expires, err := s.newMuseImportToken()
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate import token"})
		return
	}
	writeJSON(w, map[string]any{
		"token":      token,
		"expires_at": expires.Unix(),
		"expires_in": int(museImportTokenTTL.Seconds()),
	})
}

// handleMuseBrowserImport is the extension-facing endpoint. The middleware has
// already validated the one-time token for this path; this handler consumes it,
// validates the payload, and forwards to muse2api with the server-side admin key.
func (s *Server) handleMuseBrowserImport(w http.ResponseWriter, r *http.Request) {
	setMuseImportCORS(w)
	if !s.museConfigured() {
		museError(w, http.StatusServiceUnavailable, "muse sidecar is not configured")
		return
	}
	if !s.consumeMuseImportToken(bearerFromHeader(r)) {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired import token"})
		return
	}

	limited := http.MaxBytesReader(w, r.Body, museImportMaxBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "request body too large or unreadable"})
		return
	}
	var payload struct {
		Label   string          `json:"label"`
		Cookies json.RawMessage `json:"cookies"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || len(payload.Cookies) == 0 || string(payload.Cookies) == "null" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "label/cookies payload is invalid"})
		return
	}
	forwarded, err := json.Marshal(payload)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "label/cookies payload is invalid"})
		return
	}

	resp, data, err := s.museDo(r, http.MethodPost, "/admin/accounts", forwarded)
	if err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error":  "muse2api unreachable",
			"detail": err.Error(),
		})
		return
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error": "muse2api rejected the admin key",
			"hint":  "check LINTASAN_MUSE2API_ADMIN_KEY",
		})
	case resp.StatusCode >= 500:
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error":           "muse2api upstream error",
			"upstream_status": resp.StatusCode,
		})
	default:
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(data)
	}
}

// handleMuseStatus is deliberately 200 even when the sidecar is down: the
// dashboard renders an actionable status card rather than treating this as a
// failed page load.
func (s *Server) handleMuseStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	out := map[string]any{
		"configured": s.museConfigured(),
		"base_url":   s.museBase(),
	}
	if !s.museConfigured() {
		out["reachable"] = false
		out["ok"] = false
		out["hint"] = museSetupHint
		writeJSON(w, out)
		return
	}

	resp, data, err := s.museDo(r, http.MethodGet, "/admin/status", nil)
	if err != nil {
		out["reachable"] = false
		out["ok"] = false
		out["error"] = err.Error()
		writeJSON(w, out)
		return
	}
	out["reachable"] = true
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		out["ok"] = false
		out["error"] = "muse2api rejected the admin key"
		writeJSON(w, out)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out["ok"] = false
		out["error"] = fmt.Sprintf("muse2api returned HTTP %d", resp.StatusCode)
		writeJSON(w, out)
		return
	}

	var upstream map[string]any
	if err := json.Unmarshal(data, &upstream); err != nil {
		out["ok"] = false
		out["error"] = "muse2api returned invalid JSON"
		writeJSON(w, out)
		return
	}
	out["ok"] = true
	out["status"] = upstream
	writeJSON(w, out)
}

func (s *Server) handleMuseAccountsList(w http.ResponseWriter, r *http.Request) {
	s.forwardMuse(w, r, http.MethodGet, "/admin/accounts")
}

func (s *Server) handleMuseAccountCreate(w http.ResponseWriter, r *http.Request) {
	s.forwardMuse(w, r, http.MethodPost, "/admin/accounts")
}

func (s *Server) handleMuseAccountsImport(w http.ResponseWriter, r *http.Request) {
	s.forwardMuse(w, r, http.MethodPost, "/admin/accounts/import")
}

func (s *Server) handleMuseAccountUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := musePathID(w, r)
	if !ok {
		return
	}
	s.forwardMuse(w, r, http.MethodPatch, "/admin/accounts/"+id)
}

func (s *Server) handleMuseAccountDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := musePathID(w, r)
	if !ok {
		return
	}
	s.forwardMuse(w, r, http.MethodDelete, "/admin/accounts/"+id)
}

func (s *Server) handleMuseAccountRenew(w http.ResponseWriter, r *http.Request) {
	id, ok := musePathID(w, r)
	if !ok {
		return
	}
	s.forwardMuse(w, r, http.MethodPost, "/admin/accounts/"+id+"/renew")
}

func (s *Server) handleMuseAccountReset(w http.ResponseWriter, r *http.Request) {
	id, ok := musePathID(w, r)
	if !ok {
		return
	}
	s.forwardMuse(w, r, http.MethodPost, "/admin/accounts/"+id+"/reset")
}

func (s *Server) handleMuseAccountQuota(w http.ResponseWriter, r *http.Request) {
	id, ok := musePathID(w, r)
	if !ok {
		return
	}
	s.forwardMuse(w, r, http.MethodGet, "/admin/accounts/"+id+"/quota")
}

func musePathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !museAccountIDPattern.MatchString(id) {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid account id"})
		return "", false
	}
	return id, true
}

// forwardMuse relays one allowlisted admin call. It never forwards the
// operator's dashboard JWT or any inbound header other than the body: upstream
// auth is the server-side muse2api admin key.
func (s *Server) forwardMuse(w http.ResponseWriter, r *http.Request, method, upstreamPath string) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if !s.museConfigured() {
		museError(w, http.StatusServiceUnavailable, "muse sidecar is not configured")
		return
	}

	var body []byte
	if r.Body != nil {
		limited := http.MaxBytesReader(w, r.Body, museMaxBodyBytes)
		b, err := io.ReadAll(limited)
		if err != nil {
			writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "request body too large or unreadable"})
			return
		}
		if len(b) > 0 {
			body = b
		}
	}

	resp, data, err := s.museDo(r, method, upstreamPath, body)
	if err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error":  "muse2api unreachable",
			"detail": err.Error(),
		})
		return
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error": "muse2api rejected the admin key",
			"hint":  "check LINTASAN_MUSE2API_ADMIN_KEY",
		})
	case resp.StatusCode >= 500:
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error":           "muse2api upstream error",
			"upstream_status": resp.StatusCode,
		})
	default:
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(data)
	}
}

func (s *Server) museDo(r *http.Request, method, upstreamPath string, body []byte) (*http.Response, []byte, error) {
	return s.museDoContext(r.Context(), method, upstreamPath, body)
}

func (s *Server) museDoContext(ctx context.Context, method, upstreamPath string, body []byte) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.museBase()+upstreamPath, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.museAdminKey)
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.museHTTPClient().Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, museMaxBodyBytes+1))
	if err != nil {
		return resp, nil, err
	}
	if len(data) > museMaxBodyBytes {
		return resp, nil, fmt.Errorf("muse2api response too large")
	}
	return resp, data, nil
}

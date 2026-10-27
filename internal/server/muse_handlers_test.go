package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/auth"
	"github.com/sanhaji182/lintasan-go/internal/config"
)

func museAdminRequest(method, target string, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	ctx := context.WithValue(r.Context(), auth.UserContextKey, &auth.User{Username: "admin", Role: "admin"})
	return r.WithContext(ctx)
}

// newMuseProxy returns a Server wired to an upstream stub. The stub records the
// last request so tests can assert path, auth injection, and body forwarding.
func newMuseProxy(t *testing.T, handler http.HandlerFunc) (*Server, *httptest.Server) {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	s := &Server{
		museBaseURL:  upstream.URL,
		museAdminKey: "muse-admin-secret",
		museClient:   upstream.Client(),
	}
	return s, upstream
}

func decodeMuse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestMuseStatusUnconfigured(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleMuseStatus(rec, museAdminRequest(http.MethodGet, "/api/muse/status", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	out := decodeMuse(t, rec)
	if out["configured"] != false {
		t.Errorf("configured = %v, want false", out["configured"])
	}
	if out["hint"] == "" {
		t.Error("missing setup hint")
	}
}

func TestMuseRejectsNonAdmin(t *testing.T) {
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called for a non-admin request")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/muse/accounts", nil)
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.User{Username: "user", Role: "user"})
	rec := httptest.NewRecorder()
	s.handleMuseAccountsList(rec, req.WithContext(ctx))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestMuseStatusProxiesAndInjectsAdminKey(t *testing.T) {
	var gotPath, gotAuth string
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"version":"0.2.0","driver":{"driver":"browser","ok":true},"accounts":{"total":2,"available":1,"invalid":0,"inflight":0}}`))
	})

	rec := httptest.NewRecorder()
	s.handleMuseStatus(rec, museAdminRequest(http.MethodGet, "/api/muse/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	out := decodeMuse(t, rec)
	if out["configured"] != true || out["reachable"] != true || out["ok"] != true {
		t.Fatalf("unexpected status payload: %v", out)
	}
	if gotPath != "/admin/status" {
		t.Errorf("upstream path = %q", gotPath)
	}
	if gotAuth != "Bearer muse-admin-secret" {
		t.Errorf("upstream auth = %q", gotAuth)
	}
	status, _ := out["status"].(map[string]any)
	if status["version"] != "0.2.0" {
		t.Errorf("upstream status not forwarded: %v", out)
	}
}

func TestMuseCreateForwardsBodyAndAuth(t *testing.T) {
	var gotBody, gotAuth, gotCT string
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_, _ = w.Write([]byte(`{"account":{"id":"acc_test"},"missing_cookies":[]}`))
	})

	body := `{"label":"acc1","cookies":{"hatch_sess":"x"}}`
	rec := httptest.NewRecorder()
	s.handleMuseAccountCreate(rec, museAdminRequest(http.MethodPost, "/api/muse/accounts", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if gotBody != body {
		t.Errorf("forwarded body = %q, want %q", gotBody, body)
	}
	if gotAuth != "Bearer muse-admin-secret" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
}

func TestMuseAccountIDValidation(t *testing.T) {
	called := false
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	req := museAdminRequest(http.MethodPost, "/api/muse/accounts/../secret/renew", "")
	req.SetPathValue("id", "../secret")
	rec := httptest.NewRecorder()
	s.handleMuseAccountRenew(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if called {
		t.Error("upstream must not be called for an invalid account id")
	}
}

func TestMuseUpstreamAuthFailureIsNotDashboard401(t *testing.T) {
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"invalid admin key"}`))
	})
	rec := httptest.NewRecorder()
	s.handleMuseAccountsList(rec, museAdminRequest(http.MethodGet, "/api/muse/accounts", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	out := decodeMuse(t, rec)
	if !strings.Contains(out["error"].(string), "rejected") {
		t.Errorf("missing rejection error: %v", out)
	}
}

func TestMuseUnreachable(t *testing.T) {
	s := &Server{
		museBaseURL:  "http://127.0.0.1:1",
		museAdminKey: "muse-admin-secret",
		museClient:   &http.Client{Timeout: 200 * time.Millisecond},
	}
	rec := httptest.NewRecorder()
	s.handleMuseAccountsList(rec, museAdminRequest(http.MethodGet, "/api/muse/accounts", ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestMuseConfiguredValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    *Server
		want bool
	}{
		{"valid", &Server{museBaseURL: "http://127.0.0.1:18610", museAdminKey: "k"}, true},
		{"missing key", &Server{museBaseURL: "http://127.0.0.1:18610"}, false},
		{"bad scheme", &Server{museBaseURL: "ftp://127.0.0.1:18610", museAdminKey: "k"}, false},
		{"credentials", &Server{museBaseURL: "http://admin:secret@127.0.0.1:18610", museAdminKey: "k"}, false},
		{"empty base", &Server{museBaseURL: "", museAdminKey: "k"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.museConfigured(); got != tc.want {
				t.Errorf("museConfigured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMuseImportTokenIsAdminOnly(t *testing.T) {
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("token generation must not call upstream")
	})

	rec := httptest.NewRecorder()
	s.handleMuseImportToken(rec, museAdminRequest(http.MethodPost, "/api/muse/import-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	out := decodeMuse(t, rec)
	token, _ := out["token"].(string)
	if !strings.HasPrefix(token, museImportTokenPref) {
		t.Fatalf("token %q missing prefix", token)
	}
	if out["expires_in"].(float64) != museImportTokenTTL.Seconds() {
		t.Errorf("expires_in = %v", out["expires_in"])
	}

	rec = httptest.NewRecorder()
	s.handleMuseImportToken(rec, httptest.NewRequest(http.MethodPost, "/api/muse/import-token", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", rec.Code)
	}
}

func TestMuseBrowserImportConsumesToken(t *testing.T) {
	var gotAuth, gotBody string
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"account":{"id":"acc_ext"},"missing_cookies":[]}`))
	})
	token, _, err := s.newMuseImportToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}

	body := `{"label":"ext","cookies":{"hatch_sess":"x","hatch_gw":"y","hatch_vml":"z","hatch_native_auth_device":"w"}}`
	req := httptest.NewRequest(http.MethodPost, "/muse/import", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.handleMuseBrowserImport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
	if gotAuth != "Bearer muse-admin-secret" {
		t.Errorf("upstream auth = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"hatch_sess":"x"`) || !strings.Contains(gotBody, `"label":"ext"`) {
		t.Errorf("forwarded body = %s", gotBody)
	}

	req = httptest.NewRequest(http.MethodPost, "/muse/import", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	s.handleMuseBrowserImport(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused token status = %d, want 401", rec.Code)
	}
}

func TestMuseImportTokenExpires(t *testing.T) {
	s, _ := newMuseProxy(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("expired token must not reach upstream")
	})
	token, _, err := s.newMuseImportToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	s.museImportMu.Lock()
	s.museImportTokens[token] = time.Now().Add(-time.Minute)
	s.museImportMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/muse/import", strings.NewReader(`{"cookies":{"hatch_sess":"x"}}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.handleMuseBrowserImport(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMuseImportEndpointMiddleware(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{Port: 0, MasterKey: "test-master"})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"account":{"id":"acc_mw"},"missing_cookies":[]}`))
	}))
	t.Cleanup(upstream.Close)
	s.museBaseURL = upstream.URL
	s.museAdminKey = "muse-admin-secret"
	s.museClient = upstream.Client()

	// Preflight must be reachable without a token.
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/muse/import", nil)
	req.Header.Set("Origin", "chrome-extension://test")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", resp.StatusCode)
	}

	// POST without a token stays closed.
	resp, err = http.Post(ts.URL+"/muse/import", "application/json", strings.NewReader(`{"cookies":{"hatch_sess":"x"}}`))
	if err != nil {
		t.Fatalf("no-token post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}

	// A valid token passes middleware and is consumed by the handler.
	token, _, err := s.newMuseImportToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/muse/import",
		strings.NewReader(`{"label":"ext","cookies":{"hatch_sess":"x","hatch_gw":"y","hatch_vml":"z","hatch_native_auth_device":"w"}}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("token post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token post status = %d, want 200", resp.StatusCode)
	}
}

func TestIsPublicMediaPath(t *testing.T) {
	cases := map[string]bool{
		"/v1/media/img_abc.webp":       true,
		"/v1/media/vid_abc.mp4":        true,
		"/v1/media/":                   false,
		"/v1/media/a/b":                false,
		"/v1/media/../api/connections": false,
		"/v1/models":                   false,
		"/api/media/img_abc.webp":      false,
	}
	for input, want := range cases {
		if got := isPublicMediaPath(input); got != want {
			t.Errorf("isPublicMediaPath(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestHandleMuseMediaProxiesRandomFile(t *testing.T) {
	var gotPath, gotRange string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRange = r.URL.Path, r.Header.Get("Range")
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("webp"))
	}))
	defer upstream.Close()

	s := &Server{museBaseURL: upstream.URL, museClient: upstream.Client()}
	req := httptest.NewRequest(http.MethodGet, "/v1/media/img_test.webp", nil)
	req.SetPathValue("name", "img_test.webp")
	req.Header.Set("Range", "bytes=0-3")
	rec := httptest.NewRecorder()
	s.handleMuseMedia(rec, req)

	if rec.Code != http.StatusPartialContent || rec.Body.String() != "webp" {
		t.Fatalf("media response = %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/webp" || rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("media headers not forwarded: %v", rec.Header())
	}
	if gotPath != "/v1/media/img_test.webp" || gotRange != "bytes=0-3" {
		t.Fatalf("upstream request path/range = %q/%q", gotPath, gotRange)
	}
}

func TestHandleMuseMediaRejectsTraversal(t *testing.T) {
	s := &Server{museBaseURL: "http://127.0.0.1:1", museClient: http.DefaultClient}
	for _, name := range []string{"", "../api", "a/b"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/media/x", nil)
		req.SetPathValue("name", name)
		rec := httptest.NewRecorder()
		s.handleMuseMedia(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("name %q: got %d, want 404", name, rec.Code)
		}
	}
}

func TestHandleVideoTaskProxiesToSidecar(t *testing.T) {
	var gotPath, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task_abc","status":"completed","result":{"url":"https://x/media/vid.webp"}}`))
	}))
	defer upstream.Close()

	p := newTestProxyHandler(t)
	if _, err := p.db.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"vid-conn", "Muse video", upstream.URL, "sidecar-key", "openai", 1, 0,
	); err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	if _, err := p.db.Conn().Exec(
		`INSERT INTO discovered_models (id, connection_id, model_id, is_active) VALUES (?, ?, ?, ?)`,
		"vid-model", "vid-conn", "muse-video", 1,
	); err != nil {
		t.Fatalf("insert model: %v", err)
	}
	p.mediaClient = upstream.Client()

	req := httptest.NewRequest(http.MethodGet, "/v1/videos/task_abc", nil)
	req.SetPathValue("task_id", "task_abc")
	rec := httptest.NewRecorder()
	p.HandleVideoTask(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("video task status = %d body=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/v1/videos/task_abc" || gotAuth != "Bearer sidecar-key" {
		t.Fatalf("upstream path/auth = %q/%q", gotPath, gotAuth)
	}
	if !strings.Contains(rec.Body.String(), `"status":"completed"`) {
		t.Fatalf("body not forwarded: %s", rec.Body.String())
	}
}

func TestHandleVideoTaskRejectsInvalidID(t *testing.T) {
	p := newTestProxyHandler(t)
	for _, id := range []string{"", "../admin", "task/abc", strings.Repeat("a", 129)} {
		req := httptest.NewRequest(http.MethodGet, "/v1/videos/x", nil)
		req.SetPathValue("task_id", id)
		rec := httptest.NewRecorder()
		p.HandleVideoTask(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("id %q: got %d, want 400", id, rec.Code)
		}
	}
}

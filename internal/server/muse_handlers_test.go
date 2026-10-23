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

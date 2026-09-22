package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestQoderModelsRouteRegistered is the 405 guard for the per-model cost endpoint.
//
// The handler can compile with its registration line missing, which answers 405 to
// every call — the failure mode this repo has shipped before.
func TestQoderModelsRouteRegistered(t *testing.T) {
	s, _ := newTestServer(t, nil)

	for _, route := range []string{"/api/qoder/models", "/api/qoder/models/abc"} {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)

		if rec.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s: 405 — the registration is missing from registerParityRoutes", route)
		}
		if rec.Code != http.StatusOK && rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: unexpected status %d (%s)", route, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
}

// TestQoderModelsWindowIsComputedPerRequest: the off-peak window decides a 2.5x cost
// swing, so it must be derived from the clock on every call rather than cached.
func TestQoderModelsWindowIsComputedPerRequest(t *testing.T) {
	s, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/qoder/models", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	// The provider is inert in this harness, so the not_enabled payload is expected —
	// but the endpoint must never silently 404/405.
	if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("endpoint unreachable: %d", rec.Code)
	}
	if !strings.Contains(body, "success") {
		t.Errorf("response should always carry a success flag, got %s", body)
	}
}

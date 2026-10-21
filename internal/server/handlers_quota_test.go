package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleConnectionUsageNotFound(t *testing.T) {
	srv, ts := newTestServer(t, nil)
	defer ts.Close()

	req := httptest.NewRequest("GET", "/api/usage/non-existent-id", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleQuotaProviders(t *testing.T) {
	srv, ts := newTestServer(t, nil)
	defer ts.Close()

	req := httptest.NewRequest("GET", "/api/quota/providers", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode json: %v", err)
	}

	if res["success"] != true {
		t.Fatalf("expected success: true, got %v", res["success"])
	}
}

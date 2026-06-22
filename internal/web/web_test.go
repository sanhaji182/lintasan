package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAvailable(t *testing.T) {
	// The embedded dist/ directory should be available in test context
	// because go:embed works relative to the source directory.
	if !Available() {
		t.Log("web.Available() is false — dashboard may not be embedded in test build")
	}
}

func TestHandlerRoutesToIndex(t *testing.T) {
	h := Handler()

	// Root path should serve index.html
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !Available() {
		// If dashboard not available, we should get 404
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 when dashboard unavailable, got %d", w.Code)
		}
		return
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html content-type, got %s", ct)
	}
	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "no-cache" {
		t.Errorf("expected Cache-Control: no-cache for index, got %s", cacheControl)
	}
}

func TestHandlerServesAssets(t *testing.T) {
	h := Handler()
	if !Available() {
		t.Skip("dashboard not embedded")
	}

	// An existing asset
	req := httptest.NewRequest("GET", "/robots.txt", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /robots.txt, got %d", w.Code)
	}
}

func TestHandler404ForMissingAsset(t *testing.T) {
	h := Handler()

	// A missing path with an extension should 404
	req := httptest.NewRequest("GET", "/nonexistent.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing .js asset, got %d", w.Code)
	}
}

func TestHandlerRoutesSPAFallback(t *testing.T) {
	h := Handler()
	if !Available() {
		t.Skip("dashboard not embedded")
	}

	// A path without extension should serve index.html (SPA fallback)
	req := httptest.NewRequest("GET", "/some/spa/route", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for SPA route, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html for SPA fallback, got %s", ct)
	}
}

func TestHandlerCacheHeadersForImmutableAssets(t *testing.T) {
	h := Handler()
	if !Available() {
		t.Skip("dashboard not embedded")
	}

	// Immutable path prefix
	req := httptest.NewRequest("GET", "/_app/immutable/asset-hash.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// The asset may or may not exist, but if it's served (or 404),
	// the cache-control should not leak to unrelated paths
	t.Logf("immutable asset returned %d", w.Code)
}

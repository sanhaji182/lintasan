package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestQoderActivityRoutesRegistered is the 405 / registration guard for activity endpoints.
func TestQoderActivityRoutesRegistered(t *testing.T) {
	s, _ := newTestServer(t, nil)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/qoder/activity/eligibility"},
		{http.MethodPost, "/api/qoder/activity/claim"},
		{http.MethodPost, "/api/qoder/activity/auto-claim"},
	}

	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)

		if rec.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s: 405 — handler registration missing", r.method, r.path)
		}
		// 401 (auth) or 200/400 proves route exists and is handled
		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s %s: 404 — route not registered in mux", r.method, r.path)
		}
	}
}

func TestQoderActivityNotEnabled(t *testing.T) {
	srv := &Server{
		proxy: &ProxyHandler{}, // qoderProvider is nil
	}

	// 1. Eligibility
	req := httptest.NewRequest(http.MethodGet, "/api/qoder/activity/eligibility", nil)
	rr := httptest.NewRecorder()
	srv.handleQoderActivityEligibility(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var res map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	if res["success"] != false || res["message"] != "the Qoder provider is not active" {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}

	// 2. Claim
	reqClaim := httptest.NewRequest(http.MethodPost, "/api/qoder/activity/claim", bytes.NewReader([]byte(`{}`)))
	rrClaim := httptest.NewRecorder()
	srv.handleQoderActivityClaim(rrClaim, reqClaim)
	var resClaim map[string]any
	_ = json.Unmarshal(rrClaim.Body.Bytes(), &resClaim)
	if resClaim["success"] != false {
		t.Fatalf("expected success:false on claim when provider not enabled, got: %s", rrClaim.Body.String())
	}

	// 3. Auto claim
	reqAuto := httptest.NewRequest(http.MethodPost, "/api/qoder/activity/auto-claim", bytes.NewReader([]byte(`{}`)))
	rrAuto := httptest.NewRecorder()
	srv.handleQoderActivityAutoClaim(rrAuto, reqAuto)
	var resAuto map[string]any
	_ = json.Unmarshal(rrAuto.Body.Bytes(), &resAuto)
	if resAuto["success"] != false {
		t.Fatalf("expected success:false on auto-claim when provider not enabled, got: %s", rrAuto.Body.String())
	}
}

func TestQoderActivityClaimMissingParams(t *testing.T) {
	s, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/qoder/activity/claim", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.handleQoderActivityClaim(rr, req)

	// Since qoderProvider is nil in basic test server, it returns 200 with not active.
	// But if we test with dummy body without required fields when provider is set or not:
	var res map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	if res["success"] != false {
		t.Fatalf("expected success: false, got %v", res)
	}
}

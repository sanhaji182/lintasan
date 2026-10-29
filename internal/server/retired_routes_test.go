package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

// TestRetiredDashboardRoutesRedirect verifies that retired dashboard routes
// return an intentional HTTP redirect to their canonical destination,
// ensuring bookmarks do not 404.
func TestRetiredDashboardRoutesRedirect(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	defer ts.Close()
	_ = s

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	retiredCases := []struct {
		path         string
		wantRedirect string
	}{
		{"/dashboard/providers", "/dashboard/connections"},
		{"/dashboard/providers/", "/dashboard/connections"},
	}

	for _, tc := range retiredCases {
		resp, err := client.Get(ts.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s failed: %v", tc.path, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusMovedPermanently &&
			resp.StatusCode != http.StatusPermanentRedirect &&
			resp.StatusCode != http.StatusFound &&
			resp.StatusCode != http.StatusSeeOther {
			t.Errorf("GET %s returned status %d, expected redirect (301, 308, 302, 303)", tc.path, resp.StatusCode)
		}

		location := resp.Header.Get("Location")
		if location != tc.wantRedirect {
			t.Errorf("GET %s Location header = %q, want %q", tc.path, location, tc.wantRedirect)
		}
	}
}

// TestRetiredRoutesAbsentFromSidebarNavigation verifies that retired routes
// are completely removed from the active sidebar and navigation registry.
func TestRetiredRoutesAbsentFromSidebarNavigation(t *testing.T) {
	navPath := filepath.Join("..", "..", "frontend", "src", "lib", "navigation.ts")
	content, err := os.ReadFile(navPath)
	if err != nil {
		t.Fatalf("failed to read navigation.ts: %v", err)
	}

	navText := string(content)

	retiredRoutes := []string{
		"/dashboard/providers",
	}

	for _, route := range retiredRoutes {
		if strings.Contains(navText, "path: '"+route+"'") || strings.Contains(navText, "path: \""+route+"\"") {
			t.Errorf("retired route %q still exposed in navigation.ts navigationGroups", route)
		}
	}
}

// TestBackendPresetsPreservedForConnections verifies that retiring UI routes
// does not break or delete backend preset and category catalog APIs used by
// Add Connection.
func TestBackendPresetsPreservedForConnections(t *testing.T) {
	s, ts := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	defer ts.Close()
	_ = s

	client := ts.Client()

	req, err := http.NewRequest("GET", ts.URL+"/api/presets", nil)
	if err != nil {
		t.Fatalf("NewRequest /api/presets failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-master-key-1234567890")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/presets failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/presets status = %d, want 200", resp.StatusCode)
	}

	var presetsResult struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&presetsResult); err != nil {
		t.Fatalf("failed to decode /api/presets response: %v", err)
	}

	// Categories endpoint
	catReq, err := http.NewRequest("GET", ts.URL+"/api/preset-categories", nil)
	if err != nil {
		t.Fatalf("NewRequest /api/preset-categories failed: %v", err)
	}
	catReq.Header.Set("Authorization", "Bearer test-master-key-1234567890")

	catResp, err := client.Do(catReq)
	if err != nil {
		t.Fatalf("GET /api/preset-categories failed: %v", err)
	}
	defer catResp.Body.Close()

	if catResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/preset-categories status = %d, want 200", catResp.StatusCode)
	}
}

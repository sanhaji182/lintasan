package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
)

func TestHealthExposesVerifiedPublicCatalogCounts(t *testing.T) {
	s, _ := newTestServer(t, &config.Config{MasterKey: "test-master-key-1234567890"})
	rec := httptest.NewRecorder()
	s.handleHealth(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != 200 {
		t.Fatalf("health status = %d", rec.Code)
	}
	var body struct {
		Catalog struct {
			Providers int `json:"providers"`
			Models    int `json:"models"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if body.Catalog.Providers <= 0 || body.Catalog.Models <= 0 {
		t.Fatalf("catalog counts must be verified positive values, got providers=%d models=%d", body.Catalog.Providers, body.Catalog.Models)
	}
}

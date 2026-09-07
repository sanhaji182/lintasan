package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWatchdogRun(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer good-key" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[{"id":"model-1"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired) // 402 Fatal
		w.Write([]byte(`{"error":"insufficient_quota"}`))
	}))
	defer mockServer.Close()

	s := newRESTTestServer(t)

	// Seed 2 active connections: one good, one fatal 402
	_, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority)
		VALUES 
			('w-good', 'Good Key', ?, 'good-key', 'openai', 1, 1),
			('w-fatal', 'Fatal Key', ?, 'bad-key', 'openai', 1, 1)
	`, mockServer.URL, mockServer.URL)
	if err != nil {
		t.Fatalf("seed connections: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/connections/watchdog/run?auto_disable=true", nil)
	rec := httptest.NewRecorder()

	s.handleWatchdogRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	data, _ := out["data"].(map[string]any)
	if data["total_checked"] != float64(2) {
		t.Errorf("expected total_checked 2, got %v", data["total_checked"])
	}
	if data["healthy_count"] != float64(1) {
		t.Errorf("expected healthy_count 1, got %v", data["healthy_count"])
	}
	if data["auto_disabled"] != float64(1) {
		t.Errorf("expected auto_disabled 1, got %v", data["auto_disabled"])
	}

	// Verify w-fatal was auto-disabled
	var fatalActive int
	s.db.Conn().QueryRow("SELECT is_active FROM connections WHERE id = 'w-fatal'").Scan(&fatalActive)
	if fatalActive != 0 {
		t.Errorf("expected w-fatal to be disabled, got is_active = %d", fatalActive)
	}

	// Test GET status
	reqGet := httptest.NewRequest("GET", "/api/connections/watchdog", nil)
	recGet := httptest.NewRecorder()
	s.handleWatchdogStatus(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected status 200 on GET, got %d", recGet.Code)
	}
}

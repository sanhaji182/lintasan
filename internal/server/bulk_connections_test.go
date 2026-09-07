package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBulkTestConnections(t *testing.T) {
	// Mock upstream
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "Bearer good-key" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[{"id":"model-1"},{"id":"model-2"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_api_key"}`))
	}))
	defer mockServer.Close()

	s := newRESTTestServer(t)

	// Seed 2 connections
	_, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority)
		VALUES 
			('c-good', 'Good Conn', ?, 'good-key', 'openai', 1, 1),
			('c-bad', 'Bad Conn', ?, 'bad-key', 'openai', 1, 1)
	`, mockServer.URL, mockServer.URL)
	if err != nil {
		t.Fatalf("seed connections: %v", err)
	}

	body, _ := json.Marshal(BulkTestRequest{
		IDs: []string{"c-good", "c-bad"},
	})
	req := httptest.NewRequest("POST", "/api/connections/bulk-test", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	s.handleBulkTestConnections(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res BulkTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if res.Total != 2 {
		t.Errorf("expected total 2, got %d", res.Total)
	}
	if res.Healthy != 1 {
		t.Errorf("expected healthy 1, got %d", res.Healthy)
	}
	if res.Failed != 1 {
		t.Errorf("expected failed 1, got %d", res.Failed)
	}
	if len(res.FailedIDs) != 1 || res.FailedIDs[0] != "c-bad" {
		t.Errorf("expected failed_ids ['c-bad'], got %v", res.FailedIDs)
	}
}

func TestBulkDeleteConnections(t *testing.T) {
	s := newRESTTestServer(t)

	_, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority)
		VALUES 
			('del-1', 'Conn 1', 'http://example.com', 'k1', 'openai', 1, 1),
			('del-2', 'Conn 2', 'http://example.com', 'k2', 'openai', 1, 1),
			('keep-3', 'Conn 3', 'http://example.com', 'k3', 'openai', 1, 1)
	`)
	if err != nil {
		t.Fatalf("seed connections: %v", err)
	}

	body, _ := json.Marshal(BulkActionRequest{
		IDs: []string{"del-1", "del-2"},
	})
	req := httptest.NewRequest("POST", "/api/connections/bulk-delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	s.handleBulkDeleteConnections(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["deleted_count"] != float64(2) {
		t.Errorf("expected deleted_count 2, got %v", out["deleted_count"])
	}

	var count int
	s.db.Conn().QueryRow("SELECT COUNT(*) FROM connections").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 remaining connection, got %d", count)
	}
}

func TestBulkDisableConnections(t *testing.T) {
	s := newRESTTestServer(t)

	_, err := s.db.Conn().Exec(`
		INSERT INTO connections (id, name, base_url, api_key, format, is_active, priority)
		VALUES 
			('dis-1', 'Conn 1', 'http://example.com', 'k1', 'openai', 1, 1),
			('keep-2', 'Conn 2', 'http://example.com', 'k2', 'openai', 1, 1)
	`)
	if err != nil {
		t.Fatalf("seed connections: %v", err)
	}

	body, _ := json.Marshal(BulkActionRequest{
		IDs: []string{"dis-1"},
	})
	req := httptest.NewRequest("POST", "/api/connections/bulk-disable", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	s.handleBulkDisableConnections(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["disabled_count"] != float64(1) {
		t.Errorf("expected disabled_count 1, got %v", out["disabled_count"])
	}

	var isActive int
	s.db.Conn().QueryRow("SELECT is_active FROM connections WHERE id = 'dis-1'").Scan(&isActive)
	if isActive != 0 {
		t.Errorf("expected is_active=0 for dis-1, got %d", isActive)
	}
}

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---- Connection Pool Management ----

func createTestConnection(t *testing.T, s *Server, name, poolID string) string {
	t.Helper()
	body := map[string]any{
		"name":    name,
		"base_url": "https://api.example.com",
		"api_key": "sk-test-" + name,
		"format":  "openai",
		"priority": 1,
		"pool_id": poolID,
	}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/api/connections", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleCreateConnection(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create connection %q with pool %q: got %d, want 201\nbody: %s", name, poolID, rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	// The id can be in resp.data.id or resp.id
	if data, ok := resp["data"].(map[string]any); ok {
		return data["id"].(string)
	}
	return resp["id"].(string)
}

func TestConnectionPool_CreateWithPoolID(t *testing.T) {
	s := newRESTTestServer(t)
	id := createTestConnection(t, s, "openai-1", "openai-prod")
	if id == "" {
		t.Fatal("expected non-empty connection ID")
	}

	// Verify connection has pool_id
	rec := httptest.NewRecorder()
	s.handleGetConnections(rec, httptest.NewRequest("GET", "/api/connections", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get connections: got %d", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	conns := resp["data"].([]any)
	if len(conns) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(conns))
	}
	conn := conns[0].(map[string]any)
	if conn["pool_id"] != "openai-prod" {
		t.Errorf("expected pool_id=openai-prod, got %v", conn["pool_id"])
	}
}

func TestConnectionPool_MultipleConnectionsSamePool(t *testing.T) {
	s := newRESTTestServer(t)
	createTestConnection(t, s, "openai-1", "openai-prod")
	createTestConnection(t, s, "openai-2", "openai-prod")
	createTestConnection(t, s, "anthropic-1", "claude-pool")

	// Verify pools endpoint
	rec := httptest.NewRecorder()
	s.handleGetConnectionPools(rec, httptest.NewRequest("GET", "/api/connections/pools", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get pools: got %d, want 200", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	pools := resp["data"].([]any)
	if len(pools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(pools))
	}

	// Find openai-prod pool (should have 2 accounts)
	for _, p := range pools {
		pool := p.(map[string]any)
		if pool["pool_id"] == "openai-prod" {
			if int(pool["num_accounts"].(float64)) != 2 {
				t.Errorf("expected 2 accounts in openai-prod, got %v", pool["num_accounts"])
			}
		}
		if pool["pool_id"] == "claude-pool" {
			if int(pool["num_accounts"].(float64)) != 1 {
				t.Errorf("expected 1 account in claude-pool, got %v", pool["num_accounts"])
			}
		}
	}
}

func TestConnectionPool_PatchPoolID(t *testing.T) {
	s := newRESTTestServer(t)
	id := createTestConnection(t, s, "openai-1", "")

	// Patch pool_id
	newPool := "new-pool"
	body := map[string]any{"id": id, "pool_id": newPool}
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	s.handlePatchConnection(rec, httptest.NewRequest("PATCH", "/api/connections", bytes.NewReader(b)))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch connection pool_id: got %d, want 200", rec.Code)
	}

	// Verify updated
	rec2 := httptest.NewRecorder()
	s.handleGetConnections(rec2, httptest.NewRequest("GET", "/api/connections", nil))
	var resp map[string]any
	json.Unmarshal(rec2.Body.Bytes(), &resp)
	conns := resp["data"].([]any)
	conn := conns[0].(map[string]any)
	if conn["pool_id"] != "new-pool" {
		t.Errorf("expected pool_id=new-pool after patch, got %v", conn["pool_id"])
	}
}

func TestConnectionPool_DeleteDecrementsPool(t *testing.T) {
	s := newRESTTestServer(t)
	id1 := createTestConnection(t, s, "openai-1", "openai-prod")
	createTestConnection(t, s, "openai-2", "openai-prod")

	// Delete one connection
	rec := httptest.NewRecorder()
	s.handleDeleteConnection(rec, reqWithPath("DELETE", "/api/connections/"+id1, nil, map[string]string{"id": id1}))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete connection: got %d", rec.Code)
	}

	// Verify pool now has 1 account
	rec2 := httptest.NewRecorder()
	s.handleGetConnectionPools(rec2, httptest.NewRequest("GET", "/api/connections/pools", nil))
	var resp map[string]any
	json.Unmarshal(rec2.Body.Bytes(), &resp)
	pools := resp["data"].([]any)
	if len(pools) != 1 {
		t.Fatalf("expected 1 pool after delete, got %d", len(pools))
	}
	pool := pools[0].(map[string]any)
	if int(pool["num_accounts"].(float64)) != 1 {
		t.Errorf("expected 1 account in pool after delete, got %v", pool["num_accounts"])
	}
}

func TestConnectionPool_PoolsEmptyWhenNoPoolIDs(t *testing.T) {
	s := newRESTTestServer(t)
	createTestConnection(t, s, "openai-1", "")
	createTestConnection(t, s, "openai-2", "")

	rec := httptest.NewRecorder()
	s.handleGetConnectionPools(rec, httptest.NewRequest("GET", "/api/connections/pools", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get pools: got %d, want 200", rec.Code)
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	data := resp["data"]
	if data == nil {
		data = []any{}
	}
	pools := data.([]any)
	if len(pools) != 0 {
		t.Errorf("expected 0 pools when no pool_ids, got %d", len(pools))
	}
}

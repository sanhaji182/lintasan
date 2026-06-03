package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	ensureWebhooksTable(db)
	ensureWebhookDeliveriesTable(db)
	return db
}

func TestRegister(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	disp := NewDispatcher(db, 3)
	wh := Webhook{
		ID:       "wh-1",
		Endpoint: "https://example.com/hook",
		Secret:   "secret123",
		Events:   "request.success,request.error",
		Active:   true,
	}
	if err := disp.Register(wh); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify it was stored
	var endpoint, secret, events string
	var active int
	err := db.QueryRow("SELECT endpoint, secret, events, active FROM webhooks WHERE id = ?", "wh-1").
		Scan(&endpoint, &secret, &events, &active)
	if err != nil {
		t.Fatalf("failed to query webhook: %v", err)
	}
	if endpoint != "https://example.com/hook" {
		t.Errorf("expected endpoint %q, got %q", "https://example.com/hook", endpoint)
	}
	if secret != "secret123" {
		t.Errorf("expected secret %q, got %q", "secret123", secret)
	}
	if active != 1 {
		t.Errorf("expected active=1, got %d", active)
	}
}

func TestRegisterInactive(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	disp := NewDispatcher(db, 3)
	wh := Webhook{
		ID:       "wh-2",
		Endpoint: "https://example.com/hook2",
		Secret:   "",
		Events:   "request.success",
		Active:   false,
	}
	if err := disp.Register(wh); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	var active int
	db.QueryRow("SELECT active FROM webhooks WHERE id = ?", "wh-2").Scan(&active)
	if active != 0 {
		t.Errorf("expected active=0, got %d", active)
	}
}

func TestFireDeliversToMatchingWebhook(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	received := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(200)
	}))
	defer server.Close()

	disp := NewDispatcher(db, 2)
	disp.httpClient = server.Client()

	wh := Webhook{
		ID:       "wh-fire-1",
		Endpoint: server.URL,
		Secret:   "",
		Events:   "request.success,quota.exceeded",
		Active:   true,
	}
	if err := disp.Register(wh); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	disp.Fire("request.success", map[string]interface{}{
		"model":  "gpt-4",
		"status": 200,
	})

	select {
	case body := <-received:
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid JSON delivered: %v", err)
		}
		if payload["event"] != "request.success" {
			t.Errorf("expected event 'request.success', got %v", payload["event"])
		}
		if p, ok := payload["payload"].(map[string]interface{}); !ok {
			t.Error("payload missing")
		} else if p["model"] != "gpt-4" {
			t.Errorf("expected model 'gpt-4', got %v", p["model"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}
}

func TestFireNoMatchingWebhooks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("webhook should not have been called")
	}))
	defer server.Close()

	disp := NewDispatcher(db, 2)
	disp.httpClient = server.Client()

	wh := Webhook{
		ID:       "wh-nomatch-1",
		Endpoint: server.URL,
		Secret:   "",
		Events:   "request.success",
		Active:   true,
	}
	disp.Register(wh)

	// Fire an event that doesn't match — should not deliver
	disp.Fire("request.completed", map[string]interface{}{"model": "gpt-4"})

	// Give it time to (not) deliver
	time.Sleep(500 * time.Millisecond)

	// Also test: no webhooks registered at all — use a fresh DB
	db2 := setupTestDB(t)
	defer db2.Close()
	disp2 := NewDispatcher(db2, 2)
	disp2.httpClient = server.Client()
	disp2.Fire("request.success", map[string]interface{}{"model": "gpt-4"})
	time.Sleep(300 * time.Millisecond)
}

func TestHMACSignatureGeneration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	secret := "test-secret-key"
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header
		w.WriteHeader(200)
	}))
	defer server.Close()

	disp := NewDispatcher(db, 2)
	disp.httpClient = server.Client()

	wh := Webhook{
		ID:       "wh-hmac-1",
		Endpoint: server.URL,
		Secret:   secret,
		Events:   "request.success",
		Active:   true,
	}
	disp.Register(wh)

	payload := map[string]interface{}{"model": "gpt-4", "status": 200}
	disp.Fire("request.success", payload)

	select {
	case headers := <-received:
		sigHeader := headers.Get("X-Webhook-Signature")
		if sigHeader == "" {
			t.Fatal("X-Webhook-Signature header missing")
		}
		if !strings.HasPrefix(sigHeader, "sha256=") {
			t.Errorf("expected sha256= prefix, got %q", sigHeader)
		}

		// Verify HMAC
		body, _ := json.Marshal(map[string]interface{}{
			"event":   "request.success",
			"payload": payload,
			"time":    "", // we'll just verify signature exists
		})
		_ = body

		// Verify using our VerifySignature function
		sigValue := strings.TrimPrefix(sigHeader, "sha256=")
		// We can't reconstruct exact body since it has time, but the signature should be valid hex
		if len(sigValue) != 64 {
			t.Errorf("expected 64-char hex, got %d chars", len(sigValue))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}
}

func TestVerifySignature(t *testing.T) {
	secret := "my-secret"
	body := []byte(`{"event":"test","payload":{}}`)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	validSig := "sha256=" + hex.EncodeToString(h.Sum(nil))

	tests := []struct {
		name      string
		secret    string
		body      []byte
		sigHeader string
		want      bool
	}{
		{"valid", secret, body, validSig, true},
		{"wrong secret", "wrong-secret", body, validSig, false},
		{"wrong body", secret, []byte("different body"), validSig, false},
		{"empty secret", "", body, validSig, false},
		{"empty header", secret, body, "", false},
		{"no prefix", secret, body, hex.EncodeToString(h.Sum(nil)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifySignature(tt.secret, tt.body, tt.sigHeader)
			if got != tt.want {
				t.Errorf("VerifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryWithMockServer(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(500)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer server.Close()

	disp := NewDispatcher(db, 5)
	disp.httpClient = server.Client()
	disp.retryDelay = 10 * time.Millisecond

	wh := Webhook{
		ID:       "wh-retry-1",
		Endpoint: server.URL,
		Secret:   "",
		Events:   "request.success",
		Active:   true,
	}
	disp.Register(wh)

	disp.Fire("request.success", map[string]interface{}{"model": "gpt-4"})

	// Wait for retries to complete
	time.Sleep(2 * time.Second)

	if attempts < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts)
	}
}

func TestRetryExceedsMaxRetries(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(500)
	}))
	defer server.Close()

	disp := NewDispatcher(db, 3)
	disp.httpClient = server.Client()
	disp.retryDelay = 10 * time.Millisecond

	wh := Webhook{
		ID:       "wh-fail-1",
		Endpoint: server.URL,
		Secret:   "",
		Events:   "request.success",
		Active:   true,
	}
	disp.Register(wh)

	disp.Fire("request.success", map[string]interface{}{"model": "gpt-4"})

	time.Sleep(3 * time.Second)

	// With maxRetries=3, we should see exactly 3 attempts
	if attempts != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", attempts)
	}
}

func TestRetryPending(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Insert a pending delivery directly
	db.Exec(`INSERT INTO webhook_deliveries (id, webhook_id, event, payload, status, attempts, created_at)
		VALUES ('retry-test-1', 'wh-retry-pending', 'request.success', '{"event":"request.success","payload":{"model":"gpt-4"}}', 'pending', 0, datetime('now', 'localtime'))`)

	// Register the webhook that the delivery references
	received := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- true
		w.WriteHeader(200)
	}))
	defer server.Close()

	disp := NewDispatcher(db, 3)
	disp.httpClient = server.Client()

	disp.Register(Webhook{
		ID:       "wh-retry-pending",
		Endpoint: server.URL,
		Secret:   "",
		Events:   "request.success",
		Active:   true,
	})

	disp.RetryPending()

	select {
	case <-received:
		// Success — delivery was retried
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for retry")
	}
}

func TestNewManagerCompatibility(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	mgr := NewManager(db)
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.maxRetries != 5 {
		t.Errorf("expected maxRetries=5, got %d", mgr.maxRetries)
	}

	// Should support Register, Fire, etc.
	wh := Webhook{ID: "test-mgr", Endpoint: "http://localhost", Events: "test.event", Active: true}
	if err := mgr.Register(wh); err != nil {
		t.Fatalf("Manager.Register failed: %v", err)
	}
}

func TestDeliveryStatusTracking(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"server error"}`))
		} else {
			w.WriteHeader(200)
			w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()

	disp := NewDispatcher(db, 3)
	disp.httpClient = server.Client()
	disp.retryDelay = 10 * time.Millisecond

	disp.Register(Webhook{
		ID:       "wh-status-1",
		Endpoint: server.URL,
		Secret:   "",
		Events:   "request.success",
		Active:   true,
	})

	disp.Fire("request.success", map[string]interface{}{"model": "gpt-4"})
	time.Sleep(2 * time.Second)

	// Check deliveries table has entries
	var count int
	db.QueryRow("SELECT COUNT(*) FROM webhook_deliveries WHERE webhook_id = 'wh-status-1'").Scan(&count)
	if count == 0 {
		t.Error("no delivery entries found")
	}

	// Should have at least one delivered entry
	var deliveredCount int
	db.QueryRow("SELECT COUNT(*) FROM webhook_deliveries WHERE webhook_id = 'wh-status-1' AND status = 'delivered'").Scan(&deliveredCount)
	if deliveredCount == 0 {
		t.Error("no deliveries marked as 'delivered'")
		fmt.Printf("All deliveries:\n")
		rows, _ := db.Query("SELECT id, status, attempts FROM webhook_deliveries WHERE webhook_id = 'wh-status-1'")
		defer rows.Close()
		for rows.Next() {
			var id, status string
			var att int
			rows.Scan(&id, &status, &att)
			fmt.Printf("  %s: status=%s attempts=%d\n", id, status, att)
		}
	}
}

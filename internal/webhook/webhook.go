package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

// Manager manages webhook registrations, delivery, and retry.
// Alias: NewDispatcher is also available.
type Manager struct {
	db         *sql.DB
	httpClient *http.Client
	maxRetries int
	retryDelay time.Duration
}

// Webhook represents a registered webhook endpoint
type Webhook struct {
	ID        string
	Endpoint  string
	Secret    string // HMAC secret
	Events    string // comma-separated: request.success,request.error
	Active    bool
	CreatedAt string
}

// Delivery represents a single webhook delivery attempt with status tracking
type Delivery struct {
	ID          string
	WebhookID   string
	Event       string
	Payload     []byte
	Status      string // "pending", "delivered", "failed"
	Attempts    int
	LastAttempt string
	Response    string
	CreatedAt   string
}

// NewManager creates a new webhook Manager (alias for NewDispatcher)
func NewManager(db *sql.DB) *Manager {
	return NewDispatcher(db, 5)
}

// NewDispatcher creates a new webhook Dispatcher with the given max retries
func NewDispatcher(db *sql.DB, maxRetries int) *Manager {
	return &Manager{
		db:         db,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		maxRetries: maxRetries,
		retryDelay: 1 * time.Second,
	}
}

// Register saves a webhook registration to the database
func (m *Manager) Register(wh Webhook) error {
	ensureWebhooksTable(m.db)
	ensureWebhookDeliveriesTable(m.db)
	active := 0
	if wh.Active {
		active = 1
	}
	_, err := m.db.Exec(`
		INSERT OR REPLACE INTO webhooks (id, endpoint, secret, events, active, created_at)
		VALUES (?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM webhooks WHERE id=?), datetime('now', 'localtime')))
	`, wh.ID, wh.Endpoint, wh.Secret, wh.Events, active, wh.ID)
	return err
}

// Fire triggers async webhook delivery for all matching subscriptions.
// Spawns a goroutine per matching webhook.
func (m *Manager) Fire(event string, payload map[string]interface{}) {
	go m.deliverToMatching(event, payload)
}

// RetryPending retries all pending webhook deliveries (call periodically or on startup)
func (m *Manager) RetryPending() {
	ensureWebhookDeliveriesTable(m.db)
	rows, err := m.db.Query(`
		SELECT id, webhook_id, event, payload, status, attempts
		FROM webhook_deliveries
		WHERE status = 'pending' OR status = 'failed'
		ORDER BY created_at ASC
		LIMIT 100
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	var deliveries []*Delivery
	for rows.Next() {
		d := &Delivery{}
		var payloadStr string
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &payloadStr, &d.Status, &d.Attempts); err != nil {
			continue
		}
		d.Payload = []byte(payloadStr)
		deliveries = append(deliveries, d)
	}

	for _, d := range deliveries {
		go m.retryDelivery(d)
	}
}

// deliverToMatching finds all active webhooks subscribed to this event and delivers
func (m *Manager) deliverToMatching(event string, payload map[string]interface{}) {
	ensureWebhooksTable(m.db)

	rows, err := m.db.Query(`
		SELECT id, endpoint, secret, events FROM webhooks
		WHERE active = 1
	`)
	if err != nil {
		log.Printf("[Webhook] Failed to query webhooks: %v", err)
		return
	}
	defer rows.Close()

	var matching []Webhook
	for rows.Next() {
		var wh Webhook
		if err := rows.Scan(&wh.ID, &wh.Endpoint, &wh.Secret, &wh.Events); err != nil {
			continue
		}
		// Match events: comma-separated "request.success,request.error"
		for _, ev := range strings.Split(wh.Events, ",") {
			if strings.TrimSpace(ev) == event {
				matching = append(matching, wh)
				break
			}
		}
	}

	if len(matching) == 0 {
		return
	}

	for _, wh := range matching {
		go m.deliverOne(wh, event, payload, 0)
	}
}

// deliverOne sends a single webhook delivery with retry
func (m *Manager) deliverOne(wh Webhook, event string, payload map[string]interface{}, attempt int) {
	body, _ := json.Marshal(map[string]interface{}{
		"event":   event,
		"payload": payload,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})

	deliveryID := fmt.Sprintf("del_%d_%d", time.Now().UnixNano(), attempt)
	ensureWebhookDeliveriesTable(m.db)

	// Record delivery attempt as pending
	m.db.Exec(`
		INSERT INTO webhook_deliveries (id, webhook_id, event, payload, status, attempts, response, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?, '', datetime('now', 'localtime'))
		ON CONFLICT(id) DO UPDATE SET attempts = ?, status = 'pending', last_attempt = datetime('now', 'localtime')
	`, deliveryID, wh.ID, event, string(body), attempt+1, attempt+1)

	req, err := http.NewRequest("POST", wh.Endpoint, bytes.NewReader(body))
	if err != nil {
		m.updateDeliveryStatus(deliveryID, "failed", fmt.Sprintf("request creation error: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", event)
	req.Header.Set("X-Webhook-Time", fmt.Sprintf("%d", time.Now().Unix()))

	// HMAC-SHA256 signing
	if wh.Secret != "" {
		h := hmac.New(sha256.New, []byte(wh.Secret))
		h.Write(body)
		req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(h.Sum(nil)))
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		// Retry with exponential backoff
		if attempt < m.maxRetries-1 {
			backoff := m.retryDelay * time.Duration(math.Pow(2, float64(attempt)))
			log.Printf("[Webhook] Delivery attempt %d failed for %s: %v, retrying in %v", attempt+1, wh.Endpoint, err, backoff)
			time.Sleep(backoff)
			m.deliverOne(wh, event, payload, attempt+1)
			return
		}
		m.updateDeliveryStatus(deliveryID, "failed", err.Error())
		log.Printf("[Webhook] Delivery failed for %s after %d attempts: %v", wh.Endpoint, m.maxRetries, err)
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody := make([]byte, 4096)
	n, _ := resp.Body.Read(respBody)
	respStr := string(respBody[:n])
	if n >= 4096 {
		respStr += "..."
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		m.updateDeliveryStatus(deliveryID, "delivered", respStr)
		log.Printf("[Webhook] Delivered %s to %s (status %d)", event, wh.Endpoint, resp.StatusCode)
	} else {
		if attempt < m.maxRetries-1 {
			backoff := m.retryDelay * time.Duration(math.Pow(2, float64(attempt)))
			log.Printf("[Webhook] Delivery attempt %d returned %d for %s, retrying in %v", attempt+1, resp.StatusCode, wh.Endpoint, backoff)
			time.Sleep(backoff)
			m.deliverOne(wh, event, payload, attempt+1)
			return
		}
		m.updateDeliveryStatus(deliveryID, "failed", respStr)
		log.Printf("[Webhook] Delivery failed %s to %s (status %d): %s", event, wh.Endpoint, resp.StatusCode, respStr)
	}
}

// retryDelivery retries a pending/failed delivery
func (m *Manager) retryDelivery(d *Delivery) {
	var wh Webhook
	err := m.db.QueryRow(`SELECT id, endpoint, secret, events FROM webhooks WHERE id = ?`, d.WebhookID).
		Scan(&wh.ID, &wh.Endpoint, &wh.Secret, &wh.Events)
	if err != nil {
		m.updateDeliveryStatus(d.ID, "failed", fmt.Sprintf("webhook not found: %v", err))
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(d.Payload, &payload); err != nil {
		m.updateDeliveryStatus(d.ID, "failed", fmt.Sprintf("invalid payload: %v", err))
		return
	}

	m.deliverOne(wh, d.Event, payload, d.Attempts)
}

func (m *Manager) updateDeliveryStatus(deliveryID, status, response string) {
	m.db.Exec(`
		UPDATE webhook_deliveries SET status = ?, response = ?, last_attempt = datetime('now', 'localtime') WHERE id = ?
	`, status, response, deliveryID)
}

// EnsureTables creates required tables if they don't exist
func (m *Manager) EnsureTables() error {
	if err := ensureWebhooksTable(m.db); err != nil {
		return err
	}
	return ensureWebhookDeliveriesTable(m.db)
}

func ensureWebhooksTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			endpoint TEXT NOT NULL,
			secret TEXT DEFAULT '',
			events TEXT NOT NULL,
			active INTEGER DEFAULT 1,
			created_at TEXT DEFAULT (datetime('now', 'localtime'))
		)
	`)
	return err
}

func ensureWebhookDeliveriesTable(db *sql.DB) error {
	// Add payload and attempts columns if migrating from old schema
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id TEXT PRIMARY KEY,
			webhook_id TEXT,
			event TEXT,
			payload TEXT DEFAULT '',
			status TEXT DEFAULT 'pending',
			attempts INTEGER DEFAULT 0,
			response TEXT DEFAULT '',
			last_attempt TEXT,
			created_at TEXT DEFAULT (datetime('now', 'localtime'))
		)
	`)
	return err
}

// VerifySignature checks an incoming webhook signature against the secret
func VerifySignature(secret string, body []byte, signatureHeader string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}
	parts := strings.SplitN(signatureHeader, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return false
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	expected := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

package quota

import (
	"database/sql"
	"sync"
	"time"
)

// QuotaTracker tracks per-connection and global token/resource usage.
type QuotaTracker struct {
	db       *sql.DB
	mu       sync.RWMutex
	limits   map[string]*QuotaLimit
}

// QuotaLimit defines limits for a connection.
type QuotaLimit struct {
	MaxTokensPerDay   int64
	MaxTokensPerMonth int64
	MaxRequestsPerDay  int64
	MaxRequestsPerMonth int64
}

// NewTracker creates a new QuotaTracker backed by the given DB connection.
func NewTracker(db *sql.DB) *QuotaTracker {
	return &QuotaTracker{
		db:     db,
		limits: make(map[string]*QuotaLimit),
	}
}

// Allow checks whether a connection has remaining quota and records usage.
func (qt *QuotaTracker) Allow(connID string, tokens int) bool {
	qt.mu.RLock()
	limit, hasLimit := qt.limits[connID]
	qt.mu.RUnlock()

	if !hasLimit {
		return true // no limit configured
	}

	usage := GetQuota(qt.db, connID)
	tokensToday, _ := usage["tokens_today"].(int)

	if limit.MaxTokensPerDay > 0 && int64(tokensToday+int(tokens)) > limit.MaxTokensPerDay {
		return false
	}

	return true
}

// SetLimit sets a quota limit for a connection.
func (qt *QuotaTracker) SetLimit(connID string, limit *QuotaLimit) {
	qt.mu.Lock()
	qt.limits[connID] = limit
	qt.mu.Unlock()
}

// GetLimit returns the quota limit for a connection.
func (qt *QuotaTracker) GetLimit(connID string) *QuotaLimit {
	qt.mu.RLock()
	defer qt.mu.RUnlock()
	return qt.limits[connID]
}

func InitQuotaSchema(db *sql.DB) {
	db.Exec(`CREATE TABLE IF NOT EXISTS quota_usage (
		connection_id TEXT PRIMARY KEY,
		tokens_today INTEGER DEFAULT 0,
		tokens_month INTEGER DEFAULT 0,
		requests_today INTEGER DEFAULT 0,
		requests_month INTEGER DEFAULT 0,
		last_reset_day TEXT,
		last_reset_month TEXT,
		updated_at TEXT DEFAULT (datetime('now', 'localtime'))
	)`)
}

func RecordQuota(db *sql.DB, connID string, tokens int) {
	day := time.Now().Format("2006-01-02")
	month := time.Now().Format("2006-01")
	
	res, err := db.Exec(`
		UPDATE quota_usage 
		SET tokens_today=tokens_today+?, tokens_month=tokens_month+?, requests_today=requests_today+1, requests_month=requests_month+1, updated_at=datetime('now', 'localtime')
		WHERE connection_id=? AND last_reset_day=? AND last_reset_month=?
	`, tokens, tokens, connID, day, month)
	
	if err != nil || res == nil {
		// Table may not exist, try to create it
		InitQuotaSchema(db)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		db.Exec(`
			INSERT OR REPLACE INTO quota_usage(connection_id, tokens_today, tokens_month, requests_today, requests_month, last_reset_day, last_reset_month)
			VALUES(?, ?, ?, 1, 1, ?, ?)
		`, connID, tokens, tokens, day, month)
	}
}

func GetQuota(db *sql.DB, connID string) map[string]any {
	day := time.Now().Format("2006-01-02")
	month := time.Now().Format("2006-01")
	var tToday, tMonth, rToday, rMonth int
	var lDay, lMonth string
	err := db.QueryRow("SELECT tokens_today, tokens_month, requests_today, requests_month, last_reset_day, last_reset_month FROM quota_usage WHERE connection_id=?", connID).Scan(&tToday, &tMonth, &rToday, &rMonth, &lDay, &lMonth)
	if err != nil { return map[string]any{"requests_today":0, "tokens_today":0} }
	
	if lDay != day { tToday = 0; rToday = 0 }
	if lMonth != month { tMonth = 0; rMonth = 0 }
	return map[string]any{
		"tokens_today": tToday, "tokens_month": tMonth,
		"requests_today": rToday, "requests_month": rMonth,
	}
}

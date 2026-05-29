package cost

import (
	"database/sql"
	"sync"
	"time"
)

// Entry represents a single cost-tracking record.
type Entry struct {
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model"`
	ConnectionID string    `json:"connection_id"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	InputCostUSD float64   `json:"input_cost_usd"`
	OutputCostUSD float64  `json:"output_cost_usd"`
}

// Summary aggregates costs over a time period.
type Summary struct {
	TotalRequests   int                          `json:"total_requests"`
	TotalInputTokens int                         `json:"total_input_tokens"`
	TotalOutputTokens int                        `json:"total_output_tokens"`
	TotalCostUSD    float64                      `json:"total_cost_usd"`
	ByModel         map[string]*ModelSummary     `json:"by_model"`
	ByConnection    map[string]*ConnSummary      `json:"by_connection"`
}

// ModelSummary aggregates per-model costs.
type ModelSummary struct {
	Requests     int     `json:"requests"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// ConnSummary aggregates per-connection costs.
type ConnSummary struct {
	Requests int     `json:"requests"`
	CostUSD  float64 `json:"cost_usd"`
}

// SavingsEntry represents a single cost-saving event.
type SavingsEntry struct {
	ID             int64   `json:"id"`
	Timestamp      string  `json:"timestamp"`
	Category       string  `json:"category"` // compression, routing, cache, free
	Model          string  `json:"model"`
	OriginalModel  string  `json:"original_model"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	OriginalCost   float64 `json:"original_cost_usd"`
	ActualCost     float64 `json:"actual_cost_usd"`
	SavingsUSD     float64 `json:"savings_usd"`
	SavingsPercent float64 `json:"savings_percent"`
	ConnectionID   string  `json:"connection_id"`
}

// SavingsSummary aggregates savings data.
type SavingsSummary struct {
	TotalSaved         float64                   `json:"total_saved_usd"`
	TotalOriginalCost  float64                   `json:"total_original_cost_usd"`
	TotalActualCost    float64                   `json:"total_actual_cost_usd"`
	SavingsPercent     float64                   `json:"savings_percent"`
	TotalRequests      int                       `json:"total_savings_events"`
	ByCategory         map[string]*CategorySavings `json:"by_category"`
	ByModel            map[string]*ModelSavings    `json:"by_model"`
}

// CategorySavings aggregates savings by category.
type CategorySavings struct {
	SavingsUSD float64 `json:"savings_usd"`
	Events     int     `json:"events"`
	Percent    float64 `json:"avg_savings_percent"`
}

// ModelSavings aggregates savings by model.
type ModelSavings struct {
	SavingsUSD    float64 `json:"savings_usd"`
	Events        int     `json:"events"`
	OriginalCost  float64 `json:"original_cost_usd"`
	ActualCost    float64 `json:"actual_cost_usd"`
}

// LeaderboardEntry ranks entities by savings.
type LeaderboardEntry struct {
	Rank         int     `json:"rank"`
	Name         string  `json:"name"`
	SavingsUSD   float64 `json:"savings_usd"`
	Events       int     `json:"events"`
	AvgSaving    float64 `json:"avg_saving_per_event"`
}

// Tracker records and queries cost data.
type Tracker struct {
	db      *sql.DB
	mu      sync.RWMutex
	pricing map[string]ModelPrice
	calc    *Calculator
}

// ModelPrice holds per-token pricing for a model.
type ModelPrice struct {
	InputPrice  float64
	OutputPrice float64
}

// New creates a new cost Tracker.
func New(database *sql.DB) *Tracker {
	t := &Tracker{
		db:      database,
		pricing: make(map[string]ModelPrice),
		calc:    NewCalculator(),
	}
	t.initSchema()
	t.loadPricing()
	return t
}

// Calculator returns the tracker's cost calculator.
func (t *Tracker) Calculator() *Calculator {
	return t.calc
}

func (t *Tracker) initSchema() {
	t.db.Exec(`CREATE TABLE IF NOT EXISTS cost_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		model TEXT NOT NULL,
		connection_id TEXT NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		input_cost_usd REAL DEFAULT 0,
		output_cost_usd REAL DEFAULT 0
	)`)
	t.db.Exec(`CREATE TABLE IF NOT EXISTS cost_savings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		category TEXT NOT NULL,
		model TEXT NOT NULL,
		original_model TEXT NOT NULL DEFAULT '',
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		original_cost_usd REAL DEFAULT 0,
		actual_cost_usd REAL DEFAULT 0,
		savings_usd REAL DEFAULT 0,
		savings_percent REAL DEFAULT 0,
		connection_id TEXT DEFAULT ''
	)`)
	t.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cost_savings_ts ON cost_savings(timestamp)`)
	t.db.Exec(`CREATE INDEX IF NOT EXISTS idx_cost_savings_cat ON cost_savings(category)`)
}

func (t *Tracker) loadPricing() {
	defaults := map[string]ModelPrice{
		"gpt-4o":                    {2.50, 10.00},
		"gpt-4o-mini":               {0.15, 0.60},
		"gpt-4-turbo":               {10.00, 30.00},
		"o1":                        {15.00, 60.00},
		"o1-mini":                   {3.00, 12.00},
		"claude-sonnet-4-20250514":  {3.00, 15.00},
		"claude-opus-4-20250514":    {15.00, 75.00},
		"claude-haiku-3-5":          {0.80, 4.00},
		"deepseek-v4-pro":           {0.55, 2.20},
		"deepseek-v3":               {0.27, 1.10},
		"deepseek-r1":               {0.55, 2.19},
		"gemini-2.5-pro":            {1.25, 10.00},
		"gemini-2.5-flash":          {0.15, 0.60},
		"llama-3.3-70b":             {0.35, 0.40},
		"llama-3.1-405b":            {1.00, 1.00},
		"mistral-large-2":           {2.00, 6.00},
		"mistral-small":             {0.10, 0.30},
		"qwen-2.5-72b":              {0.30, 0.60},
	}
	for k, v := range defaults {
		if _, ok := t.pricing[k]; !ok {
			t.pricing[k] = v
		}
	}
}

// RegisterPricing adds custom model pricing.
func (t *Tracker) RegisterPricing(modelID string, price ModelPrice) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pricing[modelID] = price
}

// Record logs a cost entry.
func (t *Tracker) Record(model, connID string, inputTokens, outputTokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	price, ok := t.pricing[model]
	if !ok {
		price = ModelPrice{0, 0}
	}
	inputCost := float64(inputTokens) / 1_000_000 * price.InputPrice
	outputCost := float64(outputTokens) / 1_000_000 * price.OutputPrice

	now := time.Now().UTC().Format(time.RFC3339)
	t.db.Exec(
		"INSERT INTO cost_entries (timestamp, model, connection_id, input_tokens, output_tokens, input_cost_usd, output_cost_usd) VALUES (?,?,?,?,?,?,?)",
		now, model, connID, inputTokens, outputTokens, inputCost, outputCost,
	)
}

// RecordSavings logs a savings event.
func (t *Tracker) RecordSavings(entry SavingsEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	t.db.Exec(
		`INSERT INTO cost_savings (timestamp, category, model, original_model, input_tokens, output_tokens, original_cost_usd, actual_cost_usd, savings_usd, savings_percent, connection_id) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		entry.Timestamp, entry.Category, entry.Model, entry.OriginalModel,
		entry.InputTokens, entry.OutputTokens, entry.OriginalCost, entry.ActualCost,
		entry.SavingsUSD, entry.SavingsPercent, entry.ConnectionID,
	)
}

// Summary returns aggregated costs for today.
func (t *Tracker) Summary() *Summary {
	rows, err := t.db.Query(`SELECT model, connection_id, COUNT(*) as requests,
		SUM(input_tokens), SUM(output_tokens),
		SUM(input_cost_usd), SUM(output_cost_usd)
		FROM cost_entries WHERE date(timestamp) = date('now')
		GROUP BY model, connection_id`)
	if err != nil {
		return &Summary{ByModel: map[string]*ModelSummary{}, ByConnection: map[string]*ConnSummary{}}
	}
	defer rows.Close()

	return scanSummary(rows)
}

// SummarySince returns aggregated costs since a given time.
func (t *Tracker) SummarySince(since time.Duration) *Summary {
	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)
	rows, err := t.db.Query(`SELECT model, connection_id, COUNT(*) as requests,
		SUM(input_tokens), SUM(output_tokens),
		SUM(input_cost_usd), SUM(output_cost_usd)
		FROM cost_entries WHERE timestamp >= ?
		GROUP BY model, connection_id`, cutoff)
	if err != nil {
		return &Summary{ByModel: map[string]*ModelSummary{}, ByConnection: map[string]*ConnSummary{}}
	}
	defer rows.Close()

	return scanSummary(rows)
}

func scanSummary(rows *sql.Rows) *Summary {
	s := &Summary{
		ByModel:      make(map[string]*ModelSummary),
		ByConnection: make(map[string]*ConnSummary),
	}

	for rows.Next() {
		var model, connID string
		var reqs, inT, outT int
		var inC, outC float64
		rows.Scan(&model, &connID, &reqs, &inT, &outT, &inC, &outC)

		s.TotalRequests += reqs
		s.TotalInputTokens += inT
		s.TotalOutputTokens += outT
		s.TotalCostUSD += inC + outC

		ms, ok := s.ByModel[model]
		if !ok {
			ms = &ModelSummary{}
			s.ByModel[model] = ms
		}
		ms.Requests += reqs
		ms.InputTokens += inT
		ms.OutputTokens += outT
		ms.CostUSD += inC + outC

		cs, ok := s.ByConnection[connID]
		if !ok {
			cs = &ConnSummary{}
			s.ByConnection[connID] = cs
		}
		cs.Requests += reqs
		cs.CostUSD += inC + outC
	}
	return s
}

// SavingsSummarySince returns aggregated savings since a given time.
func (t *Tracker) SavingsSummarySince(since time.Duration) *SavingsSummary {
	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)
	return t.savingsSummaryWhere("timestamp >= ?", cutoff)
}

// SavingsSummaryAll returns all-time savings summary.
func (t *Tracker) SavingsSummaryAll() *SavingsSummary {
	return t.savingsSummaryWhere("1=1")
}

func (t *Tracker) SavingsSummaryByModel(since time.Duration) *SavingsSummary {
	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)
	return t.savingsSummaryWhere("timestamp >= ?", cutoff)
}

func (t *Tracker) savingsSummaryWhere(where string, args ...any) *SavingsSummary {
	ss := &SavingsSummary{
		ByCategory: make(map[string]*CategorySavings),
		ByModel:    make(map[string]*ModelSavings),
	}

	// Aggregate totals
	row := t.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(original_cost_usd),0), COALESCE(SUM(actual_cost_usd),0), COALESCE(SUM(savings_usd),0) FROM cost_savings WHERE `+where, args...)
	var count int
	row.Scan(&count, &ss.TotalOriginalCost, &ss.TotalActualCost, &ss.TotalSaved)
	ss.TotalRequests = count
	if ss.TotalOriginalCost > 0 {
		ss.SavingsPercent = (ss.TotalSaved / ss.TotalOriginalCost) * 100
	}

	// By category
	rows, err := t.db.Query(`SELECT category, COUNT(*), COALESCE(SUM(savings_usd),0), COALESCE(AVG(savings_percent),0) FROM cost_savings WHERE `+where+` GROUP BY category`, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat string
			var events int
			var savings, pct float64
			rows.Scan(&cat, &events, &savings, &pct)
			ss.ByCategory[cat] = &CategorySavings{SavingsUSD: savings, Events: events, Percent: pct}
		}
	}

	// By model
	rows2, err := t.db.Query(`SELECT model, COUNT(*), COALESCE(SUM(savings_usd),0), COALESCE(SUM(original_cost_usd),0), COALESCE(SUM(actual_cost_usd),0) FROM cost_savings WHERE `+where+` GROUP BY model ORDER BY SUM(savings_usd) DESC`, args...)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var model string
			var events int
			var savings, origC, actC float64
			rows2.Scan(&model, &events, &savings, &origC, &actC)
			ss.ByModel[model] = &ModelSavings{SavingsUSD: savings, Events: events, OriginalCost: origC, ActualCost: actC}
		}
	}

	return ss
}

// SavingsHistory returns paginated savings entries.
func (t *Tracker) SavingsHistory(limit, offset int) []SavingsEntry {
	if limit <= 0 {
		limit = 50
	}
	rows, err := t.db.Query(`SELECT id, timestamp, category, model, original_model, input_tokens, output_tokens, original_cost_usd, actual_cost_usd, savings_usd, savings_percent, connection_id FROM cost_savings ORDER BY timestamp DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return []SavingsEntry{}
	}
	defer rows.Close()

	var entries []SavingsEntry
	for rows.Next() {
		var e SavingsEntry
		rows.Scan(&e.ID, &e.Timestamp, &e.Category, &e.Model, &e.OriginalModel,
			&e.InputTokens, &e.OutputTokens, &e.OriginalCost, &e.ActualCost,
			&e.SavingsUSD, &e.SavingsPercent, &e.ConnectionID)
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []SavingsEntry{}
	}
	return entries
}

// SavingsLeaderboard returns top savers ranked by total savings.
func (t *Tracker) SavingsLeaderboard(by string, limit int) []LeaderboardEntry {
	if limit <= 0 {
		limit = 10
	}
	groupBy := "model"
	if by == "connection" {
		groupBy = "connection_id"
	} else if by == "category" {
		groupBy = "category"
	}

	rows, err := t.db.Query(`SELECT `+groupBy+`, COUNT(*), COALESCE(SUM(savings_usd),0), COALESCE(AVG(savings_usd),0) FROM cost_savings GROUP BY `+groupBy+` ORDER BY SUM(savings_usd) DESC LIMIT ?`, limit)
	if err != nil {
		return []LeaderboardEntry{}
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	rank := 1
	for rows.Next() {
		var name string
		var events int
		var totalSavings, avgSaving float64
		rows.Scan(&name, &events, &totalSavings, &avgSaving)
		entries = append(entries, LeaderboardEntry{
			Rank:       rank,
			Name:       name,
			SavingsUSD: totalSavings,
			Events:     events,
			AvgSaving:  avgSaving,
		})
		rank++
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}
	return entries
}

package cost

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestNewTracker(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)
	if tracker == nil {
		t.Fatal("expected tracker")
	}
}

func TestRecordCost(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	tracker.Record("gpt-4o", "conn1", 1000, 500)
	// No error to check, just ensure it doesn't panic
}

func TestSummary(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	tracker.Record("gpt-4o", "conn1", 1000, 500)
	tracker.Record("gpt-4o", "conn1", 2000, 1000)

	summary := tracker.Summary()
	if summary == nil {
		t.Fatal("expected summary")
	}
}

func TestSummarySince(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	tracker.Record("gpt-4o", "conn1", 1000, 500)

	summary := tracker.SummarySince(24 * time.Hour)
	if summary == nil {
		t.Fatal("expected summary")
	}
}

func TestRecordSavings(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	entry := SavingsEntry{
		Timestamp:     time.Now().Format(time.RFC3339),
		Category:      "compression",
		Model:         "gpt-4o",
		InputTokens:   1000,
		OutputTokens:  500,
		OriginalCost:  0.05,
		ActualCost:    0.03,
		SavingsUSD:    0.02,
		SavingsPercent: 40.0,
		ConnectionID:  "conn1",
	}

	tracker.RecordSavings(entry)
}

func TestSavingsSummaryAll(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	entry := SavingsEntry{
		Timestamp:     time.Now().Format(time.RFC3339),
		Category:      "compression",
		Model:         "gpt-4o",
		InputTokens:   1000,
		OutputTokens:  500,
		OriginalCost:  0.05,
		ActualCost:    0.03,
		SavingsUSD:    0.02,
		SavingsPercent: 40.0,
		ConnectionID:  "conn1",
	}

	tracker.RecordSavings(entry)

	summary := tracker.SavingsSummaryAll()
	if summary == nil {
		t.Fatal("expected savings summary")
	}
}

func TestSavingsHistory(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	entry := SavingsEntry{
		Timestamp:     time.Now().Format(time.RFC3339),
		Category:      "compression",
		Model:         "gpt-4o",
		InputTokens:   1000,
		OutputTokens:  500,
		OriginalCost:  0.05,
		ActualCost:    0.03,
		SavingsUSD:    0.02,
		SavingsPercent: 40.0,
		ConnectionID:  "conn1",
	}

	tracker.RecordSavings(entry)

	history := tracker.SavingsHistory(10, 0)
	if history == nil {
		t.Error("expected history data")
	}
}

func TestSavingsLeaderboard(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	entry := SavingsEntry{
		Timestamp:     time.Now().Format(time.RFC3339),
		Category:      "compression",
		Model:         "gpt-4o",
		InputTokens:   1000,
		OutputTokens:  500,
		OriginalCost:  0.05,
		ActualCost:    0.03,
		SavingsUSD:    0.02,
		SavingsPercent: 40.0,
		ConnectionID:  "conn1",
	}

	tracker.RecordSavings(entry)

	leaderboard := tracker.SavingsLeaderboard("model", 10)
	if leaderboard == nil {
		t.Error("expected leaderboard data")
	}
}

func TestRegisterPricing(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	tracker.RegisterPricing("custom-model", ModelPrice{
		InputPrice:  1.0,
		OutputPrice: 2.0,
	})
	// No error to check, just ensure it doesn't panic
}

func TestCalculator(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	calc := tracker.Calculator()
	if calc == nil {
		t.Fatal("expected calculator")
	}
}

func TestSavingsSummarySince(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	entry := SavingsEntry{
		Timestamp:     time.Now().Format(time.RFC3339),
		Category:      "routing",
		Model:         "gpt-4o-mini",
		InputTokens:   2000,
		OutputTokens:  1000,
		OriginalCost:  0.10,
		ActualCost:    0.05,
		SavingsUSD:    0.05,
		SavingsPercent: 50.0,
		ConnectionID:  "conn1",
	}

	tracker.RecordSavings(entry)

	summary := tracker.SavingsSummarySince(24 * time.Hour)
	if summary == nil {
		t.Fatal("expected savings summary")
	}
}

func TestZeroSavings(t *testing.T) {
	db := setupTestDB(t)
	tracker := New(db)

	summary := tracker.SavingsSummaryAll()
	if summary == nil {
		t.Fatal("expected savings summary")
	}
	if summary.TotalRequests != 0 {
		t.Errorf("expected 0 requests, got %d", summary.TotalRequests)
	}
}

// setupTestDB creates a test database
func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

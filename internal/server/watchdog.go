package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type WatchdogReport struct {
	LastRunAt    string   `json:"last_run_at"`
	TotalChecked int      `json:"total_checked"`
	HealthyCount int      `json:"healthy_count"`
	FailedCount  int      `json:"failed_count"`
	AutoDisabled int      `json:"auto_disabled"`
	DisabledIDs  []string `json:"disabled_ids"`
	Running      bool     `json:"running"`
}

var (
	watchdogMu     sync.Mutex
	watchdogState  WatchdogReport
	watchdogActive bool
)

func (s *Server) RunKeyHealthWatchdog(autoDisableFatal bool) WatchdogReport {
	watchdogMu.Lock()
	if watchdogActive {
		r := watchdogState
		r.Running = true
		watchdogMu.Unlock()
		return r
	}
	watchdogActive = true
	watchdogMu.Unlock()

	defer func() {
		watchdogMu.Lock()
		watchdogActive = false
		watchdogMu.Unlock()
	}()

	// Query all active connections
	rows, err := s.db.Conn().Query(
		"SELECT id, name, base_url, api_key, COALESCE(oauth_provider,''), COALESCE(models_path,'/v1/models'), COALESCE(auth_header,'Authorization'), COALESCE(auth_prefix,'Bearer ') FROM connections WHERE is_active = 1",
	)
	if err != nil {
		return WatchdogReport{LastRunAt: time.Now().UTC().Format(time.RFC3339), Running: false}
	}
	defer rows.Close()

	var targets []connToTest
	for rows.Next() {
		var c connToTest
		if err := rows.Scan(&c.id, &c.name, &c.baseURL, &c.apiKey, &c.oauthProv, &c.modelsPath, &c.authHeader, &c.authPrefix); err == nil {
			targets = append(targets, c)
		}
	}

	concurrency := 15
	if len(targets) < concurrency {
		concurrency = len(targets)
	}
	if concurrency == 0 {
		return WatchdogReport{
			LastRunAt:    time.Now().UTC().Format(time.RFC3339),
			TotalChecked: 0,
			Running:      false,
		}
	}

	jobs := make(chan connToTest, len(targets))
	resultsCh := make(chan BulkTestItemResult, len(targets))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for target := range jobs {
				resultsCh <- s.executeSingleConnTest(target)
			}
		}()
	}

	for _, target := range targets {
		jobs <- target
	}
	close(jobs)

	wg.Wait()
	close(resultsCh)

	report := WatchdogReport{
		LastRunAt:    time.Now().UTC().Format(time.RFC3339),
		TotalChecked: len(targets),
		DisabledIDs:  make([]string, 0),
		Running:      false,
	}

	for res := range resultsCh {
		if res.Status == "ok" {
			report.HealthyCount++
		} else {
			report.FailedCount++
			if autoDisableFatal && (res.ErrorCode == 401 || res.ErrorCode == 402 || res.ErrorCode == 403 || res.ErrorCode == 404) {
				report.DisabledIDs = append(report.DisabledIDs, res.ID)
			}
		}
	}

	// Auto-disable fatal accounts in DB
	if len(report.DisabledIDs) > 0 {
		tx, err := s.db.Conn().Begin()
		if err == nil {
			stmt, errStmt := tx.Prepare("UPDATE connections SET is_active = 0 WHERE id = ?")
			if errStmt == nil {
				for _, id := range report.DisabledIDs {
					if r, errExec := stmt.Exec(id); errExec == nil {
						if n, _ := r.RowsAffected(); n > 0 {
							report.AutoDisabled += int(n)
						}
					}
				}
				stmt.Close()
			}
			tx.Commit()
		}
		if s.proxy != nil {
			s.proxy.RefreshMultiAccountPools()
		}
	}

	watchdogMu.Lock()
	watchdogState = report
	watchdogMu.Unlock()

	return report
}

func (s *Server) handleWatchdogStatus(w http.ResponseWriter, r *http.Request) {
	watchdogMu.Lock()
	rep := watchdogState
	rep.Running = watchdogActive
	watchdogMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    rep,
	})
}

func (s *Server) handleWatchdogRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"use POST"}`, http.StatusMethodNotAllowed)
		return
	}
	autoDisable := r.URL.Query().Get("auto_disable") != "false"
	rep := s.RunKeyHealthWatchdog(autoDisable)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"data":    rep,
	})
}

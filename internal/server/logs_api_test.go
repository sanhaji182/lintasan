package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogsAPIWithRealisticData(t *testing.T) {
	s := newRESTTestServer(t)
	
	// Insert varied test data simulating production traffic
	dbConn := s.db.Conn()
	
	testData := []struct{
		id string; model string; status int; provider string; input_tokens int; output_tokens int; latency_ms int; cached int; created_at string; err string
	}{
		{"log1","openai/gpt-4",200,"OpenAI",100,50,1200,0,"2026-09-17 10:00:00",""},
		{"log2","anthropic/claude-3",200,"Anthropic",250,180,2500,0,"2026-09-17 10:15:00",""},
		{"log3","google/gemini-pro",200,"Google",80,40,900,1,"2026-09-17 10:30:00",""},
		{"log4","mistral/mixtral",404,"Mistral",120,0,300,0,"2026-09-17 10:45:00","rate limited"},
		{"log5","deepseek/deepseek-chat",200,"DeepSeek",200,150,1800,0,"2026-09-17 11:00:00",""},
	}
	
	for _, td := range testData {
		query := `INSERT INTO request_logs (id,model,status,provider,input_tokens,output_tokens,latency_ms,cached,error,created_at) VALUES (` +
			`?,?,?,?,?,?,?,?,?,?)`
		if td.err != "" {
			dbConn.Exec(query, td.id, td.model, td.status, td.provider, td.input_tokens, td.output_tokens, td.latency_ms, td.cached, td.err, td.created_at)
		} else {
			dbConn.Exec(query, td.id, td.model, td.status, td.provider, td.input_tokens, td.output_tokens, td.latency_ms, td.cached, nil, td.created_at)
		}
	}
	
	tests := []struct{
		name string
		url  string
		expectedCount int
		checkFunc func([]any) error
	}{
		{
			name: "default limit 100 returns all data",
			url: "/api/logs",
			expectedCount: 5,
		},
		{
			name: "limit=2 returns only 2 items",
			url: "/api/logs?limit=2",
			expectedCount: 2,
		},
		{
			name: "offset=2 skips first 2 items",
			url: "/api/logs?limit=2&offset=2",
			expectedCount: 2,
		},
		{
			name: "status=success filters correctly",
			url: "/api/logs?status=success",
			expectedCount: 4, // log1, log2, log3, log5 (all 200s, including cache hit)
		},
		{
			name: "status=error filters 4xx errors",
			url: "/api/logs?status=error",
			expectedCount: 1, // log4 (404)
		},
		{
			name: "cached hits filtered separately",
			url: "/api/logs?status=cached",
			expectedCount: 1, // log3 (cached=1)
		},
		{
			name: "provider search is case-insensitive LIKE",
			url: "/api/logs?provider=open",
			expectedCount: 1, // OpenAI
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", tt.url, nil)
			s.handleLogs(rec, req)
			
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: got HTTP %d, want 200", tt.name, rec.Code)
			}
			
			var resp map[string]any
			json.Unmarshal(rec.Body.Bytes(), &resp)
			data, ok := resp["data"].([]any)
			if !ok {
				t.Fatalf("%s: no data array in response", tt.name)
			}
			
			if len(data) != tt.expectedCount {
				t.Errorf("%s: expected %d logs, got %d", tt.name, tt.expectedCount, len(data))
			}
			
			if tt.checkFunc != nil {
				if err := tt.checkFunc(data); err != nil {
					t.Errorf("%s: check failed: %v", tt.name, err)
				}
			}
		})
	}
}

package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// TestRetryPassWaitsAndSucceedsOnLaterPass exercises the retryPass loop
// end-to-end: an upstream answers twice with Qoder-style 403 queue bodies
// (backoff hint 1s), then succeeds. The client must receive the successful
// answer — not "all routes failed" — proving the pass wait + fresh pool walk
// happened instead of an immediate exhausted-routes reply.
func TestRetryPassWaitsAndSucceedsOnLaterPass(t *testing.T) {
	origPasses, origCap, origBudget := maxBackoffRetryPasses, perPassWaitCap, totalBackoffBudget
	defer func() {
		maxBackoffRetryPasses, perPassWaitCap, totalBackoffBudget = origPasses, origCap, origBudget
	}()
	maxBackoffRetryPasses = 3
	perPassWaitCap = 5 * time.Second
	totalBackoffBudget = 10 * time.Second

	queueBodies := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/health") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if atomic.AddInt32(&queueBodies, 1) <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(403)
			w.Write([]byte(`{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,\\\"retryAfterSeconds\\\":1}\"}"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"ok","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"PONG"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	_ = database.SetSetting("stream_cache_enabled", "false")
	_ = database.SetSetting("context_compression_enabled", "false")

	if _, err := database.Conn().Exec(
		`INSERT INTO connections (id,name,base_url,api_key,format,chat_path,auth_header,auth_prefix,is_active,priority)
		 VALUES ('rq-conn','retryq','` + upstream.URL + `','sk-t','openai','/v1/chat/completions','Authorization','Bearer ',1,100)`,
	); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO discovered_models (id,connection_id,model_id,model_name,is_active)
		 VALUES ('rq-dm','rq-conn','retryq-model','retryq-model',1)`,
	); err != nil {
		t.Fatalf("seed model: %v", err)
	}

	p := NewProxyHandler(&config.Config{}, database)
	gw := httptest.NewServer(http.HandlerFunc(p.HandleChatCompletions))
	t.Cleanup(gw.Close)

	body := `{"model":"retryq-model","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client-key")
	req.Header.Set("X-Lintasan-Direct", "true")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after retry passes, got %d: %s", resp.StatusCode, string(b))
	}
	if !strings.Contains(string(b), "PONG") {
		t.Fatalf("expected successful content, got: %s", string(b))
	}
	if atomic.LoadInt32(&queueBodies) < 3 {
		t.Fatalf("expected >=3 upstream attempts (2 queued + 1 success), got %d", atomic.LoadInt32(&queueBodies))
	}
	if elapsed := time.Since(start); elapsed < 1*time.Second {
		t.Fatalf("expected to honour >=1s backoff hint, request took only %v", elapsed)
	}
}

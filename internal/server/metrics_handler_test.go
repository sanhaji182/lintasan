package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
	"github.com/sanhaji182/lintasan-go/internal/memory"
)

// newMetricsTestServer builds an ACTIVE server (seeded admin + master key)
// fronted by the FULL production middleware chain INCLUDING metricsMiddleware,
// so tests exercise exactly what Start() wires. Returns the Server and a live
// httptest.Server.
func newMetricsTestServer(t *testing.T, masterKey string) (*Server, *httptest.Server) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	cfg := &config.Config{Port: 0, MasterKey: masterKey}
	s := New(cfg, database) // seeds an admin; master key in cfg -> ACTIVE
	handler := s.corsMiddleware(s.metricsMiddleware(s.authMiddleware(s.mux)))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return s, ts
}

// assertValidExposition does a strict structural check that every non-comment
// line is `metric value` where value parses as a float, and that each sampled
// family has a declared # TYPE. Mirrors the deeper parser test in the metrics
// package but keeps this server test self-contained.
func assertValidExposition(t *testing.T, text string) {
	t.Helper()
	types := map[string]bool{}
	sampleCount := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			f := strings.SplitN(line, " ", 4)
			if len(f) >= 4 && f[1] == "TYPE" {
				types[f[2]] = true
			}
			continue
		}
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			t.Fatalf("metric line without value: %q", line)
		}
		val := line[sp+1:]
		if val != "+Inf" && val != "-Inf" && val != "NaN" {
			if _, err := strconv.ParseFloat(val, 64); err != nil {
				t.Fatalf("metric %q has non-float value %q", line[:sp], val)
			}
		}
		// Base family name = strip labels + histogram suffixes.
		name := line[:sp]
		if b := strings.IndexByte(name, '{'); b >= 0 {
			name = name[:b]
		}
		for _, suf := range []string{"_bucket", "_sum", "_count"} {
			name = strings.TrimSuffix(name, suf)
		}
		if !types[name] {
			t.Errorf("sample for %q has no preceding # TYPE", name)
		}
		sampleCount++
	}
	if sampleCount == 0 {
		t.Fatal("no metric samples found in exposition output")
	}
}

// TestMetricsEndpoint_ValidAndPublic verifies GET /metrics returns 200 with the
// Prometheus content type, is reachable WITHOUT auth even in ACTIVE state (like
// /health), parses as valid exposition, and includes the H3 search counters +
// runtime gauges + the http histogram family.
func TestMetricsEndpoint_ValidAndPublic(t *testing.T) {
	t.Setenv("LINTASAN_METRICS_ENABLED", "")
	_, ts := newMetricsTestServer(t, "sk-test-master-key")

	resp := get(t, ts, "/metrics", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics: expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	assertValidExposition(t, out)

	for _, want := range []string{
		"# TYPE lintasan_memory_search_calls_total counter",
		"lintasan_memory_search_scanned_rows_total",
		"lintasan_memory_search_capped_total",
		"lintasan_memory_search_max_scan_rows",
		"# TYPE lintasan_cache_hits_total counter",
		"lintasan_cache_misses_total",
		"lintasan_process_goroutines",
		"lintasan_process_heap_alloc_bytes",
		"lintasan_build_info",
		"# TYPE lintasan_http_request_duration_seconds histogram",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("/metrics output missing %q", want)
		}
	}
}

// TestMetricsEndpoint_NoSecrets is the hard-fail security gate: the full
// /metrics output must contain NONE of master_key, connection API keys, JWT
// secret, or Authorization header contents — in any value or label.
func TestMetricsEndpoint_NoSecrets(t *testing.T) {
	t.Setenv("LINTASAN_METRICS_ENABLED", "")
	const masterKey = "sk-master-SUPERSECRET-7f3a9c2e1b"
	s, ts := newMetricsTestServer(t, masterKey)

	const connKey = "sk-conn-LEAKME-abcdef0123456789"
	const jwtSecret = "jwt-secret-LEAKME-zzzz"
	s.db.SetSetting("master_key", masterKey)
	s.db.SetSetting("jwt_secret", jwtSecret)
	if _, err := s.db.Conn().Exec(
		`INSERT INTO connections (id, name, base_url, api_key, format, is_active)
		 VALUES ('c-leak', 'leaky', 'https://x.test', ?, 'openai', 1)`, connKey); err != nil {
		t.Logf("seed connection skipped (schema variation): %v", err)
	}

	// Drive some traffic carrying secrets in the Authorization header so we'd
	// catch any accidental header echo into a metric.
	r := get(t, ts, "/v1/models", map[string]string{"Authorization": "Bearer " + masterKey})
	r.Body.Close()

	resp := get(t, ts, "/metrics", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	forbidden := []string{
		masterKey,
		connKey,
		jwtSecret,
		"SUPERSECRET",
		"sk-conn-LEAKME",
		"master_key",
		"api_key",
		"Authorization",
		"Bearer ",
	}
	for _, secret := range forbidden {
		if strings.Contains(out, secret) {
			t.Errorf("SECURITY: /metrics output leaked forbidden string %q", secret)
		}
	}
}

// TestMetricsEndpoint_GateDisabled verifies LINTASAN_METRICS_ENABLED=false
// turns the endpoint off (404).
func TestMetricsEndpoint_GateDisabled(t *testing.T) {
	t.Setenv("LINTASAN_METRICS_ENABLED", "false")
	_, ts := newMetricsTestServer(t, "sk-test-master")
	resp := get(t, ts, "/metrics", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("with metrics disabled, expected 404, got %d", resp.StatusCode)
	}
}

// TestMetricsMiddleware_RecordsRequests confirms the middleware observes served
// requests so the counter shows up after traffic, labeled by the normalized
// endpoint group (never a raw path).
func TestMetricsMiddleware_RecordsRequests(t *testing.T) {
	t.Setenv("LINTASAN_METRICS_ENABLED", "")
	_, ts := newMetricsTestServer(t, "sk-test-master")

	for i := 0; i < 3; i++ {
		r := get(t, ts, "/health", nil)
		r.Body.Close()
	}

	resp := get(t, ts, "/metrics", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	if !strings.Contains(out, `lintasan_http_requests_total{endpoint="/health",status_class="2xx"}`) {
		t.Errorf("expected http_requests_total for /health group after hitting it; got:\n%s", out)
	}
}

// TestMetricsEndpoint_SearchCountersIncrement drives the real vector-search hot
// path (the H3 O(n) scan) and asserts the /metrics search-call counter goes up.
// Uses the SQLite-backed store so it runs without Redis. The memory counters
// are process-global, so we compare the parsed counter before and after.
func TestMetricsEndpoint_SearchCountersIncrement(t *testing.T) {
	t.Setenv("LINTASAN_METRICS_ENABLED", "")
	_, ts := newMetricsTestServer(t, "sk-test-master")

	before := scrapeCounter(t, ts, "lintasan_memory_search_calls_total")

	// Build a SQLite-backed memory store and run a real Search — this calls
	// recordSearchCall() inside the memory package, bumping the global counter.
	ms := memory.NewLazy(memory.Config{Addr: "127.0.0.1:19999", DataDir: t.TempDir()})
	if !ms.Available() {
		t.Skip("no memory backend available")
	}
	defer ms.Close()
	emb := memory.Embed("observability hot path probe")
	if _, err := ms.Store.Search(emb, 5); err != nil {
		t.Fatalf("vector search: %v", err)
	}

	after := scrapeCounter(t, ts, "lintasan_memory_search_calls_total")
	if after <= before {
		t.Errorf("expected search_calls_total to increment after a vector search: before=%d after=%d", before, after)
	}
}

// scrapeCounter fetches /metrics and returns the integer value of a no-label
// counter line, or fails the test.
func scrapeCounter(t *testing.T, ts *httptest.Server, name string) int64 {
	t.Helper()
	resp := get(t, ts, "/metrics", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, name+" ") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				v, err := strconv.ParseInt(parts[1], 10, 64)
				if err != nil {
					t.Fatalf("counter %q value %q not int: %v", name, parts[1], err)
				}
				return v
			}
		}
	}
	t.Fatalf("counter %q not found in /metrics", name)
	return 0
}

// TestMetricsEndpoint_CardinalityBounded asserts the exposition never carries a
// forbidden high-cardinality label key, and that hitting a dynamic path does
// not leak its id segment as a label value.
func TestMetricsEndpoint_CardinalityBounded(t *testing.T) {
	t.Setenv("LINTASAN_METRICS_ENABLED", "")
	_, ts := newMetricsTestServer(t, "sk-test-master")

	// Hit a dynamic path (DELETE /v1/memory/{key}). Auth will reject it, but the
	// middleware still records it under the normalized /v1/memory group.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/memory/SECRETKEY123abc", nil)
	if r, err := ts.Client().Do(req); err == nil {
		r.Body.Close()
	}

	resp := get(t, ts, "/metrics", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	for _, bad := range []string{"user_id=", "request_id=", "session_id=", "prompt_hash=", "SECRETKEY123abc"} {
		if strings.Contains(out, bad) {
			t.Errorf("cardinality/leak violation: %q present in /metrics output", bad)
		}
	}
}

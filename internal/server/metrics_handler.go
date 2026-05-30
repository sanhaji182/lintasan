package server

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/memory"
	"github.com/sanhaji182/lintasan-go/internal/metrics"
)

// nowFunc is the clock used by the metrics middleware. It's a variable so tests
// can stub elapsed time deterministically.
var nowFunc = time.Now

// metricsEnabled reports whether the /metrics endpoint is exposed. Gated by
// LINTASAN_METRICS_ENABLED (default true — the endpoint serves read-only
// numeric counters with no secrets). Set to a falsey value to disable.
func metricsEnabled() bool {
	v := os.Getenv("LINTASAN_METRICS_ENABLED")
	if v == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// memorySearchCollector bridges the H3 semantic-search counters from the memory
// package into Prometheus families. This lives in the server package (not the
// metrics package) so metrics never imports memory — avoids an import cycle and
// keeps metrics dependency-free.
//
// Emits ONLY numeric counters + one bounded gauge (the configured scan cap).
// No keys, no prompt text, no secrets.
func memorySearchCollector(w io.Writer) {
	m := memory.Metrics()
	metrics.WriteCounter(w, "lintasan_memory_search_calls_total",
		"Total semantic-memory Search invocations.", float64(m.Calls))
	metrics.WriteCounter(w, "lintasan_memory_search_hits_total",
		"Searches that returned at least one result.", float64(m.Hits))
	metrics.WriteCounter(w, "lintasan_memory_search_empty_exits_total",
		"Searches that early-exited because the store was empty.", float64(m.EmptyExits))
	metrics.WriteCounter(w, "lintasan_memory_search_scanned_rows_total",
		"Cumulative rows/keys scanned by brute-force similarity search (H3 hot path).", float64(m.RowsScanned))
	metrics.WriteCounter(w, "lintasan_memory_search_capped_total",
		"Times a search hit MaxScanRows and stopped scanning early.", float64(m.CappedScans))
	metrics.WriteGauge(w, "lintasan_memory_search_max_scan_rows",
		"Configured brute-force scan cap (LINTASAN_MEMORY_MAX_SCAN; 0 = unbounded).", float64(m.MaxScanRows))
}

// buildInfoCollector emits a single build_info gauge carrying the server
// version as a BOUNDED info label. Value is always 1 (the Prometheus
// build-info idiom). Version is a fixed compile-time string, never secret.
func buildInfoCollector(w io.Writer) {
	metrics.WriteLabeledGauge(w, "lintasan_build_info",
		"Build information. Value is always 1; the version is carried as a label.",
		1, "version", serverVersion)
}

const serverVersion = "2.3.1"

// HandleMetrics serves GET /metrics in Prometheus text exposition format
// (v0.0.4). Read-only; numeric counters and bounded labels only — never
// master_key, connection API keys, JWTs, or prompt content.
func (s *Server) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if !metricsEnabled() {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	s.metrics.WritePrometheus(w)
}

// statusRecorder wraps http.ResponseWriter to capture the status code while
// transparently forwarding the optional http.Flusher interface so streaming
// (SSE chat completions, MCP SSE) keeps working when the metrics middleware is
// in the chain. The proxy asserts w.(http.Flusher) directly, so we MUST forward
// it or streaming silently buffers.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if !sr.wroteHeader {
		sr.status = code
		sr.wroteHeader = true
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.wroteHeader {
		// Default status when a handler writes a body without an explicit
		// WriteHeader call (net/http semantics: implicit 200).
		sr.status = http.StatusOK
		sr.wroteHeader = true
	}
	return sr.ResponseWriter.Write(b)
}

// Flush forwards to the underlying ResponseWriter's Flusher if present, so
// streaming responses continue to flush through the metrics wrapper.
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// metricsMiddleware records request count + latency for every served request,
// labeled by NORMALIZED endpoint group + status CLASS (bounded cardinality).
// The /metrics endpoint itself is observed too (as its own group) so scrape
// load is visible.
func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := nowFunc()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := nowFunc().Sub(start).Seconds()
		endpoint := metrics.NormalizeEndpoint(r.URL.Path)
		statusClass := metrics.StatusClass(rec.status)
		s.metrics.ObserveHTTP(endpoint, statusClass, elapsed)
	})
}

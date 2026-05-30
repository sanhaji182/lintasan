// Package metrics implements a tiny, dependency-free Prometheus metrics
// registry for Lintasan. It exposes the de-facto Prometheus text exposition
// format (v0.0.4) at GET /metrics so operators can scrape hot-path health —
// most importantly the H3 semantic-search counters (calls/hits/empty/scanned/
// capped) so an O(n)-scan regression shows up on a graph before it shows up as
// latency.
//
// Design goals (see AGENTS.md §10.5 "Pure Go, minim dependency"):
//   - No external Prometheus client library. The exposition format is plain
//     text; we generate it by hand and parse it in tests with a small reader.
//   - BOUNDED label cardinality. HTTP series are keyed only by a normalized
//     endpoint GROUP (e.g. "/v1/chat", "/api/connections" — never a raw path
//     with an {id}) and a status CLASS ("2xx"/"4xx"/"5xx"). No user_id,
//     request_id, session_id, prompt hashes, or raw connection identifiers
//     ever become labels.
//   - NO SECRETS. The registry only ever emits numeric counters/gauges and
//     bounded labels. master_key, API keys, JWTs, and prompt content are never
//     touched here. "Prefer losing a metric over leaking a secret."
package metrics

import (
	"bytes"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// latencyBuckets are the cumulative upper bounds (in seconds) for the HTTP
// request-duration histogram. Aggregative by design — same bounds for every
// endpoint group so dashboards can sum across them.
var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// httpSeries is one (endpoint, status_class) time series for the HTTP metrics.
type httpSeries struct {
	endpoint    string
	statusClass string
	count       uint64
	sum         float64
	buckets     []uint64 // cumulative; len == len(latencyBuckets)
}

// Collector is a pull-based metric source invoked at scrape time. It writes one
// or more complete metric families (HELP/TYPE + samples) to w. Used for values
// that live outside this package (memory search counters, runtime stats) so the
// metrics package never has to import them and risk an import cycle.
type Collector func(w io.Writer)

// Registry holds HTTP request metrics plus a set of pull collectors.
type Registry struct {
	mu         sync.Mutex
	http       map[string]*httpSeries
	collectors []Collector
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{http: make(map[string]*httpSeries)}
}

// RegisterCollector adds a pull-based collector invoked on every scrape.
func (r *Registry) RegisterCollector(c Collector) {
	if c == nil {
		return
	}
	r.mu.Lock()
	r.collectors = append(r.collectors, c)
	r.mu.Unlock()
}

// ObserveHTTP records one served HTTP request: it bumps the request counter and
// observes the latency histogram for the (endpoint, statusClass) series. Both
// label values MUST already be normalized/bounded by the caller.
func (r *Registry) ObserveHTTP(endpoint, statusClass string, seconds float64) {
	key := endpoint + "\x00" + statusClass
	r.mu.Lock()
	s := r.http[key]
	if s == nil {
		s = &httpSeries{
			endpoint:    endpoint,
			statusClass: statusClass,
			buckets:     make([]uint64, len(latencyBuckets)),
		}
		r.http[key] = s
	}
	s.count++
	s.sum += seconds
	for i, b := range latencyBuckets {
		if seconds <= b {
			s.buckets[i]++
		}
	}
	r.mu.Unlock()
}

// WritePrometheus serializes the full registry in Prometheus text exposition
// format. Collector output comes first (memory, runtime, build_info), then the
// HTTP request/duration families.
func (r *Registry) WritePrometheus(w io.Writer) {
	r.mu.Lock()
	collectors := make([]Collector, len(r.collectors))
	copy(collectors, r.collectors)
	series := make([]*httpSeries, 0, len(r.http))
	for _, s := range r.http {
		series = append(series, s)
	}
	r.mu.Unlock()

	var b bytes.Buffer
	for _, c := range collectors {
		c(&b)
	}
	writeHTTPFamilies(&b, series)
	w.Write(b.Bytes())
}

// writeHTTPFamilies emits the http_requests_total counter family and the
// http_request_duration_seconds histogram family across all series.
func writeHTTPFamilies(b *bytes.Buffer, series []*httpSeries) {
	// Deterministic ordering keeps the output stable (nicer for tests/diffs).
	sort.Slice(series, func(i, j int) bool {
		if series[i].endpoint != series[j].endpoint {
			return series[i].endpoint < series[j].endpoint
		}
		return series[i].statusClass < series[j].statusClass
	})

	// --- http_requests_total ---
	b.WriteString("# HELP lintasan_http_requests_total Total HTTP requests handled, by endpoint group and status class.\n")
	b.WriteString("# TYPE lintasan_http_requests_total counter\n")
	for _, s := range series {
		b.WriteString("lintasan_http_requests_total{endpoint=\"")
		b.WriteString(escapeLabel(s.endpoint))
		b.WriteString("\",status_class=\"")
		b.WriteString(escapeLabel(s.statusClass))
		b.WriteString("\"} ")
		b.WriteString(strconv.FormatUint(s.count, 10))
		b.WriteByte('\n')
	}

	// --- http_request_duration_seconds (histogram) ---
	b.WriteString("# HELP lintasan_http_request_duration_seconds HTTP request latency in seconds, by endpoint group and status class.\n")
	b.WriteString("# TYPE lintasan_http_request_duration_seconds histogram\n")
	for _, s := range series {
		for i, bound := range latencyBuckets {
			b.WriteString("lintasan_http_request_duration_seconds_bucket{endpoint=\"")
			b.WriteString(escapeLabel(s.endpoint))
			b.WriteString("\",status_class=\"")
			b.WriteString(escapeLabel(s.statusClass))
			b.WriteString("\",le=\"")
			b.WriteString(strconv.FormatFloat(bound, 'g', -1, 64))
			b.WriteString("\"} ")
			b.WriteString(strconv.FormatUint(s.buckets[i], 10))
			b.WriteByte('\n')
		}
		// +Inf bucket == total count.
		b.WriteString("lintasan_http_request_duration_seconds_bucket{endpoint=\"")
		b.WriteString(escapeLabel(s.endpoint))
		b.WriteString("\",status_class=\"")
		b.WriteString(escapeLabel(s.statusClass))
		b.WriteString("\",le=\"+Inf\"} ")
		b.WriteString(strconv.FormatUint(s.count, 10))
		b.WriteByte('\n')

		b.WriteString("lintasan_http_request_duration_seconds_sum{endpoint=\"")
		b.WriteString(escapeLabel(s.endpoint))
		b.WriteString("\",status_class=\"")
		b.WriteString(escapeLabel(s.statusClass))
		b.WriteString("\"} ")
		b.WriteString(strconv.FormatFloat(s.sum, 'g', -1, 64))
		b.WriteByte('\n')

		b.WriteString("lintasan_http_request_duration_seconds_count{endpoint=\"")
		b.WriteString(escapeLabel(s.endpoint))
		b.WriteString("\",status_class=\"")
		b.WriteString(escapeLabel(s.statusClass))
		b.WriteString("\"} ")
		b.WriteString(strconv.FormatUint(s.count, 10))
		b.WriteByte('\n')
	}
}

// --- exported helpers for collectors -------------------------------------

// WriteCounter writes a single-series counter family (no labels).
func WriteCounter(w io.Writer, name, help string, value float64) {
	io.WriteString(w, "# HELP "+name+" "+help+"\n")
	io.WriteString(w, "# TYPE "+name+" counter\n")
	io.WriteString(w, name+" "+formatNum(value)+"\n")
}

// WriteGauge writes a single-series gauge family (no labels).
func WriteGauge(w io.Writer, name, help string, value float64) {
	io.WriteString(w, "# HELP "+name+" "+help+"\n")
	io.WriteString(w, "# TYPE "+name+" gauge\n")
	io.WriteString(w, name+" "+formatNum(value)+"\n")
}

// WriteLabeledGauge writes a single-series gauge family with one info label.
// labelPairs must be an even-length name,value,... list. Used for build_info.
func WriteLabeledGauge(w io.Writer, name, help string, value float64, labelPairs ...string) {
	io.WriteString(w, "# HELP "+name+" "+help+"\n")
	io.WriteString(w, "# TYPE "+name+" gauge\n")
	io.WriteString(w, name)
	if len(labelPairs) >= 2 {
		io.WriteString(w, "{")
		for i := 0; i+1 < len(labelPairs); i += 2 {
			if i > 0 {
				io.WriteString(w, ",")
			}
			io.WriteString(w, labelPairs[i]+"=\""+escapeLabel(labelPairs[i+1])+"\"")
		}
		io.WriteString(w, "}")
	}
	io.WriteString(w, " "+formatNum(value)+"\n")
}

func formatNum(v float64) string {
	// Integers print without a decimal point; everything else uses shortest
	// round-trippable form.
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// escapeLabel escapes a Prometheus label value per the exposition format spec:
// backslash, double-quote, and newline.
func escapeLabel(s string) string {
	if !needsEscape(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func needsEscape(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '"', '\n':
			return true
		}
	}
	return false
}

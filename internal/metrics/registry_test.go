package metrics

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// parseExposition is a small, strict-enough Prometheus text exposition parser
// used to assert that our /metrics output is well-formed. It validates:
//   - every non-comment line is `name{labels} value` or `name value`
//   - the value parses as a float
//   - every metric referenced by a sample has a preceding # TYPE line
//   - label sets are well-formed key="value" pairs
//
// It returns the set of metric family names seen and the parsed samples.
type sample struct {
	name   string
	labels map[string]string
	value  string
}

func parseExposition(t *testing.T, text string) (families map[string]string, samples []sample) {
	t.Helper()
	families = map[string]string{} // name -> type
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			fields := strings.SplitN(line, " ", 4)
			// "# TYPE name kind"
			if len(fields) >= 4 && fields[1] == "TYPE" {
				families[fields[2]] = fields[3]
			}
			if len(fields) >= 3 && fields[1] != "TYPE" && fields[1] != "HELP" {
				t.Fatalf("unknown comment directive: %q", line)
			}
			continue
		}
		// metric line: split into "metricpart value"
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			t.Fatalf("metric line without value: %q", line)
		}
		metricPart := line[:sp]
		value := line[sp+1:]
		if !isFloatish(value) {
			t.Fatalf("metric %q has non-float value %q", metricPart, value)
		}
		name, labels := parseMetricPart(t, metricPart)
		samples = append(samples, sample{name: name, labels: labels, value: value})
	}
	return families, samples
}

func parseMetricPart(t *testing.T, s string) (string, map[string]string) {
	t.Helper()
	brace := strings.IndexByte(s, '{')
	if brace < 0 {
		return s, nil
	}
	name := s[:brace]
	if !strings.HasSuffix(s, "}") {
		t.Fatalf("malformed label set (no closing brace): %q", s)
	}
	inner := s[brace+1 : len(s)-1]
	labels := map[string]string{}
	if strings.TrimSpace(inner) == "" {
		return name, labels
	}
	for _, pair := range splitLabels(inner) {
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			t.Fatalf("malformed label %q in %q", pair, s)
		}
		k := pair[:eq]
		v := pair[eq+1:]
		if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
			t.Fatalf("label value not quoted: %q in %q", pair, s)
		}
		labels[k] = v[1 : len(v)-1]
	}
	return name, labels
}

// splitLabels splits a label set body on commas that are not inside quotes.
func splitLabels(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote = !inQuote
			cur.WriteByte(c)
		case c == ',' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func isFloatish(s string) bool {
	if s == "+Inf" || s == "-Inf" || s == "NaN" {
		return true
	}
	seenDigit := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			seenDigit = true
			continue
		}
		switch c {
		case '.', '-', '+', 'e', 'E':
			continue
		}
		return false
	}
	return seenDigit
}

// buildSampleRegistry wires a registry with one HTTP series plus a couple of
// collector-backed families so the parser has real input to validate.
func buildSampleRegistry() *Registry {
	r := NewRegistry()
	r.RegisterCollector(func(w io.Writer) {
		WriteCounter(w, "lintasan_memory_search_calls_total", "calls", 42)
		WriteGauge(w, "lintasan_memory_search_max_scan_rows", "cap", 2000)
		WriteLabeledGauge(w, "lintasan_build_info", "build", 1, "version", "2.3.1")
	})
	r.ObserveHTTP("/v1/chat/completions", "2xx", 0.012)
	r.ObserveHTTP("/v1/chat/completions", "5xx", 1.5)
	r.ObserveHTTP("/api/connections", "2xx", 0.003)
	return r
}

func TestWritePrometheus_ValidExposition(t *testing.T) {
	r := buildSampleRegistry()
	var buf bytes.Buffer
	r.WritePrometheus(&buf)

	families, samples := parseExposition(t, buf.String())

	// Every sample's metric family must have a declared TYPE (histogram
	// bucket/sum/count samples map back to the base family name).
	for _, s := range samples {
		base := s.name
		for _, suffix := range []string{"_bucket", "_sum", "_count"} {
			base = strings.TrimSuffix(base, suffix)
		}
		if _, ok := families[base]; !ok {
			t.Errorf("sample %q has no declared # TYPE for family %q", s.name, base)
		}
	}

	// The search counter from the collector must be present with its value.
	if !hasSample(samples, "lintasan_memory_search_calls_total", nil, "42") {
		t.Error("expected lintasan_memory_search_calls_total 42 in output")
	}

	// build_info carries a bounded version label, value 1.
	if !hasSample(samples, "lintasan_build_info", map[string]string{"version": "2.3.1"}, "1") {
		t.Error("expected lintasan_build_info{version=\"2.3.1\"} 1 in output")
	}
}

func TestHistogram_BucketsAndCount(t *testing.T) {
	r := NewRegistry()
	// Two observations: 0.012s (falls in >=0.025 bucket) and 1.5s.
	r.ObserveHTTP("/v1/chat/completions", "2xx", 0.012)
	r.ObserveHTTP("/v1/chat/completions", "2xx", 1.5)

	var buf bytes.Buffer
	r.WritePrometheus(&buf)
	_, samples := parseExposition(t, buf.String())

	// count must equal number of observations (2).
	if !hasSample(samples, "lintasan_http_request_duration_seconds_count",
		map[string]string{"endpoint": "/v1/chat/completions", "status_class": "2xx"}, "2") {
		t.Error("expected histogram _count of 2")
	}
	// +Inf bucket must equal total count.
	if !hasSample(samples, "lintasan_http_request_duration_seconds_bucket",
		map[string]string{"endpoint": "/v1/chat/completions", "status_class": "2xx", "le": "+Inf"}, "2") {
		t.Error("expected +Inf bucket == 2")
	}
	// requests_total counter must also be 2.
	if !hasSample(samples, "lintasan_http_requests_total",
		map[string]string{"endpoint": "/v1/chat/completions", "status_class": "2xx"}, "2") {
		t.Error("expected http_requests_total of 2")
	}
}

// TestCardinalityGuard asserts that NO forbidden high-cardinality label key
// ever appears in the output, and that path labels are normalized to groups
// (no raw {id}/{key} segments leak as label values).
func TestCardinalityGuard(t *testing.T) {
	r := NewRegistry()
	// Simulate the middleware normalizing dynamic paths before observing.
	for _, raw := range []string{
		"/api/connections/abc-123-def",
		"/v1/memory/sha256deadbeefkey",
		"/v1/chat/completions",
	} {
		r.ObserveHTTP(NormalizeEndpoint(raw), "2xx", 0.01)
	}
	var buf bytes.Buffer
	r.WritePrometheus(&buf)
	_, samples := parseExposition(t, buf.String())

	forbidden := map[string]bool{
		"user_id": true, "request_id": true, "session_id": true,
		"prompt": true, "prompt_hash": true, "api_key": true,
		"connection_id": true, "key": true,
	}
	for _, s := range samples {
		for k, v := range s.labels {
			if forbidden[k] {
				t.Errorf("forbidden high-cardinality label key %q present in %q", k, s.name)
			}
			// The raw dynamic segments must not appear as label values.
			if strings.Contains(v, "abc-123-def") || strings.Contains(v, "sha256deadbeefkey") {
				t.Errorf("raw dynamic path segment leaked as label value %q=%q", k, v)
			}
		}
	}
}

func hasSample(samples []sample, name string, labels map[string]string, value string) bool {
	for _, s := range samples {
		if s.name != name || s.value != value {
			continue
		}
		match := true
		for k, v := range labels {
			if s.labels[k] != v {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

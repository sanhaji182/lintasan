package metrics

import (
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// RuntimeCollector emits process-level gauges: goroutine count, Go heap alloc,
// and resident set size (RSS). These reuse the same runtime signals the H5 soak
// monitor watched, so the dashboard can confirm memory/goroutines stay flat in
// production instead of trusting a one-off 60-minute soak.
//
// No secrets, no unbounded labels — three plain gauges.
func RuntimeCollector(w io.Writer) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	WriteGauge(w, "lintasan_process_goroutines",
		"Number of goroutines currently running.",
		float64(runtime.NumGoroutine()))

	WriteGauge(w, "lintasan_process_heap_alloc_bytes",
		"Bytes of allocated heap objects (runtime.MemStats.HeapAlloc).",
		float64(m.HeapAlloc))

	WriteGauge(w, "lintasan_process_heap_sys_bytes",
		"Bytes of heap memory obtained from the OS (runtime.MemStats.HeapSys).",
		float64(m.HeapSys))

	if rss, ok := residentSetSizeBytes(); ok {
		WriteGauge(w, "lintasan_process_resident_memory_bytes",
			"Resident set size in bytes (RSS) read from /proc/self/statm.",
			float64(rss))
	}
}

// residentSetSizeBytes reads RSS from /proc/self/statm (Linux). The second
// field is resident pages; multiply by page size. Returns ok=false on any
// non-Linux platform or read error — we prefer omitting a metric over guessing.
func residentSetSizeBytes() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, false
	}
	residentPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	pageSize := uint64(os.Getpagesize())
	return residentPages * pageSize, true
}

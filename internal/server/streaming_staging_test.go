//go:build staging
// +build staging

package server

// Staging Verification Harness — F1 Streaming Sign-off
//
// Objective: answer ONE question empirically —
//   "Is streaming behavior identical when provider_sdk_enabled=true?"
//
// Method: mount the REAL HandleChatCompletions on a real TCP server
// (httptest), point it at a DETERMINISTIC mock SSE upstream with fixed
// inter-frame delays, and capture raw client-visible bytes WITH timestamps.
// Run the four required scenarios (non-stream OFF/ON, stream OFF/ON) and
// compare: status, headers, body bytes, SSE frame order, [DONE], TTFB,
// completion, and buffering behavior.
//
// Confounders are neutralized via the X-Lintasan-Direct:true header, which
// disables BOTH the semantic/exact cache and hedged requests (the only
// machinery that could make ON serve OFF's response or double-call upstream).
//
// Run:  go test ./internal/server/ -tags=staging -run TestStaging -v -count=1
//
// This file is build-tagged `staging` so it never runs in the normal unit
// suite (it uses real timers / network and is a separate verification gate).

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

const (
	stgModel  = "gpt-4o-staging"
	stgFrames = 8 // deterministic SSE frame count
)

// stgFrameDelay is the fixed inter-frame delay at the mock upstream, so the
// client can measure TTFB and detect buffering.
const stgFrameDelay = 25 * time.Millisecond

// deterministicSSE writes a fixed, reproducible SSE stream so that two runs
// (flag off vs on) are byte-comparable. Each frame is flushed with a fixed
// delay so the client can measure TTFB and detect buffering.
func deterministicSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("x-tokens-input", "11")
	w.Header().Set("x-tokens-output", "22")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	for i := 0; i < stgFrames; i++ {
		fmt.Fprintf(w, "data: {\"id\":\"stg\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"tok%d \"}}]}\n\n", i)
		if fl != nil {
			fl.Flush()
		}
		time.Sleep(stgFrameDelay)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if fl != nil {
		fl.Flush()
	}
}

// deterministicJSON writes a fixed non-stream completion body.
func deterministicJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-tokens-input", "11")
	w.Header().Set("x-tokens-output", "22")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"id":"stg","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello from staging"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33}}`)
}

// newMockUpstream returns a server that streams or returns JSON depending on
// the inbound body's "stream" flag, and counts how many times it was called.
func newMockUpstream(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Count ONLY the actual chat-completion POST. NewProxyHandler spawns
		// prewarmConnectionPool() which fires a GET base_url/health to every
		// active connection — that prewarm probe must NOT be counted as a
		// dispatch (it is symmetric across flag off/on and unrelated to F1).
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/health") {
			w.WriteHeader(http.StatusOK)
			return
		}
		*calls++
		b, _ := io.ReadAll(r.Body)
		if bytes.Contains(b, []byte(`"stream":true`)) {
			deterministicSSE(w)
			return
		}
		deterministicJSON(w)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// buildStagingGateway constructs a ProxyHandler with the flag set to sdkOn,
// seeds a single connection pointing at upstreamURL plus a discovered_model,
// and mounts the REAL HandleChatCompletions on a real TCP server.
func buildStagingGateway(t *testing.T, sdkOn bool, upstreamURL string) *httptest.Server {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if sdkOn {
		if err := database.SetSetting("provider_sdk_enabled", "true"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
	}
	// Neutralize stream cache too (defensive; direct mode already gates it).
	_ = database.SetSetting("stream_cache_enabled", "false")
	_ = database.SetSetting("context_compression_enabled", "false")

	// Seed connection + discovered model so resolveRoute finds our upstream.
	if _, err := database.Conn().Exec(
		`INSERT INTO connections (id,name,base_url,api_key,format,chat_path,auth_header,auth_prefix,is_active,priority)
		 VALUES ('stg-conn','staging','`+upstreamURL+`','sk-stg','openai','/v1/chat/completions','Authorization','Bearer ',1,100)`,
	); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if _, err := database.Conn().Exec(
		`INSERT INTO discovered_models (id,connection_id,model_id,model_name,is_active)
		 VALUES ('stg-dm','stg-conn','`+stgModel+`','`+stgModel+`',1)`,
	); err != nil {
		t.Fatalf("seed discovered_model: %v", err)
	}

	p := NewProxyHandler(&config.Config{}, database)
	if p.providerSDK != sdkOn {
		t.Fatalf("flag mismatch: want %v got %v", sdkOn, p.providerSDK)
	}
	gw := httptest.NewServer(http.HandlerFunc(p.HandleChatCompletions))
	t.Cleanup(gw.Close)
	return gw
}

// capture holds everything observed client-side for one request.
type capture struct {
	status   int
	header   http.Header
	rawBody  []byte
	ttfb     time.Duration // time to first body byte
	total    time.Duration // time to last body byte
	frames   []string      // SSE data: payloads in arrival order
	doneSeen bool
}

// doCapture performs the request and records byte-level timing. It reads the
// body in small increments so TTFB and buffering are observable.
func doCapture(t *testing.T, gwURL string, stream bool) capture {
	t.Helper()
	reqBody := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"stream":%v}`, stgModel, stream)
	req, _ := http.NewRequest(http.MethodPost, gwURL+"/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer client-key")
	req.Header.Set("X-Lintasan-Direct", "true") // disable cache + hedge

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	c := capture{status: resp.StatusCode, header: resp.Header.Clone()}
	buf := make([]byte, 256)
	var raw []byte
	first := true
	for {
		n, er := resp.Body.Read(buf)
		if n > 0 {
			now := time.Since(start)
			if first {
				c.ttfb = now
				first = false
			}
			c.total = now
			raw = append(raw, buf[:n]...)
		}
		if er != nil {
			break
		}
	}
	c.rawBody = raw

	// Parse SSE frames if streaming.
	if stream {
		sc := bufio.NewScanner(bytes.NewReader(raw))
		sc.Buffer(make([]byte, 1024*64), 1024*64)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				c.doneSeen = true
				continue
			}
			c.frames = append(c.frames, payload)
		}
	}
	return c
}

// TestStaging_F1_StreamingParity runs all four scenarios and reports.
func TestStaging_F1_StreamingParity(t *testing.T) {
	var report strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&report, format+"\n", a...) }

	w("================ F1 STAGING VERIFICATION REPORT ================")
	w("upstream: deterministic SSE (%d frames @ %v) / fixed JSON", stgFrames, stgFrameDelay)
	w("confounders neutralized: X-Lintasan-Direct=true (cache+hedge off), stream_cache off, compression off")
	w("")

	// ---- Non-stream OFF vs ON ----
	var nsOffCalls, nsOnCalls int
	upOff := newMockUpstream(t, &nsOffCalls)
	gwOff := buildStagingGateway(t, false, upOff.URL)
	nsOff := doCapture(t, gwOff.URL, false)

	upOn := newMockUpstream(t, &nsOnCalls)
	gwOn := buildStagingGateway(t, true, upOn.URL)
	nsOn := doCapture(t, gwOn.URL, false)

	w("---- SCENARIO 1+2: NON-STREAM (OFF vs ON) ----")
	w("status:        OFF=%d  ON=%d", nsOff.status, nsOn.status)
	w("body bytes:    OFF=%d  ON=%d", len(nsOff.rawBody), len(nsOn.rawBody))
	w("upstream calls:OFF=%d  ON=%d", nsOffCalls, nsOnCalls)
	w("content-type:  OFF=%q ON=%q", nsOff.header.Get("Content-Type"), nsOn.header.Get("Content-Type"))

	if nsOff.status != nsOn.status {
		t.Errorf("FAIL non-stream status: OFF=%d ON=%d", nsOff.status, nsOn.status)
	}
	if !bytes.Equal(nsOff.rawBody, nsOn.rawBody) {
		t.Errorf("FAIL non-stream body NOT byte-identical:\n OFF=%s\n ON=%s", nsOff.rawBody, nsOn.rawBody)
	} else {
		w("RESULT: non-stream body BYTE-IDENTICAL ✓")
	}
	if nsOnCalls != 1 {
		t.Errorf("FAIL non-stream ON upstream calls=%d (want 1, hedge/cache leak?)", nsOnCalls)
	}
	w("")

	// ---- Stream OFF vs ON ----
	var sOffCalls, sOnCalls int
	supOff := newMockUpstream(t, &sOffCalls)
	sgwOff := buildStagingGateway(t, false, supOff.URL)
	sOff := doCapture(t, sgwOff.URL, true)

	supOn := newMockUpstream(t, &sOnCalls)
	sgwOn := buildStagingGateway(t, true, supOn.URL)
	sOn := doCapture(t, sgwOn.URL, true)

	w("---- SCENARIO 3+4: STREAM (OFF vs ON) ----")
	w("status:         OFF=%d  ON=%d", sOff.status, sOn.status)
	w("content-type:   OFF=%q ON=%q", sOff.header.Get("Content-Type"), sOn.header.Get("Content-Type"))
	w("frame count:    OFF=%d  ON=%d (expected %d)", len(sOff.frames), len(sOn.frames), stgFrames)
	w("[DONE] present: OFF=%v  ON=%v", sOff.doneSeen, sOn.doneSeen)
	w("TTFB:           OFF=%v  ON=%v", sOff.ttfb.Round(time.Millisecond), sOn.ttfb.Round(time.Millisecond))
	w("total:          OFF=%v  ON=%v", sOff.total.Round(time.Millisecond), sOn.total.Round(time.Millisecond))
	w("upstream calls: OFF=%d  ON=%d", sOffCalls, sOnCalls)

	// Status
	if sOff.status != sOn.status {
		t.Errorf("FAIL stream status: OFF=%d ON=%d", sOff.status, sOn.status)
	}
	// Content-Type must be event-stream on both
	if !strings.HasPrefix(sOn.header.Get("Content-Type"), "text/event-stream") {
		t.Errorf("FAIL stream ON content-type=%q (want text/event-stream)", sOn.header.Get("Content-Type"))
	}
	// Frame-identity
	if len(sOff.frames) != len(sOn.frames) {
		t.Errorf("FAIL frame count: OFF=%d ON=%d", len(sOff.frames), len(sOn.frames))
	} else {
		mismatch := -1
		for i := range sOff.frames {
			if sOff.frames[i] != sOn.frames[i] {
				mismatch = i
				break
			}
		}
		if mismatch >= 0 {
			t.Errorf("FAIL frame[%d] differs:\n OFF=%s\n ON=%s", mismatch, sOff.frames[mismatch], sOn.frames[mismatch])
		} else {
			w("RESULT: %d frames FRAME-IDENTICAL, in order ✓", len(sOn.frames))
		}
	}
	// Expected frame count
	if len(sOn.frames) != stgFrames {
		t.Errorf("FAIL stream ON frame count=%d want %d (missing/dup chunk)", len(sOn.frames), stgFrames)
	}
	// [DONE]
	if !sOn.doneSeen {
		t.Errorf("FAIL stream ON missing [DONE]")
	}
	if sOff.doneSeen != sOn.doneSeen {
		t.Errorf("FAIL [DONE] presence differs OFF=%v ON=%v", sOff.doneSeen, sOn.doneSeen)
	}
	// Raw body byte-identity (strongest check)
	if !bytes.Equal(sOff.rawBody, sOn.rawBody) {
		t.Errorf("FAIL stream raw body NOT byte-identical (len OFF=%d ON=%d)", len(sOff.rawBody), len(sOn.rawBody))
	} else {
		w("RESULT: stream raw body BYTE-IDENTICAL ✓")
	}
	// No buffering: with %d frames at %v delay, a true stream spreads arrival
	// over ~(frames-1)*delay. If buffered, TTFB ≈ total. Assert ON streams
	// incrementally (TTFB clearly less than total) AND matches OFF's behavior.
	minSpread := stgFrameDelay // at least one inter-frame gap of separation
	spreadOn := sOn.total - sOn.ttfb
	spreadOff := sOff.total - sOff.ttfb
	w("arrival spread: OFF=%v  ON=%v (min expected %v)", spreadOff.Round(time.Millisecond), spreadOn.Round(time.Millisecond), minSpread)
	if spreadOn < minSpread {
		t.Errorf("FAIL stream ON appears BUFFERED: spread=%v < %v (TTFB≈total)", spreadOn, minSpread)
	} else {
		w("RESULT: stream ON is INCREMENTAL, no buffering ✓")
	}
	// TTFB must not regress significantly: ON TTFB should be within OFF + one frame delay.
	if sOn.ttfb > sOff.ttfb+2*stgFrameDelay {
		t.Errorf("FAIL stream ON TTFB regressed: OFF=%v ON=%v", sOff.ttfb, sOn.ttfb)
	} else {
		w("RESULT: TTFB within tolerance ✓")
	}
	if sOnCalls != 1 {
		t.Errorf("FAIL stream ON upstream calls=%d (want 1)", sOnCalls)
	}
	w("")
	w("=================== END REPORT ===================")

	t.Log("\n" + report.String())
	// Always print the report to stdout for the deliverable.
	fmt.Print("\n" + report.String())
}

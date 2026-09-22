package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// TestLiveProviderEndToEnd drives the complete adapter path — Provider.Prepare,
// the real HTTP call, and ConsumeStream — against the live upstream, and writes
// the outcome to a file.
//
// It exercises Prepare rather than the lower-level helpers on purpose: Prepare is
// what the router will actually call, so proving the pieces individually is not
// the same as proving the assembled path. The result is written to disk because a
// test log is easy to lose in a wrapper, and this evidence is the point of the
// run.
func TestLiveProviderEndToEnd(t *testing.T) {
	if os.Getenv("LINTASAN_QODER_LIVE") != "1" {
		t.Skip("LINTASAN_QODER_LIVE not set")
	}
	liveTestTemplate(t)
	dir := os.Getenv("LINTASAN_QODER_PAT_DIR")
	if dir == "" {
		dir = filepath.Dir(os.Getenv("LINTASAN_QODER_PAT_FILE"))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read credential dir: %v", err)
	}

	region := "global"
	if s := os.Getenv("LINTASAN_QODER_REGION"); s != "" {
		region = s
	}

	// Give each account its own Provider instance: one session cache per account is
	// what guarantees the device identity of one account cannot leak into another
	// account's request, which is the isolation the whole fingerprinting scheme is
	// for.
	build := func(cred string) (*Provider, *SessionManager) {
		p := NewProvider(os.Getenv("LINTASAN_QODER_SALT"), region, nil)
		return p, p.Sessions()
	}

	var out strings.Builder
	fmt.Fprintf(&out, "LIVE PROVIDER END-TO-END (region=%s)\n", region)
	fmt.Fprintf(&out, "PATH: Provider.Prepare -> HTTP -> ConsumeStream\n")
	fmt.Fprintf(&out, "%s\n", strings.Repeat("=", 72))

	chatOK, listed := 0, 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".token") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		cred := strings.TrimSpace(string(raw))
		if cred == "" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".token")
		if i := strings.LastIndexAny(name, "-"); i > 0 {
			name = name[i+1:]
		}

		p, sm := build(cred)
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)

		models, err := sm.ListModels(ctx, cred)
		if err != nil {
			fmt.Fprintf(&out, "%-16s AUTH-FAIL  %s\n", name, errShort(err))
			cancel()
			continue
		}
		listed++
		model := models[0].Key
		for _, mm := range models {
			if mm.IsDefault {
				model = mm.Key
				break
			}
		}

		// Assemble the inbound request exactly as the router would.
		inbound, _ := json.Marshal(map[string]any{
			"model":    model,
			"stream":   true,
			"messages": []map[string]any{{"role": "user", "content": "Reply with exactly: PONG"}},
		})

		up, err := p.Prepare(ctx, &provider.Request{Model: model, Body: inbound, Stream: true},
			&provider.ConnConfig{ID: name, APIKey: cred})
		if err != nil {
			fmt.Fprintf(&out, "%-16s PREPARE-FAIL  %s\n", name, errShort(err))
			cancel()
			continue
		}

		req, err := http.NewRequestWithContext(ctx, up.Method, up.URL, strings.NewReader(string(up.Body)))
		if err != nil {
			fmt.Fprintf(&out, "%-16s REQ-FAIL  %s\n", name, errShort(err))
			cancel()
			continue
		}
		for k, vs := range up.Header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintf(&out, "%-16s HTTP-FAIL  %s\n", name, errShort(err))
			cancel()
			continue
		}

		start := time.Now()
		var content strings.Builder
		outcome, streamErr := ConsumeStream(ctx, resp.Body, model, func(d StreamDelta) error {
			content.WriteString(d.Content)
			return nil
		})
		resp.Body.Close()
		elapsed := time.Since(start)

		switch {
		case streamErr != nil:
			if ue, ok := streamErr.(*UpstreamError); ok && ue.IsLoginExpired() {
				fmt.Fprintf(&out, "%-16s models=%-3d CHAT: LOGIN-EXPIRED (code %s)\n", name, len(models), ue.Code)
			} else if ue, ok := streamErr.(*UpstreamError); ok && ue.IsQueued() {
				fmt.Fprintf(&out, "%-16s models=%-3d CHAT: QUEUED (retry %ds)\n", name, len(models), ue.RetryAfterSeconds)
			} else {
				fmt.Fprintf(&out, "%-16s models=%-3d CHAT-ERR  %s\n", name, len(models), errShort(streamErr))
			}
		case !outcome.HadContent:
			fmt.Fprintf(&out, "%-16s models=%-3d EMPTY-STREAM (no content emitted)\n", name, len(models))
		default:
			chatOK++
			reply := strings.TrimSpace(content.String())
			if len(reply) > 40 {
				reply = reply[:40] + "…"
			}
			fmt.Fprintf(&out, "%-16s models=%-3d CHAT: OK  %dms tokens=(%d,%d) reply=%q\n",
				name, len(models), elapsed.Milliseconds(), outcome.InputTokens, outcome.OutputTokens, reply)
		}
		cancel()
	}

	fmt.Fprintf(&out, "%s\n", strings.Repeat("-", 72))
	fmt.Fprintf(&out, "accounts listed=%d  chat-capable=%d\n", listed, chatOK)
	fmt.Fprintf(&out, "%s\n", strings.Repeat("=", 72))

	os.WriteFile("/tmp/qoder-live-e2e.txt", []byte(out.String()), 0o644)
	t.Log("\n" + out.String())
}

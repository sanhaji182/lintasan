package qoder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLiveSweepAllCredentials exercises every credential in the operator's local
// store through this package's own transport, reporting per-account capability.
//
// It exists because the accounts are NOT interchangeable: entitlements differ, and
// a credential can authenticate and list models while still being refused at chat
// time. A pool is only as usable as its chat-capable members, so the pool's real
// size has to be measured rather than assumed from the number of credentials
// configured. Running this before wiring a pool prevents three dead credentials
// from being thirty percent of every routing decision.
//
// Not an assertion-heavy test: it reports. The assertion is only that the sweep
// itself completes, because the value is the report.
func TestLiveSweepAllCredentials(t *testing.T) {
	if os.Getenv("LINTASAN_QODER_LIVE") != "1" {
		t.Skip("LINTASAN_QODER_LIVE not set — skipping live sweep")
	}
	dir := strings.TrimSpace(os.Getenv("LINTASAN_QODER_PAT_DIR"))
	if dir == "" {
		t.Skip("LINTASAN_QODER_PAT_DIR not set — skipping live sweep")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read credential dir: %v", err)
	}

	type result struct {
		name   string
		models int
		chat   string
	}
	var results []result

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
		// The account name is the trailing part of the file name.
		name := strings.TrimSuffix(e.Name(), ".token")
		if i := strings.LastIndexAny(name, "-"); i > 0 {
			name = name[i+1:]
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

		// Each credential gets its own manager so a session cache cannot leak
		// one account's identity into another's request — the isolation the
		// fingerprinting is supposed to provide.
		am := liveSessionManager(t)

		models, err := am.ListModels(ctx, cred)
		if err != nil {
			results = append(results, result{name: name, chat: "AUTH FAIL: " + errShort(err)})
			cancel()
			continue
		}

		chatResult := "OK"
		if os.Getenv("LINTASAN_QODER_SWEEP_CHAT") == "1" {
			chatResult = tryLiveChat(ctx, t, am, cred, models)
		}

		results = append(results, result{name: name, models: len(models), chat: chatResult})
		cancel()
	}

	var report strings.Builder
	report.WriteString("================ CREDENTIAL SWEEP ================\n")
	usable := 0
	for _, r := range results {
		if r.chat == "OK" {
			usable++
		}
		report.WriteString(fmt.Sprintf("  %-16s models=%-3d chat=%s\n", r.name, r.models, r.chat))
	}
	report.WriteString("  ------------------------------------------------\n")
	report.WriteString(fmt.Sprintf("  chat-capable: %d of %d\n", usable, len(results)))
	report.WriteString("=================================================\n")

	os.WriteFile("/tmp/qoder-sweep-report.txt", []byte(report.String()), 0o644)
	t.Log(report.String())
}

// tryLiveChat performs one minimal chat turn and classifies the outcome.
func tryLiveChat(ctx context.Context, t *testing.T, m *SessionManager, cred string, models []Model) string {
	t.Helper()
	if len(models) == 0 {
		return "NO MODELS"
	}
	model := models[0].Key
	for _, mm := range models {
		if mm.IsDefault {
			model = mm.Key
			break
		}
	}

	body, err := BuildChatBody(ChatRequest{
		Model:     model,
		Messages:  []map[string]any{{"role": "user", "content": "hi"}},
		Stream:    true,
		MaxTokens: 16,
	})
	if err != nil {
		return "BUILD FAIL"
	}

	resp, err := m.StartChatStream(ctx, cred, model, body, "")
	if err != nil {
		if ue, ok := err.(*UpstreamError); ok {
			if ue.IsLoginExpired() {
				return "LOGIN EXPIRED"
			}
			if ue.IsQueued() {
				return "QUEUED"
			}
		}
		return "HTTP FAIL: " + errShort(err)
	}
	defer resp.Body.Close()

	var sawContent bool
	var sawErr error
	outcome, err := ConsumeStream(ctx, resp.Body, model, func(d StreamDelta) error {
		if d.Content != "" || d.Reasoning != "" || len(d.ToolCalls) > 0 {
			sawContent = true
		}
		return nil
	})
	if err != nil {
		sawErr = err
		if ue, ok := err.(*UpstreamError); ok {
			if ue.IsLoginExpired() {
				return "LOGIN EXPIRED"
			}
			if ue.IsQueued() {
				return "QUEUED"
			}
		}
		return "STREAM ERR: " + errShort(err)
	}
	if sawErr != nil {
		return "STREAM ERR"
	}
	if !sawContent {
		return "EMPTY STREAM"
	}
	if outcome.InputTokens == 0 && outcome.OutputTokens == 0 {
		return "OK (no usage)"
	}
	return "OK"
}

// errShort renders an error compactly for a table.
func errShort(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 90 {
		s = s[:90] + "…"
	}
	return s
}

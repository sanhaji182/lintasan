package qoder

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/provider"
)

// liveTestCredentialPath resolves the operator's credential for wiring tests.
func liveTestCredentialPath(t *testing.T) string {
	t.Helper()
	p := strings.TrimSpace(os.Getenv("LINTASAN_QODER_PAT_FILE"))
	if p == "" {
		t.Skip("LINTASAN_QODER_PAT_FILE not set")
	}
	return p
}

// TestWiringHTTPProbe is an operator-gated probe that exercises the provider over
// real HTTP against a RUNNING gateway, rather than in-process.
//
// It exists because the in-process tests cannot catch a wiring mistake that only
// appears when the real server routes a real request: a mis-registered format, a
// dispatch branch that never fires, or a response path that passes envelope bytes
// through. The probe sends an ordinary OpenAI chat request to the gateway's /v1
// endpoint and reports what came back, so a regression in the wiring is visible
// as a client would experience it.
//
// Run against an isolated gateway; it asserts nothing about production.
//
//	LINTASAN_QODER_WIRING_URL=http://127.0.0.1:20199
//	LINTASAN_QODER_WIRING_KEY=<master key>
func TestWiringHTTPProbe(t *testing.T) {
	if os.Getenv("LINTASAN_QODER_LIVE") != "1" {
		t.Skip("LINTASAN_QODER_LIVE not set")
	}
	base := strings.TrimSpace(os.Getenv("LINTASAN_QODER_WIRING_URL"))
	if base == "" {
		t.Skip("LINTASAN_QODER_WIRING_URL not set — needs a running gateway")
	}
	key := strings.TrimSpace(os.Getenv("LINTASAN_QODER_WIRING_KEY"))
	if key == "" {
		t.Skip("LINTASAN_QODER_WIRING_KEY not set")
	}
	model := strings.TrimSpace(os.Getenv("LINTASAN_QODER_WIRING_MODEL"))
	if model == "" {
		model = "auto"
	}

	body := `{"model":"` + model + `","stream":false,"messages":[{"role":"user","content":"Reply with exactly: PONG"}]}`

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request to the gateway failed: %v", err)
	}
	defer resp.Body.Close()

	raw, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	// The envelope must never reach a client. Its presence means the response path
	// passed upstream bytes through instead of translating them.
	if strings.Contains(string(raw), "statusCodeValue") {
		t.Errorf("envelope frames reached the client — the response path is not translating: %s", truncate(string(raw), 300))
	}
	if !strings.Contains(string(raw), "chat.completion") {
		t.Errorf("response is not an OpenAI completion (status %d): %s", resp.StatusCode, truncate(string(raw), 300))
	}
	if !strings.Contains(string(raw), "PONG") {
		t.Errorf("no model content in the response (status %d): %s", resp.StatusCode, truncate(string(raw), 300))
	}
	t.Logf("gateway returned a translated completion (status %d, %d bytes)", resp.StatusCode, len(raw))
}

// compile-time reference so the provider import is exercised even when the probe
// is skipped: the wiring tests must not silently stop covering the adapter.
var _ = func() provider.Track { return provider.TrackExperimental }

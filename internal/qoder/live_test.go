package qoder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// liveTestCredential loads a real PAT from the operator's local store when one
// has been pointed at via an environment variable, otherwise it skips.
//
// The credential is never hard-coded and never printed: the live tests exist to
// prove the port speaks the real protocol, and that proof does not require
// embedding a secret in the repository.
func liveTestCredential(t *testing.T) string {
	t.Helper()
	if os.Getenv("LINTASAN_QODER_LIVE") != "1" {
		t.Skip("LINTASAN_QODER_LIVE not set — skipping live Qoder test")
	}
	path := strings.TrimSpace(os.Getenv("LINTASAN_QODER_PAT_FILE"))
	if path == "" {
		t.Skip("LINTASAN_QODER_PAT_FILE not set — skipping live Qoder test")
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	cred := strings.TrimSpace(string(raw))
	if cred == "" {
		t.Fatal("credential file is empty")
	}
	return cred
}

// liveTestTemplate provisions the request template from the environment, exactly
// as a deployment would, and skips when the operator has not supplied one.
//
// The live request-building tests cannot run without a template — by design — so
// this reports the requirement rather than working around it.
func liveTestTemplate(t *testing.T) {
	t.Helper()
	if err := InitTemplateFromEnv(); err != nil {
		t.Fatalf("template provisioning failed (check %s): %v", TemplateEnvVar, err)
	}
	if !TemplateReady() {
		t.Skipf("%s not set — the template is operator-provisioned and required for request building", TemplateEnvVar)
	}
}

func liveSessionManager(t *testing.T) *SessionManager {
	t.Helper()
	m := NewSessionManager("", "global", nil)
	if os.Getenv("LINTASAN_QODER_SALT") != "" {
		m = NewSessionManager(os.Getenv("LINTASAN_QODER_SALT"), "global", nil)
	}
	return m
}

// TestLiveSessionEstablishes proves the credential -> job token -> session chain
// works against the real upstream, not a fixture.
//
// This is the gate for everything else in the package: if the handshake does not
// complete, no amount of correct parsing downstream matters.
func TestLiveSessionEstablishes(t *testing.T) {
	cred := liveTestCredential(t)
	m := liveSessionManager(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sess, err := m.session(ctx, cred)
	if err != nil {
		t.Fatalf("session handshake failed: %v", err)
	}
	if sess.Identity.AccountID == "" {
		t.Error("handshake succeeded but produced no account id")
	}
	if sess.CosyKey == "" || sess.Info == "" {
		t.Error("session material is incomplete")
	}
	if len(sess.MachineID) != 32 {
		t.Errorf("machine id length = %d, want 32", len(sess.MachineID))
	}
	// The account id must have driven the device seed: the identity is derived
	// from the account, and mixing that up is the subtle failure this asserts on.
	wantDevice := m.Fingerprinter().MachineID(FingerprintSeed(sess.Identity.AccountID, cred))
	if sess.MachineID != wantDevice {
		t.Error("device fingerprint was not derived from the account id")
	}
	t.Logf("live session established (account %s...)", sess.Identity.AccountID[:8])
}

// TestLiveModelList proves the model catalogue is readable through the port.
func TestLiveModelList(t *testing.T) {
	cred := liveTestCredential(t)
	m := liveSessionManager(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	models, err := m.ListModels(ctx, cred)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("no models returned")
	}
	for _, mm := range models {
		if mm.Key == "" {
			t.Error("model with empty key returned")
		}
	}
	t.Logf("live model list: %d enabled chat models: %v", len(models), ModelKeys(models))
}

// TestLiveChatStreams is the end-to-end proof: a real chat turn, requested and
// consumed through this package's own builder and stream reader.
//
// It deliberately asserts only that the stream produces content and completes
// without a protocol error, rather than pinning the model's answer. The value
// here is proving the request shape is accepted and the frame decoding works
// against real frames — the failure modes this port exists to handle.
func TestLiveChatStreams(t *testing.T) {
	liveTestTemplate(t)
	cred := liveTestCredential(t)
	m := liveSessionManager(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	sess, err := m.session(ctx, cred)
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	// Use whatever the account actually offers rather than a hard-coded model,
	// since entitlements differ per account.
	models, err := m.ListModels(ctx, cred)
	if err != nil {
		t.Fatalf("model list: %v", err)
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
		Messages:  []map[string]any{{"role": "user", "content": "Reply with exactly: PONG"}},
		Stream:    true,
		UserType:  sess.Identity.UserType,
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("BuildChatBody: %v", err)
	}

	url := m.Endpoints().ChatStreamURL
	headers, err := m.BuildAPIHeaders(sess, body, pathOf(url), "text/event-stream", map[string]string{
		"x-model-key":    model,
		"x-model-source": "system",
	})
	if err != nil {
		t.Fatalf("BuildAPIHeaders: %v", err)
	}

	resp, err := doStreamRequest(ctx, url, headers, body)
	if err != nil {
		t.Fatalf("chat request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("chat HTTP status = %d", resp.StatusCode)
	}

	var sawContent bool
	outcome, err := ConsumeStream(ctx, resp.Body, model, func(d StreamDelta) error {
		if d.Content != "" {
			sawContent = true
		}
		return nil
	})
	if err != nil {
		// A login-expired outcome is a credential problem, not a port defect, and
		// must be reported as such so the two are not confused.
		if ue, ok := err.(*UpstreamError); ok && ue.IsLoginExpired() {
			t.Fatalf("credential rejected for chat (code 105): the port works, this PAT is dead")
		}
		t.Fatalf("stream failed: %v", err)
	}
	if !sawContent {
		t.Fatalf("stream completed with no content (outcome=%+v)", outcome)
	}
	if outcome.InputTokens == 0 && outcome.OutputTokens == 0 {
		t.Log("warning: no usage reported for this turn")
	}
	t.Logf("live chat OK on model %s: content=%dB tools=%d tokens=(%d,%d)",
		model, outcome.ContentLength, outcome.ToolCalls, outcome.InputTokens, outcome.OutputTokens)
}

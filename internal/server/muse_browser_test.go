package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMuseBrowserCDPStartsAndStreams is an opt-in integration test against a
// running sidecar Chromium. It is skipped unless LINTASAN_TEST_MUSE_CDP_URL is
// set, because CI has no Chrome.
func TestMuseBrowserCDPStartsAndStreams(t *testing.T) {
	cdpURL := os.Getenv("LINTASAN_TEST_MUSE_CDP_URL")
	if cdpURL == "" {
		t.Skip("set LINTASAN_TEST_MUSE_CDP_URL to run the CDP integration test")
	}
	s := &Server{
		museCDPURL:          cdpURL,
		museBrowserSessions: make(map[string]*museBrowserSession),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	sess, err := s.startMuseBrowserSession(ctx, "test", "cdp-test")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	s.museBrowserSessions[sess.id] = sess
	defer func() {
		s.finishMuseBrowserSession(sess, nil)
		select {
		case <-sess.cleanupDone:
		case <-time.After(5 * time.Second):
		}
	}()

	select {
	case frame := <-sess.frames:
		if len(frame) < 100 {
			t.Fatalf("frame too small: %d bytes", len(frame))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no screencast frame within 30s")
	}

	time.Sleep(2 * time.Second)
	inputCtx, inputCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer inputCancel()
	for _, input := range []museBrowserInput{
		{Type: "mouse", Event: "pressed", X: 640, Y: 450, Button: "left", ClickCount: 1},
		{Type: "mouse", Event: "released", X: 640, Y: 450, Button: "left", ClickCount: 1},
		{Type: "text", Text: "test@example.com"},
		{Type: "key", Key: "Backspace"},
	} {
		if err := sess.sendInput(inputCtx, input); err != nil {
			t.Fatalf("input %s failed: %v", input.Type, err)
		}
	}
}

// TestMuseBrowserAutoImportsCookies proves the cookie hand-off without a real
// muse.ai account: cookies are placed in the isolated context, then the session
// must POST them to muse2api's account endpoint and report "imported".
func TestMuseBrowserAutoImportsCookies(t *testing.T) {
	cdpURL := os.Getenv("LINTASAN_TEST_MUSE_CDP_URL")
	if cdpURL == "" {
		t.Skip("set LINTASAN_TEST_MUSE_CDP_URL to run the CDP integration test")
	}
	var (
		mu       sync.Mutex
		gotAuth  string
		gotBody  string
		imported = make(chan struct{})
		once     sync.Once
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		gotBody = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"account":{"id":"acc_cookie_test"},"missing_cookies":[]}`))
		once.Do(func() { close(imported) })
	}))
	defer upstream.Close()

	s := &Server{
		museCDPURL:          cdpURL,
		museBaseURL:         upstream.URL,
		museAdminKey:        "test-admin-key",
		museClient:          upstream.Client(),
		museBrowserSessions: make(map[string]*museBrowserSession),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	sess, err := s.startMuseBrowserSession(ctx, "test", "cookie-test")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	s.museBrowserSessions[sess.id] = sess
	go s.monitorMuseBrowserSession(sess)
	defer func() {
		s.finishMuseBrowserSession(sess, nil)
		select {
		case <-sess.cleanupDone:
		case <-time.After(5 * time.Second):
		}
	}()

	cookieCtx, cookieCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cookieCancel()
	for _, name := range museSessionCookieNames {
		if _, err := sess.cdp.send(cookieCtx, "Network.setCookie", map[string]any{
			"url": "https://muse.ai/", "name": name, "value": "fake-" + name,
			"path": "/", "secure": true, "httpOnly": true,
		}, sess.pageSessionID); err != nil {
			t.Fatalf("set cookie %s: %v", name, err)
		}
	}

	select {
	case <-imported:
	case <-time.After(15 * time.Second):
		t.Fatal("muse2api account import was not called within 15s")
	}
	mu.Lock()
	auth, body := gotAuth, gotBody
	mu.Unlock()
	if auth != "Bearer test-admin-key" {
		t.Errorf("upstream auth = %q", auth)
	}
	for _, name := range museSessionCookieNames {
		if !strings.Contains(body, `"`+name+`"`) {
			t.Errorf("forwarded cookies missing %s: %s", name, body)
		}
	}

	select {
	case raw := <-sess.events:
		var ev struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &ev) != nil || ev.Type != "imported" {
			t.Errorf("expected imported event, got %s", raw)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no imported event after account creation")
	}
}

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

	// Authentication may close/replace the login tab. Cookie detection must
	// keep working at the browser-context level after the page is gone.
	if _, err := sess.cdp.send(cookieCtx, "Target.closeTarget", map[string]any{"targetId": sess.targetID}, ""); err != nil {
		t.Fatalf("close login target: %v", err)
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

// TestMuseBrowserPartialRenew covers the observed muse.ai login flow: the page
// sets three cookies and hatch_vml appears only after muse2api renews the
// session. This exercises the create+renew fallback without Chrome.
func TestMuseBrowserPartialRenew(t *testing.T) {
	var (
		mu      sync.Mutex
		created bool
		renewed bool
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/accounts":
			mu.Lock()
			created = true
			mu.Unlock()
			_, _ = w.Write([]byte(`{"account":{"id":"acc_partial"},"missing_cookies":["hatch_vml"]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/admin/accounts/acc_partial/renew":
			mu.Lock()
			renewed = true
			mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	s := &Server{
		museBaseURL:         upstream.URL,
		museAdminKey:        "test-admin-key",
		museClient:          upstream.Client(),
		museBrowserSessions: make(map[string]*museBrowserSession),
	}
	sess := &museBrowserSession{
		server:      s,
		id:          "sess_partial",
		label:       "partial-test",
		frames:      make(chan []byte, 1),
		events:      make(chan []byte, 16),
		done:        make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}
	s.museBrowserSessions[sess.id] = sess

	s.importMuseBrowserPartial(sess, map[string]string{
		"hatch_sess": "x", "hatch_gw": "y", "hatch_native_auth_device": "z",
	})

	deadline := time.After(5 * time.Second)
	sawImported := false
	for !sawImported {
		select {
		case raw := <-sess.events:
			var ev struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(raw, &ev) == nil && ev.Type == "imported" {
				sawImported = true
			}
		case <-deadline:
			t.Fatal("no imported event from partial fallback")
		}
	}
	mu.Lock()
	createdNow, renewedNow := created, renewed
	mu.Unlock()
	if !createdNow || !renewedNow {
		t.Fatalf("expected create+renew, got created=%v renewed=%v", createdNow, renewedNow)
	}
	select {
	case <-sess.cleanupDone:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not finish")
	}
}

func TestMuseBrowserMissingIsRenewable(t *testing.T) {
	base := map[string]string{"hatch_sess": "s", "hatch_native_auth_device": "d"}
	cases := []struct {
		name    string
		cookies map[string]string
		missing []string
		want    bool
	}{
		{"both gateway cookies pending", base, []string{"hatch_gw", "hatch_vml"}, true},
		{"only vml pending", base, []string{"hatch_vml"}, true},
		{"only gw pending", base, []string{"hatch_gw"}, true},
		{"nothing missing", map[string]string{"hatch_sess": "s", "hatch_native_auth_device": "d", "hatch_gw": "g", "hatch_vml": "v"}, nil, false},
		{"missing session", map[string]string{"hatch_native_auth_device": "d"}, []string{"hatch_sess", "hatch_vml"}, false},
		{"missing device", map[string]string{"hatch_sess": "s"}, []string{"hatch_native_auth_device", "hatch_vml"}, false},
		{"unexpected cookie missing", base, []string{"hatch_vml", "other"}, false},
	}
	for _, tc := range cases {
		if got := museBrowserMissingIsRenewable(tc.cookies, tc.missing); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

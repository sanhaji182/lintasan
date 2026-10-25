package server

// muse_browser.go — remote muse.ai login through the sidecar's Chromium.
//
// The user logs into muse.ai inside an isolated CDP browser context owned by
// the sidecar. Lintasan streams the page as JPEG screencast frames over SSE and
// forwards mouse/keyboard input; once the four hatch_* cookies appear, it
// imports them into muse2api automatically and disposes the context.
//
// SECURITY
//   - Every route is admin-only.
//   - The CDP endpoint must be loopback; it is never exposed to the browser.
//   - The browser context is disposable (Target.createBrowserContext) so the
//     login session cannot leak into the muse2api default profile.
//   - Sessions are capped, single-owner, and expire after museBrowserTTL.
//   - Passwords/2FA are typed directly into the remote page via CDP; Lintasan
//     never receives or stores them.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	museBrowserWidth      = 1280
	museBrowserHeight     = 900
	museBrowserMaxSession = 1
	museBrowserTTL        = 15 * time.Minute
	museBrowserQuality    = 60
)

var museSessionCookieNames = []string{"hatch_sess", "hatch_gw", "hatch_vml", "hatch_native_auth_device"}

type museBrowserSession struct {
	server *Server
	id     string
	owner  string
	label  string
	width  int
	height int

	cdp              *museCDP
	browserContextID string
	targetID         string
	pageSessionID    string

	frames      chan []byte
	events      chan []byte
	done        chan struct{}
	cleanupDone chan struct{}

	finishOnce sync.Once
}

type museBrowserInput struct {
	Type       string  `json:"type"`
	Event      string  `json:"event"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Button     string  `json:"button"`
	ClickCount int     `json:"clickCount"`
	Key        string  `json:"key"`
	Text       string  `json:"text"`
	DeltaY     float64 `json:"deltaY"`
}

type museCDPMessage struct {
	ID        int             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type museCDP struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int
	pending map[int]chan museCDPMessage
	closed  chan struct{}

	onEvent func(sessionID, method string, params json.RawMessage)
	onClose func()

	closeOnce sync.Once
}

func dialMuseCDP(wsURL string) (*museCDP, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	header := http.Header{"Origin": []string{"http://127.0.0.1"}}
	ws, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("connect to Chromium CDP: %w", err)
	}
	c := &museCDP{
		ws:      ws,
		pending: make(map[int]chan museCDPMessage),
		closed:  make(chan struct{}),
	}
	go c.dispatch()
	return c, nil
}

func (c *museCDP) dispatch() {
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.close()
			if c.onClose != nil {
				c.onClose()
			}
			return
		}
		var msg museCDPMessage
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg.ID > 0 {
			c.mu.Lock()
			ch := c.pending[msg.ID]
			delete(c.pending, msg.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		if msg.Method != "" && c.onEvent != nil {
			c.onEvent(msg.SessionID, msg.Method, msg.Params)
		}
	}
}

func (c *museCDP) removePending(id int) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *museCDP) send(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan museCDPMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		c.removePending(id)
		return nil, err
	}
	if err := c.write(payload); err != nil {
		c.removePending(id)
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("cdp %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	case <-c.closed:
		c.removePending(id)
		return nil, errors.New("cdp connection closed")
	}
}

// notify sends a command without waiting for its response. Used for the
// high-frequency screencast acknowledgement where a response is irrelevant.
func (c *museCDP) notify(method string, params any, sessionID string) {
	c.mu.Lock()
	c.nextID++
	msg := map[string]any{"id": c.nextID, "method": method}
	c.mu.Unlock()
	if params != nil {
		msg["params"] = params
	}
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	_ = c.write(payload)
}

func (c *museCDP) write(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.ws.WriteMessage(websocket.TextMessage, payload)
}

func (c *museCDP) close() {
	c.closeOnce.Do(func() {
		_ = c.ws.Close()
		close(c.closed)
	})
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// museBrowserWebSocketURL validates that the configured sidecar CDP endpoint is
// local and resolves Chrome's browser WebSocket URL from /json/version.
func museBrowserWebSocketURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", errors.New("muse CDP URL must be an http(s) URL")
	}
	if !isLoopbackHost(u.Hostname()) {
		return "", errors.New("muse CDP URL must be loopback (127.0.0.1/localhost)")
	}
	base := strings.TrimRight(raw, "/")
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{Proxy: nil},
	}
	resp, err := client.Get(base + "/json/version")
	if err != nil {
		return "", fmt.Errorf("Chromium CDP endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Chromium CDP endpoint returned HTTP %d", resp.StatusCode)
	}
	var version struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil || version.WS == "" {
		return "", errors.New("Chromium CDP endpoint did not return a WebSocket URL")
	}
	wsURL, err := url.Parse(version.WS)
	if err != nil || (wsURL.Scheme != "ws" && wsURL.Scheme != "wss") || !isLoopbackHost(wsURL.Hostname()) {
		return "", errors.New("Chromium CDP WebSocket URL is not loopback")
	}
	return version.WS, nil
}

func newMuseBrowserSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "musesess_" + hex.EncodeToString(b), nil
}

func (s *Server) startMuseBrowserSession(ctx context.Context, owner, label string) (*museBrowserSession, error) {
	wsURL, err := museBrowserWebSocketURL(s.museCDPURL)
	if err != nil {
		return nil, err
	}
	cdp, err := dialMuseCDP(wsURL)
	if err != nil {
		return nil, err
	}
	startCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var browserContextID, targetID, pageSessionID string
	fail := func(reason error) (*museBrowserSession, error) {
		if browserContextID != "" {
			_, _ = cdp.send(startCtx, "Target.disposeBrowserContext", map[string]any{"browserContextId": browserContextID}, "")
		}
		cdp.close()
		return nil, reason
	}

	raw, err := cdp.send(startCtx, "Target.createBrowserContext", map[string]any{}, "")
	if err != nil {
		return fail(fmt.Errorf("create isolated browser context: %w", err))
	}
	var contextResult struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(raw, &contextResult); err != nil || contextResult.BrowserContextID == "" {
		return fail(errors.New("Chromium did not return a browser context id"))
	}
	browserContextID = contextResult.BrowserContextID

	// Load muse.ai directly in the new target. Navigating an attached page can
	// swap the renderer and lose the CDP response, so the URL is set here.
	raw, err = cdp.send(startCtx, "Target.createTarget", map[string]any{
		"url":              "https://muse.ai/",
		"browserContextId": browserContextID,
		"width":            museBrowserWidth,
		"height":           museBrowserHeight,
	}, "")
	if err != nil {
		return fail(fmt.Errorf("create browser tab: %w", err))
	}
	var targetResult struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(raw, &targetResult); err != nil || targetResult.TargetID == "" {
		return fail(errors.New("Chromium did not return a target id"))
	}
	targetID = targetResult.TargetID

	raw, err = cdp.send(startCtx, "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	}, "")
	if err != nil {
		return fail(fmt.Errorf("attach to browser tab: %w", err))
	}
	var attachResult struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &attachResult); err != nil || attachResult.SessionID == "" {
		return fail(errors.New("Chromium did not return a page session id"))
	}
	pageSessionID = attachResult.SessionID

	id, err := newMuseBrowserSessionID()
	if err != nil {
		return fail(err)
	}
	sess := &museBrowserSession{
		server:           s,
		id:               id,
		owner:            owner,
		label:            label,
		width:            museBrowserWidth,
		height:           museBrowserHeight,
		cdp:              cdp,
		browserContextID: browserContextID,
		targetID:         targetID,
		pageSessionID:    pageSessionID,
		frames:           make(chan []byte, 1),
		events:           make(chan []byte, 16),
		done:             make(chan struct{}),
		cleanupDone:      make(chan struct{}),
	}
	cdp.onClose = func() {
		s.finishMuseBrowserSession(sess, map[string]any{"type": "error", "error": "browser connection closed"})
	}

	commands := []struct {
		method string
		params any
	}{
		{"Page.enable", nil},
		{"Page.bringToFront", nil},
		{"Network.enable", nil},
		{"Emulation.setDeviceMetricsOverride", map[string]any{
			"width": museBrowserWidth, "height": museBrowserHeight, "deviceScaleFactor": 1, "mobile": false,
		}},
	}
	for _, command := range commands {
		cmdCtx, cmdCancel := context.WithTimeout(startCtx, 5*time.Second)
		_, err := cdp.send(cmdCtx, command.method, command.params, pageSessionID)
		cmdCancel()
		if err != nil {
			cdp.close()
			return fail(fmt.Errorf("prepare login page: %s: %w", command.method, err))
		}
	}
	go sess.captureFrames()
	return sess, nil
}

// captureFrames polls Page.captureScreenshot. Headless Chrome on this host
// starts Page.startScreencast without producing frames, while screenshot
// capture is reliable.
func (sess *museBrowserSession) captureFrames() {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-sess.done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			raw, err := sess.cdp.send(ctx, "Page.captureScreenshot", map[string]any{
				"format": "jpeg", "quality": museBrowserQuality, "fromSurface": true,
			}, sess.pageSessionID)
			cancel()
			if err != nil {
				failures++
				if failures >= 25 {
					// The login tab was closed/replaced. Cookie polling continues
					// at the browser-context level until import or timeout.
					return
				}
				continue
			}
			failures = 0
			var shot struct {
				Data string `json:"data"`
			}
			if json.Unmarshal(raw, &shot) != nil || shot.Data == "" {
				continue
			}
			frame, _ := json.Marshal(map[string]any{
				"type": "frame", "data": shot.Data, "w": sess.width, "h": sess.height,
			})
			sess.emitFrame(frame)
		}
	}
}

func (sess *museBrowserSession) emit(payload map[string]any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	select {
	case sess.events <- raw:
	default:
		select {
		case <-sess.events:
		default:
		}
		select {
		case sess.events <- raw:
		default:
		}
	}
}

func (sess *museBrowserSession) emitFrame(payload []byte) {
	select {
	case sess.frames <- payload:
	default:
		select {
		case <-sess.frames:
		default:
		}
		select {
		case sess.frames <- payload:
		default:
		}
	}
}

func (s *Server) finishMuseBrowserSession(sess *museBrowserSession, terminal map[string]any) {
	if sess == nil {
		return
	}
	sess.finishOnce.Do(func() {
		if terminal != nil {
			sess.emit(terminal)
			detail := fmt.Sprint(terminal["type"])
			if errText, ok := terminal["error"].(string); ok && errText != "" {
				detail += ": " + errText
			}
			fmt.Fprintf(os.Stderr, "[muse-browser] %s finished: %s\n", sess.id, detail)
		} else {
			fmt.Fprintf(os.Stderr, "[muse-browser] %s closed\n", sess.id)
		}
		close(sess.done)
		s.museBrowserMu.Lock()
		if sess.id != "" {
			delete(s.museBrowserSessions, sess.id)
		}
		s.museBrowserMu.Unlock()
		go func() {
			defer close(sess.cleanupDone)
			if sess.cdp != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if sess.targetID != "" {
					_, _ = sess.cdp.send(ctx, "Target.closeTarget", map[string]any{"targetId": sess.targetID}, "")
				}
				if sess.browserContextID != "" {
					_, _ = sess.cdp.send(ctx, "Target.disposeBrowserContext", map[string]any{"browserContextId": sess.browserContextID}, "")
				}
				sess.cdp.close()
			}
		}()
	})
}

func (s *Server) getMuseBrowserSession(id, owner string) (*museBrowserSession, bool) {
	s.museBrowserMu.Lock()
	defer s.museBrowserMu.Unlock()
	sess, ok := s.museBrowserSessions[id]
	if !ok || sess.owner != owner {
		return nil, false
	}
	return sess, true
}

func (s *Server) handleMuseBrowserStart(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if !s.museConfigured() {
		museError(w, http.StatusServiceUnavailable, "muse sidecar is not configured")
		return
	}
	var input struct {
		Label string `json:"label"`
	}
	limited := http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(limited).Decode(&input); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		label = "muse-browser"
	}
	if len(label) > 80 {
		label = label[:80]
	}

	s.museBrowserMu.Lock()
	for _, existing := range s.museBrowserSessions {
		if existing.owner == user.Username {
			s.museBrowserMu.Unlock()
			writeJSONStatus(w, http.StatusConflict, map[string]string{"error": "a browser login session is already active"})
			return
		}
	}
	if len(s.museBrowserSessions) >= museBrowserMaxSession {
		s.museBrowserMu.Unlock()
		writeJSONStatus(w, http.StatusTooManyRequests, map[string]string{"error": "too many browser login sessions"})
		return
	}
	s.museBrowserMu.Unlock()

	sess, err := s.startMuseBrowserSession(r.Context(), user.Username, label)
	if err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"error":  "failed to start the muse.ai login browser",
			"detail": err.Error(),
		})
		return
	}

	s.museBrowserMu.Lock()
	// Re-check after the (slow) start so two concurrent requests cannot exceed
	// the single-owner rule.
	for _, existing := range s.museBrowserSessions {
		if existing.owner == user.Username {
			s.museBrowserMu.Unlock()
			s.finishMuseBrowserSession(sess, map[string]any{"type": "closed", "message": "superseded"})
			writeJSONStatus(w, http.StatusConflict, map[string]string{"error": "a browser login session is already active"})
			return
		}
	}
	if len(s.museBrowserSessions) >= museBrowserMaxSession {
		s.museBrowserMu.Unlock()
		s.finishMuseBrowserSession(sess, map[string]any{"type": "closed", "message": "too many sessions"})
		writeJSONStatus(w, http.StatusTooManyRequests, map[string]string{"error": "too many browser login sessions"})
		return
	}
	s.museBrowserSessions[sess.id] = sess
	s.museBrowserMu.Unlock()

	fmt.Fprintf(os.Stderr, "[muse-browser] %s started (owner=%s, label=%s)\n", sess.id, user.Username, sess.label)
	go s.monitorMuseBrowserSession(sess)
	writeJSON(w, map[string]any{
		"session_id": sess.id,
		"stream_url": "/api/muse/browser/sessions/" + sess.id + "/stream",
		"input_url":  "/api/muse/browser/sessions/" + sess.id + "/input",
		"width":      sess.width,
		"height":     sess.height,
	})
}

func (s *Server) handleMuseBrowserStream(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	sess, ok := s.getMuseBrowserSession(r.PathValue("id"), user.Username)
	if !ok {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "browser session not found"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	writeEvent := func(raw []byte) bool {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	isTerminal := func(raw []byte) bool {
		var ev struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &ev) != nil {
			return false
		}
		return ev.Type == "imported" || ev.Type == "error" || ev.Type == "closed"
	}

	for {
		select {
		case raw := <-sess.events:
			if !writeEvent(raw) || isTerminal(raw) {
				return
			}
		case frame := <-sess.frames:
			if !writeEvent(frame) {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-sess.done:
			for {
				select {
				case raw := <-sess.events:
					if !writeEvent(raw) || isTerminal(raw) {
						return
					}
				default:
					return
				}
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleMuseBrowserInput(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	sess, ok := s.getMuseBrowserSession(r.PathValue("id"), user.Username)
	if !ok {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "browser session not found"})
		return
	}
	var input museBrowserInput
	limited := http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(limited).Decode(&input); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid input"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := sess.sendInput(ctx, input); err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{"error": "browser input failed", "detail": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleMuseBrowserCancel(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	sess, ok := s.getMuseBrowserSession(r.PathValue("id"), user.Username)
	if !ok {
		writeJSONStatus(w, http.StatusNotFound, map[string]string{"error": "browser session not found"})
		return
	}
	s.finishMuseBrowserSession(sess, map[string]any{"type": "closed", "message": "cancelled"})
	writeJSON(w, map[string]any{"closed": true})
}

func (s *Server) monitorMuseBrowserSession(sess *museBrowserSession) {
	timeout := time.NewTimer(museBrowserTTL)
	ticker := time.NewTicker(time.Second)
	defer timeout.Stop()
	defer ticker.Stop()

	lastSignature := ""
	for {
		select {
		case <-sess.done:
			return
		case <-timeout.C:
			s.finishMuseBrowserSession(sess, map[string]any{"type": "error", "error": "login timed out"})
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			cookies, err := sess.readSessionCookies(ctx)
			cancel()
			if err != nil {
				continue
			}
			missing := make([]string, 0, len(museSessionCookieNames))
			for _, name := range museSessionCookieNames {
				if cookies[name] == "" {
					missing = append(missing, name)
				}
			}
			signature := strings.Join(missing, ",")
			if signature != lastSignature {
				lastSignature = signature
				fmt.Fprintf(os.Stderr, "[muse-browser] %s cookies %d/%d missing=%s\n",
					sess.id, len(museSessionCookieNames)-len(missing), len(museSessionCookieNames), signature)
				if len(missing) == 0 {
					sess.emit(map[string]any{"type": "status", "message": "muse.ai session detected — importing…"})
				} else {
					sess.emit(map[string]any{
						"type":    "status",
						"message": fmt.Sprintf("Cookies %d/%d — waiting for: %s", len(museSessionCookieNames)-len(missing), len(museSessionCookieNames), strings.Join(missing, ", ")),
					})
				}
			}
			if len(cookies) != len(museSessionCookieNames) || len(missing) > 0 {
				continue
			}
			s.importMuseBrowserCookies(sess, cookies)
			return
		}
	}
}

func (sess *museBrowserSession) readSessionCookies(ctx context.Context) (map[string]string, error) {
	// Read at the browser-context level: muse.ai may close or replace the
	// login tab after authentication, which makes the page session unusable
	// for Network.getCookies while the context still holds the cookies.
	raw, err := sess.cdp.send(ctx, "Storage.getCookies", map[string]any{
		"browserContextId": sess.browserContextID,
	}, "")
	if err != nil {
		return nil, err
	}
	var result struct {
		Cookies []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(museSessionCookieNames))
	for _, name := range museSessionCookieNames {
		want[name] = true
	}
	out := make(map[string]string, len(museSessionCookieNames))
	for _, c := range result.Cookies {
		if want[c.Name] && c.Value != "" {
			out[c.Name] = c.Value
		}
	}
	return out, nil
}

func (s *Server) importMuseBrowserCookies(sess *museBrowserSession, cookies map[string]string) {
	payload, err := json.Marshal(map[string]any{"label": sess.label, "cookies": cookies})
	if err != nil {
		s.finishMuseBrowserSession(sess, map[string]any{"type": "error", "error": "failed to encode cookies"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, data, err := s.museDoContext(ctx, http.MethodPost, "/admin/accounts", payload)
	if err != nil {
		s.finishMuseBrowserSession(sess, map[string]any{"type": "error", "error": "muse2api unreachable: " + err.Error()})
		return
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		s.finishMuseBrowserSession(sess, map[string]any{"type": "error", "error": "muse2api rejected the admin key"})
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.finishMuseBrowserSession(sess, map[string]any{"type": "error", "error": fmt.Sprintf("muse2api returned HTTP %d", resp.StatusCode)})
		return
	}
	var out struct {
		Account        map[string]any `json:"account"`
		MissingCookies []string       `json:"missing_cookies"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		s.finishMuseBrowserSession(sess, map[string]any{"type": "error", "error": "muse2api returned an invalid response"})
		return
	}
	s.finishMuseBrowserSession(sess, map[string]any{
		"type":            "imported",
		"account":         out.Account,
		"missing_cookies": out.MissingCookies,
	})
}

var museBrowserKeys = map[string]struct {
	Code string
	VK   int
}{
	"Enter":      {"Enter", 13},
	"Tab":        {"Tab", 9},
	"Backspace":  {"Backspace", 8},
	"Escape":     {"Escape", 27},
	"Delete":     {"Delete", 46},
	"ArrowLeft":  {"ArrowLeft", 37},
	"ArrowUp":    {"ArrowUp", 38},
	"ArrowRight": {"ArrowRight", 39},
	"ArrowDown":  {"ArrowDown", 40},
	"Home":       {"Home", 36},
	"End":        {"End", 35},
}

func (sess *museBrowserSession) sendInput(ctx context.Context, input museBrowserInput) error {
	switch input.Type {
	case "text":
		if input.Text == "" || len(input.Text) > 1024 {
			return errors.New("invalid text")
		}
		_, err := sess.cdp.send(ctx, "Input.insertText", map[string]any{"text": input.Text}, sess.pageSessionID)
		return err
	case "key":
		k, ok := museBrowserKeys[input.Key]
		if !ok {
			return errors.New("unsupported key")
		}
		params := map[string]any{
			"key":                   input.Key,
			"code":                  k.Code,
			"windowsVirtualKeyCode": k.VK,
			"nativeVirtualKeyCode":  k.VK,
			"type":                  "keyDown",
		}
		if input.Key == "Enter" {
			params["text"] = "\r"
		}
		if _, err := sess.cdp.send(ctx, "Input.dispatchKeyEvent", params, sess.pageSessionID); err != nil {
			return err
		}
		params["type"] = "keyUp"
		delete(params, "text")
		_, err := sess.cdp.send(ctx, "Input.dispatchKeyEvent", params, sess.pageSessionID)
		return err
	case "mouse":
		if input.Event != "pressed" && input.Event != "released" && input.Event != "moved" {
			return errors.New("invalid mouse event")
		}
		x := input.X
		y := input.Y
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if int(x) > sess.width-1 {
			x = float64(sess.width - 1)
		}
		if int(y) > sess.height-1 {
			y = float64(sess.height - 1)
		}
		button := input.Button
		if button == "" {
			button = "left"
		}
		if input.Event == "moved" {
			button = "none"
		}
		clickCount := input.ClickCount
		if clickCount < 1 && input.Event != "moved" {
			clickCount = 1
		}
		msgType := "mouseMoved"
		switch input.Event {
		case "pressed":
			msgType = "mousePressed"
		case "released":
			msgType = "mouseReleased"
		}
		buttons := 0
		if input.Event == "pressed" {
			buttons = 1
		}
		_, err := sess.cdp.send(ctx, "Input.dispatchMouseEvent", map[string]any{
			"type": msgType, "x": x, "y": y, "button": button, "buttons": buttons, "clickCount": clickCount,
		}, sess.pageSessionID)
		return err
	case "wheel":
		delta := input.DeltaY
		if delta > 2000 {
			delta = 2000
		}
		if delta < -2000 {
			delta = -2000
		}
		_, err := sess.cdp.send(ctx, "Input.dispatchMouseEvent", map[string]any{
			"type": "mouseWheel", "x": sess.width / 2, "y": sess.height / 2, "deltaX": 0, "deltaY": delta,
		}, sess.pageSessionID)
		return err
	default:
		return errors.New("unsupported input type")
	}
}

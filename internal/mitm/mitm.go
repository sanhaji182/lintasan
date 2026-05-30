package mitm

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

// MITMProxy intercepts IDE traffic (Cursor, Codex, Claude Desktop) and
// transparently forwards it to Lintasan with the per-boot bypass secret.
type MITMProxy struct {
	listenPort int
	targetPort int
	db         *db.DB
	secret     string // per-boot bypass secret injected into forwarded requests
	listener   net.Listener
	server     *http.Server
}

// New creates a new MITM proxy that listens on listenPort and forwards to targetPort.
// secret is the per-boot bypass token; it must match the server's mitmSecret.
func New(listenPort, targetPort int, database *db.DB, secret string) *MITMProxy {
	return &MITMProxy{
		listenPort: listenPort,
		targetPort: targetPort,
		db:         database,
		secret:     secret,
	}
}

// Start begins listening and forwarding IDE traffic.
func (m *MITMProxy) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handleProxy)

	m.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", m.listenPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", m.server.Addr)
	if err != nil {
		return fmt.Errorf("mitm listen on :%d: %w", m.listenPort, err)
	}
	m.listener = ln

	fmt.Printf("🔒 MITM bridge listening on :%d → forwarding to Lintasan on :%d\n", m.listenPort, m.targetPort)
	return m.server.Serve(ln)
}

// Stop gracefully shuts down the MITM proxy.
func (m *MITMProxy) Stop() error {
	if m.server != nil {
		return m.server.Close()
	}
	return nil
}

// GetListenPort returns the port the MITM proxy listens on.
func (m *MITMProxy) GetListenPort() int {
	return m.listenPort
}

// handleProxy forwards all requests to the target Lintasan instance.
func (m *MITMProxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	targetURL := fmt.Sprintf("http://localhost:%d%s", m.targetPort, r.URL.RequestURI())
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create proxy request
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy all original headers
	for k, vv := range r.Header {
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	// Add per-boot MITM bypass secret so Lintasan auth middleware skips auth.
	proxyReq.Header.Set("X-Lintasan-MITM", m.secret)

	// Preserve original host
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)
	if clientIP := r.Header.Get("X-Forwarded-For"); clientIP != "" {
		proxyReq.Header.Set("X-Forwarded-For", clientIP+", "+r.RemoteAddr)
	} else {
		proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}

	client := &http.Client{
		Timeout: 300 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
		},
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("mitm proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)

	// Stream the response body
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
	}
}

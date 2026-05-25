package server

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

type ProxyHandler struct {
	cfg    *config.Config
	db     *db.DB
	client *http.Client
}

func NewProxyHandler(cfg *config.Config, database *db.DB) *ProxyHandler {
	return &ProxyHandler{
		cfg: cfg,
		db:  database,
		client: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
			},
		},
	}
}

type Connection struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Format     string `json:"format"`
	ChatPath   string `json:"chat_path"`
	AuthHeader string `json:"auth_header"`
	AuthPrefix string `json:"auth_prefix"`
	IsActive   int    `json:"is_active"`
	Priority   int    `json:"priority"`
}

func (p *ProxyHandler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	model, _ := req["model"].(string)
	if model == "" {
		http.Error(w, `{"error":"model is required"}`, http.StatusBadRequest)
		return
	}

	candidates, resolvedModel, err := p.resolveRoute(model)
	if err != nil || len(candidates) == 0 {
		http.Error(w, fmt.Sprintf(`{"error":"no route found for model %s"}`, model), http.StatusNotFound)
		return
	}
	req["model"] = resolvedModel
	body, _ = json.Marshal(req)

	stream, _ := req["stream"].(bool)
	var lastErr string
	var lastStatus int
	for i, conn := range candidates {
		resp, err := p.doUpstream(r, conn, body)
		if err != nil {
			lastErr = err.Error()
			p.logRequest(resolvedModel, conn.ID, conn.Name, 502, time.Since(start).Milliseconds(), 0, 0, false, lastErr)
			continue
		}
		defer resp.Body.Close()
		lastStatus = resp.StatusCode
		if resp.StatusCode >= 500 && i < len(candidates)-1 {
			b, _ := io.ReadAll(resp.Body)
			lastErr = string(b)
			p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), 0, 0, false, lastErr)
			continue
		}

		for k, v := range resp.Header {
			for _, vv := range v { w.Header().Add(k, vv) }
		}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(resp.StatusCode)
			flusher, ok := w.(http.Flusher)
			if !ok { io.Copy(w, resp.Body); return }
			buf := make([]byte, 4096)
			for { n, er := resp.Body.Read(buf); if n > 0 { w.Write(buf[:n]); flusher.Flush() }; if er != nil { break } }
		} else {
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		}
		p.logRequest(resolvedModel, conn.ID, conn.Name, resp.StatusCode, time.Since(start).Milliseconds(), 0, 0, false, "")
		return
	}
	http.Error(w, fmt.Sprintf(`{"error":"all routes failed","status":%d,"last_error":%q}`, lastStatus, lastErr), http.StatusBadGateway)
}

func (p *ProxyHandler) HandleEmbeddings(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	model, _ := req["model"].(string)
	if model == "" {
		model = "text-embedding-3-small"
	}

	conn, err := p.findConnectionForModel(model)
	if err != nil {
		// Fallback: use first active connection
		conn, err = p.getFirstConnection()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"no connection found for model %s"}`, model), http.StatusNotFound)
			return
		}
	}

	upstreamURL := strings.TrimRight(conn.BaseURL, "/") + "/v1/embeddings"
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, `{"error":"failed to create upstream request"}`, http.StatusInternalServerError)
		return
	}

	upReq.Header.Set("Content-Type", "application/json")
	authHeader := conn.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authPrefix := conn.AuthPrefix
	if authPrefix == "" {
		authPrefix = "Bearer "
	}
	if conn.APIKey != "" {
		upReq.Header.Set(authHeader, authPrefix+conn.APIKey)
	}

	resp, err := p.client.Do(upReq)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"upstream error: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *ProxyHandler) doUpstream(r *http.Request, conn *Connection, body []byte) (*http.Response, error) {
	chatPath := conn.ChatPath
	if chatPath == "" { chatPath = "/v1/chat/completions" }
	upstreamURL := strings.TrimRight(conn.BaseURL, "/") + chatPath
	upReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, strings.NewReader(string(body)))
	if err != nil { return nil, err }
	upReq.Header.Set("Content-Type", "application/json")
	authHeader := conn.AuthHeader; if authHeader == "" { authHeader = "Authorization" }
	authPrefix := conn.AuthPrefix; if authPrefix == "" { authPrefix = "Bearer " }
	if conn.APIKey != "" { upReq.Header.Set(authHeader, authPrefix+conn.APIKey) }
	if xcc := r.Header.Get("X-Command-Code-Version"); xcc != "" { upReq.Header.Set("X-Command-Code-Version", xcc) }
	return p.client.Do(upReq)
}

func (p *ProxyHandler) resolveRoute(model string) ([]*Connection, string, error) {
	model = p.resolveAlias(model)
	if conns, resolved, ok := p.resolveCombo(model); ok && len(conns) > 0 { return conns, resolved, nil }
	conn, err := p.findConnectionForModel(model)
	if err != nil { return nil, model, err }
	return []*Connection{conn}, model, nil
}

func (p *ProxyHandler) resolveAlias(model string) string {
	v, _ := p.db.GetSetting("aliases")
	if v == "" { return model }
	var m map[string]any
	if json.Unmarshal([]byte(v), &m) != nil { return model }
	if raw, ok := m[model]; ok {
		switch t := raw.(type) {
		case string: return t
		case map[string]any: if s, _ := t["model"].(string); s != "" { return s }
		}
	}
	return model
}

func (p *ProxyHandler) resolveCombo(name string) ([]*Connection, string, bool) {
	v, _ := p.db.GetSetting("combos")
	if v == "" { return nil, name, false }
	var combos []map[string]any
	if json.Unmarshal([]byte(v), &combos) != nil { return nil, name, false }
	for _, c := range combos {
		if c["name"] != name { continue }
		entries, _ := c["entries"].([]any)
		strategy, _ := c["strategy"].(string)
		if strategy == "round-robin" && len(entries) > 1 {
			idx := p.nextRoundRobinIndex("combo_rr_"+name, len(entries))
			entries = append(entries[idx:], entries[:idx]...)
		}
		var out []*Connection
		resolvedModel := name
		for _, e := range entries {
			em, _ := e.(map[string]any); if em == nil { continue }
			m, _ := em["model"].(string); if m == "" { continue }
			if resolvedModel == name { resolvedModel = m }
			ids := stringSlice(em["connection_ids"])
			conns := p.connectionsForModelAndIDs(m, ids)
			out = append(out, conns...)
		}
		return out, resolvedModel, true
	}
	return nil, name, false
}

func stringSlice(v any) []string { arr, _ := v.([]any); out := []string{}; for _, x := range arr { if s, ok := x.(string); ok { out = append(out, s) } }; return out }

func (p *ProxyHandler) nextRoundRobinIndex(key string, n int) int {
	if n <= 1 { return 0 }
	v, _ := p.db.GetSetting(key); var i int; fmt.Sscanf(v, "%d", &i); next := (i+1)%n; p.db.SetSetting(key, fmt.Sprintf("%d", next)); return i % n
}

func (p *ProxyHandler) connectionsForModelAndIDs(model string, ids []string) []*Connection {
	query := `SELECT c.id, c.name, c.base_url, c.api_key, c.format, c.chat_path, c.auth_header, c.auth_prefix, c.is_active, c.priority FROM discovered_models m JOIN connections c ON m.connection_id=c.id WHERE m.model_id=? AND m.is_active=1 AND c.is_active=1`
	args := []any{model}
	if len(ids) > 0 { ph:=make([]string,len(ids)); for i,id:=range ids{ph[i]="?"; args=append(args,id)}; query += " AND c.id IN ("+strings.Join(ph,",")+")" }
	query += " ORDER BY c.priority DESC"
	rows, err := p.db.Conn().Query(query, args...); if err != nil { return nil }; defer rows.Close()
	var out []*Connection
	for rows.Next(){ var c Connection; if rows.Scan(&c.ID,&c.Name,&c.BaseURL,&c.APIKey,&c.Format,&c.ChatPath,&c.AuthHeader,&c.AuthPrefix,&c.IsActive,&c.Priority)==nil{ out=append(out,&c) } }
	return out
}

func (p *ProxyHandler) findConnectionForModel(model string) (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT c.id, c.name, c.base_url, c.api_key, c.format, c.chat_path, c.auth_header, c.auth_prefix, c.is_active, c.priority
		FROM discovered_models m
		JOIN connections c ON m.connection_id = c.id
		WHERE m.model_id = ? AND m.is_active = 1 AND c.is_active = 1
		ORDER BY c.priority DESC
		LIMIT 1
	`, model)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority)
	if err != nil {
		return nil, fmt.Errorf("model not found: %s", model)
	}
	return &conn, nil
}

func (p *ProxyHandler) getFirstConnection() (*Connection, error) {
	row := p.db.Conn().QueryRow(`
		SELECT id, name, base_url, api_key, format, chat_path, auth_header, auth_prefix, is_active, priority
		FROM connections
		WHERE is_active = 1
		ORDER BY priority DESC
		LIMIT 1
	`)

	var conn Connection
	err := row.Scan(&conn.ID, &conn.Name, &conn.BaseURL, &conn.APIKey, &conn.Format, &conn.ChatPath, &conn.AuthHeader, &conn.AuthPrefix, &conn.IsActive, &conn.Priority)
	if err != nil {
		return nil, fmt.Errorf("no active connections")
	}
	return &conn, nil
}

func (p *ProxyHandler) logRequest(model, connID, provider string, status int, latencyMs int64, tokensIn, tokensOut int, cached bool, errMsg string) {
	cachedInt := 0
	if cached {
		cachedInt = 1
	}

	id := uuid.New().String()
	p.db.Conn().Exec(`
		INSERT INTO request_logs (id, connection_id, provider, model, status, input_tokens, output_tokens, latency_ms, cached, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`, id, connID, provider, model, status, tokensIn, tokensOut, latencyMs, cachedInt, errMsg)
}

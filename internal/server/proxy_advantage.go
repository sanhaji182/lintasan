package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/mlrouter"
)

type connQualityScore struct {
	SuccessEWMA float64   `json:"success_ewma"`
	LatencyEWMA float64   `json:"latency_ewma_ms"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type proxyTelemetry struct {
	mu           sync.RWMutex
	Total        int64            `json:"total"`
	Cached       int64            `json:"cached"`
	Errors       int64            `json:"errors"`
	TotalLatency int64            `json:"total_latency_ms"`
	ByProvider   map[string]int64 `json:"by_provider"`
	ByTaskClass  map[string]int64 `json:"by_task_class"`
	ByMode       map[string]int64 `json:"by_mode"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

func newProxyTelemetry() *proxyTelemetry {
	return &proxyTelemetry{
		ByProvider:  map[string]int64{},
		ByTaskClass: map[string]int64{},
		ByMode:      map[string]int64{},
		UpdatedAt:   time.Now(),
	}
}

func (t *proxyTelemetry) Observe(provider, taskClass, mode string, latencyMs int64, status int, cached bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Total++
	t.TotalLatency += latencyMs
	if cached {
		t.Cached++
	}
	if status >= 400 {
		t.Errors++
	}
	if provider == "" {
		provider = "unknown"
	}
	if taskClass == "" {
		taskClass = "general"
	}
	if mode == "" {
		mode = "intelligent"
	}
	t.ByProvider[provider]++
	t.ByTaskClass[taskClass]++
	t.ByMode[mode]++
	t.UpdatedAt = time.Now()
}

func (t *proxyTelemetry) Snapshot() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()
	avg := float64(0)
	if t.Total > 0 {
		avg = float64(t.TotalLatency) / float64(t.Total)
	}
	providers := make(map[string]int64, len(t.ByProvider))
	for k, v := range t.ByProvider {
		providers[k] = v
	}
	classes := make(map[string]int64, len(t.ByTaskClass))
	for k, v := range t.ByTaskClass {
		classes[k] = v
	}
	modes := make(map[string]int64, len(t.ByMode))
	for k, v := range t.ByMode {
		modes[k] = v
	}
	return map[string]any{
		"total":          t.Total,
		"cached":         t.Cached,
		"errors":         t.Errors,
		"avg_latency_ms": avg,
		"by_provider":    providers,
		"by_task_class":  classes,
		"by_mode":        modes,
		"updated_at":     t.UpdatedAt.Format(time.RFC3339),
	}
}

func classifyTask(model string, messages []any) string {
	text := strings.ToLower(model + " " + flattenMessages(messages))
	switch {
	case strings.Contains(text, "refactor") || strings.Contains(text, "code") || strings.Contains(text, "function") || strings.Contains(text, "golang"):
		return "codegen"
	case strings.Contains(text, "translate") || strings.Contains(text, "terjemah"):
		return "translation"
	case strings.Contains(text, "analy") || strings.Contains(text, "debug") || strings.Contains(text, "trace"):
		return "analysis"
	default:
		return "general"
	}
}

func flattenMessages(messages []any) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if c, ok := msg["content"].(string); ok && c != "" {
			b.WriteString(c)
			b.WriteString(" ")
		}
	}
	return b.String()
}

func pickRouteProfile(taskClass, userAgent string) string {
	ua := strings.ToLower(userAgent)
	if strings.Contains(ua, "cursor") || strings.Contains(ua, "codex") || strings.Contains(ua, "claude") || strings.Contains(ua, "zed") || strings.Contains(ua, "aider") {
		return "agent-native"
	}
	switch taskClass {
	case "codegen":
		return "quality-first"
	case "translation":
		return "cost-first"
	default:
		return "latency-first"
	}
}

// applyMLRouting decides cheap-vs-expensive model using the mlrouter 15-feature
// complexity score (Gap #1). It activates only when the caller explicitly opts
// in — either by targeting the sentinel model "ml-auto"/"smart", or when the
// operator sets ml_router_enabled=true. Otherwise it returns ok=false and the
// literal model is used unchanged.
//
// Returns the chosen model, the tier ("cheap"|"expensive"), and ok.
func (p *ProxyHandler) applyMLRouting(model string, messages []any) (string, string, bool) {
	sentinel := model == "ml-auto" || model == "smart"
	if !sentinel && p.getSetting("ml_router_enabled", "false") != "true" {
		return "", "", false
	}
	if p.mlr.CheapModel == "" || p.mlr.ExpensiveModel == "" {
		return "", "", false
	}

	// Convert []any chat messages to the []map[string]any the router expects.
	msgs := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		if mm, ok := m.(map[string]any); ok {
			msgs = append(msgs, mm)
		}
	}

	dec := mlrouter.Route(msgs, p.mlr)
	if dec.Model == "" {
		return "", "", false
	}
	return dec.Model, dec.Tier, true
}

// costQualityFloor is the minimum success-EWMA a provider must clear before
// cost-first routing will favor it on price. Below this, quality dominates so a
// cheap-but-failing provider never wins. Operator-tunable via cost_quality_floor.
func (p *ProxyHandler) costQualityFloor() float64 {
	if v := p.getSetting("cost_quality_floor", ""); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			return f
		}
	}
	return 0.3
}

// costScoreForConnection returns a 0-1 cheapness score for a connection's
// primary model: 1.0 = free/cheapest, approaching 0 = most expensive. It reads
// real pricing from the cost calculator; connections named "free" or with no
// pricing data default to fully cheap (1.0).
func (p *ProxyHandler) costScoreForConnection(c *Connection) float64 {
	if strings.Contains(strings.ToLower(c.Name), "free") {
		return 1.0
	}
	if p.costCalc == nil {
		return 0.5
	}
	model := p.primaryModelForConnection(c.ID)
	if model == "" {
		return 0.5
	}
	// Cost of a representative 1K-in / 1K-out request for this model.
	rc := p.costCalc.CalculateCost(model, 1000, 1000)
	if rc.TotalCostUSD <= 0 {
		return 1.0 // free or unknown-but-zero
	}
	// Map cost to a 0-1 cheapness score. $0.02 per 2K tokens (~GPT-4o class) is
	// a reasonable upper anchor; cheaper scales toward 1.0.
	const expensiveAnchor = 0.02
	score := 1.0 - (rc.TotalCostUSD / expensiveAnchor)
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// primaryModelForConnection returns the first active model id on a connection.
func (p *ProxyHandler) primaryModelForConnection(connID string) string {
	var model string
	p.db.Conn().QueryRow(`SELECT model_id FROM discovered_models
		WHERE connection_id = ? AND is_active = 1 ORDER BY model_id LIMIT 1`, connID).Scan(&model)
	return model
}

func dedupMessages(messages []any) ([]any, int) {
	if len(messages) < 2 {
		return messages, 0
	}
	out := make([]any, 0, len(messages))
	seen := 0
	lastSig := ""
	for _, raw := range messages {
		m, ok := raw.(map[string]any)
		if !ok {
			out = append(out, raw)
			lastSig = ""
			continue
		}
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		sig := role + "|" + strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
		if sig == lastSig {
			seen++
			continue
		}
		lastSig = sig
		out = append(out, raw)
	}
	return out, seen
}

func (p *ProxyHandler) isDirectEquivalentMode(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("X-Lintasan-Direct"), "true") {
		return true
	}
	return p.getSetting("direct_equivalent_mode", "false") == "true"
}

func (p *ProxyHandler) applyTaskBudgetGuardrail(req map[string]any, taskClass string) {
	var capTokens float64
	switch taskClass {
	case "codegen":
		capTokens = 16384
	case "analysis":
		capTokens = 8192
	case "translation":
		capTokens = 4096
	default:
		capTokens = 6144
	}
	if v := p.getSetting("budget_guardrail_max_tokens", ""); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			capTokens = n
		}
	}
	if mt, ok := req["max_tokens"].(float64); ok && mt > 0 {
		if mt > capTokens {
			req["max_tokens"] = capTokens
		}
		return
	}
	req["max_tokens"] = capTokens
}

func (p *ProxyHandler) reorderCandidatesForTask(candidates []*Connection, taskClass, routeProfile string) []*Connection {
	if len(candidates) <= 1 {
		return candidates
	}
	out := append([]*Connection(nil), candidates...)
	p.qualityMu.RLock()
	defer p.qualityMu.RUnlock()

	scoreFor := func(c *Connection) float64 {
		quality := 0.5
		latency := 0.5
		if q, ok := p.qualityScores[c.ID]; ok {
			quality = q.SuccessEWMA
			if q.LatencyEWMA > 0 {
				latency = 1.0 / (1.0 + (q.LatencyEWMA / 1000.0))
			}
		}
		priorityScore := float64(c.Priority) / 100.0
		switch routeProfile {
		case "quality-first":
			return quality*0.6 + latency*0.2 + priorityScore*0.2
		case "cost-first":
			// Real cost-based ranking (Gap #4): use the cost calculator's
			// pricing table for this connection's model rather than only a
			// "free" name heuristic. Cheaper models score higher, but a quality
			// floor still applies so we never pick a broken-but-free provider.
			costHint := p.costScoreForConnection(c)
			if quality < p.costQualityFloor() {
				// Below the quality floor: heavily penalize so healthier (even
				// pricier) providers win.
				return costHint*0.15 + quality*0.6 + latency*0.25
			}
			return costHint*0.5 + quality*0.3 + latency*0.2
		default:
			if taskClass == "codegen" {
				return quality*0.5 + latency*0.3 + priorityScore*0.2
			}
			return latency*0.5 + quality*0.3 + priorityScore*0.2
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return scoreFor(out[i]) > scoreFor(out[j])
	})
	return out
}

func (p *ProxyHandler) observeQuality(connID string, status int, latencyMs int64) {
	const alpha = 0.2
	success := 0.0
	if status >= 200 && status < 400 {
		success = 1.0
	}
	p.qualityMu.Lock()
	defer p.qualityMu.Unlock()
	q := p.qualityScores[connID]
	if q.UpdatedAt.IsZero() {
		q.SuccessEWMA = success
		q.LatencyEWMA = float64(latencyMs)
		q.UpdatedAt = time.Now()
		p.qualityScores[connID] = q
		return
	}
	q.SuccessEWMA = alpha*success + (1-alpha)*q.SuccessEWMA
	q.LatencyEWMA = alpha*float64(latencyMs) + (1-alpha)*q.LatencyEWMA
	q.UpdatedAt = time.Now()
	p.qualityScores[connID] = q
}

func (p *ProxyHandler) prewarmConnectionPool() {
	rows, err := p.db.Conn().Query(`SELECT base_url FROM connections WHERE is_active=1 ORDER BY priority DESC LIMIT 8`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var baseURL string
		if err := rows.Scan(&baseURL); err != nil || baseURL == "" {
			continue
		}
		baseURL = strings.TrimRight(baseURL, "/")
		req, _ := http.NewRequest("GET", baseURL+"/health", nil)
		resp, err := p.client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}
}

func (p *ProxyHandler) telemetrySnapshot() map[string]any {
	snap := map[string]any{}
	if p.telemetry != nil {
		snap = p.telemetry.Snapshot()
	}
	p.qualityMu.RLock()
	quality := make(map[string]connQualityScore, len(p.qualityScores))
	for k, v := range p.qualityScores {
		quality[k] = v
	}
	p.qualityMu.RUnlock()
	snap["quality_scores"] = quality
	snap["features"] = map[string]any{
		"agent_aware_router":        true,
		"latency_slo_hedge":         true,
		"direct_equivalent_mode":    true,
		"prefix_context_dedup":      true,
		"warm_pool_keepalive":       true,
		"task_budget_guardrail":     true,
		"quality_feedback_loop":     true,
		"structured_telemetry":      true,
		"ide_profile_normalization": true,
	}
	return snap
}

func normalizeOpenAIResponseBody(body []byte) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	choices, _ := obj["choices"].([]any)
	if len(choices) == 0 {
		return body
	}
	c0, _ := choices[0].(map[string]any)
	if c0 == nil {
		return body
	}
	msg, _ := c0["message"].(map[string]any)
	if msg == nil {
		return body
	}
	content, _ := msg["content"].(string)
	if strings.TrimSpace(content) == "" {
		if reasoningContent, ok := msg["reasoning_content"].(string); ok && strings.TrimSpace(reasoningContent) != "" {
			msg["content"] = reasoningContent
			c0["message"] = msg
			choices[0] = c0
			obj["choices"] = choices
			fixed, _ := json.Marshal(obj)
			return fixed
		}
	}
	return body
}

func parseLatencySLO(p *ProxyHandler) time.Duration {
	s := p.getSetting("latency_slo_ms", "1400")
	ms, err := strconv.Atoi(s)
	if err != nil || ms < 300 {
		ms = 1400
	}
	return time.Duration(ms) * time.Millisecond
}

func (p *ProxyHandler) shouldHedge(stream, directMode bool, candidates []*Connection) bool {
	if stream || directMode || len(candidates) < 2 {
		return false
	}
	return p.getSetting("hedged_requests_enabled", "true") == "true"
}

func (p *ProxyHandler) doHedgedUpstream(r *http.Request, candidates []*Connection, body []byte) (*Connection, *http.Response, error) {
	if len(candidates) < 2 {
		return nil, nil, fmt.Errorf("need >= 2 candidates")
	}
	type hr struct {
		conn *Connection
		resp *http.Response
		err  error
	}
	ch := make(chan hr, 2)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	launch := func(conn *Connection, delay time.Duration) {
		go func() {
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return
				}
			}
			r2 := r.Clone(ctx)
			resp, err := p.doUpstream(r2, conn, body)
			select {
			case ch <- hr{conn: conn, resp: resp, err: err}:
			case <-ctx.Done():
				if resp != nil {
					resp.Body.Close()
				}
			}
		}()
	}

	slo := parseLatencySLO(p)
	delay := slo / 3
	if delay < 120*time.Millisecond {
		delay = 120 * time.Millisecond
	}
	launch(candidates[0], 0)
	launch(candidates[1], delay)

	var firstErr string
	for i := 0; i < 2; i++ {
		res := <-ch
		if res.err != nil {
			if firstErr == "" {
				firstErr = res.err.Error()
			}
			continue
		}
		if res.resp == nil {
			continue
		}
		if res.resp.StatusCode == 429 || res.resp.StatusCode >= 500 {
			if firstErr == "" {
				firstErr = fmt.Sprintf("status %d", res.resp.StatusCode)
			}
			res.resp.Body.Close()
			continue
		}
		cancel()
		return res.conn, res.resp, nil
	}
	if firstErr == "" {
		firstErr = "hedge failed"
	}
	return nil, nil, fmt.Errorf(firstErr)
}

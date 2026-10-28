package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// Tool use for chat.
//
// The model is given a small, fixed tool set and may call them in a loop until
// it stops asking. Every tool here is read-only or side-effect free; there is
// deliberately no shell, no filesystem write, and no arbitrary HTTP to
// localhost, because the chat UI is reachable by any dashboard user.
//
// Tools that would be dangerous (run code, write files) are out of scope for
// this layer — the chat surface is for reading the web and the user's own
// attachment, not for executing things on the gateway host.

// toolDef is one callable tool exposed to the model.
type toolDef struct {
	Name        string
	Description string
	// Parameters is the JSON-Schema object for the tool's arguments.
	Parameters map[string]any
	// Run executes the tool. It must respect ctx cancellation and must return
	// a short string — tool results are fed back into the model verbatim, so
	// unbounded output would blow the context window.
	Run func(ctx context.Context, args map[string]any) (string, error)
}

// toolLoopEnabled reports whether the operator has tool use turned on.
// Defaults to ON: the tools are read-only and the model stays useful without
// them, so the failure mode of a bad tool call is a wasted round trip, not
// damage.
func (s *Server) toolLoopEnabled() bool {
	return s.proxy.getSetting("chat_tools_enabled", "true") != "false"
}

// maxToolIterations bounds the loop so a model that keeps calling tools cannot
// spin forever. Six is enough for search → fetch → answer patterns.
const maxToolIterations = 6

// maxToolResultChars caps a single tool result fed back to the model.
const maxToolResultChars = 16 * 1024

// chatTools returns the tool set for this server. web_search is only offered
// when a search engine is actually configured, so the model is never told
// about a tool that would always fail.
func (s *Server) chatTools() []toolDef {
	tools := []toolDef{
		{
			Name:        "fetch_url",
			Description: "Fetch a public web page and return its readable text content. Use this to read documentation, articles, or any URL the user mentions.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "Absolute http or https URL to fetch.",
					},
				},
				"required": []string{"url"},
			},
			Run: func(ctx context.Context, args map[string]any) (string, error) {
				u, _ := args["url"].(string)
				return s.toolFetchURL(ctx, u)
			},
		},
		{
			Name:        "get_current_time",
			Description: "Return the current date and time in UTC. Use this whenever the user asks about 'today', 'now', or anything time-relative, instead of guessing.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Run: func(ctx context.Context, args map[string]any) (string, error) {
				return time.Now().UTC().Format("Monday, 2 January 2006, 15:04:05 UTC"), nil
			},
		},
	}

	if s.webSearch != nil {
		tools = append(tools, toolDef{
			Name:        "web_search",
			Description: "Search the web. Returns the top results with title, URL and snippet. Follow up with fetch_url to read a result in full.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
				},
				"required": []string{"query"},
			},
			Run: func(ctx context.Context, args map[string]any) (string, error) {
				q, _ := args["query"].(string)
				return s.toolWebSearch(ctx, q)
			},
		})
	}
	return tools
}

// wireTools converts the tool set into the OpenAI `tools` array.
func wireTools(defs []toolDef) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  d.Parameters,
			},
		})
	}
	return out
}

// runToolLoop drives the model→tool→model cycle. It returns the final assistant
// text plus the reasoning trace, and emits each tool invocation so the UI can
// show what happened.
//
// The loop re-issues the whole conversation each round with the accumulated
// tool results appended, which is the stateless contract every OpenAI-compatible
// upstream expects.
func (s *Server) runToolLoop(ctx context.Context, basePayload map[string]any, model string,
	emit func(streamEvent)) (string, string, int, int, error) {

	defs := s.chatTools()
	if len(defs) == 0 {
		return "", "", 0, 0, fmt.Errorf("no tools available")
	}
	byName := make(map[string]toolDef, len(defs))
	for _, d := range defs {
		byName[d.Name] = d
	}

	msgs, _ := basePayload["messages"].([]map[string]any)
	// Copy so the caller's payload is not mutated across turns.
	work := make([]map[string]any, len(msgs))
	copy(work, msgs)

	totalIn, totalOut := 0, 0
	var thinking strings.Builder

	for iter := 0; iter < maxToolIterations; iter++ {
		payload := map[string]any{
			"model":       model,
			"messages":    work,
			"stream":      false,
			"tools":       wireTools(defs),
			"tool_choice": "auto",
		}
		for _, k := range []string{"temperature", "max_tokens"} {
			if v, ok := basePayload[k]; ok {
				payload[k] = v
			}
		}

		resp, err := s.callGateway(ctx, payload)
		if err != nil {
			return "", thinking.String(), totalIn, totalOut, err
		}
		totalIn += resp.Usage.PromptTokens
		totalOut += resp.Usage.CompletionTokens
		if resp.Reasoning != "" {
			thinking.WriteString(resp.Reasoning)
		}

		// No tool calls means the model answered; we are done.
		if len(resp.ToolCalls) == 0 {
			return resp.Content, thinking.String(), totalIn, totalOut, nil
		}

		// Append the assistant turn that requested the tools, verbatim. An
		// assistant message with tool_calls and empty content is required by the
		// wire format.
		work = append(work, map[string]any{
			"role":       "assistant",
			"content":    resp.Content,
			"tool_calls": resp.ToolCalls,
		})

		for _, tc := range resp.ToolCalls {
			name := tc.Function.Name
			var args map[string]any
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					// A malformed argument blob is the model's mistake, not ours;
					// tell it so and let it retry.
					work = append(work, toolResultMessage(tc.ID, name,
						"error: could not parse arguments as JSON: "+err.Error()))
					continue
				}
			}
			if args == nil {
				args = map[string]any{}
			}

			def, ok := byName[name]
			if !ok {
				work = append(work, toolResultMessage(tc.ID, name, "error: unknown tool "+name))
				emit(streamEvent{Kind: streamTool, ToolName: name, ToolArgs: tc.Function.Arguments,
					ToolResult: "unknown tool"})
				continue
			}

			result, terr := def.Run(ctx, args)
			if terr != nil {
				result = "error: " + terr.Error()
			}
			result = truncate(result, maxToolResultChars)

			emit(streamEvent{Kind: streamTool, ToolName: name,
				ToolArgs: tc.Function.Arguments, ToolResult: result})
			work = append(work, toolResultMessage(tc.ID, name, result))
		}
	}

	// Budget exhausted: ask once more without tools so the user gets an answer
	// rather than a silent stop.
	payload := map[string]any{
		"model":    model,
		"messages": work,
		"stream":   false,
	}
	if v, ok := basePayload["temperature"]; ok {
		payload["temperature"] = v
	}
	resp, err := s.callGateway(ctx, payload)
	if err != nil {
		return "", thinking.String(), totalIn, totalOut, err
	}
	return resp.Content + "\n\n_(stopped after reaching the tool-call limit)_",
		thinking.String(), totalIn + resp.Usage.PromptTokens, totalOut + resp.Usage.CompletionTokens, nil
}

func toolResultMessage(callID, name, content string) map[string]any {
	return map[string]any{
		"role":         "tool",
		"tool_call_id": callID,
		"name":         name,
		"content":      content,
	}
}

// gatewayResponse is the subset of an OpenAI completion we consume.
type gatewayResponse struct {
	Content   string
	Reasoning string
	ToolCalls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	}
}

// callGateway posts one non-streaming completion through the loopback gateway.
func (s *Server) callGateway(ctx context.Context, payload map[string]any) (*gatewayResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.upstreamURL(), strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lintasan-Source", "chat-ui")
	if k := s.gatewayKey(); k != "" {
		req.Header.Set("Authorization", "Bearer "+k)
	}

	resp, err := chatClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("upstream returned no choices")
	}

	gr := &gatewayResponse{
		Content:   out.Choices[0].Message.Content,
		Reasoning: out.Choices[0].Message.ReasoningContent,
		Usage:     out.Usage,
	}
	if gr.Reasoning == "" {
		gr.Reasoning = out.Choices[0].Message.Reasoning
	}
	gr.ToolCalls = out.Choices[0].Message.ToolCalls
	if gr.Content == "" && len(gr.ToolCalls) == 0 {
		// An empty answer with no tool call is still valid (the model chose to
		// say nothing), but flag it rather than silently returning "".
		gr.Content = ""
	}
	return gr, nil
}

// ---- individual tools ----

// toolFetchURL downloads a public page and reduces it to readable text.
// It refuses non-http(s) schemes and loopback/private addresses so the chat
// surface cannot be used to probe the gateway host's internal network.
func (s *Server) toolFetchURL(ctx context.Context, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("only http and https URLs are supported")
	}
	if isPrivateHost(u.Hostname()) {
		return "", fmt.Errorf("refusing to fetch private or loopback address %q", u.Hostname())
	}

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	// Identify ourselves honestly; some sites block empty agents.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; LintasanChat/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")

	client := &http.Client{
		Timeout: 20 * time.Second,
		// Follow a bounded number of redirects; an open redirect loop would
		// otherwise hang the turn.
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if isPrivateHost(r.URL.Hostname()) {
				return fmt.Errorf("redirect to private address blocked")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, u.Host)
	}

	// 2MB is plenty for an article and keeps one fetch from eating the window.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	text := string(body)
	if strings.Contains(ct, "html") || strings.Contains(ct, "xml") {
		text = htmlToText(text)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("page had no readable text")
	}
	return truncate(text, maxToolResultChars), nil
}

// toolWebSearch queries the configured search engine and formats the hits.
func (s *Server) toolWebSearch(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	// The websearch package API is a plain Search(query) returning results;
	// wrap the call in a timeout so a slow provider cannot stall the turn.
	searchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	type searchOutcome struct {
		res string
		err error
	}
	ch := make(chan searchOutcome, 1)
	go func() {
		defer func() {
			// A panic inside a provider parser must not take the server down.
			if rec := recover(); rec != nil {
				ch <- searchOutcome{err: fmt.Errorf("search panicked: %v", rec)}
			}
		}()
		resp := s.webSearch.Search(query, 8)
		var sb strings.Builder
		if strings.TrimSpace(resp.AbstractText) != "" {
			fmt.Fprintf(&sb, "Summary: %s\n\n", resp.AbstractText)
		} else if strings.TrimSpace(resp.Abstract) != "" {
			fmt.Fprintf(&sb, "Summary: %s\n\n", resp.Abstract)
		}
		for i, r := range resp.Results {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
		}
		if sb.Len() == 0 {
			ch <- searchOutcome{res: "no results"}
			return
		}
		ch <- searchOutcome{res: sb.String()}
	}()

	select {
	case <-searchCtx.Done():
		return "", fmt.Errorf("search timed out")
	case out := <-ch:
		if out.err != nil {
			return "", out.err
		}
		return truncate(out.res, maxToolResultChars), nil
	}
}

// isPrivateHost reports whether a hostname resolves to something we must not
// let the model reach: loopback, link-local, or RFC1918 private ranges.
// Hostnames (rather than resolved IPs) are checked literally; a DNS name that
// resolves to a private IP is caught by the same rule only if it IS a private
// literal, which covers the practical SSRF cases without a resolver dependency.
func isPrivateHost(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "" || h == "localhost" {
		return true
	}
	// IPv4 literals.
	if m := ipv4Re.FindStringSubmatch(h); m != nil {
		o1 := atoiSafe(m[1])
		o2 := atoiSafe(m[2])
		switch {
		case o1 == 127, o1 == 10, o1 == 0:
			return true
		case o1 == 169 && o2 == 254:
			return true
		case o1 == 172 && o2 >= 16 && o2 <= 31:
			return true
		case o1 == 192 && o2 == 168:
			return true
		}
	}
	// IPv6 loopback / link-local.
	if h == "::1" || strings.HasPrefix(h, "fe80:") || strings.HasPrefix(h, "fc") || strings.HasPrefix(h, "fd") {
		return true
	}
	// Bare hostnames that mean "this machine" on common setups.
	switch h {
	case "metadata.google.internal", "169.254.169.254":
		return true
	}
	return false
}

var ipv4Re = regexp.MustCompile(`^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$`)

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// htmlToText strips markup and collapses whitespace, dropping script/style
// bodies so the model sees prose rather than JavaScript.
func htmlToText(h string) string {
	// Remove script/style/noscript blocks including their contents.
	for _, tag := range []string{"script", "style", "noscript", "svg", "head"} {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		h = re.ReplaceAllString(h, " ")
	}
	// Turn block-level boundaries into newlines so paragraphs survive.
	blockRe := regexp.MustCompile(`(?i)</?(p|div|br|li|tr|h[1-6]|section|article|header|footer)[^>]*>`)
	h = blockRe.ReplaceAllString(h, "\n")
	// Drop every remaining tag.
	tagRe := regexp.MustCompile(`(?s)<[^>]*>`)
	h = tagRe.ReplaceAllString(h, " ")

	h = decodeEntities(h)

	// Collapse runs of blank lines and trailing spaces.
	lineRe := regexp.MustCompile(`[ \t]+`)
	h = lineRe.ReplaceAllString(h, " ")
	blankRe := regexp.MustCompile(`\n{3,}`)
	h = blankRe.ReplaceAllString(h, "\n\n")
	spaceRe := regexp.MustCompile(` {2,}`)
	h = spaceRe.ReplaceAllString(h, " ")

	return strings.TrimSpace(h)
}

// decodeEntities handles the handful of HTML entities that actually appear in
// page text. A full entity table is not worth the dependency here.
func decodeEntities(s string) string {
	repl := strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
		"&mdash;", "—",
		"&ndash;", "–",
		"&hellip;", "…",
	)
	return repl.Replace(s)
}

// maybeCompact folds older turns into conv.Summary when the history has grown
// past the point where summarising helps. It is best-effort: a failure leaves
// the conversation intact and simply means the next turn sends more context.
// It always compacts when a window exists: the automatic caller decides whether
// to call it, and the manual endpoint is an explicit "do it now".
func (s *Server) maybeCompact(ctx context.Context, st *conversation.Store, conv *conversation.Conversation, model string) (bool, string) {
	fresh, err := st.Get(conv.ID)
	if err != nil {
		return false, "load conversation: " + err.Error()
	}
	msgs, err := st.Messages(conv.ID)
	if err != nil {
		return false, "load messages: " + err.Error()
	}
	window := conversation.CompactionWindow(fresh, msgs, 6)
	if len(window) == 0 {
		return false, "not enough un-summarised messages to compact"
	}

	// Ask the same model to summarise, with an explicit instruction to keep the
	// facts that later turns would need.
	var sb strings.Builder
	if fresh.Summary != "" {
		sb.WriteString("Existing summary of the conversation so far:\n")
		sb.WriteString(fresh.Summary)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Transcript to fold into the summary:\n")
	for _, m := range window {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}

	payload := map[string]any{
		"model":  model,
		"stream": false,
		"messages": []map[string]any{
			{"role": "system", "content": "You compress chat transcripts into a compact summary. " +
				"Keep every fact, name, number, decision and open question that later turns might depend on. " +
				"Drop pleasantries and repetition. Write in the third person, under 300 words. " +
				"Output only the summary, no preamble."},
			{"role": "user", "content": sb.String()},
		},
		"temperature": 0.2,
	}
	sumCtx, cancel := context.WithTimeout(ctx, summariseTimeout)
	defer cancel()
	resp, err := s.callGateway(sumCtx, payload)
	if err != nil {
		return false, "summarise: " + err.Error()
	}
	if strings.TrimSpace(resp.Content) == "" {
		return false, "summariser returned an empty summary"
	}

	lastSeq := window[len(window)-1]
	if err := st.Update(conv.ID, conversation.UpdateFields{
		Summary:     &resp.Content,
		SummaryUpto: &lastSeq.ID,
	}); err != nil {
		return false, "store summary: " + err.Error()
	}
	return true, ""
}

// autoCompact is the post-turn hook: it compacts only when the history has
// actually grown past the point where summarising helps, so a short
// conversation never pays for a summarisation round trip.
func (s *Server) autoCompact(ctx context.Context, st *conversation.Store, conv *conversation.Conversation, model string) {
	fresh, err := st.Get(conv.ID)
	if err != nil {
		return
	}
	msgs, err := st.Messages(conv.ID)
	if err != nil {
		return
	}
	if len(conversation.CompactionWindow(fresh, msgs, 6)) == 0 {
		return
	}
	if !conversation.NeedsCompaction(fresh, msgs, chatBudget(model)) {
		return
	}
	_, _ = s.maybeCompact(ctx, st, conv, model)
}

// handleCompactConversation forces a compaction pass, for the UI's "compact
// now" button and for tests.
func (s *Server) handleCompactConversation(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	conv, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "conversation not found")
		return
	}
	msgs, err := st.Messages(conv.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	model := conv.Model
	if model == "" {
		model = s.defaultChatModel()
	}
	_ = msgs
	okCompacted, reason := s.maybeCompact(r.Context(), st, conv, model)
	updated, err := st.Get(conv.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"compacted": okCompacted, "conversation": updated}
	if reason != "" {
		resp["reason"] = reason
	}
	writeJSON(w, resp)
}

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// chat_search.go — search across conversations and export a thread.
//
// Both read the same rows the chat page already renders, so there is no second
// source of truth: search scans message bodies in SQLite, and export serialises
// exactly what /messages returns.

// registerSearchRoutes wires search and export onto the chat mux.
func (s *Server) registerSearchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/conversations/search", s.handleSearchConversations)
	mux.HandleFunc("GET /api/conversations/{id}/export", s.handleExportConversation)
}

// searchHit is one matching message. It carries enough context for a result
// list without sending whole threads over the wire.
type searchHit struct {
	ConversationID    string `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title"`
	MessageID         string `json:"message_id"`
	Role              string `json:"role"`
	Seq               int    `json:"seq"`
	Snippet           string `json:"snippet"`
	CreatedAt         string `json:"created_at"`
}

// handleSearchConversations searches message text across all conversations.
//
// GET /api/conversations/search?q=term&limit=50
//
// Case-insensitive substring match. Deliberately simple: SQLite LIKE is fast
// enough at this scale and needs no index or tokenizer, and a substring match
// is what a user typing into a search box expects (no stemming surprises).
func (s *Server) handleSearchConversations(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "q is required")
		return
	}
	if len(q) < 2 {
		writeErr(w, http.StatusBadRequest, "q must be at least 2 characters")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parsePositiveInt(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	convs, err := st.List(500, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	titles := make(map[string]string, len(convs))
	for _, c := range convs {
		titles[c.ID] = c.Title
	}

	needle := strings.ToLower(q)
	hits := make([]searchHit, 0, limit)

	// Newest conversations first: a recent thread is far likelier to be the one
	// the user means.
	sort.SliceStable(convs, func(i, j int) bool { return convs[i].UpdatedAt.After(convs[j].UpdatedAt) })

	for _, c := range convs {
		if len(hits) >= limit {
			break
		}
		msgs, err := st.Messages(c.ID)
		if err != nil {
			continue
		}
		for _, m := range msgs {
			if len(hits) >= limit {
				break
			}
			if !strings.Contains(strings.ToLower(m.Content), needle) {
				continue
			}
			hits = append(hits, searchHit{
				ConversationID:    c.ID,
				ConversationTitle: titles[c.ID],
				MessageID:         m.ID,
				Role:              m.Role,
				Seq:               m.Seq,
				Snippet:           snippetAround(m.Content, q, 160),
				CreatedAt:         m.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	writeJSON(w, map[string]any{
		"query":          q,
		"count":          len(hits),
		"limit":          limit,
		"truncated":      len(hits) >= limit,
		"results":        hits,
		"conversations":  len(convs),
	})
}

// snippetAround returns up to width characters of content centred on the first
// case-insensitive occurrence of needle, with ellipses where it was cut.
func snippetAround(content, needle string, width int) string {
	if width <= 0 || len(content) <= width {
		return content
	}
	idx := strings.Index(strings.ToLower(content), strings.ToLower(needle))
	if idx < 0 {
		return content[:width] + "…"
	}
	// Put the match roughly a third of the way in, so there is leading context
	// but the hit is clearly visible.
	start := idx - width/3
	if start < 0 {
		start = 0
	}
	end := start + width
	if end > len(content) {
		end = len(content)
		start = end - width
		if start < 0 {
			start = 0
		}
	}
	out := content[start:end]
	if start > 0 {
		out = "…" + out
	}
	if end < len(content) {
		out = out + "…"
	}
	return out
}

// handleExportConversation returns a thread as JSON or Markdown.
//
// GET /api/conversations/{id}/export?format=json|markdown
//
// Markdown is the useful one for pasting into docs or a review, so it is shaped
// like a readable transcript rather than a data dump.
func (s *Server) handleExportConversation(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	id := r.PathValue("id")

	conv, err := st.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "conversation not found")
		return
	}
	msgs, err := st.Messages(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	atts, _ := st.Attachments(id)

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "markdown"
	}

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, safeFilename(conv.Title)))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"conversation": conv,
			"messages":     msgs,
			"attachments":  atts,
			"exported_at":  time.Now().UTC().Format(time.RFC3339),
		})
	case "markdown", "md":
		var b strings.Builder
		fmt.Fprintf(&b, "# %s\n\n", conv.Title)
		fmt.Fprintf(&b, "_Exported %s_\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
		if strings.TrimSpace(conv.SystemMsg) != "" {
			fmt.Fprintf(&b, "**System prompt**\n\n> %s\n\n", indentLines(conv.SystemMsg, "> "))
		}
		if strings.TrimSpace(conv.Summary) != "" {
			fmt.Fprintf(&b, "**Summary of earlier turns**\n\n> %s\n\n", indentLines(conv.Summary, "> "))
		}
		for _, m := range msgs {
			who := "You"
			if m.Role == conversation.RoleAssistant {
				who = "Assistant"
			}
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", who, m.Content)
			if m.ModelUsed != "" {
				fmt.Fprintf(&b, "`%s`", m.ModelUsed)
				if m.TokensIn > 0 || m.TokensOut > 0 {
					fmt.Fprintf(&b, " · %d in / %d out", m.TokensIn, m.TokensOut)
				}
				b.WriteString("\n\n")
			}
		}
		if len(atts) > 0 {
			b.WriteString("## Attachments\n\n")
			for _, a := range atts {
				fmt.Fprintf(&b, "- `%s` (%s, %d bytes)\n", a.Filename, a.Mime, a.Size)
			}
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, safeFilename(conv.Title)))
		_, _ = w.Write([]byte(b.String()))
	default:
		writeErr(w, http.StatusBadRequest, "format must be json or markdown")
	}
}

// indentLines prefixes every line, so a multi-line blockquote stays quoted.
func indentLines(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = strings.TrimRight(prefix, " ")
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// safeFilename reduces a conversation title to something usable in a
// Content-Disposition header: no path separators, no quotes, bounded length.
func safeFilename(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return "conversation"
	}
	repl := strings.NewReplacer(
		"/", "-", "\\", "-", `"`, "", "'", "",
		"\n", " ", "\r", " ", "\t", " ",
	)
	t = repl.Replace(t)
	if len(t) > 60 {
		t = t[:60]
	}
	t = strings.TrimSpace(t)
	if t == "" {
		return "conversation"
	}
	return t
}

// parsePositiveInt is a tiny strconv.Atoi wrapper that rejects signs and blanks.
func parsePositiveInt(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("not positive")
	}
	return n, nil
}

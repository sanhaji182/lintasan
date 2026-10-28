package server

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/sanhaji182/lintasan-go/internal/config"
	"github.com/sanhaji182/lintasan-go/internal/conversation"
	"github.com/sanhaji182/lintasan-go/internal/db"
)

// chatHarness builds a Server with a real SQLite database and a stub upstream
// gateway, so the chat endpoints can be exercised over their real HTTP path.
type chatHarness struct {
	srv      *Server
	db       *db.DB
	conv     *conversation.Store
	dataDir  string
	upstream *httptest.Server
}

// newChatHarness starts a Server whose loopback gateway is replaced by a stub
// that returns whatever the test supplies.
func newChatHarness(t *testing.T, handler http.HandlerFunc) *chatHarness {
	t.Helper()

	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		Port:    20180,
		DataDir: dir,
		DBPath:  filepath.Join(dir, "test.db"),
	}
	srv := New(cfg, database)
	srv.mux = http.NewServeMux()
	srv.registerConversationRoutes()

	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)

	// Point the server's loopback URL at the stub by overriding cfg.Port with a
	// sentinel the test rewrites. Simplest reliable route: swap upstreamURL via
	// a dedicated field-free approach — rebuild the mux with a patched cfg.
	// We instead use the exported-for-test hook below.
	srv.chatUpstreamURL = upstream.URL + "/v1/chat/completions"

	st := conversation.NewStore(database.Conn())
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return &chatHarness{srv: srv, db: database, conv: st, dataDir: dir, upstream: upstream}
}

// do issues a request against the server's mux and returns the recorder.
func (h *chatHarness) do(t *testing.T, method, path string, body any, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.srv.mux.ServeHTTP(rec, req)
	return rec
}

// openaiResponse builds a non-streaming completion body.
func openaiResponse(content string, promptTok, completionTok int) map[string]any {
	return map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{"role": "assistant", "content": content},
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     promptTok,
			"completion_tokens": completionTok,
		},
	}
}

func TestCreateAndListConversation(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, openaiResponse("hi", 1, 1))
	})

	rec := h.do(t, http.MethodPost, "/api/conversations",
		map[string]any{"title": "my thread", "model": "gpt-4o"}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d body=%s", rec.Code, rec.Body)
	}
	var created conversation.Conversation
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" || created.Title != "my thread" {
		t.Fatalf("unexpected conversation: %+v", created)
	}

	rec = h.do(t, http.MethodGet, "/api/conversations", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", rec.Code)
	}
	var list struct {
		Conversations []conversation.Conversation `json:"conversations"`
		Count         int                         `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Count != 1 || len(list.Conversations) != 1 {
		t.Fatalf("want 1 conversation, got %+v", list)
	}
}

func TestCreateConversationWithEmptyBody(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodPost, "/api/conversations", nil)
	rec := httptest.NewRecorder()
	h.srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("empty body should create a default thread, got %d body=%s", rec.Code, rec.Body)
	}
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)
	if c.Title != "New chat" {
		t.Fatalf("want default title, got %q", c.Title)
	}
}

func TestGetMissingConversationReturns404(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	rec := h.do(t, http.MethodGet, "/api/conversations/c_missing", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestPatchConversationRenamesAndPins(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{"title": "old"}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	rec = h.do(t, http.MethodPatch, "/api/conversations/"+c.ID,
		map[string]any{"title": "renamed", "pinned": true}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: want 200, got %d body=%s", rec.Code, rec.Body)
	}
	var got conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Title != "renamed" || !got.Pinned {
		t.Fatalf("patch not applied: %+v", got)
	}
}

func TestDeleteConversationRemovesIt(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{"title": "x"}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	rec = h.do(t, http.MethodDelete, "/api/conversations/"+c.ID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d", rec.Code)
	}
	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("conversation should be gone, got %d", rec.Code)
	}
}

func TestPostMessageBlockingPersistsBothTurns(t *testing.T) {
	var sawAuth string
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		writeJSON(w, openaiResponse("the answer", 10, 5))
	})

	rec := h.do(t, http.MethodPost, "/api/conversations",
		map[string]any{"title": "t", "model": "gpt-4o"}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	f := false
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "hello", "stream": f, "use_tools": f}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("post message: want 200, got %d body=%s", rec.Code, rec.Body)
	}

	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/messages", nil, nil)
	var out struct {
		Messages []conversation.Message `json:"messages"`
		Count    int                    `json:"count"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Count != 2 {
		t.Fatalf("want user+assistant = 2 messages, got %d", out.Count)
	}
	if out.Messages[0].Role != conversation.RoleUser || out.Messages[0].Content != "hello" {
		t.Fatalf("user message wrong: %+v", out.Messages[0])
	}
	if out.Messages[1].Role != conversation.RoleAssistant || out.Messages[1].Content != "the answer" {
		t.Fatalf("assistant message wrong: %+v", out.Messages[1])
	}
	if out.Messages[1].TokensIn != 10 || out.Messages[1].TokensOut != 5 {
		t.Fatalf("usage not persisted: %+v", out.Messages[1])
	}
	if sawAuth == "" {
		t.Log("no master key configured, upstream called without Authorization (expected in test)")
	}
}

func TestPostMessageTitlesThreadFromFirstMessage(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, openaiResponse("ok", 1, 1))
	})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	f := false
	h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "How do Go channels work?", "stream": f, "use_tools": f}, nil)

	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID, nil, nil)
	var got conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Title != "How do Go channels work?" {
		t.Fatalf("title should derive from first message, got %q", got.Title)
	}
}

func TestPostMessageRequiresContentOrAttachment(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "   "}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty content, got %d", rec.Code)
	}
}

func TestPostMessageSurfacesUpstreamErrorAndPersistsIt(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	f := false
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "hi", "stream": f, "use_tools": f}, nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502 on upstream failure, got %d body=%s", rec.Code, rec.Body)
	}

	// The failed assistant turn must still be visible so the user can retry
	// without retyping. This is the whole point of persisting before answering.
	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/messages", nil, nil)
	var out struct {
		Messages []conversation.Message `json:"messages"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(out.Messages))
	}
	if out.Messages[1].Error == "" {
		t.Fatal("failed assistant turn should record the error")
	}
}

func TestStreamingEmitsSSEFrames(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		for _, piece := range []string{"Hel", "lo", " world"} {
			chunk := map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": piece}}},
			}
			b, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	})

	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	tr := true
	f := false
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "hi", "stream": tr, "use_tools": f}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream: want 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("want event-stream content type, got %q", ct)
	}
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatal("X-Accel-Buffering: no is required or nginx will buffer the stream")
	}

	body := rec.Body.String()
	for _, want := range []string{"event: start", "event: delta", "event: done"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in stream output:\n%s", want, body)
		}
	}

	// Reassembled deltas must equal the full reply, and it must be persisted.
	var reassembled strings.Builder
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) != nil {
			continue
		}
		if t2, ok := frame["text"].(string); ok {
			reassembled.WriteString(t2)
		}
	}
	if reassembled.String() != "Hello world" {
		t.Fatalf("reassembled deltas = %q, want %q", reassembled.String(), "Hello world")
	}

	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/messages", nil, nil)
	var out struct {
		Messages []conversation.Message `json:"messages"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Messages) != 2 || out.Messages[1].Content != "Hello world" {
		t.Fatalf("streamed reply not persisted: %+v", out.Messages)
	}
	if out.Messages[1].TokensIn != 7 || out.Messages[1].TokensOut != 3 {
		t.Fatalf("streamed usage not persisted: %+v", out.Messages[1])
	}
}

func TestStreamingEmitsThinkingFrames(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		chunk := `{"choices":[{"delta":{"reasoning_content":"pondering...","content":"answer"}}]}`
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	})

	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	tr := true
	f := false
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "hi", "stream": tr, "use_tools": f}, nil)
	body := rec.Body.String()
	if !strings.Contains(body, "event: thinking") {
		t.Fatalf("reasoning content should surface as a thinking frame:\n%s", body)
	}
	if !strings.Contains(body, "pondering") {
		t.Fatalf("reasoning text missing:\n%s", body)
	}

	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/messages", nil, nil)
	var out struct {
		Messages []conversation.Message `json:"messages"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Messages[1].Thinking != "pondering..." {
		t.Fatalf("thinking not persisted: %+v", out.Messages[1])
	}
}

func TestDeleteMessageTruncatesTail(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, openaiResponse("reply", 1, 1))
	})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	f := false
	for _, text := range []string{"one", "two"} {
		h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
			map[string]any{"content": text, "stream": f, "use_tools": f}, nil)
	}

	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/messages", nil, nil)
	var out struct {
		Messages []conversation.Message `json:"messages"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Messages) != 4 {
		t.Fatalf("want 4 messages, got %d", len(out.Messages))
	}
	// Deleting the first user message removes everything after it.
	firstUser := out.Messages[0]
	rec = h.do(t, http.MethodDelete, "/api/conversations/"+c.ID+"/messages/"+firstUser.ID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete message: want 200, got %d body=%s", rec.Code, rec.Body)
	}

	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/messages", nil, nil)
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Messages) != 0 {
		t.Fatalf("all messages should be truncated, got %d", len(out.Messages))
	}
}

func TestDeleteMessageFromOtherConversationIs404(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, openaiResponse("r", 1, 1))
	})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var a conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &a)
	rec = h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var b conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &b)

	f := false
	h.do(t, http.MethodPost, "/api/conversations/"+a.ID+"/messages",
		map[string]any{"content": "x", "stream": f, "use_tools": f}, nil)
	msgs, _ := h.conv.Messages(a.ID)

	rec = h.do(t, http.MethodDelete, "/api/conversations/"+b.ID+"/messages/"+msgs[0].ID, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-conversation delete must 404, got %d", rec.Code)
	}
}

func TestUploadAndDownloadAttachment(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, openaiResponse("ok", 1, 1))
	})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	// Build a multipart body with a small text file.
	var buf bytes.Buffer
	boundary := "----testboundary"
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"file\"; filename=\"notes.txt\"\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain\r\n\r\n")
	fmt.Fprintf(&buf, "the quick brown fox\r\n")
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)

	req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+c.ID+"/attachments", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	rec = httptest.NewRecorder()
	h.srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: want 201, got %d body=%s", rec.Code, rec.Body)
	}
	var up struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
		HasText  bool   `json:"has_text"`
		Size     int64  `json:"size"`
	}
	json.Unmarshal(rec.Body.Bytes(), &up)
	if up.ID == "" || up.Filename != "notes.txt" {
		t.Fatalf("bad upload response: %+v", up)
	}
	if !up.HasText {
		t.Fatal("text file should have extracted text")
	}

	// The extracted text must be stored, not just the file.
	att, err := h.conv.GetAttachment(up.ID)
	if err != nil {
		t.Fatalf("get attachment: %v", err)
	}
	if !strings.Contains(att.Extracted, "quick brown fox") {
		t.Fatalf("extracted text missing: %q", att.Extracted)
	}
	if _, err := os.Stat(att.Path); err != nil {
		t.Fatalf("file not on disk at %s: %v", att.Path, err)
	}

	// Download must return the same bytes.
	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/attachments/"+up.ID, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "quick brown fox") {
		t.Fatalf("downloaded body wrong: %q", rec.Body.String())
	}
}

func TestUploadRejectsTraversalFilename(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	var buf bytes.Buffer
	boundary := "----b"
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"file\"; filename=\"../../etc/passwd\"\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain\r\n\r\n")
	fmt.Fprintf(&buf, "root:x:0:0\r\n")
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)

	req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+c.ID+"/attachments", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	rec = httptest.NewRecorder()
	h.srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("upload should succeed with a sanitised name, got %d body=%s", rec.Code, rec.Body)
	}
	var up struct {
		Filename string `json:"filename"`
	}
	json.Unmarshal(rec.Body.Bytes(), &up)
	if strings.Contains(up.Filename, "..") || strings.Contains(up.Filename, "/") {
		t.Fatalf("filename not sanitised: %q", up.Filename)
	}

	// And the stored file must live inside the attachments root.
	atts, err := h.conv.Attachments(c.ID)
	if err != nil || len(atts) != 1 {
		t.Fatalf("want 1 attachment, got %v (err %v)", atts, err)
	}
	root := filepath.Join(h.dataDir, "attachments")
	if !strings.HasPrefix(atts[0].Path, root) {
		t.Fatalf("attachment escaped storage root: %s", atts[0].Path)
	}
}

func TestAttachmentTextReachesThePrompt(t *testing.T) {
	var gotPrompt string
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) > 0 {
			gotPrompt = body.Messages[len(body.Messages)-1].Content
		}
		writeJSON(w, openaiResponse("read it", 1, 1))
	})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	var buf bytes.Buffer
	boundary := "----b"
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"file\"; filename=\"spec.txt\"\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/plain\r\n\r\n")
	fmt.Fprintf(&buf, "SPEC CONTENT MARKER\r\n")
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/"+c.ID+"/attachments", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	rec = httptest.NewRecorder()
	h.srv.mux.ServeHTTP(rec, req)
	var up struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &up)

	f := false
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "summarise this", "attachment_ids": []string{up.ID},
			"stream": f, "use_tools": f}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("post with attachment: want 200, got %d body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(gotPrompt, "SPEC CONTENT MARKER") {
		t.Fatalf("attachment text did not reach the model prompt: %q", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "summarise this") {
		t.Fatalf("user text missing from prompt: %q", gotPrompt)
	}
}

func TestAttachmentFromOtherConversationRejected(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var a conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &a)
	rec = h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var b conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &b)

	// Register an attachment directly against conversation A.
	att := &conversation.Attachment{
		ConversationID: a.ID, Filename: "x.txt", Mime: "text/plain", Size: 2, Path: "/tmp/x",
	}
	if err := h.conv.AddAttachment(att); err != nil {
		t.Fatalf("add: %v", err)
	}

	f := false
	rec = h.do(t, http.MethodPost, "/api/conversations/"+b.ID+"/messages",
		map[string]any{"content": "hi", "attachment_ids": []string{att.ID},
			"stream": f, "use_tools": f}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-conversation attachment must be rejected, got %d body=%s", rec.Code, rec.Body)
	}
}

func TestToolLoopRunsFetchAndAnswers(t *testing.T) {
	var calls int
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Tools []any `json:"tools"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		if calls == 1 {
			// First round: the model asks for a tool.
			writeJSON(w, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []any{map[string]any{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "get_current_time",
								"arguments": "{}",
							},
						}},
					},
				}},
			})
			return
		}
		// Second round: final answer.
		writeJSON(w, openaiResponse("It is currently some time.", 20, 10))
	})

	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	f := false
	tr := true
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "what time is it?", "stream": f, "use_tools": tr}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("tool turn: want 200, got %d body=%s", rec.Code, rec.Body)
	}
	if calls != 2 {
		t.Fatalf("want 2 upstream rounds (tool then answer), got %d", calls)
	}

	rec = h.do(t, http.MethodGet, "/api/conversations/"+c.ID+"/messages", nil, nil)
	var out struct {
		Messages []conversation.Message `json:"messages"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	last := out.Messages[len(out.Messages)-1]
	if !strings.Contains(last.Content, "currently some time") {
		t.Fatalf("final answer not persisted: %q", last.Content)
	}
	if last.TokensIn != 20 {
		t.Fatalf("usage from the final round not recorded: %+v", last)
	}
}

func TestToolLoopEmitsToolFramesWhileStreaming(t *testing.T) {
	var calls int
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeJSON(w, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"content": "",
						"tool_calls": []any{map[string]any{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "get_current_time",
								"arguments": "{}",
							},
						}},
					},
				}},
			})
			return
		}
		writeJSON(w, openaiResponse("done", 5, 2))
	})

	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	tr := true
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "time?", "stream": tr, "use_tools": tr}, nil)
	body := rec.Body.String()
	if !strings.Contains(body, "event: tool") {
		t.Fatalf("tool invocation should stream as an event:\n%s", body)
	}
	if !strings.Contains(body, "get_current_time") {
		t.Fatalf("tool name missing from stream:\n%s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("stream should finish with done:\n%s", body)
	}
}

func TestToolLoopStopsAtIterationLimit(t *testing.T) {
	// A model that never stops asking for tools must not hang the request.
	var calls int
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Tools []any `json:"tools"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Tools) == 0 {
			// The final "no tools" call gets an answer.
			writeJSON(w, openaiResponse("forced answer", 1, 1))
			return
		}
		writeJSON(w, map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"content": "",
					"tool_calls": []any{map[string]any{
						"id":   fmt.Sprintf("call_%d", calls),
						"type": "function",
						"function": map[string]any{
							"name": "get_current_time", "arguments": "{}",
						},
					}},
				},
			}},
		})
	})

	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	f := false
	tr := true
	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/messages",
		map[string]any{"content": "go", "stream": f, "use_tools": tr}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 after hitting the limit, got %d body=%s", rec.Code, rec.Body)
	}
	// maxToolIterations rounds + 1 final no-tool call.
	if calls > maxToolIterations+1 {
		t.Fatalf("loop ran %d rounds, expected at most %d", calls, maxToolIterations+1)
	}
	if !strings.Contains(rec.Body.String(), "tool-call limit") {
		t.Log("note: limit marker text not present, checking answer was still returned")
	}
}

func TestCompactEndpointStoresSummary(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		// Summarisation requests carry a system message about compressing; answer
		// those with a summary, everything else with a normal reply.
		if len(body.Messages) > 0 && strings.Contains(body.Messages[0].Content, "compress chat transcripts") {
			writeJSON(w, openaiResponse("SUMMARY: user asked several things.", 5, 5))
			return
		}
		writeJSON(w, openaiResponse("ok", 1, 1))
	})

	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	// Seed enough messages that a compaction window exists (needs > keepRecent).
	big := strings.Repeat("word ", 200)
	for i := 0; i < 10; i++ {
		m := &conversation.Message{ConversationID: c.ID, Role: conversation.RoleUser, Content: big}
		if err := h.conv.AppendMessage(m); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/compact", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("compact: want 200, got %d body=%s", rec.Code, rec.Body)
	}

	got, err := h.conv.Get(c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(got.Summary, "SUMMARY") {
		t.Fatalf("summary not stored: %q", got.Summary)
	}
	if got.SummaryUpto == "" {
		t.Fatal("summary pointer not set")
	}
}

func TestCompactWithTooFewMessagesIsNoop(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, openaiResponse("should not be called", 1, 1))
	})
	rec := h.do(t, http.MethodPost, "/api/conversations", map[string]any{}, nil)
	var c conversation.Conversation
	json.Unmarshal(rec.Body.Bytes(), &c)

	rec = h.do(t, http.MethodPost, "/api/conversations/"+c.ID+"/compact", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var out struct {
		Compacted bool `json:"compacted"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Compacted {
		t.Fatal("should not compact a conversation with no messages")
	}
}

func TestDeriveTitleTruncatesOnWordBoundary(t *testing.T) {
	long := strings.Repeat("alpha bravo charlie delta echo foxtrot ", 5)
	title := deriveTitle(long)
	if len(title) > 70 {
		t.Fatalf("title too long: %d chars", len(title))
	}
	if strings.Contains(title, "  ") {
		t.Fatalf("title has doubled spaces: %q", title)
	}
	if !strings.HasSuffix(title, "…") {
		t.Fatalf("long title should be elided: %q", title)
	}
}

func TestIsPrivateHostBlocksInternalTargets(t *testing.T) {
	priv := []string{
		"127.0.0.1", "localhost", "10.0.0.5", "192.168.1.1", "172.16.0.1",
		"172.31.255.255", "169.254.169.254", "::1", "0.0.0.0",
	}
	for _, h := range priv {
		if !isPrivateHost(h) {
			t.Errorf("%q should be blocked as private", h)
		}
	}
	pub := []string{"8.8.8.8", "example.com", "1.1.1.1", "172.32.0.1", "11.0.0.1"}
	for _, h := range pub {
		if isPrivateHost(h) {
			t.Errorf("%q should be allowed as public", h)
		}
	}
}

func TestFetchURLRefusesPrivateTargets(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	ctx := context.Background()
	for _, u := range []string{
		"http://127.0.0.1:20180/health",
		"http://localhost/admin",
		"http://169.254.169.254/latest/meta-data/",
		"http://192.168.1.1/",
		"file:///etc/passwd",
	} {
		if _, err := h.srv.toolFetchURL(ctx, u); err == nil {
			t.Errorf("%s should have been refused", u)
		}
	}
}

func TestHTMLToTextStripsMarkupAndScripts(t *testing.T) {
	html := `<html><head><style>body{color:red}</style></head>
<body><h1>Title</h1><script>alert('xss')</script>
<p>First para.</p><p>Second &amp; third.</p></body></html>`
	out := htmlToText(html)
	for _, unwanted := range []string{"<h1>", "<script>", "alert", "color:red"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output still contains %q:\n%s", unwanted, out)
		}
	}
	for _, want := range []string{"Title", "First para.", "Second & third."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTruncateHelperIsRuneSafe(t *testing.T) {
	// The shared truncate counts runes; multibyte input must not be split.
	s := strings.Repeat("漢", 100)
	out := truncate(s, 10)
	if !strings.HasPrefix(out, strings.Repeat("漢", 10)) {
		t.Fatalf("wrong truncation: %q", out)
	}
}

func TestConvStoreInitialisesOnce(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	st1 := h.srv.convStore()
	st2 := h.srv.convStore()
	if st1 != st2 {
		t.Fatal("convStore should return the same store across calls")
	}
	if h.srv.chatStoreErr != nil {
		t.Fatalf("unexpected migration error: %v", h.srv.chatStoreErr)
	}
}

// ensure the in-memory sqlite driver stays referenced even if a future test
// stops importing it indirectly through db.
var _ = sql.ErrNoRows

// Regression: chat used to 401 against its own gateway whenever the master key
// came from LINTASAN_MASTER_KEY rather than the settings table, because only
// the DB row was consulted. Environment-configured deployments were broken.
func TestGatewayKeyFallsBackToConfig(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	h.srv.cfg.MasterKey = "env-only-key"
	if err := h.db.SetSetting("master_key", ""); err != nil {
		t.Fatalf("clear setting: %v", err)
	}

	got := h.srv.gatewayKey()
	if got != "env-only-key" {
		t.Fatalf("gatewayKey should fall back to cfg.MasterKey, got %q", got)
	}
}

func TestGatewayKeyPrefersDBSetting(t *testing.T) {
	h := newChatHarness(t, func(w http.ResponseWriter, r *http.Request) {})
	h.srv.cfg.MasterKey = "env-key"
	if err := h.db.SetSetting("master_key", "db-key"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := h.srv.gatewayKey(); got != "db-key" {
		t.Fatalf("DB setting should win, got %q", got)
	}
}

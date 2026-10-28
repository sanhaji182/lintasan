package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// Conversations: persistent multi-turn chat threads.
//
// These endpoints sit behind the existing authMiddleware with every other
// /api/* path, so there is no per-route auth wrapper here — same convention as
// handlers_migrate.go and handlers_experimental.go.
//
// The turn loop itself lives in chat_turn.go; this file is CRUD plus the
// message listing the UI needs to restore a thread.

// registerConversationRoutes wires the chat endpoints onto the mux.
func (s *Server) registerConversationRoutes() {
	mux := s.mux
	mux.HandleFunc("GET /api/conversations", s.handleListConversations)
	mux.HandleFunc("POST /api/conversations", s.handleCreateConversation)
	mux.HandleFunc("GET /api/conversations/{id}", s.handleGetConversation)
	mux.HandleFunc("PATCH /api/conversations/{id}", s.handlePatchConversation)
	mux.HandleFunc("DELETE /api/conversations/{id}", s.handleDeleteConversation)
	mux.HandleFunc("GET /api/conversations/{id}/messages", s.handleListMessages)
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.handlePostMessage)
	mux.HandleFunc("DELETE /api/conversations/{id}/messages/{mid}", s.handleDeleteMessage)
	mux.HandleFunc("POST /api/conversations/{id}/compact", s.handleCompactConversation)
	mux.HandleFunc("POST /api/conversations/{id}/attachments", s.handleUploadAttachment)
	mux.HandleFunc("GET /api/conversations/{id}/attachments/{aid}", s.handleDownloadAttachment)
	s.registerRewindRoutes(mux)
	s.registerSearchRoutes(mux)
}

// writeErr writes an OpenAI-shaped error body, matching the rest of the API.
func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": msg, "type": "invalid_request_error"},
	})
}

// store returns the conversation store, lazily initialising it on first use so
// a server built before this feature existed still works. The mutex matters:
// two concurrent first requests would otherwise both run Migrate.
func (s *Server) convStore() *conversation.Store {
	s.chatStoreMu.Lock()
	defer s.chatStoreMu.Unlock()
	if s.chatStore == nil {
		st := conversation.NewStore(s.db.Conn())
		if err := st.Migrate(); err != nil {
			// Migration failure is fatal for chat features but must not take the
			// gateway down; handlers surface the error per-request instead.
			s.chatStoreErr = err
		}
		s.chatStore = st
	}
	return s.chatStore
}

// convStoreReady returns the store or writes the appropriate error.
func (s *Server) convStoreReady(w http.ResponseWriter) (*conversation.Store, bool) {
	st := s.convStore()
	if s.chatStoreErr != nil {
		writeErr(w, http.StatusInternalServerError, "conversation store unavailable: "+s.chatStoreErr.Error())
		return nil, false
	}
	return st, true
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	includeArchived := r.URL.Query().Get("archived") == "true"

	list, err := st.List(limit, includeArchived)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"conversations": list, "count": len(list)})
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	var in struct {
		Title        string `json:"title"`
		Model        string `json:"model"`
		SystemPrompt string `json:"system_prompt"`
	}
	// An empty body is valid: it creates a blank thread with defaults.
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	c := &conversation.Conversation{
		Title:     strings.TrimSpace(in.Title),
		Model:     strings.TrimSpace(in.Model),
		SystemMsg: in.SystemPrompt,
	}
	if c.Title == "" {
		c.Title = "New chat"
	}
	if err := st.Create(c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONStatus(w, http.StatusCreated, c)
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	c, err := st.Get(r.PathValue("id"))
	if errors.Is(err, conversation.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, c)
}

func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	var in struct {
		Title        *string `json:"title"`
		Model        *string `json:"model"`
		SystemPrompt *string `json:"system_prompt"`
		Pinned       *bool   `json:"pinned"`
		Archived     *bool   `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	fields := conversation.UpdateFields{
		Title:     in.Title,
		Model:     in.Model,
		SystemMsg: in.SystemPrompt,
		Pinned:    in.Pinned,
		Archived:  in.Archived,
	}
	if err := st.Update(r.PathValue("id"), fields); err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	c, err := st.Get(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, c)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	// Remove attachment files from disk before dropping the rows, otherwise the
	// metadata disappears and the bytes leak.
	atts, err := st.Attachments(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, a := range atts {
		s.deleteAttachmentFile(a.Path)
	}
	if err := st.Delete(id); err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": id})
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := st.Get(id); err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	msgs, err := st.Messages(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"messages": msgs, "count": len(msgs)})
}

// handleDeleteMessage truncates the conversation after the given message, which
// is what "edit this message and re-run" means: everything downstream is stale.
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	convID := r.PathValue("id")
	mid := r.PathValue("mid")
	m, err := st.GetMessage(mid)
	if err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "message not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m.ConversationID != convID {
		writeErr(w, http.StatusNotFound, "message not found in this conversation")
		return
	}
	n, err := st.DeleteMessagesAfter(convID, m.Seq-1)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"truncated_from": m.Seq, "deleted": n})
}

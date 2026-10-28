package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// chat_rewind.go — regenerate and edit-and-resend.
//
// Both are the same operation underneath: drop every message after some point,
// then run a turn from what is left. DeleteMessagesAfter already existed for
// the delete handler; this exposes it as the two actions users actually ask
// for. Splitting them keeps the truncation rule in one place and means the
// history stays a clean linear transcript — we do not fork threads.
//
// Regenerate: keep the user turn, drop its assistant reply, ask again.
// Edit:       replace the user turn's text, drop everything after it, ask again.

// registerRewindRoutes wires these onto the same mux as the other chat routes.
func (s *Server) registerRewindRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/conversations/{id}/messages/{mid}/regenerate", s.handleRegenerate)
	mux.HandleFunc("POST /api/conversations/{id}/messages/{mid}/edit", s.handleEditMessage)
}

// handleRegenerate re-runs the assistant reply to an existing user message.
//
// The given message must be an assistant message: regenerating a user message
// would mean inventing a prompt, and the UI only ever offers this on replies.
// Everything from that reply onward is discarded first, so the retried turn
// does not see the answer it is replacing (which would bias it toward the same
// answer).
func (s *Server) handleRegenerate(w http.ResponseWriter, r *http.Request) {
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
	if m.Role != conversation.RoleAssistant {
		writeErr(w, http.StatusBadRequest, "regenerate applies to an assistant message; use edit on a user message")
		return
	}

	// The prompt we are answering again is the user message just before this
	// reply.
	prompt, err := s.userMessageBefore(st, convID, m.Seq)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Drop the old reply and anything after it.
	if _, err := st.DeleteMessagesAfter(convID, m.Seq-1); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var in postMessageRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	in.Content = prompt.Content
	// The prompt is already the last message on disk; do not append it again.
	in.ReuseUserMessage = true

	s.resendTurn(w, r, convID, prompt, in)
}

// handleEditMessage rewrites a user message's text and re-runs from there.
func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) {
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
	if m.Role != conversation.RoleUser {
		writeErr(w, http.StatusBadRequest, "edit applies to a user message")
		return
	}

	var in postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	newText := strings.TrimSpace(in.Content)
	if newText == "" {
		writeErr(w, http.StatusBadRequest, "content must not be empty")
		return
	}

	// Drop this message and everything after it, then post the edited text as a
	// fresh turn. Rewriting in place would leave the old reply's tokens in the
	// transcript and the seq numbers would stop being monotonic.
	if _, err := st.DeleteMessagesAfter(convID, m.Seq-1); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Keep the original attachments so editing the wording does not silently
	// detach the image or file the user attached.
	if len(in.AttachmentIDs) == 0 && strings.TrimSpace(m.Attachments) != "" {
		in.AttachmentIDs = attachmentIDsFromMeta(m.Attachments)
	}
	in.Content = newText

	s.resendTurn(w, r, convID, nil, in)
}

// userMessageBefore returns the closest user message at or before seq.
func (s *Server) userMessageBefore(st *conversation.Store, convID string, seq int) (*conversation.Message, error) {
	msgs, err := st.Messages(convID)
	if err != nil {
		return nil, err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Seq <= seq && msgs[i].Role == conversation.RoleUser {
			return msgs[i], nil
		}
	}
	return nil, errors.New("no user message to re-run")
}

// lastUserMessage returns the most recent user message in the conversation.
func (s *Server) lastUserMessage(st *conversation.Store, convID string) (*conversation.Message, error) {
	msgs, err := st.Messages(convID)
	if err != nil {
		return nil, err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == conversation.RoleUser {
			return msgs[i], nil
		}
	}
	return nil, errors.New("conversation has no user message to re-run")
}

// attachmentPayloadFor rebuilds the inline text and image parts for a message
// that is already stored, from the ids in its metadata blob. Used by
// regenerate, which replays an existing turn rather than a freshly uploaded
// one.
func (s *Server) attachmentPayloadFor(st *conversation.Store, m *conversation.Message) (string, []map[string]any) {
	ids := attachmentIDsFromMeta(m.Attachments)
	if len(ids) == 0 {
		return "", nil
	}
	_, text, images, err := s.loadAttachments(st, m.ConversationID, ids)
	if err != nil {
		return "", nil
	}
	return text, images
}

// attachmentIDsFromMeta pulls the ids back out of the JSON metadata blob stored
// on a message, so an edited turn keeps its files.
func attachmentIDsFromMeta(blob string) []string {
	var metas []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(blob), &metas); err != nil {
		return nil
	}
	ids := make([]string, 0, len(metas))
	for _, m := range metas {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// resendTurn runs the normal turn handler with a synthetic request, so
// regenerate/edit share the exact streaming, tool-loop, attachment and
// persistence behaviour of a fresh message instead of reimplementing it.
//
// The request is rebuilt rather than mutated so the caller's *http.Request is
// not repurposed mid-flight (its body and context belong to the original
// handler).
func (s *Server) resendTurn(w http.ResponseWriter, r *http.Request, convID string, _ *conversation.Message, in postMessageRequest) {
	body, err := json.Marshal(in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.servePostMessage(w, r, convID, body)
}

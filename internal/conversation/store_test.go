package conversation

import (
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// openTestStore returns a Store backed by an in-memory SQLite database that is
// destroyed when the test ends.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	// :memory: is per-connection; keep the pool at 1 so every query hits the
	// same in-memory database.
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })

	s := NewStore(conn)
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := openTestStore(t)
	if err := s.Migrate(); err != nil {
		t.Fatalf("second migrate should be a no-op, got: %v", err)
	}
}

func TestCreateAndGetConversation(t *testing.T) {
	s := openTestStore(t)

	c := &Conversation{Title: "first chat", Model: "gpt-4o", SystemMsg: "be terse"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ID == "" {
		t.Fatal("create should assign an id")
	}

	got, err := s.Get(c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "first chat" || got.Model != "gpt-4o" || got.SystemMsg != "be terse" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.MsgCount != 0 {
		t.Fatalf("new conversation should have 0 messages, got %d", got.MsgCount)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not parsed: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestGetMissingConversation(t *testing.T) {
	s := openTestStore(t)
	_, err := s.Get("c_doesnotexist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAppendMessageAssignsIncreasingSeq(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}

	for i, role := range []string{RoleUser, RoleAssistant, RoleUser} {
		m := &Message{ConversationID: c.ID, Role: role, Content: "hi"}
		if err := s.AppendMessage(m); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if m.Seq != i+1 {
			t.Fatalf("append %d: want seq %d, got %d", i, i+1, m.Seq)
		}
		if m.ID == "" {
			t.Fatalf("append %d: id not assigned", i)
		}
	}

	msgs, err := s.Messages(c.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Seq <= msgs[i-1].Seq {
			t.Fatalf("seq not ascending: %d then %d", msgs[i-1].Seq, msgs[i].Seq)
		}
	}
}

func TestAppendMessageBumpsConversationUpdatedAt(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	before, err := s.Get(c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if err := s.AppendMessage(&Message{ConversationID: c.ID, Role: RoleUser, Content: "x"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	after, err := s.Get(c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.MsgCount != 1 {
		t.Fatalf("want msg count 1, got %d", after.MsgCount)
	}
	if after.UpdatedAt.Before(before.UpdatedAt) {
		t.Fatalf("updated_at went backwards: %v -> %v", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestUpdateMessagePersistsReplyMetadata(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	m := &Message{ConversationID: c.ID, Role: RoleAssistant}
	if err := s.AppendMessage(m); err != nil {
		t.Fatalf("append: %v", err)
	}

	m.Content = "final answer"
	m.Thinking = "reasoning trace"
	m.ModelUsed = "sonnet"
	m.TokensIn = 120
	m.TokensOut = 45
	m.CostUSD = 0.0021
	if err := s.UpdateMessage(m); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.GetMessage(m.ID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if got.Content != "final answer" || got.Thinking != "reasoning trace" ||
		got.ModelUsed != "sonnet" || got.TokensIn != 120 || got.TokensOut != 45 {
		t.Fatalf("metadata not persisted: %+v", got)
	}
	if got.CostUSD < 0.0020 || got.CostUSD > 0.0022 {
		t.Fatalf("cost not persisted: %v", got.CostUSD)
	}
}

func TestLastNReturnsAscendingWindow(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 10; i++ {
		if err := s.AppendMessage(&Message{ConversationID: c.ID, Role: RoleUser, Content: string(rune('a' + i))}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, err := s.LastN(c.ID, 3)
	if err != nil {
		t.Fatalf("lastN: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	// Should be the NEWEST three, but in ascending order.
	if got[0].Content != "h" || got[1].Content != "i" || got[2].Content != "j" {
		t.Fatalf("wrong window: %q %q %q", got[0].Content, got[1].Content, got[2].Content)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq <= got[i-1].Seq {
			t.Fatalf("not ascending: %d then %d", got[i-1].Seq, got[i].Seq)
		}
	}
}

func TestMessagesAfterSkipsSummarisedTail(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 6; i++ {
		if err := s.AppendMessage(&Message{ConversationID: c.ID, Role: RoleUser, Content: "m"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, err := s.MessagesAfter(c.ID, 4)
	if err != nil {
		t.Fatalf("messagesAfter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 messages after seq 4, got %d", len(got))
	}
	if got[0].Seq != 5 || got[1].Seq != 6 {
		t.Fatalf("wrong seqs: %d %d", got[0].Seq, got[1].Seq)
	}
}

func TestDeleteMessagesAfterTruncatesTail(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.AppendMessage(&Message{ConversationID: c.ID, Role: RoleUser, Content: "m"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	n, err := s.DeleteMessagesAfter(c.ID, 2)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3 deleted, got %d", n)
	}

	msgs, err := s.Messages(c.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 remaining, got %d", len(msgs))
	}
}

func TestListOrdersPinnedFirst(t *testing.T) {
	s := openTestStore(t)
	a := &Conversation{Title: "a"}
	b := &Conversation{Title: "b"}
	if err := s.Create(a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := s.Create(b); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if err := s.AppendMessage(&Message{ConversationID: a.ID, Role: RoleUser, Content: "x"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	pin := true
	if err := s.Update(b.ID, UpdateFields{Pinned: &pin}); err != nil {
		t.Fatalf("pin: %v", err)
	}

	list, err := s.List(10, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2, got %d", len(list))
	}
	if list[0].ID != b.ID {
		t.Fatalf("pinned conversation should sort first, got %q", list[0].Title)
	}
}

func TestListHidesArchivedByDefault(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "old"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	arch := true
	if err := s.Update(c.ID, UpdateFields{Archived: &arch}); err != nil {
		t.Fatalf("archive: %v", err)
	}

	list, err := s.List(10, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("archived conversation should be hidden, got %d", len(list))
	}

	withArchived, err := s.List(10, true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(withArchived) != 1 {
		t.Fatalf("want 1 with archived, got %d", len(withArchived))
	}
}

func TestDeleteRemovesMessagesAndAttachments(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.AppendMessage(&Message{ConversationID: c.ID, Role: RoleUser, Content: "x"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := s.AddAttachment(&Attachment{
		ConversationID: c.ID, Filename: "a.txt", Mime: "text/plain", Size: 3, Path: "/tmp/a.txt",
	}); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := s.Delete(c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("conversation should be gone, got %v", err)
	}
	msgs, err := s.Messages(c.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("orphan messages remain: %d", len(msgs))
	}
	atts, err := s.Attachments(c.ID)
	if err != nil {
		t.Fatalf("attachments: %v", err)
	}
	if len(atts) != 0 {
		t.Fatalf("orphan attachments remain: %d", len(atts))
	}
}

func TestDeleteMissingReturnsNotFound(t *testing.T) {
	s := openTestStore(t)
	if err := s.Delete("c_nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAttachmentRoundTrip(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}

	a := &Attachment{
		ConversationID: c.ID,
		Filename:       "spec.pdf",
		Mime:           "application/pdf",
		Size:           2048,
		Path:           "/data/attachments/x/spec.pdf",
		Extracted:      "the extracted text",
	}
	if err := s.AddAttachment(a); err != nil {
		t.Fatalf("add: %v", err)
	}
	if a.ID == "" {
		t.Fatal("id not assigned")
	}

	got, err := s.GetAttachment(a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Filename != "spec.pdf" || got.Extracted != "the extracted text" || got.Size != 2048 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestUpdateMissingConversationReturnsNotFound(t *testing.T) {
	s := openTestStore(t)
	title := "x"
	if err := s.Update("c_nope", UpdateFields{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateNoFieldsIsNoop(t *testing.T) {
	s := openTestStore(t)
	c := &Conversation{Title: "t"}
	if err := s.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	// An empty UpdateFields should not error and should not blank anything.
	if err := s.Update(c.ID, UpdateFields{}); err != nil {
		t.Fatalf("empty update: %v", err)
	}
	got, err := s.Get(c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "t" {
		t.Fatalf("title changed on empty update: %q", got.Title)
	}
}

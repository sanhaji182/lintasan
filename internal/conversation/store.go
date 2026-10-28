// Package conversation implements persistent multi-turn chat sessions for
// Lintasan: a conversation holds an ordered list of messages, a rolling
// summary of the older turns, and the model that answered each turn.
//
// This is the storage layer only. The turn loop (context assembly, upstream
// dispatch, tool calls) lives in the server package so it can reuse the
// existing gateway routing.
package conversation

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a conversation or message id does not exist.
var ErrNotFound = errors.New("conversation: not found")

// Role values stored in messages.role.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Conversation is one chat thread.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	SystemMsg string    `json:"system_prompt,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	// SummaryUpto is the id of the newest message folded into Summary. Messages
	// after it are still sent verbatim; everything at or before it is replaced
	// by Summary when the context is assembled.
	SummaryUpto string    `json:"summary_upto,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Pinned      bool      `json:"pinned"`
	Archived    bool      `json:"archived"`
	MsgCount    int       `json:"message_count"`
}

// Message is one turn in a conversation.
type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Seq            int       `json:"seq"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	// Thinking holds the model's reasoning trace when the upstream exposes one.
	Thinking string `json:"thinking,omitempty"`
	// Attachments is a JSON array of attachment metadata (id, name, mime, size).
	Attachments string `json:"attachments,omitempty"`
	// ToolCalls / ToolResults are JSON blobs for the tool-use loop.
	ToolCalls   string `json:"tool_calls,omitempty"`
	ToolResults string `json:"tool_results,omitempty"`
	ModelUsed   string `json:"model_used,omitempty"`
	TokensIn    int    `json:"tokens_in"`
	TokensOut   int    `json:"tokens_out"`
	CostUSD     float64 `json:"cost_usd"`
	// Error is set when the turn failed; the message is kept so the user can
	// retry without retyping.
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Store is the persistent conversation store backed by SQLite.
type Store struct {
	conn *sql.DB
}

// NewStore wraps an existing database connection. The caller is responsible
// for running Migrate first.
func NewStore(conn *sql.DB) *Store {
	return &Store{conn: conn}
}

// Migrate creates the conversation tables and indexes if they do not exist.
func (s *Store) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			system_prompt TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			summary_upto TEXT NOT NULL DEFAULT '',
			pinned INTEGER NOT NULL DEFAULT 0,
			archived INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			thinking TEXT NOT NULL DEFAULT '',
			attachments TEXT NOT NULL DEFAULT '',
			tool_calls TEXT NOT NULL DEFAULT '',
			tool_results TEXT NOT NULL DEFAULT '',
			model_used TEXT NOT NULL DEFAULT '',
			tokens_in INTEGER NOT NULL DEFAULT 0,
			tokens_out INTEGER NOT NULL DEFAULT 0,
			cost_usd REAL NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conv_seq ON messages(conversation_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_conversations_updated ON conversations(updated_at DESC)`,
		// Attachments live on disk; only metadata is stored here.
		`CREATE TABLE IF NOT EXISTS chat_attachments (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			message_id TEXT NOT NULL DEFAULT '',
			filename TEXT NOT NULL,
			mime TEXT NOT NULL,
			size INTEGER NOT NULL,
			path TEXT NOT NULL,
			extracted TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_attachments_conv ON chat_attachments(conversation_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.conn.Exec(stmt); err != nil {
			return fmt.Errorf("conversation migrate: %w", err)
		}
	}
	return nil
}

// NewID returns a random id with the given prefix. Exported so callers that
// must know an id before inserting (e.g. to build an on-disk filename) can
// reserve one.
func NewID(prefix string) string { return newID(prefix) }

// newID returns a 12-byte hex id with the given prefix.
func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is unrecoverable for id generation; fall back to
		// a timestamp so the call still returns something unique enough.
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(b[:])
}

// now returns the timestamp format used throughout the table.
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Create inserts a new conversation.
func (s *Store) Create(c *Conversation) error {
	if c.ID == "" {
		c.ID = newID("c_")
	}
	ts := now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	c.UpdatedAt = time.Now().UTC()
	_, err := s.conn.Exec(
		`INSERT INTO conversations (id, title, model, system_prompt, summary, summary_upto, pinned, archived, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.Model, c.SystemMsg, c.Summary, c.SummaryUpto,
		boolInt(c.Pinned), boolInt(c.Archived), ts, ts,
	)
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}
	return nil
}

// Get loads one conversation by id.
func (s *Store) Get(id string) (*Conversation, error) {
	row := s.conn.QueryRow(
		`SELECT id, title, model, system_prompt, summary, summary_upto, pinned, archived, created_at, updated_at
		 FROM conversations WHERE id = ?`, id)
	var c Conversation
	var created, updated string
	err := row.Scan(&c.ID, &c.Title, &c.Model, &c.SystemMsg, &c.Summary, &c.SummaryUpto,
		&c.Pinned, &c.Archived, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	c.CreatedAt = parseTime(created)
	c.UpdatedAt = parseTime(updated)
	// Message count is cheap and the UI wants it on the list view anyway.
	_ = s.conn.QueryRow(`SELECT COUNT(*) FROM messages WHERE conversation_id = ?`, id).Scan(&c.MsgCount)
	return &c, nil
}

// List returns conversations ordered by pinned then most recently updated.
// limit <= 0 means 50.
func (s *Store) List(limit int, includeArchived bool) ([]*Conversation, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT c.id, c.title, c.model, c.system_prompt, c.summary, c.summary_upto, c.pinned, c.archived,
	             c.created_at, c.updated_at,
	             (SELECT COUNT(*) FROM messages m WHERE m.conversation_id = c.id)
	      FROM conversations c`
	if !includeArchived {
		q += ` WHERE c.archived = 0`
	}
	q += ` ORDER BY c.pinned DESC, c.updated_at DESC LIMIT ?`

	rows, err := s.conn.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	out := make([]*Conversation, 0, 16)
	for rows.Next() {
		var c Conversation
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Title, &c.Model, &c.SystemMsg, &c.Summary, &c.SummaryUpto,
			&c.Pinned, &c.Archived, &created, &updated, &c.MsgCount); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		c.CreatedAt = parseTime(created)
		c.UpdatedAt = parseTime(updated)
		out = append(out, &c)
	}
	return out, rows.Err()
}

// UpdateFields applies a partial update. Only non-nil fields are written.
type UpdateFields struct {
	Title       *string
	Model       *string
	SystemMsg   *string
	Summary     *string
	SummaryUpto *string
	Pinned      *bool
	Archived    *bool
}

// Update applies a partial update to a conversation.
func (s *Store) Update(id string, f UpdateFields) error {
	sets := []string{}
	args := []any{}
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if f.Title != nil {
		add("title", *f.Title)
	}
	if f.Model != nil {
		add("model", *f.Model)
	}
	if f.SystemMsg != nil {
		add("system_prompt", *f.SystemMsg)
	}
	if f.Summary != nil {
		add("summary", *f.Summary)
	}
	if f.SummaryUpto != nil {
		add("summary_upto", *f.SummaryUpto)
	}
	if f.Pinned != nil {
		add("pinned", boolInt(*f.Pinned))
	}
	if f.Archived != nil {
		add("archived", boolInt(*f.Archived))
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, now(), id)

	res, err := s.conn.Exec(
		fmt.Sprintf(`UPDATE conversations SET %s WHERE id = ?`, joinComma(sets)), args...)
	if err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a conversation and all its messages/attachments metadata.
// Attachment files on disk are the caller's responsibility.
func (s *Store) Delete(id string) error {
	// Foreign keys are not guaranteed to be ON in sqlite3 by default, so delete
	// children explicitly rather than relying on the cascade.
	if _, err := s.conn.Exec(`DELETE FROM chat_attachments WHERE conversation_id = ?`, id); err != nil {
		return fmt.Errorf("delete attachments: %w", err)
	}
	if _, err := s.conn.Exec(`DELETE FROM messages WHERE conversation_id = ?`, id); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}
	res, err := s.conn.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AppendMessage adds a message to the end of a conversation and bumps the
// conversation's updated_at. Seq is assigned server-side so callers cannot
// create gaps or race each other.
func (s *Store) AppendMessage(m *Message) error {
	if m.ID == "" {
		m.ID = newID("m_")
	}
	var maxSeq sql.NullInt64
	if err := s.conn.QueryRow(
		`SELECT MAX(seq) FROM messages WHERE conversation_id = ?`, m.ConversationID).Scan(&maxSeq); err != nil {
		return fmt.Errorf("next seq: %w", err)
	}
	m.Seq = int(maxSeq.Int64) + 1
	m.CreatedAt = time.Now().UTC()
	ts := m.CreatedAt.Format(time.RFC3339Nano)

	_, err := s.conn.Exec(
		`INSERT INTO messages (id, conversation_id, seq, role, content, thinking, attachments,
		                      tool_calls, tool_results, model_used, tokens_in, tokens_out, cost_usd, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ConversationID, m.Seq, m.Role, m.Content, m.Thinking, m.Attachments,
		m.ToolCalls, m.ToolResults, m.ModelUsed, m.TokensIn, m.TokensOut, m.CostUSD, m.Error, ts,
	)
	if err != nil {
		return fmt.Errorf("append message: %w", err)
	}
	if _, err := s.conn.Exec(`UPDATE conversations SET updated_at = ? WHERE id = ?`, ts, m.ConversationID); err != nil {
		return fmt.Errorf("touch conversation: %w", err)
	}
	return nil
}

// UpdateMessage patches the mutable fields of an existing message. Used to
// fill in the assistant reply after streaming finishes.
func (s *Store) UpdateMessage(m *Message) error {
	_, err := s.conn.Exec(
		`UPDATE messages SET content = ?, thinking = ?, attachments = ?, tool_calls = ?, tool_results = ?,
		                    model_used = ?, tokens_in = ?, tokens_out = ?, cost_usd = ?, error = ?
		 WHERE id = ?`,
		m.Content, m.Thinking, m.Attachments, m.ToolCalls, m.ToolResults,
		m.ModelUsed, m.TokensIn, m.TokensOut, m.CostUSD, m.Error, m.ID,
	)
	if err != nil {
		return fmt.Errorf("update message: %w", err)
	}
	return nil
}

// Messages returns all messages in seq order.
func (s *Store) Messages(convID string) ([]*Message, error) {
	return s.messagesWhere(convID, `ORDER BY seq ASC`, -1)
}

// MessagesAfter returns messages with seq greater than afterSeq. Used by
// context assembly to skip everything already folded into the summary.
func (s *Store) MessagesAfter(convID string, afterSeq int) ([]*Message, error) {
	return s.messagesWhere(convID, `AND seq > ? ORDER BY seq ASC`, afterSeq)
}

// LastN returns the newest n messages, restored to ascending order.
func (s *Store) LastN(convID string, n int) ([]*Message, error) {
	if n <= 0 {
		n = 20
	}
	msgs, err := s.messagesWhere(convID, `ORDER BY seq DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	// Reverse into ascending order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *Store) messagesWhere(convID, tail string, arg any) ([]*Message, error) {
	q := `SELECT id, conversation_id, seq, role, content, thinking, attachments, tool_calls,
	             tool_results, model_used, tokens_in, tokens_out, cost_usd, error, created_at
	      FROM messages WHERE conversation_id = ? ` + tail
	args := []any{convID}
	if arg != nil && !(argIsInt(arg) && arg.(int) == -1) {
		args = append(args, arg)
	}

	rows, err := s.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	out := make([]*Message, 0, 16)
	for rows.Next() {
		var m Message
		var created string
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.Role, &m.Content, &m.Thinking,
			&m.Attachments, &m.ToolCalls, &m.ToolResults, &m.ModelUsed,
			&m.TokensIn, &m.TokensOut, &m.CostUSD, &m.Error, &created); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.CreatedAt = parseTime(created)
		out = append(out, &m)
	}
	return out, rows.Err()
}

// GetMessage loads a single message.
func (s *Store) GetMessage(id string) (*Message, error) {
	row := s.conn.QueryRow(
		`SELECT id, conversation_id, seq, role, content, thinking, attachments, tool_calls,
		        tool_results, model_used, tokens_in, tokens_out, cost_usd, error, created_at
		 FROM messages WHERE id = ?`, id)
	var m Message
	var created string
	err := row.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.Role, &m.Content, &m.Thinking,
		&m.Attachments, &m.ToolCalls, &m.ToolResults, &m.ModelUsed,
		&m.TokensIn, &m.TokensOut, &m.CostUSD, &m.Error, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	m.CreatedAt = parseTime(created)
	return &m, nil
}

// DeleteMessagesAfter removes every message with seq greater than afterSeq.
// This is what "edit an old message" does: truncate the tail, then re-run.
func (s *Store) DeleteMessagesAfter(convID string, afterSeq int) (int64, error) {
	res, err := s.conn.Exec(`DELETE FROM messages WHERE conversation_id = ? AND seq > ?`, convID, afterSeq)
	if err != nil {
		return 0, fmt.Errorf("delete messages after: %w", err)
	}
	// If the summary pointed into the deleted tail, drop the pointer so the
	// next assembly re-summarises from what actually remains.
	if _, err := s.conn.Exec(
		`UPDATE conversations SET summary_upto = '' WHERE id = ? AND summary_upto != '' AND
		 summary_upto NOT IN (SELECT id FROM messages WHERE conversation_id = ?)`, convID, convID); err != nil {
		return 0, fmt.Errorf("reset summary pointer: %w", err)
	}
	return res.RowsAffected()
}

// ---- attachments ----

// Attachment is file metadata for an uploaded file. The bytes live on disk at
// Path; Extracted holds the plain text pulled out of it for models that cannot
// read the binary format directly.
type Attachment struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	MessageID      string    `json:"message_id,omitempty"`
	Filename       string    `json:"filename"`
	Mime           string    `json:"mime"`
	Size           int64     `json:"size"`
	Path           string    `json:"-"`
	Extracted      string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}

// AddAttachment records an uploaded file.
func (s *Store) AddAttachment(a *Attachment) error {
	if a.ID == "" {
		a.ID = newID("a_")
	}
	a.CreatedAt = time.Now().UTC()
	_, err := s.conn.Exec(
		`INSERT INTO chat_attachments (id, conversation_id, message_id, filename, mime, size, path, extracted, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ConversationID, a.MessageID, a.Filename, a.Mime, a.Size, a.Path, a.Extracted,
		a.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("add attachment: %w", err)
	}
	return nil
}

// Attachments lists the attachments belonging to a conversation.
func (s *Store) Attachments(convID string) ([]*Attachment, error) {
	rows, err := s.conn.Query(
		`SELECT id, conversation_id, message_id, filename, mime, size, path, extracted, created_at
		 FROM chat_attachments WHERE conversation_id = ? ORDER BY created_at ASC`, convID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()

	out := make([]*Attachment, 0, 8)
	for rows.Next() {
		var a Attachment
		var created string
		if err := rows.Scan(&a.ID, &a.ConversationID, &a.MessageID, &a.Filename, &a.Mime,
			&a.Size, &a.Path, &a.Extracted, &created); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		a.CreatedAt = parseTime(created)
		out = append(out, &a)
	}
	return out, rows.Err()
}

// GetAttachment loads one attachment by id.
func (s *Store) GetAttachment(id string) (*Attachment, error) {
	row := s.conn.QueryRow(
		`SELECT id, conversation_id, message_id, filename, mime, size, path, extracted, created_at
		 FROM chat_attachments WHERE id = ?`, id)
	var a Attachment
	var created string
	err := row.Scan(&a.ID, &a.ConversationID, &a.MessageID, &a.Filename, &a.Mime,
		&a.Size, &a.Path, &a.Extracted, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get attachment: %w", err)
	}
	a.CreatedAt = parseTime(created)
	return &a, nil
}

// ---- helpers ----

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func argIsInt(v any) bool {
	_, ok := v.(int)
	return ok
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// SetAttachmentPath records the final on-disk location of an attachment.
//
// Uploads need to write the bytes first (so a crash leaves no row pointing at
// nothing), but the destination name depends on the row id, which is only known
// after the insert. This closes that gap.
func (s *Store) SetAttachmentPath(id, path string) error {
	res, err := s.conn.Exec(`UPDATE chat_attachments SET path = ? WHERE id = ?`, path, id)
	if err != nil {
		return fmt.Errorf("set attachment path: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAttachment removes an attachment row. The caller owns the file.
func (s *Store) DeleteAttachment(id string) error {
	res, err := s.conn.Exec(`DELETE FROM chat_attachments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

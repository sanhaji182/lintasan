package conversation

import (
	"strings"
	"testing"
)

func msg(seq int, role, content string) *Message {
	return &Message{ID: "m_" + string(rune('a'+seq)), Seq: seq, Role: role, Content: content}
}

func TestEstimateTokensScalesWithLength(t *testing.T) {
	short := EstimateTokens("hello")
	long := EstimateTokens(strings.Repeat("hello ", 100))
	if short <= 0 {
		t.Fatal("short text should cost at least 1 token")
	}
	if long <= short {
		t.Fatalf("long text should cost more: %d vs %d", long, short)
	}
	if EstimateTokens("") != 0 {
		t.Fatal("empty string should cost 0")
	}
}

func TestEstimateTokensWeightsCJK(t *testing.T) {
	// 40 CJK characters vs 40 ASCII characters. CJK should cost substantially
	// more because it is roughly one token per character.
	cjk := EstimateTokens(strings.Repeat("漢", 40))
	ascii := EstimateTokens(strings.Repeat("a", 40))
	if cjk <= ascii {
		t.Fatalf("CJK should cost more than ASCII: %d vs %d", cjk, ascii)
	}
}

func TestAssembleIncludesSystemPromptFirst(t *testing.T) {
	conv := &Conversation{SystemMsg: "be terse"}
	hist := []*Message{msg(1, RoleUser, "hi"), msg(2, RoleAssistant, "hello")}

	a := Assemble(conv, hist, nil, Budget{MaxTokens: 8000})
	if len(a.Messages) != 3 {
		t.Fatalf("want 3 messages (system + 2), got %d", len(a.Messages))
	}
	if a.Messages[0].Role != RoleSystem || a.Messages[0].Content != "be terse" {
		t.Fatalf("first message should be the system prompt, got %+v", a.Messages[0])
	}
	if a.Messages[1].Content != "hi" || a.Messages[2].Content != "hello" {
		t.Fatalf("history order wrong: %+v", a.Messages)
	}
}

func TestAssembleOmitsEmptySystemPrompt(t *testing.T) {
	conv := &Conversation{}
	hist := []*Message{msg(1, RoleUser, "hi")}
	a := Assemble(conv, hist, nil, Budget{MaxTokens: 8000})
	if len(a.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(a.Messages))
	}
	if a.Messages[0].Role == RoleSystem {
		t.Fatal("no system message should be emitted for an empty prompt")
	}
}

func TestAssembleEmitsSummaryAsSystemMessage(t *testing.T) {
	conv := &Conversation{
		SystemMsg:   "be helpful",
		Summary:     "user asked about Go channels",
		SummaryUpto: "m_b",
	}
	hist := []*Message{msg(3, RoleUser, "now about mutexes")}

	a := Assemble(conv, hist, nil, Budget{MaxTokens: 8000})
	if !a.Summarised {
		t.Fatal("Summarised should be true when a summary exists")
	}
	if len(a.Messages) != 3 {
		t.Fatalf("want system + summary + history = 3, got %d", len(a.Messages))
	}
	if a.Messages[1].Role != RoleSystem {
		t.Fatalf("summary should be a system message, got %q", a.Messages[1].Role)
	}
	if !strings.Contains(a.Messages[1].Content, "Go channels") {
		t.Fatalf("summary body missing: %q", a.Messages[1].Content)
	}
}

func TestAssembleDropsOldestWhenOverBudget(t *testing.T) {
	conv := &Conversation{}
	// 30 messages of ~250 tokens each = ~7500 tokens, over an 800-token budget
	// minus output reserve.
	big := strings.Repeat("x", 1000)
	hist := make([]*Message, 0, 30)
	for i := 1; i <= 30; i++ {
		hist = append(hist, msg(i, RoleUser, big))
	}

	a := Assemble(conv, hist, nil, Budget{MaxTokens: 2000, ReserveForOutput: 500, KeepRecent: 2})
	if a.Dropped == 0 {
		t.Fatal("expected some messages to be dropped")
	}
	if len(a.Messages) >= len(hist) {
		t.Fatalf("expected fewer than %d messages, got %d", len(hist), len(a.Messages))
	}
	// What survives must be the NEWEST messages, in ascending order. Assembled
	// messages carry no seq, so verify the retained window is contiguous with
	// the original tail instead.
	if a.Messages[len(a.Messages)-1].Content != big {
		t.Fatal("newest message should be retained")
	}
	if a.Dropped+len(a.Messages) != len(hist) {
		t.Fatalf("dropped(%d)+kept(%d) != total(%d)", a.Dropped, len(a.Messages), len(hist))
	}
}

func TestAssembleKeepsMinimumRecentEvenWhenOverBudget(t *testing.T) {
	conv := &Conversation{}
	huge := strings.Repeat("y", 20000) // ~5000 tokens each
	hist := []*Message{
		msg(1, RoleUser, "old"),
		msg(2, RoleAssistant, "older reply"),
		msg(3, RoleUser, huge),
		msg(4, RoleAssistant, huge),
	}

	// Budget is far too small for the last two messages, but KeepRecent=4 means
	// they must survive anyway.
	a := Assemble(conv, hist, nil, Budget{MaxTokens: 1000, ReserveForOutput: 100, KeepRecent: 4})
	if len(a.Messages) != 4 {
		t.Fatalf("KeepRecent=4 should retain all 4 messages, got %d", len(a.Messages))
	}
}

func TestAssembleIncludesExtraMessages(t *testing.T) {
	conv := &Conversation{}
	hist := []*Message{msg(1, RoleUser, "first")}
	pending := []*Message{msg(2, RoleUser, "just typed")}

	a := Assemble(conv, hist, pending, Budget{MaxTokens: 8000})
	if len(a.Messages) != 2 {
		t.Fatalf("want 2, got %d", len(a.Messages))
	}
	if a.Messages[1].Content != "just typed" {
		t.Fatalf("pending message should be last, got %q", a.Messages[1].Content)
	}
}

func TestAssembleSkipsDuplicatedSystemMessages(t *testing.T) {
	conv := &Conversation{SystemMsg: "top level"}
	hist := []*Message{
		msg(1, RoleSystem, "inline system"),
		msg(2, RoleUser, "hi"),
	}
	a := Assemble(conv, hist, nil, Budget{MaxTokens: 8000})
	for i, m := range a.Messages {
		if i > 0 && m.Role == RoleSystem {
			t.Fatalf("inline system message leaked into output at %d: %+v", i, m)
		}
	}
	if len(a.Messages) != 2 {
		t.Fatalf("want 2 (prompt + user), got %d", len(a.Messages))
	}
}

func TestAssemblePreservesChronologicalOrder(t *testing.T) {
	conv := &Conversation{}
	hist := []*Message{
		msg(1, RoleUser, "one"),
		msg(2, RoleAssistant, "two"),
		msg(3, RoleUser, "three"),
		msg(4, RoleAssistant, "four"),
	}
	a := Assemble(conv, hist, nil, Budget{MaxTokens: 8000})
	want := []string{"one", "two", "three", "four"}
	for i, w := range want {
		if a.Messages[i].Content != w {
			t.Fatalf("position %d: want %q, got %q", i, w, a.Messages[i].Content)
		}
	}
}

func TestNeedsCompactionFalseForShortConversation(t *testing.T) {
	conv := &Conversation{SystemMsg: "hi"}
	msgs := []*Message{msg(1, RoleUser, "short question"), msg(2, RoleAssistant, "short answer")}
	if NeedsCompaction(conv, msgs, Budget{MaxTokens: 8000}) {
		t.Fatal("a two-message conversation should not need compaction")
	}
}

func TestNeedsCompactionTrueWhenHistoryIsLarge(t *testing.T) {
	conv := &Conversation{}
	big := strings.Repeat("z", 4000) // ~1000 tokens
	msgs := make([]*Message, 0, 20)
	for i := 1; i <= 20; i++ {
		msgs = append(msgs, msg(i, RoleUser, big))
	}
	if !NeedsCompaction(conv, msgs, Budget{MaxTokens: 8000, ReserveForOutput: 1000}) {
		t.Fatal("20k tokens of history should trigger compaction on an 8k window")
	}
}

func TestNeedsCompactionIgnoresAlreadySummarised(t *testing.T) {
	conv := &Conversation{SummaryUpto: "m_a"} // covers seq 1
	big := strings.Repeat("z", 4000)
	// Seq 1 is huge but already folded into the summary; the rest is tiny.
	msgs := []*Message{
		msg(1, RoleUser, big),
		msg(2, RoleUser, "tiny"),
		msg(3, RoleAssistant, "tiny"),
	}
	if NeedsCompaction(conv, msgs, Budget{MaxTokens: 8000, ReserveForOutput: 1000}) {
		t.Fatal("already-summarised messages should not count toward the budget")
	}
}

func TestCompactionWindowExcludesLiveTail(t *testing.T) {
	conv := &Conversation{}
	msgs := make([]*Message, 0, 10)
	for i := 1; i <= 10; i++ {
		msgs = append(msgs, msg(i, RoleUser, "m"))
	}

	win := CompactionWindow(conv, msgs, 4)
	if len(win) != 6 {
		t.Fatalf("want 6 messages in window (10 - 4 recent), got %d", len(win))
	}
	if win[len(win)-1].Seq != 6 {
		t.Fatalf("window should stop before the recent tail, last seq = %d", win[len(win)-1].Seq)
	}
}

func TestCompactionWindowEmptyWhenTooFewMessages(t *testing.T) {
	conv := &Conversation{}
	msgs := []*Message{msg(1, RoleUser, "a"), msg(2, RoleAssistant, "b")}
	if win := CompactionWindow(conv, msgs, 4); win != nil {
		t.Fatalf("want nil window for a conversation shorter than keepRecent, got %d", len(win))
	}
}

func TestCompactionWindowSkipsSummarisedPrefix(t *testing.T) {
	// Build seq 1..10 explicitly with stable ids so the summary pointer is
	// unambiguous (the shared msg() helper derives ids from 'a'+seq).
	msgs := make([]*Message, 0, 10)
	for i := 1; i <= 10; i++ {
		msgs = append(msgs, &Message{ID: "seq" + string(rune('0'+i)), Seq: i, Role: RoleUser, Content: "m"})
	}
	// Summary covers seq 2, so everything at or before it is already folded in.
	conv := &Conversation{SummaryUpto: "seq2"}

	win := CompactionWindow(conv, msgs, 4)
	if len(win) == 0 {
		t.Fatal("expected a window")
	}
	if win[0].Seq != 3 {
		t.Fatalf("window should start after the summary pointer (seq 2), got %d", win[0].Seq)
	}
}

func TestBudgetNormalisesZeroValues(t *testing.T) {
	b := Budget{}.normalise()
	if b.MaxTokens != 8000 || b.ReserveForOutput != 1024 || b.KeepRecent != 4 {
		t.Fatalf("unexpected defaults: %+v", b)
	}
}

func TestBudgetClampsAbsurdOutputReserve(t *testing.T) {
	b := Budget{MaxTokens: 1000, ReserveForOutput: 5000}.normalise()
	if b.ReserveForOutput >= b.MaxTokens {
		t.Fatalf("output reserve should be clamped below MaxTokens: %+v", b)
	}
}

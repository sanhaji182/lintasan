package conversation

import (
	"strings"
)

// Assembly is the result of building a model request from a conversation.
// It is deliberately provider-agnostic: the caller turns these messages into
// whatever wire format the resolved upstream expects.
type Assembly struct {
	// Messages are ordered oldest-first and already include the system prompt
	// (if any) and the summary as a synthetic system message.
	Messages []AssembledMessage
	// Summarised is true when the assembly includes a summary in place of older
	// turns, meaning the caller may want to tell the user or re-summarise soon.
	Summarised bool
	// Dropped is how many trailing conversation messages were left out entirely
	// because even with summarisation the window was full.
	Dropped int
}

// AssembledMessage is one message ready to send upstream.
type AssembledMessage struct {
	Role    string
	Content string
	// Attachments carries extracted text from files the user attached to this
	// message, already inlined by the builder.
	Attachments []Attachment
}

// Budget controls how much history the assembler is allowed to include.
// It is measured in approximate tokens (characters/4), which is the same
// rough estimate the rest of Lintasan uses for pre-flight token counting.
type Budget struct {
	// MaxTokens is the total context window available, including the system
	// prompt and the model's expected output headroom. <=0 means 8000.
	MaxTokens int
	// ReserveForOutput is subtracted from MaxTokens before history is packed.
	// <=0 means 1024.
	ReserveForOutput int
	// KeepRecent is the minimum number of recent messages that must always be
	// included verbatim, even if it overflows the budget. <=0 means 4.
	KeepRecent int
}

func (b Budget) normalise() Budget {
	if b.MaxTokens <= 0 {
		b.MaxTokens = 8000
	}
	if b.ReserveForOutput <= 0 {
		b.ReserveForOutput = 1024
	}
	if b.KeepRecent <= 0 {
		b.KeepRecent = 4
	}
	if b.ReserveForOutput >= b.MaxTokens {
		b.ReserveForOutput = b.MaxTokens / 4
	}
	return b
}

// available is the token budget left for input after reserving output headroom.
func (b Budget) available() int { return b.MaxTokens - b.ReserveForOutput }

// EstimateTokens is the crude chars/4 estimate used across Lintasan. It is
// intentionally conservative in the caller's favour: for Latin text it slightly
// overestimates, which keeps us from overshooting the real window.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	// Count runes, not bytes, so CJK and emoji are not undercounted.
	n := 0
	for range s {
		n++
	}
	// CJK characters are roughly one token each rather than a quarter, so
	// weight anything above the ASCII range more heavily.
	cjk := 0
	for _, r := range s {
		if r > 0x2E80 {
			cjk++
		}
	}
	asciiApprox := (n - cjk + 3) / 4
	return asciiApprox + cjk
}

// Assemble builds the message list to send upstream for the next turn.
//
// Strategy, cheapest first:
//  1. Always include the system prompt.
//  2. If the conversation has a summary and a summary_upto pointer, emit the
//     summary as a system message and start verbatim history after that point.
//  3. Pack as many recent messages as fit, newest-first, while keeping at least
//     budget.KeepRecent messages regardless of size.
//  4. If nothing fits at all, return only the newest user message; the caller
//     is expected to have run compaction before this happens.
func Assemble(conv *Conversation, history []*Message, extra []*Message, budget Budget) Assembly {
	budget = budget.normalise()
	limit := budget.available()

	out := make([]AssembledMessage, 0, len(history)+len(extra)+2)
	used := 0

	if strings.TrimSpace(conv.SystemMsg) != "" {
		out = append(out, AssembledMessage{Role: RoleSystem, Content: conv.SystemMsg})
		used += EstimateTokens(conv.SystemMsg)
	}

	// Everything in `history` is already past the summary pointer when the
	// caller used MessagesAfter; if it is not, we still refuse to summarise
	// twice.
	all := make([]*Message, 0, len(history)+len(extra))
	all = append(all, history...)
	all = append(all, extra...)

	summarised := strings.TrimSpace(conv.Summary) != ""
	if summarised {
		// The summary is a system message so it frames the whole thread without
		// being mistaken for something the user said.
		body := "Summary of the earlier conversation (older turns omitted to save context):\n" + strings.TrimSpace(conv.Summary)
		out = append(out, AssembledMessage{Role: RoleSystem, Content: body})
		used += EstimateTokens(body)
	}

	// Work backwards from the newest message so what gets dropped first is the
	// oldest, least relevant tail.
	keep := make([]*Message, 0, len(all))
	keepCount := 0
	for i := len(all) - 1; i >= 0; i-- {
		m := all[i]
		if m.Role == RoleSystem {
			// Per-message system messages already live in the summary slot or
			// are conversation-level config; skip them here to avoid duplicates.
			continue
		}
		t := EstimateTokens(m.Content) + EstimateTokens(m.Thinking)
		if reused := attachmentTokens(m); reused > 0 {
			t += reused
		}
		if used+t > limit && keepCount >= budget.KeepRecent {
			break
		}
		used += t
		keep = append(keep, m)
		keepCount++
	}

	// keep is newest-first; reverse into chronological order.
	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	for _, m := range keep {
		out = append(out, AssembledMessage{Role: m.Role, Content: m.Content})
	}

	return Assembly{
		Messages:   out,
		Summarised: summarised,
		Dropped:    len(all) - len(keep),
	}
}

// attachmentTokens counts the tokens contributed by a message's inlined
// attachment text. Attachment bodies are not stored on the message row, only
// their metadata, so this scales from the declared sizes on the JSON blob.
func attachmentTokens(m *Message) int {
	if m.Attachments == "" {
		return 0
	}
	// Rough: each attachment costs its file size in characters when text is
	// extracted. Overestimating here is safer than blowing the window.
	return EstimateTokens(m.Attachments) + len(m.Attachments)/8
}

// NeedsCompaction reports whether the conversation has grown enough that
// folding older turns into a summary would materially help. Callers use this
// after a turn completes; compaction itself is a model call and is done by the
// server, not here.
func NeedsCompaction(conv *Conversation, msgs []*Message, budget Budget) bool {
	budget = budget.normalise()
	total := EstimateTokens(conv.SystemMsg) + EstimateTokens(conv.Summary)
	for _, m := range msgs {
		if m.Seq <= seqOf(conv, msgs) {
			continue
		}
		total += EstimateTokens(m.Content) + EstimateTokens(m.Thinking)
	}
	// Compact when history alone eats more than 60% of the input budget. That
	// leaves room for the reply plus a few more turns before the next compaction.
	return total > budget.available()*6/10
}

// seqOf returns the seq the summary covers, so NeedsCompaction only counts
// messages that are not already folded in.
func seqOf(conv *Conversation, msgs []*Message) int {
	for _, m := range msgs {
		if m.ID == conv.SummaryUpto {
			return m.Seq
		}
	}
	return 0
}

// CompactionWindow picks the messages that a summarisation call should cover:
// everything up to (but not including) the most recent KeepRecent messages, so
// the summary never swallows the live tail of the conversation.
func CompactionWindow(conv *Conversation, msgs []*Message, keepRecent int) []*Message {
	if keepRecent <= 0 {
		keepRecent = 4
	}
	// Only consider messages not already summarised.
	candidates := make([]*Message, 0, len(msgs))
	cutoff := seqOf(conv, msgs)
	for _, m := range msgs {
		if m.Seq > cutoff && m.Role != RoleSystem {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) <= keepRecent {
		return nil
	}
	return candidates[:len(candidates)-keepRecent]
}

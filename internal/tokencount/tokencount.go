// Package tokencount provides a real BPE token counter (tiktoken cl100k_base)
// with a safe char/4 fallback. It is used wherever an accurate-ish input token
// estimate matters (quota gating, budget guardrails) without paying a round
// trip to the upstream provider's tokenizer.
package tokencount

import (
	"sync"

	"github.com/pkoukk/tiktoken-go"
	tiktokenloader "github.com/pkoukk/tiktoken-go-loader"
)

var (
	encOnce sync.Once
	enc     *tiktoken.Tiktoken
)

// encoder lazily initializes the cl100k_base encoder using the embedded offline
// vocab (no network). Returns nil if init fails, signaling callers to fall back.
func encoder() *tiktoken.Tiktoken {
	encOnce.Do(func() {
		tiktoken.SetBpeLoader(tiktokenloader.NewOfflineLoader())
		e, err := tiktoken.GetEncoding("cl100k_base")
		if err == nil {
			enc = e
		}
	})
	return enc
}

// Count returns the cl100k_base token count for s, falling back to a char/4
// heuristic if the tokenizer is unavailable.
func Count(s string) int {
	if s == "" {
		return 0
	}
	if e := encoder(); e != nil {
		return len(e.Encode(s, nil, nil))
	}
	return (len(s) + 3) / 4
}

// CountMessages estimates the total input tokens across chat messages by
// counting each message's textual content. Non-string / structured content is
// flattened to its text parts. This is an input-side estimate; it does not add
// per-message role framing overhead.
func CountMessages(messages []any) int {
	total := 0
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		switch c := msg["content"].(type) {
		case string:
			total += Count(c)
		case []any:
			for _, item := range c {
				if im, ok := item.(map[string]any); ok {
					if txt, ok := im["text"].(string); ok {
						total += Count(txt)
					}
				}
			}
		}
	}
	return total
}

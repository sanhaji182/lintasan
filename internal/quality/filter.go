// Package quality scores LLM responses on multiple dimensions and decides
// whether to reroute to a fallback model when quality is low.
//
// No external dependencies — pure Go standard library.
package quality

import (
	"math"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Weights for the four quality dimensions.
type Weights struct {
	Completeness float64
	Coherence    float64
	Correctness  float64
	Relevance    float64
}

// DefaultWeights returns the standard quality weights.
func DefaultWeights() Weights {
	return Weights{
		Completeness: 0.30,
		Coherence:    0.25,
		Correctness:  0.20,
		Relevance:    0.25,
	}
}

// Dimensions holds the per-dimension scores.
type Dimensions struct {
	Completeness float64
	Coherence    float64
	Correctness  float64
	Relevance    float64
}

// Result is the output of a quality evaluation.
type Result struct {
	Score      float64    // weighted total 0–1
	Pass       bool       // true if score >= threshold
	Reason     string     // explanation when !Pass
	Dimensions Dimensions // per-dimension scores
	Threshold  float64
}

// Filter is a configured quality evaluator.
type Filter struct {
	Threshold float64
	Weights   Weights
}

// New creates a Filter with the given threshold and weights.
// If weights are zero-valued, DefaultWeights is used.
func New(threshold float64, weights Weights) *Filter {
	if threshold <= 0 {
		threshold = 0.4
	}
	if weights.Completeness == 0 && weights.Coherence == 0 &&
		weights.Correctness == 0 && weights.Relevance == 0 {
		weights = DefaultWeights()
	}
	return &Filter{Threshold: threshold, Weights: weights}
}

// ---------------------------------------------------------------------------
// Quality scoring
// ---------------------------------------------------------------------------

// Evaluate scores response content against the original user query and returns a Result.
func (f *Filter) Evaluate(response string, userQuery string) Result {
	trimmed := strings.TrimSpace(response)

	// Hard failure: empty or whitespace-only response
	if len(trimmed) == 0 {
		return Result{
			Score:  0,
			Pass:   false,
			Reason: "empty response",
			Dimensions: Dimensions{
				Completeness: 0,
				Coherence:    0,
				Correctness:  0,
				Relevance:    0,
			},
			Threshold: f.Threshold,
		}
	}

	dim := Dimensions{
		Completeness: scoreCompleteness(trimmed),
		Coherence:    scoreCoherence(trimmed),
		Correctness:  scoreCorrectness(trimmed),
		Relevance:    scoreRelevance(trimmed, userQuery),
	}

	total := dim.Completeness*f.Weights.Completeness +
		dim.Coherence*f.Weights.Coherence +
		dim.Correctness*f.Weights.Correctness +
		dim.Relevance*f.Weights.Relevance

	total = math.Round(total*100) / 100
	pass := total >= f.Threshold

	reason := ""
	if !pass {
		reason = "quality below threshold"
	}

	return Result{
		Score:      total,
		Pass:       pass,
		Reason:     reason,
		Dimensions: dim,
		Threshold:  f.Threshold,
	}
}

// ---------------------------------------------------------------------------
// Dimension scorers
// ---------------------------------------------------------------------------

// scoreCompleteness evaluates whether the response appears complete.
// Penalizes empty/short responses and those ending mid-sentence.
func scoreCompleteness(response string) float64 {
	score := 1.0

	trimmed := strings.TrimSpace(response)

	// Empty response
	if len(trimmed) == 0 {
		return 0.0
	}

	words := strings.Fields(trimmed)
	wordCount := len(words)

	// Very short response (< 10 words)
	if wordCount < 10 {
		score -= 0.3
	}

	// Check if it ends cleanly (with punctuation, code fence close, etc.)
	lastChar := trimmed[len(trimmed)-1]
	endsClean := strings.ContainsRune(".!?)}\"'\n", rune(lastChar)) ||
		strings.HasSuffix(trimmed, "```")

	if !endsClean && wordCount < 20 {
		score -= 0.2
	}

	// Penalty for obvious truncation markers
	if strings.HasSuffix(trimmed, "...") {
		score -= 0.3
	}

	return clampScore(score)
}

// scoreCoherence evaluates sentence flow and structure.
func scoreCoherence(response string) float64 {
	score := 0.8
	trimmed := strings.TrimSpace(response)

	// Split into sentences
	sentences := splitSentences(trimmed)
	if len(sentences) >= 3 {
		// Check for repetitive sentences
		unique := make(map[string]struct{}, len(sentences))
		for _, s := range sentences {
			unique[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
		}
		uniqueRatio := float64(len(unique)) / float64(len(sentences))
		if uniqueRatio < 0.5 {
			score -= 0.4
		}
	}

	// Gibberish check: high ratio of non-alphanumeric chars
	alphaCount := 0
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r > 127 {
			alphaCount++
		}
	}
	alphaRatio := float64(alphaCount) / float64(max(len(trimmed), 1))
	if alphaRatio < 0.3 && !strings.Contains(trimmed, "```") {
		score -= 0.3
	}

	// Bonus for structured content
	if hasMarkdownHeaders.MatchString(trimmed) ||
		hasNumberedList.MatchString(trimmed) ||
		strings.Contains(trimmed, "```") {
		score += 0.1
	}

	return clampScore(score)
}

// scoreCorrectness checks for obvious error patterns in the response.
func scoreCorrectness(response string) float64 {
	score := 1.0
	lower := strings.ToLower(response)

	// Error/apology patterns that suggest a bad response
	errorPatterns := []string{
		"i'm sorry",
		"i cannot",
		"i can't",
		"as an ai",
		"i am unable",
		"i don't have",
		"i do not have",
		"unfortunately, i cannot",
		"error occurred",
		"something went wrong",
	}

	for _, p := range errorPatterns {
		if strings.Contains(lower, p) {
			score -= 0.15
			if score < 0 {
				score = 0
			}
		}
	}

	// Hallucination/confabulation indicators (common nonsense phrases)
	nonsensePatterns := []string{
		"the meaning of life is",
		"according to my calculations",
		"i am fully confident",
	}

	for _, p := range nonsensePatterns {
		if strings.Contains(lower, p) && len(strings.Fields(response)) < 30 {
			score -= 0.1
		}
	}

	return clampScore(score)
}

// scoreRelevance checks keyword overlap between the response and the user query.
func scoreRelevance(response string, userQuery string) float64 {
	if userQuery == "" {
		return 0.7 // can't assess without query
	}

	userTerms := extractKeyTerms(userQuery)
	respTerms := extractKeyTerms(response)

	if len(userTerms) == 0 {
		return 0.7
	}

	// Build a set of response terms for O(1) lookup
	respSet := make(map[string]struct{}, len(respTerms))
	for _, t := range respTerms {
		respSet[t] = struct{}{}
	}

	overlap := 0
	for _, t := range userTerms {
		if _, ok := respSet[t]; ok {
			overlap++
		}
	}

	overlapRatio := float64(overlap) / float64(len(userTerms))
	score := 0.5 + overlapRatio*0.5

	return clampScore(score)
}

// ---------------------------------------------------------------------------
// Reroute decision
// ---------------------------------------------------------------------------

// ShouldReroute returns true when the quality filter recommends rerouting
// to a fallback model.
func ShouldReroute(result Result, attempt int, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}
	if !result.Pass {
		return true
	}
	// Also reroute if score is very low even if technically passing
	if result.Score < 0.3 {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return math.Round(s*100) / 100
}

func splitSentences(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == '\n'
	})
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) > 10 {
			out = append(out, p)
		}
	}
	return out
}

// stopWords is a set of common English and Indonesian stop words.
var stopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},
	"have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {}, "will": {}, "would": {}, "could": {},
	"should": {}, "may": {}, "might": {}, "shall": {}, "can": {}, "need": {}, "dare": {}, "ought": {},
	"used": {}, "to": {}, "of": {}, "in": {}, "for": {}, "on": {}, "with": {}, "at": {}, "by": {}, "from": {},
	"as": {}, "into": {}, "through": {}, "during": {}, "before": {}, "after": {}, "above": {}, "below": {},
	"between": {}, "out": {}, "off": {}, "over": {}, "under": {}, "again": {}, "further": {}, "then": {},
	"once": {}, "here": {}, "there": {}, "when": {}, "where": {}, "why": {}, "how": {}, "all": {}, "each": {},
	"every": {}, "both": {}, "few": {}, "more": {}, "most": {}, "other": {}, "some": {}, "such": {}, "no": {},
	"nor": {}, "not": {}, "only": {}, "own": {}, "same": {}, "so": {}, "than": {}, "too": {}, "very": {},
	"just": {}, "because": {}, "but": {}, "and": {}, "or": {}, "if": {}, "while": {}, "that": {}, "this": {},
	"yang": {}, "dan": {}, "di": {}, "ke": {}, "dari": {}, "untuk": {}, "dengan": {}, "ini": {}, "itu": {},
	"what": {}, "you": {}, "i": {}, "me": {}, "my": {}, "we": {}, "our": {}, "it": {}, "its": {},
}

var nonAlphaRegex = regexp.MustCompile(`[^a-z0-9\s]+`)

// extractKeyTerms returns meaningful words from text, filtering stop words.
func extractKeyTerms(text string) []string {
	lower := strings.ToLower(text)
	lower = nonAlphaRegex.ReplaceAllString(lower, " ")
	words := strings.Fields(lower)
	var out []string
	for _, w := range words {
		if len(w) > 2 {
			if _, stop := stopWords[w]; !stop {
				out = append(out, w)
			}
		}
	}
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

var (
	hasMarkdownHeaders = regexp.MustCompile(`(?m)^#{1,3}\s`)
	hasNumberedList    = regexp.MustCompile(`(?m)^\d+\.\s`)
)

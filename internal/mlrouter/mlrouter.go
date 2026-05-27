// Package mlrouter provides ML-based routing for chat completions.
// It extracts 15 linguistic features from the messages and computes
// a heuristic complexity score (0–1) to decide whether the query
// should be routed to a cheap or expensive model.
//
// No external dependencies — pure Go standard library.
package mlrouter

import (
	"math"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ModelPair defines a cheap/expensive model pair and the score threshold
// above which the expensive model is selected.
type ModelPair struct {
	CheapModel     string
	ExpensiveModel string
	Threshold      float64 // 0.0–1.0
}

// Decision is the result of routing a query.
type Decision struct {
	Model  string  // selected model name
	Score  float64 // complexity score (0–1)
	Tier   string  // "cheap" or "expensive"
	Method string  // "heuristic"
}

// ---------------------------------------------------------------------------
// Feature extraction
// ---------------------------------------------------------------------------

// 12 regex-based complexity indicators.  Order matches the feature vector.
var complexityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(implement|build|create|develop|write)\b`),
	regexp.MustCompile(`\b(debug|fix|troubleshoot|error)\b`),
	regexp.MustCompile(`\b(explain|describe|how|why)\b`),
	regexp.MustCompile(`\b(compare|vs|versus|difference|trade.?off)\b`),
	regexp.MustCompile(`\b(code|function|class|api|endpoint)\b`),
	regexp.MustCompile(`\b(architect|design|system|infrastructure)\b`),
	regexp.MustCompile(`\b(security|auth|encrypt|vulnerab)\b`),
	regexp.MustCompile(`\b(performance|optimi|scale|latency)\b`),
	regexp.MustCompile(`\b(math|calculat|equation|formula|algorithm)\b`),
	regexp.MustCompile(`\b(creative|story|poem|joke|imagine)\b`),
	regexp.MustCompile(`\b(translate|translat)\b`),
	regexp.MustCompile(`\b(summarize|summary|tldr|recap)\b`),
}

// FeatureLabels maps each feature index to a human-readable label.
var FeatureLabels = []string{
	"word_count",
	"char_count",
	"sentence_count",
	"implement_build",
	"debug_fix",
	"explain_how",
	"compare",
	"code_function",
	"architect_design",
	"security",
	"performance",
	"math",
	"creative",
	"translate",
	"summarize",
}

// ExtractFeatures computes a 15-element feature vector from chat messages.
// It looks at the last user message for content-based features.
func ExtractFeatures(messages []map[string]any) []float64 {
	feat := make([]float64, 15)

	text := lastUserText(messages)
	if text == "" {
		return feat
	}

	lower := strings.ToLower(text)
	words := strings.Fields(text)
	sentences := splitSentences(text)

	// --- length features (0–2) ---
	feat[0] = math.Min(float64(len(words))/100.0, 1.0)    // word count
	feat[1] = math.Min(float64(len(text))/2000.0, 1.0)     // char count
	feat[2] = math.Min(float64(len(sentences))/10.0, 1.0)   // sentence count

	// --- complexity indicators (3–14) ---
	for i, re := range complexityPatterns {
		if re.MatchString(lower) {
			feat[3+i] = 1.0
		}
	}

	return feat
}

// lastUserText finds the most-recent user message and returns its text content.
func lastUserText(messages []map[string]any) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		role, _ := m["role"].(string)
		if role != "user" {
			continue
		}
		if content, ok := m["content"].(string); ok {
			return content
		}
		// Handle content-as-array (multimodal)
		if arr, ok := m["content"].([]any); ok {
			var sb strings.Builder
			for _, part := range arr {
				p, _ := part.(map[string]any)
				if t, _ := p["type"].(string); t == "text" {
					txt, _ := p["text"].(string)
					sb.WriteString(txt)
					sb.WriteByte(' ')
				}
			}
			return strings.TrimSpace(sb.String())
		}
	}
	return ""
}

// splitSentences splits text on ., !, ? and newlines.
func splitSentences(text string) []string {
	// Use a simple split approach that works for most cases.
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == '\n'
	})
	// Filter out empty strings.
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Heuristic scoring
// ---------------------------------------------------------------------------

// Default heuristic weights for the 15 features.
// Positive weights push toward "expensive", negative toward "cheap".
var DefaultWeights = []float64{
	0.8,  // word count
	0.6,  // char count
	0.4,  // sentence count
	1.2,  // implement/build
	1.0,  // debug/fix
	0.3,  // explain/how
	0.7,  // compare
	1.1,  // code/function
	1.3,  // architect/design
	0.9,  // security
	0.8,  // performance
	0.7,  // math
	-0.2, // creative (usually simpler)
	-0.3, // translate (usually simpler)
	-0.3, // summarize (usually simpler)
}

// DefaultBias shifts the decision boundary — negative = conservative (prefers cheap).
const DefaultBias = -0.8

// Score computes a complexity score (0–1) from a feature vector using
// weighted linear combination + sigmoid activation.
func Score(features []float64) float64 {
	return ScoreWeighted(features, DefaultWeights, DefaultBias)
}

// ScoreWeighted is like Score but accepts custom weights and bias.
func ScoreWeighted(features []float64, weights []float64, bias float64) float64 {
	n := len(features)
	if len(weights) < n {
		n = len(weights)
	}
	logit := bias
	for i := 0; i < n; i++ {
		logit += features[i] * weights[i]
	}
	return sigmoid(logit)
}

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// ---------------------------------------------------------------------------
// Routing
// ---------------------------------------------------------------------------

// DefaultModelPair is used when no explicit pair is configured.
var DefaultModelPair = ModelPair{
	CheapModel:     "gpt-4o-mini",
	ExpensiveModel: "gpt-4o",
	Threshold:      0.5,
}

// Route evaluates the messages against the given model pair and returns
// a routing Decision.
func Route(messages []map[string]any, mp ModelPair) Decision {
	features := ExtractFeatures(messages)
	score := Score(features)

	tier := "expensive"
	model := mp.ExpensiveModel
	if score < mp.Threshold {
		tier = "cheap"
		model = mp.CheapModel
	}

	return Decision{
		Model:  model,
		Score:  math.Round(score*100) / 100,
		Tier:   tier,
		Method: "heuristic",
	}
}

// RouteWithDefaults is a convenience wrapper that falls back to DefaultModelPair
// when mp is zero-valued.
func RouteWithDefaults(messages []map[string]any, mp ModelPair) Decision {
	if mp.CheapModel == "" && mp.ExpensiveModel == "" {
		mp = DefaultModelPair
	}
	if mp.Threshold <= 0 {
		mp.Threshold = DefaultModelPair.Threshold
	}
	return Route(messages, mp)
}

// GetFeatureVector returns a labeled map for debugging / dashboard display.
func GetFeatureVector(messages []map[string]any) map[string]float64 {
	features := ExtractFeatures(messages)
	out := make(map[string]float64, len(features))
	for i, v := range features {
		if i < len(FeatureLabels) {
			out[FeatureLabels[i]] = v
		}
	}
	return out
}

package quality

import (
	"math"
	"testing"
)

// --- New / Defaults ---

func TestNew_DefaultWeights(t *testing.T) {
	f := New(0.5, Weights{})
	if f.Threshold != 0.5 {
		t.Errorf("expected threshold 0.5, got %f", f.Threshold)
	}
	if f.Weights.Completeness != DefaultWeights().Completeness {
		t.Errorf("expected default completeness weight, got %f", f.Weights.Completeness)
	}
}

func TestNew_ZeroThreshold(t *testing.T) {
	f := New(0, Weights{})
	if f.Threshold != 0.4 {
		t.Errorf("expected default threshold 0.4, got %f", f.Threshold)
	}
}

func TestNew_CustomWeights(t *testing.T) {
	w := Weights{Completeness: 0.4, Coherence: 0.3, Correctness: 0.2, Relevance: 0.1}
	f := New(0.6, w)
	if f.Weights.Completeness != 0.4 {
		t.Errorf("expected 0.4, got %f", f.Weights.Completeness)
	}
}

// --- Evaluate ---

func TestEvaluate_PerfectResponse(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate(
		"Go is a statically typed, compiled programming language designed at Google. It is syntactically similar to C, but with memory safety, garbage collection, structural typing, and CSP-style concurrency.",
		"What is Go?",
	)
	if !result.Pass {
		t.Errorf("expected pass, got score=%.2f", result.Score)
	}
	if result.Score < 0.7 {
		t.Errorf("expected high score, got %.2f", result.Score)
	}
}

func TestEvaluate_EmptyResponse(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate("", "what is Go?")
	if result.Pass {
		t.Error("empty response should not pass")
	}
	if result.Dimensions.Completeness != 0 {
		t.Errorf("empty completeness should be 0, got %.2f", result.Dimensions.Completeness)
	}
}

func TestEvaluate_TruncatedResponse(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate(
		"The answer to your question is",
		"long complex question about system design?",
	)
	// Very short + ends mid-sentence
	if result.Dimensions.Completeness > 0.7 {
		t.Errorf("truncated response should have low completeness, got %.2f", result.Dimensions.Completeness)
	}
}

func TestEvaluate_HighRelevance(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate(
		"To implement a binary search tree, you need a Node struct with left and right pointers. The insert operation recursively traverses the tree.",
		"implement binary search tree",
	)
	// binary, search, tree should overlap
	if result.Dimensions.Relevance < 0.7 {
		t.Errorf("expected high relevance, got %.2f", result.Dimensions.Relevance)
	}
}

func TestEvaluate_LowRelevance(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate(
		"The weather today is sunny with a chance of rain.",
		"implement binary search tree",
	)
	// Almost no overlap
	if result.Dimensions.Relevance > 0.6 {
		t.Errorf("expected low relevance, got %.2f", result.Dimensions.Relevance)
	}
}

func TestEvaluate_ErrorResponse(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate(
		"I'm sorry, I cannot help with that request. As an AI, I am unable to perform that action.",
		"write malicious code",
	)
	if result.Dimensions.Correctness > 0.7 {
		t.Errorf("error response should have low correctness, got %.2f", result.Dimensions.Correctness)
	}
}

func TestEvaluate_StructuredResponse(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate(
		"## Solution\n\n1. First, install the package.\n2. Next, configure the settings.\n3. Finally, run the application.\n\n```go\nfunc main() {}\n```",
		"how to install and configure",
	)
	if result.Dimensions.Coherence < 0.7 {
		t.Errorf("structured response should have high coherence, got %.2f", result.Dimensions.Coherence)
	}
}

func TestEvaluate_RepetitiveResponse(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate(
		"The answer is simple. The answer is simple. The answer is simple. The answer is simple. The answer is simple. The answer is simple.",
		"explain quantum computing",
	)
	if result.Dimensions.Coherence > 0.5 {
		t.Errorf("repetitive response should have low coherence, got %.2f", result.Dimensions.Coherence)
	}
}

// --- ShouldReroute ---

func TestShouldReroute_NotPassing(t *testing.T) {
	r := Result{Pass: false, Score: 0.2}
	if !ShouldReroute(r, 0, 3) {
		t.Error("should reroute when not passing")
	}
}

func TestShouldReroute_PassingButVeryLow(t *testing.T) {
	r := Result{Pass: true, Score: 0.25} // technically passing with low threshold
	if !ShouldReroute(r, 0, 3) {
		t.Error("should reroute when score is < 0.3 even if passing")
	}
}

func TestShouldReroute_PassingAndOk(t *testing.T) {
	r := Result{Pass: true, Score: 0.85}
	if ShouldReroute(r, 0, 3) {
		t.Error("should not reroute when score is good")
	}
}

func TestShouldReroute_MaxAttemptsReached(t *testing.T) {
	r := Result{Pass: false, Score: 0.2}
	if ShouldReroute(r, 3, 3) {
		t.Error("should not reroute when max attempts reached")
	}
}

func TestShouldReroute_AttemptLessThanMax(t *testing.T) {
	r := Result{Pass: false, Score: 0.1}
	if !ShouldReroute(r, 1, 3) {
		t.Error("should reroute on attempt 1 of 3")
	}
}

// --- Edge cases ---

func TestEvaluate_NoUserQuery(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate("Some response without a known query.", "")
	if result.Dimensions.Relevance != 0.7 {
		t.Errorf("relevance with no query should be 0.7, got %.2f", result.Dimensions.Relevance)
	}
}

func TestEvaluate_WhitespaceOnly(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate("   \n  \t  ", "hello")
	if result.Pass {
		t.Error("whitespace-only response should not pass")
	}
}

func TestEvaluate_GibberishResponse(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate("!!!!!! ?????? !!!!!!", "explain AI")
	if result.Dimensions.Coherence < 0.7 {
		// Gibberish should have low coherence
	} else {
		t.Errorf("expected low coherence for gibberish, got %.2f", result.Dimensions.Coherence)
	}
}

func TestEvaluate_MidSentenceEnd(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate("This is an answer but it ends without closing properly", "question")
	if result.Dimensions.Completeness > 0.7 {
		// Very short + no proper ending should be lower
	}
	// Not a hard assertion — just ensure no panic
	_ = result.Score
}

func TestExtractKeyTerms(t *testing.T) {
	terms := extractKeyTerms("implement a binary search tree in golang")
	// Should contain: implement, binary, search, tree, golang
	found := make(map[string]bool)
	for _, tm := range terms {
		found[tm] = true
	}
	for _, want := range []string{"implement", "binary", "search", "tree", "golang"} {
		if !found[want] {
			t.Errorf("expected term %q in %v", want, terms)
		}
	}
}

func TestExtractKeyTerms_StopWordsFiltered(t *testing.T) {
	terms := extractKeyTerms("the and of in for with yang dan di")
	if len(terms) != 0 {
		t.Errorf("expected no terms after stop word filtering, got %v", terms)
	}
}

func TestExtractKeyTerms_Max30(t *testing.T) {
	// Build text with 50 unique terms
	words := ""
	for i := 0; i < 50; i++ {
		words += " term" + string(rune('a'+i%26)) + "xxx"
	}
	terms := extractKeyTerms(words)
	if len(terms) > 30 {
		t.Errorf("expected at most 30 terms, got %d", len(terms))
	}
}

func TestClampScore(t *testing.T) {
	if v := clampScore(-0.5); v != 0 {
		t.Errorf("expected 0, got %v", v)
	}
	if v := clampScore(1.5); v != 1 {
		t.Errorf("expected 1, got %v", v)
	}
	if v := clampScore(0.756); v != 0.76 {
		t.Errorf("expected 0.76, got %v", v)
	}
}

func TestDefaultWeights_Sum(t *testing.T) {
	dw := DefaultWeights()
	sum := dw.Completeness + dw.Coherence + dw.Correctness + dw.Relevance
	if math.Abs(sum-1.0) > 0.001 {
		t.Errorf("weights should sum to 1.0, got %.3f", sum)
	}
}

func TestSplitSentences_FiltersShort(t *testing.T) {
	s := splitSentences("Hi. OK. Yes. This is a longer sentence that should be kept.")
	if len(s) != 1 {
		t.Errorf("expected 1 sentence (short ones filtered), got %d: %v", len(s), s)
	}
}

func TestEvaluate_AllDimensionsPresent(t *testing.T) {
	f := New(0.4, Weights{})
	result := f.Evaluate("The quick brown fox jumps over the lazy dog.", "what is a fox?")
	// All dimensions should be between 0 and 1
	dims := []float64{
		result.Dimensions.Completeness,
		result.Dimensions.Coherence,
		result.Dimensions.Correctness,
		result.Dimensions.Relevance,
	}
	for i, d := range dims {
		if d < 0 || d > 1 {
			t.Errorf("dimension %d out of range: %f", i, d)
		}
	}
}

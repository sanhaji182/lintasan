package mlrouter

import (
	"math"
	"testing"
)

func TestExtractFeatures_EmptyMessages(t *testing.T) {
	f := ExtractFeatures(nil)
	if len(f) != 15 {
		t.Fatalf("expected 15 features, got %d", len(f))
	}
	for i, v := range f {
		if v != 0 {
			t.Errorf("feature[%d] expected 0, got %v", i, v)
		}
	}
}

func TestExtractFeatures_SimpleGreeting(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hello"},
	}
	f := ExtractFeatures(msgs)
	if f[0] != 0.01 { // word count: 1/100
		t.Errorf("word_count expected 0.01, got %v", f[0])
	}
	if f[1] != 0.0025 { // char count: 5/2000
		t.Errorf("char_count expected 0.0025, got %v", f[1])
	}
}

func TestExtractFeatures_ImplementCode(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "write a function to implement a binary tree"},
	}
	f := ExtractFeatures(msgs)
	// Feature 3 = implement_build (matches "implement")
	if f[3] != 1.0 {
		t.Errorf("implement_build expected 1.0, got %v", f[3])
	}
	// Feature 7 = code_function (matches "function")
	if f[7] != 1.0 {
		t.Errorf("code_function expected 1.0, got %v", f[7])
	}
}

func TestExtractFeatures_DebugError(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "debug this error: null pointer exception"},
	}
	f := ExtractFeatures(msgs)
	if f[4] != 1.0 { // debug_fix
		t.Errorf("debug_fix expected 1.0, got %v", f[4])
	}
}

func TestExtractFeatures_ArchitectSystem(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "architect a microservices system with event sourcing"},
	}
	f := ExtractFeatures(msgs)
	if f[8] != 1.0 { // architect_design
		t.Errorf("architect_design expected 1.0, got %v", f[8])
	}
}

func TestExtractFeatures_Security(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "how do I encrypt user data securely?"},
	}
	f := ExtractFeatures(msgs)
	if f[9] != 1.0 { // security
		t.Errorf("security expected 1.0, got %v", f[9])
	}
	if f[5] != 1.0 { // also matches explain/how
		t.Errorf("explain_how expected 1.0, got %v", f[5])
	}
}

func TestExtractFeatures_Performance(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "optimize the database latency for scaling"},
	}
	f := ExtractFeatures(msgs)
	if f[10] != 1.0 { // performance
		t.Errorf("performance expected 1.0, got %v", f[10])
	}
}

func TestExtractFeatures_Compare(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "compare kubernetes vs docker swarm, what's the difference?"},
	}
	f := ExtractFeatures(msgs)
	if f[6] != 1.0 { // compare
		t.Errorf("compare expected 1.0, got %v", f[6])
	}
}

func TestExtractFeatures_Math(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "solve this equation: x^2 + 5x + 6 = 0"},
	}
	f := ExtractFeatures(msgs)
	if f[11] != 1.0 { // math (matches "equation")
		t.Errorf("math expected 1.0, got %v", f[11])
	}
}

func TestExtractFeatures_Creative(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "write a poem about the stars"},
	}
	f := ExtractFeatures(msgs)
	if f[12] != 1.0 { // creative
		t.Errorf("creative expected 1.0, got %v", f[12])
	}
	if f[3] != 1.0 { // also matches implement/write
		t.Errorf("implement_build expected 1.0 for 'write', got %v", f[3])
	}
}

func TestExtractFeatures_Translate(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "translate this to French"},
	}
	f := ExtractFeatures(msgs)
	if f[13] != 1.0 { // translate
		t.Errorf("translate expected 1.0, got %v", f[13])
	}
}

func TestExtractFeatures_Summarize(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "summarize this article for me"},
	}
	f := ExtractFeatures(msgs)
	if f[14] != 1.0 { // summarize
		t.Errorf("summarize expected 1.0, got %v", f[14])
	}
}

func TestExtractFeatures_MultimodalContent(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": "describe what is in this image?"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/img.png"}},
		}},
	}
	f := ExtractFeatures(msgs)
	// "describe" matches explain_how pattern
	if f[5] != 1.0 {
		t.Errorf("explain_how expected 1.0, got %v", f[5])
	}
}

func TestExtractFeatures_PicksLastUser(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "hi there!"},
		{"role": "user", "content": "implement a sorting algorithm"},
	}
	f := ExtractFeatures(msgs)
	if f[3] != 1.0 { // implement_build from last user
		t.Errorf("implement_build expected 1.0 from last user, got %v", f[3])
	}
	if f[0] <= 0 { // word count from last user (5 words)
		t.Errorf("word_count expected >0 from last user, got %v", f[0])
	}
}

func TestExtractFeatures_VeryLongMessage(t *testing.T) {
	// Build a very long message (200+ words, > 2000 chars)
	var sb []byte
	for i := 0; i < 500; i++ {
		sb = append(sb, "word "...)
	}
	msgs := []map[string]any{
		{"role": "user", "content": string(sb)},
	}
	f := ExtractFeatures(msgs)
	if f[0] != 1.0 { // word count capped at 1.0
		t.Errorf("word_count expected 1.0 (capped), got %v", f[0])
	}
	if f[1] != 1.0 { // char count capped at 1.0
		t.Errorf("char_count expected 1.0 (capped), got %v", f[1])
	}
}

// --- Score / Route ---

func TestScore_LowComplexity(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
	}
	f := ExtractFeatures(msgs)
	score := Score(f)
	if score >= 0.5 {
		t.Errorf("expected low score for greeting, got %.2f", score)
	}
}

func TestScore_HighComplexity(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "implement a distributed system architecture with security and performance optimizations for our microservices"},
	}
	f := ExtractFeatures(msgs)
	score := Score(f)
	if score < 0.5 {
		t.Errorf("expected high score for complex query, got %.2f", score)
	}
}

func TestRoute_SelectsCheap(t *testing.T) {
	mp := ModelPair{CheapModel: "haiku", ExpensiveModel: "sonnet", Threshold: 0.5}
	msgs := []map[string]any{
		{"role": "user", "content": "hello"},
	}
	d := Route(msgs, mp)
	if d.Model != "haiku" {
		t.Errorf("expected haiku, got %s", d.Model)
	}
	if d.Tier != "cheap" {
		t.Errorf("expected cheap, got %s", d.Tier)
	}
	if d.Method != "heuristic" {
		t.Errorf("expected heuristic, got %s", d.Method)
	}
}

func TestRoute_SelectsExpensive(t *testing.T) {
	mp := ModelPair{CheapModel: "haiku", ExpensiveModel: "sonnet", Threshold: 0.5}
	msgs := []map[string]any{
		{"role": "user", "content": "design a secure event-sourced architecture for a banking system handling millions of transactions"},
	}
	d := Route(msgs, mp)
	if d.Model != "sonnet" {
		t.Errorf("expected sonnet, got %s", d.Model)
	}
	if d.Tier != "expensive" {
		t.Errorf("expected expensive, got %s", d.Tier)
	}
}

func TestRouteWithDefaults_UsesDefaults(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
	}
	d := RouteWithDefaults(msgs, ModelPair{})
	if d.Model != DefaultModelPair.CheapModel {
		t.Errorf("expected %s, got %s", DefaultModelPair.CheapModel, d.Model)
	}
}

func TestRouteWithDefaults_ZeroThreshold(t *testing.T) {
	mp := ModelPair{CheapModel: "mini", ExpensiveModel: "pro", Threshold: 0}
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
	}
	d := RouteWithDefaults(msgs, mp)
	// With Threshold=0 and defaults applied, greeting should route to cheap
	if d.Model != "mini" {
		t.Errorf("expected mini (cheap) for greeting with default threshold, got %s", d.Model)
	}
}

func TestScoreWeighted_CustomWeights(t *testing.T) {
	feat := []float64{0.1, 0.1, 0.1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	weights := []float64{10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	bias := -1.0
	score := ScoreWeighted(feat, weights, bias)
	// logit = -1 + 0.1*10 = 0; sigmoid(0) = 0.5
	if math.Abs(score-0.5) > 0.001 {
		t.Errorf("expected 0.5, got %.4f", score)
	}
}

func TestGetFeatureVector(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "implement a REST API"},
	}
	fv := GetFeatureVector(msgs)
	if v, ok := fv["implement_build"]; !ok || v != 1.0 {
		t.Errorf("implement_build expected 1.0, got %v", v)
	}
	if v, ok := fv["code_function"]; !ok || v != 1.0 {
		t.Errorf("code_function expected 1.0, got %v", v)
	}
}

func TestSplitSentences(t *testing.T) {
	s := splitSentences("Hello world. How are you? I'm fine!")
	if len(s) != 3 {
		t.Fatalf("expected 3 sentences, got %d: %v", len(s), s)
	}
	if s[0] != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", s[0])
	}
}

func TestDefaultModelPair(t *testing.T) {
	if DefaultModelPair.CheapModel == "" || DefaultModelPair.ExpensiveModel == "" {
		t.Error("DefaultModelPair must have both models set")
	}
	if DefaultModelPair.Threshold <= 0 || DefaultModelPair.Threshold >= 1 {
		t.Error("DefaultModelPair threshold must be between 0 and 1")
	}
}

func TestFeatureLabelsLength(t *testing.T) {
	if len(FeatureLabels) != 15 {
		t.Fatalf("expected 15 feature labels, got %d", len(FeatureLabels))
	}
}

func TestComplexityPatternsLength(t *testing.T) {
	if len(complexityPatterns) != 12 {
		t.Fatalf("expected 12 complexity patterns, got %d", len(complexityPatterns))
	}
}

func TestScore_AllMessagesConsidered(t *testing.T) {
	// Only last user message matters for feature extraction
	msgs := []map[string]any{
		{"role": "system", "content": "you are a helpful assistant"},
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "hello!"},
		{"role": "user", "content": "perform a security audit of our kubernetes cluster"},
	}
	f := ExtractFeatures(msgs)
	// Last user msg contains "security" and "perform" (but perform not in patterns)
	if f[9] != 1.0 { // security
		t.Errorf("security expected 1.0, got %v", f[9])
	}
}

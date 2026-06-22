package reflect

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestReflectPerfectFirstTry(t *testing.T) {
	gen := func(prompt string, prevErrors []string) (string, error) {
		return "```python\ndef add(a, b): return a + b\n```", nil
	}
	ver := func(output string) VerifyResult {
		return VerifyResult{Score: 1.0, Passed: 10, Total: 10}
	}

	result, err := Reflect(context.Background(), 3, gen, ver, "write an add function", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
	if result.BestScore != 1.0 {
		t.Errorf("expected best score 1.0, got %f", result.BestScore)
	}
	if len(result.Attempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(result.Attempts))
	}
}

func TestReflectMaxIterations(t *testing.T) {
	calls := 0
	gen := func(prompt string, prevErrors []string) (string, error) {
		calls++
		return "```python\ndef broken(): return 1/0\n```", nil
	}
	ver := func(output string) VerifyResult {
		return VerifyResult{Score: 0.3, Passed: 3, Total: 10, Errors: []string{"test_a", "test_b"}}
	}

	result, err := Reflect(context.Background(), 3, gen, ver, "write code", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 generator calls, got %d", calls)
	}
	if result.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", result.Iterations)
	}
	if len(result.Attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(result.Attempts))
	}
}

func TestReflectBestScore(t *testing.T) {
	scores := []float64{0.3, 0.7, 0.5}
	callIdx := 0
	gen := func(prompt string, prevErrors []string) (string, error) {
		s := scores[callIdx]
		callIdx++
		return fmt.Sprintf("response with score %f", s), nil
	}
	ver := func(output string) VerifyResult {
		// return score based on call index (0-indexed by output)
		for i, s := range scores {
			if strings.Contains(output, fmt.Sprintf("%f", s)) {
				return VerifyResult{Score: s, Passed: int(s * 10), Total: 10}
			}
			_ = i
		}
		return VerifyResult{Score: 0, Passed: 0, Total: 10}
	}

	result, err := Reflect(context.Background(), 3, gen, ver, "write code", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BestScore != 0.7 {
		t.Errorf("expected best score 0.7, got %f", result.BestScore)
	}
}

func TestReflectContextInjection(t *testing.T) {
	// Test that error messages from previous iteration ARE injected into next generator call
	var capturedErrors [][]string
	gen := func(prompt string, prevErrors []string) (string, error) {
		capturedErrors = append(capturedErrors, prevErrors)
		return "```python\ndef foo(): pass\n```", nil
	}

	iteration := 0
	ver := func(output string) VerifyResult {
		iteration++
		if iteration == 1 {
			// First iteration: partial score with specific errors
			return VerifyResult{
				Score:  0.5,
				Passed: 5,
				Total:  10,
				Errors: []string{"test_foo failed: expected 5 got 3", "test_bar failed: time out"},
			}
		}
		// Second iteration: still not perfect
		return VerifyResult{
			Score:  0.8,
			Passed: 8,
			Total:  10,
			Errors: []string{"test_baz failed: off by one"},
		}
	}

	result, err := Reflect(context.Background(), 3, gen, ver, "write code", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First call: prevErrors should be nil/empty
	if len(capturedErrors) < 1 {
		t.Fatal("expected at least 1 generator call")
	}
	if capturedErrors[0] != nil && len(capturedErrors[0]) > 0 {
		t.Logf("first call prevErrors: %v (expected nil or empty)", capturedErrors[0])
	}

	// Second call: prevErrors should contain the errors from verifier's first call
	if len(capturedErrors) < 2 {
		t.Fatalf("expected 2 generator calls, got %d", len(capturedErrors))
	}

	foundFoo := false
	foundBar := false
	for _, e := range capturedErrors[1] {
		if strings.Contains(e, "test_foo") {
			foundFoo = true
		}
		if strings.Contains(e, "test_bar") {
			foundBar = true
		}
	}
	if !foundFoo || !foundBar {
		t.Errorf("prevErrors[1] should contain test_foo and test_bar errors, got: %v", capturedErrors[1])
	}

	// Build fix prompt should contain the errors
	if len(result.Attempts) >= 2 {
		a2 := result.Attempts[1]
		if !strings.Contains(a2.Prompt, "test_foo") {
			t.Errorf("fix prompt should reference test_foo error, got: %s", a2.Prompt)
		}
		if !strings.Contains(a2.Prompt, "test_bar") {
			t.Errorf("fix prompt should reference test_bar error, got: %s", a2.Prompt)
		}
	}

	_ = result
}

func TestPytestVerifierPassing(t *testing.T) {
	// Create temp test file and module file
	testFile, err := os.CreateTemp("", "test_reflect_pass_*.py")
	if err != nil {
		t.Fatalf("create temp test file: %v", err)
	}
	defer os.Remove(testFile.Name())

	modFile, err := os.CreateTemp("", "reflect_pass_module_*.py")
	if err != nil {
		t.Fatalf("create temp module file: %v", err)
	}
	modPath := modFile.Name()
	modFile.Close()
	defer os.Remove(modPath)

	// Write a valid pytest test
	modBase := strings.TrimSuffix(modPath, ".py")
	importName := modBase[strings.LastIndex(modBase, "/")+1:]
	testContent := fmt.Sprintf(`import sys
sys.path.insert(0, '/tmp')
import %s

def test_add():
    assert %s.add(2, 3) == 5

def test_subtract():
    assert %s.subtract(5, 3) == 2
`, importName, importName, importName)
	if err := os.WriteFile(testFile.Name(), []byte(testContent), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Create verifier
	verifier := NewPytestVerifier(testFile.Name(), modPath)

	// Generate valid Python code
	code := "```python\ndef add(a, b): return a + b\ndef subtract(a, b): return a - b\n```"
	result := verifier(code)

	if result.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f (output: %s)", result.Score, result.Output)
	}
	if result.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", result.Passed)
	}
}

func TestPytestVerifierFailing(t *testing.T) {
	testFile, err := os.CreateTemp("", "test_reflect_fail_*.py")
	if err != nil {
		t.Fatalf("create temp test file: %v", err)
	}
	defer os.Remove(testFile.Name())

	modFile, err := os.CreateTemp("", "reflect_fail_module_*.py")
	if err != nil {
		t.Fatalf("create temp module file: %v", err)
	}
	modPath := modFile.Name()
	modFile.Close()
	defer os.Remove(modPath)

	modBase := strings.TrimSuffix(modPath, ".py")
	importName := modBase[strings.LastIndex(modBase, "/")+1:]
	testContent := fmt.Sprintf(`import sys
sys.path.insert(0, '/tmp')
import %s

def test_add():
    assert %s.add(2, 3) == 5

def test_subtract():
    assert %s.subtract(5, 3) == 2
`, importName, importName, importName)
	if err := os.WriteFile(testFile.Name(), []byte(testContent), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	verifier := NewPytestVerifier(testFile.Name(), modPath)

	// Generate buggy Python code (subtract is wrong)
	code := "```python\ndef add(a, b): return a + b\ndef subtract(a, b): return a * b\n```"
	result := verifier(code)

	if result.Score >= 1.0 {
		t.Errorf("expected score < 1.0, got %f (output: %s)", result.Score, result.Output)
	}
	if result.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", result.Passed)
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least 1 error")
	}
}

func TestPytestVerifierNoCodeBlock(t *testing.T) {
	verifier := NewPytestVerifier("/tmp/nonexistent_test.py", "/tmp/nonexistent_module.py")
	result := verifier("this is just plain text with no code blocks")
	if result.Score != 0 {
		t.Errorf("expected score 0, got %f", result.Score)
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors for no code block")
	}
}

func TestExtractCodeBlock(t *testing.T) {
	// Python block
	text := "some text\n```python\ndef foo(): pass\n```\nmore text"
	code := ExtractCodeBlock(text, "python")
	if code != "def foo(): pass\n" {
		t.Errorf("expected 'def foo(): pass\\n', got %q", code)
	}

	// No-lang block
	text2 := "text\n```\nprint('hello')\n```\nend"
	code2 := ExtractCodeBlock(text2, "")
	if code2 != "print('hello')\n" {
		t.Errorf("expected 'print('hello')\\n', got %q", code2)
	}

	// Empty text
	code3 := ExtractCodeBlock("", "python")
	if code3 != "" {
		t.Errorf("expected empty, got %q", code3)
	}

	// Multiple blocks — should pick longest
	text4 := "```python\nshort\n```\n```python\nthis is the longer block\n```"
	code4 := ExtractCodeBlock(text4, "python")
	if code4 != "this is the longer block\n" {
		t.Errorf("expected longest block, got %q", code4)
	}
}

func TestBuildFixPrompt(t *testing.T) {
	vr := VerifyResult{
		Score:  0.7,
		Passed: 7,
		Total:  10,
		Errors: []string{"test_foo.py::test_add", "test_foo.py::test_subtract", "test_foo.py::test_mul"},
	}
	prompt := buildFixPrompt("write code", "def add(a,b): return a+b", vr)

	if !strings.Contains(prompt, "70%") {
		t.Errorf("expected '70%%' in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, "7/10") {
		t.Errorf("expected '7/10' in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, "test_add") {
		t.Errorf("expected 'test_add' in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, "Fix ALL failing tests") {
		t.Errorf("expected 'Fix ALL failing tests' in prompt, got: %s", prompt)
	}
}

func TestTruncateStr(t *testing.T) {
	// Short
	if s := truncateStr("hello", 10); s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}

	// Long
	long := strings.Repeat("a", 100)
	if s := truncateStr(long, 10); s != "aaaaaaaaaa...[truncated]" {
		t.Errorf("expected truncated string, got %q", s)
	}

	// Exact boundary
	if s := truncateStr("hello", 5); s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}
}

func TestSyntaxVerifierValidPython(t *testing.T) {
	verifier := NewSyntaxVerifier()

	// Valid Python code
	code := "```python\ndef add(a, b): return a + b\n```"
	result := verifier(code)

	if result.Score != 1.0 {
		t.Errorf("expected score 1.0 for valid Python, got %f (output: %s)", result.Score, result.Output)
	}
	if result.Passed != 1 {
		t.Errorf("expected 1 passed for valid Python, got %d", result.Passed)
	}
}

func TestSyntaxVerifierInvalidPython(t *testing.T) {
	verifier := NewSyntaxVerifier()

	// Invalid Python syntax
	code := "```python\ndef broken( a b c ):\n  return\n```"
	result := verifier(code)

	if result.Score != 0 {
		t.Errorf("expected score 0 for invalid Python, got %f", result.Score)
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least 1 error for invalid Python")
	}
}

func TestSyntaxVerifierNoCodeBlock(t *testing.T) {
	verifier := NewSyntaxVerifier()

	result := verifier("this is just plain text with no code blocks")
	if result.Score != 0 {
		t.Errorf("expected score 0 for no code block, got %f", result.Score)
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors for no code block")
	}
}

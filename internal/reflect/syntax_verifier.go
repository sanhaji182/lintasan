package reflect

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// NewSyntaxVerifier creates a Verifier that checks Python syntax.
// Returns score = 1.0 if valid Python, 0.0 with error message if invalid.
func NewSyntaxVerifier() Verifier {
	return func(output string) VerifyResult {
		// Extract code from response (try ```python block first)
		code := ExtractCodeBlock(output, "python")
		if code == "" {
			code = ExtractCodeBlock(output, "")
		}
		if code == "" {
			return VerifyResult{Score: 0, Errors: []string{"no code block found in response"}}
		}

		// Write to temp file
		tmpFile, err := os.CreateTemp("", "reflect_syntax_*.py")
		if err != nil {
			return VerifyResult{Score: 0, Errors: []string{fmt.Sprintf("create temp: %v", err)}}
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.WriteString(code); err != nil {
			tmpFile.Close()
			return VerifyResult{Score: 0, Errors: []string{fmt.Sprintf("write error: %v", err)}}
		}
		tmpFile.Close()

		// Check syntax using python3 -c "compile(...)"
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// python3 -m py_compile compiles and checks syntax
		cmd := exec.CommandContext(ctx, "python3", "-m", "py_compile", tmpPath)
		output2, err := cmd.CombinedOutput()
		outStr := string(output2)

		if err != nil {
			// Parse errors from output
			lines := strings.Split(outStr, "\n")
			var errors []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, tmpPath) {
					errors = append(errors, line)
				}
			}
			if len(errors) == 0 {
				errors = append(errors, fmt.Sprintf("syntax error: %v", err))
			}
			return VerifyResult{Score: 0, Errors: errors, Output: outStr}
		}

		return VerifyResult{Score: 1.0, Passed: 1, Total: 1, Output: "syntax ok"}
	}
}

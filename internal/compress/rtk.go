package compress

import (
	"regexp"
	"strconv"
	"strings"
)

// RTK-style tool output compression filters.
// Each filter detects and compresses specific tool output formats.

// CompressRTK auto-detects content type and applies the best compression filter.
// Returns compressed text and savings percentage (0-1).
func CompressRTK(content string) (string, float64) {
	if len(content) < 50 {
		return content, 0
	}

	original := len(content)
	compressed := content

	// Auto-detect and apply best filter (order matters!)
	switch {
	case isGitDiff(content):
		compressed = CompressGitDiff(content)
	case isLogDump(content):
		compressed = CompressLogs(content)
	case isGrepOutput(content):
		compressed = CompressGrep(content)
	case isLsOutput(content):
		compressed = CompressLs(content)
	case isTreeOutput(content):
		compressed = CompressTree(content)
	case isFindOutput(content):
		compressed = CompressFind(content)
	default:
		compressed = CompressGeneric(content)
	}

	// Safe: if compression made it bigger, keep original
	if len(compressed) >= original {
		return content, 0
	}

	savings := 1.0 - float64(len(compressed))/float64(original)
	return compressed, savings
}

// --- Git Diff ---

var (
	diffHeaderRe = regexp.MustCompile(`(?m)^diff --git .+$`)
	diffIndexRe  = regexp.MustCompile(`(?m)^index [a-f0-9]+\.\.[a-f0-9]+.*$`)
	diffChunkRe  = regexp.MustCompile(`(?m)^@@ -\d+(?:,\d+)? \+\d+(?:,\d+)? @@.*$`)
	diffMetaRe   = regexp.MustCompile(`(?m)^(?:old mode|new mode|rename from|rename to|similarity index|deleted file|new file).*$`)
)

func isGitDiff(s string) bool {
	return diffHeaderRe.MatchString(s) || strings.Contains(s, "diff --git")
}

// CompressGitDiff compresses git diff output by removing context lines and redundant metadata.
func CompressGitDiff(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inHunk := false
	removedContext := 0

	for _, line := range lines {
		switch {
		case diffHeaderRe.MatchString(line):
			if removedContext > 0 {
				result = append(result, "  ...")
				removedContext = 0
			}
			result = append(result, line)
			inHunk = false
		case diffIndexRe.MatchString(line):
			continue // Skip index lines
		case diffMetaRe.MatchString(line):
			result = append(result, line)
		case diffChunkRe.MatchString(line):
			if removedContext > 0 {
				result = append(result, "  ...")
				removedContext = 0
			}
			result = append(result, line)
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			result = append(result, line)
		case inHunk && strings.HasPrefix(line, "-"):
			result = append(result, line)
		case inHunk && strings.HasPrefix(line, " "):
			removedContext++
		case inHunk && line == "":
			removedContext++
		default:
			if removedContext > 0 {
				result = append(result, "  ...")
				removedContext = 0
			}
			result = append(result, line)
		}
	}

	if removedContext > 0 {
		result = append(result, "  ...")
	}

	return strings.Join(result, "\n")
}

// --- Grep/RG Output ---

var grepLineRe = regexp.MustCompile(`^(.+?):(\d+):(.*)$`)

func isGrepOutput(s string) bool {
	lines := strings.SplitN(s, "\n", 5)
	matches := 0
	for _, l := range lines {
		if grepLineRe.MatchString(l) {
			matches++
		}
	}
	return matches >= 2
}

// CompressGrep compresses grep/rg output by deduplicating paths and compressing line numbers.
func CompressGrep(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	pathCounts := make(map[string]int)
	var currentPath string

	for _, line := range lines {
		m := grepLineRe.FindStringSubmatch(line)
		if m == nil {
			result = append(result, line)
			continue
		}

		path := m[1]
		lineno := m[2]
		text := m[3]

		if path != currentPath {
			if pathCounts[path] == 0 {
				result = append(result, path+":")
				currentPath = path
			}
			pathCounts[path]++
		}

		// Compact: just line number + content
		result = append(result, "  "+lineno+": "+strings.TrimSpace(text))
	}

	return strings.Join(result, "\n")
}

// --- ls Output ---

func isLsOutput(s string) bool {
	lines := strings.SplitN(s, "\n", 3)
	if len(lines) < 2 {
		return false
	}
	// ls -l format: permissions user group size date name
	lsRe := regexp.MustCompile(`^[d-][rwx-]{9}\s+\d+\s+\w+`)
	for _, l := range lines[:3] {
		if lsRe.MatchString(l) {
			return true
		}
	}
	return false
}

// CompressLs compresses ls output by removing redundant fields.
func CompressLs(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		if line == "" {
			continue
		}
		// Keep filename only for ls -l
		parts := strings.Fields(line)
		if len(parts) >= 9 {
			// ls -l: perms links user group size month day time name
			name := strings.Join(parts[8:], " ")
			if parts[0][0] == 'd' {
				result = append(result, "d "+name+"/")
			} else {
				result = append(result, name+" ("+parts[4]+"B)")
			}
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// --- tree Output ---

func isTreeOutput(s string) bool {
	return strings.Contains(s, "├──") || strings.Contains(s, "└──") ||
		strings.Contains(s, "│   ")
}

// CompressTree compresses tree output by deduplicating similar branches.
func CompressTree(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	seen := make(map[string]int)

	for _, line := range lines {
		// Extract just the name part
		trimmed := strings.TrimLeft(line, "│├└─ ")
		if trimmed == "" {
			continue
		}
		count := seen[trimmed]
		seen[trimmed]++

		if count > 0 {
			if count == 1 {
				// Mark as duplicate
				result = append(result, "  [... "+trimmed+" (repeated)]")
			}
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// --- find Output ---

func isFindOutput(s string) bool {
	lines := strings.SplitN(s, "\n", 5)
	if len(lines) < 3 {
		return false
	}
	// find output: paths starting with ./ or /
	pathRe := regexp.MustCompile(`^[./]`)
	matches := 0
	for _, l := range lines[:5] {
		if pathRe.MatchString(l) {
			matches++
		}
	}
	return matches >= 3
}

// CompressFind compresses find output by grouping common path prefixes.
func CompressFind(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	pathCounts := make(map[string]int)

	for _, line := range lines {
		if line == "" {
			continue
		}
		// Group by directory
		dir := line
		if idx := strings.LastIndex(line, "/"); idx > 0 {
			dir = line[:idx]
		}
		pathCounts[dir]++
	}

	// Show unique paths with counts
	seen := make(map[string]bool)
	for _, line := range lines {
		if line == "" {
			continue
		}
		dir := line
		if idx := strings.LastIndex(line, "/"); idx > 0 {
			dir = line[:idx]
		}

		if !seen[dir] {
			seen[dir] = true
			count := pathCounts[dir]
			if count > 3 {
				result = append(result, dir+"/ ("+itoa(count)+" files)")
			}
		}

		if pathCounts[dir] <= 3 {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}

// --- Log Dumps ---

var logLineRe = regexp.MustCompile(`^\d{4}[-/]\d{2}[-/]\d{2}|\[\d{4}`)

func isLogDump(s string) bool {
	lines := strings.SplitN(s, "\n", 10)
	matches := 0
	for _, l := range lines {
		if logLineRe.MatchString(l) {
			matches++
		}
	}
	return matches >= 5
}

// CompressLogs compresses log dumps by deduplicating repeated lines.
func CompressLogs(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	lineCounts := make(map[string]int)
	var lastLine string
	repeatCount := 0

	for _, line := range lines {
		if line == lastLine {
			repeatCount++
			continue
		}

		if repeatCount > 0 {
			if repeatCount > 2 {
				result = append(result, "  [... repeated "+itoa(repeatCount)+" times]")
			} else {
				for i := 0; i < repeatCount; i++ {
					result = append(result, lastLine)
				}
			}
			repeatCount = 0
		}

		// Deduplicate similar log lines (same prefix)
		prefix := line
		if len(line) > 60 {
			prefix = line[:60]
		}
		count := lineCounts[prefix]
		lineCounts[prefix]++

		if count < 3 {
			result = append(result, line)
		} else if count == 3 {
			result = append(result, "  [... more similar lines suppressed]")
		}

		lastLine = line
	}

	// Handle trailing repeats
	if repeatCount > 0 {
		if repeatCount > 2 {
			result = append(result, "  [... repeated "+itoa(repeatCount)+" times]")
		} else {
			for i := 0; i < repeatCount; i++ {
				result = append(result, lastLine)
			}
		}
	}

	return strings.Join(result, "\n")
}

// --- Generic Compression ---

// CompressGeneric applies basic compression to generic content.
// Removes excessive whitespace and truncates very long output.
func CompressGeneric(content string) string {
	// Collapse multiple blank lines
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")

	// Truncate very long content
	if len(content) > 10000 {
		content = content[:5000] + "\n\n[... truncated " + itoa(len(content)-10000) + " chars ...]\n\n" + content[len(content)-5000:]
	}

	return content
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

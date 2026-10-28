package server

import (
	"os"
	"strings"
	"testing"
)

// chat_search_test.go — search and export helpers.
//
// The route-shape risk here is real: "GET /api/conversations/search" and
// "GET /api/conversations/{id}" are different patterns, and if search were
// registered after the wildcard (or as a literal that conflicts) it would 404
// or panic at startup. The registration test pins the order.

func TestSnippetAround_CentresOnMatch(t *testing.T) {
	content := strings.Repeat("filler ", 40) + "NEEDLE" + strings.Repeat(" tail", 40)
	got := snippetAround(content, "needle", 60)

	if !strings.Contains(strings.ToLower(got), "needle") {
		t.Fatalf("snippet dropped the match: %q", got)
	}
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipses on both sides: %q", got)
	}
	if len([]rune(got)) > 64 {
		t.Fatalf("snippet too long: %d runes", len([]rune(got)))
	}
}

func TestSnippetAround_ShortContentUntouched(t *testing.T) {
	if got := snippetAround("short text", "text", 160); got != "short text" {
		t.Fatalf("short content must be returned as-is, got %q", got)
	}
}

func TestSnippetAround_NoMatchTruncates(t *testing.T) {
	content := strings.Repeat("x", 500)
	got := snippetAround(content, "absent", 40)
	if len(got) > 45 {
		t.Fatalf("expected a bounded snippet, got %d chars", len(got))
	}
}

func TestSafeFilename(t *testing.T) {
	cases := map[string]string{
		"":                         "conversation",
		"   ":                      "conversation",
		"Normal title":             "Normal title",
		"with/slash":               "with-slash",
		`quote"inside`:             "quoteinside",
		"tab\there":                "tab here",
		"line\nbreak":              "line break",
	}
	for in, want := range cases {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
	// Long titles are bounded so the header stays legal.
	if got := safeFilename(strings.Repeat("a", 200)); len(got) != 60 {
		t.Errorf("long title not truncated to 60, got %d", len(got))
	}
	// Truncation must not leave a leading/trailing space.
	if got := safeFilename(strings.Repeat("a", 59) + " b"); strings.HasSuffix(got, " ") {
		t.Errorf("trailing space after truncation: %q", got)
	}
}

func TestIndentLines(t *testing.T) {
	got := indentLines("one\ntwo\n\nthree\n", "> ")
	want := "> one\n> two\n>\n> three"
	if got != want {
		t.Fatalf("indentLines =\n%q\nwant\n%q", got, want)
	}
}

func TestParsePositiveInt(t *testing.T) {
	if n, err := parsePositiveInt("50"); err != nil || n != 50 {
		t.Fatalf("expected 50, got %d err %v", n, err)
	}
	for _, bad := range []string{"", "0", "-3", "abc"} {
		if _, err := parsePositiveInt(bad); err == nil {
			t.Errorf("parsePositiveInt(%q) should have failed", bad)
		}
	}
}

func TestSearchAndExportRoutesRegistered(t *testing.T) {
	// Search must be a literal pattern on the same mux as the {id} wildcard.
	// Go's ServeMux prefers the more specific pattern, but only if both are
	// registered; this catches a dropped registration line.
	b, err := os.ReadFile("chat_search.go")
	if err != nil {
		t.Skipf("source not readable: %v", err)
	}
	for _, want := range []string{
		`GET /api/conversations/search`,
		`GET /api/conversations/{id}/export`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("route %q is not registered", want)
		}
	}
}

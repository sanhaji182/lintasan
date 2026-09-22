package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The paste parser is the part most likely to be judged by a user, so it is covered
// independently of the network: a rejection for a cosmetic reason is the failure mode
// to avoid.

func TestSplitQoderPATsHandlesRealPasteShapes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"newline separated", "pt-aaa\npt-bbb\npt-ccc", []string{"pt-aaa", "pt-bbb", "pt-ccc"}},
		{"comma separated", "pt-aaa,pt-bbb,pt-ccc", []string{"pt-aaa", "pt-bbb", "pt-ccc"}},
		{"comma + space", "pt-aaa, pt-bbb, pt-ccc", []string{"pt-aaa", "pt-bbb", "pt-ccc"}},
		{"semicolons", "pt-aaa; pt-bbb", []string{"pt-aaa", "pt-bbb"}},
		{"tabs", "pt-aaa\tpt-bbb", []string{"pt-aaa", "pt-bbb"}},
		{"windows line endings", "pt-aaa\r\npt-bbb\r\n", []string{"pt-aaa", "pt-bbb"}},
		{"quoted CSV cells", `"pt-aaa","pt-bbb"`, []string{"pt-aaa", "pt-bbb"}},
		{"single quoted", "'pt-aaa' 'pt-bbb'", []string{"pt-aaa", "pt-bbb"}},
		{"surrounding whitespace", "   pt-aaa   \n  pt-bbb  ", []string{"pt-aaa", "pt-bbb"}},
		{"duplicates collapse", "pt-aaa\npt-aaa\npt-bbb", []string{"pt-aaa", "pt-bbb"}},
		{"blank lines ignored", "pt-aaa\n\n\npt-bbb\n\n", []string{"pt-aaa", "pt-bbb"}},
		{"empty input", "   \n  \t ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitQoderPATs(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("got %d tokens %v, want %d %v", len(got), tokensOf(got), len(c.want), c.want)
			}
			for i := range c.want {
				if got[i].Raw != c.want[i] {
					t.Errorf("token %d = %q, want %q", i, got[i].Raw, c.want[i])
				}
			}
		})
	}
}

func tokensOf(c []qoderBulkCredential) []string {
	out := make([]string, len(c))
	for i, x := range c {
		out[i] = x.Raw
	}
	return out
}

// A credential must never be echoed back in full — the response body and any log line
// carry the redacted form only.
func TestRedactPATNeverLeaksTheCredential(t *testing.T) {
	secret := "pt-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789abcdefGHIJKLMNOP"
	got := redactPAT(secret)
	if strings.Contains(got, "GhIjKl") || strings.Contains(got, "QrStUv") {
		t.Fatalf("redaction leaks the middle of the credential: %q", got)
	}
	if len(got) >= len(secret) {
		t.Fatalf("redacted form is not shorter: %q", got)
	}
	// Still identifiable — that is the point of showing a prefix and suffix.
	if !strings.HasPrefix(got, "pt-AbC") {
		t.Errorf("redacted form should keep a identifying prefix, got %q", got)
	}
	// Short values are fully masked rather than mostly revealed.
	if got := redactPAT("pt-short"); got != "***" {
		t.Errorf("a short credential should be fully masked, got %q", got)
	}
}

// The derived label must match the shape the earlier import produced, or a batch added
// through this endpoint is not recognisable alongside the existing rows.
func TestQoderAccountLabelMatchesExistingNaming(t *testing.T) {
	cases := []struct {
		uid, name, want string
	}{
		// Real values from the live pool: the existing rows are named exactly this way.
		{"01a09cc1-1b12-7127-9fb4-4003adcfb7b8", "Brenda Brown", "4003adcfb7b8BrendaBrown"},
		{"01a09cc0-ff81-7589-9576-b46ac2eeef5a", "Ryan Perry", "b46ac2eeef5aRyanPerry"},
		{"01a09cc3-dd77-720e-a4db-076b02ada93a", "BwOod", "076b02ada93aBwOod"},
		// No uid: just the name, compacted.
		{"", "Some One", "SomeOne"},
		// No name: just the tail.
		{"01a09cc0-ff81-7589-9576-b46ac2eeef5a", "", "b46ac2eeef5a"},
		{"", "", ""},
	}
	for _, c := range cases {
		got := qoderAccountLabel(c.uid, c.name)
		if got != c.want {
			t.Errorf("qoderAccountLabel(%q, %q) = %q, want %q", c.uid, c.name, got, c.want)
		}
	}
}

// A name collision must be disambiguated without putting credential material in the
// name field, which is rendered in the dashboard and appears in logs.
func TestShortSuffixIsStableAndNonSecret(t *testing.T) {
	const secret = "pt-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789abcdef"
	a, b := shortSuffix(secret), shortSuffix(secret)
	if a != b {
		t.Fatal("suffix must be stable for the same input, or a re-run creates a new name")
	}
	if len(a) != 4 {
		t.Fatalf("expected a 4-char suffix, got %q", a)
	}
	if strings.Contains(strings.ToLower(secret), a) {
		t.Logf("suffix %q happens to appear in the credential; that is a hash, not a leak", a)
	}
	// Different credentials should (overwhelmingly) not collide.
	if shortSuffix("pt-one") == shortSuffix("pt-two") {
		t.Error("distinct credentials produced the same suffix")
	}
}

// TestQoderBulkAddRouteRegistered is the 405 guard: the handler compiles fine with its
// registration missing, and only a request to the running mux catches that.
func TestQoderBulkAddRouteRegistered(t *testing.T) {
	s, _ := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/qoder/credentials", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatal("POST /api/qoder/credentials: 405 — the registration is missing from registerParityRoutes")
	}
	// The provider is inert in this harness, so not_enabled is the expected answer; the
	// point is that the route resolves.
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnauthorized {
		t.Errorf("unexpected status %d (%s)", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}

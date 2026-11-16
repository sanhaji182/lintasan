package discover

import (
	"testing"

	"github.com/sanhaji182/lintasan-go/internal/db"
)

// The live-network version of this lives in antigravity_models_live_test.go and
// is skipped unless LINTASAN_AG_LIVE_TOKEN is set: it needs a real account token
// and must never run in the default suite.

func TestIsServableAntigravityModel(t *testing.T) {
	cases := map[string]bool{
		"gemini-3.8-flash-high":       true,
		"gemini-2.5-flash":            true,
		"claude-sonnet-4-6":           true,
		"gpt-oss-120b-medium":         true,
		"chat_20706":                  false, // IDE-internal chat scaffolding
		"tab_flash_lite_preview":      false, // editor tab model
		"tab_jump_flash_lite_preview": false,
		"gemini-3.1-flash-image":      false, // image specialist
	}
	for id, want := range cases {
		if got := isServableAntigravityModel(id); got != want {
			t.Errorf("isServableAntigravityModel(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestAntigravityContextWindow(t *testing.T) {
	if got := antigravityContextWindow("claude-opus-4-6-thinking"); got != 200000 {
		t.Errorf("claude window = %d", got)
	}
	if got := antigravityContextWindow("gpt-oss-120b-medium"); got != 128000 {
		t.Errorf("gpt-oss window = %d", got)
	}
	if got := antigravityContextWindow("gemini-3.8-flash-high"); got != 1048576 {
		t.Errorf("gemini window = %d", got)
	}
}

// A connection with no token must fail loudly so the caller keeps the static
// catalogue instead of writing an empty model list over a working one.
func TestFetchAntigravityModels_NoToken(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	d := NewDiscoverer(database)
	if _, err := d.fetchAntigravityModels(map[string]any{"api_key": ""}); err == nil {
		t.Fatal("expected an error for a tokenless connection")
	}
}

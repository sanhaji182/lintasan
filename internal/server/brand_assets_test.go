package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBrandAssetsContract keeps the visible UI, metadata, and distributable
// identity in sync. It deliberately reads source assets because the Go suite is
// the repository's always-on frontend contract runner.
func TestBrandAssetsContract(t *testing.T) {
	root := filepath.Join("..", "..", "frontend")
	static := filepath.Join(root, "static")

	required := []string{
		"lintasan-mark.svg", "lintasan-mark-dark.svg",
		"lintasan-wordmark.svg", "lintasan-wordmark-dark.svg",
		"lintasan-mark-512.png", "favicon.svg", "favicon.ico",
		"favicon-16.png", "favicon-32.png", "favicon-48.png",
		"favicon-192.png", "apple-touch-icon.png", "site.webmanifest",
	}
	for _, name := range required {
		info, err := os.Stat(filepath.Join(static, name))
		if err != nil {
			t.Errorf("required brand asset %s: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("required brand asset %s is empty", name)
		}
	}

	for _, name := range []string{"lintasan-mark.svg", "lintasan-mark-dark.svg", "lintasan-wordmark.svg", "lintasan-wordmark-dark.svg"} {
		body, err := os.ReadFile(filepath.Join(static, name))
		if err != nil {
			continue
		}
		svg := string(body)
		for _, token := range []string{"<title", "<desc", "viewBox="} {
			if !strings.Contains(svg, token) {
				t.Errorf("%s missing accessible/portable SVG token %q", name, token)
			}
		}
	}

	for _, rel := range []string{
		filepath.Join("src", "routes", "+page.svelte"),
		filepath.Join("src", "routes", "login", "+page.svelte"),
		filepath.Join("src", "lib", "components", "Sidebar.svelte"),
	} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(body), "LogoMark") {
			t.Errorf("%s must render the shared LogoMark component", rel)
		}
	}

	app, err := os.ReadFile(filepath.Join(root, "src", "app.html"))
	if err != nil {
		t.Fatalf("read app.html: %v", err)
	}
	for _, token := range []string{"favicon.svg", "favicon.ico", "apple-touch-icon.png", "site.webmanifest", "theme-color"} {
		if !strings.Contains(string(app), token) {
			t.Errorf("app.html missing brand metadata %q", token)
		}
	}
}

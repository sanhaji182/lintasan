package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHopliteCloudAgentFrontendContract(t *testing.T) {
	root := filepath.Join("..", "..", "frontend", "src")
	pagePath := filepath.Join(root, "routes", "dashboard", "experimental", "hoplite", "+page.svelte")
	page, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("read Hoplite page: %v", err)
	}
	body := string(page)
	for _, required := range []string{
		"/api/experimental/cloud-agents/hoplite/status",
		"/api/experimental/cloud-agents/hoplite/test",
		"/api/experimental/cloud-agents/hoplite/projects",
		"/api/experimental/cloud-agents/hoplite/threads",
		"let autoFix = $state(false)",
		"let autoMerge = $state(false)",
		"Automatic PR fixes",
		"Automatic PR merge",
		"selectedThread = null",
		"clientOperationId: pendingOperationId",
		"crypto.randomUUID()",
		"setInterval",
		"clearInterval",
		"Update key",
		"never returned to this browser",
		"Cloud Agent",
		"Isolated from LLM routing",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("Hoplite page missing required contract %q", required)
		}
	}
	if strings.Contains(body, "localStorage.setItem('HOPLITE_API_KEY'") || strings.Contains(body, "X-Api-Key") {
		t.Fatal("Hoplite page must never handle the upstream API key directly")
	}

	experimental, err := os.ReadFile(filepath.Join(root, "routes", "dashboard", "experimental", "+page.svelte"))
	if err != nil {
		t.Fatalf("read Experimental page: %v", err)
	}
	if !strings.Contains(string(experimental), `/dashboard/experimental/hoplite`) {
		t.Fatal("Experimental page must link to Hoplite Cloud Agent")
	}

	layout, err := os.ReadFile(filepath.Join(root, "routes", "dashboard", "+layout.svelte"))
	if err != nil {
		t.Fatalf("read dashboard layout: %v", err)
	}
	if !strings.Contains(string(layout), "'/dashboard/experimental/hoplite': 'Hoplite Cloud Agent'") {
		t.Fatal("dashboard title map missing Hoplite Cloud Agent")
	}

	tabNav, err := os.ReadFile(filepath.Join(root, "lib", "components", "TabNav.svelte"))
	if err != nil {
		t.Fatalf("read TabNav: %v", err)
	}
	tabBody := string(tabNav)
	for _, required := range []string{"function activePath()", "activePath() === t.path", "b.path.length - a.path.length"} {
		if !strings.Contains(tabBody, required) {
			t.Errorf("TabNav missing nested-route active-tab contract %q", required)
		}
	}
}

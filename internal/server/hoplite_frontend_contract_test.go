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
		"Step 1",
		"Connect Hoplite",
		"Connect",
		"Update key",
		"encrypted at rest and never returned to this browser",
		"Step 2",
		"Detect projects",
		"Detecting projects...",
		"Retry detection",
		"Step 3",
		"Ready to use",
		"Connect Hoplite to activate models in Lintasan Proxy.",
		"Copy model ID",
		"Test in Chat",
		"Context window",
		"Catalog eligibility",
		"Available to your account is confirmed only when Hoplite accepts a thread",
		"/dashboard/playground?model=",
		"Ready",
		"Connection diagnostics",
		"if (status.configured) await testConnection();",
		"Last checked",
		"Project default",
		"Custom model ID",
		"Run real agent test",
		"This creates a real Hoplite thread and may use quota",
		"Hoplite exposes no dry-run guarantee",
		"autoFix: false",
		"autoMerge: false",
		"Agent test result",
		"thread-create not tested",
		"Cloud Agent",
		"runs coding-agent tasks",
		"may take several minutes",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("Hoplite page missing required contract %q", required)
		}
	}
	if strings.Contains(body, "localStorage.setItem('HOPLITE_API_KEY'") || strings.Contains(body, "X-Api-Key") {
		t.Fatal("Hoplite page must never handle the upstream API key directly")
	}
	if strings.Contains(body, "error = connectionError") {
		t.Fatal("connection failures must render in diagnostics without a duplicate global banner")
	}
	if strings.Contains(body, "Isolated from LLM routing") {
		t.Fatal("Hoplite onboarding must not describe the adapter as isolated from LLM routing")
	}

	playground, err := os.ReadFile(filepath.Join(root, "routes", "dashboard", "playground", "+page.svelte"))
	if err != nil {
		t.Fatalf("read Playground page: %v", err)
	}
	playgroundBody := string(playground)
	for _, required := range []string{
		"url.searchParams.get('model')",
		"selectedModel.startsWith('hoplite-agent/')",
		"selectedModel.startsWith('hoplite-model/v1/')",
		"stream: !isHopliteModel",
		"await res.json()",
	} {
		if !strings.Contains(playgroundBody, required) {
			t.Errorf("Playground missing Hoplite handoff contract %q", required)
		}
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

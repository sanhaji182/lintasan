package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryDashboardRouteIsReachable guards against orphaned dashboard pages.
//
// A SvelteKit route under frontend/src/routes/dashboard/<name>/ is served by the
// SPA whether or not anything links to it. That makes an unlinked page invisible
// but not broken — the failure mode is silent, and it already happened: the
// /dashboard/migrate page shipped with a full backend (internal/migrate,
// handlers_migrate.go) but no sidebar entry, so it was reachable only by typing
// the URL.
//
// The Go test suite is the only thing that runs on every change here (there is
// no frontend test runner wired up), so the check lives in this package and
// reads the Svelte sources directly.
func TestEveryDashboardRouteIsReachable(t *testing.T) {
	root := filepath.Join("..", "..", "frontend", "src")
	routesDir := filepath.Join(root, "routes", "dashboard")

	entries, err := os.ReadDir(routesDir)
	if err != nil {
		t.Skipf("frontend sources not present (%v) — nothing to check", err)
	}

	sidebar, err := os.ReadFile(filepath.Join(root, "lib", "components", "Sidebar.svelte"))
	if err != nil {
		t.Fatalf("read Sidebar.svelte: %v", err)
	}
	layout, err := os.ReadFile(filepath.Join(routesDir, "+layout.svelte"))
	if err != nil {
		t.Fatalf("read dashboard/+layout.svelte: %v", err)
	}

	// Any page linked from anywhere in the app counts as reachable, not just
	// the sidebar — a route reached from a button on another page is fine.
	linked := map[string]bool{}
	pathRe := regexp.MustCompile(`/dashboard/[a-z0-9-]+`)
	if err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if ext := filepath.Ext(p); ext != ".svelte" && ext != ".ts" {
			return nil
		}
		// A page linking to itself proves nothing about reachability.
		if strings.HasPrefix(p, routesDir+string(filepath.Separator)) {
			rel, _ := filepath.Rel(routesDir, p)
			if seg := strings.Split(rel, string(filepath.Separator))[0]; seg != "+layout.svelte" && seg != "+page.svelte" {
				return nil
			}
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range pathRe.FindAllString(string(body), -1) {
			linked[strings.TrimPrefix(m, "/dashboard/")] = true
		}
		return nil
	}); err != nil {
		t.Fatalf("walk frontend sources: %v", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if _, err := os.Stat(filepath.Join(routesDir, name, "+page.svelte")); err != nil {
			continue // a directory without a page is not a route
		}
		if !linked[name] {
			t.Errorf("dashboard route /dashboard/%s has a +page.svelte but nothing links to it — "+
				"add it to menuItems/manageItems/toolItems in Sidebar.svelte (or link it from another page), "+
				"otherwise it is only reachable by typing the URL", name)
		}
		// A linked page with no title falls back to a blank header.
		if !strings.Contains(string(layout), "'/dashboard/"+name+"'") {
			t.Errorf("dashboard route /dashboard/%s is missing a pageTitles entry in dashboard/+layout.svelte — "+
				"the header renders empty without it", name)
		}
	}

	// Sanity: the sidebar must actually be the source we think it is, so a
	// refactor that renames the nav arrays fails loudly instead of silently
	// making every route look "unlinked from the sidebar".
	if !strings.Contains(string(sidebar), "/dashboard/connections") {
		t.Fatal("Sidebar.svelte no longer lists /dashboard/connections — nav structure changed, update this guard")
	}
}

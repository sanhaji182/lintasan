package server

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed dashboard/templates/* dashboard/templates/pages/* dashboard/templates/components/*
var dashboardFS embed.FS

type NavItem struct {
	Path  string
	Label string
	SVG   template.HTML
}

type NavGroup struct {
	Title string
	Items []NavItem
}

var navGroups = []NavGroup{
	{
		Title: "MENU",
		Items: []NavItem{
			{Path: "/dashboard", Label: "Overview", SVG: iconOverview},
			{Path: "/dashboard/connections", Label: "Accounts", SVG: iconConnections},
			{Path: "/dashboard/routing", Label: "Routing", SVG: iconRouting},
			{Path: "/dashboard/fallback", Label: "Fallback", SVG: iconFallback},
			{Path: "/dashboard/logs", Label: "Logs", SVG: iconLogs},
			{Path: "/dashboard/usage", Label: "Usage", SVG: iconUsage},
			{Path: "/dashboard/analytics", Label: "Analytics", SVG: iconAnalytics},
		},
	},
	{
		Title: "MANAGE",
		Items: []NavItem{
			{Path: "/dashboard/keys", Label: "API Keys", SVG: iconAPIKeys},
			{Path: "/dashboard/teams", Label: "Teams", SVG: iconTeams},
			{Path: "/dashboard/users", Label: "Users", SVG: iconUsers},
			{Path: "/dashboard/webhooks", Label: "Webhooks", SVG: iconWebhooks},
			{Path: "/dashboard/backup", Label: "Backup", SVG: iconBackup},
			{Path: "/dashboard/settings", Label: "Settings", SVG: iconSettings},
		},
	},
	{
		Title: "TOOLS",
		Items: []NavItem{
			{Path: "/dashboard/plugins", Label: "Plugins", SVG: iconPlugins},
			{Path: "/dashboard/playground", Label: "Playground", SVG: iconPlayground},
			{Path: "/dashboard/docs", Label: "Docs", SVG: iconDocs},
		},
	},
}

// pageLabels maps page name to human-readable title
var pageLabels = map[string]string{
	"overview":    "Overview",
	"connections": "Provider Accounts",
	"routing":     "Routing",
	"fallback":    "Fallback Chains",
	"logs":        "Request Logs",
	"usage":       "Usage",
	"analytics":   "Analytics",
	"metrics":     "Metrics",
	"keys":        "API Keys",
	"teams":       "Teams",
	"users":       "Users",
	"webhooks":    "Webhooks",
	"backup":      "Backup",
	"settings":    "Settings",
	"plugins":     "Plugins",
	"playground":  "Playground",
	"docs":        "Docs",
}

// SVG icons (same as Node.js layout)
const iconOverview = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>`
const iconConnections = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`
const iconRouting = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/><polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/><line x1="4" y1="4" x2="9" y2="9"/></svg>`
const iconFallback = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>`
const iconLogs = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>`
const iconUsage = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>`
const iconAnalytics = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/><polyline points="22 4 12 14 8 10 2 16"/></svg>`
const iconAPIKeys = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>`
const iconTeams = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>`
const iconUsers = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>`
const iconWebhooks = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 16.98h1a2 2 0 0 0 2-2v-1a2 2 0 0 0-4 0v1a2 2 0 0 1-2 2h-2"/><circle cx="9" cy="11" r="3"/><path d="M9 14v4"/><path d="M12 18H6"/><path d="M3 7V5a2 2 0 0 1 2-2h2"/><path d="M17 3h2a2 2 0 0 1 2 2v2"/></svg>`
const iconBackup = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>`
const iconSettings = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9c.26.604.852.997 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`
const iconPlugins = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="2" y="2" width="20" height="20" rx="2"/><path d="M9 2v20"/><path d="M14 2v20"/><path d="M2 9h20"/><path d="M2 14h20"/></svg>`
const iconPlayground = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>`
const iconDocs = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>`

type DashboardEngine struct {
	templates *template.Template
}

func NewDashboardEngine() (*DashboardEngine, error) {
	tmpl := template.New("")

	// Parse base
	baseBytes, err := dashboardFS.ReadFile("dashboard/templates/base.gohtml")
	if err != nil {
		return nil, err
	}
	tmpl, err = tmpl.New("base.gohtml").Parse(string(baseBytes))
	if err != nil {
		return nil, err
	}

	// Parse all page templates
	pagesDir, _ := fs.Sub(dashboardFS, "dashboard/templates/pages")
	pageFiles, _ := fs.Glob(pagesDir, "*.gohtml")
	for _, f := range pageFiles {
		data, err := fs.ReadFile(pagesDir, f)
		if err != nil {
			continue
		}
		tmpl, err = tmpl.New("pages/" + f).Parse(string(data))
		if err != nil {
			return nil, err
		}
	}

	// Parse all component templates
	compsDir, _ := fs.Sub(dashboardFS, "dashboard/templates/components")
	compFiles, _ := fs.Glob(compsDir, "*.gohtml")
	for _, f := range compFiles {
		data, err := fs.ReadFile(compsDir, f)
		if err != nil {
			continue
		}
		tmpl, err = tmpl.New("components/" + f).Parse(string(data))
		if err != nil {
			return nil, err
		}
	}

	return &DashboardEngine{templates: tmpl}, nil
}

// RenderPage renders a page template within the base layout.
func (d *DashboardEngine) RenderPage(w http.ResponseWriter, pageName string, data any) {
	fullName := "pages/" + pageName + ".gohtml"

	// Render page content
	var buf strings.Builder
	err := d.templates.ExecuteTemplate(&buf, fullName, data)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	label := pageLabels[pageName]
	if label == "" {
		label = strings.Title(pageName)
	}

	// Render base with full context
	err = d.templates.ExecuteTemplate(w, "base.gohtml", map[string]any{
		"Title":     pageName,
		"PageTitle": label,
		"NavGroups": navGroups,
		"Content":   template.HTML(buf.String()),
		"Year":      time.Now().Year(),
	})
	if err != nil {
		http.Error(w, "Base template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// RenderPartial renders just a component or page partial (for HTMX swaps).
func (d *DashboardEngine) RenderPartial(w http.ResponseWriter, name string, data any) {
	err := d.templates.ExecuteTemplate(w, name, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

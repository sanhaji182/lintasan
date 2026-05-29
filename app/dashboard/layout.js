"use client";
import { useState, useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";

/* SVG Icon Components */
const Icons = {
  Overview: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/>
      <rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>
    </svg>
  ),
  Connections: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/>
      <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>
    </svg>
  ),
  Providers: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="10"/>
      <line x1="2" y1="12" x2="22" y2="12"/>
      <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
    </svg>
  ),
  Routing: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/>
      <polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/>
      <line x1="4" y1="4" x2="9" y2="9"/>
    </svg>
  ),
  Logs: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
      <polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/>
      <polyline points="10 9 9 9 8 9"/>
    </svg>
  ),
  Usage: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/>
      <line x1="6" y1="20" x2="6" y2="14"/>
    </svg>
  ),
  APIKeys: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/>
    </svg>
  ),
  Settings: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
    </svg>
  ),
  Playground: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>
    </svg>
  ),
  Docs: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>
    </svg>
  ),
  Analytics: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/>
      <polyline points="22 4 12 14 8 10 2 16"/>
    </svg>
  ),
  Sun: () => (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/>
      <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
      <line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/>
      <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
    </svg>
  ),
  Moon: () => (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
    </svg>
  ),
  Teams: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/>
      <path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>
    </svg>
  ),
  Plugins: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="2" width="20" height="20" rx="2"/><path d="M9 2v20"/><path d="M14 2v20"/>
      <path d="M2 9h20"/><path d="M2 14h20"/>
    </svg>
  ),
  Webhooks: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 16.98h1a2 2 0 0 0 2-2v-1a2 2 0 0 0-4 0v1a2 2 0 0 1-2 2h-2"/>
      <circle cx="9" cy="11" r="3"/><path d="M9 14v4"/><path d="M12 18H6"/>
      <path d="M3 7V5a2 2 0 0 1 2-2h2"/><path d="M17 3h2a2 2 0 0 1 2 2v2"/>
    </svg>
  ),
  Backup: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/>
      <line x1="12" y1="15" x2="12" y2="3"/>
    </svg>
  ),
  Fallback: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
    </svg>
  ),
  Users: () => (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>
    </svg>
  ),
};

const NAV_GROUPS = [
  {
    title: "MENU",
    items: [
      { path: "/dashboard", label: "Overview", icon: "Overview" },
      { path: "/dashboard/connections", label: "Accounts", icon: "Connections" },
      { path: "/dashboard/routing", label: "Routing", icon: "Routing" },
      { path: "/dashboard/fallback", label: "Fallback", icon: "Fallback" },
      { path: "/dashboard/logs", label: "Logs", icon: "Logs" },
      { path: "/dashboard/usage", label: "Usage", icon: "Usage" },
      { path: "/dashboard/analytics", label: "Analytics", icon: "Analytics" },
    ],
  },
  {
    title: "MANAGE",
    items: [
      { path: "/dashboard/keys", label: "API Keys", icon: "APIKeys" },
      { path: "/dashboard/teams", label: "Teams", icon: "Teams" },
      { path: "/dashboard/users", label: "Users", icon: "Users" },
      { path: "/dashboard/webhooks", label: "Webhooks", icon: "Webhooks" },
      { path: "/dashboard/backup", label: "Backup", icon: "Backup" },
      { path: "/dashboard/settings", label: "Settings", icon: "Settings" },
    ],
  },
  {
    title: "TOOLS",
    items: [
      { path: "/dashboard/plugins", label: "Plugins", icon: "Plugins" },
      { path: "/dashboard/playground", label: "Playground", icon: "Playground" },
      { path: "/dashboard/docs", label: "Docs", icon: "Docs" },
    ],
  },
];

export default function DashboardLayout({ children }) {
  const [auth, setAuth] = useState(false);
  const [pass, setSetPass] = useState("");
  const [theme, setTheme] = useState("light");
  const [checking, setChecking] = useState(true);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const pathname = usePathname();
  const router = useRouter();

  useEffect(() => {
    const t = localStorage.getItem("sr-theme") || "light";
    setTheme(t);
    document.documentElement.setAttribute("data-theme", t);
    fetch("/api/auth/check").then(r => r.json()).then(d => {
      setAuth(d.authenticated || false);
      setChecking(false);
    }).catch(() => { setAuth(false); setChecking(false); });
  }, []);

  const toggleTheme = () => {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    localStorage.setItem("sr-theme", next);
    document.documentElement.setAttribute("data-theme", next);
  };

  const login = () => {
    fetch("/api/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password: pass }) })
      .then(r => r.json()).then(d => {
        if (d.success) setAuth(true);
        else alert("Wrong password");
      }).catch(() => alert("Login failed"));
  };

  if (checking) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--bg-body)" }}>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: "16px" }}>
          <div style={{ width: "44px", height: "44px", border: "3px solid var(--border)", borderTopColor: "var(--primary)", borderRadius: "50%", animation: "spin 0.7s linear infinite" }} />
          <span style={{ fontSize: "13px", color: "var(--fg-3)", animation: "pulse 1.5s infinite" }}>Loading...</span>
        </div>
      </div>
    );
  }

  if (!auth) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--bg-body)", position: "relative", overflow: "hidden" }}>
        {/* Background decoration */}
        <div style={{ position: "absolute", top: "-20%", right: "-10%", width: "500px", height: "500px", borderRadius: "50%", background: "radial-gradient(circle, var(--primary-glow) 0%, transparent 70%)", filter: "blur(60px)", animation: "float 6s ease-in-out infinite" }} />
        <div style={{ position: "absolute", bottom: "-20%", left: "-10%", width: "400px", height: "400px", borderRadius: "50%", background: "radial-gradient(circle, rgba(139,92,246,0.1) 0%, transparent 70%)", filter: "blur(60px)", animation: "float 8s ease-in-out infinite reverse" }} />

        <div className="fade-in-scale login-card" style={{ width: "420px", background: "var(--bg-card)", borderRadius: "var(--radius-lg)", padding: "48px 40px", boxShadow: "var(--shadow-lg)", border: "1px solid var(--border)", position: "relative", backdropFilter: "blur(20px)" }}>
          <div style={{ textAlign: "center", marginBottom: "36px" }}>
            <div style={{ width: "56px", height: "56px", borderRadius: "14px", background: "linear-gradient(135deg, var(--primary) 0%, #6366f1 100%)", margin: "0 auto 20px", display: "flex", alignItems: "center", justifyContent: "center", boxShadow: "0 8px 24px var(--primary-glow)", animation: "glow 3s ease-in-out infinite" }}>
              <span style={{ color: "#fff", fontSize: "24px", fontWeight: 700 }}>L</span>
            </div>
            <h1 style={{ fontSize: "24px", fontWeight: 700, color: "var(--fg-0)", marginBottom: "6px", letterSpacing: "-0.3px" }}>Lintasan</h1>
            <p style={{ fontSize: "14px", color: "var(--fg-2)" }}>Sign in to access your dashboard</p>
          </div>
          <div style={{ marginBottom: "20px" }}>
            <label style={{ display: "block", fontSize: "13px", fontWeight: 500, color: "var(--fg-1)", marginBottom: "8px" }}>Password</label>
            <input type="password" value={pass} onChange={e => setSetPass(e.target.value)} onKeyDown={e => e.key === "Enter" && login()}
              placeholder="Enter your password"
              style={{ width: "100%", padding: "14px 16px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", color: "var(--fg-0)", fontSize: "14px", outline: "none", transition: "all var(--transition)" }} />
          </div>
          <button onClick={login} style={{ width: "100%", padding: "14px", background: "linear-gradient(135deg, var(--primary) 0%, #6366f1 100%)", color: "#fff", border: "none", borderRadius: "var(--radius-sm)", fontSize: "14px", fontWeight: 600, cursor: "pointer", transition: "all var(--transition)", boxShadow: "0 4px 12px var(--primary-glow)" }}>
            Sign In
          </button>
        </div>
      </div>
    );
  }

  const closeSidebar = () => setSidebarOpen(false);

  const handleNavClick = (path) => {
    router.push(path);
    setSidebarOpen(false);
  };

  return (
    <div style={{ display: "flex", minHeight: "100vh" }}>
      {/* Mobile backdrop */}
      <div
        className={`sidebar-backdrop${sidebarOpen ? " visible" : ""}`}
        onClick={closeSidebar}
        aria-hidden="true"
      />

      {/* Sidebar */}
      <aside className={`dashboard-sidebar${sidebarOpen ? " open" : ""}`} style={{
        width: 260, minHeight: "100vh", background: "var(--bg-sidebar)",
        display: "flex", flexDirection: "column", position: "fixed", left: 0, top: 0, bottom: 0,
        overflowY: "auto", zIndex: 50, borderRight: "1px solid var(--sidebar-border)",
        boxShadow: "var(--shadow-sm)",
      }}>
        {/* Logo */}
        <div style={{ padding: "20px 16px", display: "flex", alignItems: "center", gap: "12px", borderBottom: "1px solid var(--sidebar-border)" }}>
          <div style={{ width: "36px", height: "36px", borderRadius: "10px", background: "linear-gradient(135deg, var(--primary) 0%, #6366f1 100%)", display: "flex", alignItems: "center", justifyContent: "center", boxShadow: "0 4px 12px var(--primary-glow)", flexShrink: 0 }}>
            <span style={{ color: "#fff", fontSize: "16px", fontWeight: 700 }}>L</span>
          </div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 700, color: "var(--sidebar-logo-text)", letterSpacing: "-0.2px" }}>Lintasan</h2>
            <p style={{ fontSize: "11px", color: "var(--fg-sidebar-muted)", fontFamily: "var(--mono)" }}>v1.0.0</p>
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex: 1, padding: "12px 10px" }}>
          {NAV_GROUPS.map(group => (
            <div key={group.title} style={{ marginBottom: "20px" }}>
              <p style={{ fontSize: "10px", fontWeight: 600, color: "var(--fg-sidebar-muted)", padding: "0 10px", marginBottom: "6px", letterSpacing: "0.8px", textTransform: "uppercase" }}>{group.title}</p>
              {group.items.map(item => {
                const active = pathname === item.path;
                const IconComponent = Icons[item.icon];
                return (
                  <a key={item.path} className="nav-link" onClick={() => handleNavClick(item.path)} style={{
                    display: "flex", alignItems: "center", gap: "10px", padding: "10px 12px", borderRadius: "var(--radius-sm)",
                    fontSize: "13px", fontWeight: active ? 600 : 500,
                    color: active ? "var(--primary)" : "var(--fg-sidebar)",
                    background: active ? "var(--primary-light)" : "transparent",
                    cursor: "pointer", marginBottom: "2px", textDecoration: "none",
                    transition: "all 0.15s ease",
                    borderLeft: active ? "3px solid var(--primary)" : "3px solid transparent",
                  }}>
                    <span style={{ opacity: active ? 1 : 0.7, transition: "opacity 0.15s ease", display: "flex" }}>
                      {IconComponent && <IconComponent />}
                    </span>
                    {item.label}
                    {active && <div style={{ position: "absolute", right: "12px", width: "6px", height: "6px", borderRadius: "50%", background: "var(--primary)", animation: "dotPulse 2s infinite" }} />}
                  </a>
                );
              })}
            </div>
          ))}
        </nav>

        {/* Theme toggle */}
        <div style={{ padding: "12px 16px", borderTop: "1px solid var(--sidebar-border)" }}>
          <button onClick={toggleTheme} style={{
            width: "100%", padding: "10px 12px", background: "var(--bg-sidebar-hover)", border: "1px solid var(--sidebar-border)",
            borderRadius: "var(--radius-sm)", color: "var(--fg-sidebar)", fontSize: "13px", cursor: "pointer", fontWeight: 500,
            display: "flex", alignItems: "center", justifyContent: "center", gap: "8px",
            transition: "all 0.15s ease", minHeight: "44px",
          }}>
            <span style={{ display: "flex", opacity: 0.8 }}>{theme === "dark" ? <Icons.Sun /> : <Icons.Moon />}</span>
            {theme === "dark" ? "Light Mode" : "Dark Mode"}
          </button>
        </div>
      </aside>

      {/* Main content */}
      <div className="dashboard-main" style={{ flex: 1, marginLeft: "var(--sidebar-w)" }}>
        {/* Top header */}
        <header className="dashboard-header" style={{
          height: "60px", background: "var(--bg-card)", borderBottom: "1px solid var(--border)",
          display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 24px",
          position: "sticky", top: 0, zIndex: 40, backdropFilter: "blur(12px)",
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: "12px" }}>
            {/* Hamburger menu - mobile only */}
            <button
              className="hamburger-btn"
              onClick={() => setSidebarOpen(true)}
              aria-label="Open menu"
            >
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <line x1="3" y1="6" x2="21" y2="6" />
                <line x1="3" y1="12" x2="21" y2="12" />
                <line x1="3" y1="18" x2="21" y2="18" />
              </svg>
            </button>
            <h1 style={{ fontSize: "15px", fontWeight: 600, color: "var(--fg-0)" }}>
              {NAV_GROUPS.flatMap(g => g.items).find(i => i.path === pathname)?.label || "Dashboard"}
            </h1>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: "14px" }}>
            <div className="mobile-hidden" style={{ display: "flex", alignItems: "center", gap: "6px", background: "var(--bg-body)", padding: "5px 10px", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
              <div style={{ width: "6px", height: "6px", borderRadius: "50%", background: "var(--success)", animation: "dotPulse 2s infinite" }} />
              <span style={{ fontSize: "12px", color: "var(--fg-2)", fontFamily: "var(--mono)" }}>Port 20180</span>
            </div>
            <div style={{ width: "32px", height: "32px", borderRadius: "50%", background: "linear-gradient(135deg, var(--primary) 0%, #6366f1 100%)", display: "flex", alignItems: "center", justifyContent: "center", cursor: "pointer", transition: "transform 0.15s ease" }} title="Admin">
              <span style={{ color: "#fff", fontSize: "12px", fontWeight: 600 }}>SR</span>
            </div>
          </div>
        </header>

        {/* Page content */}
        <main className="dashboard-content" style={{ padding: "24px" }}>
          {children}
        </main>
      </div>
    </div>
  );
}

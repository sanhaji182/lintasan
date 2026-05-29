"use client";
import { useState, useEffect } from "react";

const CATEGORY_COLORS = {
  security: { bg: "rgba(239,68,68,0.1)", color: "#ef4444", border: "rgba(239,68,68,0.2)" },
  monitoring: { bg: "rgba(59,130,246,0.1)", color: "#3b82f6", border: "rgba(59,130,246,0.2)" },
  optimization: { bg: "rgba(16,185,129,0.1)", color: "#10b981", border: "rgba(16,185,129,0.2)" },
  utility: { bg: "rgba(139,92,246,0.1)", color: "#8b5cf6", border: "rgba(139,92,246,0.2)" },
  integration: { bg: "rgba(245,158,11,0.1)", color: "#f59e0b", border: "rgba(245,158,11,0.2)" },
};

export default function PluginStorePage() {
  const [plugins, setPlugins] = useState([]);
  const [categories, setCategories] = useState([]);
  const [activeCategory, setActiveCategory] = useState("all");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [installedNames, setInstalledNames] = useState(new Set());
  const [installing, setInstalling] = useState(null);

  const loadStore = (category) => {
    const url = category && category !== "all"
      ? `/api/plugins/store?category=${category}`
      : "/api/plugins/store";
    fetch(url, { credentials: "include" })
      .then(r => r.json())
      .then(d => {
        setPlugins(d.plugins || []);
        setCategories(d.categories || []);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  };

  const loadInstalled = () => {
    fetch("/api/plugins", { credentials: "include" })
      .then(r => r.json())
      .then(d => {
        const names = new Set((d.plugins || []).map(p => p.name));
        setInstalledNames(names);
      })
      .catch(() => {});
  };

  useEffect(() => {
    loadStore(activeCategory);
    loadInstalled();
  }, [activeCategory]);

  const installPlugin = async (id) => {
    setInstalling(id);
    try {
      const res = await fetch("/api/plugins/store", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ action: "install", id }),
      });
      const data = await res.json();
      if (data.ok) {
        loadInstalled();
      } else {
        alert(data.error || "Failed to install plugin");
      }
    } catch (e) {
      alert("Failed to install plugin");
    }
    setInstalling(null);
  };

  const isInstalled = (plugin) => {
    return installedNames.has(plugin.id) || installedNames.has(plugin.name);
  };

  const filtered = plugins.filter(p => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      p.name.toLowerCase().includes(q) ||
      p.description.toLowerCase().includes(q) ||
      p.tags.some(t => t.toLowerCase().includes(q))
    );
  });

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px", flexWrap: "wrap", gap: "12px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Plugin Store</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Browse and install ready-to-use plugin templates</p>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
          <span style={{ fontSize: "12px", color: "var(--fg-3)", fontFamily: "var(--mono)" }}>{filtered.length} plugins</span>
        </div>
      </div>

      {/* Search */}
      <div style={{ marginBottom: "16px" }}>
        <input
          type="text"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Search plugins by name, description, or tag..."
          style={inputStyle}
        />
      </div>

      {/* Category Tabs */}
      <div style={{ display: "flex", gap: "8px", marginBottom: "24px", flexWrap: "wrap" }}>
        <button
          onClick={() => setActiveCategory("all")}
          style={activeCategory === "all" ? tabActive : tabInactive}
        >
          All
        </button>
        {categories.map(cat => (
          <button
            key={cat}
            onClick={() => setActiveCategory(cat)}
            style={activeCategory === cat ? tabActive : tabInactive}
          >
            {cat.charAt(0).toUpperCase() + cat.slice(1)}
          </button>
        ))}
      </div>

      {/* Plugin Grid */}
      {filtered.length === 0 ? (
        <div style={card}>
          <div style={{ padding: "48px", textAlign: "center" }}>
            <p style={{ fontSize: "14px", color: "var(--fg-2)" }}>No plugins found matching your search.</p>
          </div>
        </div>
      ) : (
        <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: "16px" }}>
          {filtered.map(plugin => {
            const installed = isInstalled(plugin);
            const catColor = CATEGORY_COLORS[plugin.category] || CATEGORY_COLORS.utility;
            return (
              <div key={plugin.id} style={{ ...card, marginBottom: 0, display: "flex", flexDirection: "column", justifyContent: "space-between" }}>
                <div>
                  {/* Top row: name + category badge */}
                  <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", marginBottom: "10px" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                      <div style={{ width: "36px", height: "36px", borderRadius: "8px", background: catColor.bg, display: "flex", alignItems: "center", justifyContent: "center", border: `1px solid ${catColor.border}` }}>
                        <IconStore color={catColor.color} />
                      </div>
                      <div>
                        <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>{plugin.name}</p>
                        <p style={{ fontSize: "11px", color: "var(--fg-3)", fontFamily: "var(--mono)" }}>by {plugin.author} • v{plugin.version}</p>
                      </div>
                    </div>
                    <span style={{
                      fontSize: "10px",
                      fontWeight: 600,
                      padding: "3px 8px",
                      borderRadius: "4px",
                      background: catColor.bg,
                      color: catColor.color,
                      border: `1px solid ${catColor.border}`,
                      textTransform: "uppercase",
                      letterSpacing: "0.3px",
                    }}>
                      {plugin.category}
                    </span>
                  </div>

                  {/* Description */}
                  <p style={{ fontSize: "13px", color: "var(--fg-2)", lineHeight: "1.5", marginBottom: "12px" }}>
                    {plugin.description}
                  </p>

                  {/* Tags */}
                  <div style={{ display: "flex", gap: "6px", flexWrap: "wrap", marginBottom: "14px" }}>
                    {plugin.tags.map(tag => (
                      <span key={tag} style={{
                        fontSize: "11px",
                        padding: "2px 8px",
                        borderRadius: "4px",
                        background: "var(--bg-body)",
                        border: "1px solid var(--border)",
                        color: "var(--fg-3)",
                        fontFamily: "var(--mono)",
                      }}>
                        {tag}
                      </span>
                    ))}
                  </div>
                </div>

                {/* Install button */}
                <div style={{ paddingTop: "12px", borderTop: "1px solid var(--border)" }}>
                  {installed ? (
                    <button disabled style={btnInstalled}>
                      <IconCheck /> Installed
                    </button>
                  ) : (
                    <button
                      onClick={() => installPlugin(plugin.id)}
                      disabled={installing === plugin.id}
                      style={{ ...btnInstall, opacity: installing === plugin.id ? 0.6 : 1 }}
                    >
                      {installing === plugin.id ? "Installing..." : "Install"}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* Icons */
function IconStore({ color = "currentColor" }) {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
      <polyline points="9 22 9 12 15 12 15 22" />
    </svg>
  );
}

function IconCheck() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div>
          <div className="skeleton" style={{ width: "120px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} />
          <div className="skeleton" style={{ width: "280px", height: "14px", borderRadius: "6px" }} />
        </div>
      </div>
      <div className="skeleton" style={{ width: "100%", height: "40px", borderRadius: "var(--radius-sm)", marginBottom: "16px" }} />
      <div style={{ display: "flex", gap: "8px", marginBottom: "24px" }}>
        {[1, 2, 3, 4, 5].map(i => <div key={i} className="skeleton" style={{ width: "80px", height: "32px", borderRadius: "6px" }} />)}
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: "16px" }}>
        {[1, 2, 3, 4].map(i => <div key={i} className="skeleton" style={{ height: "200px", borderRadius: "var(--radius)" }} />)}
      </div>
    </div>
  );
}

/* Styles */
const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", marginBottom: "16px" };

const inputStyle = {
  width: "100%",
  padding: "12px 16px",
  background: "var(--bg-body)",
  border: "1px solid var(--border)",
  borderRadius: "var(--radius-sm)",
  color: "var(--fg-0)",
  fontSize: "13px",
  outline: "none",
};

const tabActive = {
  padding: "7px 14px",
  fontSize: "12px",
  fontWeight: 600,
  borderRadius: "6px",
  border: "1px solid var(--primary)",
  background: "var(--primary-light)",
  color: "var(--primary)",
  cursor: "pointer",
};

const tabInactive = {
  padding: "7px 14px",
  fontSize: "12px",
  fontWeight: 500,
  borderRadius: "6px",
  border: "1px solid var(--border)",
  background: "var(--bg-card)",
  color: "var(--fg-2)",
  cursor: "pointer",
};

const btnInstall = {
  display: "inline-flex",
  alignItems: "center",
  gap: "6px",
  padding: "9px 18px",
  background: "var(--primary)",
  color: "#fff",
  border: "none",
  borderRadius: "var(--radius-sm)",
  fontSize: "13px",
  fontWeight: 500,
  cursor: "pointer",
  width: "100%",
  justifyContent: "center",
};

const btnInstalled = {
  display: "inline-flex",
  alignItems: "center",
  gap: "6px",
  padding: "9px 18px",
  background: "rgba(16,185,129,0.1)",
  color: "#10b981",
  border: "1px solid rgba(16,185,129,0.2)",
  borderRadius: "var(--radius-sm)",
  fontSize: "13px",
  fontWeight: 500,
  cursor: "default",
  width: "100%",
  justifyContent: "center",
};

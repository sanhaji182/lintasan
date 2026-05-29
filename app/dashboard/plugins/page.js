"use client";
import { useState, useEffect } from "react";

const TABS = [
  { id: "installed", label: "My Plugins" },
  { id: "store", label: "Store" },
  { id: "generate", label: "✨ AI Generate" },
];

const CATEGORY_COLORS = {
  security: { bg: "rgba(239,68,68,0.1)", color: "#ef4444", border: "rgba(239,68,68,0.2)" },
  monitoring: { bg: "rgba(59,130,246,0.1)", color: "#3b82f6", border: "rgba(59,130,246,0.2)" },
  optimization: { bg: "rgba(16,185,129,0.1)", color: "#10b981", border: "rgba(16,185,129,0.2)" },
  utility: { bg: "rgba(139,92,246,0.1)", color: "#8b5cf6", border: "rgba(139,92,246,0.2)" },
  integration: { bg: "rgba(245,158,11,0.1)", color: "#f59e0b", border: "rgba(245,158,11,0.2)" },
};

export default function PluginsPage() {
  const [tab, setTab] = useState("installed");

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "20px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Plugins</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Install, manage, and create plugins for the proxy pipeline</p>
        </div>
      </div>

      {/* Tabs */}
      <div style={{ display: "flex", gap: "0", marginBottom: "24px", borderBottom: "1px solid var(--border)" }}>
        {TABS.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)} style={{
            padding: "10px 20px", fontSize: "13px", fontWeight: tab === t.id ? 600 : 400,
            color: tab === t.id ? "var(--primary)" : "var(--fg-3)",
            background: "none", border: "none", borderBottom: tab === t.id ? "2px solid var(--primary)" : "2px solid transparent",
            cursor: "pointer", transition: "all 0.15s ease",
          }}>
            {t.label}
          </button>
        ))}
      </div>

      {/* Tab Content */}
      {tab === "installed" && <InstalledTab />}
      {tab === "store" && <StoreTab />}
      {tab === "generate" && <GenerateTab />}
    </div>
  );
}

/* ==================== INSTALLED TAB ==================== */
function InstalledTab() {
  const [plugins, setPlugins] = useState([]);
  const [loading, setLoading] = useState(true);
  const [viewing, setViewing] = useState(null);
  const [saving, setSaving] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({ name: "", code: "" });
  const [creating, setCreating] = useState(false);

  const load = () => {
    fetch("/api/plugins", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setPlugins(d.plugins || d.data || []); setLoading(false); })
      .catch(() => setLoading(false));
  };
  useEffect(() => { load(); }, []);

  const togglePlugin = async (name, enabled) => {
    await fetch("/api/plugins", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ name, enabled: !enabled }) });
    load();
  };

  const deletePlugin = async (name) => {
    if (!confirm(`Delete plugin "${name}"?`)) return;
    await fetch("/api/plugins", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ action: "delete", name }) });
    load();
  };

  const viewSource = async (name) => {
    const res = await fetch(`/api/plugins?name=${encodeURIComponent(name)}`, { credentials: "include" });
    const data = await res.json();
    if (data.code) setViewing({ name: data.name, code: data.code });
  };

  const saveSource = async () => {
    if (!viewing) return;
    setSaving(true);
    const res = await fetch("/api/plugins", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ action: "update", name: viewing.name, code: viewing.code }) });
    const data = await res.json();
    setSaving(false);
    if (data.ok) { setViewing(null); load(); }
  };

  const createPlugin = async () => {
    if (!form.name) return;
    setCreating(true);
    const code = form.code || generateTemplate(form.name);
    await fetch("/api/plugins", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ action: "create", name: form.name, code }) });
    setCreating(false);
    setShowCreate(false);
    setForm({ name: "", code: "" });
    load();
  };

  if (loading) return <div className="skeleton" style={{ height: "200px", borderRadius: "var(--radius)" }} />;

  const enabledCount = plugins.filter(p => p.enabled).length;

  return (
    <>
      {/* Stats */}
      <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "1px", background: "var(--border)", borderRadius: "var(--radius)", overflow: "hidden", border: "1px solid var(--border)", marginBottom: "20px" }}>
        <div style={metricCell}><p style={metricLabel}>Installed</p><p style={metricValue}>{plugins.length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Enabled</p><p style={metricValue}>{enabledCount}</p></div>
        <div style={metricCell}><p style={metricLabel}>Disabled</p><p style={metricValue}>{plugins.length - enabledCount}</p></div>
      </div>

      {/* Actions */}
      <div style={{ marginBottom: "16px" }}>
        <button onClick={() => setShowCreate(!showCreate)} style={btnPrimary}>{showCreate ? "✕ Cancel" : "+ Create Manually"}</button>
      </div>

      {/* Create Form */}
      {showCreate && (
        <div style={card} className="fade-in">
          <div style={{ display: "grid", gridTemplateColumns: "200px 1fr", gap: "14px", marginBottom: "14px" }}>
            <div><label style={labelSt}>Name</label><input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} style={input} placeholder="my-plugin" /></div>
            <div><label style={labelSt}>Code (empty = template)</label><textarea value={form.code} onChange={e => setForm({ ...form, code: e.target.value })} style={{ ...input, minHeight: "120px", fontFamily: "var(--mono)", fontSize: "12px" }} placeholder="Leave empty for starter template..." /></div>
          </div>
          <button onClick={createPlugin} disabled={creating || !form.name} style={{ ...btnPrimary, opacity: (!form.name) ? 0.5 : 1 }}>{creating ? "Creating..." : "Create"}</button>
        </div>
      )}

      {/* Source Editor Modal */}
      {viewing && (
        <div style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.5)", zIndex: 1000, display: "flex", alignItems: "center", justifyContent: "center", padding: "24px" }} onClick={() => setViewing(null)}>
          <div style={{ background: "var(--bg-card)", borderRadius: "var(--radius-lg)", width: "100%", maxWidth: "720px", maxHeight: "80vh", display: "flex", flexDirection: "column", boxShadow: "var(--shadow-lg)", border: "1px solid var(--border)" }} onClick={e => e.stopPropagation()}>
            <div style={{ padding: "16px 20px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <p style={{ fontSize: "15px", fontWeight: 600, color: "var(--fg-0)" }}>{viewing.name}.js</p>
              <div style={{ display: "flex", gap: "8px" }}>
                <button onClick={saveSource} disabled={saving} style={btnPrimary}>{saving ? "Saving..." : "Save"}</button>
                <button onClick={() => setViewing(null)} style={btnSecondary}>Close</button>
              </div>
            </div>
            <div style={{ flex: 1, overflow: "auto", padding: "16px" }}>
              <textarea value={viewing.code} onChange={e => setViewing({ ...viewing, code: e.target.value })} style={{ width: "100%", minHeight: "400px", padding: "14px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", color: "var(--fg-0)", fontSize: "12px", fontFamily: "var(--mono)", lineHeight: "1.7", resize: "vertical", outline: "none" }} />
            </div>
          </div>
        </div>
      )}

      {/* Plugin List */}
      {plugins.length === 0 ? (
        <div style={{ ...card, padding: "48px", textAlign: "center" }}>
          <p style={{ fontSize: "14px", color: "var(--fg-2)" }}>No plugins installed yet. Browse the Store or use AI Generate.</p>
        </div>
      ) : (
        <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: "14px" }}>
          {plugins.map(p => (
            <div key={p.name} style={{ ...card, marginBottom: 0, padding: "16px" }}>
              <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "10px" }}>
                <div style={{ width: "32px", height: "32px", borderRadius: "8px", background: p.enabled ? "var(--success-light)" : "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", border: "1px solid " + (p.enabled ? "rgba(16,185,129,0.2)" : "var(--border)") }}>
                  <IconPlugin color={p.enabled ? "var(--success)" : "var(--fg-3)"} />
                </div>
                <div style={{ flex: 1 }}>
                  <p style={{ fontSize: "13px", fontWeight: 600, color: "var(--fg-0)" }}>{p.name}</p>
                  {p.description && <p style={{ fontSize: "11px", color: "var(--fg-3)" }}>{p.description}</p>}
                </div>
              </div>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", paddingTop: "10px", borderTop: "1px solid var(--border)" }}>
                <div style={{ display: "flex", gap: "6px" }}>
                  <button onClick={() => viewSource(p.name)} style={btnSmall} title="Source"><IconCode /></button>
                  <button onClick={() => deletePlugin(p.name)} style={{ ...btnSmall, background: "var(--error-light)", borderColor: "rgba(239,68,68,0.2)" }} title="Delete"><IconTrash /></button>
                </div>
                <button onClick={() => togglePlugin(p.name, p.enabled)} style={{ position: "relative", width: "36px", height: "20px", borderRadius: "10px", border: "none", cursor: "pointer", background: p.enabled ? "var(--primary)" : "var(--border)", transition: "background 0.2s", padding: 0 }}>
                  <div style={{ position: "absolute", top: "3px", left: p.enabled ? "19px" : "3px", width: "14px", height: "14px", borderRadius: "50%", background: "#fff", boxShadow: "0 1px 3px rgba(0,0,0,0.2)", transition: "left 0.2s" }} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  );
}

/* ==================== STORE TAB ==================== */
function StoreTab() {
  const [plugins, setPlugins] = useState([]);
  const [categories, setCategories] = useState([]);
  const [activeCategory, setActiveCategory] = useState("all");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [installedNames, setInstalledNames] = useState(new Set());
  const [installing, setInstalling] = useState(null);

  useEffect(() => {
    Promise.all([
      fetch("/api/plugins/store", { credentials: "include" }).then(r => r.json()),
      fetch("/api/plugins", { credentials: "include" }).then(r => r.json()),
    ]).then(([store, installed]) => {
      setPlugins(store.plugins || []);
      setCategories(store.categories || []);
      const names = new Set((installed.plugins || []).map(p => p.name));
      setInstalledNames(names);
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  const install = async (id) => {
    setInstalling(id);
    const res = await fetch("/api/plugins/store", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ action: "install", id }) });
    const data = await res.json();
    if (data.ok) setInstalledNames(prev => new Set([...prev, data.name || id]));
    setInstalling(null);
  };

  if (loading) return <div className="skeleton" style={{ height: "200px", borderRadius: "var(--radius)" }} />;

  const filtered = plugins.filter(p => {
    if (activeCategory !== "all" && p.category !== activeCategory) return false;
    if (search && !p.name.includes(search.toLowerCase()) && !p.description.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  return (
    <>
      {/* Filters */}
      <div style={{ display: "flex", gap: "10px", marginBottom: "16px", flexWrap: "wrap", alignItems: "center" }}>
        <input value={search} onChange={e => setSearch(e.target.value)} style={{ ...input, width: "220px" }} placeholder="Search plugins..." />
        <div style={{ display: "flex", gap: "4px", flexWrap: "wrap" }}>
          <button onClick={() => setActiveCategory("all")} style={{ ...tagBtn, ...(activeCategory === "all" ? tagActive : {}) }}>All</button>
          {categories.map(c => (
            <button key={c} onClick={() => setActiveCategory(c)} style={{ ...tagBtn, ...(activeCategory === c ? tagActive : {}) }}>{c}</button>
          ))}
        </div>
      </div>

      {/* Grid */}
      <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: "14px" }}>
        {filtered.map(p => {
          const catColor = CATEGORY_COLORS[p.category] || CATEGORY_COLORS.utility;
          const isInstalled = installedNames.has(p.name) || installedNames.has(p.id);
          return (
            <div key={p.id} style={{ ...card, marginBottom: 0, padding: "16px" }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: "8px" }}>
                <div>
                  <p style={{ fontSize: "13px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>{p.name}</p>
                  <p style={{ fontSize: "12px", color: "var(--fg-3)", lineHeight: "1.4" }}>{p.description}</p>
                </div>
              </div>
              <div style={{ display: "flex", gap: "6px", marginBottom: "12px", flexWrap: "wrap" }}>
                <span style={{ fontSize: "10px", padding: "2px 8px", borderRadius: "4px", background: catColor.bg, color: catColor.color, border: `1px solid ${catColor.border}`, fontWeight: 500 }}>{p.category}</span>
                {(p.tags || []).slice(0, 2).map(t => <span key={t} style={{ fontSize: "10px", padding: "2px 6px", borderRadius: "4px", background: "var(--bg-body)", color: "var(--fg-3)", border: "1px solid var(--border)" }}>{t}</span>)}
              </div>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>{p.author || "Lintasan"} · v{p.version || "1.0"}</span>
                {isInstalled ? (
                  <span style={{ fontSize: "12px", color: "var(--success)", fontWeight: 500 }}>✓ Installed</span>
                ) : (
                  <button onClick={() => install(p.id)} disabled={installing === p.id} style={{ ...btnPrimary, padding: "6px 12px", fontSize: "12px" }}>
                    {installing === p.id ? "..." : "Install"}
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </>
  );
}

/* ==================== AI GENERATE TAB ==================== */
function GenerateTab() {
  const [prompt, setPrompt] = useState("");
  const [name, setName] = useState("");
  const [generating, setGenerating] = useState(false);
  const [result, setResult] = useState(null);
  const [installing, setInstalling] = useState(false);
  const [installed, setInstalled] = useState(false);
  const [error, setError] = useState("");

  const generate = async () => {
    if (!prompt.trim()) return;
    setGenerating(true); setResult(null); setError(""); setInstalled(false);
    try {
      const res = await fetch("/api/plugins/generate", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ prompt: prompt.trim(), name: name.trim() || undefined }) });
      const data = await res.json();
      if (data.ok) setResult(data);
      else setError(data.error || "Generation failed");
    } catch (e) { setError("Network error: " + e.message); }
    setGenerating(false);
  };

  const install = async () => {
    if (!result) return;
    setInstalling(true);
    const res = await fetch("/api/plugins", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ action: "create", name: result.name, code: result.code }) });
    const data = await res.json();
    if (data.ok) setInstalled(true);
    else setError(data.error || "Install failed");
    setInstalling(false);
  };

  return (
    <>
      {/* Input */}
      <div style={card}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "14px" }}>
          <div style={{ width: "32px", height: "32px", borderRadius: "8px", background: "linear-gradient(135deg, var(--primary), #8b5cf6)", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <span style={{ fontSize: "16px" }}>🤖</span>
          </div>
          <div>
            <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>Describe your plugin</p>
            <p style={{ fontSize: "11px", color: "var(--fg-3)" }}>AI will generate working code from your description</p>
          </div>
        </div>

        <textarea value={prompt} onChange={e => setPrompt(e.target.value)} style={{ ...input, minHeight: "80px", marginBottom: "10px", resize: "vertical" }} placeholder="e.g. Block requests with more than 30 messages and return 400 error..." />

        <div style={{ display: "flex", gap: "10px", alignItems: "center" }}>
          <input value={name} onChange={e => setName(e.target.value)} style={{ ...input, width: "180px" }} placeholder="Plugin name (optional)" />
          <button onClick={generate} disabled={generating || !prompt.trim()} style={{ ...btnPrimary, background: "linear-gradient(135deg, var(--primary), #8b5cf6)", opacity: (!prompt.trim()) ? 0.5 : 1 }}>
            {generating ? "Generating..." : "✨ Generate"}
          </button>
        </div>
      </div>

      {/* Error */}
      {error && <div style={{ ...card, background: "var(--error-light)", borderColor: "rgba(239,68,68,0.3)" }}><p style={{ fontSize: "13px", color: "var(--error)" }}>⚠️ {error}</p></div>}

      {/* Result */}
      {result && (
        <div style={card} className="fade-in">
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "12px" }}>
            <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>{result.name}.js {result.model_used && <span style={{ fontSize: "11px", color: "var(--fg-3)", fontWeight: 400 }}>via {result.model_used}</span>}</p>
            <div style={{ display: "flex", gap: "8px" }}>
              {!installed ? (
                <button onClick={install} disabled={installing} style={{ ...btnPrimary, background: "var(--success)" }}>{installing ? "..." : "📦 Install"}</button>
              ) : (
                <span style={{ fontSize: "12px", color: "var(--success)", fontWeight: 600 }}>✓ Installed</span>
              )}
              <button onClick={() => navigator.clipboard.writeText(result.code)} style={btnSecondary}>📋 Copy</button>
            </div>
          </div>
          <pre style={{ background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", padding: "14px", fontSize: "11px", fontFamily: "var(--mono)", lineHeight: "1.6", color: "var(--fg-1)", overflow: "auto", maxHeight: "350px", whiteSpace: "pre-wrap" }}>{result.code}</pre>
          {installed && <p style={{ marginTop: "10px", fontSize: "12px", color: "var(--success)" }}>✓ Active now. Switch to "My Plugins" tab to manage.</p>}
        </div>
      )}

      {/* Examples */}
      {!result && !generating && (
        <div style={card}>
          <p style={{ fontSize: "12px", fontWeight: 600, color: "var(--fg-2)", marginBottom: "10px" }}>💡 Try these:</p>
          <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
            {["Block requests with more than 30 messages, return 400 error", "Add X-Request-ID UUID header to every response", "Log all requests to data/requests.jsonl with timestamp and model", "Block expensive models (gpt-4, claude-3-opus) outside 9am-6pm", "Auto-add system message 'be concise' if conversation > 5 messages", "Rate limit each API key to 10 req/min, return 429 if exceeded"].map((ex, i) => (
              <button key={i} onClick={() => setPrompt(ex)} style={{ textAlign: "left", padding: "8px 12px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", color: "var(--fg-2)", fontSize: "12px", cursor: "pointer" }}>{ex}</button>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

/* ==================== HELPERS ==================== */
function generateTemplate(name) {
  return `export default {\n  name: "${name}",\n  version: "1.0.0",\n  description: "",\n  priority: 10,\n  enabled: true,\n  hooks: {\n    beforeRequest(ctx) {\n      // ctx: { model, messages, stream, auth, headers, metadata }\n    },\n    afterRequest(ctx, response) {\n      // response is parsed JSON\n    }\n  }\n};\n`;
}

function IconPlugin({ color = "currentColor" }) { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"/></svg>; }
function IconTrash() { return <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--error)" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconCode() { return <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--primary)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>; }

const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", marginBottom: "16px" };
const metricCell = { padding: "14px 20px", background: "var(--bg-card)" };
const metricLabel = { fontSize: "11px", color: "var(--fg-3)", marginBottom: "4px", textTransform: "uppercase", letterSpacing: "0.5px", fontWeight: 500 };
const metricValue = { fontSize: "20px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)" };
const input = { width: "100%", padding: "9px 12px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", color: "var(--fg-0)", fontSize: "13px", outline: "none" };
const labelSt = { display: "block", fontSize: "12px", fontWeight: 500, color: "var(--fg-2)", marginBottom: "6px" };
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 14px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "var(--radius-sm)", fontSize: "12px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 14px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "12px", fontWeight: 500, cursor: "pointer" };
const btnSmall = { display: "inline-flex", alignItems: "center", padding: "5px", background: "var(--primary-light)", border: "1px solid rgba(60,80,224,0.2)", borderRadius: "5px", cursor: "pointer" };
const tagBtn = { padding: "5px 10px", fontSize: "11px", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", background: "var(--bg-body)", color: "var(--fg-3)", cursor: "pointer", fontWeight: 500, textTransform: "capitalize" };
const tagActive = { background: "var(--primary)", color: "#fff", borderColor: "var(--primary)" };

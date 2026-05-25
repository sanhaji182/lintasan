
import { useState, useEffect } from "react";

export default function FallbackPage() {
  const [data, setData] = useState({ model_chains: [], connection_chains: [], stats: {} });
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [formType, setFormType] = useState("model");
  const [form, setForm] = useState({ name: "", models: "" });

  const load = () => {
    fetch("/api/fallback", { credentials: "include" })
      .then(r => r.json())
      .then(d => {
        const raw = d.data || d;
        // Convert object format {name: [models]} to array format [{name, models}]
        const mc = raw.model_chains || {};
        const cc = raw.connection_chains || {};
        const modelArr = Array.isArray(mc) ? mc : Object.entries(mc).map(([name, models]) => ({ name, models: Array.isArray(models) ? models : [] }));
        const connArr = Array.isArray(cc) ? cc : Object.entries(cc).map(([name, models]) => ({ name, models: Array.isArray(models) ? models : [] }));
        setData({ model_chains: modelArr, connection_chains: connArr, stats: raw.stats || {} });
        setLoading(false);
      })
      .catch(() => setLoading(false));
  };
  useEffect(() => { load(); }, []);

  const handleCreate = async () => {
    if (!form.name || !form.models) return;
    const models = form.models.split(",").map(m => m.trim()).filter(Boolean);
    await fetch("/api/fallback", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ type: formType, id: form.name, fallbacks: models })
    });
    setShowForm(false);
    setForm({ name: "", models: "" });
    load();
  };

  const handleDelete = async (name, type) => {
    if (!confirm("Remove this fallback chain?")) return;
    await fetch(`/api/fallback?type=${type}&id=${encodeURIComponent(name)}`, {
      method: "DELETE",
      credentials: "include",
    });
    load();
  };

  if (loading) return <LoadingSkeleton />;

  const modelChains = data.model_chains || [];
  const connChains = data.connection_chains || [];
  const stats = data.stats || {};

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Fallback Chains</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Configure model and connection fallback strategies</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} style={btnPrimary}>
          {showForm ? <><IconX /> Cancel</> : <><IconPlus /> Add Chain</>}
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "1px", background: "var(--border)", borderRadius: "var(--radius)", overflow: "hidden", border: "1px solid var(--border)", marginBottom: "24px" }}>
        <div style={metricCell}><p style={metricLabel}>Model Chains</p><p style={metricValue}>{modelChains.length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Connection Chains</p><p style={metricValue}>{connChains.length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Total Fallbacks Used</p><p style={metricValue}>{stats.total_used || 0}</p></div>
        <div style={metricCell}><p style={metricLabel}>Success Rate</p><p style={metricValue}>{stats.success_rate || "—"}%</p></div>
      </div>

      {/* Create Form */}
      {showForm && (
        <div style={card} className="fade-in">
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
            <div style={iconBadge}><IconChain color="var(--primary)" /></div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>New Fallback Chain</p>
              <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>Define ordered fallback sequence</p>
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 2fr 2fr", gap: "14px" }}>
            <div>
              <label style={labelSt}>Type</label>
              <select value={formType} onChange={e => setFormType(e.target.value)} style={input}>
                <option value="model">Model</option>
                <option value="connection">Connection</option>
              </select>
            </div>
            <div>
              <label style={labelSt}>Chain Name</label>
              <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} style={input} placeholder="e.g. gpt4-fallback" />
            </div>
            <div>
              <label style={labelSt}>Models (comma-separated)</label>
              <input value={form.models} onChange={e => setForm({ ...form, models: e.target.value })} style={input} placeholder="gpt-4, gpt-3.5-turbo, claude-3" />
            </div>
          </div>
          <div style={{ display: "flex", gap: "10px", marginTop: "16px" }}>
            <button onClick={handleCreate} style={btnPrimary}><IconCheck /> Create Chain</button>
            <button onClick={() => setShowForm(false)} style={btnSecondary}>Cancel</button>
          </div>
        </div>
      )}

      {/* Model Chains */}
      <div style={{ marginBottom: "24px" }}>
        <h2 style={sectionTitle}>Model Fallback Chains</h2>
        <div style={{ ...card, padding: 0, overflow: "hidden" }}>
          {modelChains.length === 0 ? (
            <EmptyState text="No model fallback chains configured" />
          ) : (
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }} className="responsive-table">
              <thead>
                <tr style={{ background: "var(--bg-body)" }}>
                  {["Name", "Chain", "Used", ""].map(h => <th key={h} style={th}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {modelChains.map(c => (
                  <tr key={c.id || c.name} style={row}>
                    <td style={td}><span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{c.name}</span></td>
                    <td style={td}>
                      <div style={{ display: "flex", gap: "6px", flexWrap: "wrap" }}>
                        {(c.models || []).map((m, i) => (
                          <span key={i} style={chainBadge}>
                            {i > 0 && <span style={{ color: "var(--fg-3)", marginRight: "4px" }}>→</span>}
                            {m}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td style={td}><code style={mono}>{c.used_count || 0}</code></td>
                    <td style={{ ...td, textAlign: "right" }}>
                      <button onClick={() => handleDelete(c.name, "model")} style={btnDangerSmall}><IconTrash size={14} /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      {/* Connection Chains */}
      <div>
        <h2 style={sectionTitle}>Connection Fallback Chains</h2>
        <div style={{ ...card, padding: 0, overflow: "hidden" }}>
          {connChains.length === 0 ? (
            <EmptyState text="No connection fallback chains configured" />
          ) : (
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }} className="responsive-table">
              <thead>
                <tr style={{ background: "var(--bg-body)" }}>
                  {["Name", "Chain", "Used", ""].map(h => <th key={h} style={th}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {connChains.map(c => (
                  <tr key={c.id || c.name} style={row}>
                    <td style={td}><span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{c.name}</span></td>
                    <td style={td}>
                      <div style={{ display: "flex", gap: "6px", flexWrap: "wrap" }}>
                        {(c.models || c.connections || []).map((m, i) => (
                          <span key={i} style={chainBadge}>
                            {i > 0 && <span style={{ color: "var(--fg-3)", marginRight: "4px" }}>→</span>}
                            {m}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td style={td}><code style={mono}>{c.used_count || 0}</code></td>
                    <td style={{ ...td, textAlign: "right" }}>
                      <button onClick={() => handleDelete(c.name, "connection")} style={btnDangerSmall}><IconTrash size={14} /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}

/* Icons */
function IconPlus() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>; }
function IconX() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>; }
function IconCheck() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="20 6 9 17 4 12"/></svg>; }
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconChain({ color = "currentColor" }) { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>; }

function EmptyState({ text }) {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <div style={{ width: "56px", height: "56px", borderRadius: "12px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", margin: "0 auto 16px" }}>
        <IconChain color="var(--fg-3)" />
      </div>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>{text}</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div><div className="skeleton" style={{ width: "140px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "260px", height: "14px", borderRadius: "6px" }} /></div>
        <div className="skeleton" style={{ width: "120px", height: "36px", borderRadius: "6px" }} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "1px", marginBottom: "24px" }}>
        {[1,2,3,4].map(i => <div key={i} className="skeleton" style={{ height: "70px" }} />)}
      </div>
      <div className="skeleton" style={{ height: "200px", borderRadius: "var(--radius)", marginBottom: "16px" }} />
      <div className="skeleton" style={{ height: "200px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", marginBottom: "16px" };
const metricCell = { padding: "16px 20px", background: "var(--bg-card)" };
const metricLabel = { fontSize: "11px", color: "var(--fg-3)", marginBottom: "4px", textTransform: "uppercase", letterSpacing: "0.5px", fontWeight: 500 };
const metricValue = { fontSize: "22px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)", letterSpacing: "-0.5px" };
const iconBadge = { width: "36px", height: "36px", borderRadius: "8px", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center" };
const labelSt = { display: "block", fontSize: "12px", color: "var(--fg-2)", marginBottom: "6px", fontWeight: 500 };
const input = { width: "100%", padding: "9px 12px", border: "1px solid var(--border)", borderRadius: "6px", background: "var(--bg-body)", color: "var(--fg-0)", fontSize: "13px", outline: "none" };
const th = { textAlign: "left", padding: "12px 16px", fontWeight: 500, color: "var(--fg-3)", fontSize: "11px", borderBottom: "1px solid var(--border)", textTransform: "uppercase", letterSpacing: "0.5px" };
const td = { padding: "14px 16px", verticalAlign: "middle" };
const row = { borderBottom: "1px solid var(--border)", transition: "background 0.1s ease" };
const mono = { fontFamily: "var(--mono)", fontSize: "12px", color: "var(--fg-2)", background: "var(--bg-body)", padding: "3px 8px", borderRadius: "4px" };
const sectionTitle = { fontSize: "14px", fontWeight: 600, color: "var(--fg-1)", marginBottom: "12px" };
const chainBadge = { fontSize: "12px", padding: "4px 10px", borderRadius: "4px", background: "var(--bg-body)", border: "1px solid var(--border)", color: "var(--fg-1)", fontFamily: "var(--mono)" };
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { padding: "8px 16px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnDangerSmall = { display: "flex", alignItems: "center", justifyContent: "center", width: "30px", height: "30px", background: "none", border: "1px solid var(--border)", borderRadius: "6px", color: "var(--error)", cursor: "pointer", transition: "all 0.15s ease" };

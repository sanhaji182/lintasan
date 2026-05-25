
import { useState, useEffect } from "react";

export default function RoutingPage() {
  const [combos, setCombos] = useState([]);
  const [connections, setConnections] = useState([]);
  const [models, setModels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showAddCombo, setShowAddCombo] = useState(false);
  const [comboForm, setComboForm] = useState({ name: "", description: "", strategy: "priority", stickyLimit: 3, entries: [] });
  const [strategy, setStrategy] = useState("priority");
  const [aliases, setAliases] = useState({});
  const [aliasForm, setAliasForm] = useState({ alias: "", model: "" });
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([
      fetch("/api/combos", { credentials: "include" }).then(r => r.json()),
      fetch("/api/v1/models", { credentials: "include" }).then(r => r.json()),
      fetch("/api/connections", { credentials: "include" }).then(r => r.json()),
      fetch("/api/load-balancer", { credentials: "include" }).then(r => r.json()).catch(() => ({ data: { strategy: "priority" } })),
      fetch("/api/aliases", { credentials: "include" }).then(r => r.json()).catch(() => ({ data: {} })),
    ]).then(([combosData, modelsData, connsData, lbData, aliasData]) => {
      setCombos(combosData.data || []);
      setModels((modelsData.data || []).filter(m => m.owned_by !== "combo"));
      setConnections(connsData.data || []);
      setStrategy(lbData.data?.strategy || "priority");
      setAliases(aliasData.data || {});
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  function addEntry() {
    setComboForm({ ...comboForm, entries: [...comboForm.entries, { model: "", connection_ids: [], label: "" }] });
  }

  function updateEntry(idx, field, value) {
    const entries = [...comboForm.entries];
    entries[idx] = { ...entries[idx], [field]: value };
    if (field === "model") entries[idx].label = value;
    setComboForm({ ...comboForm, entries });
  }

  function toggleConnection(entryIdx, connId) {
    const entries = [...comboForm.entries];
    const ids = entries[entryIdx].connection_ids || [];
    if (ids.includes(connId)) {
      entries[entryIdx].connection_ids = ids.filter(id => id !== connId);
    } else {
      entries[entryIdx].connection_ids = [...ids, connId];
    }
    setComboForm({ ...comboForm, entries });
  }

  function removeEntry(idx) {
    setComboForm({ ...comboForm, entries: comboForm.entries.filter((_, i) => i !== idx) });
  }

  function moveEntry(idx, dir) {
    const arr = [...comboForm.entries];
    const newIdx = idx + dir;
    if (newIdx < 0 || newIdx >= arr.length) return;
    [arr[idx], arr[newIdx]] = [arr[newIdx], arr[idx]];
    setComboForm({ ...comboForm, entries: arr });
  }

  async function createCombo(e) {
    e.preventDefault();
    setError("");
    if (!comboForm.name) { setError("Combo name is required"); return; }
    if (comboForm.entries.length === 0) { setError("Add at least 1 model entry"); return; }
    if (comboForm.entries.some(e => !e.model)) { setError("All entries must have a model selected"); return; }
    const res = await fetch("/api/combos", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify(comboForm) });
    const data = await res.json();
    if (data.error) { setError(data.error.message); return; }
    setCombos([...combos, data.data || comboForm]);
    setComboForm({ name: "", description: "", strategy: "priority", stickyLimit: 3, entries: [] });
    setShowAddCombo(false);
  }

  async function deleteCombo(name) {
    if (!confirm("Delete combo '" + name + "'?")) return;
    await fetch("/api/combos?name=" + encodeURIComponent(name), { method: "DELETE", credentials: "include" });
    setCombos(combos.filter(c => c.name !== name));
  }

  async function saveStrategy(newStrategy) {
    setStrategy(newStrategy);
    await fetch("/api/load-balancer", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ strategy: newStrategy }) }).catch(() => {});
  }

  async function addAlias(e) {
    e.preventDefault();
    if (!aliasForm.alias || !aliasForm.model) return;
    const newAliases = { ...aliases, [aliasForm.alias]: { model: aliasForm.model } };
    await fetch("/api/aliases", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify(newAliases) }).catch(() => {});
    setAliases(newAliases);
    setAliasForm({ alias: "", model: "" });
  }

  async function removeAlias(key) {
    const newAliases = { ...aliases };
    delete newAliases[key];
    await fetch("/api/aliases", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify(newAliases) }).catch(() => {});
    setAliases(newAliases);
  }

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      <p style={{ fontSize: "13px", color: "var(--fg-3)", marginBottom: "24px" }}>Create combos, set strategy, and manage aliases. A combo = one model name that routes across your accounts.</p>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "16px", marginBottom: "24px" }}>
        <MiniStat label="COMBOS" value={combos.length} color="var(--primary)" />
        <MiniStat label="STRATEGY" value={strategy} color="var(--success)" />
        <MiniStat label="ALIASES" value={Object.keys(aliases).length} color="var(--info)" />
      </div>

      {/* Combos */}
      <div style={card}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "16px" }}>
          <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
            <div style={iconBadge}><IconCombo /></div>
            <div>
              <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Combos</h2>
              <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Pick models + accounts, set strategy. Use combo name as your model.</p>
            </div>
          </div>
          <button onClick={() => setShowAddCombo(!showAddCombo)} style={btnPrimary}><IconPlus size={14} /> New Combo</button>
        </div>

        {showAddCombo && (
          <div style={{ padding: "20px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", marginBottom: "16px", border: "1px solid var(--border)" }}>
            {error && <div style={{ padding: "8px 12px", background: "var(--error-light)", color: "var(--error)", borderRadius: "6px", fontSize: "13px", marginBottom: "12px" }}>{error}</div>}
            <form onSubmit={createCombo}>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px", marginBottom: "16px" }}>
                <div>
                  <label style={labelStyle}>Combo Name *</label>
                  <input style={inputStyle} placeholder="e.g. always-on, coding" value={comboForm.name} onChange={e => setComboForm({ ...comboForm, name: e.target.value })} />
                  <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "4px 0 0" }}>This becomes your model name in tools</p>
                </div>
                <div>
                  <label style={labelStyle}>Description</label>
                  <input style={inputStyle} placeholder="What this combo is for" value={comboForm.description} onChange={e => setComboForm({ ...comboForm, description: e.target.value })} />
                </div>
              </div>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px", marginBottom: "16px" }}>
                <div>
                  <label style={labelStyle}>Strategy</label>
                  <select style={inputStyle} value={comboForm.strategy} onChange={e => setComboForm({ ...comboForm, strategy: e.target.value })}>
                    <option value="priority">Priority (try in order, fallback on fail)</option>
                    <option value="round-robin">Round-robin (rotate across entries)</option>
                  </select>
                </div>
                <div>
                  <label style={labelStyle}>Sticky Limit</label>
                  <input style={inputStyle} type="number" min="0" max="100" value={comboForm.stickyLimit} onChange={e => setComboForm({ ...comboForm, stickyLimit: parseInt(e.target.value) || 3 })} />
                  <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "4px 0 0" }}>Requests before rotating (0 = every time)</p>
                </div>
              </div>

              <div style={{ marginBottom: "16px" }}>
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "8px" }}>
                  <label style={{ ...labelStyle, margin: 0 }}>Model Entries (order = fallback priority)</label>
                  <button type="button" onClick={addEntry} style={btnSecondary}><IconPlus size={12} /> Add Model</button>
                </div>

                {comboForm.entries.length === 0 && (
                  <div style={{ padding: "24px", textAlign: "center", background: "var(--bg-card)", borderRadius: "var(--radius-sm)", border: "1px dashed var(--border)" }}>
                    <p style={{ fontSize: "13px", color: "var(--fg-3)", margin: 0 }}>Click "Add Model" to start building your combo.</p>
                  </div>
                )}

                {comboForm.entries.map((entry, idx) => (
                  <div key={idx} style={{ padding: "14px", background: "var(--bg-card)", borderRadius: "var(--radius-sm)", marginBottom: "8px", border: "1px solid var(--border)" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: "8px", marginBottom: "10px" }}>
                      <span style={{ width: "24px", height: "24px", borderRadius: "50%", background: "var(--primary)", color: "white", display: "flex", alignItems: "center", justifyContent: "center", fontSize: "12px", fontWeight: 700, flexShrink: 0 }}>{idx + 1}</span>
                      <select style={{ ...inputStyle, flexGrow: 1, margin: 0 }} value={entry.model} onChange={e => updateEntry(idx, "model", e.target.value)}>
                        <option value="">Select model...</option>
                        {models.map(m => <option key={m.id} value={m.id}>{m.id}</option>)}
                      </select>
                      <button type="button" onClick={() => moveEntry(idx, -1)} disabled={idx === 0} style={btnMini}>&#8593;</button>
                      <button type="button" onClick={() => moveEntry(idx, 1)} disabled={idx === comboForm.entries.length - 1} style={btnMini}>&#8595;</button>
                      <button type="button" onClick={() => removeEntry(idx)} style={{ ...btnMini, color: "var(--error)" }}>&#215;</button>
                    </div>
                    {entry.model && (
                      <div>
                        <p style={{ fontSize: "11px", color: "var(--fg-3)", marginBottom: "6px", fontWeight: 500 }}>Use these accounts (keys round-robin automatically):</p>
                        <div style={{ display: "flex", flexWrap: "wrap", gap: "6px" }}>
                          {connections.map(conn => (
                            <button key={conn.id} type="button" onClick={() => toggleConnection(idx, conn.id)} style={{ padding: "4px 10px", fontSize: "12px", borderRadius: "9999px", border: (entry.connection_ids || []).includes(conn.id) ? "2px solid var(--primary)" : "1px solid var(--border)", background: (entry.connection_ids || []).includes(conn.id) ? "var(--primary-light)" : "var(--bg-body)", color: (entry.connection_ids || []).includes(conn.id) ? "var(--primary)" : "var(--fg-2)", cursor: "pointer", fontWeight: (entry.connection_ids || []).includes(conn.id) ? 600 : 400 }}>
                              {conn.name}
                            </button>
                          ))}
                        </div>
                        {(entry.connection_ids || []).length === 0 && (
                          <p style={{ fontSize: "11px", color: "var(--warning)", margin: "6px 0 0" }}>No account selected = uses any account that has this model</p>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </div>

              <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}>
                <button type="button" onClick={() => { setShowAddCombo(false); setError(""); }} style={btnSecondary}>Cancel</button>
                <button type="submit" style={btnPrimary}>Create Combo</button>
              </div>
            </form>
          </div>
        )}

        {combos.length === 0 && !showAddCombo ? (
          <div style={{ padding: "32px", textAlign: "center" }}>
            <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>No combos yet. Create one to get a model name you can use anywhere.</p>
          </div>
        ) : (
          combos.map(combo => (
            <div key={combo.name} style={{ padding: "14px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", marginBottom: "8px", border: "1px solid var(--border)" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <div style={{ flex: 1 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: "8px", marginBottom: "4px" }}>
                    <span style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", fontFamily: "var(--mono)" }}>{combo.name}</span>
                    <span style={{ fontSize: "11px", padding: "2px 8px", borderRadius: "9999px", background: "var(--primary-light)", color: "var(--primary)", fontWeight: 500 }}>{(combo.entries || combo.models || []).length} models</span>
                    <span style={{ fontSize: "11px", padding: "2px 8px", borderRadius: "9999px", background: "var(--success)" + "18", color: "var(--success)", fontWeight: 500 }}>{combo.strategy || "priority"}</span>
                  </div>
                  {combo.description && <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: "0 0 6px" }}>{combo.description}</p>}
                  <div style={{ display: "flex", gap: "4px", flexWrap: "wrap" }}>
                    {(combo.entries || combo.models || []).map((entry, i) => (
                      <span key={i} style={{ fontSize: "11px", padding: "3px 8px", background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "4px", fontFamily: "var(--mono)", color: "var(--fg-2)" }}>
                        {i + 1}. {entry.model || entry.label || "?"}
                        {entry.connection_ids && entry.connection_ids.length > 0 && <span style={{ color: "var(--primary)", marginLeft: "4px" }}>({entry.connection_ids.length} keys)</span>}
                      </span>
                    ))}
                  </div>
                </div>
                <button onClick={() => deleteCombo(combo.name)} style={{ ...btnSmall, color: "var(--error)" }} title="Delete"><IconTrash /></button>
              </div>
            </div>
          ))
        )}

        {combos.length > 0 && (
          <div style={{ marginTop: "12px", padding: "12px 16px", background: "var(--primary-light)", borderRadius: "var(--radius-sm)", borderLeft: "3px solid var(--primary)" }}>
            <p style={{ fontSize: "12px", color: "var(--fg-1)", margin: 0 }}><strong>How to use:</strong> Base URL <code style={{ fontFamily: "var(--mono)", fontSize: "11px" }}>http://100.99.2.14:20180/api/v1</code>, model = <code style={{ fontFamily: "var(--mono)", fontSize: "11px" }}>{combos[0]?.name || "combo-name"}</code></p>
          </div>
        )}
      </div>

      {/* Load Balancer */}
      <div style={{ ...card, marginTop: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
          <div style={iconBadge}><IconBalance /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Load Balancer</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Default strategy when multiple accounts have the same model</p>
          </div>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "8px" }}>
          {["priority", "round-robin", "least-connections", "random"].map(s => (
            <button key={s} onClick={() => saveStrategy(s)} style={{ padding: "12px", background: strategy === s ? "var(--primary-light)" : "var(--bg-body)", border: strategy === s ? "2px solid var(--primary)" : "1px solid var(--border)", borderRadius: "var(--radius-sm)", cursor: "pointer", textAlign: "center" }}>
              <p style={{ fontSize: "13px", fontWeight: 600, color: strategy === s ? "var(--primary)" : "var(--fg-0)", margin: 0 }}>{s}</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "4px 0 0" }}>
                {s === "priority" && "Highest priority first"}
                {s === "round-robin" && "Rotate evenly"}
                {s === "least-connections" && "Fewest active requests"}
                {s === "random" && "Random selection"}
              </p>
            </button>
          ))}
        </div>
      </div>

      {/* Aliases */}
      <div style={{ ...card, marginTop: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
          <div style={iconBadge}><IconAlias /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Model Aliases</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Short names that map to full model IDs</p>
          </div>
        </div>
        <form onSubmit={addAlias} style={{ display: "flex", gap: "8px", marginBottom: "12px" }}>
          <input style={{ ...inputStyle, width: "150px" }} placeholder="Alias (e.g. gpt4)" value={aliasForm.alias} onChange={e => setAliasForm({ ...aliasForm, alias: e.target.value })} />
          <select style={{ ...inputStyle, flexGrow: 1 }} value={aliasForm.model} onChange={e => setAliasForm({ ...aliasForm, model: e.target.value })}>
            <option value="">Select target model...</option>
            {models.map(m => <option key={m.id} value={m.id}>{m.id}</option>)}
          </select>
          <button type="submit" style={btnPrimary}>Add</button>
        </form>
        {Object.keys(aliases).length === 0 ? (
          <p style={{ fontSize: "12px", color: "var(--fg-3)", textAlign: "center", padding: "16px" }}>No aliases configured</p>
        ) : (
          Object.entries(aliases).map(([alias, target]) => (
            <div key={alias} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "8px 12px", background: "var(--bg-body)", borderRadius: "6px", marginBottom: "4px", border: "1px solid var(--border)" }}>
              <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                <span style={{ fontSize: "13px", fontWeight: 600, fontFamily: "var(--mono)", color: "var(--primary)" }}>{alias}</span>
                <span style={{ fontSize: "12px", color: "var(--fg-3)" }}>&rarr;</span>
                <span style={{ fontSize: "13px", fontFamily: "var(--mono)", color: "var(--fg-1)" }}>{target.model || target}</span>
              </div>
              <button onClick={() => removeAlias(alias)} style={{ ...btnMini, color: "var(--error)" }}>&times;</button>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function MiniStat({ label, value, color }) {
  return (
    <div style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "16px 18px", boxShadow: "var(--shadow)", border: "1px solid var(--border)" }}>
      <p style={{ fontSize: "20px", fontWeight: 700, color: color || "var(--fg-0)", fontFamily: "var(--mono)", letterSpacing: "-0.3px", marginBottom: "2px" }}>{value}</p>
      <p style={{ fontSize: "11px", color: "var(--fg-3)", fontWeight: 500, letterSpacing: "0.5px" }}>{label}</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ marginBottom: "24px" }}><div className="skeleton" style={{ width: "280px", height: "14px", borderRadius: "6px" }} /></div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "16px", marginBottom: "24px" }}>
        {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: "70px", borderRadius: "var(--radius)" }} />)}
      </div>
      <div className="skeleton" style={{ height: "200px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

function IconCombo() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/><polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/><line x1="4" y1="4" x2="9" y2="9"/></svg>; }
function IconPlus({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>; }
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconBalance() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="12" y1="3" x2="12" y2="21"/><polyline points="8 8 4 12 8 16"/><polyline points="16 8 20 12 16 16"/></svg>; }
function IconAlias() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>; }

const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)" };
const iconBadge = { width: "36px", height: "36px", borderRadius: "8px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0 };
const btnPrimary = { display: "flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "var(--radius-sm)", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { display: "flex", alignItems: "center", gap: "4px", padding: "8px 14px", background: "transparent", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "13px", cursor: "pointer" };
const btnSmall = { width: "30px", height: "30px", display: "flex", alignItems: "center", justifyContent: "center", background: "transparent", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", cursor: "pointer", color: "var(--fg-2)" };
const btnMini = { width: "24px", height: "24px", display: "flex", alignItems: "center", justifyContent: "center", background: "transparent", border: "1px solid var(--border)", borderRadius: "4px", cursor: "pointer", color: "var(--fg-2)", fontSize: "12px" };
const labelStyle = { display: "block", fontSize: "12px", fontWeight: 500, color: "var(--fg-2)", marginBottom: "4px" };
const inputStyle = { width: "100%", padding: "8px 12px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "13px", color: "var(--fg-0)", outline: "none" };

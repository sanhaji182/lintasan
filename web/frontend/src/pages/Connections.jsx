
import { useState, useEffect } from "react";

export default function ConnectionsPage() {
  const [connections, setConnections] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [presets, setPresets] = useState([]);
  const [categories, setCategories] = useState([]);
  const [selectedPreset, setSelectedPreset] = useState(null);
  const [syncing, setSyncing] = useState({});
  const [expanded, setExpanded] = useState({});
  const [connModels, setConnModels] = useState({});
  const [form, setForm] = useState({ name: "", baseUrl: "", apiKey: "", format: "openai", chatPath: "/v1/chat/completions", modelsPath: "/v1/models", authHeader: "Authorization", authPrefix: "Bearer " });
  const [error, setError] = useState("");
  const [testStatus, setTestStatus] = useState(null); // null | "testing" | "success" | "failed"
  const [testMessage, setTestMessage] = useState("");
  const [step, setStep] = useState("pick"); // pick | configure

  useEffect(() => { fetchConnections(); fetchPresets(); }, []);

  function fetchConnections() {
    fetch("/api/connections", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setConnections(d.data || []); setLoading(false); })
      .catch(() => setLoading(false));
  }

  function fetchModelsForConnection(connId) {
    fetch(`/api/models/manual?connectionId=${connId}`, { credentials: "include" })
      .then(r => r.json())
      .then(d => { setConnModels(prev => ({ ...prev, [connId]: d.models || [] })); });
  }

  async function removeModel(connId, modelId) {
    await fetch(`/api/models/manual?connectionId=${connId}&modelId=${encodeURIComponent(modelId)}`, { method: "DELETE", credentials: "include" });
    loadModels(connId);
    fetchConnections();
  }

  async function toggleModelActive(connId, modelId, currentActive) {
    await fetch("/api/models/manual", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "toggle", connectionId: connId, modelId, active: !currentActive })
    });
    loadModels(connId);
  }

  function fetchPresets() {
    fetch("/api/providers/presets", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setPresets(d.data || []); setCategories(d.categories || []); })
      .catch(() => {});
  }

  function selectPreset(preset) {
    setSelectedPreset(preset);
    // Auto-fill form from preset
    fetch("/api/providers/presets/config?id=" + preset.id, { credentials: "include" })
      .then(r => r.json())
      .then(d => {
        if (d.data) {
          setForm({
            name: d.data.name || preset.name,
            baseUrl: d.data.baseUrl || "",
            apiKey: "",
            format: d.data.format || "openai",
            chatPath: d.data.chatPath || "/v1/chat/completions",
            modelsPath: d.data.modelsPath || "/v1/models",
            authHeader: d.data.authHeader || "Authorization",
            authPrefix: d.data.authPrefix || "Bearer ",
          });
        }
        setStep("configure");
      })
      .catch(() => {
        // Fallback: use preset info directly
        setForm({ name: preset.name, baseUrl: "", apiKey: "", format: "openai", chatPath: "/v1/chat/completions", modelsPath: "/v1/models", authHeader: "Authorization", authPrefix: "Bearer " });
        setStep("configure");
      });
  }

  async function testConnection() {
    setTestStatus("testing");
    setTestMessage("");
    setError("");
    try {
      const res = await fetch("/api/connections/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(form),
      });
      const data = await res.json();
      if (data.success) {
        setTestStatus("success");
        setTestMessage(data.message);
      } else {
        setTestStatus("failed");
        setTestMessage(data.message || "Connection failed");
      }
    } catch (e) {
      setTestStatus("failed");
      setTestMessage("Network error: " + e.message);
    }
  }

  async function addConnection(e) {
    e.preventDefault();
    setError("");
    if (!form.name || !form.baseUrl) { setError("Name and Base URL are required"); return; }
    if (!form.apiKey && !selectedPreset?.noAuth) { setError("API Key is required"); return; }
    if (testStatus !== "success") { setError("Please test the connection first"); return; }

    const res = await fetch("/api/connections", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(form),
    });
    const data = await res.json();
    if (data.error) { setError(data.error.message || "Failed to add"); return; }

    setForm({ name: "", baseUrl: "", apiKey: "", format: "openai", chatPath: "/v1/chat/completions", modelsPath: "/v1/models", authHeader: "Authorization", authPrefix: "Bearer " });
    setShowAdd(false);
    setStep("pick");
    setSelectedPreset(null);
    setTestStatus(null);
    setTestMessage("");
    fetchConnections();
    // Auto-sync models
    if (data.data?.id) syncModels(data.data.id);
  }

  async function syncModels(connId) {
    setSyncing(s => ({ ...s, [connId]: true }));
    try {
      await fetch("/api/models/sync", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ connection_id: connId }) });
      fetchConnections();
      // Auto-expand and load models after sync
      loadModels(connId);
      setExpanded(e => ({ ...e, [connId]: true }));
    } catch (e) {}
    setSyncing(s => ({ ...s, [connId]: false }));
  }

  async function syncAll() {
    setSyncing(s => ({ ...s, all: true }));
    try {
      await fetch("/api/models/sync", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({}) });
      fetchConnections();
    } catch (e) {}
    setSyncing(s => ({ ...s, all: false }));
  }

  async function loadModels(connId) {
    try {
      const res = await fetch("/api/models/sync?connection_id=" + connId, { credentials: "include" });
      const d = await res.json();
      setConnModels(m => ({ ...m, [connId]: d.data || [] }));
    } catch (e) {}
  }

  function toggleExpand(connId) {
    const isExpanded = !expanded[connId];
    setExpanded(e => ({ ...e, [connId]: isExpanded }));
    if (isExpanded && !connModels[connId]) {
      loadModels(connId);
    }
  }

  async function deleteConn(id) {
    if (!confirm("Delete this connection and all its discovered models?")) return;
    await fetch("/api/connections?id=" + id, { method: "DELETE", credentials: "include" });
    fetchConnections();
  }

  async function toggleActive(id, currentActive) {
    await fetch("/api/connections", { method: "PATCH", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ id, is_active: currentActive ? 0 : 1 }) });
    fetchConnections();
  }

  if (loading) return <LoadingSkeleton />;

  const totalModels = connections.reduce((sum, c) => sum + (c.models_count || 0), 0);
  const activeCount = connections.filter(c => c.is_active).length;

  return (
    <div className="fade-in">
      <div style={{ marginBottom: "24px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Connect LLM providers and auto-discover available models</p>
        <div style={{ display: "flex", gap: "8px" }}>
          <button onClick={syncAll} disabled={syncing.all} style={btnSecondary}>{syncing.all ? "Syncing..." : "Sync All"}</button>
          <button onClick={() => { setShowAdd(!showAdd); setStep("pick"); setSelectedPreset(null); }} style={btnPrimary}><IconPlus size={14} /> Add Provider</button>
        </div>
      </div>

      {/* Stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "16px", marginBottom: "24px" }}>
        <MiniStat label="PROVIDERS" value={connections.length} color="var(--primary)" />
        <MiniStat label="ACTIVE" value={activeCount} color="var(--success)" />
        <MiniStat label="MODELS" value={totalModels} color="var(--info)" />
      </div>

      {/* Add Provider Flow */}
      {showAdd && (
        <div style={card}>
          {step === "pick" ? (
            <>
              <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
                <div style={iconBadge}><IconPlus size={16} /></div>
                <div>
                  <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Choose a Provider</h2>
                  <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Select a preset or add a custom endpoint</p>
                </div>
              </div>
              {categories.map(cat => {
                const catPresets = presets.filter(p => p.category === cat.id);
                if (catPresets.length === 0) return null;
                return (
                  <div key={cat.id} style={{ marginBottom: "16px" }}>
                    <p style={{ fontSize: "11px", fontWeight: 600, color: "var(--fg-3)", letterSpacing: "0.5px", textTransform: "uppercase", marginBottom: "8px" }}>{cat.name}</p>
                    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: "8px" }}>
                      {catPresets.map(p => (
                        <button key={p.id} onClick={() => selectPreset(p)} style={presetBtn}>
                          <span style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)" }}>{p.name}</span>
                          <span style={{ fontSize: "11px", color: "var(--fg-3)", marginTop: "2px", display: "block" }}>{p.description}</span>
                        </button>
                      ))}
                    </div>
                  </div>
                );
              })}
            </>
          ) : (
            <>
              <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
                <button onClick={() => { setStep("pick"); setSelectedPreset(null); }} style={{ ...btnSmall, marginRight: "4px" }}><IconBack /></button>
                <div>
                  <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Configure {selectedPreset?.name || "Provider"}</h2>
                  <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>{selectedPreset?.description || "Enter connection details"}</p>
                </div>
              </div>
              {error && <div style={{ padding: "8px 12px", background: "var(--error-light)", color: "var(--error)", borderRadius: "6px", fontSize: "13px", marginBottom: "12px" }}>{error}</div>}
              <form onSubmit={addConnection}>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px", marginBottom: "12px" }}>
                  <div>
                    <label style={labelStyle}>Name</label>
                    <input style={inputStyle} value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
                  </div>
                  <div>
                    <label style={labelStyle}>Format</label>
                    <select style={inputStyle} value={form.format} onChange={e => setForm({ ...form, format: e.target.value })}>
                      <option value="openai">OpenAI Compatible</option>
                      <option value="anthropic">Anthropic</option>
                      <option value="commandcode">CommandCode</option>
                    </select>
                  </div>
                </div>
                <div style={{ marginBottom: "12px" }}>
                  <label style={labelStyle}>Base URL</label>
                  <input style={inputStyle} placeholder="https://api.example.com/v1" value={form.baseUrl} onChange={e => setForm({ ...form, baseUrl: e.target.value })} />
                </div>
                {!selectedPreset?.noAuth && (
                  <div style={{ marginBottom: "12px" }}>
                    <label style={labelStyle}>API Key</label>
                    <input style={inputStyle} type="password" placeholder="sk-..." value={form.apiKey} onChange={e => setForm({ ...form, apiKey: e.target.value })} />
                  </div>
                )}
                <details style={{ marginBottom: "16px" }}>
                  <summary style={{ fontSize: "12px", color: "var(--fg-3)", cursor: "pointer", marginBottom: "8px" }}>Advanced Settings</summary>
                  <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
                    <div>
                      <label style={labelStyle}>Chat Path</label>
                      <input style={inputStyle} value={form.chatPath} onChange={e => setForm({ ...form, chatPath: e.target.value })} />
                    </div>
                    <div>
                      <label style={labelStyle}>Models Path</label>
                      <input style={inputStyle} value={form.modelsPath} onChange={e => setForm({ ...form, modelsPath: e.target.value })} />
                    </div>
                    <div>
                      <label style={labelStyle}>Auth Header</label>
                      <input style={inputStyle} value={form.authHeader} onChange={e => setForm({ ...form, authHeader: e.target.value })} />
                    </div>
                    <div>
                      <label style={labelStyle}>Auth Prefix</label>
                      <input style={inputStyle} value={form.authPrefix} onChange={e => setForm({ ...form, authPrefix: e.target.value })} />
                    </div>
                  </div>
                </details>

                {/* Test Result */}
                {testStatus === "success" && (
                  <div style={{ padding: "10px 14px", background: "var(--success)" + "15", border: "1px solid var(--success)", borderRadius: "var(--radius-sm)", marginBottom: "12px" }}>
                    <p style={{ fontSize: "13px", color: "var(--success)", margin: 0, fontWeight: 500 }}>✓ {testMessage}</p>
                  </div>
                )}
                {testStatus === "failed" && (
                  <div style={{ padding: "10px 14px", background: "var(--error)" + "15", border: "1px solid var(--error)", borderRadius: "var(--radius-sm)", marginBottom: "12px" }}>
                    <p style={{ fontSize: "13px", color: "var(--error)", margin: 0, fontWeight: 500 }}>✗ {testMessage}</p>
                  </div>
                )}

                <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}>
                  <button type="button" onClick={() => { setShowAdd(false); setStep("pick"); setTestStatus(null); }} style={btnSecondary}>Cancel</button>
                  <button type="button" onClick={testConnection} disabled={testStatus === "testing" || !form.baseUrl || !form.apiKey} style={{ ...btnSecondary, color: testStatus === "testing" ? "var(--fg-3)" : "var(--info)", borderColor: "var(--info)" }}>
                    {testStatus === "testing" ? "Testing..." : "Test Connection"}
                  </button>
                  <button type="submit" disabled={testStatus !== "success"} style={{ ...btnPrimary, opacity: testStatus !== "success" ? 0.5 : 1, cursor: testStatus !== "success" ? "not-allowed" : "pointer" }}>Connect & Sync Models</button>
                </div>
              </form>
            </>
          )}
        </div>
      )}

      {/* Connections List */}
      {connections.length === 0 && !showAdd ? (
        <div style={card}>
          <EmptyState text="No providers connected yet. Click 'Add Provider' to get started." />
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
          {connections.map(conn => (
            <div key={conn.id} style={{ ...card, marginBottom: 0 }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <div style={{ display: "flex", alignItems: "center", gap: "12px", cursor: "pointer" }} onClick={() => toggleExpand(conn.id)}>
                  <div style={{ width: "10px", height: "10px", borderRadius: "50%", background: conn.is_active ? "var(--success)" : "var(--fg-3)", boxShadow: conn.is_active ? "0 0 6px rgba(16,185,129,0.4)" : "none" }} />
                  <div>
                    <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                      <span style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>{conn.name}</span>
                      <span style={formatBadge(conn.format)}>{conn.format}</span>
                      <IconChevron expanded={expanded[conn.id]} />
                    </div>
                    <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: "2px 0 0", fontFamily: "var(--mono)" }}>{conn.base_url}</p>
                  </div>
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                  <span style={{ fontSize: "12px", color: "var(--fg-2)", fontFamily: "var(--mono)" }}>{conn.models_count || 0} models</span>
                  {conn.last_sync && <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>synced {timeAgo(conn.last_sync)}</span>}
                  <button onClick={() => syncModels(conn.id)} disabled={syncing[conn.id]} style={btnSmall} title="Sync models">{syncing[conn.id] ? <IconSpinner /> : <IconRefresh />}</button>
                  <button onClick={() => toggleActive(conn.id, conn.is_active)} style={btnSmall} title={conn.is_active ? "Disable" : "Enable"}>{conn.is_active ? <IconPause /> : <IconPlay />}</button>
                  <button onClick={() => deleteConn(conn.id)} style={{ ...btnSmall, color: "var(--error)" }} title="Delete"><IconTrash /></button>
                </div>
              </div>
              {/* Expanded: show discovered models */}
              {expanded[conn.id] && (
                <div style={{ marginTop: "14px", paddingTop: "14px", borderTop: "1px solid var(--border)" }}>
                  {!connModels[conn.id] ? (
                    <div style={{ display: "flex", gap: "8px", flexWrap: "wrap" }}>
                      {[1,2,3].map(i => <div key={i} className="skeleton" style={{ width: "140px", height: "28px", borderRadius: "6px" }} />)}
                    </div>
                  ) : (
                    <div>
                      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "8px" }}>
                        <p style={{ fontSize: "11px", fontWeight: 600, color: "var(--fg-3)", letterSpacing: "0.5px", textTransform: "uppercase" }}>Models ({connModels[conn.id].length})</p>
                        <AddModelInline connectionId={conn.id} onAdded={() => { loadModels(conn.id); fetchConnections(); }} />
                      </div>
                      {connModels[conn.id].length === 0 ? (
                        <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>No models yet. Use "Add Model" above or click sync.</p>
                      ) : (
                        <div style={{ display: "flex", gap: "6px", flexWrap: "wrap" }}>
                          {connModels[conn.id].map(m => (
                            <span key={m.id} style={{ fontSize: "12px", padding: "4px 8px 4px 10px", background: m.is_active === 0 ? "var(--bg-body)" : "var(--bg-body)", border: m.is_active === 0 ? "1px dashed var(--border)" : "1px solid var(--border)", borderRadius: "6px", fontFamily: "var(--mono)", color: m.is_active === 0 ? "var(--fg-3)" : "var(--fg-1)", display: "inline-flex", alignItems: "center", gap: "6px", opacity: m.is_active === 0 ? 0.5 : 1, transition: "all 0.2s" }}>
                              <button onClick={() => toggleModelActive(conn.id, m.model_id, m.is_active)} style={{ background: "none", border: "none", cursor: "pointer", padding: "0", display: "flex", alignItems: "center" }} title={m.is_active ? "Disable model" : "Enable model"}>
                                <div style={{ width: "8px", height: "8px", borderRadius: "50%", background: m.is_active === 0 ? "var(--fg-3)" : "var(--success)", transition: "background 0.2s" }} />
                              </button>
                              {m.model_id}
                              <button onClick={() => removeModel(conn.id, m.model_id)} style={{ background: "none", border: "none", cursor: "pointer", padding: "0", display: "flex", opacity: 0.5 }} title="Remove model">
                                <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="var(--error)" strokeWidth="2.5" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                              </button>
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function timeAgo(dateStr) {
  const diff = Date.now() - new Date(dateStr + "Z").getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return mins + "m ago";
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return hrs + "h ago";
  return Math.floor(hrs / 24) + "d ago";
}

function formatBadge(format) {
  const colors = { openai: "var(--success)", anthropic: "var(--purple)", commandcode: "var(--info)" };
  const color = colors[format] || "var(--fg-2)";
  return { fontSize: "11px", padding: "2px 8px", borderRadius: "9999px", background: color + "18", color, fontWeight: 500 };
}

function MiniStat({ label, value, color }) {
  return (
    <div style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "16px 18px", boxShadow: "var(--shadow)", border: "1px solid var(--border)" }}>
      <p style={{ fontSize: "20px", fontWeight: 700, color: color || "var(--fg-0)", fontFamily: "var(--mono)", letterSpacing: "-0.3px", marginBottom: "2px" }}>{value}</p>
      <p style={{ fontSize: "11px", color: "var(--fg-3)", fontWeight: 500, letterSpacing: "0.5px" }}>{label}</p>
    </div>
  );
}

function EmptyState({ text }) {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <div style={{ width: "56px", height: "56px", borderRadius: "12px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", margin: "0 auto 16px" }}>
        <IconLink color="var(--fg-3)" size={24} />
      </div>
      <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--fg-1)", marginBottom: "4px" }}>No connections</p>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>{text}</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ marginBottom: "24px" }}><div className="skeleton" style={{ width: "320px", height: "14px", borderRadius: "6px" }} /></div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "16px", marginBottom: "24px" }}>
        {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: "70px", borderRadius: "var(--radius)" }} />)}
      </div>
      <div className="skeleton" style={{ height: "200px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

// Icons
function IconPlus({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>; }
function IconRefresh({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>; }
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconPause({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>; }
function IconPlay({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>; }
function IconLink({ size = 14, color }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color || "currentColor"} strokeWidth="2" strokeLinecap="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>; }
function IconSpinner() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ animation: "spin 1s linear infinite" }}><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>; }
function IconBack({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="15 18 9 12 15 6"/></svg>; }
function IconChevron({ expanded, size = 12 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ transition: "transform 0.2s", transform: expanded ? "rotate(90deg)" : "rotate(0deg)" }}><polyline points="9 18 15 12 9 6"/></svg>; }

function AddModelInline({ connectionId, onAdded }) {
  const [show, setShow] = useState(false);
  const [value, setValue] = useState("");
  const [adding, setAdding] = useState(false);

  const add = async () => {
    if (!value.trim()) return;
    setAdding(true);
    const models = value.split(",").map(m => m.trim()).filter(Boolean);
    await fetch("/api/models/manual", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ connectionId, models })
    });
    setAdding(false);
    setValue("");
    setShow(false);
    onAdded();
  };

  if (!show) {
    return <button onClick={() => setShow(true)} style={{ fontSize: "11px", padding: "4px 10px", background: "var(--primary-light)", color: "var(--primary)", border: "1px solid rgba(60,80,224,0.2)", borderRadius: "6px", cursor: "pointer", fontWeight: 500 }}>+ Add Model</button>;
  }

  return (
    <div style={{ display: "flex", gap: "6px", alignItems: "center" }}>
      <input
        value={value}
        onChange={e => setValue(e.target.value)}
        onKeyDown={e => e.key === "Enter" && add()}
        placeholder="model-id, another-model"
        style={{ padding: "4px 8px", fontSize: "12px", border: "1px solid var(--border)", borderRadius: "6px", background: "var(--bg-body)", color: "var(--fg-0)", width: "240px", outline: "none", fontFamily: "var(--mono)" }}
        autoFocus
      />
      <button onClick={add} disabled={adding || !value.trim()} style={{ fontSize: "11px", padding: "4px 10px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "6px", cursor: "pointer", fontWeight: 500 }}>
        {adding ? "..." : "Add"}
      </button>
      <button onClick={() => { setShow(false); setValue(""); }} style={{ fontSize: "11px", padding: "4px 8px", background: "none", border: "1px solid var(--border)", borderRadius: "6px", cursor: "pointer", color: "var(--fg-3)" }}>✕</button>
    </div>
  );
}

// Styles
const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", marginBottom: "16px" };
const iconBadge = { width: "36px", height: "36px", borderRadius: "8px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0 };
const btnPrimary = { display: "flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "var(--radius-sm)", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { padding: "8px 16px", background: "transparent", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSmall = { width: "30px", height: "30px", display: "flex", alignItems: "center", justifyContent: "center", background: "transparent", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", cursor: "pointer", color: "var(--fg-2)" };
const presetBtn = { padding: "12px 14px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", cursor: "pointer", textAlign: "left", transition: "var(--transition)" };
const labelStyle = { display: "block", fontSize: "12px", fontWeight: 500, color: "var(--fg-2)", marginBottom: "4px" };
const inputStyle = { width: "100%", padding: "8px 12px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "13px", color: "var(--fg-0)", outline: "none", boxSizing: "border-box" };


import { useState, useEffect } from "react";

export default function KeysPage() {
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ name: "", daily_limit: 0, monthly_limit: 0 });
  const [copied, setCopied] = useState(null);

  const load = () => { fetch("/api/keys", { credentials: "include" }).then(r => r.json()).then(d => { setKeys(d.data || []); setLoading(false); }).catch(() => setLoading(false)); };
  useEffect(() => { load(); }, []);

  const handleCreate = async () => {
    if (!form.name) return;
    await fetch("/api/keys", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ action: "create", ...form }) });
    setShowForm(false); setForm({ name: "", daily_limit: 0, monthly_limit: 0 }); load();
  };

  const handleDelete = async (id) => {
    if (!confirm("Delete this API key? This cannot be undone.")) return;
    await fetch("/api/keys", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ action: "delete", id }) }); load();
  };

  const copyKey = (key) => {
    navigator.clipboard.writeText(key);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  };

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>API Keys</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Manage access keys for the router API</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} style={btnPrimary}>
          {showForm ? (
            <><IconX /> Cancel</>
          ) : (
            <><IconPlus /> Create Key</>
          )}
        </button>
      </div>

      {/* Create Form */}
      {showForm && (
        <div style={card} className="fade-in">
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
            <div style={iconBadge}>
              <IconKey color="var(--primary)" />
            </div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>New API Key</p>
              <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>Set a name and optional usage limits</p>
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr 1fr", gap: "14px" }}>
            <div>
              <label style={labelSt}>Name</label>
              <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} style={input} placeholder="e.g. Hermes Agent" />
            </div>
            <div>
              <label style={labelSt}>Daily Limit</label>
              <input type="number" value={form.daily_limit} onChange={e => setForm({ ...form, daily_limit: parseInt(e.target.value) || 0 })} style={input} placeholder="0 = unlimited" />
            </div>
            <div>
              <label style={labelSt}>Monthly Limit</label>
              <input type="number" value={form.monthly_limit} onChange={e => setForm({ ...form, monthly_limit: parseInt(e.target.value) || 0 })} style={input} placeholder="0 = unlimited" />
            </div>
          </div>
          <div style={{ display: "flex", gap: "10px", marginTop: "16px" }}>
            <button onClick={handleCreate} style={btnPrimary}><IconCheck /> Create Key</button>
            <button onClick={() => setShowForm(false)} style={btnSecondary}>Cancel</button>
          </div>
        </div>
      )}

      {/* Summary */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "1px", background: "var(--border)", borderRadius: "var(--radius)", overflow: "hidden", border: "1px solid var(--border)", marginBottom: "24px" }}>
        <div style={metricCell}><p style={metricLabel}>Total Keys</p><p style={metricValue}>{keys.length}</p></div>
        <div style={metricCell}><p style={metricLabel}>With Limits</p><p style={metricValue}>{keys.filter(k => k.daily_limit || k.monthly_limit).length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Unlimited</p><p style={metricValue}>{keys.filter(k => !k.daily_limit && !k.monthly_limit).length}</p></div>
      </div>

      {/* Keys Table */}
      <div style={{ ...card, padding: 0, overflow: "hidden" }}>
        {keys.length === 0 ? (
          <EmptyState />
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }}>
            <thead>
              <tr style={{ background: "var(--bg-body)" }}>
                {["Name", "Key", "Daily Limit", "Monthly Limit", ""].map(h => (
                  <th key={h} style={th}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {keys.map(k => (
                <tr key={k.id} style={row}>
                  <td style={td}>
                    <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                      <div style={{ width: "28px", height: "28px", borderRadius: "6px", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center" }}>
                        <IconKey color="var(--primary)" size={12} />
                      </div>
                      <span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{k.name || "Unnamed"}</span>
                    </div>
                  </td>
                  <td style={td}>
                    <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
                      <code style={mono}>{k.key?.slice(0, 12)}...{k.key?.slice(-4)}</code>
                      <button onClick={() => copyKey(k.key)} style={copyBtn} title="Copy key">
                        {copied === k.key ? <IconCheck color="var(--success)" size={12} /> : <IconCopy size={12} />}
                      </button>
                    </div>
                  </td>
                  <td style={td}>
                    {k.daily_limit ? (
                      <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                        <code style={mono}>{fmt(k.daily_limit)}</code>
                        <div style={progressBg}><div style={{ ...progressFill, width: "45%" }} /></div>
                      </div>
                    ) : <span style={unlimited}>Unlimited</span>}
                  </td>
                  <td style={td}>
                    {k.monthly_limit ? (
                      <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                        <code style={mono}>{fmt(k.monthly_limit)}</code>
                        <div style={progressBg}><div style={{ ...progressFill, width: "30%" }} /></div>
                      </div>
                    ) : <span style={unlimited}>Unlimited</span>}
                  </td>
                  <td style={{ ...td, textAlign: "right" }}>
                    <button onClick={() => handleDelete(k.id)} style={btnDangerSmall} title="Delete key">
                      <IconTrash size={14} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

/* Icons */
function IconPlus() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>; }
function IconX() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>; }
function IconCheck({ color = "currentColor", size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><polyline points="20 6 9 17 4 12"/></svg>; }
function IconKey({ color = "currentColor", size = 16 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 0-7.778 7.778 5.5 5.5 0 0 0 7.777 0L15.5 15.5m0 0l2.5 2.5M15.5 15.5l2.5-2.5"/></svg>; }
function IconCopy({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>; }
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }

function EmptyState() {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <div style={{ width: "56px", height: "56px", borderRadius: "12px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", margin: "0 auto 16px" }}>
        <IconKey color="var(--fg-3)" size={24} />
      </div>
      <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--fg-1)", marginBottom: "4px" }}>No API keys yet</p>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Create a key to authenticate requests to the router</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div><div className="skeleton" style={{ width: "100px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "220px", height: "14px", borderRadius: "6px" }} /></div>
        <div className="skeleton" style={{ width: "120px", height: "36px", borderRadius: "6px" }} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "1px", marginBottom: "24px" }}>
        {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: "70px" }} />)}
      </div>
      <div className="skeleton" style={{ height: "250px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

function fmt(n) { if (n >= 1000000) return (n / 1000000).toFixed(1) + "M"; if (n >= 1000) return Math.round(n / 1000) + "K"; return String(n); }

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
const unlimited = { fontSize: "12px", color: "var(--fg-3)", fontStyle: "italic" };
const progressBg = { width: "50px", height: "4px", borderRadius: "2px", background: "var(--border)", overflow: "hidden" };
const progressFill = { height: "100%", borderRadius: "2px", background: "var(--primary)", transition: "width 0.3s ease" };
const copyBtn = { display: "flex", alignItems: "center", justifyContent: "center", width: "24px", height: "24px", background: "none", border: "1px solid var(--border)", borderRadius: "4px", cursor: "pointer", color: "var(--fg-3)", transition: "all 0.15s ease" };
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { padding: "8px 16px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnDangerSmall = { display: "flex", alignItems: "center", justifyContent: "center", width: "30px", height: "30px", background: "none", border: "1px solid var(--border)", borderRadius: "6px", color: "var(--error)", cursor: "pointer", transition: "all 0.15s ease" };

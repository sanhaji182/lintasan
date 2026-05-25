
import { useState, useEffect } from "react";

export default function WebhooksPage() {
  const [webhooks, setWebhooks] = useState([]);
  const [history, setHistory] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ name: "", url: "", secret: "", events: [] });
  const [testing, setTesting] = useState(null);

  const allEvents = ["request.completed", "request.failed", "cache.hit", "cache.miss", "key.created", "key.deleted", "model.fallback", "rate.limited"];

  const load = () => {
    fetch("/api/webhooks", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setWebhooks(d.data?.webhooks || d.data || []); setHistory(d.data?.history || []); setLoading(false); })
      .catch(() => setLoading(false));
  };
  useEffect(() => { load(); }, []);

  const handleCreate = async () => {
    if (!form.name || !form.url) return;
    await fetch("/api/webhooks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "create", ...form })
    });
    setShowForm(false);
    setForm({ name: "", url: "", secret: "", events: [] });
    load();
  };

  const toggleActive = async (id, active) => {
    await fetch("/api/webhooks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "toggle", id, active: !active })
    });
    load();
  };

  const handleDelete = async (id) => {
    if (!confirm("Delete this webhook?")) return;
    await fetch("/api/webhooks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "delete", id })
    });
    load();
  };

  const testWebhook = async (id) => {
    setTesting(id);
    try {
      await fetch("/api/webhooks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ action: "test", id })
      });
    } catch {}
    setTimeout(() => setTesting(null), 2000);
    load();
  };

  const toggleEvent = (event) => {
    setForm(f => ({
      ...f,
      events: f.events.includes(event) ? f.events.filter(e => e !== event) : [...f.events, event]
    }));
  };

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Webhooks</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Configure event notifications to external services</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} style={btnPrimary}>
          {showForm ? <><IconX /> Cancel</> : <><IconPlus /> Add Webhook</>}
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "1px", background: "var(--border)", borderRadius: "var(--radius)", overflow: "hidden", border: "1px solid var(--border)", marginBottom: "24px" }}>
        <div style={metricCell}><p style={metricLabel}>Total Webhooks</p><p style={metricValue}>{webhooks.length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Active</p><p style={metricValue}>{webhooks.filter(w => w.active !== false).length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Notifications Sent</p><p style={metricValue}>{history.length}</p></div>
      </div>

      {/* Create Form */}
      {showForm && (
        <div style={card} className="fade-in">
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
            <div style={iconBadge}><IconWebhook color="var(--primary)" /></div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>New Webhook</p>
              <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>Send event notifications to a URL</p>
            </div>
          </div>
          <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: "1fr 2fr 1fr", gap: "14px", marginBottom: "14px" }}>
            <div>
              <label style={labelSt}>Name</label>
              <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} style={input} placeholder="e.g. Slack Alerts" />
            </div>
            <div>
              <label style={labelSt}>URL</label>
              <input value={form.url} onChange={e => setForm({ ...form, url: e.target.value })} style={input} placeholder="https://hooks.example.com/webhook" />
            </div>
            <div>
              <label style={labelSt}>Secret (optional)</label>
              <input value={form.secret} onChange={e => setForm({ ...form, secret: e.target.value })} style={input} placeholder="signing secret" />
            </div>
          </div>
          <div>
            <label style={labelSt}>Events</label>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "8px" }}>
              {allEvents.map(event => (
                <label key={event} style={{ display: "flex", alignItems: "center", gap: "6px", padding: "6px 12px", borderRadius: "6px", border: "1px solid " + (form.events.includes(event) ? "var(--primary)" : "var(--border)"), background: form.events.includes(event) ? "var(--primary-light)" : "var(--bg-body)", cursor: "pointer", fontSize: "12px", color: form.events.includes(event) ? "var(--primary)" : "var(--fg-2)", fontWeight: 500, transition: "all 0.15s ease" }}>
                  <input type="checkbox" checked={form.events.includes(event)} onChange={() => toggleEvent(event)} style={{ display: "none" }} />
                  {event}
                </label>
              ))}
            </div>
          </div>
          <div style={{ display: "flex", gap: "10px", marginTop: "16px" }}>
            <button onClick={handleCreate} style={btnPrimary}><IconCheck /> Create Webhook</button>
            <button onClick={() => setShowForm(false)} style={btnSecondary}>Cancel</button>
          </div>
        </div>
      )}

      {/* Webhooks Table */}
      <div style={{ ...card, padding: 0, overflow: "hidden", marginBottom: "24px" }}>
        {webhooks.length === 0 ? (
          <EmptyState text="No webhooks configured" />
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }} className="responsive-table">
            <thead>
              <tr style={{ background: "var(--bg-body)" }}>
                {["Name", "URL", "Events", "Status", "Actions"].map(h => <th key={h} style={th}>{h}</th>)}
              </tr>
            </thead>
            <tbody>
              {webhooks.map(w => (
                <tr key={w.id || w.name} style={row}>
                  <td style={td}><span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{w.name}</span></td>
                  <td style={td}><code style={mono}>{w.url?.length > 40 ? w.url.slice(0, 40) + "..." : w.url}</code></td>
                  <td style={td}>
                    <div style={{ display: "flex", gap: "4px", flexWrap: "wrap" }}>
                      {(w.events || []).slice(0, 3).map(e => (
                        <span key={e} style={{ fontSize: "10px", padding: "2px 6px", borderRadius: "3px", background: "var(--bg-body)", border: "1px solid var(--border)", color: "var(--fg-2)" }}>{e}</span>
                      ))}
                      {(w.events || []).length > 3 && <span style={{ fontSize: "10px", padding: "2px 6px", color: "var(--fg-3)" }}>+{w.events.length - 3}</span>}
                    </div>
                  </td>
                  <td style={td}>
                    <button onClick={() => toggleActive(w.id, w.active !== false)} style={{ display: "flex", alignItems: "center", gap: "6px", padding: "4px 10px", borderRadius: "12px", border: "none", cursor: "pointer", fontSize: "11px", fontWeight: 500, background: w.active !== false ? "var(--success-light)" : "var(--error-light)", color: w.active !== false ? "var(--success)" : "var(--error)" }}>
                      <div style={{ width: "6px", height: "6px", borderRadius: "50%", background: w.active !== false ? "var(--success)" : "var(--error)" }} />
                      {w.active !== false ? "Active" : "Inactive"}
                    </button>
                  </td>
                  <td style={{ ...td, textAlign: "right" }}>
                    <div style={{ display: "flex", gap: "6px", justifyContent: "flex-end" }}>
                      <button onClick={() => testWebhook(w.id)} style={btnSmall} disabled={testing === w.id}>
                        {testing === w.id ? "Sent ✓" : "Test"}
                      </button>
                      <button onClick={() => handleDelete(w.id)} style={btnDangerSmall}><IconTrash size={14} /></button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Recent History */}
      {history.length > 0 && (
        <div>
          <h2 style={sectionTitle}>Recent Notifications</h2>
          <div style={{ ...card, padding: 0, overflow: "hidden" }}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }} className="responsive-table">
              <thead>
                <tr style={{ background: "var(--bg-body)" }}>
                  {["Webhook", "Event", "Status", "Time"].map(h => <th key={h} style={th}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {history.slice(0, 10).map((h, i) => (
                  <tr key={i} style={row}>
                    <td style={td}><span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{h.webhook_name || h.webhook}</span></td>
                    <td style={td}><code style={mono}>{h.event}</code></td>
                    <td style={td}>
                      <span style={{ fontSize: "11px", padding: "3px 8px", borderRadius: "4px", fontWeight: 500, background: h.success ? "var(--success-light)" : "var(--error-light)", color: h.success ? "var(--success)" : "var(--error)" }}>
                        {h.success ? "Delivered" : "Failed"}
                      </span>
                    </td>
                    <td style={td}><span style={{ fontSize: "12px", color: "var(--fg-3)" }}>{h.timestamp ? new Date(h.timestamp).toLocaleString() : "—"}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

/* Icons */
function IconPlus() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>; }
function IconX() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>; }
function IconCheck() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="20 6 9 17 4 12"/></svg>; }
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconWebhook({ color = "currentColor" }) { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><path d="M18 16.98h1.67c1.47 0 2.67-1.2 2.67-2.67v0c0-1.47-1.2-2.67-2.67-2.67H18"/><path d="M6 16.98H4.33c-1.47 0-2.67-1.2-2.67-2.67v0c0-1.47 1.2-2.67 2.67-2.67H6"/><circle cx="12" cy="12" r="4"/><line x1="12" y1="8" x2="12" y2="2"/><line x1="12" y1="22" x2="12" y2="16"/></svg>; }

function EmptyState({ text }) {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <div style={{ width: "56px", height: "56px", borderRadius: "12px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", margin: "0 auto 16px" }}>
        <IconWebhook color="var(--fg-3)" />
      </div>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>{text}</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div><div className="skeleton" style={{ width: "100px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "280px", height: "14px", borderRadius: "6px" }} /></div>
        <div className="skeleton" style={{ width: "140px", height: "36px", borderRadius: "6px" }} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "1px", marginBottom: "24px" }}>
        {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: "70px" }} />)}
      </div>
      <div className="skeleton" style={{ height: "250px", borderRadius: "var(--radius)" }} />
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
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { padding: "8px 16px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSmall = { padding: "5px 12px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "4px", fontSize: "11px", fontWeight: 500, cursor: "pointer" };
const btnDangerSmall = { display: "flex", alignItems: "center", justifyContent: "center", width: "30px", height: "30px", background: "none", border: "1px solid var(--border)", borderRadius: "6px", color: "var(--error)", cursor: "pointer", transition: "all 0.15s ease" };

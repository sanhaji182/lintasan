"use client";
import { useState, useEffect } from "react";

export default function TeamsPage() {
  const [teams, setTeams] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ name: "", description: "" });
  const [selected, setSelected] = useState(null);

  const load = () => {
    fetch("/api/teams", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setTeams(d.data || []); setLoading(false); })
      .catch(() => setLoading(false));
  };
  useEffect(() => { load(); }, []);

  const handleCreate = async () => {
    if (!form.name) return;
    await fetch("/api/teams", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "create", ...form })
    });
    setShowForm(false);
    setForm({ name: "", description: "" });
    load();
  };

  const handleDelete = async (id) => {
    if (!confirm("Delete this team? Members will be unassigned.")) return;
    await fetch("/api/teams", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "delete", id })
    });
    if (selected?.id === id) setSelected(null);
    load();
  };

  const selectTeam = async (team) => {
    setSelected(team);
    try {
      const r = await fetch(`/api/teams?id=${team.id}`, { credentials: "include" });
      const d = await r.json();
      setSelected(d.data || team);
    } catch {}
  };

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Teams</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Organize users into teams with shared quotas</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} style={btnPrimary}>
          {showForm ? <><IconX /> Cancel</> : <><IconPlus /> New Team</>}
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "1px", background: "var(--border)", borderRadius: "var(--radius)", overflow: "hidden", border: "1px solid var(--border)", marginBottom: "24px" }}>
        <div style={metricCell}><p style={metricLabel}>Total Teams</p><p style={metricValue}>{teams.length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Total Members</p><p style={metricValue}>{teams.reduce((s, t) => s + (t.member_count || 0), 0)}</p></div>
        <div style={metricCell}><p style={metricLabel}>Active Teams</p><p style={metricValue}>{teams.filter(t => (t.member_count || 0) > 0).length}</p></div>
      </div>

      {/* Create Form */}
      {showForm && (
        <div style={card} className="fade-in">
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
            <div style={iconBadge}><IconTeam color="var(--primary)" /></div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>Create Team</p>
              <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>Add a new team to organize users</p>
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 2fr", gap: "14px" }}>
            <div>
              <label style={labelSt}>Team Name</label>
              <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} style={input} placeholder="e.g. Engineering" />
            </div>
            <div>
              <label style={labelSt}>Description</label>
              <input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} style={input} placeholder="Optional description" />
            </div>
          </div>
          <div style={{ display: "flex", gap: "10px", marginTop: "16px" }}>
            <button onClick={handleCreate} style={btnPrimary}><IconCheck /> Create Team</button>
            <button onClick={() => setShowForm(false)} style={btnSecondary}>Cancel</button>
          </div>
        </div>
      )}

      {/* Content */}
      <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: selected ? "1fr 1fr" : "1fr", gap: "20px" }}>
        {/* Teams List */}
        <div style={{ ...card, padding: 0, overflow: "hidden" }}>
          {teams.length === 0 ? (
            <EmptyState text="No teams created yet" />
          ) : (
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }}>
              <thead>
                <tr style={{ background: "var(--bg-body)" }}>
                  {["Team", "Members", "Usage", ""].map(h => <th key={h} style={th}>{h}</th>)}
                </tr>
              </thead>
              <tbody>
                {teams.map(t => (
                  <tr key={t.id} style={{ ...row, background: selected?.id === t.id ? "var(--primary-light)" : "transparent", cursor: "pointer" }} onClick={() => selectTeam(t)}>
                    <td style={td}>
                      <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                        <div style={{ width: "28px", height: "28px", borderRadius: "6px", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center" }}>
                          <IconTeam color="var(--primary)" size={12} />
                        </div>
                        <div>
                          <span style={{ fontWeight: 500, color: "var(--fg-0)", display: "block" }}>{t.name}</span>
                          {t.description && <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>{t.description}</span>}
                        </div>
                      </div>
                    </td>
                    <td style={td}><code style={mono}>{t.member_count || 0}</code></td>
                    <td style={td}><code style={mono}>{fmt(t.usage || 0)}</code></td>
                    <td style={{ ...td, textAlign: "right" }}>
                      <button onClick={(e) => { e.stopPropagation(); handleDelete(t.id); }} style={btnDangerSmall}><IconTrash size={14} /></button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        {/* Team Detail */}
        {selected && (
          <div style={card} className="fade-in">
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "16px" }}>
              <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                <div style={iconBadge}><IconTeam color="var(--primary)" /></div>
                <div>
                  <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>{selected.name}</p>
                  <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>{selected.description || "No description"}</p>
                </div>
              </div>
              <button onClick={() => setSelected(null)} style={btnSecondary}>Close</button>
            </div>

            <h3 style={{ fontSize: "12px", fontWeight: 600, color: "var(--fg-3)", textTransform: "uppercase", letterSpacing: "0.5px", marginBottom: "12px" }}>Members</h3>
            {(selected.members || []).length === 0 ? (
              <p style={{ fontSize: "13px", color: "var(--fg-3)", fontStyle: "italic" }}>No members in this team</p>
            ) : (
              <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
                {(selected.members || []).map((m, i) => (
                  <div key={i} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 14px", borderRadius: "6px", background: "var(--bg-body)", border: "1px solid var(--border)" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                      <div style={{ width: "24px", height: "24px", borderRadius: "50%", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: "10px", fontWeight: 600, color: "var(--primary)" }}>
                        {(m.username || m.name || "?")[0].toUpperCase()}
                      </div>
                      <span style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)" }}>{m.username || m.name}</span>
                    </div>
                    <span style={{ ...roleBadge, background: "var(--primary-light)", color: "var(--primary)" }}>{m.role || "member"}</span>
                  </div>
                ))}
              </div>
            )}

            {selected.usage !== undefined && (
              <div style={{ marginTop: "16px", padding: "12px 14px", borderRadius: "6px", background: "var(--bg-body)", border: "1px solid var(--border)" }}>
                <p style={{ fontSize: "11px", color: "var(--fg-3)", textTransform: "uppercase", marginBottom: "4px" }}>Total Usage</p>
                <p style={{ fontSize: "18px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)" }}>{fmt(selected.usage || 0)} tokens</p>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/* Icons */
function IconPlus() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>; }
function IconX() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>; }
function IconCheck() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="20 6 9 17 4 12"/></svg>; }
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconTeam({ color = "currentColor", size = 16 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>; }

function EmptyState({ text }) {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <div style={{ width: "56px", height: "56px", borderRadius: "12px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", margin: "0 auto 16px" }}>
        <IconTeam color="var(--fg-3)" />
      </div>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>{text}</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div><div className="skeleton" style={{ width: "80px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "240px", height: "14px", borderRadius: "6px" }} /></div>
        <div className="skeleton" style={{ width: "120px", height: "36px", borderRadius: "6px" }} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "1px", marginBottom: "24px" }}>
        {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: "70px" }} />)}
      </div>
      <div className="skeleton" style={{ height: "300px", borderRadius: "var(--radius)" }} />
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
const roleBadge = { fontSize: "11px", padding: "3px 8px", borderRadius: "4px", fontWeight: 500 };
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { padding: "8px 16px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnDangerSmall = { display: "flex", alignItems: "center", justifyContent: "center", width: "30px", height: "30px", background: "none", border: "1px solid var(--border)", borderRadius: "6px", color: "var(--error)", cursor: "pointer", transition: "all 0.15s ease" };

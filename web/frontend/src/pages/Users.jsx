
import { useState, useEffect } from "react";

export default function UsersPage() {
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ username: "", password: "", role: "viewer", email: "" });

  const load = () => {
    fetch("/api/users", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setUsers(d.data || []); setLoading(false); })
      .catch(() => setLoading(false));
  };
  useEffect(() => { load(); }, []);

  const handleCreate = async () => {
    if (!form.username || !form.password) return;
    await fetch("/api/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "create", ...form })
    });
    setShowForm(false);
    setForm({ username: "", password: "", role: "viewer", email: "" });
    load();
  };

  const toggleActive = async (id, active) => {
    await fetch("/api/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "toggle", id, active: !active })
    });
    load();
  };

  const handleDelete = async (id) => {
    if (!confirm("Delete this user? This cannot be undone.")) return;
    await fetch("/api/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "delete", id })
    });
    load();
  };

  if (loading) return <LoadingSkeleton />;

  const roleColors = {
    admin: { bg: "var(--error-light)", color: "var(--error)" },
    editor: { bg: "var(--warning-light)", color: "var(--warning)" },
    viewer: { bg: "var(--primary-light)", color: "var(--primary)" }
  };

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Users</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Manage user accounts and access roles</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} style={btnPrimary}>
          {showForm ? <><IconX /> Cancel</> : <><IconPlus /> Add User</>}
        </button>
      </div>

      {/* Stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "1px", background: "var(--border)", borderRadius: "var(--radius)", overflow: "hidden", border: "1px solid var(--border)", marginBottom: "24px" }}>
        <div style={metricCell}><p style={metricLabel}>Total Users</p><p style={metricValue}>{users.length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Admins</p><p style={metricValue}>{users.filter(u => u.role === "admin").length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Active</p><p style={metricValue}>{users.filter(u => u.active !== false).length}</p></div>
        <div style={metricCell}><p style={metricLabel}>Inactive</p><p style={metricValue}>{users.filter(u => u.active === false).length}</p></div>
      </div>

      {/* Create Form */}
      {showForm && (
        <div style={card} className="fade-in">
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
            <div style={iconBadge}><IconUser color="var(--primary)" /></div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>New User</p>
              <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>Create a new user account</p>
            </div>
          </div>
          <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr 1fr", gap: "14px" }}>
            <div>
              <label style={labelSt}>Username</label>
              <input value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} style={input} placeholder="username" />
            </div>
            <div>
              <label style={labelSt}>Password</label>
              <input type="password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} style={input} placeholder="••••••••" />
            </div>
            <div>
              <label style={labelSt}>Email</label>
              <input type="email" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} style={input} placeholder="user@example.com" />
            </div>
            <div>
              <label style={labelSt}>Role</label>
              <select value={form.role} onChange={e => setForm({ ...form, role: e.target.value })} style={input}>
                <option value="viewer">Viewer</option>
                <option value="editor">Editor</option>
                <option value="admin">Admin</option>
              </select>
            </div>
          </div>
          <div style={{ display: "flex", gap: "10px", marginTop: "16px" }}>
            <button onClick={handleCreate} style={btnPrimary}><IconCheck /> Create User</button>
            <button onClick={() => setShowForm(false)} style={btnSecondary}>Cancel</button>
          </div>
        </div>
      )}

      {/* Users Table */}
      <div style={{ ...card, padding: 0, overflow: "hidden" }}>
        {users.length === 0 ? (
          <EmptyState />
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }} className="responsive-table">
            <thead>
              <tr style={{ background: "var(--bg-body)" }}>
                {["User", "Email", "Role", "Status", ""].map(h => <th key={h} style={th}>{h}</th>)}
              </tr>
            </thead>
            <tbody>
              {users.map(u => (
                <tr key={u.id} style={row}>
                  <td style={td}>
                    <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                      <div style={{ width: "32px", height: "32px", borderRadius: "50%", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: "12px", fontWeight: 600, color: "var(--primary)" }}>
                        {(u.username || "?")[0].toUpperCase()}
                      </div>
                      <span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{u.username}</span>
                    </div>
                  </td>
                  <td style={td}><span style={{ color: "var(--fg-2)", fontSize: "12px" }}>{u.email || "—"}</span></td>
                  <td style={td}>
                    <span style={{ fontSize: "11px", padding: "3px 10px", borderRadius: "4px", fontWeight: 600, background: (roleColors[u.role] || roleColors.viewer).bg, color: (roleColors[u.role] || roleColors.viewer).color }}>
                      {u.role || "viewer"}
                    </span>
                  </td>
                  <td style={td}>
                    <button onClick={() => toggleActive(u.id, u.active !== false)} style={{ display: "flex", alignItems: "center", gap: "6px", padding: "4px 10px", borderRadius: "12px", border: "none", cursor: "pointer", fontSize: "11px", fontWeight: 500, background: u.active !== false ? "var(--success-light)" : "var(--error-light)", color: u.active !== false ? "var(--success)" : "var(--error)" }}>
                      <div style={{ width: "6px", height: "6px", borderRadius: "50%", background: u.active !== false ? "var(--success)" : "var(--error)" }} />
                      {u.active !== false ? "Active" : "Inactive"}
                    </button>
                  </td>
                  <td style={{ ...td, textAlign: "right" }}>
                    <button onClick={() => handleDelete(u.id)} style={btnDangerSmall}><IconTrash size={14} /></button>
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
function IconCheck() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="20 6 9 17 4 12"/></svg>; }
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconUser({ color = "currentColor" }) { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>; }

function EmptyState() {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <div style={{ width: "56px", height: "56px", borderRadius: "12px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", margin: "0 auto 16px" }}>
        <IconUser color="var(--fg-3)" />
      </div>
      <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--fg-1)", marginBottom: "4px" }}>No users yet</p>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Create a user to manage access</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div><div className="skeleton" style={{ width: "80px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "220px", height: "14px", borderRadius: "6px" }} /></div>
        <div className="skeleton" style={{ width: "120px", height: "36px", borderRadius: "6px" }} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "1px", marginBottom: "24px" }}>
        {[1,2,3,4].map(i => <div key={i} className="skeleton" style={{ height: "70px" }} />)}
      </div>
      <div className="skeleton" style={{ height: "300px", borderRadius: "var(--radius)" }} />
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
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { padding: "8px 16px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnDangerSmall = { display: "flex", alignItems: "center", justifyContent: "center", width: "30px", height: "30px", background: "none", border: "1px solid var(--border)", borderRadius: "6px", color: "var(--error)", cursor: "pointer", transition: "all 0.15s ease" };

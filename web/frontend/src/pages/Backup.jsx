
import { useState, useEffect } from "react";

export default function BackupPage() {
  const [backups, setBackups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [exporting, setExporting] = useState(null);

  const load = () => {
    fetch("/api/backup", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setBackups(d.data || []); setLoading(false); })
      .catch(() => setLoading(false));
  };
  useEffect(() => { load(); }, []);

  const createBackup = async () => {
    setCreating(true);
    try {
      await fetch("/api/backup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ action: "create" })
      });
      load();
    } catch {}
    setCreating(false);
  };

  const handleExport = async (type) => {
    setExporting(type);
    try {
      const r = await fetch("/api/backup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ action: "export", type })
      });
      const blob = await r.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      const ext = type === "analytics" ? "csv" : "json";
      a.download = `lintasan-${type}-${new Date().toISOString().slice(0, 10)}.${ext}`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {}
    setExporting(null);
  };

  const handleRestore = async (filename) => {
    if (!confirm(`Restore from backup "${filename}"? This will overwrite current configuration.`)) return;
    if (!confirm("Are you absolutely sure? This action cannot be undone.")) return;
    await fetch("/api/backup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "restore", filename })
    });
    load();
  };

  const handleDelete = async (filename) => {
    if (!confirm(`Delete backup "${filename}"?`)) return;
    await fetch("/api/backup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ action: "delete", filename })
    });
    load();
  };

  const formatSize = (bytes) => {
    if (!bytes) return "—";
    if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + " MB";
    if (bytes >= 1024) return (bytes / 1024).toFixed(1) + " KB";
    return bytes + " B";
  };

  const formatDate = (d) => {
    if (!d) return "—";
    return new Date(d).toLocaleString();
  };

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Backup & Export</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Create backups and export configuration data</p>
        </div>
        <button onClick={createBackup} disabled={creating} style={btnPrimary}>
          {creating ? <><IconSpinner /> Creating...</> : <><IconPlus /> Create Backup</>}
        </button>
      </div>

      {/* Export Cards */}
      <div className="responsive-grid" style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "16px", marginBottom: "24px" }}>
        <div style={card}>
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "12px" }}>
            <div style={{ ...iconBadge, background: "var(--primary-light)" }}><IconConfig color="var(--primary)" /></div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>Config JSON</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)" }}>Export all configuration</p>
            </div>
          </div>
          <button onClick={() => handleExport("config")} disabled={exporting === "config"} style={{ ...btnPrimary, width: "100%", justifyContent: "center" }}>
            {exporting === "config" ? "Exporting..." : "Export Config"}
          </button>
        </div>

        <div style={card}>
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "12px" }}>
            <div style={{ ...iconBadge, background: "var(--success-light)" }}><IconChart color="var(--success)" /></div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>Analytics CSV</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)" }}>Export usage analytics</p>
            </div>
          </div>
          <button onClick={() => handleExport("analytics")} disabled={exporting === "analytics"} style={{ ...btnPrimary, width: "100%", justifyContent: "center", background: "var(--success)" }}>
            {exporting === "analytics" ? "Exporting..." : "Export Analytics"}
          </button>
        </div>

        <div style={card}>
          <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "12px" }}>
            <div style={{ ...iconBadge, background: "var(--warning-light)" }}><IconArchive color="var(--warning)" /></div>
            <div>
              <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>Full Export</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)" }}>Config + analytics + logs</p>
            </div>
          </div>
          <button onClick={() => handleExport("full")} disabled={exporting === "full"} style={{ ...btnPrimary, width: "100%", justifyContent: "center", background: "var(--warning)" }}>
            {exporting === "full" ? "Exporting..." : "Full Export"}
          </button>
        </div>
      </div>

      {/* Backups Table */}
      <h2 style={sectionTitle}>Saved Backups</h2>
      <div style={{ ...card, padding: 0, overflow: "hidden" }}>
        {backups.length === 0 ? (
          <EmptyState />
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }} className="responsive-table">
            <thead>
              <tr style={{ background: "var(--bg-body)" }}>
                {["Filename", "Size", "Created", "Actions"].map(h => <th key={h} style={th}>{h}</th>)}
              </tr>
            </thead>
            <tbody>
              {backups.map((b, i) => (
                <tr key={i} style={row}>
                  <td style={td}>
                    <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                      <div style={{ width: "28px", height: "28px", borderRadius: "6px", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center" }}>
                        <IconArchive color="var(--primary)" size={12} />
                      </div>
                      <span style={{ fontWeight: 500, color: "var(--fg-0)", fontFamily: "var(--mono)", fontSize: "12px" }}>{b.filename || b.name}</span>
                    </div>
                  </td>
                  <td style={td}><code style={mono}>{formatSize(b.size)}</code></td>
                  <td style={td}><span style={{ fontSize: "12px", color: "var(--fg-2)" }}>{formatDate(b.created_at || b.date)}</span></td>
                  <td style={{ ...td, textAlign: "right" }}>
                    <div style={{ display: "flex", gap: "6px", justifyContent: "flex-end" }}>
                      <button onClick={() => handleRestore(b.filename || b.name)} style={btnRestore}>
                        <IconRestore size={12} /> Restore
                      </button>
                      <button onClick={() => handleDelete(b.filename || b.name)} style={btnDangerSmall}><IconTrash size={14} /></button>
                    </div>
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
function IconTrash({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconConfig({ color = "currentColor" }) { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>; }
function IconChart({ color = "currentColor" }) { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/></svg>; }
function IconArchive({ color = "currentColor", size = 16 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round"><polyline points="21 8 21 21 3 21 3 8"/><rect x="1" y="3" width="22" height="5"/><line x1="10" y1="12" x2="14" y2="12"/></svg>; }
function IconRestore({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"/></svg>; }
function IconSpinner() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" style={{ animation: "spin 1s linear infinite" }}><line x1="12" y1="2" x2="12" y2="6"/><line x1="12" y1="18" x2="12" y2="22"/><line x1="4.93" y1="4.93" x2="7.76" y2="7.76"/><line x1="16.24" y1="16.24" x2="19.07" y2="19.07"/><line x1="2" y1="12" x2="6" y2="12"/><line x1="18" y1="12" x2="22" y2="12"/><line x1="4.93" y1="19.07" x2="7.76" y2="16.24"/><line x1="16.24" y1="7.76" x2="19.07" y2="4.93"/></svg>; }

function EmptyState() {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <div style={{ width: "56px", height: "56px", borderRadius: "12px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", margin: "0 auto 16px" }}>
        <IconArchive color="var(--fg-3)" />
      </div>
      <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--fg-1)", marginBottom: "4px" }}>No backups yet</p>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Create a backup to save your current configuration</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div><div className="skeleton" style={{ width: "140px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "260px", height: "14px", borderRadius: "6px" }} /></div>
        <div className="skeleton" style={{ width: "140px", height: "36px", borderRadius: "6px" }} />
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "16px", marginBottom: "24px" }}>
        {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: "120px", borderRadius: "var(--radius)" }} />)}
      </div>
      <div className="skeleton" style={{ height: "250px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", marginBottom: "16px" };
const iconBadge = { width: "36px", height: "36px", borderRadius: "8px", display: "flex", alignItems: "center", justifyContent: "center" };
const sectionTitle = { fontSize: "14px", fontWeight: 600, color: "var(--fg-1)", marginBottom: "12px" };
const th = { textAlign: "left", padding: "12px 16px", fontWeight: 500, color: "var(--fg-3)", fontSize: "11px", borderBottom: "1px solid var(--border)", textTransform: "uppercase", letterSpacing: "0.5px" };
const td = { padding: "14px 16px", verticalAlign: "middle" };
const row = { borderBottom: "1px solid var(--border)", transition: "background 0.1s ease" };
const mono = { fontFamily: "var(--mono)", fontSize: "12px", color: "var(--fg-2)", background: "var(--bg-body)", padding: "3px 8px", borderRadius: "4px" };
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "8px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "6px", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnRestore = { display: "inline-flex", alignItems: "center", gap: "4px", padding: "5px 12px", background: "var(--warning-light)", color: "var(--warning)", border: "1px solid rgba(245,158,11,0.3)", borderRadius: "4px", fontSize: "11px", fontWeight: 500, cursor: "pointer" };
const btnDangerSmall = { display: "flex", alignItems: "center", justifyContent: "center", width: "30px", height: "30px", background: "none", border: "1px solid var(--border)", borderRadius: "6px", color: "var(--error)", cursor: "pointer", transition: "all 0.15s ease" };

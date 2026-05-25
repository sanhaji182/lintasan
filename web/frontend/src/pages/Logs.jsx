
import { useState, useEffect } from "react";

export default function LogsPage() {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");
  const [expanded, setExpanded] = useState(null);

  const [page, setPage] = useState(1);
  const perPage = 25;

  const load = () => { fetch("/api/logs?limit=100", { credentials: "include" }).then(r => r.json()).then(d => { setLogs(d.data || []); setLoading(false); }).catch(() => setLoading(false)); };
  useEffect(() => { load(); const i = setInterval(load, 30000); return () => clearInterval(i); }, []);

  const filtered = filter ? logs.filter(l => (l.provider + l.model + l.status + (l.error || "")).toLowerCase().includes(filter.toLowerCase())) : logs;
  const totalPages = Math.ceil(filtered.length / perPage);
  const paginated = filtered.slice((page - 1) * perPage, page * perPage);

  const successCount = logs.filter(l => l.status < 400).length;
  const errorCount = logs.filter(l => l.status >= 400).length;
  const avgLatency = logs.length ? Math.round(logs.reduce((s, l) => s + (l.latency_ms || 0), 0) / logs.length) : 0;

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "24px" }}>
        <div>
          <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Request Logs</h1>
          <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Monitor all API requests passing through the router</p>
        </div>
        <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
          <div style={{ position: "relative" }}>
            <IconSearch style={{ position: "absolute", left: "10px", top: "50%", transform: "translateY(-50%)" }} />
            <input value={filter} onChange={e => setFilter(e.target.value)} placeholder="Filter logs..." style={{ ...inputStyle, paddingLeft: "32px" }} />
          </div>
          <button onClick={load} style={btnRefresh} title="Refresh">
            <IconRefresh />
          </button>
        </div>
      </div>

      {/* Stats */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "16px", marginBottom: "24px" }}>
        <MiniStat label="Total Requests" value={logs.length} />
        <MiniStat label="Success" value={successCount} color="var(--success)" />
        <MiniStat label="Errors" value={errorCount} color="var(--error)" />
        <MiniStat label="Avg Latency" value={avgLatency + "ms"} />
      </div>

      {/* Table */}
      <div style={{ ...card, padding: 0, overflow: "hidden" }}>
        {filtered.length === 0 ? (
          <EmptyState text={filter ? "No logs match your filter" : "No requests logged yet"} />
        ) : (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }}>
            <thead>
              <tr style={{ background: "var(--bg-body)" }}>
                {["Provider", "Model", "Status", "Latency", "Input", "Output", "Time"].map(h => (
                  <th key={h} style={th}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {paginated.map((l, i) => (
                <tr key={i} style={{ ...rowStyle, background: i % 2 === 0 ? "transparent" : "var(--bg-body)" }} onClick={() => setExpanded(expanded === i ? null : i)}>
                  <td style={td}>
                    <span style={providerBadge}>{l.provider}</span>
                  </td>
                  <td style={td}><code style={mono}>{l.model?.split("/").pop()}</code></td>
                  <td style={td}>
                    <span style={{ ...statusBadge, background: l.status < 400 ? "var(--success-light)" : "var(--error-light)", color: l.status < 400 ? "var(--success)" : "var(--error)" }}>
                      {l.status}
                    </span>
                  </td>
                  <td style={td}>
                    <code style={{ ...mono, color: l.latency_ms > 5000 ? "var(--error)" : l.latency_ms > 2000 ? "var(--warning)" : "var(--fg-2)" }}>
                      {l.latency_ms < 10 ? "<10" : l.latency_ms}ms
                    </code>
                  </td>
                  <td style={td}><code style={mono}>{l.input_tokens ? fmtNum(l.input_tokens) : "—"}</code></td>
                  <td style={td}><code style={mono}>{l.output_tokens ? fmtNum(l.output_tokens) : "—"}</code></td>
                  <td style={td}><span style={{ fontSize: "12px", color: "var(--fg-3)" }}>{timeAgo(l.created_at)}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Footer with pagination */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: "12px", padding: "0 4px" }}>
        <span style={{ fontSize: "12px", color: "var(--fg-3)" }}>Showing {paginated.length} of {filtered.length} entries</span>
        {totalPages > 1 && (
          <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
            <button onClick={() => setPage(Math.max(1, page - 1))} disabled={page === 1} style={pageBtn}>←</button>
            <span style={{ fontSize: "12px", color: "var(--fg-2)", padding: "0 8px" }}>Page {page} of {totalPages}</span>
            <button onClick={() => setPage(Math.min(totalPages, page + 1))} disabled={page === totalPages} style={pageBtn}>→</button>
          </div>
        )}
        <span style={{ fontSize: "12px", color: "var(--fg-3)" }}>Auto-refreshes every 30s</span>
      </div>
    </div>
  );
}

/* Components */
function MiniStat({ label, value, color }) {
  return (
    <div style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "16px 18px", boxShadow: "var(--shadow)", border: "1px solid var(--border)" }}>
      <p style={{ fontSize: "20px", fontWeight: 700, color: color || "var(--fg-0)", fontFamily: "var(--mono)", letterSpacing: "-0.3px", marginBottom: "2px" }}>{value}</p>
      <p style={{ fontSize: "12px", color: "var(--fg-3)", fontWeight: 500 }}>{label}</p>
    </div>
  );
}

function EmptyState({ text }) {
  return (
    <div style={{ padding: "48px", textAlign: "center" }}>
      <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--fg-3)" strokeWidth="1.5" strokeLinecap="round" style={{ marginBottom: "12px", opacity: 0.4 }}><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>{text}</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "24px" }}>
        <div><div className="skeleton" style={{ width: "130px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "280px", height: "14px", borderRadius: "6px" }} /></div>
        <div style={{ display: "flex", gap: "8px" }}><div className="skeleton" style={{ width: "180px", height: "36px", borderRadius: "6px" }} /><div className="skeleton" style={{ width: "36px", height: "36px", borderRadius: "6px" }} /></div>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "16px", marginBottom: "24px" }}>
        {[1,2,3,4].map(i => <div key={i} className="skeleton" style={{ height: "70px", borderRadius: "var(--radius)" }} />)}
      </div>
      <div className="skeleton" style={{ height: "350px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

/* Icons */
function IconSearch({ style = {} }) { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--fg-3)" strokeWidth="2" strokeLinecap="round" style={style}><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>; }
function IconRefresh() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>; }

function fmtNum(n) { if (n >= 1000000) return (n / 1000000).toFixed(1) + "M"; if (n >= 1000) return Math.round(n / 1000) + "K"; return String(n); }
function timeAgo(ts) { if (!ts) return "—"; const d = Date.now() - new Date(ts).getTime(); if (d < 60000) return "just now"; if (d < 3600000) return Math.floor(d / 60000) + "m ago"; if (d < 86400000) return Math.floor(d / 3600000) + "h ago"; return Math.floor(d / 86400000) + "d ago"; }

const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)" };
const inputStyle = { padding: "8px 12px", border: "1px solid var(--border)", borderRadius: "6px", background: "var(--bg-body)", color: "var(--fg-0)", fontSize: "13px", width: "200px", outline: "none" };
const btnRefresh = { display: "flex", alignItems: "center", justifyContent: "center", width: "36px", height: "36px", background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "6px", color: "var(--fg-2)", cursor: "pointer", transition: "all 0.15s ease" };
const th = { textAlign: "left", padding: "12px 16px", fontWeight: 500, color: "var(--fg-3)", fontSize: "11px", borderBottom: "1px solid var(--border)", textTransform: "uppercase", letterSpacing: "0.5px" };
const td = { padding: "12px 16px", verticalAlign: "middle" };
const rowStyle = { borderBottom: "1px solid var(--border)", transition: "background 0.1s ease", cursor: "pointer" };
const mono = { fontFamily: "var(--mono)", fontSize: "12px", color: "var(--fg-2)", background: "var(--bg-body)", padding: "3px 8px", borderRadius: "4px" };
const providerBadge = { fontSize: "12px", fontWeight: 500, padding: "3px 10px", borderRadius: "9999px", background: "var(--primary-light)", color: "var(--primary)" };
const statusBadge = { fontSize: "11px", fontWeight: 600, padding: "3px 8px", borderRadius: "4px" };
const pageBtn = { display: "flex", alignItems: "center", justifyContent: "center", width: "28px", height: "28px", background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "6px", fontSize: "12px", color: "var(--fg-2)", cursor: "pointer" };

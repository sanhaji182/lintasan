"use client";
import { useState, useEffect } from "react";

export default function UsagePage() {
  const [usage, setUsage] = useState({ daily: [], byProvider: [], byModel: [] });
  const [loading, setLoading] = useState(true);

  useEffect(() => { fetch("/api/usage", { credentials: "include" }).then(r => r.json()).then(d => {
    const raw = d.data || {};
    // Map API fields to frontend fields
    const providers = (raw.providers || []).map(p => ({ ...p, tokens: (p.input_tokens || 0) + (p.output_tokens || 0) }));
    const models = (raw.models || []).map(m => ({ ...m, tokens: (m.input_tokens || 0) + (m.output_tokens || 0) }));
    const daily = (raw.daily || []).map(d => ({ ...d, tokens: (d.input_tokens || 0) + (d.output_tokens || 0) }));
    setUsage({ ...raw, byProvider: providers, byModel: models, daily });
    setLoading(false);
  }).catch(() => setLoading(false)); }, []);

  if (loading) return <LoadingSkeleton />;

  const total = usage.byProvider?.reduce((s, p) => s + (p.tokens || 0), 0) || 0;
  const maxProvider = Math.max(...(usage.byProvider || []).map(p => p.tokens || 0), 1);
  const maxModel = Math.max(...(usage.byModel || []).map(m => m.tokens || 0), 1);
  const maxDaily = Math.max(...(usage.daily || []).map(d => d.tokens || 0), 1);

  return (
    <div className="fade-in">
      {/* Header */}
      <div style={{ marginBottom: "24px" }}>
        <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Usage</h1>
        <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Token consumption and request analytics</p>
      </div>

      {/* Summary Cards */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "16px", marginBottom: "24px" }}>
        <StatCard label="Total Tokens" value={fmt(total)} icon={<IconCoins />} color="var(--primary)" />
        <StatCard label="Providers" value={usage.byProvider?.length || 0} icon={<IconServer />} color="var(--success)" />
        <StatCard label="Models" value={usage.byModel?.length || 0} icon={<IconCpu />} color="var(--warning)" />
        <StatCard label="Days Tracked" value={usage.daily?.length || 0} icon={<IconCalendar />} color="var(--purple, #8b5cf6)" />
      </div>

      {/* Charts Row */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "16px", marginBottom: "24px" }}>
        {/* By Provider */}
        <div style={card}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "16px" }}>
            <div>
              <p style={cardTitle}>By Provider</p>
              <p style={cardSubtitle}>Token distribution across providers</p>
            </div>
            <span style={badgeCount}>{usage.byProvider?.length || 0}</span>
          </div>
          {(!usage.byProvider || usage.byProvider.length === 0) ? (
            <EmptyState text="No provider data yet" />
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
              {usage.byProvider.map((p, i) => (
                <div key={i}>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "6px" }}>
                    <span style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)" }}>{p.provider}</span>
                    <code style={monoSmall}>{fmt(p.tokens)}</code>
                  </div>
                  <div style={progressBg}>
                    <div style={{ ...progressFill, width: `${(p.tokens / maxProvider) * 100}%`, background: COLORS[i % COLORS.length] }} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* By Model */}
        <div style={card}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "16px" }}>
            <div>
              <p style={cardTitle}>By Model</p>
              <p style={cardSubtitle}>Top models by token usage</p>
            </div>
            <span style={badgeCount}>{usage.byModel?.length || 0}</span>
          </div>
          {(!usage.byModel || usage.byModel.length === 0) ? (
            <EmptyState text="No model data yet" />
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
              {(usage.byModel || []).slice(0, 8).map((m, i) => (
                <div key={i}>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "6px" }}>
                    <code style={{ fontFamily: "var(--mono)", fontSize: "12px", color: "var(--fg-1)" }}>{m.model?.split("/").pop()}</code>
                    <code style={monoSmall}>{fmt(m.tokens)}</code>
                  </div>
                  <div style={progressBg}>
                    <div style={{ ...progressFill, width: `${(m.tokens / maxModel) * 100}%`, background: COLORS[(i + 2) % COLORS.length] }} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Daily Chart */}
      <div style={card}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "20px" }}>
          <div>
            <p style={cardTitle}>Daily Usage</p>
            <p style={cardSubtitle}>Token consumption over the last 30 days</p>
          </div>
        </div>
        {(!usage.daily || usage.daily.length === 0) ? (
          <EmptyState text="No daily data available yet" />
        ) : (
          <div>
            {/* Bar Chart */}
            <div style={{ height: "160px", display: "flex", alignItems: "flex-end", gap: "4px", padding: "0 0 8px 0", marginBottom: "16px" }}>
              {usage.daily.slice(-30).map((d, i) => (
                <div key={i} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", height: "100%" }}>
                  <div style={{ flex: 1, width: "100%", display: "flex", alignItems: "flex-end" }}>
                    <div style={{
                      width: "100%",
                      height: `${Math.max((d.tokens / maxDaily) * 100, 2)}%`,
                      background: "linear-gradient(180deg, var(--primary) 0%, rgba(60,80,224,0.3) 100%)",
                      borderRadius: "4px 4px 1px 1px",
                      transition: "height 0.3s ease",
                      minHeight: "2px",
                    }} title={`${d.date}: ${fmt(d.tokens)} tokens, ${d.requests} requests`} />
                  </div>
                </div>
              ))}
            </div>
            {/* Table */}
            <div style={{ borderTop: "1px solid var(--border)", paddingTop: "16px" }}>
              <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }}>
                <thead>
                  <tr>
                    {["Date", "Requests", "Tokens"].map(h => <th key={h} style={thStyle}>{h}</th>)}
                  </tr>
                </thead>
                <tbody>
                  {usage.daily.slice(-10).reverse().map((d, i) => (
                    <tr key={i} style={rowStyle}>
                      <td style={tdStyle}><code style={monoSmall}>{d.date}</code></td>
                      <td style={tdStyle}><span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{d.requests}</span></td>
                      <td style={tdStyle}>
                        <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                          <code style={monoSmall}>{fmt(d.tokens)}</code>
                          <div style={{ ...progressBg, width: "80px" }}>
                            <div style={{ ...progressFill, width: `${(d.tokens / maxDaily) * 100}%` }} />
                          </div>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/* Components */
function StatCard({ label, value, icon, color }) {
  return (
    <div style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "18px 20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", display: "flex", alignItems: "center", gap: "14px" }}>
      <div style={{ width: "40px", height: "40px", borderRadius: "10px", background: `${color}15`, display: "flex", alignItems: "center", justifyContent: "center", color }}>
        {icon}
      </div>
      <div>
        <p style={{ fontSize: "20px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)", letterSpacing: "-0.3px" }}>{value}</p>
        <p style={{ fontSize: "12px", color: "var(--fg-3)", fontWeight: 500 }}>{label}</p>
      </div>
    </div>
  );
}

function EmptyState({ text }) {
  return (
    <div style={{ padding: "32px", textAlign: "center" }}>
      <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="var(--fg-3)" strokeWidth="1.5" strokeLinecap="round" style={{ marginBottom: "10px", opacity: 0.4 }}><path d="M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0z"/><path d="M9 10h.01M15 10h.01M9.5 15a3.5 3.5 0 0 0 5 0"/></svg>
      <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>{text}</p>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ marginBottom: "24px" }}><div className="skeleton" style={{ width: "80px", height: "20px", borderRadius: "6px", marginBottom: "8px" }} /><div className="skeleton" style={{ width: "240px", height: "14px", borderRadius: "6px" }} /></div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "16px", marginBottom: "24px" }}>
        {[1,2,3,4].map(i => <div key={i} className="skeleton" style={{ height: "80px", borderRadius: "var(--radius)" }} />)}
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "16px", marginBottom: "24px" }}>
        <div className="skeleton" style={{ height: "250px", borderRadius: "var(--radius)" }} />
        <div className="skeleton" style={{ height: "250px", borderRadius: "var(--radius)" }} />
      </div>
      <div className="skeleton" style={{ height: "300px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

/* Icons */
function IconCoins() { return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="8" cy="8" r="6"/><path d="M18.09 10.37A6 6 0 1 1 10.34 18"/><path d="M7 6h1v4"/></svg>; }
function IconServer() { return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>; }
function IconCpu() { return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><line x1="9" y1="1" x2="9" y2="4"/><line x1="15" y1="1" x2="15" y2="4"/><line x1="9" y1="20" x2="9" y2="23"/><line x1="15" y1="20" x2="15" y2="23"/></svg>; }
function IconCalendar() { return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>; }

function fmt(n) { if (n >= 1000000) return (n / 1000000).toFixed(1) + "M"; if (n >= 1000) return Math.round(n / 1000) + "K"; return String(n || 0); }

const COLORS = ["var(--primary)", "var(--success)", "var(--warning)", "#8b5cf6", "#ec4899", "#06b6d4"];
const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)" };
const cardTitle = { fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "2px" };
const cardSubtitle = { fontSize: "12px", color: "var(--fg-3)" };
const badgeCount = { fontSize: "11px", fontWeight: 600, padding: "3px 10px", borderRadius: "9999px", background: "var(--bg-body)", color: "var(--fg-2)", border: "1px solid var(--border)" };
const monoSmall = { fontFamily: "var(--mono)", fontSize: "12px", color: "var(--fg-2)" };
const progressBg = { width: "100%", height: "6px", borderRadius: "3px", background: "var(--border)", overflow: "hidden" };
const progressFill = { height: "100%", borderRadius: "3px", background: "var(--primary)", transition: "width 0.4s ease" };
const thStyle = { textAlign: "left", padding: "10px 12px", fontWeight: 500, color: "var(--fg-3)", fontSize: "11px", textTransform: "uppercase", letterSpacing: "0.5px", borderBottom: "1px solid var(--border)" };
const tdStyle = { padding: "10px 12px" };
const rowStyle = { borderBottom: "1px solid var(--border)" };

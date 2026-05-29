"use client";
import { useState, useEffect, useRef } from "react";

export default function DashboardOverview() {
  const [stats, setStats] = useState(null);
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);

  const load = () => {
    Promise.all([
      fetch("/api/overview-stats", { credentials: "include" }).then(r => r.json()),
      fetch("/api/logs?limit=6", { credentials: "include" }).then(r => r.json()),
    ]).then(([s, l]) => { setStats(s.data || {}); setLogs(l.data || []); setLoading(false); }).catch(() => setLoading(false));
  };
  useEffect(() => { load(); const i = setInterval(load, 15000); return () => clearInterval(i); }, []);

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      {/* Base URL Info Card */}
      <BaseUrlCard />

      {/* Metric Cards */}
      <div className="stagger metric-grid-4" style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "20px", marginBottom: "28px" }}>
        <MetricCard icon="📡" label="Total Requests" value={stats.totalRequests || 0} color="var(--primary)" bg="var(--primary-light)" gradient="linear-gradient(135deg, #3c50e0 0%, #6366f1 100%)" delay={0} />
        <MetricCard icon="⚡" label="Cache Hit Rate" value={(stats.cacheHitRate || 0) + "%"} color="var(--success)" bg="var(--success-light)" gradient="linear-gradient(135deg, #10b981 0%, #34d399 100%)" delay={1} />
        <MetricCard icon="🪙" label="Tokens Today" value={stats.tokensToday || 0} color="var(--warning)" bg="var(--warning-light)" gradient="linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%)" delay={2} />
        <MetricCard icon="⏱️" label="Avg Latency" value={(stats.avgLatency || 0) + "ms"} color="var(--purple)" bg="var(--purple-light)" gradient="linear-gradient(135deg, #8b5cf6 0%, #a78bfa 100%)" delay={3} />
      </div>

      {/* Charts Row */}
      <div className="chart-grid" style={{ display: "grid", gridTemplateColumns: "5fr 3fr", gap: "20px", marginBottom: "28px" }}>
        <Card title="Request Volume" subtitle="Last 7 days" delay={4}>
          <AnimatedBarChart data={[35, 55, 40, 70, 45, 80, 65]} labels={["Mon","Tue","Wed","Thu","Fri","Sat","Sun"]} />
        </Card>
        <Card title="Cache Performance" subtitle="Hit vs Miss ratio" delay={5}>
          <AnimatedDonut value={stats.cacheHitRate || 0} />
        </Card>
      </div>

      {/* Features + Providers */}
      <div className="metric-grid-2" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "20px", marginBottom: "28px" }}>
        <Card title="Features" subtitle={`${(stats.features || []).filter(f => f.enabled).length} of ${(stats.features || []).length} active`} delay={6}>
          <div className="stagger" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "8px" }}>
            {(stats.features || []).map((f, i) => (
              <div key={i} className="fade-in" style={{ display: "flex", alignItems: "center", gap: "10px", padding: "10px 14px", borderRadius: "var(--radius-sm)", background: f.enabled ? "var(--success-light)" : "var(--bg-body)", border: "1px solid " + (f.enabled ? "rgba(16,185,129,0.2)" : "var(--border)"), transition: "all var(--transition)", cursor: "default" }}>
                <div style={{ width: "8px", height: "8px", borderRadius: "50%", background: f.enabled ? "var(--success)" : "var(--fg-3)", animation: f.enabled ? "dotPulse 2s infinite" : "none", transition: "all var(--transition)" }} />
                <span style={{ fontSize: "12px", fontWeight: 500, color: f.enabled ? "var(--fg-0)" : "var(--fg-2)" }}>{f.name}</span>
              </div>
            ))}
          </div>
        </Card>

        <Card title="Providers" subtitle="Health status" delay={7}>
          {(stats.providers || []).length === 0 ? (
            <EmptyState text="No providers configured" />
          ) : (
            <div className="stagger" style={{ display: "flex", flexDirection: "column", gap: "10px" }}>
              {(stats.providers || []).map((p, i) => (
                <div key={i} className="fade-in" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", borderRadius: "var(--radius-sm)", background: "var(--bg-body)", border: "1px solid var(--border)", transition: "all var(--transition)", cursor: "default" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                    <div style={{ width: "10px", height: "10px", borderRadius: "50%", background: p.healthy ? "var(--success)" : "var(--error)", boxShadow: p.healthy ? "0 0 8px rgba(16,185,129,0.5)" : "0 0 8px rgba(239,68,68,0.5)", animation: "dotPulse 2s infinite" }} />
                    <span style={{ fontSize: "14px", fontWeight: 500, color: "var(--fg-0)" }}>{p.name}</span>
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: "12px" }}>
                    <span style={{ fontSize: "12px", color: "var(--fg-2)", fontFamily: "var(--mono)" }}>{p.latency || "—"}ms</span>
                    <StatusBadge status={p.healthy ? "Active" : "Down"} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>

      {/* Token Stats */}
      <div className="stagger metric-grid-4" style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "20px", marginBottom: "28px" }}>
        <StatCard label="Tokens Today" value={fmt(stats.tokensToday || 0)} icon="📤" />
        <StatCard label="This Month" value={fmt(stats.tokensMonth || 0)} icon="📅" />
        <StatCard label="Tokens Saved" value={fmt(stats.tokensSaved || 0)} icon="💰" color="var(--success)" />
        <StatCard label="Compressed" value={fmt(stats.tokensCompressed || 0)} icon="🗜️" />
      </div>

      {/* Recent Requests */}
      <Card title="Recent Requests" subtitle="Latest API calls" delay={8}>
        {logs.length === 0 ? (
          <EmptyState text="No requests yet. Send a request to /v1/chat/completions" />
        ) : (
          <div className="responsive-table" style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ borderBottom: "2px solid var(--border)" }}>
                  {["Provider", "Model", "Status", "Latency", "Tokens", "Cache", "Time"].map(h => (
                    <th key={h} style={{ textAlign: "left", padding: "12px 14px", fontSize: "11px", fontWeight: 600, color: "var(--fg-3)", textTransform: "uppercase", letterSpacing: "0.5px" }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody className="stagger">
                {logs.map((l, i) => (
                  <tr key={i} className="fade-in" style={{ borderBottom: "1px solid var(--border-light)", transition: "background var(--transition)" }}>
                    <td style={td}><span style={{ fontWeight: 500, color: "var(--fg-0)" }}>{l.provider}</span></td>
                    <td style={td}><code style={mono}>{l.model?.split("/").pop()}</code></td>
                    <td style={td}><StatusBadge status={l.status < 400 ? "Success" : "Error"} /></td>
                    <td style={td}><code style={mono}>{l.latency_ms < 10 ? "<10" : l.latency_ms}ms</code></td>
                    <td style={td}><span style={{ fontSize: "13px", color: "var(--fg-1)" }}>{l.input_tokens ? Math.round(l.input_tokens / 1000) + "K" : "—"}</span></td>
                    <td style={td}>{l.cache_hit ? <StatusBadge status="Hit" /> : <span style={{ fontSize: "12px", color: "var(--fg-3)" }}>Miss</span>}</td>
                    <td style={td}><span style={{ fontSize: "12px", color: "var(--fg-3)" }}>{timeAgo(l.created_at)}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}

/* Animated Bar Chart */
function AnimatedBarChart({ data, labels }) {
  const [mounted, setMounted] = useState(false);
  useEffect(() => { setTimeout(() => setMounted(true), 200); }, []);
  const max = Math.max(...data);
  return (
    <div style={{ height: "200px", display: "flex", alignItems: "flex-end", gap: "10px", padding: "20px 0" }}>
      {data.map((h, i) => (
        <div key={i} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: "8px", height: "100%" }}>
          <div style={{ flex: 1, width: "100%", display: "flex", alignItems: "flex-end" }}>
            <div style={{
              width: "100%",
              height: mounted ? `${(h / max) * 100}%` : "0%",
              background: "linear-gradient(180deg, var(--primary) 0%, rgba(60,80,224,0.3) 100%)",
              borderRadius: "6px 6px 2px 2px",
              transition: `height 0.6s cubic-bezier(0.34, 1.56, 0.64, 1) ${i * 80}ms`,
              position: "relative",
              overflow: "hidden",
            }}>
              <div style={{ position: "absolute", inset: 0, background: "linear-gradient(180deg, rgba(255,255,255,0.2) 0%, transparent 100%)" }} />
            </div>
          </div>
          <span style={{ fontSize: "11px", color: "var(--fg-3)", fontWeight: 500 }}>{labels[i]}</span>
        </div>
      ))}
    </div>
  );
}

/* Animated Donut Chart */
function AnimatedDonut({ value }) {
  const [mounted, setMounted] = useState(false);
  useEffect(() => { setTimeout(() => setMounted(true), 400); }, []);
  const circumference = 2 * Math.PI * 15.9;
  const offset = circumference - (value / 100) * circumference;
  return (
    <div style={{ display: "flex", flexDirection: "column", alignItems: "center", padding: "20px 0" }}>
      <div style={{ position: "relative", width: "150px", height: "150px", marginBottom: "20px" }}>
        <svg viewBox="0 0 36 36" style={{ width: "100%", height: "100%", transform: "rotate(-90deg)" }}>
          <circle cx="18" cy="18" r="15.9" fill="none" stroke="var(--border)" strokeWidth="2.5" />
          <circle cx="18" cy="18" r="15.9" fill="none" stroke="var(--success)" strokeWidth="2.5"
            strokeDasharray={circumference}
            strokeDashoffset={mounted ? offset : circumference}
            strokeLinecap="round"
            style={{ transition: "stroke-dashoffset 1.2s cubic-bezier(0.4, 0, 0.2, 1) 0.3s" }} />
        </svg>
        <div style={{ position: "absolute", inset: 0, display: "flex", alignItems: "center", justifyContent: "center", flexDirection: "column" }}>
          <span style={{ fontSize: "28px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)", letterSpacing: "-1px" }}>{value}%</span>
          <span style={{ fontSize: "11px", color: "var(--fg-3)", fontWeight: 500 }}>Hit Rate</span>
        </div>
      </div>
      <div style={{ display: "flex", gap: "20px" }}>
        <Legend color="var(--success)" label="Cache Hit" />
        <Legend color="var(--border)" label="Cache Miss" />
      </div>
    </div>
  );
}

/* Components */
function MetricCard({ icon, label, value, trend, up, color, bg, gradient, delay }) {
  return (
    <div className="fade-in" style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "24px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", transition: "all var(--transition)", animationDelay: `${delay * 60}ms`, position: "relative", overflow: "hidden" }}>
      {/* Subtle gradient accent top */}
      <div style={{ position: "absolute", top: 0, left: 0, right: 0, height: "3px", background: gradient, borderRadius: "var(--radius) var(--radius) 0 0" }} />
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: "16px" }}>
        <div style={{ width: "44px", height: "44px", borderRadius: "12px", background: bg, display: "flex", alignItems: "center", justifyContent: "center", fontSize: "20px" }}>{icon}</div>
        {trend && (
          <span style={{ fontSize: "12px", fontWeight: 600, color: up ? "var(--success)" : "var(--error)", display: "flex", alignItems: "center", gap: "2px", padding: "3px 8px", borderRadius: "9999px", background: up ? "var(--success-light)" : "var(--error-light)" }}>
            {up ? "↑" : "↓"} {trend}
          </span>
        )}
      </div>
      <p style={{ fontSize: "28px", fontWeight: 700, color: "var(--fg-0)", marginBottom: "4px", fontFamily: "var(--mono)", letterSpacing: "-0.5px" }}>
        <AnimatedNumber value={value} />
      </p>
      <p style={{ fontSize: "13px", color: "var(--fg-2)", fontWeight: 500 }}>{label}</p>
    </div>
  );
}

function AnimatedNumber({ value }) {
  const [display, setDisplay] = useState(typeof value === "number" ? 0 : value);
  useEffect(() => {
    if (typeof value !== "number") { setDisplay(value); return; }
    let start = 0;
    const end = value;
    const duration = 800;
    const startTime = Date.now();
    const tick = () => {
      const elapsed = Date.now() - startTime;
      const progress = Math.min(elapsed / duration, 1);
      const eased = 1 - Math.pow(1 - progress, 3);
      setDisplay(Math.round(start + (end - start) * eased));
      if (progress < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }, [value]);
  return typeof value === "number" ? fmt(display) : display;
}

function Card({ title, subtitle, children, delay = 0 }) {
  return (
    <div className="fade-in" style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "24px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", animationDelay: `${delay * 60}ms`, transition: "all var(--transition)" }}>
      <div style={{ marginBottom: "16px", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <div>
          <h3 style={{ fontSize: "15px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "2px" }}>{title}</h3>
          {subtitle && <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>{subtitle}</p>}
        </div>
      </div>
      {children}
    </div>
  );
}

function StatCard({ label, value, icon, color }) {
  return (
    <div className="fade-in" style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "18px 20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", display: "flex", alignItems: "center", gap: "14px", transition: "all var(--transition)" }}>
      <span style={{ fontSize: "24px" }}>{icon}</span>
      <div>
        <p style={{ fontSize: "20px", fontWeight: 700, color: color || "var(--fg-0)", fontFamily: "var(--mono)", letterSpacing: "-0.3px" }}>{value}</p>
        <p style={{ fontSize: "12px", color: "var(--fg-2)", fontWeight: 500 }}>{label}</p>
      </div>
    </div>
  );
}

function StatusBadge({ status }) {
  const colors = {
    Success: { bg: "var(--success-light)", fg: "var(--success)" },
    Error: { bg: "var(--error-light)", fg: "var(--error)" },
    Active: { bg: "var(--success-light)", fg: "var(--success)" },
    Down: { bg: "var(--error-light)", fg: "var(--error)" },
    Hit: { bg: "var(--info-light)", fg: "var(--info)" },
  };
  const c = colors[status] || { bg: "var(--bg-body)", fg: "var(--fg-2)" };
  return (
    <span style={{ fontSize: "11px", fontWeight: 600, padding: "4px 10px", borderRadius: "9999px", background: c.bg, color: c.fg, transition: "all var(--transition)" }}>{status}</span>
  );
}

function Legend({ color, label }) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
      <div style={{ width: "10px", height: "10px", borderRadius: "3px", background: color }} />
      <span style={{ fontSize: "12px", color: "var(--fg-2)", fontWeight: 500 }}>{label}</span>
    </div>
  );
}

function EmptyState({ text }) {
  return (
    <div style={{ padding: "40px", textAlign: "center" }}>
      <div style={{ fontSize: "32px", marginBottom: "12px", animation: "float 3s ease-in-out infinite" }}>📭</div>
      <p style={{ fontSize: "14px", color: "var(--fg-3)" }}>{text}</p>
    </div>
  );
}

function BaseUrlCard() {
  const [copied, setCopied] = useState(false);
  const [baseUrl, setBaseUrl] = useState("http://100.99.2.14:20180/api/v1");

  useEffect(() => {
    if (typeof window !== "undefined") {
      const host = window.location.hostname;
      const port = window.location.port || "20180";
      setBaseUrl(`http://${host}:${port}/api/v1`);
    }
  }, []);

  function copyUrl() {
    navigator.clipboard.writeText(baseUrl).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div style={{ background: "linear-gradient(135deg, var(--primary-light) 0%, var(--bg-card) 100%)", borderRadius: "var(--radius)", padding: "20px 24px", boxShadow: "var(--shadow)", border: "1px solid var(--primary)", marginBottom: "20px", display: "flex", alignItems: "center", justifyContent: "space-between", gap: "16px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: "14px" }}>
        <div style={{ width: "40px", height: "40px", borderRadius: "10px", background: "var(--primary)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: "18px", flexShrink: 0 }}>🔗</div>
        <div>
          <p style={{ fontSize: "12px", fontWeight: 600, color: "var(--fg-2)", margin: "0 0 4px", textTransform: "uppercase", letterSpacing: "0.5px" }}>Base URL</p>
          <code style={{ fontSize: "14px", fontFamily: "var(--mono)", color: "var(--fg-0)", fontWeight: 600 }}>{baseUrl}</code>
          <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "4px 0 0" }}>Use this URL in your AI tools, IDE extensions, and API clients as the OpenAI-compatible endpoint</p>
        </div>
      </div>
      <button onClick={copyUrl} style={{ padding: "8px 14px", background: copied ? "var(--success)" : "var(--primary)", color: "#fff", border: "none", borderRadius: "var(--radius-sm)", fontSize: "12px", fontWeight: 500, cursor: "pointer", whiteSpace: "nowrap", transition: "all 0.2s", flexShrink: 0 }}>
        {copied ? "Copied ✓" : "Copy URL"}
      </button>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div className="metric-grid-4" style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "20px", marginBottom: "28px" }}>
        {[1,2,3,4].map(i => (
          <div key={i} className="skeleton" style={{ height: "140px", borderRadius: "var(--radius)" }} />
        ))}
      </div>
      <div className="chart-grid" style={{ display: "grid", gridTemplateColumns: "5fr 3fr", gap: "20px", marginBottom: "28px" }}>
        <div className="skeleton" style={{ height: "280px", borderRadius: "var(--radius)" }} />
        <div className="skeleton" style={{ height: "280px", borderRadius: "var(--radius)" }} />
      </div>
    </div>
  );
}

function fmt(n) { if (n >= 1000000) return (n / 1000000).toFixed(1) + "M"; if (n >= 1000) return Math.round(n / 1000) + "K"; return String(n); }
function timeAgo(ts) { if (!ts) return "—"; const d = Date.now() - new Date(ts).getTime(); if (d < 60000) return "just now"; if (d < 3600000) return Math.floor(d / 60000) + "m ago"; if (d < 86400000) return Math.floor(d / 3600000) + "h ago"; return Math.floor(d / 86400000) + "d ago"; }

const td = { padding: "12px 14px", verticalAlign: "middle" };
const mono = { fontFamily: "var(--mono)", fontSize: "12px", color: "var(--fg-2)", background: "var(--bg-body)", padding: "3px 8px", borderRadius: "6px" };


import { useState, useEffect } from "react";

const card = {
  background: "var(--bg-card)",
  borderRadius: "var(--radius-lg)",
  border: "1px solid var(--border)",
  padding: "24px",
  boxShadow: "var(--shadow-sm)",
};

const statCard = {
  ...card,
  display: "flex",
  flexDirection: "column",
  gap: "8px",
};

export default function AnalyticsPage() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [period, setPeriod] = useState("7d");

  useEffect(() => {
    setLoading(true);
    fetch(`/api/analytics?period=${period}`, { credentials: "include" })
      .then(r => r.json())
      .then(d => { setData(d); setLoading(false); })
      .catch(() => setLoading(false));
  }, [period]);

  if (loading) return <LoadingSkeleton />;
  if (!data) return <EmptyState />;

  const { summary, today, dailyUsage, providerBreakdown } = data;

  // Calculate max for bar chart scaling
  const maxTokens = Math.max(...(dailyUsage || []).map(d => (d.input_tokens || 0) + (d.output_tokens || 0)), 1);

  // Donut chart data
  const cacheProviders = (providerBreakdown || []).filter(p => ["cache", "semantic-cache", "stream-cache"].includes(p.provider));
  const directProviders = (providerBreakdown || []).filter(p => !["cache", "semantic-cache", "stream-cache"].includes(p.provider));
  const totalCacheRequests = cacheProviders.reduce((s, p) => s + p.count, 0);
  const totalDirectRequests = directProviders.reduce((s, p) => s + p.count, 0);

  return (
    <div className="fade-in">
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "24px" }}>
        <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Token usage analytics and savings breakdown</p>
        <div style={{ display: "flex", gap: "6px" }}>
          {["1d", "7d", "30d"].map(p => (
            <button key={p} onClick={() => setPeriod(p)} style={{
              padding: "6px 14px", fontSize: "12px", fontWeight: 500, borderRadius: "var(--radius-sm)",
              border: period === p ? "1px solid var(--primary)" : "1px solid var(--border)",
              background: period === p ? "var(--primary-light)" : "var(--bg-card)",
              color: period === p ? "var(--primary)" : "var(--fg-2)",
              cursor: "pointer",
            }}>{p === "1d" ? "Today" : p === "7d" ? "7 Days" : "30 Days"}</button>
          ))}
        </div>
      </div>

      {/* Summary Cards */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "16px", marginBottom: "24px" }}>
        <div style={statCard}>
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <div style={{ width: "32px", height: "32px", borderRadius: "8px", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--primary)" strokeWidth="2"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>
            </div>
            <span style={{ fontSize: "12px", color: "var(--fg-3)", fontWeight: 500 }}>Tokens Saved Today</span>
          </div>
          <span style={{ fontSize: "24px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)" }}>{formatNumber(today.tokensSaved)}</span>
          <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>{today.cacheHits} cache hits today</span>
        </div>

        <div style={statCard}>
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <div style={{ width: "32px", height: "32px", borderRadius: "8px", background: "rgba(34,197,94,0.1)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--success)" strokeWidth="2"><polyline points="20 12 20 22 4 22 4 12"/><rect x="2" y="7" width="20" height="5"/><line x1="12" y1="22" x2="12" y2="7"/></svg>
            </div>
            <span style={{ fontSize: "12px", color: "var(--fg-3)", fontWeight: 500 }}>Cache Hit Rate</span>
          </div>
          <span style={{ fontSize: "24px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)" }}>{summary.cacheHitRate}%</span>
          <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>{summary.cacheHits} / {summary.totalRequests} requests</span>
        </div>

        <div style={statCard}>
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <div style={{ width: "32px", height: "32px", borderRadius: "8px", background: "rgba(234,179,8,0.1)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" strokeWidth="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/></svg>
            </div>
            <span style={{ fontSize: "12px", color: "var(--fg-3)", fontWeight: 500 }}>Total Tokens Used</span>
          </div>
          <span style={{ fontSize: "24px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)" }}>{formatNumber(summary.totalTokens)}</span>
          <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>{formatNumber(summary.tokensSaved)} saved by cache</span>
        </div>

        <div style={statCard}>
          <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
            <div style={{ width: "32px", height: "32px", borderRadius: "8px", background: "rgba(139,92,246,0.1)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#8b5cf6" strokeWidth="2"><line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>
            </div>
            <span style={{ fontSize: "12px", color: "var(--fg-3)", fontWeight: 500 }}>Cost Saved</span>
          </div>
          <span style={{ fontSize: "24px", fontWeight: 700, color: "var(--fg-0)", fontFamily: "var(--mono)" }}>${summary.costSaved}</span>
          <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>Estimated at $0.002/1K tokens</span>
        </div>
      </div>

      {/* Charts Row */}
      <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: "16px" }}>
        {/* Bar Chart - Daily Token Usage */}
        <div style={card}>
          <h3 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: "0 0 16px" }}>Daily Token Usage</h3>
          {dailyUsage && dailyUsage.length > 0 ? (
            <div style={{ height: "220px", display: "flex", alignItems: "flex-end", gap: "8px", padding: "0 4px" }}>
              {dailyUsage.map((day, i) => {
                const total = (day.input_tokens || 0) + (day.output_tokens || 0);
                const inputHeight = maxTokens > 0 ? ((day.input_tokens || 0) / maxTokens) * 180 : 0;
                const outputHeight = maxTokens > 0 ? ((day.output_tokens || 0) / maxTokens) * 180 : 0;
                const dayLabel = new Date(day.day + "T00:00:00").toLocaleDateString("en", { weekday: "short" });
                return (
                  <div key={i} style={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: "4px" }}>
                    <span style={{ fontSize: "10px", color: "var(--fg-3)", fontFamily: "var(--mono)" }}>{formatNumber(total)}</span>
                    <div style={{ width: "100%", display: "flex", flexDirection: "column", alignItems: "center" }}>
                      <svg width="100%" height={Math.max(inputHeight + outputHeight, 4)} style={{ overflow: "visible" }}>
                        <rect x="20%" width="60%" height={outputHeight} y="0" rx="3" fill="var(--primary)" opacity="0.6" />
                        <rect x="20%" width="60%" height={inputHeight} y={outputHeight} rx="3" fill="var(--primary)" />
                      </svg>
                    </div>
                    <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>{dayLabel}</span>
                    {day.cache_hits > 0 && (
                      <span style={{ fontSize: "9px", color: "var(--success)", fontFamily: "var(--mono)" }}>+{day.cache_hits} hits</span>
                    )}
                  </div>
                );
              })}
            </div>
          ) : (
            <div style={{ height: "220px", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--fg-3)", fontSize: "13px" }}>
              No data for this period
            </div>
          )}
          <div style={{ display: "flex", gap: "16px", marginTop: "12px", justifyContent: "center" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
              <div style={{ width: "10px", height: "10px", borderRadius: "2px", background: "var(--primary)" }} />
              <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>Input Tokens</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
              <div style={{ width: "10px", height: "10px", borderRadius: "2px", background: "var(--primary)", opacity: 0.6 }} />
              <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>Output Tokens</span>
            </div>
          </div>
        </div>

        {/* Donut Chart - Savings Breakdown */}
        <div style={card}>
          <h3 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: "0 0 16px" }}>Request Breakdown</h3>
          <div style={{ display: "flex", justifyContent: "center", marginBottom: "16px" }}>
            <DonutChart cached={totalCacheRequests} direct={totalDirectRequests} />
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
            {(providerBreakdown || []).slice(0, 6).map((p, i) => (
              <div key={i} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "6px 10px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
                <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                  <div style={{ width: "8px", height: "8px", borderRadius: "50%", background: getProviderColor(p.provider) }} />
                  <span style={{ fontSize: "12px", color: "var(--fg-1)" }}>{p.provider || "unknown"}</span>
                </div>
                <span style={{ fontSize: "12px", fontWeight: 600, color: "var(--fg-0)", fontFamily: "var(--mono)" }}>{p.count}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Avg Latency */}
      <div style={{ ...card, marginTop: "16px" }}>
        <h3 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: "0 0 12px" }}>Performance Summary</h3>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "16px" }}>
          <div style={{ padding: "16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)", textAlign: "center" }}>
            <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "0 0 4px", textTransform: "uppercase", letterSpacing: "0.5px" }}>Avg Latency</p>
            <p style={{ fontSize: "20px", fontWeight: 700, color: "var(--fg-0)", margin: 0, fontFamily: "var(--mono)" }}>{summary.avgLatency}ms</p>
          </div>
          <div style={{ padding: "16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)", textAlign: "center" }}>
            <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "0 0 4px", textTransform: "uppercase", letterSpacing: "0.5px" }}>Total Requests</p>
            <p style={{ fontSize: "20px", fontWeight: 700, color: "var(--fg-0)", margin: 0, fontFamily: "var(--mono)" }}>{formatNumber(summary.totalRequests)}</p>
          </div>
          <div style={{ padding: "16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)", textAlign: "center" }}>
            <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "0 0 4px", textTransform: "uppercase", letterSpacing: "0.5px" }}>Input / Output Ratio</p>
            <p style={{ fontSize: "20px", fontWeight: 700, color: "var(--fg-0)", margin: 0, fontFamily: "var(--mono)" }}>
              {summary.totalOutputTokens > 0 ? (summary.totalInputTokens / summary.totalOutputTokens).toFixed(1) : "0"}:1
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

function DonutChart({ cached, direct }) {
  const total = cached + direct;
  if (total === 0) {
    return (
      <svg width="140" height="140" viewBox="0 0 140 140">
        <circle cx="70" cy="70" r="50" fill="none" stroke="var(--border)" strokeWidth="20" />
        <text x="70" y="70" textAnchor="middle" dominantBaseline="middle" fill="var(--fg-3)" fontSize="12">No data</text>
      </svg>
    );
  }

  const cachedPct = cached / total;
  const circumference = 2 * Math.PI * 50;
  const cachedLen = cachedPct * circumference;
  const directLen = circumference - cachedLen;

  return (
    <svg width="140" height="140" viewBox="0 0 140 140">
      <circle cx="70" cy="70" r="50" fill="none" stroke="var(--border)" strokeWidth="20" opacity="0.3" />
      <circle cx="70" cy="70" r="50" fill="none" stroke="var(--success)" strokeWidth="20"
        strokeDasharray={`${cachedLen} ${directLen}`}
        strokeDashoffset={circumference * 0.25}
        strokeLinecap="round"
      />
      <circle cx="70" cy="70" r="50" fill="none" stroke="var(--primary)" strokeWidth="20"
        strokeDasharray={`${directLen} ${cachedLen}`}
        strokeDashoffset={circumference * 0.25 - cachedLen}
        strokeLinecap="round"
      />
      <text x="70" y="64" textAnchor="middle" dominantBaseline="middle" fill="var(--fg-0)" fontSize="18" fontWeight="700">
        {Math.round(cachedPct * 100)}%
      </text>
      <text x="70" y="82" textAnchor="middle" dominantBaseline="middle" fill="var(--fg-3)" fontSize="10">
        cached
      </text>
    </svg>
  );
}

function getProviderColor(provider) {
  const colors = {
    "cache": "var(--success)",
    "semantic-cache": "#22d3ee",
    "stream-cache": "#a78bfa",
    "coalesced": "#f59e0b",
  };
  return colors[provider] || "var(--primary)";
}

function formatNumber(n) {
  if (!n || n === 0) return "0";
  if (n >= 1000000) return (n / 1000000).toFixed(1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "K";
  return String(n);
}

function LoadingSkeleton() {
  return (
    <div className="fade-in">
      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "16px", marginBottom: "24px" }}>
        {[1, 2, 3, 4].map(i => (
          <div key={i} style={{ ...card, height: "120px", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <div style={{ width: "60%", height: "20px", background: "var(--border)", borderRadius: "4px", animation: "pulse 1.5s infinite" }} />
          </div>
        ))}
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: "16px" }}>
        <div style={{ ...card, height: "300px", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <div style={{ width: "80%", height: "200px", background: "var(--border)", borderRadius: "8px", animation: "pulse 1.5s infinite" }} />
        </div>
        <div style={{ ...card, height: "300px", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <div style={{ width: "120px", height: "120px", borderRadius: "50%", background: "var(--border)", animation: "pulse 1.5s infinite" }} />
        </div>
      </div>
    </div>
  );
}

function EmptyState() {
  return (
    <div className="fade-in" style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", minHeight: "400px", gap: "16px" }}>
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--fg-3)" strokeWidth="1.5">
        <line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/>
      </svg>
      <p style={{ fontSize: "14px", color: "var(--fg-2)" }}>No analytics data yet</p>
      <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>Start making requests through the proxy to see token usage analytics</p>
    </div>
  );
}

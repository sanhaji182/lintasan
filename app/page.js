"use client";
import { useRouter } from "next/navigation";

export default function Home() {
  const router = useRouter();
  return (
    <div style={{ minHeight: "100vh", background: "linear-gradient(135deg, #1c2434 0%, #2d3a4a 100%)", fontFamily: "'Inter', system-ui, sans-serif" }}>
      {/* Hero */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "center", minHeight: "80vh", padding: "40px 20px" }}>
        <div style={{ textAlign: "center", maxWidth: "640px" }}>
          <div style={{ width: "72px", height: "72px", borderRadius: "18px", background: "#3c50e0", margin: "0 auto 28px", display: "flex", alignItems: "center", justifyContent: "center", boxShadow: "0 8px 24px rgba(60,80,224,0.3)" }}>
            <span style={{ color: "#fff", fontSize: "32px", fontWeight: 700 }}>L</span>
          </div>
          <h1 style={{ fontSize: "48px", fontWeight: 700, color: "#fff", letterSpacing: "-1px", marginBottom: "16px" }}>Lintasan</h1>
          <p style={{ fontSize: "18px", color: "#94a3b8", lineHeight: 1.7, marginBottom: "12px" }}>
            Setiap Koneksi Punya Jalannya
          </p>
          <p style={{ fontSize: "15px", color: "#64748b", lineHeight: 1.7, marginBottom: "36px", maxWidth: "480px", margin: "0 auto 36px" }}>
            Open source LLM proxy gateway with 34+ features. One endpoint for all AI providers — smart routing, embedding cache, MITM bridge, and plugin system.
          </p>
          <div style={{ display: "flex", gap: "12px", justifyContent: "center", marginBottom: "24px", flexWrap: "wrap" }}>
            <button onClick={() => router.push("/dashboard")} style={{ padding: "14px 32px", background: "#3c50e0", color: "#fff", border: "none", borderRadius: "10px", fontSize: "15px", fontWeight: 600, cursor: "pointer", boxShadow: "0 4px 12px rgba(60,80,224,0.3)" }}>
              Open Dashboard
            </button>
            <a href="https://github.com/sanhaji182/lintasan" target="_blank" style={{ padding: "14px 32px", background: "rgba(255,255,255,0.08)", border: "1px solid rgba(255,255,255,0.15)", borderRadius: "10px", fontSize: "15px", color: "#dee4ee", textDecoration: "none", fontWeight: 500 }}>
              GitHub →
            </a>
          </div>
          <p style={{ fontSize: "13px", color: "#475569", fontFamily: "'JetBrains Mono', monospace", marginBottom: "48px" }}>
            npm install -g lintasan && lintasan
          </p>

          {/* Feature pills */}
          <div style={{ display: "flex", justifyContent: "center", gap: "10px", flexWrap: "wrap" }}>
            {["Embedding Cache", "MITM Bridge", "Combo Routing", "Plugin System", "OAuth Flow", "Docker"].map(f => (
              <span key={f} style={{ fontSize: "12px", color: "#94a3b8", padding: "6px 14px", borderRadius: "9999px", border: "1px solid rgba(255,255,255,0.12)", background: "rgba(255,255,255,0.04)" }}>{f}</span>
            ))}
          </div>
        </div>
      </div>

      {/* Features Grid */}
      <div style={{ maxWidth: "900px", margin: "0 auto", padding: "0 20px 80px" }}>
        <h2 style={{ fontSize: "24px", fontWeight: 700, color: "#fff", textAlign: "center", marginBottom: "40px" }}>Why Lintasan?</h2>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: "16px" }}>
          {FEATURES.map((f) => (
            <div key={f.title} style={{ background: "rgba(255,255,255,0.04)", border: "1px solid rgba(255,255,255,0.08)", borderRadius: "12px", padding: "24px" }}>
              <div style={{ fontSize: "24px", marginBottom: "12px" }}>{f.icon}</div>
              <h3 style={{ fontSize: "15px", fontWeight: 600, color: "#f0f4f8", marginBottom: "8px" }}>{f.title}</h3>
              <p style={{ fontSize: "13px", color: "#64748b", lineHeight: 1.6 }}>{f.desc}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Quick Start */}
      <div style={{ maxWidth: "640px", margin: "0 auto", padding: "0 20px 80px" }}>
        <h2 style={{ fontSize: "24px", fontWeight: 700, color: "#fff", textAlign: "center", marginBottom: "32px" }}>Quick Start</h2>
        <div style={{ background: "rgba(0,0,0,0.3)", borderRadius: "12px", padding: "24px", border: "1px solid rgba(255,255,255,0.08)" }}>
          <pre style={{ color: "#94a3b8", fontSize: "13px", fontFamily: "'JetBrains Mono', monospace", lineHeight: 1.8, margin: 0, whiteSpace: "pre-wrap" }}>
{`# Install globally
npm install -g lintasan

# Setup & start
lintasan setup
lintasan build
lintasan

# Or use Docker
docker compose up --build

# MITM Bridge (intercept IDE traffic)
lintasan mitm trust
sudo lintasan mitm start`}
          </pre>
        </div>
      </div>

      {/* Footer */}
      <div style={{ textAlign: "center", padding: "40px 20px", borderTop: "1px solid rgba(255,255,255,0.06)" }}>
        <p style={{ fontSize: "13px", color: "#475569" }}>
          MIT License · Built with AI · Orchestrated by <a href="https://github.com/sanhaji182" style={{ color: "#3c50e0", textDecoration: "none" }}>Sanhaji</a>
        </p>
      </div>
    </div>
  );
}

const FEATURES = [
  { icon: "⚡", title: "2360x Faster (Cached)", desc: "Embedding-based semantic cache. Repeat queries return in 4ms instead of 10 seconds." },
  { icon: "🔀", title: "Smart Routing", desc: "Multi-provider combos with priority, round-robin, fallback chains, and circuit breaker." },
  { icon: "🔒", title: "MITM Bridge", desc: "Intercept Copilot, Cursor, and Kiro traffic. Zero IDE config needed." },
  { icon: "🔌", title: "Plugin System", desc: "Extend with custom plugins. AI-powered generator creates plugins from natural language." },
  { icon: "🔑", title: "OAuth Device Flow", desc: "Connect GitHub Copilot subscription without separate API keys." },
  { icon: "📊", title: "Real-time Analytics", desc: "Streaming metrics, per-combo stats, cost tracking, and token savings dashboard." },
  { icon: "🐳", title: "Docker Ready", desc: "One command deployment. Self-host anywhere with docker compose up." },
  { icon: "🌐", title: "27 Provider Presets", desc: "OpenAI, Anthropic, DeepSeek, Gemini, Groq, CommandCode, Sumopod, and more." },
  { icon: "☁️", title: "Cloud Sync", desc: "Export/import config across devices. Share setup via share codes." },
];

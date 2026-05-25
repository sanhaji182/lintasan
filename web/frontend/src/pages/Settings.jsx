
import { useState, useEffect } from "react";

export default function SettingsPage() {
  const [settings, setSettings] = useState({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [availableModels, setAvailableModels] = useState([]);

  useEffect(() => {
    fetch("/api/settings", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setSettings(d.data || {}); setLoading(false); })
      .catch(() => setLoading(false));
    // Fetch available models for AI Agent selector
    fetch("/api/v1/models", { credentials: "include" })
      .then(r => r.json())
      .then(d => { setAvailableModels(d.data || []); })
      .catch(() => {});
  }, []);

  async function saveSetting(key, value) {
    setSaving(true);
    const newSettings = { ...settings, [key]: value };
    setSettings(newSettings);
    await fetch("/api/settings", { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "include", body: JSON.stringify({ key, value }) });
    setSaving(false);
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  }

  if (loading) return <LoadingSkeleton />;

  return (
    <div className="fade-in">
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "24px" }}>
        <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Configure Lintasanter features and optimization</p>
        {saved && <span style={{ fontSize: "12px", color: "var(--success)", padding: "4px 12px", background: "var(--success-light)", borderRadius: "9999px" }}>Saved ✓</span>}
      </div>

      {/* Token Optimization */}
      <div style={card}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "20px" }}>
          <div style={iconBadge}><IconZap /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Token Optimization</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Save tokens automatically on every request</p>
          </div>
        </div>

        {/* RTK Toggle */}
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "14px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", marginBottom: "8px", border: "1px solid var(--border)" }}>
          <div>
            <p style={{ fontSize: "13px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>RTK (Input Compression)</p>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: "2px 0 0" }}>Compresses tool outputs — saves 20-40% input tokens</p>
          </div>
          <ToggleSwitch checked={settings.rtk_enabled !== "false"} onChange={v => saveSetting("rtk_enabled", v ? "true" : "false")} />
        </div>

        {/* Caveman Mode */}
        <div style={{ padding: "14px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "10px" }}>
            <div>
              <p style={{ fontSize: "13px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Caveman Mode (Output Compression)</p>
              <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: "2px 0 0" }}>LLM replies in terse style — saves up to 65% output tokens</p>
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "8px" }}>
            {[
              { value: "off", label: "Off", desc: "Normal responses" },
              { value: "lite", label: "Lite", desc: "~20% savings" },
              { value: "full", label: "Full", desc: "~40% savings" },
              { value: "ultra", label: "Ultra", desc: "~65% savings" },
            ].map(opt => (
              <button key={opt.value} onClick={() => saveSetting("caveman_mode", opt.value)} style={{ padding: "10px", background: (settings.caveman_mode || "off") === opt.value ? "var(--primary-light)" : "var(--bg-card)", border: (settings.caveman_mode || "off") === opt.value ? "2px solid var(--primary)" : "1px solid var(--border)", borderRadius: "var(--radius-sm)", cursor: "pointer", textAlign: "center" }}>
                <p style={{ fontSize: "13px", fontWeight: 600, color: (settings.caveman_mode || "off") === opt.value ? "var(--primary)" : "var(--fg-0)", margin: 0 }}>{opt.label}</p>
                <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>{opt.desc}</p>
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* API & Security */}
      <div style={{ ...card, marginTop: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "20px" }}>
          <div style={iconBadge}><IconKey /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>API & Security</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Authentication and access control</p>
          </div>
        </div>

        <div style={{ display: "grid", gap: "12px" }}>
          <SettingRow label="Master API Key" description="Used to authenticate proxy requests" type="password" value={settings.master_key || ""} onChange={v => saveSetting("master_key", v)} placeholder="sk-..." />
          <SettingRow label="Dashboard Password" description="Password for dashboard login" type="password" value={settings.dashboard_password || ""} onChange={v => saveSetting("dashboard_password", v)} placeholder="Enter new password" />
          <SettingRow label="Rate Limit (RPM)" description="Max requests per minute per key (0 = unlimited)" type="number" value={settings.rate_limit_rpm || "60"} onChange={v => saveSetting("rate_limit_rpm", v)} placeholder="60" />
        </div>
      </div>

      {/* Caching */}
      <div style={{ ...card, marginTop: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "20px" }}>
          <div style={iconBadge}><IconCache /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Caching</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Response caching for faster repeated queries</p>
          </div>
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
            <div>
              <p style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)", margin: 0 }}>Exact Match Cache</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>Cache identical requests</p>
            </div>
            <ToggleSwitch checked={settings.cache_enabled !== "false"} onChange={v => saveSetting("cache_enabled", v ? "true" : "false")} />
          </div>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
            <div>
              <p style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)", margin: 0 }}>Semantic Cache</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>Cache similar requests (TF-IDF matching)</p>
            </div>
            <ToggleSwitch checked={settings.semantic_cache_enabled === "true"} onChange={v => saveSetting("semantic_cache_enabled", v ? "true" : "false")} />
          </div>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
            <div>
              <p style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)", margin: 0 }}>Stream Cache</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>Cache streaming responses for replay</p>
            </div>
            <ToggleSwitch checked={settings.stream_cache_enabled === "true"} onChange={v => saveSetting("stream_cache_enabled", v ? "true" : "false")} />
          </div>
        </div>
      </div>

      {/* Embedding Cache */}
      <div style={{ ...card, marginTop: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "20px" }}>
          <div style={iconBadge}><IconBrain /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Embedding Cache</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Vector similarity cache using real embeddings (more accurate than TF-IDF)</p>
          </div>
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
            <div>
              <p style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)", margin: 0 }}>Enable Embedding Cache</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>Use real embeddings for semantic similarity matching</p>
            </div>
            <ToggleSwitch checked={settings.embedding_cache_enabled === "true"} onChange={v => saveSetting("embedding_cache_enabled", v ? "true" : "false")} />
          </div>

          <SettingRow label="Provider URL" description="Base URL for embedding API (OpenAI-compatible)" type="text" value={settings.embedding_provider_url || "https://ai.sumopod.com/v1"} onChange={v => saveSetting("embedding_provider_url", v)} placeholder="https://ai.sumopod.com/v1" />
          <SettingRow label="API Key" description="Authentication key for embedding provider" type="password" value={settings.embedding_api_key || ""} onChange={v => saveSetting("embedding_api_key", v)} placeholder="sk-..." />

          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
            <div style={{ flexGrow: 1, marginRight: "16px" }}>
              <p style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)", margin: 0 }}>Embedding Model</p>
              <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>Model used to generate embeddings</p>
            </div>
            <select
              value={settings.embedding_model || "text-embedding-3-small"}
              onChange={e => saveSetting("embedding_model", e.target.value)}
              style={{ padding: "8px 12px", background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "4px", fontSize: "12px", fontFamily: "var(--mono)", color: "var(--fg-0)", outline: "none", minWidth: "220px", cursor: "pointer" }}
            >
              <option value="text-embedding-3-small">text-embedding-3-small</option>
              <option value="text-embedding-3-large">text-embedding-3-large</option>
              <option value="text-embedding-ada-002">text-embedding-ada-002</option>
            </select>
          </div>

          <SettingRow label="Similarity Threshold" description="Minimum cosine similarity to consider a cache hit (0.0-1.0)" type="number" value={settings.embedding_similarity_threshold || "0.88"} onChange={v => saveSetting("embedding_similarity_threshold", v)} placeholder="0.88" />
        </div>
      </div>

      {/* Advanced */}
      <div style={{ ...card, marginTop: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "20px" }}>
          <div style={iconBadge}><IconSettings /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>Advanced</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Fine-tune routing behavior</p>
          </div>
        </div>

        <div style={{ display: "grid", gap: "12px" }}>
          <SettingRow label="Provider Timeout (ms)" description="Max wait time for upstream response" type="number" value={settings.provider_timeout || "30000"} onChange={v => saveSetting("provider_timeout", v)} placeholder="30000" />
          <SettingRow label="Max Retries" description="Retry count on retryable errors (429, 503)" type="number" value={settings.max_retries || "2"} onChange={v => saveSetting("max_retries", v)} placeholder="2" />
          <SettingRow label="Cache TTL (seconds)" description="How long cached responses are valid" type="number" value={settings.cache_ttl || "3600"} onChange={v => saveSetting("cache_ttl", v)} placeholder="3600" />
        </div>
      </div>

      {/* AI Agent Model */}
      <div style={{ ...card, marginTop: "16px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "20px" }}>
          <div style={iconBadge}><IconAI /></div>
          <div>
            <h2 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>AI Agent Model</h2>
            <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: 0, marginTop: "2px" }}>Model used by plugin generator and AI-powered features</p>
          </div>
        </div>

        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "14px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
          <div style={{ flexGrow: 1, marginRight: "16px" }}>
            <p style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)", margin: 0 }}>Selected Model</p>
            <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>Choose which model powers AI features like plugin generation</p>
          </div>
          <select
            value={settings.ai_agent_model || ""}
            onChange={e => saveSetting("ai_agent_model", e.target.value)}
            style={{ padding: "8px 12px", background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "4px", fontSize: "12px", fontFamily: "var(--mono)", color: "var(--fg-0)", outline: "none", minWidth: "220px", cursor: "pointer" }}
          >
            <option value="">Auto-detect (first available)</option>
            {availableModels.map(m => (
              <option key={m.id} value={m.id}>{m.id}</option>
            ))}
          </select>
        </div>
      </div>
    </div>
  );
}

function SettingRow({ label, description, type, value, onChange, placeholder }) {
  const [localValue, setLocalValue] = useState(value);
  const [editing, setEditing] = useState(false);

  useEffect(() => { setLocalValue(value); }, [value]);

  function handleSave() {
    onChange(localValue);
    setEditing(false);
  }

  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", border: "1px solid var(--border)" }}>
      <div style={{ flexGrow: 1, marginRight: "16px" }}>
        <p style={{ fontSize: "13px", fontWeight: 500, color: "var(--fg-0)", margin: 0 }}>{label}</p>
        <p style={{ fontSize: "11px", color: "var(--fg-3)", margin: "2px 0 0" }}>{description}</p>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
        <input type={type} value={localValue} onChange={e => { setLocalValue(e.target.value); setEditing(true); }} placeholder={placeholder} style={{ width: type === "number" ? "80px" : "180px", padding: "6px 10px", background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "4px", fontSize: "12px", fontFamily: "var(--mono)", color: "var(--fg-0)", outline: "none", textAlign: type === "number" ? "center" : "left" }} />
        {editing && <button onClick={handleSave} style={{ padding: "4px 10px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "4px", fontSize: "11px", cursor: "pointer" }}>Save</button>}
      </div>
    </div>
  );
}

function ToggleSwitch({ checked, onChange }) {
  return (
    <button onClick={() => onChange(!checked)} style={{ width: "44px", height: "24px", borderRadius: "12px", background: checked ? "var(--primary)" : "var(--border)", border: "none", cursor: "pointer", position: "relative", transition: "var(--transition)", flexShrink: 0 }}>
      <div style={{ width: "18px", height: "18px", borderRadius: "50%", background: "#fff", position: "absolute", top: "3px", left: checked ? "23px" : "3px", transition: "var(--transition)", boxShadow: "0 1px 3px rgba(0,0,0,0.2)" }} />
    </button>
  );
}

function LoadingSkeleton() {
  return (
    <div>
      <div style={{ marginBottom: "24px" }}><div className="skeleton" style={{ width: "280px", height: "14px", borderRadius: "6px" }} /></div>
      {[1,2,3].map(i => <div key={i} className="skeleton" style={{ height: "180px", borderRadius: "var(--radius)", marginBottom: "16px" }} />)}
    </div>
  );
}

// Icons
function IconZap() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>; }
function IconKey() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>; }
function IconCache() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg>; }
function IconSettings() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9c.26.604.852.997 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>; }
function IconAI() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 2a4 4 0 0 1 4 4v2a4 4 0 0 1-8 0V6a4 4 0 0 1 4-4z"/><path d="M16 14h.01"/><path d="M8 14h.01"/><path d="M12 17v3"/><path d="M8 21h8"/><rect x="5" y="11" width="14" height="8" rx="2"/></svg>; }
function IconBrain() { return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M9.5 2A5.5 5.5 0 0 0 4 7.5c0 1.58.67 3 1.74 4.01L4 13l1.5 1.5L4 16l2 2 1.5-1.5L9 18l-1.74 1.49A5.5 5.5 0 0 0 9.5 22a5.5 5.5 0 0 0 5.26-3.87"/><path d="M14.5 2A5.5 5.5 0 0 1 20 7.5c0 1.58-.67 3-1.74 4.01L20 13l-1.5 1.5L20 16l-2 2-1.5-1.5L15 18l1.74 1.49A5.5 5.5 0 0 1 14.5 22a5.5 5.5 0 0 1-5.26-3.87"/><circle cx="12" cy="8" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="16" r="1"/></svg>; }

// Styles
const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)" };
const iconBadge = { width: "36px", height: "36px", borderRadius: "8px", background: "var(--bg-body)", display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0 };

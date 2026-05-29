"use client";
import { useState } from "react";

export default function PluginGeneratorPage() {
  const [prompt, setPrompt] = useState("");
  const [name, setName] = useState("");
  const [generating, setGenerating] = useState(false);
  const [result, setResult] = useState(null); // { name, code, model_used }
  const [installing, setInstalling] = useState(false);
  const [installed, setInstalled] = useState(false);
  const [error, setError] = useState("");

  const generate = async () => {
    if (!prompt.trim()) return;
    setGenerating(true);
    setResult(null);
    setError("");
    setInstalled(false);

    try {
      const res = await fetch("/api/plugins/generate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ prompt: prompt.trim(), name: name.trim() || undefined })
      });
      const data = await res.json();
      if (data.ok) {
        setResult(data);
      } else {
        setError(data.error || "Generation failed");
        if (data.generated) setResult({ name: "failed", code: data.generated, model_used: "" });
      }
    } catch (e) {
      setError("Network error: " + e.message);
    }
    setGenerating(false);
  };

  const install = async () => {
    if (!result) return;
    setInstalling(true);
    try {
      const res = await fetch("/api/plugins", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ action: "create", name: result.name, code: result.code })
      });
      const data = await res.json();
      if (data.ok) {
        setInstalled(true);
      } else {
        setError(data.error || "Install failed");
      }
    } catch (e) {
      setError("Install error: " + e.message);
    }
    setInstalling(false);
  };

  return (
    <div className="fade-in" style={{ maxWidth: "900px" }}>
      {/* Header */}
      <div style={{ marginBottom: "24px" }}>
        <h1 style={{ fontSize: "18px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>
          ✨ AI Plugin Generator
        </h1>
        <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>
          Describe what you want your plugin to do — AI will generate the code for you
        </p>
      </div>

      {/* Input Section */}
      <div style={card}>
        <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "16px" }}>
          <div style={{ width: "36px", height: "36px", borderRadius: "8px", background: "linear-gradient(135deg, var(--primary) 0%, #8b5cf6 100%)", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <span style={{ fontSize: "18px" }}>🤖</span>
          </div>
          <div>
            <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>What should your plugin do?</p>
            <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>Be specific — the more detail, the better the result</p>
          </div>
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "1fr 200px", gap: "14px", marginBottom: "14px" }}>
          <div>
            <label style={labelSt}>Description</label>
            <textarea
              value={prompt}
              onChange={e => setPrompt(e.target.value)}
              style={{ ...input, minHeight: "100px", resize: "vertical" }}
              placeholder="e.g. Block all requests that contain more than 50 messages to save tokens. Return a 400 error telling the user to start a new conversation."
            />
          </div>
          <div>
            <label style={labelSt}>Plugin Name (optional)</label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              style={input}
              placeholder="auto-generated"
            />
            <p style={{ fontSize: "11px", color: "var(--fg-3)", marginTop: "6px" }}>Leave empty to auto-generate from description</p>
          </div>
        </div>

        <div style={{ display: "flex", gap: "10px", alignItems: "center" }}>
          <button onClick={generate} disabled={generating || !prompt.trim()} style={{ ...btnPrimary, opacity: (generating || !prompt.trim()) ? 0.6 : 1, background: "linear-gradient(135deg, var(--primary) 0%, #8b5cf6 100%)" }}>
            {generating ? (
              <><span style={{ display: "inline-block", width: "14px", height: "14px", border: "2px solid rgba(255,255,255,0.3)", borderTopColor: "#fff", borderRadius: "50%", animation: "spin 0.7s linear infinite" }} /> Generating...</>
            ) : (
              "✨ Generate Plugin"
            )}
          </button>
          {generating && <span style={{ fontSize: "12px", color: "var(--fg-3)" }}>Using AI to write your plugin...</span>}
        </div>
      </div>

      {/* Error */}
      {error && (
        <div style={{ ...card, background: "var(--error-light)", borderColor: "rgba(239,68,68,0.3)" }}>
          <p style={{ fontSize: "13px", color: "var(--error)", fontWeight: 500 }}>⚠️ {error}</p>
        </div>
      )}

      {/* Result */}
      {result && result.code && (
        <div style={card} className="fade-in">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "16px" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
              <div style={{ width: "36px", height: "36px", borderRadius: "8px", background: "var(--success-light)", display: "flex", alignItems: "center", justifyContent: "center" }}>
                <span style={{ fontSize: "18px" }}>✅</span>
              </div>
              <div>
                <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)" }}>{result.name}.js</p>
                <p style={{ fontSize: "12px", color: "var(--fg-3)" }}>
                  Generated{result.model_used ? ` by ${result.model_used}` : ""} — review and install
                </p>
              </div>
            </div>
            <div style={{ display: "flex", gap: "8px" }}>
              {!installed ? (
                <button onClick={install} disabled={installing} style={{ ...btnPrimary, background: "var(--success)", opacity: installing ? 0.6 : 1 }}>
                  {installing ? "Installing..." : "📦 Install Plugin"}
                </button>
              ) : (
                <span style={{ display: "inline-flex", alignItems: "center", gap: "6px", padding: "10px 16px", background: "var(--success-light)", color: "var(--success)", borderRadius: "var(--radius-sm)", fontSize: "13px", fontWeight: 600, border: "1px solid rgba(16,185,129,0.3)" }}>
                  ✓ Installed
                </span>
              )}
              <button onClick={() => { navigator.clipboard.writeText(result.code); }} style={btnSecondary}>
                📋 Copy
              </button>
            </div>
          </div>

          {/* Code Preview */}
          <div style={{ position: "relative" }}>
            <pre style={{ background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", padding: "16px", fontSize: "12px", fontFamily: "var(--mono)", lineHeight: "1.7", color: "var(--fg-1)", overflow: "auto", maxHeight: "400px", whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
              {result.code}
            </pre>
          </div>

          {installed && (
            <div style={{ marginTop: "12px", padding: "10px 14px", background: "var(--success-light)", borderRadius: "var(--radius-sm)", border: "1px solid rgba(16,185,129,0.2)" }}>
              <p style={{ fontSize: "12px", color: "var(--success)", fontWeight: 500 }}>
                ✓ Plugin installed and active. Go to <a href="/dashboard/plugins" style={{ color: "var(--primary)", textDecoration: "underline" }}>Plugins page</a> to manage it.
              </p>
            </div>
          )}
        </div>
      )}

      {/* Examples */}
      {!result && !generating && (
        <div style={card}>
          <p style={{ fontSize: "13px", fontWeight: 600, color: "var(--fg-1)", marginBottom: "12px" }}>💡 Example prompts:</p>
          <div style={{ display: "flex", flexDirection: "column", gap: "8px" }}>
            {examples.map((ex, i) => (
              <button key={i} onClick={() => setPrompt(ex)} style={{ textAlign: "left", padding: "10px 14px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", color: "var(--fg-2)", fontSize: "12px", cursor: "pointer", transition: "all 0.15s ease" }}>
                {ex}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

const examples = [
  "Block requests with more than 30 messages and return error asking user to start new conversation",
  "Log every request to a JSON file at data/plugin-logs.json with timestamp, model, and token count",
  "Add a custom header X-Request-ID with a UUID to every response",
  "Rate limit each API key to max 5 requests per minute, return 429 if exceeded",
  "Automatically add a system message reminding the AI to be concise if the conversation has more than 5 messages",
  "Track total tokens used per model per day and log a summary every hour",
  "Block requests to expensive models (gpt-4, claude-3-opus) outside business hours (9am-6pm)",
  "Cache identical requests for 60 seconds and return cached response without hitting upstream",
];

const card = { background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "20px", boxShadow: "var(--shadow)", border: "1px solid var(--border)", marginBottom: "16px" };
const input = { width: "100%", padding: "10px 12px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", color: "var(--fg-0)", fontSize: "13px", outline: "none" };
const labelSt = { display: "block", fontSize: "12px", fontWeight: 500, color: "var(--fg-2)", marginBottom: "6px" };
const btnPrimary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "10px 16px", background: "var(--primary)", color: "#fff", border: "none", borderRadius: "var(--radius-sm)", fontSize: "13px", fontWeight: 500, cursor: "pointer" };
const btnSecondary = { display: "inline-flex", alignItems: "center", gap: "6px", padding: "10px 16px", background: "var(--bg-body)", color: "var(--fg-1)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "13px", fontWeight: 500, cursor: "pointer" };

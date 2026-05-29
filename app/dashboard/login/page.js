"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";

export default function LoginPage() {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const router = useRouter();

  const handleLogin = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError("");
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    });
    const data = await res.json();
    if (data.success) router.push("/dashboard");
    else { setError("Invalid password"); setLoading(false); }
  };

  return (
    <div style={{
      minHeight: "100vh",
      background: `radial-gradient(ellipse at center, rgba(14, 165, 233, 0.06) 0%, transparent 60%),
                   radial-gradient(ellipse at top right, rgba(6, 214, 160, 0.04) 0%, transparent 40%),
                   var(--ocean-deepest)`,
      display: "flex", alignItems: "center", justifyContent: "center",
    }}>
      <div style={{
        width: "380px", padding: "40px",
        background: "var(--glass-bg)",
        borderRadius: "var(--radius-xl)",
        border: "1px solid var(--card-border)",
        backdropFilter: "blur(20px)",
        boxShadow: "var(--shadow-lg), 0 0 60px rgba(14, 165, 233, 0.05)",
      }}>
        {/* Logo */}
        <div style={{ textAlign: "center", marginBottom: "32px" }}>
          <div style={{
            width: "56px", height: "56px", borderRadius: "14px",
            background: "linear-gradient(135deg, var(--cyan-primary), var(--blue-primary))",
            display: "inline-flex", alignItems: "center", justifyContent: "center",
            fontSize: "22px", fontWeight: 700, color: "#fff",
            boxShadow: "0 8px 24px rgba(6, 214, 160, 0.3)",
            marginBottom: "16px",
          }}>L</div>
          <h1 style={{ color: "var(--text-primary)", fontSize: "20px", fontWeight: 600, margin: "0 0 4px", letterSpacing: "-0.3px" }}>Lintasan</h1>
          <p style={{ color: "var(--text-muted)", fontSize: "13px", margin: 0 }}>LLM Proxy Gateway</p>
        </div>

        <form onSubmit={handleLogin}>
          <div style={{ marginBottom: "20px" }}>
            <label style={{ color: "var(--text-muted)", fontSize: "12px", display: "block", marginBottom: "8px", textTransform: "uppercase", letterSpacing: "0.5px", fontWeight: 500 }}>Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter dashboard password"
              style={{
                width: "100%", padding: "12px 16px",
                background: "rgba(4, 18, 37, 0.6)",
                border: error ? "1px solid var(--error)" : "1px solid var(--border-default)",
                borderRadius: "var(--radius-sm)",
                color: "var(--text-primary)", fontSize: "14px",
                outline: "none", transition: "all 0.2s",
                fontFamily: "'Inter', sans-serif",
              }}
              onFocus={(e) => e.target.style.borderColor = "var(--cyan-primary)"}
              onBlur={(e) => e.target.style.borderColor = error ? "var(--error)" : "var(--border-default)"}
              autoFocus
            />
            {error && <p style={{ color: "var(--error)", fontSize: "12px", marginTop: "6px" }}>{error}</p>}
          </div>
          <button type="submit" disabled={loading} style={{
            width: "100%", padding: "12px",
            background: loading ? "var(--ocean-mid)" : "linear-gradient(135deg, var(--cyan-primary), var(--blue-primary))",
            border: "none", borderRadius: "var(--radius-sm)",
            color: "#fff", fontSize: "14px", fontWeight: 600,
            cursor: loading ? "not-allowed" : "pointer",
            transition: "all 0.2s",
            fontFamily: "'Inter', sans-serif",
            boxShadow: loading ? "none" : "0 4px 16px rgba(6, 214, 160, 0.25)",
          }}>
            {loading ? "Authenticating..." : "Login"}
          </button>
        </form>
      </div>
    </div>
  );
}

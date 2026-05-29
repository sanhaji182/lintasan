"use client";
import { useState, useEffect, useRef } from "react";

export default function PlaygroundPage() {
  const [models, setModels] = useState([]);
  const [model, setModel] = useState("");
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [loading, setLoading] = useState(false);
  const [pageLoading, setPageLoading] = useState(true);
  const [masterKey, setMasterKey] = useState("");
  const [showSettings, setShowSettings] = useState(false);
  const [singleTurn, setSingleTurn] = useState(false);
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState(4096);
  const [streamText, setStreamText] = useState("");
  const [copied, setCopied] = useState(null);
  const chatRef = useRef(null);
  const inputRef = useRef(null);

  useEffect(() => {
    Promise.all([
      fetch("/api/v1/models", { credentials: "include" }).then(r => r.json()),
      fetch("/api/settings", { credentials: "include" }).then(r => r.json()).catch(() => ({ data: {} })),
    ]).then(([modelsData, settingsData]) => {
      const list = modelsData.data || [];
      setModels(list);
      if (list.length > 0) setModel(list[0].id);
      const key = settingsData.data?.master_key || localStorage.getItem("sr_master_key") || "";
      setMasterKey(key);
      setPageLoading(false);
    }).catch(() => setPageLoading(false));
  }, []);

  useEffect(() => {
    if (chatRef.current) chatRef.current.scrollTop = chatRef.current.scrollHeight;
  }, [messages, streamText]);

  async function sendMessage(e) {
    e.preventDefault();
    if (!input.trim() || !model || loading) return;

    if (!masterKey) {
      setShowSettings(true);
      return;
    }

    const userMsg = { role: "user", content: input.trim(), time: new Date() };
    const allMessages = [...messages, userMsg];
    setMessages(allMessages);
    setInput("");
    setLoading(true);
    setStreamText("");

    try {
      const reqMessages = singleTurn
        ? (systemPrompt ? [{ role: "system", content: systemPrompt }, { role: "user", content: input.trim() }] : [{ role: "user", content: input.trim() }])
        : (systemPrompt
          ? [{ role: "system", content: systemPrompt }, ...allMessages.map(m => ({ role: m.role, content: m.content }))]
          : allMessages.map(m => ({ role: m.role, content: m.content })));

      const startTime = Date.now();
      const res = await fetch("/api/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Authorization": "Bearer " + masterKey },
        body: JSON.stringify({ model, messages: reqMessages, stream: true, temperature, max_tokens: maxTokens }),
      });

      if (!res.ok) {
        const errData = await res.json().catch(() => ({ error: { message: "Request failed" } }));
        setMessages([...allMessages, { role: "assistant", content: "Error: " + (errData.error?.message || res.statusText), error: true, time: new Date() }]);
        setLoading(false);
        return;
      }

      // Stream reading
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let fullContent = "";
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          if (!line.startsWith("data: ")) continue;
          const data = line.slice(6).trim();
          if (data === "[DONE]") continue;
          try {
            const parsed = JSON.parse(data);
            const delta = parsed.choices?.[0]?.delta?.content || "";
            fullContent += delta;
            setStreamText(fullContent);
          } catch {}
        }
      }

      const latency = Date.now() - startTime;
      const provider = res.headers.get("X-Provider") || "";
      setMessages([...allMessages, {
        role: "assistant",
        content: fullContent || "No response",
        model: model,
        latency,
        provider,
        time: new Date(),
      }]);
      setStreamText("");

    } catch (err) {
      setMessages([...allMessages, { role: "assistant", content: "Network error: " + err.message, error: true, time: new Date() }]);
    }
    setLoading(false);
    if (inputRef.current) inputRef.current.focus();
  }

  function copyMessage(content, idx) {
    navigator.clipboard.writeText(content);
    setCopied(idx);
    setTimeout(() => setCopied(null), 2000);
  }

  function clearChat() { setMessages([]); setStreamText(""); }

  function handleKeyDown(e) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      sendMessage(e);
    }
  }

  // Group models by owned_by/connection
  const modelGroups = {};
  models.forEach(m => {
    const group = m.owned_by || "other";
    if (!modelGroups[group]) modelGroups[group] = [];
    modelGroups[group].push(m);
  });

  if (pageLoading) return <LoadingSkeleton />;

  return (
    <div className="fade-in" style={{ display: "flex", flexDirection: "column", height: "calc(100vh - 80px)" }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", gap: "10px", marginBottom: "12px", padding: "12px 16px", background: "var(--bg-card)", borderRadius: "var(--radius)", border: "1px solid var(--border)" }}>
        <div style={{ flexGrow: 1 }}>
          <select value={model} onChange={e => setModel(e.target.value)} style={selectStyle}>
            {models.length === 0 && <option value="">No models available</option>}
            {Object.entries(modelGroups).map(([group, gModels]) => (
              <optgroup key={group} label={group}>
                {gModels.map(m => <option key={m.id} value={m.id}>{m.id}</option>)}
              </optgroup>
            ))}
          </select>
        </div>
        <div style={{ display: "flex", gap: "6px" }}>
          <button onClick={() => setShowSettings(!showSettings)} style={{ ...btnIcon, color: showSettings ? "var(--primary)" : "var(--fg-2)" }} title="Settings">
            <IconSettings size={16} />
          </button>
          <button onClick={clearChat} style={btnIcon} title="Clear chat">
            <IconClear size={16} />
          </button>
        </div>
      </div>

      {/* Settings panel */}
      {showSettings && (
        <div style={{ padding: "14px 16px", background: "var(--bg-card)", borderRadius: "var(--radius)", border: "1px solid var(--border)", marginBottom: "12px" }}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: "12px", marginBottom: "12px" }}>
            <div>
              <label style={labelStyle}>Temperature ({temperature})</label>
              <input type="range" min="0" max="2" step="0.1" value={temperature} onChange={e => setTemperature(parseFloat(e.target.value))} style={{ width: "100%" }} />
            </div>
            <div>
              <label style={labelStyle}>Max Tokens</label>
              <input type="number" min="1" max="128000" value={maxTokens} onChange={e => setMaxTokens(parseInt(e.target.value) || 4096)} style={inputSmall} />
            </div>
            <div>
              <label style={labelStyle}>API Key {masterKey ? "\u2713" : ""}</label>
              <input type="password" value={masterKey} onChange={e => { setMasterKey(e.target.value); localStorage.setItem("sr_master_key", e.target.value); }} placeholder="Master key" style={inputSmall} />
            </div>
          </div>
          <div style={{ display: "flex", gap: "16px", alignItems: "center", marginBottom: "12px" }}>
            <label style={{ display: "flex", alignItems: "center", gap: "8px", cursor: "pointer", fontSize: "13px", color: "var(--fg-1)" }}>
              <input type="checkbox" checked={singleTurn} onChange={e => setSingleTurn(e.target.checked)} style={{ width: "16px", height: "16px", accentColor: "var(--primary)" }} />
              Single-turn mode
            </label>
            <span style={{ fontSize: "11px", color: "var(--fg-3)" }}>{singleTurn ? "Each message sent independently (cache-friendly)" : "Full conversation history sent (multi-turn)"}</span>
          </div>
          <div>
            <label style={labelStyle}>System Prompt</label>
            <textarea value={systemPrompt} onChange={e => setSystemPrompt(e.target.value)} placeholder="You are a helpful assistant..." style={{ ...inputSmall, height: "50px", resize: "vertical", fontFamily: "var(--mono)" }} />
          </div>
        </div>
      )}

      {/* Chat area */}
      <div ref={chatRef} style={{ flexGrow: 1, overflowY: "auto", marginBottom: "12px", padding: "20px", background: "var(--bg-body)", borderRadius: "var(--radius)", border: "1px solid var(--border)" }}>
        {messages.length === 0 && !streamText ? (
          <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100%", flexDirection: "column", gap: "16px" }}>
            <div style={{ width: "56px", height: "56px", borderRadius: "16px", background: "var(--primary-light)", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <IconChat size={28} color="var(--primary)" />
            </div>
            <div style={{ textAlign: "center" }}>
              <p style={{ fontSize: "15px", fontWeight: 600, color: "var(--fg-0)", marginBottom: "4px" }}>Playground</p>
              <p style={{ fontSize: "13px", color: "var(--fg-3)" }}>Test any model. Select one above and start chatting.</p>
            </div>
            <div style={{ display: "flex", gap: "8px", flexWrap: "wrap", justifyContent: "center", marginTop: "8px" }}>
              {["Hello! Who are you?", "Write a haiku about AI", "Explain quantum computing simply"].map(s => (
                <button key={s} onClick={() => setInput(s)} style={suggestionChip}>{s}</button>
              ))}
            </div>
          </div>
        ) : (
          <>
            {messages.map((msg, i) => (
              <MessageBubble key={i} msg={msg} idx={i} copied={copied} onCopy={copyMessage} />
            ))}
            {streamText && (
              <MessageBubble msg={{ role: "assistant", content: streamText, streaming: true }} idx={-1} copied={null} onCopy={() => {}} />
            )}
            {loading && !streamText && (
              <div style={{ display: "flex", gap: "10px", marginBottom: "16px" }}>
                <div style={avatarBot}><IconBot /></div>
                <div style={{ padding: "12px 16px", borderRadius: "12px", background: "var(--bg-card)", border: "1px solid var(--border)" }}>
                  <div style={{ display: "flex", gap: "6px", alignItems: "center" }}>
                    <span className="skeleton" style={{ width: "8px", height: "8px", borderRadius: "50%" }} />
                    <span className="skeleton" style={{ width: "8px", height: "8px", borderRadius: "50%", animationDelay: "0.2s" }} />
                    <span className="skeleton" style={{ width: "8px", height: "8px", borderRadius: "50%", animationDelay: "0.4s" }} />
                  </div>
                </div>
              </div>
            )}
          </>
        )}
      </div>

      {/* Input area */}
      <div style={{ background: "var(--bg-card)", borderRadius: "var(--radius)", border: "1px solid var(--border)", padding: "12px 14px" }}>
        <form onSubmit={sendMessage} style={{ display: "flex", gap: "10px", alignItems: "flex-end" }}>
          <textarea
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={model ? "Type a message... (Enter to send, Shift+Enter for newline)" : "Select a model first"}
            disabled={!model || loading}
            rows={1}
            style={{ ...textareaStyle, minHeight: "40px", maxHeight: "120px", height: Math.min(120, Math.max(40, (input.split("\n").length) * 20 + 20)) + "px" }}
          />
          <button type="submit" disabled={!model || loading || !input.trim()} style={{ ...btnSend, opacity: (!model || loading || !input.trim()) ? 0.4 : 1 }}>
            <IconSend size={18} />
          </button>
        </form>
        <div style={{ display: "flex", justifyContent: "space-between", marginTop: "6px", fontSize: "11px", color: "var(--fg-3)" }}>
          <span>{model || "No model selected"}{singleTurn ? " · single-turn" : ""}</span>
          <span>temp: {temperature} | max: {maxTokens.toLocaleString()}</span>
        </div>
      </div>
    </div>
  );
}

function MessageBubble({ msg, idx, copied, onCopy }) {
  const isUser = msg.role === "user";
  const isError = msg.error;

  return (
    <div style={{ display: "flex", gap: "10px", marginBottom: "16px", flexDirection: isUser ? "row-reverse" : "row" }}>
      {/* Avatar */}
      {isUser ? (
        <div style={avatarUser}><IconUser /></div>
      ) : (
        <div style={avatarBot}><IconBot /></div>
      )}

      {/* Content */}
      <div style={{ maxWidth: "75%", minWidth: "60px" }}>
        <div style={{
          padding: "12px 16px",
          borderRadius: isUser ? "16px 16px 4px 16px" : "16px 16px 16px 4px",
          background: isUser ? "var(--primary)" : isError ? "var(--error)" + "12" : "var(--bg-card)",
          color: isUser ? "#fff" : isError ? "var(--error)" : "var(--fg-0)",
          border: isUser ? "none" : "1px solid " + (isError ? "var(--error)" : "var(--border)"),
          fontSize: "13px",
          lineHeight: "1.7",
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
          position: "relative",
        }}>
          <FormattedContent content={msg.content} isUser={isUser} />

          {/* Copy button */}
          {!isUser && !msg.streaming && (
            <button onClick={() => onCopy(msg.content, idx)} style={{ position: "absolute", top: "6px", right: "6px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "4px", padding: "2px 6px", fontSize: "10px", cursor: "pointer", color: copied === idx ? "var(--success)" : "var(--fg-3)", opacity: 0.7 }}>
              {copied === idx ? "\u2713" : "copy"}
            </button>
          )}
        </div>

        {/* Meta info */}
        {!isUser && !isError && !msg.streaming && (
          <div style={{ display: "flex", gap: "8px", marginTop: "4px", fontSize: "11px", color: "var(--fg-3)", paddingLeft: "4px" }}>
            {msg.provider && <span style={{ padding: "1px 6px", background: "var(--success)" + "18", color: "var(--success)", borderRadius: "4px", fontWeight: 500 }}>{msg.provider}</span>}
            {msg.latency && <span>{(msg.latency / 1000).toFixed(1)}s</span>}
            {msg.tokens && <span>{msg.tokens.total_tokens} tokens</span>}
          </div>
        )}
        {msg.streaming && (
          <div style={{ marginTop: "4px", fontSize: "11px", color: "var(--fg-3)", paddingLeft: "4px" }}>
            <span style={{ color: "var(--primary)" }}>\u25CF</span> Streaming...
          </div>
        )}
      </div>
    </div>
  );
}

function FormattedContent({ content, isUser }) {
  if (isUser) return <>{content}</>;

  // Simple code block detection and rendering
  const parts = content.split(/(```[\s\S]*?```)/g);
  return (
    <>
      {parts.map((part, i) => {
        if (part.startsWith("```") && part.endsWith("```")) {
          const lines = part.slice(3, -3).split("\n");
          const lang = lines[0].trim();
          const code = (lang && !lang.includes(" ")) ? lines.slice(1).join("\n") : lines.join("\n");
          return (
            <pre key={i} style={{ background: "#1e1e2e", color: "#cdd6f4", padding: "12px 14px", borderRadius: "8px", fontSize: "12px", fontFamily: "var(--mono)", overflowX: "auto", margin: "8px 0", lineHeight: "1.5", border: "1px solid #313244" }}>
              {lang && !lang.includes(" ") && <div style={{ fontSize: "10px", color: "#6c7086", marginBottom: "6px", textTransform: "uppercase" }}>{lang}</div>}
              <code>{code}</code>
            </pre>
          );
        }
        // Inline code
        const inlineParts = part.split(/(`[^`]+`)/g);
        return (
          <span key={i}>
            {inlineParts.map((ip, j) => {
              if (ip.startsWith("`") && ip.endsWith("`")) {
                return <code key={j} style={{ background: "var(--bg-body)", padding: "1px 5px", borderRadius: "4px", fontSize: "12px", fontFamily: "var(--mono)", border: "1px solid var(--border)" }}>{ip.slice(1, -1)}</code>;
              }
              // Bold
              const boldParts = ip.split(/(\*\*[^*]+\*\*)/g);
              return (
                <span key={j}>
                  {boldParts.map((bp, k) => {
                    if (bp.startsWith("**") && bp.endsWith("**")) {
                      return <strong key={k}>{bp.slice(2, -2)}</strong>;
                    }
                    return <span key={k}>{bp}</span>;
                  })}
                </span>
              );
            })}
          </span>
        );
      })}
    </>
  );
}

function LoadingSkeleton() {
  return (
    <div style={{ display: "flex", flexDirection: "column", height: "calc(100vh - 80px)" }}>
      <div className="skeleton" style={{ height: "52px", borderRadius: "var(--radius)", marginBottom: "12px" }} />
      <div className="skeleton" style={{ flexGrow: 1, borderRadius: "var(--radius)", marginBottom: "12px" }} />
      <div className="skeleton" style={{ height: "70px", borderRadius: "var(--radius)" }} />
    </div>
  );
}

// Icons
function IconChat({ size = 14, color }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color || "currentColor"} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>; }
function IconSend({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>; }
function IconClear({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>; }
function IconSettings({ size = 14 }) { return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>; }
function IconUser() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>; }
function IconBot() { return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="11" width="18" height="10" rx="2"/><circle cx="12" cy="5" r="2"/><path d="M12 7v4"/><line x1="8" y1="16" x2="8" y2="16"/><line x1="16" y1="16" x2="16" y2="16"/></svg>; }

// Styles
const selectStyle = { width: "100%", padding: "8px 12px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "13px", color: "var(--fg-0)", outline: "none" };
const inputSmall = { width: "100%", padding: "7px 10px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "12px", color: "var(--fg-0)", outline: "none", boxSizing: "border-box" };
const textareaStyle = { width: "100%", padding: "10px 14px", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", fontSize: "13px", color: "var(--fg-0)", outline: "none", resize: "none", lineHeight: "1.5", fontFamily: "inherit", boxSizing: "border-box" };
const btnIcon = { width: "36px", height: "36px", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", cursor: "pointer", color: "var(--fg-2)" };
const btnSend = { width: "40px", height: "40px", display: "flex", alignItems: "center", justifyContent: "center", background: "var(--primary)", border: "none", borderRadius: "var(--radius-sm)", cursor: "pointer", color: "#fff", flexShrink: 0 };
const labelStyle = { display: "block", fontSize: "11px", fontWeight: 500, color: "var(--fg-3)", marginBottom: "4px" };
const suggestionChip = { fontSize: "12px", padding: "8px 14px", background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "20px", cursor: "pointer", color: "var(--fg-1)", transition: "var(--transition)" };
const avatarUser = { width: "32px", height: "32px", borderRadius: "50%", background: "var(--primary)", display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", flexShrink: 0 };
const avatarBot = { width: "32px", height: "32px", borderRadius: "50%", background: "var(--bg-card)", border: "1px solid var(--border)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--fg-2)", flexShrink: 0 };

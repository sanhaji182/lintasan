"use client";
import { useState } from "react";

const card = (color) => ({ background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: "var(--radius)", marginBottom: "16px", overflow: "hidden" });
const cardHeader = (color) => ({ background: color + "0a", borderBottom: "1px solid " + color + "20", padding: "16px 24px", display: "flex", alignItems: "center", gap: "12px" });
const cardBody = { padding: "20px 24px" };
const iconBadge = (color) => ({ width: "36px", height: "36px", borderRadius: "var(--radius-sm)", background: color + "20", display: "flex", alignItems: "center", justifyContent: "center" });

export default function DocsPage() {
  const [activeSection, setActiveSection] = useState("tour");
  const [lang, setLang] = useState("en");

  const sections = [
    { id: "tour", label: lang === "en" ? "Dashboard Tour" : "Tur Dashboard", color: "var(--primary)" },
    { id: "quickstart", label: lang === "en" ? "Quick Start" : "Mulai Cepat", color: "var(--success)" },
    { id: "connect", label: lang === "en" ? "Add Provider" : "Tambah Provider", color: "var(--info)" },
    { id: "models", label: "Models", color: "var(--purple)" },
    { id: "combo", label: "Combo Mode", color: "var(--warning)" },
    { id: "rtk", label: "RTK (Token Saver)", color: "var(--success)" },
    { id: "caveman", label: "Caveman Mode", color: "var(--error)" },
    { id: "routing", label: "Smart Routing", color: "var(--info)" },
    { id: "multikey", label: "Multi-Account", color: "var(--purple)" },
    { id: "quota", label: lang === "en" ? "Quota & Limits" : "Kuota & Batas", color: "var(--warning)" },
    { id: "fallback", label: "Fallback Chains", color: "var(--error)" },
    { id: "plugins", label: "Plugins", color: "var(--purple)" },
    { id: "teams", label: "Teams & Users", color: "var(--info)" },
    { id: "webhooks", label: "Webhooks", color: "var(--warning)" },
    { id: "backup", label: "Backup & Export", color: "var(--success)" },
    { id: "endpoints", label: lang === "en" ? "Extra Endpoints" : "Endpoint Tambahan", color: "var(--primary)" },
    { id: "tools", label: lang === "en" ? "Tool Integration" : "Integrasi Tool", color: "var(--primary)" },
    { id: "api", label: "API Reference", color: "var(--fg-2)" },
  ];

  return (
    <div className="fade-in" style={{ display: "flex", gap: "24px", height: "calc(100vh - 80px)" }}>
      <div style={{ width: "220px", flexShrink: 0, overflowY: "auto", paddingRight: "16px", borderRight: "1px solid var(--border)", background: "var(--bg-card)", borderRadius: "var(--radius)", padding: "16px", marginRight: "8px" }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "16px" }}>
          <p style={{ fontSize: "11px", fontWeight: 600, color: "var(--fg-3)", letterSpacing: "0.5px", textTransform: "uppercase", margin: 0 }}>Documentation</p>
          <div style={{ display: "flex", borderRadius: "var(--radius-sm)", overflow: "hidden", border: "1px solid var(--border)" }}>
            <button onClick={() => setLang("en")} style={{ padding: "4px 8px", fontSize: "11px", fontWeight: 600, border: "none", cursor: "pointer", background: lang === "en" ? "var(--primary)" : "var(--bg-body)", color: lang === "en" ? "white" : "var(--fg-3)", transition: "var(--transition)" }}>EN</button>
            <button onClick={() => setLang("id")} style={{ padding: "4px 8px", fontSize: "11px", fontWeight: 600, border: "none", borderLeft: "1px solid var(--border)", cursor: "pointer", background: lang === "id" ? "var(--primary)" : "var(--bg-body)", color: lang === "id" ? "white" : "var(--fg-3)", transition: "var(--transition)" }}>ID</button>
          </div>
        </div>
        {sections.map(s => (
          <button key={s.id} onClick={() => setActiveSection(s.id)} style={{ display: "flex", alignItems: "center", gap: "8px", width: "100%", textAlign: "left", padding: "8px 12px", marginBottom: "2px", background: activeSection === s.id ? "var(--primary-light)" : "transparent", color: activeSection === s.id ? "var(--primary)" : "var(--fg-2)", border: "none", borderRadius: "var(--radius-sm)", fontSize: "13px", fontWeight: activeSection === s.id ? 600 : 400, cursor: "pointer", transition: "var(--transition)" }}>
            <span style={{ width: "6px", height: "6px", borderRadius: "50%", background: activeSection === s.id ? s.color : "var(--border)", flexShrink: 0 }} />
            {s.label}
          </button>
        ))}
      </div>
      <div style={{ flexGrow: 1, overflowY: "auto", padding: "0 8px" }}>
        {activeSection === "tour" && <DashboardTour lang={lang} />}
        {activeSection === "quickstart" && <QuickStart lang={lang} />}
        {activeSection === "connect" && <ConnectProviders lang={lang} />}
        {activeSection === "models" && <ModelsSync lang={lang} />}
        {activeSection === "combo" && <ComboMode lang={lang} />}
        {activeSection === "rtk" && <RtkSection lang={lang} />}
        {activeSection === "caveman" && <CavemanSection lang={lang} />}
        {activeSection === "routing" && <RoutingSection lang={lang} />}
        {activeSection === "multikey" && <MultiKeySection lang={lang} />}
        {activeSection === "quota" && <QuotaSection lang={lang} />}
        {activeSection === "fallback" && <FallbackSection lang={lang} />}
        {activeSection === "plugins" && <PluginsSection lang={lang} />}
        {activeSection === "teams" && <TeamsSection lang={lang} />}
        {activeSection === "webhooks" && <WebhooksSection lang={lang} />}
        {activeSection === "backup" && <BackupSection lang={lang} />}
        {activeSection === "endpoints" && <EndpointsSection lang={lang} />}
        {activeSection === "tools" && <ToolsSection lang={lang} />}
        {activeSection === "api" && <ApiSection lang={lang} />}
      </div>
    </div>
  );
}

function SectionCard({ title, subtitle, color = "var(--primary)", children }) {
  return (
    <div style={card(color)}>
      <div style={cardHeader(color)}>
        <div style={iconBadge(color)}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>
        </div>
        <div>
          <h2 style={{ fontSize: "16px", fontWeight: 700, color: "var(--fg-0)", margin: 0 }}>{title}</h2>
          {subtitle && <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: "2px 0 0" }}>{subtitle}</p>}
        </div>
      </div>
      <div style={cardBody}>
        {children}
      </div>
    </div>
  );
}

function CodeBlock({ code }) {
  return (
    <pre style={{ background: "var(--bg-body)", border: "1px solid var(--border)", borderRadius: "var(--radius-sm)", padding: "14px 16px", fontSize: "12px", fontFamily: "var(--mono)", color: "var(--fg-1)", overflowX: "auto", marginBottom: "16px", lineHeight: "1.6", whiteSpace: "pre-wrap" }}>
      <code>{code}</code>
    </pre>
  );
}

function P({ children }) {
  return <p style={{ fontSize: "13px", color: "var(--fg-1)", lineHeight: "1.7", marginBottom: "10px" }}>{children}</p>;
}

function H3({ children }) {
  return <h3 style={{ fontSize: "14px", fontWeight: 600, color: "var(--fg-0)", marginTop: "20px", marginBottom: "8px" }}>{children}</h3>;
}

function Badge({ children, color = "var(--primary)" }) {
  return <span style={{ display: "inline-block", fontSize: "11px", padding: "2px 8px", borderRadius: "9999px", background: color + "18", color, fontWeight: 500, marginRight: "6px" }}>{children}</span>;
}

function Tip({ children }) {
  return (
    <div style={{ borderLeft: "3px solid var(--primary)", background: "var(--primary-light)", borderRadius: "0 var(--radius-sm) var(--radius-sm) 0", padding: "12px 16px", marginBottom: "16px" }}>
      <strong style={{ color: "var(--primary)", fontSize: "11px", textTransform: "uppercase", letterSpacing: "0.3px" }}>Tip</strong>
      <p style={{ margin: "4px 0 0", fontSize: "13px", color: "var(--fg-1)", lineHeight: "1.6" }}>{children}</p>
    </div>
  );
}

function Warning({ children }) {
  return (
    <div style={{ borderLeft: "3px solid var(--warning)", background: "var(--warning)" + "12", borderRadius: "0 var(--radius-sm) var(--radius-sm) 0", padding: "12px 16px", marginBottom: "16px" }}>
      <strong style={{ color: "var(--warning)", fontSize: "11px", textTransform: "uppercase", letterSpacing: "0.3px" }}>Note</strong>
      <p style={{ margin: "4px 0 0", fontSize: "13px", color: "var(--fg-1)", lineHeight: "1.6" }}>{children}</p>
    </div>
  );
}

function Step({ number, title, children }) {
  return (
    <div style={{ display: "flex", gap: "12px", marginBottom: "14px" }}>
      <div style={{ width: "24px", height: "24px", borderRadius: "50%", background: "var(--primary)", color: "white", display: "flex", alignItems: "center", justifyContent: "center", fontSize: "12px", fontWeight: 700, flexShrink: 0 }}>{number}</div>
      <div style={{ flex: 1 }}>
        <p style={{ fontSize: "13px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>{title}</p>
        <p style={{ fontSize: "12px", color: "var(--fg-2)", lineHeight: "1.6", margin: "2px 0 0" }}>{children}</p>
      </div>
    </div>
  );
}

function FeatureRow({ label, desc }) {
  return (
    <div style={{ display: "flex", alignItems: "flex-start", gap: "12px", padding: "10px 14px", background: "var(--bg-body)", borderRadius: "var(--radius-sm)", marginBottom: "6px", border: "1px solid var(--border)" }}>
      <div style={{ width: "6px", height: "6px", borderRadius: "50%", background: "var(--primary)", marginTop: "6px", flexShrink: 0 }} />
      <div>
        <p style={{ fontSize: "13px", fontWeight: 600, color: "var(--fg-0)", margin: 0 }}>{label}</p>
        <p style={{ fontSize: "12px", color: "var(--fg-3)", margin: "2px 0 0" }}>{desc}</p>
      </div>
    </div>
  );
}

function DashboardTour({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title={en ? "Dashboard Tour" : "Tur Dashboard"} subtitle={en ? "Overview of every page in Lintasan" : "Gambaran semua halaman di Lintasan"} color="var(--primary)">
        <P>{en ? "Everything is manageable from the UI. No terminal needed." : "Semua bisa dikelola dari UI. Tidak perlu terminal."}</P>
      </SectionCard>
      <FeatureRow label="Overview" desc={en ? "Real-time stats: requests today, active connections, cache hit rate, token usage." : "Statistik real-time: request hari ini, koneksi aktif, cache hit rate, pemakaian token."} />
      <FeatureRow label="Connections" desc={en ? "Add LLM providers. 26 presets available. Click to expand and see discovered models." : "Tambah provider LLM. 26 preset tersedia. Klik untuk expand dan lihat model yang ditemukan."} />
      <FeatureRow label="Routing" desc={en ? "Combos (model sequences), load balancer strategy, and model aliases." : "Combo (urutan model), strategi load balancer, dan alias model."} />
      <FeatureRow label="Playground" desc={en ? "Test any model from the browser. Pick model, type message, see response." : "Test model apapun dari browser. Pilih model, ketik pesan, lihat respons."} />
      <FeatureRow label="Logs" desc={en ? "Every request logged: model, provider, latency, tokens, cache status. Paginated." : "Semua request tercatat: model, provider, latensi, token, status cache. Berpaginasi."} />
      <FeatureRow label="Usage" desc={en ? "Token breakdown by provider/model. Daily and monthly charts." : "Rincian token per provider/model. Grafik harian dan bulanan."} />
      <FeatureRow label="API Keys" desc={en ? "Create keys for different tools/users. Per-key usage tracking. Revoke anytime." : "Buat key untuk tool/user berbeda. Tracking pemakaian per key. Cabut kapan saja."} />
      <FeatureRow label="Settings" desc={en ? "Toggle all features: RTK, Caveman, cache, rate limits, master key." : "Toggle semua fitur: RTK, Caveman, cache, rate limit, master key."} />
      <FeatureRow label="Docs" desc={en ? "You are here. Reference for all features and API endpoints." : "Kamu di sini. Referensi semua fitur dan endpoint API."} />
    </>
  );
}

function QuickStart({ lang }) {
  const en = lang === "en";
  return (
    <SectionCard title={en ? "Quick Start" : "Mulai Cepat"} subtitle={en ? "Get Lintasan working in 3 steps" : "Lintasan jalan dalam 3 langkah"} color="var(--success)">
      <Step number={1} title={en ? "Add a provider" : "Tambah provider"}>{en ? "Go to Connections page, click Add Provider, pick a preset, paste your API key, Save." : "Buka halaman Connections, klik Add Provider, pilih preset, paste API key kamu, Save."}</Step>
      <Step number={2} title={en ? "Test it" : "Test"}>{en ? "Go to Playground, pick any discovered model, type a message, Send." : "Buka Playground, pilih model yang ditemukan, ketik pesan, Send."}</Step>
      <Step number={3} title={en ? "Connect your tools" : "Hubungkan tool kamu"}>{en ? "Point any OpenAI-compatible tool to Lintasan:" : "Arahkan tool yang OpenAI-compatible ke Lintasan:"}</Step>
      <CodeBlock code={"Base URL: http://100.99.2.14:20180/api/v1\nAPI Key:  " + (en ? "(your master key from Settings)" : "(master key dari halaman Settings)") + "\nModel:    " + (en ? "any discovered model or combo name" : "model apapun atau nama combo")} />
      <Tip>{en ? "Your master key is on the Settings page. Copy it and use as API key in any tool." : "Master key ada di halaman Settings. Copy dan pakai sebagai API key di tool apapun."}</Tip>
    </SectionCard>
  );
}

function ConnectProviders({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title={en ? "Add a Provider" : "Tambah Provider"} subtitle={en ? "26 providers supported out of the box" : "26 provider didukung langsung"} color="var(--info)">
        <Step number={1} title={en ? "Open Connections page" : "Buka halaman Connections"}>{en ? "Click Connections in the sidebar." : "Klik Connections di sidebar."}</Step>
        <Step number={2} title={en ? "Click Add Provider" : "Klik Add Provider"}>{en ? "You will see a grid of 26 provider presets." : "Kamu akan lihat grid 26 preset provider."}</Step>
        <Step number={3} title={en ? "Pick your provider" : "Pilih provider"}>{en ? "Click it. Base URL, format, paths are auto-filled." : "Klik. Base URL, format, path otomatis terisi."}</Step>
        <Step number={4} title={en ? "Paste your API key" : "Paste API key"}>{en ? "Only thing you need to enter. Hit Save." : "Satu-satunya yang perlu diisi. Klik Save."}</Step>
        <Step number={5} title={en ? "Models auto-sync" : "Model otomatis sync"}>{en ? "Lintasan discovers available models automatically." : "Lintasan menemukan model yang tersedia secara otomatis."}</Step>
      </SectionCard>
      <SectionCard title={en ? "Available Providers" : "Provider Tersedia"} subtitle={en ? "Grouped by category" : "Dikelompokkan per kategori"} color="var(--info)">
        <P><Badge>Major</Badge> OpenAI, Anthropic, DeepSeek, Google Gemini, xAI, Mistral AI</P>
        <P><Badge color="var(--info)">Aggregator</Badge> OpenRouter (200+ models)</P>
        <P><Badge color="var(--success)">Fast</Badge> Groq, Together AI, Fireworks, Cerebras, NVIDIA NIM</P>
        <P><Badge color="var(--warning)">Chinese</Badge> CommandCode, CC Alpha, GLM, Kimi, MiniMax, Qwen, SiliconFlow</P>
        <P><Badge color="var(--purple)">Other</Badge> Perplexity, Cohere, DeepInfra, SambaNova, Nebius AI</P>
        <P><Badge color="var(--fg-2)">Self-Hosted</Badge> Ollama (local), Custom (any OpenAI-compatible URL)</P>
      </SectionCard>
      <SectionCard title="CommandCode Alpha (Free)" subtitle={en ? "Uses CLI token instead of paid API key" : "Pakai token CLI, bukan API key berbayar"} color="var(--warning)">
        <CodeBlock code={"npm install -g command-code\ncmd login\ncmd auth token   <- copy this value"} />
        <P>{en ? "Then: Add Provider, pick CommandCode (Alpha), paste the token as API key." : "Lalu: Add Provider, pilih CommandCode (Alpha), paste token sebagai API key."}</P>
      </SectionCard>
    </>
  );
}

function ModelsSync({ lang }) {
  const en = lang === "en";
  return (
    <SectionCard title="Models" subtitle={en ? "Auto-discovered from connected providers" : "Otomatis ditemukan dari provider yang terhubung"} color="var(--purple)">
      <H3>{en ? "How it works" : "Cara kerja"}</H3>
      <P>{en ? "When you add a provider, Lintasan calls its /models endpoint and imports all available models. They appear as chips on the connection card and in the Playground dropdown." : "Saat kamu tambah provider, Lintasan memanggil endpoint /models dan mengimpor semua model yang tersedia. Muncul sebagai chip di kartu koneksi dan dropdown Playground."}</P>
      <H3>{en ? "Re-sync models" : "Sync ulang model"}</H3>
      <P>{en ? "Click the refresh icon on any connection card, or use Sync All to refresh everything at once." : "Klik ikon refresh di kartu koneksi manapun, atau gunakan Sync All untuk refresh semuanya sekaligus."}</P>
      <H3>{en ? "Same model, multiple providers" : "Model sama, banyak provider"}</H3>
      <P>{en ? "If the same model exists on multiple connections, Lintasan picks the highest-priority connection first. If it fails, falls back to the other." : "Jika model yang sama ada di beberapa koneksi, Lintasan pilih koneksi prioritas tertinggi dulu. Kalau gagal, otomatis fallback ke yang lain."}</P>
      <Tip>{en ? "Control priority by reordering connections. Higher in the list = higher priority." : "Atur prioritas dengan mengurutkan ulang koneksi. Lebih atas = prioritas lebih tinggi."}</Tip>
      <Warning>{en ? "Some providers (CommandCode, Anthropic) have no /models endpoint. Lintasan uses built-in known model lists for these." : "Beberapa provider (CommandCode, Anthropic) tidak punya endpoint /models. Lintasan pakai daftar model bawaan untuk ini."}</Warning>
    </SectionCard>
  );
}

function ComboMode({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title="Combo Mode" subtitle={en ? "Model sequences with auto-fallback" : "Urutan model dengan auto-fallback"} color="var(--warning)">
        <P>{en ? "Create a sequence of models. If one fails or hits quota, it automatically tries the next. Zero downtime." : "Buat urutan model. Kalau satu gagal atau kena kuota, otomatis coba yang berikutnya. Zero downtime."}</P>
        <H3>{en ? "Create from Dashboard" : "Buat dari Dashboard"}</H3>
        <Step number={1} title={en ? "Go to Routing page" : "Buka halaman Routing"}>{en ? "Click Routing in the sidebar." : "Klik Routing di sidebar."}</Step>
        <Step number={2} title={en ? "Click Create Combo" : "Klik Create Combo"}>{en ? "Give it a name like always-on or coding." : "Beri nama seperti always-on atau coding."}</Step>
        <Step number={3} title={en ? "Add models" : "Tambah model"}>{en ? "Pick from discovered models. Order matters." : "Pilih dari model yang ditemukan. Urutan penting."}</Step>
        <Step number={4} title="Save">{en ? "Combo appears as a selectable model everywhere." : "Combo muncul sebagai model yang bisa dipilih di mana saja."}</Step>
      </SectionCard>
      <SectionCard title={en ? "How Fallback Works" : "Cara Fallback Bekerja"} subtitle={en ? "Automatic cascade through model sequence" : "Cascade otomatis melalui urutan model"} color="var(--warning)">
        <CodeBlock code={en ? "Request with model: \"always-on\"\n  -> Try first model in sequence\n  -> If fails (error/timeout/quota) -> try next\n  -> Continue until one succeeds or all fail" : "Request dengan model: \"always-on\"\n  -> Coba model pertama di urutan\n  -> Kalau gagal (error/timeout/kuota) -> coba berikutnya\n  -> Lanjut sampai satu berhasil atau semua gagal"} />
        <H3>Sticky Round-Robin</H3>
        <P>{en ? "Combos stick to the last successful model for 3 requests before rotating. Set sticky limit to 0 for pure round-robin." : "Combo tetap di model terakhir yang berhasil selama 3 request sebelum rotasi. Set sticky limit ke 0 untuk round-robin murni."}</P>
        <H3>{en ? "Example Combos" : "Contoh Combo"}</H3>
        <CodeBlock code={"\"coding\"     -> deepseek-v4-pro -> kimi-k2.6 -> qwen3-coder\n\"cheap\"      -> minimax-m2.7 -> glm-4.7 -> qwen3-coder\n\"always-on\"  -> deepseek -> kimi -> glm -> minimax -> qwen"} />
        <Tip>{en ? "Use combo names as your model in any tool. They show up in /v1/models like regular models." : "Pakai nama combo sebagai model di tool apapun. Muncul di /v1/models seperti model biasa."}</Tip>
      </SectionCard>
    </>
  );
}

function RtkSection({ lang }) {
  const en = lang === "en";
  return (
    <SectionCard title="RTK (Token Saver)" subtitle={en ? "Automatic input compression, 20-40% savings" : "Kompresi input otomatis, hemat 20-40%"} color="var(--success)">
      <H3>{en ? "Enable/Disable" : "Aktifkan/Nonaktifkan"}</H3>
      <P>{en ? "Settings page, find RTK Compression, toggle on/off." : "Halaman Settings, cari RTK Compression, toggle on/off."}</P>
      <H3>{en ? "What it compresses" : "Apa yang dikompres"}</H3>
      <FeatureRow label="git diff" desc={en ? "Reduces context lines, collapses unchanged sections" : "Kurangi baris konteks, lipat bagian yang tidak berubah"} />
      <FeatureRow label="Directory listing" desc={en ? "Removes metadata columns, caps depth" : "Hapus kolom metadata, batasi kedalaman"} />
      <FeatureRow label="grep output" desc={en ? "Deduplicates file paths, compact format" : "Deduplikasi path file, format ringkas"} />
      <FeatureRow label="JSON" desc={en ? "Minifies pretty-printed JSON" : "Minifikasi JSON yang di-pretty-print"} />
      <FeatureRow label="Logs" desc={en ? "Deduplicates repeated patterns" : "Deduplikasi pola yang berulang"} />
      <FeatureRow label="Generic" desc={en ? "Removes excessive whitespace" : "Hapus whitespace berlebihan"} />
      <H3>{en ? "Safety" : "Keamanan"}</H3>
      <P>{en ? "Never returns empty content. Never makes content bigger. Skips content under 100 chars. Preserves code blocks and error traces." : "Tidak pernah mengembalikan konten kosong. Tidak pernah membuat konten lebih besar. Skip konten di bawah 100 karakter. Menjaga code block dan error trace."}</P>
      <Tip>{en ? "Works best with coding agents that send lots of tool output. For simple chat, savings are minimal." : "Paling efektif untuk coding agent yang kirim banyak output tool. Untuk chat biasa, penghematannya minimal."}</Tip>
    </SectionCard>
  );
}

function CavemanSection({ lang }) {
  const en = lang === "en";
  return (
    <SectionCard title="Caveman Mode" subtitle={en ? "Output compression, up to 65% savings" : "Kompresi output, hemat sampai 65%"} color="var(--error)">
      <H3>{en ? "Set Level (from Settings page)" : "Atur Level (dari halaman Settings)"}</H3>
      <FeatureRow label="Lite (~20%)" desc={en ? "Drops filler words and pleasantries. Still readable." : "Buang kata pengisi dan basa-basi. Masih terbaca."} />
      <FeatureRow label="Full (~40%)" desc={en ? "Drops articles, uses fragments, lists over prose." : "Buang artikel, pakai fragmen, list daripada paragraf."} />
      <FeatureRow label="Ultra (~65%)" desc={en ? "Maximum compression. Abbreviations everywhere." : "Kompresi maksimal. Singkatan di mana-mana."} />
      <H3>{en ? "Example" : "Contoh"}</H3>
      <CodeBlock code={en ? "Normal: \"I think the issue is that your database connection\nstring is not properly configured. You should check the\nenvironment variable DATABASE_URL.\"\n\nUltra: \"DB conn string wrong -> check env.DATABASE_URL\"" : "Normal: \"Saya rasa masalahnya adalah connection string\ndatabase kamu tidak dikonfigurasi dengan benar. Kamu harus\ncek environment variable DATABASE_URL.\"\n\nUltra: \"DB conn string salah -> cek env.DATABASE_URL\""} />
      <H3>{en ? "Always preserved" : "Selalu dijaga"}</H3>
      <P>{en ? "Code blocks, file paths, commands, error messages, URLs, security warnings." : "Code block, path file, command, pesan error, URL, peringatan keamanan."}</P>
      <Warning>{en ? "Do not use Ultra for user-facing chat. Best for automated coding agents." : "Jangan pakai Ultra untuk chat dengan user. Paling cocok untuk coding agent otomatis."}</Warning>
    </SectionCard>
  );
}

function RoutingSection({ lang }) {
  const en = lang === "en";
  return (
    <SectionCard title="Smart Routing" subtitle={en ? "Automatic provider selection and fallback" : "Pemilihan provider otomatis dan fallback"} color="var(--info)">
      <H3>{en ? "Routing Flow" : "Alur Routing"}</H3>
      <CodeBlock code={en ? "Request -> Check aliases -> Check combos -> Find connections\n  -> Filter (quota OK, circuit closed)\n  -> Load balance (strategy)\n  -> Execute with retry\n  -> On failure: next connection\n  -> On success: cache + track quota" : "Request masuk -> Cek alias -> Cek combo -> Cari koneksi\n  -> Filter (kuota OK, circuit tertutup)\n  -> Load balance (strategi)\n  -> Eksekusi dengan retry\n  -> Kalau gagal: koneksi berikutnya\n  -> Kalau berhasil: cache + catat kuota"} />
      <H3>{en ? "Load Balance Strategies" : "Strategi Load Balance"}</H3>
      <FeatureRow label={en ? "Priority (default)" : "Prioritas (default)"} desc={en ? "Always uses highest-priority connection first" : "Selalu pakai koneksi prioritas tertinggi dulu"} />
      <FeatureRow label="Round-robin" desc={en ? "Rotates evenly across all connections" : "Rotasi merata ke semua koneksi"} />
      <FeatureRow label="Least-connections" desc={en ? "Picks connection with fewest active requests" : "Pilih koneksi dengan request aktif paling sedikit"} />
      <FeatureRow label="Random" desc={en ? "Random selection" : "Pilihan acak"} />
      <H3>Circuit Breaker</H3>
      <P>{en ? "After 3 consecutive failures, connection is skipped for 30 seconds. Auto-recovers after cooldown." : "Setelah 3 kegagalan berturut-turut, koneksi dilewati selama 30 detik. Otomatis pulih setelah cooldown."}</P>
      <H3>{en ? "Model Aliases (set from Routing page)" : "Alias Model (atur dari halaman Routing)"}</H3>
      <CodeBlock code={"\"gpt4\"   -> gpt-4o\n\"claude\" -> claude-sonnet-4-20250514\n\"ds\"     -> deepseek/deepseek-v4-pro"} />
    </SectionCard>
  );
}

function MultiKeySection({ lang }) {
  const en = lang === "en";
  return (
    <SectionCard title="Multi-Account Pooling" subtitle={en ? "Multiple API keys per connection" : "Banyak API key per koneksi"} color="var(--purple)">
      <P>{en ? "Add multiple keys per connection. Lintasan rotates between them. More keys = higher rate limits." : "Tambah banyak key per koneksi. Lintasan rotasi di antaranya. Lebih banyak key = rate limit lebih tinggi."}</P>
      <H3>{en ? "Add Keys (from Dashboard)" : "Tambah Key (dari Dashboard)"}</H3>
      <Step number={1} title={en ? "Go to Connections" : "Buka Connections"}>{en ? "Find the connection you want." : "Cari koneksi yang kamu mau."}</Step>
      <Step number={2} title={en ? "Expand it" : "Expand"}>{en ? "Click the connection card." : "Klik kartu koneksi."}</Step>
      <Step number={3} title={en ? "Add Key" : "Tambah Key"}>{en ? "Click Add Key, paste API key, label it, Save." : "Klik Add Key, paste API key, beri label, Save."}</Step>
      <H3>{en ? "How rotation works" : "Cara rotasi bekerja"}</H3>
      <FeatureRow label="Round-robin" desc={en ? "Rotates across all keys for that connection" : "Rotasi ke semua key untuk koneksi tersebut"} />
      <FeatureRow label="Auto-cooldown" desc={en ? "429 response puts key in 60s cooldown" : "Respons 429 membuat key masuk cooldown 60 detik"} />
      <FeatureRow label="Self-healing" desc={en ? "After cooldown, key is back in rotation" : "Setelah cooldown, key kembali ke rotasi"} />
      <Tip>{en ? "Especially useful for CommandCode and free-tier providers with strict per-key rate limits." : "Sangat berguna untuk CommandCode dan provider gratis dengan rate limit ketat per key."}</Tip>
    </SectionCard>
  );
}

function QuotaSection({ lang }) {
  const en = lang === "en";
  return (
    <SectionCard title={en ? "Quota and Limits" : "Kuota dan Batas"} subtitle={en ? "Token budgets with automatic fallback" : "Budget token dengan fallback otomatis"} color="var(--warning)">
      <H3>{en ? "Set Limits" : "Atur Batas"}</H3>
      <P>{en ? "Settings page, Quota section. Set daily and/or monthly token limits per connection." : "Halaman Settings, bagian Quota. Atur batas token harian dan/atau bulanan per koneksi."}</P>
      <H3>{en ? "How it works" : "Cara kerja"}</H3>
      <FeatureRow label={en ? "Auto-counting" : "Hitung otomatis"} desc={en ? "Every request input + output tokens counted per connection" : "Setiap input + output token request dihitung per koneksi"} />
      <FeatureRow label="Auto-reset" desc={en ? "Daily at midnight UTC, monthly on 1st" : "Harian tengah malam UTC, bulanan tanggal 1"} />
      <FeatureRow label="Auto-fallback" desc={en ? "Exhausted connection skipped in routing" : "Koneksi yang habis dilewati saat routing"} />
      <FeatureRow label="Combo-aware" desc={en ? "Combos fall through to next model when quota hit" : "Combo lanjut ke model berikutnya saat kuota habis"} />
      <H3>{en ? "Check Usage" : "Cek Pemakaian"}</H3>
      <P>{en ? "Usage page shows token breakdown by provider, daily/monthly charts, and which connections are near limits." : "Halaman Usage menampilkan rincian token per provider, grafik harian/bulanan, dan koneksi mana yang mendekati batas."}</P>
      <Tip>{en ? "Set limits slightly below actual budget for headroom. Combo + quotas = automatic cost control." : "Set batas sedikit di bawah budget aktual untuk ruang gerak. Combo + kuota = kontrol biaya otomatis."}</Tip>
    </SectionCard>
  );
}

function FallbackSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title="Fallback Chains" subtitle={en ? "Automatic failover when models or connections fail" : "Failover otomatis saat model atau koneksi gagal"} color="var(--error)">
        <P>{en ? "Configure ordered fallback sequences so requests automatically retry on alternative models or connections when the primary fails." : "Konfigurasi urutan fallback agar request otomatis retry ke model atau koneksi alternatif saat primary gagal."}</P>
      </SectionCard>
      <div style={{...card("var(--error)"), padding: "20px 24px"}}>
        <H3>{en ? "Model Fallback" : "Fallback Model"}</H3>
        <P>{en ? "If model A fails → try model B → try model C. Triggers: timeout, 5xx errors, 429 rate limit, circuit breaker open." : "Jika model A gagal → coba model B → coba model C. Trigger: timeout, error 5xx, 429 rate limit, circuit breaker open."}</P>
        <H3>{en ? "Connection Fallback" : "Fallback Koneksi"}</H3>
        <P>{en ? "If connection 1 fails → try connection 2 → try connection 3. Works independently from model fallback." : "Jika koneksi 1 gagal → coba koneksi 2 → coba koneksi 3. Bekerja independen dari fallback model."}</P>
        <H3>{en ? "Configuration" : "Konfigurasi"}</H3>
        <P>{en ? "Go to Dashboard → Fallback page. Add chains with comma-separated model/connection names in priority order." : "Buka Dashboard → halaman Fallback. Tambah chain dengan nama model/koneksi dipisah koma sesuai urutan prioritas."}</P>
        <H3>API</H3>
        <CodeBlock code={"POST /api/fallback\n  { type: \"model\", id: \"gpt-4o\", fallbacks: [\"gpt-4o-mini\", \"claude-3-haiku\"] }\n\nGET /api/fallback\n  Returns all configured chains\n\nGET /api/fallback?stats=true&hours=24\n  Returns fallback usage metrics"} />
      </div>
    </>
  );
}

function PluginsSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title="Plugin System" subtitle={en ? "Extend Lintasan with custom middleware" : "Extend Lintasan dengan middleware custom"} color="var(--purple)">
        <P>{en ? "Plugins hook into the request lifecycle: before request, after response, on error, and on stream chunks." : "Plugin hook ke lifecycle request: sebelum request, setelah response, saat error, dan saat stream chunk."}</P>
      </SectionCard>
      <div style={{...card("var(--purple)"), padding: "20px 24px"}}>
        <H3>{en ? "How It Works" : "Cara Kerja"}</H3>
        <P>{en ? "Place .js files in the /plugins/ directory. Each exports a plugin object with hooks. Plugins are hot-loadable — no restart needed." : "Taruh file .js di direktori /plugins/. Masing-masing export object plugin dengan hooks. Plugin hot-loadable — tidak perlu restart."}</P>
        <H3>{en ? "Plugin Structure" : "Struktur Plugin"}</H3>
        <CodeBlock code={"// plugins/my-plugin.js\nexport default {\n  name: \"my-plugin\",\n  version: \"1.0.0\",\n  description: \"What it does\",\n  priority: 10, // lower = runs first\n  hooks: {\n    beforeRequest(ctx) {\n      // modify ctx.messages, ctx.model, etc.\n    },\n    afterRequest(ctx, response) {\n      // log, transform response\n    },\n    onError(ctx, error) {\n      // handle errors\n    }\n  }\n};"} />
        <H3>{en ? "Built-in Plugins" : "Plugin Bawaan"}</H3>
        <FeatureRow label="request-logger" desc={en ? "Logs all requests with timing info" : "Log semua request dengan info timing"} />
        <FeatureRow label="content-filter" desc={en ? "Blocks requests with banned words (configurable)" : "Blokir request dengan kata terlarang (configurable)"} />
        <H3>{en ? "Management" : "Manajemen"}</H3>
        <P>{en ? "Enable/disable plugins from Dashboard → Plugins page, or via API:" : "Enable/disable plugin dari Dashboard → halaman Plugins, atau via API:"}</P>
        <CodeBlock code={"GET  /api/plugins          # list all plugins\nPOST /api/plugins          # { name, enabled: true/false }"} />
      </div>
    </>
  );
}

function TeamsSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title="Teams & Users" subtitle={en ? "Multi-user access control and team management" : "Kontrol akses multi-user dan manajemen tim"} color="var(--info)">
        <P>{en ? "Lintasan supports multiple users with role-based access and team organization." : "Lintasan mendukung multi-user dengan akses berbasis role dan organisasi tim."}</P>
      </SectionCard>
      <div style={{...card("var(--info)"), padding: "20px 24px"}}>
        <H3>{en ? "User Roles" : "Role User"}</H3>
        <FeatureRow label="Admin" desc={en ? "Full access — manage users, teams, settings, connections" : "Akses penuh — kelola user, tim, settings, koneksi"} />
        <FeatureRow label="Editor" desc={en ? "Manage connections, keys, combos — cannot manage users" : "Kelola koneksi, key, combo — tidak bisa kelola user"} />
        <FeatureRow label="Viewer" desc={en ? "Read-only dashboard access" : "Akses dashboard read-only"} />
        <H3>Teams</H3>
        <P>{en ? "Group users into teams. Team API keys are shared among all members. Usage is tracked per-team for billing visibility." : "Kelompokkan user ke dalam tim. API key tim dibagi ke semua anggota. Usage dilacak per-tim untuk visibilitas billing."}</P>
        <H3>{en ? "Team Roles" : "Role Tim"}</H3>
        <FeatureRow label="Owner" desc={en ? "Can manage team members and settings" : "Bisa kelola anggota dan settings tim"} />
        <FeatureRow label="Member" desc={en ? "Can use team API keys" : "Bisa pakai API key tim"} />
        <H3>API</H3>
        <CodeBlock code={"GET/POST         /api/users           # list/create users\nPUT/DELETE       /api/users/[id]      # update/delete user\n\nGET/POST         /api/teams           # list/create teams\nGET/PUT/DELETE   /api/teams/[id]      # team details\nGET/POST/DELETE  /api/teams/[id]/members  # manage members"} />
      </div>
    </>
  );
}

function WebhooksSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title="Webhook Notifications" subtitle={en ? "Get alerted when things go wrong" : "Dapat notifikasi saat ada masalah"} color="var(--warning)">
        <P>{en ? "Register webhook URLs to receive real-time notifications about system events." : "Daftarkan URL webhook untuk menerima notifikasi real-time tentang event sistem."}</P>
      </SectionCard>
      <div style={{...card("var(--warning)"), padding: "20px 24px"}}>
        <H3>{en ? "Event Types" : "Tipe Event"}</H3>
        <FeatureRow label="budget_warning" desc={en ? "80% of token budget used" : "80% budget token terpakai"} />
        <FeatureRow label="budget_exhausted" desc={en ? "Token budget fully consumed" : "Budget token habis"} />
        <FeatureRow label="provider_down" desc={en ? "Circuit breaker opened for a provider" : "Circuit breaker terbuka untuk provider"} />
        <FeatureRow label="provider_recovered" desc={en ? "Provider back online" : "Provider kembali online"} />
        <FeatureRow label="anomaly_detected" desc={en ? "Sudden spike in errors or latency" : "Lonjakan tiba-tiba di error atau latency"} />
        <FeatureRow label="high_latency" desc={en ? "p95 latency exceeds threshold" : "Latency p95 melebihi threshold"} />
        <FeatureRow label="cache_hit_rate_low" desc={en ? "Cache effectiveness dropping" : "Efektivitas cache menurun"} />
        <H3>{en ? "Security" : "Keamanan"}</H3>
        <P>{en ? "Each webhook has a secret. Payloads are signed with HMAC-SHA256 — verify the X-Signature header to confirm authenticity." : "Setiap webhook punya secret. Payload ditandatangani dengan HMAC-SHA256 — verifikasi header X-Signature untuk konfirmasi keaslian."}</P>
        <H3>{en ? "Retry Logic" : "Logika Retry"}</H3>
        <P>{en ? "Failed deliveries retry 3 times with exponential backoff (1s, 2s, 4s)." : "Pengiriman gagal retry 3 kali dengan exponential backoff (1s, 2s, 4s)."}</P>
        <H3>API</H3>
        <CodeBlock code={"GET    /api/webhooks              # list + history\nPOST   /api/webhooks              # register new\nPUT    /api/webhooks              # update\nDELETE /api/webhooks              # remove\nPOST   /api/webhooks?action=test&id=ID  # send test"} />
      </div>
    </>
  );
}

function BackupSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title="Backup & Export" subtitle={en ? "Protect your data and export analytics" : "Lindungi data dan export analytics"} color="var(--success)">
        <P>{en ? "Create SQLite backups, export configuration, and download analytics data." : "Buat backup SQLite, export konfigurasi, dan download data analytics."}</P>
      </SectionCard>
      <div style={{...card("var(--success)"), padding: "20px 24px"}}>
        <H3>{en ? "Backup" : "Backup"}</H3>
        <P>{en ? "One-click backup copies the entire SQLite database. Stored in data/backups/ with timestamps. Auto-backup can be scheduled via settings." : "Backup satu klik menyalin seluruh database SQLite. Disimpan di data/backups/ dengan timestamp. Auto-backup bisa dijadwalkan via settings."}</P>
        <H3>Export</H3>
        <FeatureRow label="Config JSON" desc={en ? "Settings, connections, combos, aliases, fallback chains" : "Settings, koneksi, combo, alias, fallback chain"} />
        <FeatureRow label="Analytics CSV" desc={en ? "Request logs with date range filter" : "Log request dengan filter rentang tanggal"} />
        <FeatureRow label="Full Export" desc={en ? "Everything in one JSON file" : "Semua dalam satu file JSON"} />
        <H3>{en ? "Restore" : "Restore"}</H3>
        <P>{en ? "Restore from any backup file. The current database is replaced — make sure to backup first!" : "Restore dari file backup manapun. Database saat ini diganti — pastikan backup dulu!"}</P>
        <H3>API</H3>
        <CodeBlock code={"GET  /api/export?type=config|analytics|keys|full\nGET  /api/export?type=analytics&format=csv&from=DATE&to=DATE\nPOST /api/export  # import config from JSON body\n\nGET  /api/backup          # list backups\nPOST /api/backup          # create backup\nPOST /api/backup?action=restore&file=FILENAME\nDELETE /api/backup?file=FILENAME"} />
      </div>
    </>
  );
}

function EndpointsSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title={en ? "Extra OpenAI-Compatible Endpoints" : "Endpoint OpenAI-Compatible Tambahan"} subtitle={en ? "Beyond chat completions" : "Selain chat completions"} color="var(--primary)">
        <P>{en ? "Lintasan now proxies embeddings, image generation, audio transcription, and text-to-speech — all through the same unified endpoint." : "Lintasan sekarang proxy embeddings, image generation, audio transcription, dan text-to-speech — semua lewat endpoint yang sama."}</P>
      </SectionCard>
      <div style={{...card("var(--primary)"), padding: "20px 24px"}}>
        <H3>Embeddings</H3>
        <CodeBlock code={"POST /api/v1/embeddings\n  { model: \"text-embedding-3-small\", input: \"Hello world\" }\n  → Returns: { data: [{ embedding: [...], index: 0 }] }"} />
        <H3>{en ? "Image Generation" : "Generasi Gambar"}</H3>
        <CodeBlock code={"POST /api/v1/images/generations\n  { model: \"dall-e-3\", prompt: \"A cat in space\", size: \"1024x1024\" }\n  → Returns: { data: [{ url: \"...\" }] }"} />
        <H3>{en ? "Audio Transcription" : "Transkripsi Audio"}</H3>
        <CodeBlock code={"POST /api/v1/audio/transcriptions\n  Content-Type: multipart/form-data\n  Fields: file (audio), model, language\n  → Returns: { text: \"...\" }"} />
        <H3>Text-to-Speech</H3>
        <CodeBlock code={"POST /api/v1/audio/speech\n  { model: \"tts-1\", input: \"Hello!\", voice: \"alloy\" }\n  → Returns: audio binary stream"} />
        <Tip>{en ? "All endpoints use the same auth (Bearer API key), circuit breaker, and connection fallback as chat completions." : "Semua endpoint pakai auth yang sama (Bearer API key), circuit breaker, dan connection fallback seperti chat completions."}</Tip>
      </div>
    </>
  );
}

function ToolsSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title={en ? "Tool Integration" : "Integrasi Tool"} subtitle={en ? "Works with any OpenAI-compatible tool" : "Bekerja dengan tool apapun yang OpenAI-compatible"} color="var(--primary)">
        <P>{en ? "Just change the base URL and API key in your tool." : "Cukup ganti base URL dan API key di tool kamu."}</P>
      </SectionCard>
      <div style={{...card("var(--primary)"), padding: "20px 24px"}}>
        <H3>Hermes Agent</H3>
        <CodeBlock code={"# ~/.hermes/config.yaml\nproviders:\n  lintasan:\n    type: openai\n    base_url: http://100.99.2.14:20180/api/v1\n    api_key: YOUR_MASTER_KEY\n    models:\n      - coding-combo\n      - deepseek/deepseek-v4-pro"} />
        <H3>Claude Code / Codex</H3>
        <CodeBlock code={"export OPENAI_API_BASE=http://100.99.2.14:20180/api/v1\nexport OPENAI_API_KEY=YOUR_MASTER_KEY"} />
        <H3>Cursor IDE</H3>
        <P>{en ? "Settings, Models, OpenAI API Key, paste master key. Base URL: http://100.99.2.14:20180/api/v1" : "Settings, Models, OpenAI API Key, paste master key. Base URL: http://100.99.2.14:20180/api/v1"}</P>
        <H3>Python SDK</H3>
        <CodeBlock code={"from openai import OpenAI\n\nclient = OpenAI(\n    base_url=\"http://100.99.2.14:20180/api/v1\",\n    api_key=\"YOUR_MASTER_KEY\"\n)\n\nresponse = client.chat.completions.create(\n    model=\"coding-combo\",\n    messages=[{\"role\": \"user\", \"content\": \"Hello!\"}]\n)"} />
        <H3>{en ? "Any other tool" : "Tool lainnya"}</H3>
        <CodeBlock code={"Base URL: http://100.99.2.14:20180/api/v1\nAPI Key:  YOUR_MASTER_KEY\nModel:    " + (en ? "any model or combo name" : "model apapun atau nama combo")} />
      </div>
    </>
  );
}

function ApiSection({ lang }) {
  const en = lang === "en";
  return (
    <>
      <SectionCard title="API Reference" subtitle={en ? "For advanced users and automation" : "Untuk pengguna lanjutan dan otomasi"} color="var(--fg-2)">
        <P>{en ? "Most of this is already available from the dashboard UI." : "Sebagian besar sudah tersedia dari dashboard UI."}</P>
      </SectionCard>
      <div style={{...card("var(--fg-2)"), padding: "20px 24px"}}>
        <H3>{en ? "Proxy Endpoints (API key auth)" : "Endpoint Proxy (auth API key)"}</H3>
        <CodeBlock code={"POST /api/v1/chat/completions\n  Headers: Authorization: Bearer ***  Body: { model, messages, stream, temperature, ... }\n\nPOST /api/v1/embeddings\n  Body: { model, input }\n\nPOST /api/v1/images/generations\n  Body: { model, prompt, size, n }\n\nPOST /api/v1/audio/transcriptions\n  multipart/form-data: file, model, language\n\nPOST /api/v1/audio/speech\n  Body: { model, input, voice }\n\nGET /api/v1/models\n  Returns all discovered models + combos"} />
        <H3>{en ? "Management Endpoints (cookie auth)" : "Endpoint Manajemen (auth cookie)"}</H3>
        <CodeBlock code={"POST /api/auth/login        { password }\nPOST /api/auth/logout\nGET  /api/auth/check\n\nGET/POST/PATCH/DELETE /api/connections\nGET/POST             /api/models/sync\nGET/POST/DELETE      /api/combos\nGET/POST             /api/quota\nGET/POST             /api/settings\nGET                  /api/logs\nGET                  /api/usage\nGET/POST/DELETE      /api/keys\nGET/POST/PUT/DELETE  /api/fallback\nGET/POST             /api/plugins\nGET/POST/PUT/DELETE  /api/webhooks\nGET/POST             /api/users\nGET/POST/PUT/DELETE  /api/teams\nGET/POST             /api/export\nGET/POST/DELETE      /api/backup\nGET                  /api/analytics/realtime\nGET                  /api/analytics/stream (SSE)"} />
        <H3>Response Headers</H3>
        <CodeBlock code={"X-Provider: CommandCode     <- which connection handled it\nX-Cache: HIT|MISS|SEMANTIC  <- cache status\nX-Latency: 234             <- upstream latency (ms)\nX-Connection: conn-id      <- connection ID used"} />
        <H3>Error Codes</H3>
        <FeatureRow label="401" desc={en ? "Invalid API key" : "API key tidak valid"} />
        <FeatureRow label="404" desc={en ? "Model not found on any connection" : "Model tidak ditemukan di koneksi manapun"} />
        <FeatureRow label="429" desc={en ? "Rate limited or quota exhausted" : "Rate limit atau kuota habis"} />
        <FeatureRow label="502" desc={en ? "All providers failed (includes list of what was tried)" : "Semua provider gagal (termasuk daftar yang dicoba)"} />
      </div>
    </>
  );
}

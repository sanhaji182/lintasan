/** Help tooltip content for each dashboard page */
export const helpContent: Record<string, string> = {
  '/dashboard':
    '<strong>Overview</strong> — Ringkasan global sistem Anda. Lihat total requests, cache hit rate, active connections, latency rata-rata, dan token usage dalam satu tampilan.',

  '/dashboard/connections':
    '<strong>Accounts</strong> — Kelola koneksi provider AI Anda. Tambah, test, sync, atau hapus koneksi. Setiap koneksi punya format API sendiri (openai, anthropic, google, dll).',

  '/dashboard/providers':
    '<strong>Providers</strong> — 118+ provider presets siap pakai. Cari provider, isi API key, dan langsung tambahkan sebagai koneksi. Termasuk free providers, self-hosted, dan enterprise.',

  '/dashboard/experimental':
    '<strong>Experimental</strong> — Fitur eksperimental yang masih dalam pengembangan. Aktifkan dengan hati-hati — beberapa fitur mungkin belum stabil untuk production.',

  '/dashboard/oauth-ide':
    '<strong>OAuth IDE</strong> — Experimental OAuth flow untuk IDE agents. Mendukung Claude Code, Codex, Copilot, Cline, dan lainnya. LAB — gunakan dengan pemahaman risiko.',

  '/dashboard/discover':
    '<strong>Discover</strong> — Temukan free dan public provider API secara otomatis. Scan provider yang tersedia dan tambahkan langsung ke koneksi Anda.',

  '/dashboard/routing':
    '<strong>Routing</strong> — Atur strategi routing request LLM Anda. Konfigurasi combo (model → provider), load balancer, aliases, dan priority routing.',

  '/dashboard/fallback':
    '<strong>Fallback</strong> — Buat multi-level fallback chain per model atau connection. Jika provider utama gagal, request otomatis dialihkan ke cadangan.',

  '/dashboard/logs':
    '<strong>Logs</strong> — Log request real-time. Filter berdasarkan provider, model, status, atau cari keyword. Lihat latency, token usage, dan error details.',

  '/dashboard/usage':
    '<strong>Usage</strong> — Token usage dan cost breakdown per provider, model, dan periode waktu. Lihat tren pemakaian untuk optimasi biaya.',

  '/dashboard/analytics':
    '<strong>Analytics</strong> — Metrics dashboard dengan latency distribution, throughput, cache efficiency, dan cost savings. Data historis untuk analisis tren.',

  '/dashboard/observability':
    '<strong>Observability</strong> — Exportable /metrics endpoint (Prometheus-compatible) + real-time panels. Pantau health sistem, cache hit/miss, dan performa provider.',

  '/dashboard/memory':
    '<strong>Memory</strong> — Vector memory untuk RAG (Retrieval-Augmented Generation). Simpan, cari, dan kelola memory embeddings. Default: SQLite TF-IDF, opsional: Redis.',

  '/dashboard/keys':
    '<strong>API Keys</strong> — Generate, copy, dan revoke API keys. Setiap key punya usage tracking dan bisa dibatasi per-key (rate limit, budget).',

  '/dashboard/teams':
    '<strong>Teams</strong> — Team-based access control. Buat tim, atur member, dan assign permissions. Cocok untuk organisasi dengan multiple users.',

  '/dashboard/users':
    '<strong>User Management</strong> — Kelola user accounts. Tambah, edit, hapus user. Atur role dan akses ke dashboard.',

  '/dashboard/webhooks':
    '<strong>Webhooks</strong> — Event-driven webhook system. Trigger notifikasi atau automation berdasarkan request completion, error, atau threshold tertentu.',

  '/dashboard/backup':
    '<strong>Backup</strong> — Backup dan restore database SQLite. Export konfigurasi JSON untuk migrasi atau disaster recovery.',

  '/dashboard/settings':
    '<strong>Settings</strong> — Konfigurasi global: theme, default model, rate limits, CORS, cache behavior, log retention, dan observability.',

  '/dashboard/plugins':
    '<strong>Plugins</strong> — Plugin store + management. Install plugin dari store atau generate dengan AI. Extend Lintasan tanpa mengubah core.',

  '/dashboard/playground':
    '<strong>Playground</strong> — Interactive chat console untuk testing API. Coba berbagai model, streaming, dan parameter langsung dari browser.',

  '/dashboard/mcp':
    '<strong>MCP Server</strong> — Model Context Protocol server. JSON-RPC 2.0 + SSE transport. 14+ tools untuk MCP-compatible clients.',

  '/dashboard/savings':
    '<strong>Cost Savings</strong> — Lihat penghematan biaya dari cache hits, token compression, dan routing cerdas. Breakdown per provider dan periode.',

  '/dashboard/translator':
    '<strong>Format Translator</strong> — Terjemahkan request antar format API (OpenAI → Anthropic, Google → OpenAI, dll). 5+ format didukung.',

  '/dashboard/docs':
    '<strong>Docs</strong> — Dokumentasi API built-in. Referensi endpoint, contoh curl, dan panduan cepat.',
};

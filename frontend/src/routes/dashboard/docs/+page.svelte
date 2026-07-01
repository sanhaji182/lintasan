<script lang="ts">
  import {
    BookOpen, ChevronRight, ChevronDown, Search,
    Zap, Code2, Settings, Lightbulb, Copy, Check, Globe,
    Rocket, Wrench, HelpCircle, Server, Terminal, Play, Cpu,
    Plug, RefreshCw, Shield, DollarSign, Globe as WebIcon,
    Database, Undo2, Route, Layers, Box, Puzzle, Gauge,
    FileText
  } from 'lucide-svelte';

  interface DocSubsection {
    id: string;
    id_title: string;
    en_title: string;
    id_content: string;
    en_content: string;
    id_code?: string;
    en_code?: string;
    language?: string;
  }

  interface DocSection {
    id: string;
    id_title: string;
    en_title: string;
    icon: typeof BookOpen;
    subsections: DocSubsection[];
  }

  let lang = $state<'id' | 'en'>('en');
  let activeSection = $state('getting-started');
  let searchQuery = $state('');
  let expandedSections = $state<Set<string>>(new Set(['getting-started']));
  let copiedCode = $state<string | null>(null);

  const docs: DocSection[] = [
    {
      id: 'getting-started',
      id_title: 'Mulai Cepat',
      en_title: 'Getting Started',
      icon: Rocket,
      subsections: [
        {
          id: 'what-is-lintasan',
          id_title: 'Apa itu Lintasan?',
          en_title: 'What is Lintasan?',
          id_content: `Lintasan adalah **LLM gateway** — sebuah server perantara yang berada di antara aplikasi kamu dan berbagai provider AI (OpenAI, Anthropic, DeepSeek, Google Gemini, Groq, dll).

**Kenapa butuh Lintasan?**

Bayangkan kamu pakai 5 provider AI berbeda. Masing-masing punya API key sendiri, endpoint berbeda, format response berbeda, dan dashboard berbeda. Ribet, kan?

Lintasan menyatukan semuanya:
- **Satu endpoint API** → OpenAI-compatible, semua tools support
- **Satu API key** → kamu bikin 1 master key, Lintasan yang atur sisanya
- **Auto-failover** → provider mati? Auto pindah ke lain
- **Caching & compression** → hemat token, hemat biaya
- **Dashboard UI** → pantau semuanya dari 1 tempat

**Cara kerjanya:**

\`\`\`
Kamu → Lintasan (routing) → Provider AI (OpenAI / Claude / Gemini / ...)
\`\`\`

Kamu kirim request ke Lintasan pakai format standard OpenAI. Lintasan memilih provider terbaik, meneruskan request, dan mengembalikan response — semua transparan.`,
          en_content: `Lintasan is an **LLM gateway** — a middleware server that sits between your application and multiple AI providers (OpenAI, Anthropic, DeepSeek, Google Gemini, Groq, etc.).

**Why do you need Lintasan?**

Imagine you use 5 different AI providers. Each has its own API key, different endpoints, different response formats, different dashboards. Messy, right?

Lintasan unifies everything:
- **One API endpoint** → OpenAI-compatible, all tools supported
- **One API key** → you create 1 master key, Lintasan handles the rest
- **Auto-failover** → provider down? Automatically switches to another
- **Caching & compression** → save tokens, save money
- **Dashboard UI** → monitor everything from one place

**How it works:**

\`\`\`
You → Lintasan (routing) → AI Provider (OpenAI / Claude / Gemini / ...)
\`\`\`

You send requests to Lintasan in standard OpenAI format. Lintasan picks the best provider, forwards the request, and returns the response — all transparent.`,
        },
        {
          id: 'installation',
          id_title: 'Cara Install',
          en_title: 'Installation',
          id_content: `Lintasan adalah **1 file binary** (~24MB). Tidak perlu Node.js, Python, atau Docker. Download → run → jalan.

**3 cara install:**`,
          en_content: `Lintasan is a **single binary file** (~24MB). No Node.js, Python, or Docker required. Download → run → done.

**3 ways to install:**`,
          id_code: `# ==========================================
# CARA 1: Download Binary (rekomendasi)
# ==========================================
curl -L -o lintasan \\
  https://github.com/sanhaji182/lintasan-go/releases/latest/download/lintasan
chmod +x lintasan
./lintasan start

# ==========================================
# CARA 2: Docker
# ==========================================
docker run -d \\
  --name lintasan \\
  -p 20180:20180 \\
  -v ./data:/data \\
  ghcr.io/sanhaji182/lintasan-go:latest

# ==========================================
# CARA 3: Build dari source
# ==========================================
git clone https://github.com/sanhaji182/lintasan-go
cd lintasan-go
make build        # frontend SvelteKit + Go binary
./lintasan start`,
          en_code: `# ==========================================
# METHOD 1: Download Binary (recommended)
# ==========================================
curl -L -o lintasan \\
  https://github.com/sanhaji182/lintasan-go/releases/latest/download/lintasan
chmod +x lintasan
./lintasan start

# ==========================================
# METHOD 2: Docker
# ==========================================
docker run -d \\
  --name lintasan \\
  -p 20180:20180 \\
  -v ./data:/data \\
  ghcr.io/sanhaji182/lintasan-go:latest

# ==========================================
# METHOD 3: Build from source
# ==========================================
git clone https://github.com/sanhaji182/lintasan-go
cd lintasan-go
make build        # frontend SvelteKit + Go binary
./lintasan start`,
          language: 'bash',
        },
        {
          id: 'first-login',
          id_title: 'Login Dashboard',
          en_title: 'Dashboard Login',
          id_content: `Setelah server berjalan di http://localhost:20180, buka dashboard di browser:

**Default login:**
|- Username: \`admin\`
|- Password: (generated randomly on first run — check the terminal output for \`generated admin password: ...\`)

⚠️ **Ganti password segera** setelah login pertama! Dashboard → Users → klik user → ganti password.

Dashboard punya **17+ halaman** untuk mengelola semuanya — dari connections, models, routing, sampai logs dan analytics.`,
          en_content: `Once the server is running at http://localhost:20180, open the dashboard in your browser:

**Default login:**
|- Username: \`admin\`
|- Password: (generated randomly on first run — check the terminal output for \`generated admin password: ...\`)

⚠️ **Change the password immediately** after first login! Dashboard → Users → click user → change password.

The dashboard has **17+ pages** to manage everything — from connections, models, routing, to logs and analytics.`,
        },
        {
          id: 'add-provider',
          id_title: 'Menambah Provider',
          en_title: 'Adding a Provider',
          id_content: `Ada **3 cara** menambah provider ke Lintasan:

**Cara 1: Import dari Curl (paling gampang! 🔥)**
Copy curl command dari docs provider → paste → selesai. Lintasan auto-extract URL, API key, dan discover models.

**Cara 2: Pilih dari Preset**
Dashboard → Connections → pilih dari daftar 100+ provider yang sudah disiapkan.

**Cara 3: Manual**
Isi form manual: nama, base URL, API key, format.`,

          en_content: `There are **3 ways** to add a provider to Lintasan:

**Method 1: Import from Curl (easiest! 🔥)**
Copy curl command from provider docs → paste → done. Lintasan auto-extracts URL, API key, and discovers models.

**Method 2: Pick from Presets**
Dashboard → Connections → pick from 100+ pre-configured providers.

**Method 3: Manual**
Fill the form manually: name, base URL, API key, format.`,
          id_code: `# IMPORT DARI CURL (copy-paste dari docs provider)
# Dashboard → Connections → Import Curl → paste:
curl https://api.openai.com/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}'

# Lintasan auto:
# 1. Extract base URL + API key
# 2. Infer provider name (OpenAI)
# 3. Discover models (/v1/models)`,
          en_code: `# IMPORT FROM CURL (copy-paste from provider docs)
# Dashboard → Connections → Import Curl → paste:
curl https://api.openai.com/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}'

# Lintasan auto:
# 1. Extracts base URL + API key
# 2. Infers provider name (OpenAI)
# 3. Discovers models (/v1/models)`,
          language: 'bash',
        },
        {
          id: 'first-request',
          id_title: 'Request Pertama',
          en_title: 'Your First Request',
          id_content: `Setelah provider terhubung, kirim request chat:

**API key:** pakai API key yang kamu set di dashboard (Users → buka user → copy API key). BUKAN API key provider.`,

          en_content: `Once a provider is connected, send your first chat request:

**API key:** use the API key you set in the dashboard (Users → open user → copy API key). NOT the provider's API key.`,
          id_code: `# Kirim chat via Lintasan
curl http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Response (OpenAI-compatible):
# {
#   "choices": [{ "message": { "content": "..." } }],
#   "usage": { "prompt_tokens": ..., "completion_tokens": ... }
# }`,
          en_code: `# Send chat via Lintasan
curl http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Response (OpenAI-compatible):
# {
#   "choices": [{ "message": { "content": "..." } }],
#   "usage": { "prompt_tokens": ..., "completion_tokens": ... }
# }`,
          language: 'bash',
        },
        {
          id: 'first-streaming',
          id_title: 'Streaming Request',
          en_title: 'Streaming Request',
          id_content: `Streaming pakai Server-Sent Events (SSE). Tambahkan \`"stream": true\` ke request body.`,

          en_content: `Streaming uses Server-Sent Events (SSE). Add \`"stream": true\` to the request body.`,
          id_code: `# Streaming chat
curl -N http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Write a poem"}],
    "stream": true
  }'

# Response: data: {...chunk...}\\n\\ndata: {...chunk...}\\n\\n`,
          en_code: `# Streaming chat
curl -N http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Write a poem"}],
    "stream": true
  }'

# Response: data: {...chunk...}\\n\\ndata: {...chunk...}\\n\\n`,
          language: 'bash',
        },
      ],
    },
    {
      id: 'dashboard-guide',
      id_title: 'Panduan Dashboard',
      en_title: 'Dashboard Guide',
      icon: Play,
      subsections: [
        {
          id: 'dashboard-overview',
          id_title: 'Tur Dashboard',
          en_title: 'Dashboard Tour',
          id_content: `Dashboard Lintasan punya **17 halaman** untuk manajemen penuh:

| Halaman | Fungsi |
|---------|--------|
| **Overview** | Ringkasan: request total, model terpakai, cache hit rate |
| **Connections** | Kelola provider: tambah, edit, test, sync models |
| **Playground** | Test chat langsung dari dashboard |
| **Routing** | Atur fallback chains, routing rules, combos |
| **Logs** | Request log dengan filter & search |
| **Analytics** | Token savings, cache performance |
| **Usage** | Breakdown per provider & model |
| **Settings** | Konfigurasi global (thinking mode, quality, dll) |
| **Users** | Manajemen user & API key |
| **Keys** | API key management |
| **Plugins** | Plugin store & management |
| **Webhooks** | Event-driven webhook subscriptions |
| **MCP** | Model Context Protocol server |
| **Translator** | Format translation testing |
| **Memory** | Vector memory search & manage |
| **Discover** | Auto-discover free providers |
| **Docs** | Dokumentasi ini 😄 |
| **Backup** | Export/import data |`,

          en_content: `The Lintasan dashboard has **17 pages** for full management:

| Page | Function |
|------|----------|
| **Overview** | Summary: total requests, models used, cache hit rate |
| **Connections** | Manage providers: add, edit, test, sync models |
| **Playground** | Live chat testing from the dashboard |
| **Routing** | Set up fallback chains, routing rules, combos |
| **Logs** | Filterable & searchable request log |
| **Analytics** | Token savings, cache performance |
| **Usage** | Per-provider & model breakdown |
| **Settings** | Global configuration (thinking mode, quality, etc.) |
| **Users** | User & API key management |
| **Keys** | API key management |
| **Plugins** | Plugin store & management |
| **Webhooks** | Event-driven webhook subscriptions |
| **MCP** | Model Context Protocol server |
| **Translator** | Format translation testing |
| **Memory** | Vector memory search & manage |
| **Discover** | Auto-discover free providers |
| **Docs** | This documentation 😄 |
| **Backup** | Export/import data |`,
        },
        {
          id: 'curl-import-guide',
          id_title: 'Fitur Curl Import',
          en_title: 'Curl Import Feature',
          id_content: `Fitur **Curl Import** adalah cara paling cepat untuk menambah provider baru. Tidak perlu isi form manual — cukup copy-paste curl command dari dokumentasi provider.

**Cara pakai:**
1. Buka halaman **Dashboard → Connections**
2. Klik tombol **"Import Curl"** di kanan atas
3. Paste curl command dari docs provider (contoh: dari docs OpenAI, Anthropic, DeepSeek, dll)
4. Klik **"Import & Discover"**
5. Lintasan otomatis:
   - Mengekstrak base URL dan chat path
   - Mendeteksi API key dari header Authorization
   - Membuat nama dari domain (api.openai.com → "OpenAI")
   - Auto-discover semua model yang tersedia via /v1/models

**Contoh curl yang didukung:**
\`\`\`
curl https://api.example.com/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4","messages":[...]}'
\`\`\`

Lintasan membaca: URL, headers (-H), body (-d), dan method (-X).`,
          en_content: `The **Curl Import** feature is the fastest way to add a new provider. No manual form filling — just copy-paste a curl command from provider docs.

**How to use:**
1. Go to **Dashboard → Connections**
2. Click the **"Import Curl"** button (top right)
3. Paste a curl command from any provider's docs (e.g., OpenAI, Anthropic, DeepSeek docs)
4. Click **"Import & Discover"**
5. Lintasan automatically:
   - Extracts base URL and chat path
   - Detects API key from Authorization header
   - Creates a name from domain (api.openai.com → "OpenAI")
   - Auto-discovers all available models via /v1/models

**Supported curl format:**
\`\`\`
curl https://api.example.com/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4","messages":[...]}'
\`\`\`

Lintasan reads: URL, headers (-H), body (-d), and method (-X).`,
        },
        {
          id: 'auth-flow',
          id_title: 'Autentikasi & API Key',
          en_title: 'Authentication & API Keys',
          id_content: `Lintasan pakai **JWT (JSON Web Token)** untuk dashboard dan **API Key for gateway access.

**Dashboard Auth:**
- Login pakai username + password → dapat JWT token
- Token di-attach otomatis oleh dashboard frontend
- Untuk API dari luar: \`Authorization: Bearer <JWT>\`

**Proxy API Key (untuk OpenAI-compatible endpoint):**
- Buka Dashboard → Users → pilih user → copy **API Key**
- Kirim via: \`Authorization: Bearer <API_KEY>\`
- Atau: \`api-key: <API_KEY>\` header

**Security:**
- Semua koneksi ke provider disimpan di SQLite (terenkripsi AES-256)
- API key tidak pernah di-expose ke client
- Rate limiting default: 60 request/menit per key`,
          en_content: `Lintasan uses **JWT (JSON Web Token)** for dashboard access and **API Key for gateway access.

**Dashboard Auth:**
- Login with username + password → get JWT token
- Token is auto-attached by the dashboard frontend
- For external API calls: \`Authorization: Bearer <JWT>\`

**Proxy API Key (for OpenAI-compatible endpoints):**
- Go to Dashboard → Users → select user → copy **API Key**
- Send via: \`Authorization: Bearer <API_KEY>\`
- Or: \`api-key: <API_KEY>\` header

**Security:**
- All provider connections stored in SQLite (AES-256 encrypted)
- API keys are never exposed to clients
- Rate limiting default: 60 requests/minute per key`,
          id_code: `# Dashboard auth (JWT)
# Password is generated on first run — check terminal for "generated admin password: ..."
curl -X POST http://localhost:20180/api/auth/login \\
  -H "Content-Type: application/json" \\
  -d '{"username":"admin","password":"<your-generated-password>"}'

# Response: { "token": "eyJhbG...", "user": {...} }

# Proxy request (pakai API Key dari dashboard)
curl http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}]}'`,
          en_code: `# Dashboard auth (JWT)
# Password is generated on first run — check terminal for "generated admin password: ..."
curl -X POST http://localhost:20180/api/auth/login \\
  -H "Content-Type: application/json" \\
  -d '{"username":"admin","password":"<your-generated-password>"}'

# Response: { "token": "eyJhbG...", "user": {...} }

# Proxy request (use API Key from dashboard)
curl http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}]}'`,
          language: 'bash',
        },
      ],
    },
    {
      id: 'api-reference',
      id_title: 'Referensi API',
      en_title: 'API Reference',
      icon: Code2,
      subsections: [
        {
          id: 'chat-completions',
          id_title: 'Chat Completions',
          en_title: 'Chat Completions',
          id_content: `Endpoint utama untuk chat. **100% kompatibel dengan OpenAI API**.

**Endpoint:** \`POST /v1/chat/completions\`

**Parameters:**
- \`model\` (string, wajib) — Model ID atau combo name
- \`messages\` (array, wajib) — Array pesan (role: system/user/assistant)
- \`temperature\` (number, 0-2) — Kreativitas (0 = deterministik)
- \`max_tokens\` (integer) — Batas output token
- \`stream\` (boolean) — Streaming SSE
- \`top_p\` (number, 0-1) — Nucleus sampling
- \`frequency_penalty\` (number) — Hukuman pengulangan kata
- \`presence_penalty\` (number) — Hukuman topik baru

**Response (non-streaming):**
\`\`\`json
{"choices":[{"message":{"role":"assistant","content":"..."}}],"usage":{...}}
\`\`\`

**Response (streaming):**
\`\`\`json
data: {"choices":[{"delta":{"content":"..."}}]}
data: {"choices":[{"delta":{"content":"..."}}]}
data: [DONE]
\`\`\``,
          en_content: `The main chat endpoint. **100% compatible with the OpenAI API spec**.

**Endpoint:** \`POST /v1/chat/completions\`

**Parameters:**
- \`model\` (string, required) — Model ID or combo name
- \`messages\` (array, required) — Array of messages (role: system/user/assistant)
- \`temperature\` (number, 0-2) — Creativity (0 = deterministic)
- \`max_tokens\` (integer) — Output token limit
- \`stream\` (boolean) — SSE streaming
- \`top_p\` (number, 0-1) — Nucleus sampling
- \`frequency_penalty\` (number) — Word repetition penalty
- \`presence_penalty\` (number) — New topic penalty

**Response (non-streaming):**
\`\`\`json
{"choices":[{"message":{"role":"assistant","content":"..."}}],"usage":{...}}
\`\`\`

**Response (streaming):**
\`\`\`json
data: {"choices":[{"delta":{"content":"..."}}]}
data: {"choices":[{"delta":{"content":"..."}}]}
data: [DONE]
\`\`\``,
        },
        {
          id: 'embeddings',
          id_title: 'Embeddings',
          en_title: 'Embeddings',
          id_content: `Generate vector embeddings untuk semantic search, clustering, dan RAG.

**Endpoint:** \`POST /v1/embeddings\`

**Parameters:**
- \`model\` (string) — Model embedding
- \`input\` (string | array) — Teks yang di-embed
- \`encoding_format\` (string) — \`"float"\` atau \`"base64"\`

**Contoh response:**
\`\`\`json
{"data":[{"embedding":[0.12, -0.45, 0.89, ...]}],"usage":{"prompt_tokens":5}}
\`\`\``,
          en_content: `Generate vector embeddings for semantic search, clustering, and RAG.

**Endpoint:** \`POST /v1/embeddings\`

**Parameters:**
- \`model\` (string) — Embedding model
- \`input\` (string | array) — Text to embed
- \`encoding_format\` (string) — \`"float"\` or \`"base64"\`

**Example response:**
\`\`\`json
{"data":[{"embedding":[0.12, -0.45, 0.89, ...]}],"usage":{"prompt_tokens":5}}
\`\`\``,
          id_code: `curl http://localhost:20180/v1/embeddings \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"text-embedding-3-small","input":"Hello world"}'`,
          en_code: `curl http://localhost:20180/v1/embeddings \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"text-embedding-3-small","input":"Hello world"}'`,
          language: 'bash',
        },
        {
          id: 'endpoints-table',
          id_title: 'Semua Endpoint API',
          en_title: 'All API Endpoints',
          id_content: `Semua endpoint OpenAI-compatible yang didukung Lintasan:

| Endpoint | Method | Fungsi |
|----------|--------|--------|
| \`/v1/chat/completions\` | POST | Chat (streaming & non-streaming) |
| \`/v1/embeddings\` | POST | Vector embeddings |
| \`/v1/images/generations\` | POST | Image generation (DALL-E / SD) |
| \`/v1/audio/speech\` | POST | Text-to-Speech |
| \`/v1/audio/transcriptions\` | POST | Speech-to-Text |
| \`/v1/models\` | GET | List semua model |
| \`/v1/web/search\` | POST | Web search terintegrasi |
| \`/v1/memory\` | POST | Store ke vector memory |
| \`/v1/memory/search\` | GET | Search vector memory |
| \`/v1/memory/stats\` | GET | Memory statistics |
| \`/v1/memory/{key}\` | DELETE | Delete memory entry |
| \`/health\` | GET | Health check |`,
          en_content: `All OpenAI-compatible endpoints supported by Lintasan:

| Endpoint | Method | Function |
|----------|--------|----------|
| \`/v1/chat/completions\` | POST | Chat (streaming & non-streaming) |
| \`/v1/embeddings\` | POST | Vector embeddings |
| \`/v1/images/generations\` | POST | Image generation (DALL-E / SD) |
| \`/v1/audio/speech\` | POST | Text-to-Speech |
| \`/v1/audio/transcriptions\` | POST | Speech-to-Text |
| \`/v1/models\` | GET | List all models |
| \`/v1/web/search\` | POST | Integrated web search |
| \`/v1/memory\` | POST | Store to vector memory |
| \`/v1/memory/search\` | GET | Search vector memory |
| \`/v1/memory/stats\` | GET | Memory statistics |
| \`/v1/memory/{key}\` | DELETE | Delete memory entry |
| \`/health\` | GET | Health check |`,
        },
      ],
    },
    {
      id: 'features',
      id_title: 'Fitur & Optimasi',
      en_title: 'Features & Optimization',
      icon: Settings,
      subsections: [
        {
          id: 'smart-routing',
          id_title: 'Smart Routing',
          en_title: 'Smart Routing',
          id_content: `Lintasan menggunakan **multi-stage routing** untuk menentukan provider mana yang menerima setiap request:

**1. Direct Connection Targeting**
Header \`X-Connection: <id>\` untuk bypass routing — langsung ke provider spesifik.

**2. Model-Provider Matching**
Lintasan mencocokkan \`model\` di request dengan model yang tersedia di setiap provider.

**3. Capability Filtering**
Provider yang tidak support fitur yang diminta (streaming, function calling, vision) di-skip.

**4. Priority + Load Balancing**
Provider prioritas tertinggi diutamakan. Bobot merata di antara provider setara.

**5. Cost Optimization**
Provider termurah untuk model yang sama diprioritaskan (bisa di-toggle).

**6. Automatic Fallback**
Jika provider gagal (timeout, error) → auto coba provider berikutnya. Tanpa konfigurasi tambahan.

**7. Circuit Breaker**
Provider yang gagal 3x berturut-turut auto-disabled selama 30 detik.`,
          en_content: `Lintasan uses **multi-stage routing** to decide which provider handles each request:

**1. Direct Connection Targeting**
Header \`X-Connection: <id>\` to bypass routing — directly to a specific provider.

**2. Model-Provider Matching**
Lintasan matches the request \`model\` field with models available at each provider.

**3. Capability Filtering**
Providers that don't support requested features (streaming, function calling, vision) are skipped.

**4. Priority + Load Balancing**
Highest priority providers first. Even distribution among equal-priority providers.

**5. Cost Optimization**
Cheapest provider for the same model is preferred (toggleable).

**6. Automatic Fallback**
If a provider fails (timeout, error) → auto-try next provider. No extra config needed.

**7. Circuit Breaker**
Providers that fail 3x consecutively are auto-disabled for 30 seconds.`,
        },
        {
          id: 'fallback-routing',
          id_title: 'Fallback & Routing Rules',
          en_title: 'Fallback & Routing Rules',
          id_content: `**Fallback Chains** adalah daftar model berurutan: kalau model pertama gagal, coba berikutnya, dan seterusnya.

**Contoh:** \`gpt-4o → claude-sonnet-4 → gemini-2.5-pro\`

**Routing Rules** lebih advanced — kamu bisa atur:
- Berdasarkan waktu (jam sibuk pakai provider murah)
- Berdasarkan task (coding pakai Claude, general pakai GPT)
- Berdasarkan budget (kalau >$10, switch ke model murah)`,
          en_content: `**Fallback Chains** are ordered model lists: if the first model fails, try the next, and so on.

**Example:** \`gpt-4o → claude-sonnet-4 → gemini-2.5-pro\`

**Routing Rules** are more advanced — you can set:
- Time-based (peak hours use cheaper provider)
- Task-based (coding goes to Claude, general to GPT)
- Budget-based (if >$10, switch to cheaper model)`,
          id_code: `# Buat fallback chain
curl -X POST http://localhost:20180/api/fallback \\
  -H "Content-Type: application/json" \\
  -d '{
    "type": "model",
    "id": "production-chain",
    "fallbacks": ["gpt-4o", "claude-sonnet-4", "gemini-2.5-pro"]
  }'

# Pakai fallback
curl http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"production-chain","messages":[...]}'`,
          en_code: `# Create fallback chain
curl -X POST http://localhost:20180/api/fallback \\
  -H "Content-Type: application/json" \\
  -d '{
    "type": "model",
    "id": "production-chain",
    "fallbacks": ["gpt-4o", "claude-sonnet-4", "gemini-2.5-pro"]
  }'

# Use fallback
curl http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"production-chain","messages":[...]}'`,
          language: 'bash',
        },
        {
          id: 'caching',
          id_title: 'Caching & Kompresi',
          en_title: 'Caching & Compression',
          id_content: `**Semantic Cache (3-tier):**

1. **Exact Match** — request identical persis → cached response (instant)
2. **Stream Replay** — streaming response di-cache & di-replay
3. **Cosine Similarity** — request mirip (>0.95) → suggest cached

**Token Compression:**
Lintasan otomatis mengkompresi context panjang sebelum dikirim ke provider. Hemat token 20-40% tanpa kehilangan makna.

**Cache Headers:**
- \`X-Lintasan-Cache: HIT\` — response dari cache
- \`X-Lintasan-Cache: MISS\` — response baru dari provider`,
          en_content: `**Semantic Cache (3-tier):**

1. **Exact Match** — identical request → cached response (instant)
2. **Stream Replay** — streaming responses cached & replayed
3. **Cosine Similarity** — similar request (>0.95) → suggest from cache

**Token Compression:**
Lintasan auto-compresses long contexts before sending to providers. Saves 20-40% tokens without losing meaning.

**Cache Headers:**
- \`X-Lintasan-Cache: HIT\` — response from cache
- \`X-Lintasan-Cache: MISS\` — fresh response from provider`,
        },
        {
          id: 'mcp-server',
          id_title: 'MCP Server',
          en_title: 'MCP Server',
          id_content: `Lintasan built-in **MCP (Model Context Protocol)** server dengan 14 tools via JSON-RPC 2.0.

**Tools yang tersedia:**
- \`lintasan_models_list\` — List model yang tersedia
- \`lintasan_connections_list\` — List provider connections
- \`lintasan_chat\` — Chat completion via Lintasan
- \`lintasan_memory_search\` — Search vector memory
- \`lintasan_memory_store\` — Store ke vector memory
- \`lintasan_web_search\` — Web search
- \`lintasan_stats\` — Usage statistics
- Dan 7+ lainnya...

**Koneksi MCP Client:**
\`\`\`json
{
  "mcpServers": {
    "lintasan": {
      "url": "http://localhost:20180/mcp"
    }
  }
}
\`\`\``,
          en_content: `Lintasan has a built-in **MCP (Model Context Protocol)** server with 14 tools via JSON-RPC 2.0.

**Available Tools:**
- \`lintasan_models_list\` — List available models
- \`lintasan_connections_list\` — List provider connections
- \`lintasan_chat\` — Chat completion via Lintasan
- \`lintasan_memory_search\` — Search vector memory
- \`lintasan_memory_store\` — Store to vector memory
- \`lintasan_web_search\` — Web search
- \`lintasan_stats\` — Usage statistics
- And 7+ more...

**Connect MCP Client:**
\`\`\`json
{
  "mcpServers": {
    "lintasan": {
      "url": "http://localhost:20180/mcp"
    }
  }
}
\`\`\``,
          language: 'json',
        },
        {
          id: 'translation',
          id_title: 'Format Translation',
          en_title: 'Format Translation',
          id_content: `Lintasan bisa menerjemahkan request & response antar format AI provider berbeda:

**5 format supported:**
- \`openai\` — OpenAI Chat Completion
- \`anthropic\` — Anthropic Messages API
- \`gemini\` — Google Gemini GenerateContent
- \`commandcode\` — CommandCode Alpha format
- \`llama\` — Llama Chat Format

**Gunanya:** kalau app-mu pakai format OpenAI, tapi provider-mu cuma support Anthropic → Lintasan auto-translate.`,

          en_content: `Lintasan can translate requests & responses between different AI provider formats:

**5 formats supported:**
- \`openai\` — OpenAI Chat Completion
- \`anthropic\` — Anthropic Messages API
- \`gemini\` — Google Gemini GenerateContent
- \`commandcode\` — CommandCode Alpha format
- \`llama\` — Llama Chat Format

**Use case:** your app uses OpenAI format, but your provider only supports Anthropic → Lintasan auto-translates.`,
        },
        {
          id: 'web-search',
          id_title: 'Web Search Terintegrasi',
          en_title: 'Integrated Web Search',
          id_content: `Lintasan punya fitur **web search** yang bisa dipanggil langsung via API:

**Endpoint:** \`POST /v1/web/search\`

**Response:** hasil pencarian web + AI summary.

Ini memungkinkan RAG (Retrieval-Augmented Generation) tanpa setup vector DB eksternal.`,

          en_content: `Lintasan has a built-in **web search** feature callable directly via API:

**Endpoint:** \`POST /v1/web/search\`

**Response:** web search results + AI summary.

This enables RAG (Retrieval-Augmented Generation) without external vector DB setup.`,
          id_code: `curl -X POST http://localhost:20180/v1/web/search \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"query":"latest AI news 2026","max_results":5}'`,
          en_code: `curl -X POST http://localhost:20180/v1/web/search \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"query":"latest AI news 2026","max_results":5}'`,
          language: 'bash',
        },
        {
          id: 'cost-budget',
          id_title: 'Cost Tracking & Budget',
          en_title: 'Cost Tracking & Budget',
          id_content: `Lintasan melacak biaya setiap request secara real-time:

- **Cost per request:** dihitung dari token input + output × harga model
- **Budget limits:** set batas harian / bulanan per user
- **Savings tracker:** lihat berapa yang dihemat dari caching & compression
- **Dashboard:** grafik biaya real-time di halaman Analytics & Savings`,

          en_content: `Lintasan tracks cost per request in real-time:

- **Cost per request:** calculated from input + output tokens × model price
- **Budget limits:** set daily / monthly caps per user
- **Savings tracker:** see how much saved from caching & compression
- **Dashboard:** real-time cost graphs on Analytics & Savings pages`,
        },
        {
          id: 'plugins-system',
          id_title: 'Plugin System',
          en_title: 'Plugin System',
          id_content: `Lintasan punya **plugin system** yang bisa di-extend tanpa ubah core:

- **Built-in:** Request Logger, Rate Limiter, Cost Guard
- **Plugin Store:** Install plugin dari komunitas
- **AI Generator:** Generate plugin baru pakai natural language

Plugin auto-register dan jalan di request pipeline. Bisa nambah preprocessing, postprocessing, atau custom routing logic tanpa fork kode.`,

          en_content: `Lintasan has an extensible **plugin system** that works without core changes:

- **Built-in:** Request Logger, Rate Limiter, Cost Guard
- **Plugin Store:** Install community plugins
- **AI Generator:** Generate new plugins with natural language

Plugins auto-register and run in the request pipeline. Add preprocessing, postprocessing, or custom routing logic without forking code.`,
        },
      ],
    },
    {
      id: 'integrations',
      id_title: 'Panduan Integrasi',
      en_title: 'Integration Guide',
      icon: Plug,
      subsections: [
        {
          id: 'python-client',
          id_title: 'Python',
          en_title: 'Python',
          id_content: `**Drop-in replacement** — ganti \`base_url\` ke Lintasan, sisanya sama. Semua fitur OpenAI SDK support.`,

          en_content: `**Drop-in replacement** — change \`base_url\` to Lintasan, everything else stays the same. All OpenAI SDK features supported.`,
          id_code: `from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:20180/v1",
    api_key="lintasan-api-key",  # dari Dashboard → Users
)

# Chat
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello!"}],
    temperature=0.7,
    max_tokens=500,
)
print(response.choices[0].message.content)

# Streaming
stream = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Write a poem"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")

# Embeddings
emb = client.embeddings.create(
    model="text-embedding-3-small",
    input="Hello world",
)
print(emb.data[0].embedding[:5])

# Function calling
tools = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "parameters": {"city": "string"}
    }
}]
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Weather in Jakarta?"}],
    tools=tools,
)`,
          en_code: `from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:20180/v1",
    api_key="lintasan-api-key",  # from Dashboard → Users
)

# Chat
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello!"}],
    temperature=0.7,
    max_tokens=500,
)
print(response.choices[0].message.content)

# Streaming
stream = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Write a poem"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")

# Embeddings
emb = client.embeddings.create(
    model="text-embedding-3-small",
    input="Hello world",
)
print(emb.data[0].embedding[:5])

# Function calling
tools = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "parameters": {"city": "string"}
    }
}]
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Weather in Jakarta?"}],
    tools=tools,
)`,
          language: 'python',
        },
        {
          id: 'nodejs-client',
          id_title: 'Node.js / TypeScript',
          en_title: 'Node.js / TypeScript',
          id_content: `**Drop-in replacement** untuk semua OpenAI Node.js SDK features. Ganti \`baseURL\` aja.`,

          en_content: `**Drop-in replacement** for all OpenAI Node.js SDK features. Just change \`baseURL\`.`,
          id_code: `import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:20180/v1',
  apiKey: process.env.LINTASAN_API_KEY, // dari Dashboard → Users
});

// Chat
const response = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Hello!' }],
});
console.log(response.choices[0].message.content);

// Streaming
const stream = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Write a poem' }],
  stream: true,
});
for await (const chunk of stream) {
  const content = chunk.choices[0]?.delta?.content;
  if (content) process.stdout.write(content);
}

// Embeddings
const emb = await client.embeddings.create({
  model: 'text-embedding-3-small',
  input: 'Hello world',
});
console.log(emb.data[0].embedding.slice(0, 5));`,
          en_code: `import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:20180/v1',
  apiKey: process.env.LINTASAN_API_KEY, // from Dashboard → Users
});

// Chat
const response = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Hello!' }],
});
console.log(response.choices[0].message.content);

// Streaming
const stream = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Write a poem' }],
  stream: true,
});
for await (const chunk of stream) {
  const content = chunk.choices[0]?.delta?.content;
  if (content) process.stdout.write(content);
}

// Embeddings
const emb = await client.embeddings.create({
  model: 'text-embedding-3-small',
  input: 'Hello world',
});
console.log(emb.data[0].embedding.slice(0, 5));`,
          language: 'typescript',
        },
        {
          id: 'curl-tips',
          id_title: 'curl / Terminal Tips',
          en_title: 'curl / Terminal Tips',
          id_content: `Semua endpoint bisa diakses via curl.

**Tips:** pakai \`-N\` untuk streaming dan \`-s\` untuk silent mode.`,

          en_content: `All endpoints are accessible via curl.

**Tips:** use \`-N\` for streaming and \`-s\` for silent mode.`,
          id_code: `# Non-streaming chat
curl -s http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}]}' \\
  | python3 -c "import sys,json; print(json.load(sys.stdin)['choices'][0]['message']['content'])"

# Streaming chat (real-time output)
curl -Ns http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Tell a joke"}],"stream":true}'

# List models + count
curl -s http://localhost:20180/v1/models \\
  -H "Authorization: Bearer *** \\
  | python3 -c "import sys,json; d=json.load(sys.stdin); [print(m['id']) for m in d['data']]"

# Health check
curl -s http://localhost:20180/health`,
          en_code: `# Non-streaming chat
curl -s http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}]}' \\
  | python3 -c "import sys,json; print(json.load(sys.stdin)['choices'][0]['message']['content'])"

# Streaming chat (real-time output)
curl -Ns http://localhost:20180/v1/chat/completions \\
  -H "Authorization: Bearer *** \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Tell a joke"}],"stream":true}'

# List models + count
curl -s http://localhost:20180/v1/models \\
  -H "Authorization: Bearer *** \\
  | python3 -c "import sys,json; d=json.load(sys.stdin); [print(m['id']) for m in d['data']]"

# Health check
curl -s http://localhost:20180/health`,
          language: 'bash',
        },
        {
          id: 'ide-tools-header',
          id_title: 'AI Coding Tools (IDE/CLI)',
          en_title: 'AI Coding Tools (IDE/CLI)',
          id_content: `Semua AI coding tools yang support OpenAI-compatible API bisa diarahkan ke Lintasan. Cukup ganti endpoint dan API key. **Otomatis dapat routing, caching, dan fallback.**

Di bawah ini konfigurasi untuk tools populer.`,

          en_content: `All AI coding tools that support OpenAI-compatible API can point to Lintasan. Just change the endpoint and API key. **Automatically get routing, caching, and fallback.**

Below are configs for popular tools.`,
        },
        {
          id: 'claude-code',
          id_title: 'Claude Code',
          en_title: 'Claude Code',
          id_content: `Anthropic Claude Code CLI agent. Arahkan ke Lintasan via env var. **Semua model provider bisa dipakai** — tidak terbatas Claude.`,

          en_content: `Anthropic Claude Code CLI agent. Point to Lintasan via env var. **Any provider model works** — not limited to Claude.`,
          id_code: `# ~/.bashrc
export ANTHROPIC_BASE_URL="http://localhost:20180/v1"
export ANTHROPIC_API_KEY="lintasan-api-key"
claude`,
          en_code: `# ~/.bashrc
export ANTHROPIC_BASE_URL="http://localhost:20180/v1"
export ANTHROPIC_API_KEY="lintasan-api-key"
claude`,
          language: 'bash',
        },
        {
          id: 'hermes-agent',
          id_title: 'Hermes Agent',
          en_title: 'Hermes Agent',
          id_content: `Hermes Agent dengan konfigurasi Lintasan sebagai custom provider. Semua model provider muncul di Hermes.`,

          en_content: `Hermes Agent with Lintasan configured as a custom provider. All provider models appear in Hermes.`,
          id_code: `# ~/.hermes/config.yaml
models:
  providers:
    lintasan:
      base_url: "http://localhost:20180/v1"
      api_key: "lintasan-api-key"
      models:
        - "gpt-4o"
        - "claude-sonnet-4"
        - "gemini-2.5-pro"`,
          en_code: `# ~/.hermes/config.yaml
models:
  providers:
    lintasan:
      base_url: "http://localhost:20180/v1"
      api_key: "lintasan-api-key"
      models:
        - "gpt-4o"
        - "claude-sonnet-4"
        - "gemini-2.5-pro"`,
          language: 'yaml',
        },
        {
          id: 'more-tools',
          id_title: 'Tools Lainnya',
          en_title: 'Other Tools',
          id_content: `**Codex CLI (OpenAI):**
\`\`\`bash
export OPENAI_API_KEY="lintasan-api-key"
export OPENAI_API_BASE_URL="http://localhost:20180/v1"
codex edit "fix the auth middleware"
\`\`\`

**Aider:**
\`\`\`bash
export OPENAI_API_KEY="lintasan-api-key"
export OPENAI_API_BASE="http://localhost:20180/v1"
aider --model gpt-4o
\`\`\`

**OpenCode CLI:**
\`\`\`bash
export OPENAI_API_KEY="lintasan-api-key"
export OPENAI_ENDPOINT="http://localhost:20180/v1"
opencode "create a REST API"
\`\`\`

**Zed Editor** — settings.json:
\`\`\`json
{"assistant": {"provider": {"name": "lintasan", "type": "open_ai_compatible", "api_url": "http://localhost:20180/v1"}, "default_model": "gpt-4o"}}
\`\`\`

**Cursor IDE** — Settings → OpenAI:
\`\`\`
API Key: lintasan-api-key
Base URL: http://localhost:20180/v1
\`\`\`

**Continue.dev** (VS Code / JetBrains):
\`\`\`json
{"models": [{"title": "Lintasan", "provider": "openai", "model": "gpt-4o", "apiKey": "lintasan-api-key", "apiBase": "http://localhost:20180/v1"}]}
\`\`\``,

          en_content: `**Codex CLI (OpenAI):**
\`\`\`bash
export OPENAI_API_KEY="lintasan-api-key"
export OPENAI_API_BASE_URL="http://localhost:20180/v1"
codex edit "fix the auth middleware"
\`\`\`

**Aider:**
\`\`\`bash
export OPENAI_API_KEY="lintasan-api-key"
export OPENAI_API_BASE="http://localhost:20180/v1"
aider --model gpt-4o
\`\`\`

**OpenCode CLI:**
\`\`\`bash
export OPENAI_API_KEY="lintasan-api-key"
export OPENAI_ENDPOINT="http://localhost:20180/v1"
opencode "create a REST API"
\`\`\`

**Zed Editor** — settings.json:
\`\`\`json
{"assistant": {"provider": {"name": "lintasan", "type": "open_ai_compatible", "api_url": "http://localhost:20180/v1"}, "default_model": "gpt-4o"}}
\`\`\`

**Cursor IDE** — Settings → OpenAI:
\`\`\`
API Key: lintasan-api-key
Base URL: http://localhost:20180/v1
\`\`\`

**Continue.dev** (VS Code / JetBrains):
\`\`\`json
{"models": [{"title": "Lintasan", "provider": "openai", "model": "gpt-4o", "apiKey": "lintasan-api-key", "apiBase": "http://localhost:20180/v1"}]}
\`\`\``,
        },
      ],
    },
    {
      id: 'deployment',
      id_title: 'Deployment',
      en_title: 'Deployment',
      icon: Server,
      subsections: [
        {
          id: 'systemd',
          id_title: 'Systemd (Linux)',
          en_title: 'Systemd (Linux)',
          id_content: `Cara paling stabil untuk production di Linux. Auto-restart jika crash.`,

          en_content: `Most stable way for production on Linux. Auto-restart on crash.`,
          id_code: `# 1. Copy binary ke system path
sudo cp lintasan /usr/local/bin/lintasan
sudo chmod +x /usr/local/bin/lintasan

# 2. Buat systemd service
sudo tee /etc/systemd/system/lintasan.service <<'EOF'
[Unit]
Description=Lintasan — LLM Proxy Router
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/lintasan-go
ExecStart=/home/ubuntu/lintasan-go/lintasan start
Restart=always
RestartSec=3
Environment=PORT=20180

[Install]
WantedBy=multi-user.target
EOF

# 3. Enable & start
sudo systemctl daemon-reload
sudo systemctl enable lintasan
sudo systemctl start lintasan
sudo systemctl status lintasan`,
          en_code: `# 1. Copy binary to system path
sudo cp lintasan /usr/local/bin/lintasan
sudo chmod +x /usr/local/bin/lintasan

# 2. Create systemd service
sudo tee /etc/systemd/system/lintasan.service <<'EOF'
[Unit]
Description=Lintasan — LLM Proxy Router
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/lintasan-go
ExecStart=/home/ubuntu/lintasan-go/lintasan start
Restart=always
RestartSec=3
Environment=PORT=20180

[Install]
WantedBy=multi-user.target
EOF

# 3. Enable & start
sudo systemctl daemon-reload
sudo systemctl enable lintasan
sudo systemctl start lintasan
sudo systemctl status lintasan`,
          language: 'bash',
        },
        {
          id: 'nginx-reverse-proxy',
          id_title: 'Nginx Reverse Proxy',
          en_title: 'Nginx Reverse Proxy',
          id_content: `Untuk production dengan domain dan HTTPS. Nginx di depan Lintasan.`,

          en_content: `For production with domain and HTTPS. Nginx in front of Lintasan.`,
          id_code: `server {
    listen 443 ssl http2;
    server_name lintasan.example.com;

    ssl_certificate     /etc/ssl/example.crt;
    ssl_certificate_key /etc/ssl/example.key;

    location / {
        proxy_pass http://localhost:20180;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Important for streaming
        proxy_buffering off;
        proxy_read_timeout 300s;
        proxy_connect_timeout 60s;
    }
}

# HTTP → HTTPS redirect
server {
    listen 80;
    server_name lintasan.example.com;
    return 301 https://$host$request_uri;
}`,
          en_code: `server {
    listen 443 ssl http2;
    server_name lintasan.example.com;

    ssl_certificate     /etc/ssl/example.crt;
    ssl_certificate_key /etc/ssl/example.key;

    location / {
        proxy_pass http://localhost:20180;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Important for streaming
        proxy_buffering off;
        proxy_read_timeout 300s;
        proxy_connect_timeout 60s;
    }
}

# HTTP → HTTPS redirect
server {
    listen 80;
    server_name lintasan.example.com;
    return 301 https://$host$request_uri;
}`,
          language: 'nginx',
        },
        {
          id: 'production-checklist',
          id_title: 'Production Checklist',
          en_title: 'Production Checklist',
          id_content: `Sebelum deploy production:

☐ Ganti password admin default
☐ Setup HTTPS (Let's Encrypt + nginx)
☐ Backup SQLite database (\`/data/lintasan.db\`)
☐ Set rate limiting sesuai kapasitas provider
☐ Enable circuit breaker
☐ Monitor health via \`/health\` endpoint
☐ Setup periodic backup (cron)
☐ Uji fallback chain (matikan provider → harus auto-failover)
☐ Check streaming berfungsi (proxy_buffering off)`,

          en_content: `Before deploying to production:

☐ Change default admin password
☐ Set up HTTPS (Let's Encrypt + nginx)
☐ Back up SQLite database (\`/data/lintasan.db\`)
☐ Set rate limiting according to provider capacity
☐ Enable circuit breaker
☐ Monitor health via \`/health\` endpoint
☐ Set up periodic backup (cron)
☐ Test fallback chain (disable provider → must auto-failover)
☐ Check streaming works (proxy_buffering off)`,
        },
      ],
    },
    {
      id: 'faq',
      id_title: 'FAQ & Troubleshooting',
      en_title: 'FAQ & Troubleshooting',
      icon: HelpCircle,
      subsections: [
        {
          id: 'faq-models',
          id_title: 'Model tidak muncul?',
          en_title: 'Models not showing?',
          id_content: `**Penyebab:** Lintasan belum sync models dari provider.

**Solusi:** Dashboard → Connections → klik provider → **"Sync Now"**. Model akan muncul setelah sync berhasil.

Kalau masih kosong, cek:
- Provider aktif? (toggle ON)
- API key valid? (Test Connection)
- Endpoint /v1/models bisa diakses dari server Lintasan?`,
          en_content: `**Cause:** Lintasan hasn't synced models from the provider.

**Solution:** Dashboard → Connections → click provider → **"Sync Now"**. Models will appear after successful sync.

If still empty, check:
- Provider active? (toggle ON)
- API key valid? (Test Connection)
- Can the Lintasan server reach the provider's /v1/models endpoint?`,
        },
        {
          id: 'faq-streaming',
          id_title: 'Streaming tidak jalan?',
          en_title: 'Streaming not working?',
          id_content: `**Penyebab umum:** Reverse proxy (nginx/Cloudflare) mem-buffer response.

**Solusi:**
- Nginx: tambahkan \`proxy_buffering off;\`
- Cloudflare: matikan "Rocket Loader" dan "Auto Minify"
- curl: tambahkan \`-N\` flag

**Cek:** coba curl langsung ke localhost tanpa proxy.`,

          en_content: `**Common cause:** Reverse proxy (nginx/Cloudflare) buffers the response.

**Solution:**
- Nginx: add \`proxy_buffering off;\`
- Cloudflare: disable "Rocket Loader" and "Auto Minify"
- curl: add \`-N\` flag

**Check:** try curl directly to localhost without proxy.`,
        },
        {
          id: 'faq-migration',
          id_title: 'Migrasi dari OpenAI langsung',
          en_title: 'Migrating from direct OpenAI',
          id_content: `**Paling gampang!** Hanya 2 langkah:

1. **Ganti environment variable:**
\`\`\`
# Sebelum:
OPENAI_API_KEY="sk-..."
OPENAI_BASE_URL="https://api.openai.com/v1"

# Sesudah:
OPENAI_API_KEY="lintasan-api-key"  # dari Dashboard → Users
OPENAI_BASE_URL="http://localhost:20180/v1"
\`\`\`

2. **Tambah OpenAI sebagai provider di Lintasan** — Dashboard → Connections → Import Curl.

**Kode tidak perlu diubah.** Semua SDK OpenAI tetap jalan.`,

          en_content: `**Super easy!** Only 2 steps:

1. **Change environment variables:**
\`\`\`
# Before:
OPENAI_API_KEY="sk-..."
OPENAI_BASE_URL="https://api.openai.com/v1"

# After:
OPENAI_API_KEY="lintasan-api-key"  # from Dashboard → Users
OPENAI_BASE_URL="http://localhost:20180/v1"
\`\`\`

2. **Add OpenAI as a provider in Lintasan** — Dashboard → Connections → Import Curl.

**No code changes needed.** All OpenAI SDKs continue to work.`,
        },
        {
          id: 'faq-9router',
          id_title: 'Migrasi dari 9Router',
          en_title: 'Migrating from 9Router',
          id_content: '**9Router vs Lintasan — Perbedaan Konsep**\n\n| 9Router | Lintasan |\n|---------|----------|\n| Prefix model (`cc/`, `or/`, `cx/`) | Otomatis — model dicocokkan ke connection yang memilikinya |\n| Combo = urutan model fallback | Fallback Chain = fungsi yang sama |\n| Tambah provider via config manual | **Import Curl** (copy-paste) / Presets / Manual |\n\n**Cara migrasi:**\n\n1. **Pindahkan provider:** Copy curl dari 9Router dashboard → Lintasan Connections → **Import Curl** → auto-discover models.\n\n2. **Ganti base URL:**\n```\n# Sebelum (9Router)\nhttp://localhost:20128/v1\n\n# Sesudah (Lintasan)\nhttp://localhost:20180/v1\n```\n\n3. **Model mapping — prefix diganti jadi model langsung:**\n```\n# 9Router prefix → Lintasan model\ncc/claude-sonnet-4.5   → claude-sonnet-4.5   (via Claude Code connection)\nor/claude-sonnet-4.5   → claude-sonnet-4.5   (via OpenRouter connection)\nglm/glm-4.5            → glm-4.5             (via GLM connection)\n```\nModel yang sama di beberapa provider akan otomatis ke provider prioritas tertinggi.\n\n4. **Fallback Chain = pengganti Combo:**\n```\n// 9Router Combo: Claude-Fallback = [cc/sonnet, cx/sonnet, or/sonnet]\n// ↓ jadi ↓\n// Lintasan Fallback Chain: ["claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4"]\n// (tiap entry di-resolve ke connection dengan priority tertinggi)\n```\n\n5. **Ubah config di tools:**\n```\n# Claude Code, Cursor, Aider, Codex, dll\nBase URL: http://localhost:20180/v1\nAPI Key: (dari Lintasan Dashboard → Users)\n```\n\n**Yang otomatis tanpa config:**\n- ✅ Routing — model otomatis ke provider terbaik\n- ✅ Circuit breaker — provider down auto-skip\n- ✅ Caching — response mirip dari cache\n- ✅ Load balancing — distribusi antar provider',
          en_content: '**9Router vs Lintasan — Concept Differences**\n\n| 9Router | Lintasan |\n|---------|----------|\n| Model prefix (`cc/`, `or/`, `cx/`) | Automatic — model matched to connection that has it |\n| Combo = ordered model fallback list | Fallback Chain = same function |\n| Add provider via manual config | **Import Curl** (copy-paste) / Presets / Manual |\n\n**How to migrate:**\n\n1. **Move providers:** Copy curl from 9Router dashboard → Lintasan Connections → **Import Curl** → auto-discover models.\n\n2. **Change base URL:**\n```\n# Before (9Router)\nhttp://localhost:20128/v1\n\n# After (Lintasan)\nhttp://localhost:20180/v1\n```\n\n3. **Model mapping — prefix becomes direct model name:**\n```\n# 9Router prefix → Lintasan model\ncc/claude-sonnet-4.5   → claude-sonnet-4.5   (via Claude Code connection)\nor/claude-sonnet-4.5   → claude-sonnet-4.5   (via OpenRouter connection)\nglm/glm-4.5            → glm-4.5             (via GLM connection)\n```\nSame model on multiple providers auto-routes to highest-priority connection.\n\n4. **Fallback Chain replaces Combo:**\n```\n// 9Router Combo: Claude-Fallback = [cc/sonnet, cx/sonnet, or/sonnet]\n// ↓ becomes ↓\n// Lintasan Fallback Chain: ["claude-sonnet-4", "claude-sonnet-4", "claude-sonnet-4"]\n// (each entry resolves to the connection with highest priority)\n```\n\n5. **Update config in your tools:**\n```\n# Claude Code, Cursor, Aider, Codex, etc.\nBase URL: http://localhost:20180/v1\nAPI Key: (from Lintasan Dashboard → Users)\n```\n\n**Automatic features — no config needed:**\n- ✅ Smart routing — model auto-routes to best provider\n- ✅ Circuit breaker — down providers auto-skipped\n- ✅ Caching — similar responses served from cache\n- ✅ Load balancing — distributed across providers',
        },
        {
          id: 'faq-errors',
          id_title: 'Error umum & solusi',
          en_title: 'Common errors & solutions',
          id_content: `**"Connection refused"**
→ Lintasan belum jalan. Cek: \`./lintasan start\` atau \`systemctl status lintasan\`

**"Invalid API key"**
→ API key salah. Pastikan pakai **API key dari dashboard Users**, bukan API key provider.

**"All routes failed"**
→ Semua provider gagal. Cek: Test Connection di dashboard per provider.

**"gzip: invalid header" (atau body kosong)**
→ Provider upstream kirim encoding header tapi body nggak compressed. Lintasan sudah auto-handle ini sejak v0.24.0. Kalau masih terjadi, report issue.

**"Too many requests"**
→ Rate limit kena. Default 60 req/menit. Naikkan di Settings → Rate Limit.`,
          en_content: `**"Connection refused"**
→ Lintasan isn't running. Check: \`./lintasan start\` or \`systemctl status lintasan\`

**"Invalid API key"**
→ Wrong API key. Make sure you're using the **API key from dashboard Users**, NOT the provider API key.

**"All routes failed"**
→ All providers failed. Check: Test Connection in dashboard per provider.

**"gzip: invalid header" (or empty body)**
→ Upstream provider sends encoding header but body isn't actually compressed. Lintasan auto-handles this since v0.24.0. If still happening, report issue.

**"Too many requests"**
→ Rate limit hit. Default 60 req/min. Increase in Settings → Rate Limit.`,
        },
      ],
    },
  ];

  function toggleSection(id: string) {
    if (expandedSections.has(id)) {
      expandedSections.delete(id);
    } else {
      expandedSections.add(id);
    }
    expandedSections = new Set(expandedSections);
  }

  function scrollToSubsection(sectionId: string, subsectionId: string) {
    activeSection = sectionId;
    if (!expandedSections.has(sectionId)) {
      expandedSections.add(sectionId);
      expandedSections = new Set(expandedSections);
    }
    requestAnimationFrame(() => {
      const el = document.getElementById(`doc-${subsectionId}`);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  }

  async function copyCode(code: string) {
    await navigator.clipboard.writeText(code);
    copiedCode = code;
    setTimeout(() => { copiedCode = null; }, 2000);
  }

  function renderDocContent(content: string): string {
    let html = content
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');

    html = html.replace(/`([^`]+)`/g, '<code class="doc-inline-code">$1</code>');
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');
    html = html.replace(/```json\n([\s\S]*?)```/g, '<pre class="code-block-inline"><code>$1</code></pre>');
    html = html.replace(/```bash\n([\s\S]*?)```/g, '<pre class="code-block-inline"><code>$1</code></pre>');
    html = html.replace(/```\n([\s\S]*?)```/g, '<pre class="code-block-inline"><code>$1</code></pre>');
    html = html.replace(/\n\n/g, '</p><p style="margin-top: 8px;">');
    html = html.replace(/\n/g, '<br>');

    return `<p>${html}</p>`;
  }

  let filteredDocs = $derived(
    searchQuery.trim()
      ? docs.map(section => ({
          ...section,
          subsections: section.subsections.filter(
            sub =>
              (lang === 'id' ? sub.id_title : sub.en_title).toLowerCase().includes(searchQuery.toLowerCase()) ||
              (lang === 'id' ? sub.id_content : sub.en_content).toLowerCase().includes(searchQuery.toLowerCase())
          ),
        })).filter(section => section.subsections.length > 0)
      : docs
  );

  function t(idText: string, enText: string): string {
    return lang === 'id' ? idText : enText;
  }
</script>

<svelte:head>
  <title>Docs — Lintasan</title>
</svelte:head>

<div style="display: flex; gap: 24px; min-height: calc(100vh - var(--header-h) - 48px);">
  <!-- Sidebar -->
  <div
    class="docs-sidebar"
    style="
      width: 260px; flex-shrink: 0;
      background: var(--color-bg-card);
      border: 1px solid var(--color-border);
      border-radius: var(--radius);
      overflow: hidden;
      position: sticky; top: calc(var(--header-h) + 24px);
      height: fit-content; max-height: calc(100vh - var(--header-h) - 48px);
      display: flex; flex-direction: column;
    "
  >
    <!-- Language toggle + Search -->
    <div style="padding: 14px; border-bottom: 1px solid var(--color-border); display: flex; flex-direction: column; gap: 10px;">
      <div style="display: flex; gap: 6px;">
        <button class="lang-btn" class:active={lang === 'id'} onclick={() => lang = 'id'}>🇮🇩 ID</button>
        <button class="lang-btn" class:active={lang === 'en'} onclick={() => lang = 'en'}>🇬🇧 EN</button>
      </div>
      <div style="position: relative;">
        <Search size={14} style="color: var(--color-fg-3); position: absolute; left: 10px; top: 50%; transform: translateY(-50%); pointer-events: none;" />
        <input
          class="input-field"
          placeholder={lang === 'id' ? 'Cari docs...' : 'Search docs...'}
          bind:value={searchQuery}
          style="padding-left: 32px; font-size: 12px;"
        />
      </div>
    </div>

    <nav style="flex: 1; overflow-y: auto; padding: 12px 10px;">
      {#each filteredDocs as section}
        <div style="margin-bottom: 4px;">
          <button
            class="sidebar-section-btn"
            class:active={activeSection === section.id}
            onclick={() => {
              activeSection = section.id;
              toggleSection(section.id);
            }}
          >
            <section.icon size={16} stroke-width={1.8} />
            <span style="flex: 1; text-align: left;">{t(section.id_title, section.en_title)}</span>
            {#if expandedSections.has(section.id)}
              <ChevronDown size={14} />
            {:else}
              <ChevronRight size={14} />
            {/if}
          </button>

          {#if expandedSections.has(section.id)}
            <div style="padding-left: 12px; animation: fadeInUp 0.2s ease-out;">
              {#each section.subsections as sub}
                <button
                  class="sidebar-sub-btn"
                  onclick={() => scrollToSubsection(section.id, sub.id)}
                >
                  {t(sub.id_title, sub.en_title)}
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    </nav>
  </div>

  <!-- Main content -->
  <div style="flex: 1; min-width: 0;">
    <div style="display: flex; flex-direction: column; gap: 24px;">
      {#if filteredDocs.length === 0}
        <div class="card">
          <div class="flex flex-col items-center justify-center" style="padding: 48px; opacity: 0.6;">
            <Search size={48} style="color: var(--color-fg-3); margin-bottom: 12px;" stroke-width={1.2} />
            <div style="font-size: 14px; font-weight: 500; color: var(--color-fg-2);">
              {lang === 'id' ? 'Tidak ditemukan' : 'No results found'}
            </div>
          </div>
        </div>
      {:else}
        {#each filteredDocs as section}
          <div class="card" style="padding: 0; overflow: hidden;">
            <button class="section-header" onclick={() => toggleSection(section.id)}>
              <div class="flex items-center gap-3">
                <div
                  class="flex items-center justify-center rounded-lg"
                  style="width: 36px; height: 36px; background: var(--color-primary-light);"
                >
                  <section.icon size={18} style="color: var(--color-primary);" />
                </div>
                <span style="font-size: 16px; font-weight: 600; color: var(--color-fg-0);">
                  {t(section.id_title, section.en_title)}
                </span>
                <span style="font-size: 12px; color: var(--color-fg-3); font-weight: 400;">
                  {section.subsections.length} {lang === 'id' ? 'bagian' : 'sections'}
                </span>
              </div>
              {#if expandedSections.has(section.id)}
                <ChevronDown size={18} style="color: var(--color-fg-3);" />
              {:else}
                <ChevronRight size={18} style="color: var(--color-fg-3);" />
              {/if}
            </button>

            {#if expandedSections.has(section.id)}
              <div style="border-top: 1px solid var(--color-border);">
                {#each section.subsections as sub, i}
                  <div
                    id="doc-{sub.id}"
                    class="doc-subsection"
                    style="animation: fadeInUp {0.2 + i * 0.06}s ease-out;"
                  >
                    <h3 style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px;">
                      {t(sub.id_title, sub.en_title)}
                    </h3>
                    <div class="doc-content">
                      {@html renderDocContent(t(sub.id_content, sub.en_content))}
                    </div>
                    {#if (lang === 'id' ? sub.id_code : sub.en_code)}
                      <div class="code-container">
                        <div class="code-header">
                          <span style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">
                            {sub.language || 'code'}
                          </span>
                          <button
                            class="code-copy-btn"
                            onclick={() => copyCode((lang === 'id' ? sub.id_code : sub.en_code)!)}
                            title={lang === 'id' ? 'Salin kode' : 'Copy code'}
                          >
                            {#if copiedCode === (lang === 'id' ? sub.id_code : sub.en_code)}
                              <Check size={12} style="color: var(--color-success);" />
                            {:else}
                              <Copy size={12} />
                            {/if}
                          </button>
                        </div>
                        <pre class="code-block"><code>{lang === 'id' ? sub.id_code : sub.en_code}</code></pre>
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  </div>
</div>

<style>
  .docs-sidebar { display: block; }
  @media (max-width: 768px) {
    .docs-sidebar { display: none !important; }
  }

  .lang-btn {
    flex: 1; padding: 6px 10px; border: 1px solid var(--color-border);
    background: transparent; color: var(--color-fg-2);
    font-size: 12px; font-weight: 500; cursor: pointer;
    border-radius: 6px; transition: var(--transition);
  }
  .lang-btn:hover { background: var(--color-bg-body); }
  .lang-btn.active { background: var(--color-primary); color: #fff; border-color: var(--color-primary); }

  .sidebar-section-btn {
    display: flex; align-items: center; gap: 10px; width: 100%;
    padding: 10px 12px; border: none; background: transparent;
    color: var(--color-fg-1); font-size: 13px; font-weight: 500;
    cursor: pointer; border-radius: var(--radius-sm); transition: var(--transition);
  }
  .sidebar-section-btn:hover { background: var(--color-bg-body); color: var(--color-fg-0); }
  .sidebar-section-btn.active { color: var(--color-primary); background: var(--color-primary-light); font-weight: 600; }

  .sidebar-sub-btn {
    display: block; width: 100%; padding: 7px 12px; border: none;
    background: transparent; color: var(--color-fg-2); font-size: 12px;
    cursor: pointer; text-align: left; border-radius: var(--radius-sm); transition: var(--transition);
  }
  .sidebar-sub-btn:hover { background: var(--color-bg-body); color: var(--color-fg-0); }

  .section-header {
    display: flex; align-items: center; justify-content: space-between;
    width: 100%; padding: 18px 20px; border: none; background: transparent;
    cursor: pointer; transition: var(--transition);
  }
  .section-header:hover { background: var(--color-bg-body); }

  .doc-subsection {
    padding: 20px 24px; border-bottom: 1px solid var(--color-border-light);
  }
  .doc-subsection:last-child { border-bottom: none; }

  .doc-content {
    font-size: 13px; color: var(--color-fg-1); line-height: 1.7;
  }
  .doc-content :global(.doc-inline-code) {
    background: var(--color-bg-body); padding: 2px 6px; border-radius: 4px;
    font-family: var(--font-mono); font-size: 12px; color: var(--color-primary);
  }
  .doc-content :global(.code-block-inline) {
    background: var(--color-bg-card); margin: 10px 0; padding: 12px 16px;
    border-radius: 6px; font-family: var(--font-mono); font-size: 12px;
    line-height: 1.6; overflow-x: auto; border: 1px solid var(--color-border);
  }
  .doc-content :global(.code-block-inline code) {
    color: var(--color-fg-1);
  }

  .code-container {
    margin-top: 14px; border: 1px solid var(--color-border);
    border-radius: var(--radius-sm); overflow: hidden;
  }
  .code-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: 8px 14px; background: var(--color-bg-body);
    border-bottom: 1px solid var(--color-border);
  }
  .code-copy-btn {
    display: flex; align-items: center; gap: 4px; padding: 4px 8px;
    border: none; background: transparent; color: var(--color-fg-3);
    cursor: pointer; border-radius: 4px; font-size: 11px; transition: var(--transition);
  }
  .code-copy-btn:hover { background: var(--color-border-light); color: var(--color-fg-1); }
  .code-block {
    padding: 16px; margin: 0; background: var(--color-bg-card);
    overflow-x: auto; font-family: var(--font-mono);
    font-size: 12px; line-height: 1.7; color: var(--color-fg-1);
  }
</style>
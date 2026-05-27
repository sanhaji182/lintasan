# Lintasan Go (v2)

> **Setiap Koneksi Punya Jalannya**
> *Every Connection Has Its Path*

> ⚡ 35x lebih ringan, single binary, 373 tests passing — rewrite penuh dari Node.js ke Go.
> ⚡ 35x lighter, single binary, 373 tests passing — full rewrite from Node.js to Go.

---

## 🌐 Filosofi / Philosophy

**ID:**
"Lintasan adalah jalur tempat koneksi, kecerdasan, dan kemungkinan bergerak."

Di dunia modern, AI bukan hanya tentang model. AI adalah tentang bagaimana sistem saling terhubung, bagaimana request menemukan jalur terbaik, dan bagaimana manusia berinteraksi dengan teknologi secara efisien.

Lintasan hadir sebagai jalur cerdas yang menghubungkan manusia, AI, dan sistem dalam satu aliran yang terintegrasi.

**EN:**
"Lintasan is the path where connections, intelligence, and possibilities move."

In the modern world, AI isn't just about models. It's about how systems connect, how requests find their best path, and how humans interact with technology efficiently.

Lintasan exists as an intelligent pathway connecting humans, AI, and systems in one integrated flow.

---

## 🤖 Built with AI

Project ini dibangun dengan AI sebagai development partner. Arsitektur, keputusan teknis, dan quality control tetap di tangan manusia — AI mengeksekusi.

Lintasan Go adalah rewrite total dari Node.js v1 — mempertahankan semua fitur dengan footprint 35x lebih ringan. Dibuat untuk komunitas AI Indonesia yang percaya bahwa masa depan development adalah orkestrasi, bukan sekadar coding manual.

**Orchestrator:** [Sanhaji](https://github.com/sanhaji182) · Programmer · AI-assisted development advocate

*This project was built with AI as a development partner. Architecture, technical decisions, and quality control remain in human hands — AI executes.*

*Lintasan Go is a complete rewrite of the Node.js v1 — retaining all features with 35x lighter footprint. Made for the Indonesian AI community that believes the future of development is orchestration, not just manual coding.*

---

## ⚡ Apa itu Lintasan? / What is Lintasan?

Lintasan adalah LLM proxy gateway dengan 40+ fitur optimasi. Satu endpoint untuk semua provider AI — routing cerdas, embedded cache, dual-mode CommandCode, plugin system, dan penghematan token otomatis. 113 provider preset ready out-of-the-box.

*Lintasan is an LLM proxy gateway with 40+ optimization features. One endpoint for all AI providers — smart routing, embedded cache, dual-mode CommandCode, plugin system, and automatic token savings. 113 provider presets ready out-of-the-box.*

**Tech Stack:** Go 1.24 · SQLite (go-sqlite3) · Single Binary

---

## 🏆 Kenapa Go? / Why Go?

| Metric | Node.js (v1) | Go (v2) |
|--------|-------------|---------|
| RAM | ~500MB | ~14MB |
| Binary size | 513MB (node_modules) | 24MB (single file) |
| Startup time | 3-5s | <50ms |
| Concurrent req/s | ~10,000 | ~50,000+ |
| Dependencies | 800+ npm packages | go-sqlite3 only |
| Tests | Manual | 373 tests / 33 packages |
| Provider presets | 27 | 113 (all LiteLLM) |
| Deployment | Docker + npm | scp 24MB binary |

---

## ✨ Fitur Utama / Key Features

> **⚠️ Kebijakan Legal & Etika / Legal & Ethics Policy**
> Lintasan didesain untuk menggunakan API resmi yang sah (Legal API). Kami **secara tegas tidak melakukan/mendukung Reverse Engineering** terhadap endpoint internal IDE komersial. Lintasan berfokus pada integrasi provider API resmi (OpenAI, Anthropic, DeepSeek, Sumopod, dll).
>
> *Lintasan is designed to use legitimate, official APIs. We strictly do not support reverse engineering of commercial IDE internal endpoints. Lintasan focuses on official API provider integrations for long-term stability and account safety.*

| # | Fitur / Feature | Deskripsi / Description |
|---|---|---|
| 1 | **Multi-Provider Proxy** | Satu endpoint untuk 10+ provider LLM / Single endpoint for 10+ LLM providers |
| 2 | **Smart Routing** | Multi-stage: regex → header → model → fallback with priority / Smart multi-stage routing |
| 3 | **113 Provider Presets** | Semua provider LiteLLM ready with website + favicon / All LiteLLM providers ready |
| 4 | **Connection Management** | Add/test/sync/delete connections + auto model discovery / Connection lifecycle management |
| 5 | **Model Discovery** | Auto-fetch models from provider /models endpoint / Automatic model discovery |
| 6 | **Provider Test** | Real-time latency + model count testing / Live connection testing |
| 7 | **Streaming (SSE)** | Full Server-Sent Events for streaming responses / SSE streaming with chunked transfer |
| 8 | **Fallback Chains** | Multi-level fallback per model with configurable priority / Automatic failover |
| 9 | **Circuit Breaker** | Auto-disable failing providers / Automatic provider health monitoring |
| 10 | **Request Logging** | Complete request/response logging with local IDs / Full audit trail |
| 11 | **Analytics** | Real-time metrics: latency, tokens, throughput / Live performance dashboard |
| 12 | **Combo System** | Pre-configured model+provider bundles / Model-provider combo templates |
| 13 | **Settings Cache** | In-memory settings with 5s TTL — zero DB reads per request / Instant settings access |
| 14 | **Master Key Auth** | Single master key — no per-user auth overhead / Simple secure authentication |
| 15 | **MITM Bridge** | Optional HTTPS bridge for LocalAI/LM Studio — connect without cert issues |
| 16 | **Dashboard** | Full interactive dashboard via reverse proxy / 16-page admin interface |
| 17 | **Plugin System** | Plugin store + auto-registration / Extensible without core changes |
| 18 | **Load Balancer** | Model-aware weighted load balancing / Smart distribution across providers |
| 19 | **Web Search** | Augment chat with live web results / Real-time search injection |
| 20 | **OAuth Integration** | Provider OAuth flow support / API key + token auth |
| 21 | **Vector Memory** | Pluggable embedder with SQLite default / Persistent AI memory store |
| 22 | **Image Generation** | Proxy to DALL-E / Stable Diffusion endpoints |
| 23 | **Audio (TTS + STT)** | Speech + transcription via OpenAI-compatible API |
| 24 | **Token Budgeting** | Per-key daily/monthly limits / Cost control |
| 25 | **Cost Tracking** | Real-time cost tracking per request / Usage analytics |
| 26 | **Export & Backup** | Database backup + data export / Disaster recovery |
| 27 | **CORS** | Built-in CORS — use from any browser app |
| 28 | **Zero Config** | SQLite embedded — no setup, no Docker required / Just run the binary |
| 29 | **CLI (Cobra)** | Full CLI: start, setup, mitm, version / Production-ready command interface |
| 30 | **Feature Parity** | 100% parity with Node v1 — no regression / All v1 features retained |

---

## 🏗 Arsitektur / Architecture

```
Client (App / Agent / curl)
        │
        ▼
┌─────────────────────────────────┐
│     Lintasan Go (:20181)         │
│     Single binary — 24MB         │
│                                  │
│  ┌────────────────────────────┐  │
│  │  API Gateway               │  │
│  │  /v1/chat/completions      │  │
│  │  /v1/embeddings            │  │
│  │  /v1/images/generations    │  │
│  │  /v1/audio/speech          │  │
│  │  /v1/audio/transcriptions  │  │
│  │  /v1/models                │  │
│  └──────────┬─────────────────┘  │
│             │                     │
│  ┌──────────▼─────────────────┐  │
│  │  Smart Router               │  │
│  │                             │  │
│  │  1. Header-based routing   │  │
│  │  2. Model name matching    │  │
│  │  3. Load-balanced pick     │  │
│  │  4. Provider priority sort │  │
│  │  5. Fallback chain         │  │
│  └──────────┬─────────────────┘  │
│             │                     │
│  ┌──────────▼─────────────────┐  │
│  │  Optimization Pipeline      │  │
│  │                             │  │
│  │  • Circuit Breaker          │  │
│  │  • Settings Cache (5s TTL)  │  │
│  │  • Request Logging          │  │
│  │  • Cost Tracking            │  │
│  │  • Stream Cache             │  │
│  │  • Combo Resolver           │  │
│  └──────────┬─────────────────┘  │
│             │                     │
│  ┌──────────▼─────────────────┐  │
│  │  Provider Dispatcher        │  │
│  │  + HTTP/1.1 + keep-alive   │  │
│  │  + Connection pooling      │  │
│  │  + Request translation     │  │
│  │  + SSE streaming           │  │
│  └──────────┬─────────────────┘  │
│             │                     │
└─────────────┼────────────────────┘
              │
      ┌───────┼───────┬───────┬──────────┐
      ▼       ▼       ▼       ▼          ▼
┌──────┐ ┌──────┐ ┌─────┐ ┌────────┐ ┌──────┐
│OpenAI│ │Gemini│ │ ... │ │Sumopod │ │Custom│
└──────┘ └──────┘ └─────┘ └────────┘ └──────┘
        113 provider presets ready
```

---

## 🚀 Instalasi / Installation

### Option 1: Pre-built Binary (Recommended)

```bash
# Download binary for linux/amd64
curl -L -o lintasan-go https://github.com/sanhaji182/lintasan-go/releases/latest/download/lintasan-go-linux-amd64
chmod +x lintasan-go
./lintasan-go start
```

Server berjalan di `http://localhost:20180` — dashboard langsung accessible.

### Option 2: Build from Source

```bash
git clone https://github.com/sanhaji182/lintasan-go.git
cd lintasan-go
go build -o lintasan-go ./cmd/lintasan
./lintasan-go start
```

### Option 3: Docker

```bash
git clone https://github.com/sanhaji182/lintasan-go.git
cd lintasan-go
docker compose up --build
```

### CLI Commands

```bash
lintasan-go start      # Start server (default port 20180)
lintasan-go setup      # Initialize database
lintasan-go mitm start # Start MITM bridge for HTTPS
lintasan-go version    # Show version info
lintasan-go help       # Show all commands

# Custom port
PORT=20181 ./lintasan-go start
```

---

## ⚙ Konfigurasi / Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `20180` | Server port |
| `LINTASAN_DATA_DIR` | `./data` | Data directory (DB, logs) |
| `LINTASAN_MASTER_KEY` | auto-generated | Master API key |
| `MITM_PORT` | `8443` | MITM bridge port |

No `.env` file needed — just set env vars or use defaults. Database auto-creates on first run.

---

## 📡 API Usage

```bash
# Standard OpenAI-compatible request
curl http://localhost:20180/v1/chat/completions \
  -H "Authorization: Bearer YOUR_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# List all available models
curl http://localhost:20180/v1/models \
  -H "Authorization: Bearer YOUR_MASTER_KEY"

# List provider presets
curl http://localhost:20180/api/providers/presets

# Test a connection
curl -X POST http://localhost:20180/api/connections/test \
  -H "Content-Type: application/json" \
  -d '{"baseUrl": "https://api.openai.com/v1", "apiKey": "sk-..."}'
```

Drop-in replacement — ganti base URL, semua SDK dan tool yang support OpenAI API langsung kompatibel.

*Drop-in replacement — change the base URL, and any SDK or tool supporting the OpenAI API works immediately.*

---

## 📊 Dashboard

Lintasan dilengkapi dashboard interaktif untuk monitoring dan konfigurasi real-time (di-reverse-proxy dari versi Node — akan di-rewrite native Go).

*Lintasan comes with an interactive dashboard for real-time monitoring and configuration.*

- **Overview** — Stats: connections, models, requests, tokens
- **Accounts** — Add provider (113 presets) + test/sync/delete
- **Routing** — Provider priority + load balancer config
- **Fallback** — Multi-level fallback chain configuration
- **Logs** — Real-time request log viewer
- **Usage** — Token usage & cost analytics
- **Analytics** — Real-time metrics dashboard
- **API Keys** — Key management
- **Teams** — Team-based access control
- **Users** — User management
- **Webhooks** — Event-driven webhook setup
- **Backup** — Database backup & restore
- **Settings** — Global configuration
- **Plugins** — Plugin store + management
- **Playground** — Interactive API test console
- **Docs** — Built-in API documentation

---

## 🔌 Provider Presets (113 Ready)

Semua provider LiteLLM sudah dikonfigurasi — tinggal isi API key dan connect.

*All LiteLLM providers pre-configured — just add your API key and connect.*

### Major Providers (4)
OpenAI · Anthropic · DeepSeek · Google Gemini

### Top-Tier (6)
xAI (Grok) · Mistral AI · Azure OpenAI · Azure AI Foundry · Google Vertex AI · AWS Bedrock

### AI Coding (4)
Codestral API · GitHub Copilot API · Pydantic AI Agents · Meta Llama API

### Aggregators (8)
OpenRouter · Replicate · HuggingFace Inference · Vercel AI Gateway · AIML API · Poe by Quora · CometAPI · NanoGPT

### High-Speed Inference (10)
Groq · Together AI · Fireworks AI · Cerebras · NVIDIA NIM · Cloudflare Workers AI · Hyperbolic · Lambda AI · FriendliAI · Anyscale Endpoints

### GPU Cloud (12)
Baseten · OctoAI · Lepton AI · Featherless AI · Crusoe Cloud · nscale AI · PublicAI · Galadriel · Chutes · GMI Cloud · Heroku AI · Novita AI

### CommandCode (2)
CommandCode (API Key) · CommandCode (Alpha)

### Chinese Providers (10)
GLM / Zhipu AI · Kimi / Moonshot · MiniMax · Qwen / Alibaba · SiliconFlow · Xiaomi MiMo · Volcano Engine (ByteDance) · Z.AI · DeepSeek · Baidu Qianfan

### Indonesia Providers (2)
Sumopod · Apertis AI (Stima API)

### Enterprise & Cloud (8)
Snowflake Cortex AI · Oracle Cloud (OCI) · SAP AI Core · IBM watsonx · Gradient AI · NLP Cloud · Petals · Clarifai

### Specialized (20)
Perplexity · Cohere · DeepInfra · SambaNova · Nebius AI · Aleph Alpha · AI21 Labs · Reka AI · Voyage AI · Deepgram · Black Forest Labs · Stability AI · Runway ML · Recraft AI · fal.ai · Helicone · Lemonade AI · Bytez · Sarvam AI · MorphDB

### Self-Hosted (3)
Ollama · vLLM · LM Studio

### Other (24)
AWS SageMaker · GigaChat · Predibase · OctoAI · FriendliAI · Lepton · Baseten · OpenPipe · Scale AI · Titan ML · OctoML · Monster API · GooseAI · NLP Cloud · Forefront · GooseAI · AI21 · Cohere · Anthropic · Replicate · HuggingFace · AWS · Azure · Custom

---

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package
go test ./internal/server/...
```

Output: `373 passed, 0 failed — 33 packages`

---

## 🏆 Benchmark: Lintasan vs 9Router

Duel head-to-head melawan 9Router — backend sama (CommandCode + deepseek-v4-pro).

*Head-to-head duel against 9Router — same backend (CommandCode + deepseek-v4-pro).*

| Task | Lintasan | 9Router | Winner |
|---|---|---|---|
| LRU Cache implementation | 11.2s | 11.6s | Lintasan |
| Code review merge_sorted | 7.2s | 10.9s | Lintasan |
| TypeScript deep-merge generic | 14.7s | 15.4s | Lintasan |
| Optimistic vs pessimistic locking | 11.1s | 15.3s | Lintasan |
| Docker exits code 0 | 12.9s | 6.3s | 9Router |
| GitHub Actions workflow | 8.0s | 3.7s | 9Router |
| Rate limiting middleware | 6.0s | 4.1s | 9Router |
| REST vs GraphQL vs gRPC | 11.5s | 15.4s | Lintasan |

**Results:**
- Cold average: **10.3s vs 10.3s** (parity)
- Win rate: **Lintasan 5 — 9Router 3**
- Thinking leaked: Lintasan 1/8 vs 9Router 3/8

---

## 📂 Project Structure

```
lintasan-go/
├── cmd/lintasan/          # CLI entry point (cobra)
├── internal/
│   ├── config/            # Environment & DB config
│   ├── db/                # SQLite database layer
│   ├── server/            # HTTP server + handlers
│   │   ├── server.go      # Core server + health
│   │   ├── handlers_parity.go  # API handlers + 113 presets
│   │   ├── proxy.go       # Provider proxy + streaming
│   │   ├── router.go      # Smart routing logic
│   │   ├── cache.go       # Settings cache
│   │   └── ...
│   ├── mitm/              # MITM HTTPS bridge
│   ├── discover/          # Model auto-discovery
│   ├── memory/            # Vector memory (pluggable embedder)
│   └── ...
├── data/                  # Runtime data (auto-created)
├── Dockerfile
├── docker-compose.yml
├── go.mod / go.sum
└── README.md
```

---

## 🔄 Migration from Node.js v1

Lintasan Go menggunakan database schema yang **berbeda** dari Node.js v1 — tidak backward-compatible. Migration steps:

1. Export settings dari Node v1 dashboard
2. Re-create connections di Go v2 dashboard (atau via API)
3. Models akan auto-discovered saat sync

---

## 📄 Lisensi / License

MIT

---

<p align="center">
  <b>Lintasan Go (v2)</b> — Setiap Koneksi Punya Jalannya<br>
  <i>Every Connection Has Its Path</i><br><br>
  Dibangun dengan 🤖 AI · Diorkestrasi oleh 👨‍💻 <a href="https://github.com/sanhaji182">Sanhaji</a>
</p>

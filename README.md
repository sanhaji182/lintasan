# 🇮🇩 Lintasan

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Version](https://img.shields.io/badge/version-v0.29.3-blue)](https://github.com/sanhaji182/lintasan/releases)
[![Tests](https://img.shields.io/badge/tests-1021+-success)](.)
[![Docker](https://img.shields.io/badge/docker-ready-2496ED?logo=docker)](docker-compose.yml)

> **Setiap Koneksi Punya Jalannya** — Satu endpoint OpenAI-compatible untuk semua provider AI. Smart routing, failover, caching, dan dashboard monitoring dalam satu binary ~26MB.

> ⚡ 35× lebih ringan dari Node.js · single binary ~26MB · 1021+ tests · 25 halaman dashboard · Go + SvelteKit Embedded SPA

---

## 🇬🇧 Lintasan

> **Every Connection Has Its Path** — One OpenAI-compatible endpoint for all AI providers. Smart routing, failover, caching, and dashboard monitoring in a single ~26MB binary.

> ⚡ 35× lighter than Node.js · single ~26MB binary · 1021+ tests · 25 dashboard pages · Go + SvelteKit Embedded SPA

---

## 🌐 Filosofi / Philosophy

<details open>
<summary>🇮🇩 Bahasa Indonesia</summary>

"Lintasan adalah jalur tempat koneksi, kecerdasan, dan kemungkinan bergerak."

Di dunia modern, AI bukan hanya tentang model. AI adalah tentang bagaimana sistem saling terhubung, bagaimana request menemukan jalur terbaik, dan bagaimana manusia berinteraksi dengan teknologi secara efisien.

Lintasan hadir sebagai jalur cerdas yang menghubungkan manusia, AI, dan sistem dalam satu aliran yang terintegrasi. Satu endpoint untuk semua provider AI — tidak perlu lagi berganti-ganti SDK atau mengelola banyak API key di berbagai tempat.

</details>

<details>
<summary>🇬🇧 English</summary>

"Lintasan is the path where connections, intelligence, and possibilities move."

In the modern world, AI isn't just about models. It's about how systems connect, how requests find their best path, and how humans interact with technology efficiently.

Lintasan exists as an intelligent pathway connecting humans, AI, and systems in one integrated flow. One endpoint for all AI providers — no more switching SDKs or managing multiple API keys across different places.

</details>

---

## 📖 Daftar Isi / Table of Contents

- [Apa itu Lintasan? / What is Lintasan?](#-apa-itu-lintasan--what-is-lintasan)
- [Kenapa Go? / Why Go?](#-kenapa-go--why-go)
- [Tech Stack](#-tech-stack)
- [Fitur Utama / Key Features](#-fitur-utama--key-features)
- [Arsitektur / Architecture](#-arsitektur--architecture)
- [Quick Start](#-quick-start)
- [Instalasi / Installation](#-instalasi--installation)
- [Konfigurasi / Configuration](#-konfigurasi--configuration)
- [API Usage](#-api-usage)
- [Dashboard](#-dashboard)
- [Provider Presets](#-provider-presets)
- [Struktur Project / Project Structure](#-struktur-project--project-structure)
- [Development](#-development)
- [Testing](#-testing)
- [Deployment](#-deployment)
- [Benchmark](#-benchmark)
- [Contributing](#-contributing)
- [Lisensi / License](#-lisensi--license)

---

## ❓ Apa itu Lintasan? / What is Lintasan?

<details open>
<summary>🇮🇩 Bahasa Indonesia</summary>

Lintasan adalah **LLM proxy gateway** — satu endpoint OpenAI-compatible untuk semua provider AI. Routing cerdas, failover otomatis, embedded cache, token compression, dan dashboard monitoring real-time.

**Masalah yang diselesaikan:**
- 🔀 **Multi-provider complexity** — ganti provider = ganti SDK, ganti API key, ganti format
- 💸 **No cost visibility** — tidak tahu berapa token terpakai per model/provider
- 🔄 **No failover** — satu provider down, semua request gagal
- 🔐 **Key management chaos** — API key tersebar di mana-mana
- 📊 **No observability** — tidak bisa lihat usage, latency, error rate

**Solusi Lintasan:**
- Satu endpoint → semua provider (OpenAI, Anthropic, DeepSeek, Gemini, Groq, dll)
- Satu API key → autentikasi terpusat
- Smart routing → request otomatis ke provider terbaik
- Circuit breaker → provider gagal auto-disable
- Grouped connections → kelompokkan akun per provider dengan load balancing
- Dashboard → monitoring real-time 25 halaman

</details>

<details>
<summary>🇬🇧 English</summary>

Lintasan is an **LLM proxy gateway** — one OpenAI-compatible endpoint for all AI providers. Smart routing, automatic failover, embedded cache, token compression, and real-time dashboard monitoring.

**Problems solved:**
- 🔀 **Multi-provider complexity** — switching providers means switching SDKs, API keys, and formats
- 💸 **No cost visibility** — can't see token usage per model/provider
- 🔄 **No failover** — one provider down, all requests fail
- 🔐 **Key management chaos** — API keys scattered everywhere
- 📊 **No observability** — can't see usage, latency, error rates

**Lintasan's solution:**
- One endpoint → all providers (OpenAI, Anthropic, DeepSeek, Gemini, Groq, etc.)
- One API key → centralized authentication
- Smart routing → requests automatically go to the best provider
- Circuit breaker → failing providers auto-disable
- Grouped connections → group accounts per provider with load balancing
- Dashboard → 25-page real-time monitoring

</details>

---

## 💪 Kenapa Go? / Why Go?

| Metric | Node.js (legacy) | Go (current) | Improvement |
|--------|------------------|--------------|-------------|
| **RAM** | ~500MB | ~14MB | **35x lebih hemat** |
| **Binary size** | 513MB (node_modules) | 26MB (single file) | **20x lebih kecil** |
| **Startup** | 3-5 detik | <50ms | **60-100x lebih cepat** |
| **Concurrent req/s** | ~10,000 | ~50,000+ | **5x throughput** |
| **Dependencies** | 800+ npm packages | 1 (go-sqlite3) | **800x lebih sedikit** |
| **Tests** | Manual | 1021+ / 43 packages | **Automated** |
| **Deployment** | Docker + npm install | `scp` 26MB binary | **Zero setup** |

---

## 🛠 Tech Stack

| Layer | Technology | Notes |
|-------|-----------|-------|
| **Backend** | Go 1.22+ | HTTP server, routing, proxy, streaming |
| **Database** | SQLite (go-sqlite3) | Embedded, zero config, single-file |
| **Frontend** | SvelteKit 5 + TypeScript | Dashboard SPA, **embedded into Go binary via go:embed** |
| **Styling** | CSS variables + custom design system | Dark/light mode, responsive |
| **CLI** | Cobra | `start`, `setup`, `mitm`, `version` |
| **Testing** | Go standard library | 1021+ tests, 43 packages |
| **Deployment** | **Single self-contained binary** + systemd | UI + API in one executable; **no Node.js required** |

---

## ✨ Fitur Utama / Key Features

> **⚠️ Kebijakan Legal & Etika / Legal & Ethics Policy**
>
> **🇮🇩** Lintasan didesain untuk menggunakan **API resmi yang sah** (Legal API). Kami secara tegas **tidak melakukan/mendukung Reverse Engineering** terhadap endpoint internal IDE komersial. Lintasan berfokus pada integrasi provider API resmi dan Experimental Provider (ACP).
>
> **🇬🇧** Lintasan is designed to use **legitimate, official APIs**. We strictly do **not support reverse engineering** of commercial IDE internal endpoints. Lintasan focuses on official API provider integrations and Experimental Provider pipelines (ACP).

<details open>
<summary>🇮🇩 Fitur</summary>

| # | Fitur | Deskripsi |
|---|-------|-----------|
| 1 | **Multi-Provider Proxy** | Satu endpoint OpenAI-compatible untuk semua provider LLM |
| 2 | **Smart Routing** | Multi-stage: header → model → priority → fallback |
| 3 | **Grouped Connections** | Kelompokkan akun per provider, shared models viewer |
| 4 | **Connection Pool** | Load balancing round-robin + failover per pool |
| 5 | **Connection Management** | Add/test/sync/delete + cURL import + auto discovery |
| 6 | **Model Discovery** | Auto-fetch models dari provider /models endpoint |
| 7 | **Shared Models Viewer** | Lihat models ter-grouped dari semua akun dalam satu view |
| 8 | **Balance Checker** | Monitor credit balance & rate limits per akun |
| 9 | **Streaming (SSE)** | Full Server-Sent Events untuk streaming response |
| 10 | **Fallback Chains** | Multi-level fallback per model |
| 11 | **Circuit Breaker** | Auto-disable provider yang gagal |
| 12 | **Request Logging** | Complete request/response logging dengan filter |
| 13 | **Analytics** | Real-time metrics: latency, tokens, throughput |
| 14 | **Combo System** | Pre-configured model+provider bundles |
| 15 | **Load Balancer** | Model-aware weighted load balancing |
| 16 | **Plugin System** | Plugin store + auto-registration |
| 17 | **Vector Memory** | Pluggable embedder dengan SQLite default |
| 18 | **Web Search** | Augment chat dengan live web results |
| 19 | **OAuth IDE** | OAuth flow untuk IDE agents (Experimental) |
| 20 | **Image Generation** | Proxy ke DALL-E / Stable Diffusion |
| 21 | **Audio (TTS + STT)** | Speech + transcription via OpenAI API |
| 22 | **Cost Tracking** | Real-time cost tracking per request |
| 23 | **Token Compression** | RTK compressor untuk hemat token |
| 24 | **Semantic Cache** | Cosine similarity cache untuk response dedup |
| 25 | **API Keys** | Key management + usage tracking |
| 26 | **Teams & Users** | Multi-user access control dengan role |
| 27 | **Webhooks** | Event-driven webhook system |
| 28 | **Backup & Export** | Database backup + disaster recovery |
| 29 | **Dashboard** | 25 halaman interactive SvelteKit dashboard |
| 30 | **Playground** | Built-in API test console |
| 31 | **MCP Server** | Model Context Protocol server (14+ tools) |
| 32 | **Format Translator** | Terjemahan format API lintas provider |
| 33 | **CORS** | Built-in — use from any browser app |
| 34 | **Zero Config** | SQLite embedded — no setup required |
| 35 | **CLI (Cobra)** | `start`, `setup`, `mitm`, `version` |
| 36 | **MITM Bridge** | Optional HTTPS bridge untuk LocalAI/LM Studio |
| 37 | **Credential Vault** | AES-256-GCM encrypted credential storage |
| 38 | **Observability** | Exportable `/metrics` endpoint + dashboard panels |
| 39 | **Guardrails** | Input/output filtering untuk safety |

</details>

<details>
<summary>🇬🇧 Features</summary>

| # | Feature | Description |
|---|---------|-------------|
| 1 | **Multi-Provider Proxy** | One OpenAI-compatible endpoint for all LLM providers |
| 2 | **Smart Routing** | Multi-stage: header → model → priority → fallback |
| 3 | **Grouped Connections** | Group accounts per provider, shared models viewer |
| 4 | **Connection Pool** | Round-robin load balancing + failover per pool |
| 5 | **Connection Management** | Add/test/sync/delete + cURL import + auto discovery |
| 6 | **Model Discovery** | Auto-fetch models from provider /models endpoint |
| 7 | **Shared Models Viewer** | View grouped models from all accounts in one view |
| 8 | **Balance Checker** | Monitor credit balance & rate limits per account |
| 9 | **Streaming (SSE)** | Full Server-Sent Events for streaming responses |
| 10 | **Fallback Chains** | Multi-level fallback per model |
| 11 | **Circuit Breaker** | Auto-disable failing providers |
| 12 | **Request Logging** | Complete request/response logging with filters |
| 13 | **Analytics** | Real-time metrics: latency, tokens, throughput |
| 14 | **Combo System** | Pre-configured model+provider bundles |
| 15 | **Load Balancer** | Model-aware weighted load balancing |
| 16 | **Plugin System** | Plugin store + auto-registration |
| 17 | **Vector Memory** | Pluggable embedder with SQLite default |
| 18 | **Web Search** | Augment chat with live web results |
| 19 | **OAuth IDE** | OAuth flow for IDE agents (Experimental) |
| 20 | **Image Generation** | Proxy to DALL-E / Stable Diffusion |
| 21 | **Audio (TTS + STT)** | Speech + transcription via OpenAI API |
| 22 | **Cost Tracking** | Real-time cost tracking per request |
| 23 | **Token Compression** | RTK compressor for token savings |
| 24 | **Semantic Cache** | Cosine similarity cache for response dedup |
| 25 | **API Keys** | Key management + usage tracking |
| 26 | **Teams & Users** | Multi-user access control with roles |
| 27 | **Webhooks** | Event-driven webhook system |
| 28 | **Backup & Export** | Database backup + disaster recovery |
| 29 | **Dashboard** | 25-page interactive SvelteKit dashboard |
| 30 | **Playground** | Built-in API test console |
| 31 | **MCP Server** | Model Context Protocol server (14+ tools) |
| 32 | **Format Translator** | Cross-format API translation tool |
| 33 | **CORS** | Built-in — use from any browser app |
| 34 | **Zero Config** | SQLite embedded — no setup required |
| 35 | **CLI (Cobra)** | `start`, `setup`, `mitm`, `version` |
| 36 | **MITM Bridge** | Optional HTTPS bridge for LocalAI/LM Studio |
| 37 | **Credential Vault** | AES-256-GCM encrypted credential storage |
| 38 | **Observability** | Exportable `/metrics` endpoint + dashboard panels |
| 39 | **Guardrails** | Input/output filtering for safety |

</details>

---

## 🏗 Arsitektur / Architecture

```
Client (App / Agent / curl / IDE)
        │
        ▼
┌─────────────────────────────────────────────────────┐
│ Nginx (SSL Termination) — lintasan.sans.biz.id      │
│   * All traffic proxies to Go:20180                 │
└─────────────────────────┬───────────────────────────┘
                          │
         ┌────────────────▼──────────────────┐
         │ Go Backend :20180 (lintasan start) │
         │ ── Serves BOTH API & UI ──         │
         │ • Embedded SPA UI (go:embed)       │
         │ • OpenAI-compatible LLM proxy      │
         └──────────────┬────────────────────┘
                    │
         ┌──────────▼──────────────┐
         │  API Gateway            │
         │  /v1/chat/completions   │
         │  /v1/embeddings         │
         │  /v1/images/generations │
         │  /v1/audio/*            │
         │  /v1/models             │
         │  /v1/memory/*           │
         │  /metrics               │
         └──────────┬──────────────┘
                    │
         ┌──────────▼──────────────┐
         │  Smart Router           │
         │  1. Header-based        │
         │  2. Model name match    │
         │  3. Load-balanced pick  │
         │  4. Priority sort       │
         │  5. Fallback chain      │
         └──────────┬──────────────┘
                    │
         ┌──────────▼──────────────┐
         │  Optimization Pipeline  │
         │  • Circuit Breaker      │
         │  • Semantic Cache       │
         │  • Request Logging      │
         │  • Cost Tracking        │
         │  • Vector Context Inject│
         │  • Token Compression    │
         └──────────┬──────────────┘
                    │
         ┌──────────▼──────────────┐
         │  Provider Dispatcher    │
         │  + Connection Pool      │
         │  + HTTP/1.1 keep-alive  │
         │  + SSE streaming        │
         └──────────┬──────────────┘
                    │
         ┌─────────┼─────────┬──────────┐
         ▼         ▼         ▼          ▼
    ┌────────┐ ┌──────┐ ┌──────┐ ┌──────────┐
    │ OpenAI │ │Gemini│ │Groq  │ │Sumopod   │ ...
    └────────┘ └──────┘ └──────┘ └──────────┘
```

---

## 🚀 Quick Start

The dashboard UI is **embedded inside the binary**, so one executable serves the full app (UI + API) on `:20180` — no Node, no nginx.

```bash
# Download the pre-built binary and run
curl -L -o lintasan https://github.com/sanhaji182/lintasan/releases/latest/download/lintasan-linux-amd64
chmod +x lintasan
./lintasan start

# Dashboard → http://localhost:20180/dashboard
# API       → http://localhost:20180/v1/chat/completions
```

---

## 📦 Instalasi / Installation

3 methods — pick one:

<details open>
<summary>🇮🇩 3 Cara Install</summary>

### Opsi 1: Binary Pre-built (Recommended)
Satu binary, dashboard sudah termasuk di dalamnya.
```bash
curl -L -o lintasan https://github.com/sanhaji182/lintasan/releases/latest/download/lintasan-linux-amd64
chmod +x lintasan
./lintasan start
```

### Opsi 2: Build dari Source
Butuh **Go 1.22+** dan **Node 20+** (untuk build dashboard). `make build` meng-compile frontend SvelteKit jadi SPA statis, meng-embed-nya ke binary Go, lalu compile binary tunggal.
```bash
git clone https://github.com/sanhaji182/lintasan.git
cd lintasan
make build       # frontend → embed → ./lintasan
./lintasan start
```
> Tanpa Node? `go build -o lintasan ./cmd/lintasan` tetap jalan, tapi menghasilkan server **API-only** (tanpa UI dashboard).

### Opsi 3: Docker (single container)
```bash
git clone https://github.com/sanhaji182/lintasan.git
cd lintasan
LINTASAN_MASTER_KEY=$(openssl rand -hex 32) docker compose up --build
# UI + API → http://localhost:20180
```

### CLI Commands
```bash
./lintasan start      # Start server (UI + API, default :20180)
./lintasan setup      # Initialize database
./lintasan mitm start # Start MITM HTTPS bridge (optional)
./lintasan version    # Show version
./lintasan help       # All commands

# Custom port
PORT=8080 ./lintasan start
```

</details>

<details>
<summary>🇬🇧 3 Installation Methods</summary>

### Option 1: Pre-built Binary (Recommended)
One binary, dashboard included.
```bash
curl -L -o lintasan https://github.com/sanhaji182/lintasan/releases/latest/download/lintasan-linux-amd64
chmod +x lintasan
./lintasan start
```

### Option 2: Build from Source
Requires **Go 1.22+** and **Node 20+** (to build the dashboard). `make build` compiles the SvelteKit frontend into a static SPA, embeds it into the Go binary, and compiles a single executable.
```bash
git clone https://github.com/sanhaji182/lintasan.git
cd lintasan
make build       # frontend → embed → ./lintasan
./lintasan start
```
> No Node? `go build -o lintasan ./cmd/lintasan` still works but produces an **API-only** server (no dashboard UI).

### Option 3: Docker (single container)
```bash
git clone https://github.com/sanhaji182/lintasan.git
cd lintasan
LINTASAN_MASTER_KEY=$(openssl rand -hex 32) docker compose up --build
# UI + API → http://localhost:20180
```

### CLI Commands
```bash
./lintasan start      # Start server (UI + API, default :20180)
./lintasan setup      # Initialize database
./lintasan mitm start # Start MITM HTTPS bridge (optional)
./lintasan version    # Show version
./lintasan help       # All commands

# Custom port
PORT=8080 ./lintasan start
```

</details>

---

## ⚙ Konfigurasi / Configuration

Quick env reference:

<details open>
<summary>🇮🇩 Environment Variables</summary>

| Variable | Default | Keterangan |
|----------|---------|------------|
| `PORT` | `20180` | Port server utama |
| `LINTASAN_DATA_DIR` | `./data` | Direktori data (DB, logs) |
| `LINTASAN_MASTER_KEY` | auto-generated | Master API key |

Tidak perlu `.env` file — set env vars atau gunakan default. Database auto-create saat pertama run.

</details>

<details>
<summary>🇬🇧 Environment Variables</summary>

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `20180` | Main server port |
| `LINTASAN_DATA_DIR` | `./data` | Data directory (DB, logs) |
| `LINTASAN_MASTER_KEY` | auto-generated | Master API key |

No `.env` file needed — just set env vars or use defaults. Database auto-creates on first run.

</details>

---

## 📡 API Usage

```bash
# Chat completion (OpenAI-compatible)
curl http://localhost:20180/v1/chat/completions \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'

# List models
curl http://localhost:20180/v1/models \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY"

# Embeddings
curl http://localhost:20180/v1/embeddings \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "text-embedding-3-small", "input": "Hello world"}'

# Image generation
curl http://localhost:20180/v1/images/generations \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "dall-e-3", "prompt": "A sunset over mountains"}'

# Text-to-speech
curl http://localhost:20180/v1/audio/speech \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "tts-1", "input": "Hello world", "voice": "alloy"}'

# Speech-to-text
curl http://localhost:20180/v1/audio/transcriptions \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -F "file=@audio.mp3" -F "model=whisper-1"

# Vector memory search
curl "http://localhost:20180/v1/memory/search?q=hello&limit=5" \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY"

# Web search augmented chat
curl http://localhost:20180/v1/web/search \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o", "query": "latest AI news"}'
```

> Drop-in replacement untuk OpenAI API — ganti base URL, semua SDK (`openai-python`, `langchain`, `llama-index`, dll) langsung kompatibel.

---

## 📊 Dashboard

<details open>
<summary>🇮🇩 25 Halaman Dashboard</summary>

Lintasan dilengkapi dashboard interaktif berbasis **SvelteKit 5** untuk monitoring dan konfigurasi real-time. Embedded di binary Go — tanpa Node.js terpisah.

| Halaman | Fungsi |
|---------|--------|
| **Overview** | Statistik global — requests, tokens, cache hit rate, latency |
| **Connections** | Kelola koneksi provider — grouped by provider, pool, balance checker |
| **Providers** | Provider presets, one-click add + category management |
| **Discover** | Auto-discover free & public provider API |
| **Routing** | Konfigurasi combo + load balancer + aliases |
| **Fallback** | Multi-level fallback chain per model/connection |
| **Logs** | Real-time request log dengan filter & search |
| **Usage** | Token usage + cost per provider/model |
| **Observability** | Exportable `/metrics` + real-time panels |
| **Analytics** | Metrics dashboard — latency, throughput, savings |
| **Cost Savings** | Breakdown penghematan token & cache |
| **Memory** | Vector memory untuk RAG (SQLite) |
| **API Keys** | Generate, copy, revoke API keys |
| **Teams** | Team-based access control |
| **Users** | User management + role assignment |
| **Webhooks** | Event-driven webhook setup & testing |
| **Backup** | Database backup, restore, export |
| **Settings** | Global configuration — port, keys, limits |
| **Experimental** | Laboratorium fitur eksperimental |
| **OAuth IDE** | OAuth flow untuk IDE agents (Experimental) |
| **Plugins** | Plugin store + management |
| **Playground** | Interactive chat console untuk testing API |
| **MCP Server** | Model Context Protocol server (14+ tools) |
| **Format Translator** | Terjemahan format API lintas provider |
| **Docs** | Dokumentasi API built-in |

Akses: `http://localhost:20180/dashboard`

</details>

<details>
<summary>🇬🇧 25 Dashboard Pages</summary>

Lintasan comes with an interactive dashboard built with **SvelteKit 5** for real-time monitoring and configuration. Embedded in the Go binary — no separate Node process.

| Page | Function |
|------|----------|
| **Overview** | Global stats — requests, tokens, cache hit rate, latency |
| **Connections** | Manage provider connections — grouped by provider, pool, balance checker |
| **Providers** | Provider presets, one-click add + category management |
| **Discover** | Auto-discover free & public provider APIs |
| **Routing** | Smart-route configs: combo, load balancer, aliases |
| **Fallback** | Multi-level fallback chains per model/connection |
| **Logs** | Real-time request log with filter & search |
| **Usage** | Token usage & cost per provider/model |
| **Observability** | Exportable `/metrics` + real-time panels |
| **Analytics** | Metrics dashboard — latency, throughput, savings |
| **Cost Savings** | Token compression & cache savings breakdown |
| **Memory** | Vector memory for RAG (SQLite) |
| **API Keys** | Generate, copy, revoke API keys |
| **Teams** | Team-based access control |
| **Users** | User management + role assignment |
| **Webhooks** | Event-driven webhook setup & testing |
| **Backup** | Database backup, restore, export |
| **Settings** | Global configuration — port, keys, limits |
| **Experimental** | Experimental features lab |
| **OAuth IDE** | OAuth flow for IDE agents (Experimental) |
| **Plugins** | Plugin store + management |
| **Playground** | Interactive chat console for API testing |
| **MCP Server** | Model Context Protocol server (14+ tools) |
| **Format Translator** | Cross-format API translation tool |
| **Docs** | Built-in API documentation |

Access: `http://localhost:20180/dashboard`

</details>

---

## 🔌 Provider Presets

Lintasan ships with built-in provider presets and supports custom providers. Major supported providers:

| Category | Providers |
|----------|-----------|
| **Major** | OpenAI · Anthropic · DeepSeek · Google Gemini |
| **Top-Tier** | xAI (Grok) · Mistral AI · Azure OpenAI · Google Vertex AI · AWS Bedrock |
| **Aggregators** | OpenRouter · Replicate · HuggingFace |
| **High-Speed** | Groq · Together AI · Fireworks AI · Cerebras · NVIDIA NIM |
| **Chinese** | GLM/Zhipu · Kimi/Moonshot · MiniMax · Qwen/Alibaba · SiliconFlow |
| **Indonesia** | Sumopod · Apertis AI |
| **Self-Hosted** | Ollama · vLLM · LM Studio |
| **Specialized** | Perplexity · Cohere · DeepInfra · Stability AI · fal.ai |

Plus any custom OpenAI-compatible endpoint — just provide a base URL and API key.

---

## 📂 Struktur Project / Project Structure

```
lintasan-go/
├── cmd/lintasan/              # CLI entry point (Cobra)
├── internal/                  # 43 Go packages
│   ├── auth/                  # JWT auth, password hashing, user CRUD
│   ├── server/                # HTTP mux, 70+ routes, middleware
│   ├── proxy/                 # Core LLM proxy: chat, embeddings, images, audio
│   ├── config/                # Loading & validation
│   ├── db/                    # SQLite schema + migrations
│   ├── cache/                 # Semantic cache (cosine similarity)
│   ├── circuit/               # Circuit breaker
│   ├── combo/                 # Routing combos
│   ├── cost/                  # Cost tracking per request
│   ├── fallback/              # Fallback chain antar provider
│   ├── lb/                    # Load balancing
│   ├── logging/               # Request logging
│   ├── mcp/                   # MCP server (JSON-RPC 2.0, 14 tools)
│   ├── memory/                # Vector memory (search/store/stats)
│   ├── models/                # Model catalog & metadata
│   ├── plugin/                # Plugin system
│   ├── guard/                 # Guardrails (input/output filtering)
│   ├── compress/              # Token compression
│   ├── translator/            # Cross-format translation
│   ├── webhook/               # Webhook subscriptions
│   ├── websearch/             # Web search integration
│   ├── mlrouter/              # ML-based smart routing
│   ├── rtk/                   # RTK token compressor
│   ├── web/                   # Embedded SPA (go:embed)
│   └── ...                    # 43 packages total
├── frontend/                  # SvelteKit 5 dashboard
│   ├── src/
│   │   ├── routes/            # 25 dashboard page routes
│   │   ├── lib/               # Components, stores, API client
│   │   └── app.css            # Design tokens (CSS vars)
│   └── svelte.config.js       # adapter-static for go:embed
├── data/                      # Runtime data (SQLite, auto-created)
├── docs/                      # Documentation
├── Makefile                   # Build automation
├── go.mod / go.sum            # Go dependencies
├── Dockerfile                 # Docker build
├── docker-compose.yml         # Docker compose
└── README.md                  # This file
```

---

## 💻 Development

```bash
# Clone
git clone https://github.com/sanhaji182/lintasan.git
cd lintasan

# Build all (frontend → embed → binary)
make build

# Run
./lintasan start               # UI + API on :20180

# Frontend dev (with HMR, requires Go backend running)
cd frontend && npm run dev -- --port 5173

# Backend only (API-only, no dashboard)
go run ./cmd/lintasan start
```

---

## 🧪 Testing

```bash
# All backend tests
go test ./...                  # 1021+ tests, 43 packages

# With coverage
go test -cover ./...

# Specific package
go test -v ./internal/server/...

# Frontend type-check
cd frontend && npm run check
```

---

## 🚢 Deployment

<details open>
<summary>🇮🇩 Production Deployment</summary>

Lintasan deploy sebagai single binary. Dashboard embedded di dalamnya — tidak perlu proses Node terpisah.

```bash
# Build
make build                     # frontend → embed → ./lintasan

# Copy to server
scp lintasan user@server:/opt/lintasan/

# Systemd service
sudo tee /etc/systemd/system/lintasan.service << 'EOF'
[Unit]
Description=Lintasan LLM Proxy Gateway
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/lintasan
ExecStart=/opt/lintasan/lintasan start
Restart=on-failure
RestartSec=5
Environment=PORT=20180

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now lintasan
```

### Nginx Reverse Proxy (optional)

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    location / {
        proxy_pass http://127.0.0.1:20180;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

</details>

<details>
<summary>🇬🇧 Production Deployment</summary>

Lintasan deploys as a single binary. The dashboard is embedded inside — no separate Node process needed.

```bash
# Build
make build                     # frontend → embed → ./lintasan

# Copy to server
scp lintasan user@server:/opt/lintasan/

# Systemd service
sudo tee /etc/systemd/system/lintasan.service << 'EOF'
[Unit]
Description=Lintasan LLM Proxy Gateway
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/lintasan
ExecStart=/opt/lintasan/lintasan start
Restart=on-failure
RestartSec=5
Environment=PORT=20180

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now lintasan
```

### Nginx Reverse Proxy (optional)

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    location / {
        proxy_pass http://127.0.0.1:20180;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

</details>

---

## 🏆 Benchmark

Head-to-head melawan 9Router — backend sama (CommandCode + deepseek-v4-pro).

| Task | Lintasan | 9Router | Winner |
|------|----------|---------|--------|
| LRU Cache implementation | 11.2s | 11.6s | Lintasan |
| Code review merge_sorted | 7.2s | 10.9s | Lintasan |
| TypeScript deep-merge generic | 14.7s | 15.4s | Lintasan |
| Optimistic vs pessimistic locking | 11.1s | 15.3s | Lintasan |
| Docker exits code 0 | 12.9s | 6.3s | 9Router |
| GitHub Actions workflow | 8.0s | 3.7s | 9Router |
| Rate limiting middleware | 6.0s | 4.1s | 9Router |
| REST vs GraphQL vs gRPC | 11.5s | 15.4s | Lintasan |

**Results:** Cold avg 10.3s vs 10.3s (parity) · **Lintasan 5 — 9Router 3**

---

## 🤝 Contributing

Contributions welcome! Open an issue for bug reports or feature requests.

1. Fork the repo
2. Create a branch (`git checkout -b feature/amazing-feature`)
3. Commit changes (`git commit -m 'feat: amazing feature'`)
4. Push to branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

```bash
go test ./...     # Must be 0 failures
```

---

## 📄 Lisensi / License

MIT

---

## 🤖 Built with AI

This project was built with AI as a development partner. Architecture, technical decisions, and quality control remain in human hands — AI executes.

**Orchestrator:** [Sanhaji](https://github.com/sanhaji182) · Programmer · AI-assisted development advocate

---

<p align="center">
  <b>🇮🇩 Lintasan</b> — Setiap Koneksi Punya Jalannya<br>
  <b>🇬🇧 Lintasan</b> — Every Connection Has Its Path<br><br>
  Dibangun dengan 🤖 AI · Diorkestrasi oleh 👨‍💻 <a href="https://github.com/sanhaji182">Sanhaji</a>
</p>

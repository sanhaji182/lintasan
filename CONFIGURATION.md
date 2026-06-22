# 🇮🇩 Lintasan — Konfigurasi

> Panduan lengkap konfigurasi Lintasan Go: environment variables, dashboard settings, connection setup, dan advanced config.

---

## 🇬🇧 Lintasan — Configuration

> Complete configuration guide: environment variables, dashboard settings, connection setup, and advanced config.

---

## 📋 Daftar Isi / Table of Contents

- [Environment Variables](#-environment-variables)
- [Dashboard Settings](#-dashboard-settings)
- [Connection Setup](#-connection-setup)
- [Provider Presets](#-provider-presets)
- [Routing Configuration](#-routing-configuration)
- [Security](#-security)
- [Advanced](#-advanced)

---

## 🔧 Environment Variables

Lintasan dikonfigurasi melalui environment variables. **Tidak perlu file `.env`** — set langsung atau gunakan default.

### Server

| Variable | Default | Deskripsi |
|----------|---------|-----------|
| `PORT` | `20180` | Port server utama (UI + API) |
| `LINTASAN_DATA_DIR` | `./data` | Direktori data — database SQLite, logs |
| `LINTASAN_MASTER_KEY` | auto-generated | Master API key. **WAJIB set untuk production** |

### Optional

| Variable | Default | Deskripsi |
|----------|---------|-----------|
| `MITM_PORT` | `8443` | Port MITM HTTPS bridge |
| `LINTASAN_MITM_ENABLED` | `false` | Aktifkan MITM bridge |
| `LINTASAN_OAUTH_IDE_ENABLED` | `false` | Aktifkan Experimental OAuth IDE lab |
| `LINTASAN_OAUTH_PUBLIC_BASE_URL` | — | Public origin untuk OAuth redirect (misal `https://lintasan.example.com`) |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis address untuk vector memory |
| `DASHBOARD_URL` | `http://127.0.0.1:20180` | Dashboard URL (jika proxy terpisah) |

### Penggunaan

```bash
# Langsung di CLI
PORT=8080 LINTASAN_MASTER_KEY=mykey ./lintasan start

# Via export
export LINTASAN_MASTER_KEY=$(openssl rand -hex 32)
export PORT=20180
./lintasan start

# Di systemd service
Environment="LINTASAN_MASTER_KEY=..."
Environment="PORT=20180"

# Di docker-compose
environment:
  LINTASAN_MASTER_KEY: "${LINTASAN_MASTER_KEY}"
  PORT: "20180"
```

---

## ⚙ Dashboard Settings

Settings dashboard dapat diakses via UI di `/dashboard/settings`. Semua setting disimpan di database SQLite.

### General

| Setting | Tipe | Default | Deskripsi |
|---------|------|---------|-----------|
| Theme | toggle | System | Light / Dark / System |
| Language | select | English | UI language |
| Server Port | number | 20180 | Port server (restart required) |

### Proxy

| Setting | Tipe | Default | Deskripsi |
|---------|------|---------|-----------|
| Default Provider | select | — | Provider default untuk semua request |
| Default Model | text | gpt-4o | Model default |
| Max Tokens | number | 4096 | Default max tokens |
| Temperature | number | 0.7 | Default temperature |
| Request Timeout | number | 120 | Timeout per request (detik) |

### Cache

| Setting | Tipe | Default | Deskripsi |
|---------|------|---------|-----------|
| Response Cache | toggle | ON | Cache response identik |
| Semantic Cache | toggle | OFF | Cache berdasarkan semantic similarity |
| Cache TTL | number | 300 | Cache lifetime (detik) |
| Similarity Threshold | number | 0.92 | Threshold cosine similarity |

### Security

| Setting | Tipe | Default | Deskripsi |
|---------|------|---------|-----------|
| CORS Origins | text | `*` | Allowed origins (pisah koma) |
| Rate Limit (RPM) | number | 60 | Request per minute per key |
| Rate Limit (RPD) | number | 10000 | Request per day per key |
| Max Input Tokens | number | 128000 | Maksimum input tokens |

### Observability

| Setting | Tipe | Default | Deskripsi |
|---------|------|---------|-----------|
| Metrics Enabled | toggle | ON | Ekspos `/metrics` endpoint |
| Request Logging | toggle | ON | Log semua request |
| Log Retention | number | 7 | Retention log (hari) |

---

## 🔗 Connection Setup

Connection adalah konfigurasi provider API yang akan diproxikan oleh Lintasan.

### Add via Dashboard

1. Buka **Accounts** → Klik **+ Add Connection**
2. Isi:
   - **Name**: Label koneksi (misal "OpenAI Prod")
   - **Base URL**: Endpoint provider (misal `https://api.openai.com/v1`)
   - **API Key**: API key provider
   - **Format**: Format API (`openai`, `anthropic`, `google`, dll)
   - **Priority**: Prioritas routing (higher = preferred)
3. Klik **Test** untuk verifikasi koneksi
4. Klik **Save**

### Add via API

```bash
curl -X POST http://localhost:20180/api/connections \
  -H "Authorization: Bearer <master-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "OpenAI Prod",
    "base_url": "https://api.openai.com/v1",
    "api_key": "sk-...",
    "format": "openai",
    "priority": 100
  }'
```

### Import via cURL

```bash
curl -X POST http://localhost:20180/api/connections/import-curl \
  -H "Authorization: Bearer <master-key>" \
  -H "Content-Type: text/plain" \
  -d 'curl https://api.openai.com/v1/chat/completions \
    -H "Authorization: Bearer sk-..." \
    -d "{\"model\":\"gpt-4o\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}"'
```

### Supported Connection Formats

| Format | Contoh Provider |
|--------|----------------|
| `openai` | OpenAI, Groq, Together AI, Fireworks |
| `anthropic` | Anthropic Claude |
| `google` | Google Gemini, Vertex AI |
| `openai-compatible` | vLLM, Ollama, LM Studio (self-hosted) |
| `aws-bedrock` | AWS Bedrock |
| `azure` | Azure OpenAI |
| `sumopod` | Sumopod AI |

---

## 🔌 Provider Presets

Lintasan memiliki **118+ provider presets** siap pakai. Cukup pilih dari UI, isi API key, dan langsung bisa digunakan.

### Cara menggunakan

1. Buka **Providers** di dashboard
2. Cari provider (filter by name / category)
3. Klik **+ Add** untuk menambahkan koneksi
4. Isi API key (jika diperlukan)
5. Klik **Save** — koneksi siap digunakan

### Categories

| Kategori | Jumlah | Contoh |
|----------|--------|--------|
| Major Providers | 4 | OpenAI, Anthropic, DeepSeek, Google Gemini |
| Top-Tier | 6 | xAI (Grok), Mistral AI, Azure OpenAI |
| AI Coding | 4 | GitHub Copilot, Codestral |
| Aggregators | 8 | OpenRouter, Replicate, HuggingFace |
| High-Speed Inference | 11 | Groq, Together AI, Fireworks, Cerebras |
| GPU Cloud | 12 | Baseten, OctoAI, Lepton AI |
| Chinese Providers | 10 | GLM, Kimi, Qwen, MiniMax |
| Indonesia Providers | 2 | **Sumopod**, Apertis AI |
| Self-Hosted | 3 | Ollama, vLLM, LM Studio |
| Enterprise & Cloud | 8 | Snowflake, Oracle OCI, SAP AI Core |
| Specialized | 19 | Perplexity, Cohere, Stability AI |
| Other | 24 | AWS SageMaker, Custom |

> 💡 **Free providers** bisa ditemukan otomatis via **Discover** page di dashboard.

---

## 🔀 Routing Configuration

Lintasan mendukung 4 level routing strategy:

### 1. Smart Routing (Multi-Stage)

```
Request masuk → Header match → Model match → Load balance → Fallback → Provider
```

Priority routing:
1. **Header routing** — paksa provider via header `X-Provider`
2. **Model routing** — model name → connection mapping (Combos)
3. **Weighted load balancing** — distribusi request berdasarkan priority
4. **Fallback chain** — jika provider gagal, coba provider lain

### 2. Combo System

Combo adalah pasangan model+provider yang sudah ditentukan:

```json
{
  "alias": "gpt4-fast",
  "model": "gpt-4o-mini",
  "connection_id": 1,
  "priority": 100
}
```

Atur combo di dashboard **Routing** → **Combos**.

### 3. Load Balancer

| Strategy | Deskripsi |
|----------|-----------|
| `round-robin` | Bergiliran antar connection |
| `weighted` | Berdasarkan priority field |
| `least-latency` | Connection dengan latency terendah |

### 4. Fallback Chains

Multi-level fallback per model:

```
user request "gpt-4o-mini"
  → OpenAI (primary)
  → Groq (fallback 1, model: llama-3.1-8b)
  → Together AI (fallback 2, model: llama-3.1-8b)
  → 502 All providers failed
```

Atur di dashboard **Fallback**.

---

## 🔐 Security

### Master API Key

- Auto-generated jika tidak diset
- **Wajib** set `LINTASAN_MASTER_KEY` untuk production
- Generate: `openssl rand -hex 32`

### Authentication Flow

```
Client → Master Key / API Key → Lintasan → Provider API Key
         (Bearer header)                    (disimpan di DB)
```

### API Key Rotation

1. **Dashboard API Keys** → buat key baru
2. Update aplikasi client dengan key baru
3. **Dashboard API Keys** → revoke key lama

### CORS Settings

Default: `Access-Control-Allow-Origin: *`

Untuk production dengan domain spesifik:
```
Dashboard → Settings → CORS Origins → https://app.domain.com
```

---

## 🧠 Advanced

### Vector Memory

Lintasan memiliki built-in vector memory untuk RAG (Retrieval-Augmented Generation).

- **Default**: SQLite-based TF-IDF (zero config)
- **Optional**: Redis (set `REDIS_ADDR`)
- **Pluggable embedders**: dukungan berbagai embedding model

### Plugin System

Ekstensi tanpa ubah core:

1. Buka **Plugins** → **Plugin Store**
2. Install plugin dari store
3. Atau generate plugin dengan AI via **AI Generate**

### MITM Bridge

Mode debug untuk memonitor traffic HTTP/HTTPS:

```bash
./lintasan mitm start
```

Certificate auto-generated. Buka `http://mitm.lintasan/` untuk instruksi setup CA di perangkat.

### Metrics Endpoint

Ekspos metrik Prometheus-compatible:

```
GET /metrics
```

- Cache hit/miss rate
- Request volume per endpoint
- Latency distribution
- Active connections
- Token usage

---

## 📝 Contoh Konfigurasi Lengkap

### Minimal (development)

```bash
./lintasan start
```

### Production (hardened)

```bash
export LINTASAN_MASTER_KEY=$(openssl rand -hex 32)
export PORT=20180
export LINTASAN_DATA_DIR=/var/lib/lintasan

./lintasan start
```

### Docker Compose (production)

```yaml
services:
  lintasan:
    build: .
    ports: ["20180:20180"]
    environment:
      LINTASAN_MASTER_KEY: "${LINTASAN_MASTER_KEY}"
      LINTASAN_DATA_DIR: /app/data
    volumes:
      - lintasan-data:/app/data
    restart: unless-stopped
```

---

> **🇮🇩** Ada pertanyaan konfigurasi? Buka [issue](https://github.com/sanhaji182/lintasan/issues).
>
> **🇬🇧** Configuration questions? Open an [issue](https://github.com/sanhaji182/lintasan/issues).

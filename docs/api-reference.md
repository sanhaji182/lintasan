# 🇮🇩 Lintasan Go — API Reference

> Referensi lengkap semua endpoint API Lintasan. Cocok untuk integrasi langsung tanpa dashboard.

---

## 🇬🇧 Lintasan Go — API Reference

> Complete API endpoint reference. Use these directly from your apps, scripts, or SDKs.

---

## 📡 Base URL

```
http://localhost:20180
```

Production: `https://lintasan.example.com` (via nginx reverse proxy)

---

## 🔐 Authentication

| Header | Value |
|--------|-------|
| `Authorization` | `Bearer <LINTASAN_MASTER_KEY>` |

### Public endpoints (no auth required)

| Endpoint | Notes |
|----------|-------|
| `POST /api/auth/login` | Login with username + password |
| `GET /api/auth/check` | Check if session token is valid |
| `GET /health` | Health check |
| `GET /api/providers/presets` | Public provider presets |

> All other `/api/*` and `/v1/*` endpoints require `Authorization: Bearer *** header.

### Session Auth (Dashboard)

```bash
# Login → get session token
curl -X POST http://localhost:20180/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "<otp>"}'
# → {"success": true, "token": "dashboard-session-token"}

# Check session
curl http://localhost:20180/api/auth/check
# → {"authenticated": true}
```

---

## 🏥 Health

```bash
GET /health
```

**Response:**
```json
{
  "status": "ok",
  "version": "v0.24.2",
  "uptime": "2h34m12s"
}
```

---

## 🤖 LLM Proxy (OpenAI-compatible)

Lintasan adalah **drop-in replacement** untuk OpenAI API. Ganti `https://api.openai.com/v1` → `http://localhost:20180/v1`.

### Chat Completions

```bash
POST /v1/chat/completions
```

Supports **streaming** (SSE) and **non-streaming** responses.

```bash
curl http://localhost:20180/v1/chat/completions \
  -H "Authorization: Bearer *** \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `model` | string | required | Model name (e.g. `gpt-4o`, `claude-3-opus`, `deepseek-chat`) |
| `messages` | array | required | Chat messages (OpenAI format) |
| `stream` | boolean | `false` | Enable SSE streaming |
| `temperature` | number | `0.7` | Sampling temperature |
| `max_tokens` | number | `4096` | Maximum response tokens |
| `top_p` | number | `1` | Nucleus sampling |
| `stop` | string/array | — | Stop sequences |

### List Models

```bash
GET /v1/models
```

Returns all models from all connected providers.

**Response:** OpenAI-compatible model list format.

### Embeddings

```bash
POST /v1/embeddings
```

```bash
curl http://localhost:20180/v1/embeddings \
  -H "Authorization: Bearer *** \
  -H "Content-Type: application/json" \
  -d '{"model": "text-embedding-3-small", "input": "Hello world"}'
```

### Image Generation

```bash
POST /v1/images/generations
```

Proxies to DALL-E, Stable Diffusion, or other image providers.

```bash
curl http://localhost:20180/v1/images/generations \
  -H "Authorization: Bearer *** \
  -H "Content-Type: application/json" \
  -d '{"model": "dall-e-3", "prompt": "A sunset over mountains", "n": 1}'
```

### Audio: Text-to-Speech

```bash
POST /v1/audio/speech
```

```bash
curl http://localhost:20180/v1/audio/speech \
  -H "Authorization: Bearer *** \
  -H "Content-Type: application/json" \
  -d '{"model": "tts-1", "input": "Hello world", "voice": "alloy"}'
```

### Audio: Speech-to-Text

```bash
POST /v1/audio/transcriptions
```

```bash
curl http://localhost:20180/v1/audio/transcriptions \
  -H "Authorization: Bearer *** \
  -F "file=@audio.mp3" -F "model=whisper-1"
```

### Web Search

```bash
POST /v1/web/search
```

Augment chat with live web results.

```bash
curl http://localhost:20180/v1/web/search \
  -H "Authorization: Bearer *** \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o", "query": "latest AI news"}'
```

### Vector Memory

```bash
# Store memory
POST /v1/memory
-d '{"key": "user-preference", "content": "User prefers concise responses"}'

# Search memories
GET /v1/memory/search?q=preference&limit=5

# Memory stats
GET /v1/memory/stats

# List all memories
GET /v1/memory

# Delete memory
DELETE /v1/memory/{key}
```

---

## 🔧 Dashboard API

### Stats & Overview

| Method | Endpoint | Response |
|--------|----------|----------|
| `GET` | `/api/stats` | Global stats: `{totalRequests, cachedRequests, cacheHitRate, avgLatency, tokensToday, tokensMonth, tokensSaved, tokensCompressed, activeModels, activeConnections, features[], providers[], requestVolume[]}` |
| `GET` | `/api/dashboard/stats` | Simplified: `{total_requests, active_connections, cache_hit_rate, avg_latency, uptime}` |

### Connections

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/connections` | List all connections |
| `POST` | `/api/connections` | Create new connection |
| `PATCH` | `/api/connections` | Toggle active/inactive |
| `DELETE` | `/api/connections/{id}` | Delete connection |
| `POST` | `/api/connections/test` | Test connection (latency + model count) |
| `GET` | `/api/connections/sync` | Sync all connections |
| `POST` | `/api/connections/import-curl` | Import connection from cURL command |
| `POST` | `/api/models/sync/{connection_id}` | Sync models for a connection |

### Routing (Combos)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/combos` | List all combos |
| `POST` | `/api/combos` | Create combo |
| `PUT` | `/api/combos?id={id}` | Update combo |
| `DELETE` | `/api/combos?id={id}` | Delete combo |
| `POST` | `/api/routing/reorder` | Reorder combos |

### Load Balancer

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/load-balancer` | Get strategy |
| `POST` | `/api/load-balancer` | Set strategy (`round-robin`, `weighted`, `least-latency`) |

### Aliases

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/aliases` | List aliases |
| `POST` | `/api/aliases` | Create/update alias |
| `DELETE` | `/api/aliases?id={id}` | Delete alias |

### Fallback

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/fallback` | List fallback chains |
| `POST` | `/api/fallback` | Create fallback chain |
| `DELETE` | `/api/fallback?type={type}&id={id}` | Delete fallback chain |

**POST body:**
```json
{
  "type": "model",
  "id": "gpt4-fallback",
  "fallbacks": ["gpt-4o", "gpt-4-turbo", "gpt-3.5-turbo"]
}
```

### Logs

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/logs?limit=20` | Recent request logs |

### Analytics & Usage

| Method | Endpoint | Response |
|--------|----------|----------|
| `GET` | `/api/analytics` | `{tokensSavedToday, cacheHitRate, totalTokensUsed, costSaved, avgLatency, totalRequests, daily[], breakdown}` |
| `GET` | `/api/usage` | `{providers[], models[], daily[]}` |
| `GET` | `/api/telemetry` | Telemetry data |
| `GET` | `/api/savings/summary` | Cost savings summary |
| `GET` | `/api/savings/history` | Cost savings history |

### API Keys

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/keys` | List API keys |
| `POST` | `/api/keys` | Create key |
| `DELETE` | `/api/keys?id={id}` | Revoke key |

### Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/settings` | Get all settings |
| `PUT` | `/api/settings` | Update settings (partial) |

### Providers & Presets

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/providers/presets` | 118+ provider presets |
| `GET` | `/api/providers/presets/config?id={id}` | Single preset detail |
| `POST` | `/api/providers/presets/test` | Test preset connectivity |
| `GET` | `/api/providers/discover` | Scan for free providers |

### Models

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/models/catalog` | Model catalog with pricing |
| `GET` | `/api/models/discovered` | Auto-discovered models |
| `GET` | `/api/models/manual` | Manually added models |
| `POST` | `/api/models/manual` | Add manual model |

### Teams

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/teams` | List teams |
| `POST` | `/api/teams` | Create team |
| `GET` | `/api/teams/{id}` | Team detail |
| `PUT` | `/api/teams/{id}` | Update team |
| `DELETE` | `/api/teams/{id}` | Delete team |
| `GET` | `/api/teams/{id}/members` | List members |
| `POST` | `/api/teams/{id}/members` | Add member |

### Users

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/users` | List users |
| `POST` | `/api/users` | Create user |
| `GET` | `/api/users/{id}` | User detail |
| `PUT` | `/api/users/{id}` | Update user |
| `DELETE` | `/api/users/{id}` | Delete user |

### Webhooks

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/webhooks` | List webhooks |
| `POST` | `/api/webhooks` | Create webhook |
| `DELETE` | `/api/webhooks?id={id}` | Delete webhook |

### Plugins

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/plugins` | List installed plugins |
| `POST` | `/api/plugins` | Enable/disable plugin |
| `GET` | `/api/plugins/store` | Plugin store |
| `POST` | `/api/plugins/store` | Install from store |
| `POST` | `/api/plugins/generate` | AI-generate plugin |

### Backup

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/backup` | List backup files |
| `POST` | `/api/backup` | Create/restore backup |
| `DELETE` | `/api/backup?file={filename}` | Delete backup file |

### MCP Server

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/mcp` | MCP server info |
| `SSE` | `/mcp` | MCP SSE stream |
| `POST` | `/mcp` | MCP JSON-RPC request |

### Chat Test

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/chat-test` | Test chat (proxied) |

### Export

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/export` | Export config as JSON |

### Other

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/favicon?domain={domain}` | Proxy favicon |
| `GET` | `/api/translate` | Available translation formats |
| `POST` | `/api/translate` | Translate request across formats |
| `GET` | `/metrics` | Prometheus-compatible metrics endpoint |

---

## 📦 Response Format

All `/api/*` responses are wrapped in `{"data": ...}` unless stated otherwise.

Exceptions:
- `/v1/chat/completions` — SSE streaming or raw OpenAI-compatible JSON
- `/health` — `{"status": "ok", "version": "...", "uptime": "..."}`
- `/v1/models` — OpenAI-compatible model list
- `/metrics` — Prometheus text format

---

## 🚨 Error Codes

| Code | 🇮🇩 Indonesia | 🇬🇧 English |
|------|-------------|------------|
| `400` | Request tidak valid | Invalid request |
| `401` | API key tidak valid | Invalid API key |
| `404` | Endpoint/model tidak ditemukan | Endpoint/model not found |
| `429` | Rate limit atau kuota habis | Rate limited or quota exhausted |
| `500` | Internal server error | Internal server error |
| `502` | Semua provider gagal (termasuk daftar yang dicoba) | All providers failed (with retry list) |

---

## 💡 SDK Compatibility

Lintasan works as a **drop-in replacement** for any OpenAI-compatible SDK:

```python
# OpenAI Python SDK
from openai import OpenAI
client = OpenAI(base_url="http://localhost:20180/v1", api_key="<master-key>")

# LangChain
from langchain_openai import ChatOpenAI
llm = ChatOpenAI(base_url="http://localhost:20180/v1", api_key="<master-key>")

# Vercel AI SDK
import { OpenAI } from 'openai'
const client = new OpenAI({ baseURL: 'http://localhost:20180/v1', apiKey: '<master-key>' })

# cURL (any language)
curl http://localhost:20180/v1/chat/completions -H "Authorization: Bearer *** ...
```

---

> **🇮🇩** Ada endpoint yang tidak terdaftar? Buka [issue](https://github.com/sanhaji182/lintasan/issues).
>
> **🇬🇧** Missing an endpoint? Open an [issue](https://github.com/sanhaji182/lintasan/issues).

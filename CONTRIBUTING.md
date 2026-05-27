# Contributing to Lintasan Go

Terima kasih atas minatnya untuk berkontribusi! 🚀

Lintasan Go adalah LLM proxy router open-source yang menghubungkan client (OpenAI-compatible SDK, IDE plugin, CLI) ke berbagai provider AI — dengan failover otomatis, caching, rate limiting, dan vector memory.

---

## Prasyarat

- **Go 1.22+**
- **Node.js 20+** (untuk dashboard frontend)
- **Git**
- **Gitleaks** (untuk secret scanning) — `sudo apt install gitleaks` atau `brew install gitleaks`

---

## Setup Development

```bash
# 1. Clone repo
git clone https://github.com/sanhaji182/lintasan-go.git
cd lintasan-go

# 2. Setup env
cp .env.example .env
# Edit .env — set LINTASAN_MASTER_KEY=
# Setidaknya tambah satu provider (Sumopod, CommandCode Alpha, dll)

# 3. Build & Install dependencies
go build -o bin/lintasan ./cmd/lintasan/

# 4. (Opsional) Setup dashboard
cd dashboard && npm install && cd ..

# 5. Jalankan
./bin/lintasan start
# Go server di http://localhost:20180
# API di http://localhost:20180/v1/chat/completions
# Dashboard di http://localhost:20180/dashboard (jika dashboard terpisah dijalankan)
```

---

## Struktur Proyek

```
lintasan-go/
├── cmd/lintasan/          # Entry point + CLI (Cobra)
├── internal/              # Core packages
│   ├── auth/              # OAuth token manager (Cursor/Codex/Copilot)
│   ├── batch/             # Concurrent request batching
│   ├── budget/            # Adaptive token budget
│   ├── cache/             # Exact hash, stream, semantic cache
│   ├── circuit/           # Circuit breaker (3-state)
│   ├── combo/             # Multi-key rotation (priority + round-robin)
│   ├── compress/          # Context compression (>8K tokens)
│   ├── config/            # Environment-based config
│   ├── cost/              # Per-token cost tracking
│   ├── db/                # SQLite WAL-mode database + migrations
│   ├── discover/          # Auto model discovery
│   ├── fallback/          # Model + connection fallback chains
│   ├── freeproviders/     # Local provider scanner (Ollama, LM Studio, etc.)
│   ├── lb/                # 5-strategy load balancer
│   ├── memory/            # Vector memory (TF-IDF embedder + Redis)
│   ├── mitm/              # MITM proxy for IDE traffic
│   ├── mlrouter/          # ML-based smart routing
│   ├── models/            # Provider + model catalog (65+ models)
│   ├── optimizer/         # System prompt optimizer
│   ├── plugin/            # Goja JS plugin runtime
│   ├── quality/           # Response quality scoring
│   ├── quota/             # Per-user token quota
│   ├── ratelimit/         # Sliding window rate limiter
│   ├── reasoning/         # Reasoning content extractor
│   ├── reflect/           # Self-review auto-fix loop (X-Lintasan-Reflect)
│   ├── retry/             # Exponential backoff + jitter
│   ├── rtk/               # RTK subprocess compression
│   ├── server/            # HTTP server, proxy handler, format translators
│   ├── webhook/           # HMAC-signed webhook delivery
│   └── websearch/         # DuckDuckGo Instant Answer web search
├── web/frontend/          # Vite + React dashboard (SPA)
├── dashboard/             # (Future) Next.js dashboard
├── data/                  # SQLite database (gitignored)
└── .env                   # Environment variables (gitignored)
```

---

## Cara Menjalankan Test

```bash
# Semua test
go test ./internal/...

# Package spesifik dengan verbose
go test -v ./internal/cache/

# Dengan race detector
go test -race ./internal/...

# Coverage
go test -cover ./internal/...
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out
```

---

## Coding Convention

- **Format**: `gofmt -w .` sebelum commit
- **Lint**: `go vet ./...` harus bersih
- **Naming**: CamelCase untuk exported, camelCase untuk unexported
- **Error handling**: Jangan ignore error. Gunakan `fmt.Errorf("context: %w", err)` untuk wrapping
- **Comments**: Deskripsikan exported functions, types, dan constants
- **Tests**: Test file di package yang sama dengan suffix `_test.go`
- **SQL**: Gunakan WAL mode, prepared statements, `ON CONFLICT` untuk upsert

### Commit Message

```
<type>: <short description>

<optional body>

<optional footer>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`

Contoh:
```
feat: add provider preset for Mistral AI

Adds Mistral AI preset with correct base URL, auth format,
and model discovery path. Also updates provider categories.

Closes #42
```

---

## Cara Menambah Provider

### Provider Preset (dashboard template)

Edit `internal/server/handlers_parity.go` → `providerPresets()`. Format:

```go
{"id":"mistral","name":"Mistral AI","description":"Open-weight models","website":"https://mistral.ai",
 "category":"major","baseUrl":"https://api.mistral.ai/v1",
 "format":"openai","chatPath":"/chat/completions","modelsPath":"/models",
 "authHeader":"Authorization","authPrefix":"Bearer "},
```

- `id`: unique, lowercase, no spaces
- `category`: salah satu dari `major`, `aggregator`, `inference`, `chinese`, `indonesia`, `enterprise`, `media`, `other`, `self-hosted`
- `format`: `"openai"` untuk 95% provider, `"anthropic"` / `"gemini"` / `"commandcode"` untuk format non-OpenAI
- `authHeader` + `authPrefix`: contoh `"Authorization"` / `"Bearer "`

### Active Connection (database)

Setelah user add provider via dashboard, Lintasan auto-discovers models dan menyimpannya ke DB. Untuk manual:

```sql
-- Tambah connection
INSERT OR REPLACE INTO connections
(id, name, base_url, api_key, format, chat_path, models_path, is_active, priority)
VALUES ('mistral-001', 'Mistral AI', 'https://api.mistral.ai/v1', '<KEY>',
        'openai', '/chat/completions', '/models', 1, 10);

-- Sync models
-- Trigger: POST /api/models/sync/mistral-001
```

---

## Cara Menambah Fitur Baru

1. **Buka issue** — jelaskan fitur, use case, dan acceptance criteria
2. **Diskusi** — tunggu feedback maintainer
3. **Branch**: `git checkout -b feat/descriptive-name`
4. **Implementasi**:
   - Buat package baru di `internal/<feature>/`
   - Tulis tests (target: coverage >80%)
   - Wire package ke `internal/server/proxy.go` atau `server.go`
   - Update `go.mod` hanya jika butuh dependency baru
5. **Verifikasi**:
   ```bash
   gofmt -w .
   go vet ./...
   go test -race ./internal/...
   go build -o /dev/null ./cmd/lintasan/
   ```
6. **Commit** dengan pesan yang jelas
7. **Push** dan buka Pull Request ke `main`

---

## Keamanan

### Jangan commit credentials

- Semua API keys, token, dan secret HARUS disimpan di `.env` (gitignored)
- Gunakan `LINTASAN_MASTER_KEY=` di `.env.example`
- Contoh placeholder: `YOUR_API_KEY`, `YOUR_ACCOUNT`, `REGION`

### Secret Scanning

Sebelum push, jalankan:

```bash
gitleaks detect --source . --no-banner
```

Kalo ada leak, bersihkan dulu. Jangan pernah push real credentials.

### Kalo kamu accidentally commit secret

1. **Rotasi kredensialnya segera** (ini yang paling penting)
2. Hapus dari history dengan `git filter-repo`
3. Force push
4. Laporkan ke maintainer

---

## Dokumentasi

- **README.md**: Overview, quickstart, fitur, arsitektur
- **.env.example**: Semua environment variables dengan deskripsi
- **Komentar kode**: Untuk logic yang kompleks atau non-obvious
- **ADR** (Architecture Decision Records): untuk keputusan arsitektur besar — tulis di `docs/adr/`

---

## Review Process

1. Semua PR butuh review dari maintainer
2. CI harus passing: build + test + vet
3. Secret scan harus bersih
4. Reviewers akan cek:
   - Kode mengikuti convention
   - Ada test coverage yang cukup
   - Tidak ada breaking changes tanpa diskusi
   - Tidak ada hardcoded credentials atau URLs
5. Iterasi: address feedback → push update → review lagi
6. Merge jika semua ✅

---

## Release Process

1. Update version di codebase
2. Update CHANGELOG.md
3. Tag: `git tag vX.Y.Z`
4. Build binary: `go build -ldflags="-s -w" -o lintasan-go ./cmd/lintasan/`
5. Buat GitHub Release dengan binary attachment
6. Announce di Discussions

---

## Komunitas

- **Issues**: Bug report, feature request
- **Discussions**: Tanya jawab, ide, showcase
- **Bahasa**: Indonesia atau Inggris — dua duanya welcome 🇮🇩 🇬🇧

---

## Lisensi

Dengan berkontribusi, kamu setuju kontribusimu dilisensikan di bawah [MIT License](LICENSE).

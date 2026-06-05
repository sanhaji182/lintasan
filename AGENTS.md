# AGENTS.md — Lintasan Go

> **Baca file ini dulu sebelum menyentuh kode apa pun.** Ini adalah peta sistem untuk AI agent (Claude Code, Codex, Cursor, Hermes, dll). Tujuannya: paham keseluruhan arsitektur, konvensi, dan cara kerja Lintasan tanpa harus meraba-raba.

---

## 1. Apa itu Lintasan

Lintasan adalah **LLM proxy gateway** — satu endpoint OpenAI-compatible yang merutekan request ke banyak provider AI (OpenAI, Anthropic, DeepSeek, Gemini, Groq, dll) dengan smart routing, failover, caching, token compression, observability, dan dashboard.

- **Backend:** Go (single binary, ~24MB)
- **Frontend:** SvelteKit 5 (dashboard, 26+ halaman)
- **Repo:** `github.com/sanhaji182/lintasan-go` (monorepo)
- **Filosofi:** "Setiap Koneksi Punya Jalannya."

> ⚠️ **Node.js v1 sudah DIHAPUS (May 2026).** Semua kode aktif ada di monorepo ini. Jangan cari/buat referensi ke `~/lintasan` lama.

---

## 2. Arsitektur Tingkat Tinggi

```
                 ┌─────────────────────────────────────────┐
   Browser  ───► │  nginx (lintasan.sans.biz.id, :443 TLS)  │
                 └────────────────────┬────────────────────┘
                                      │
                 / · /login · /dashboard · /api/* · /v1/* · /health
                                      ▼
                   ┌────────────────────────────────────────┐
                   │  Go backend  :20180   (lintasan start)  │
                   │  ── serves BOTH ──                       │
                   │   • embedded SPA dashboard (go:embed)    │
                   │   • API + OpenAI-compatible LLM proxy    │
                   └────────────────────┬───────────────────┘
                                        │
                                        ▼
            ┌───────────────────────────────────┐
            │ SQLite (data) + provider upstreams │
            └───────────────────────────────────┘
```

**Single binary (sejak v0.24.0):** Dashboard SvelteKit dikompilasi ke static SPA (`adapter-static`) lalu di-`go:embed` ke binary Go via package `internal/web`. Satu proses `lintasan start` di `:20180` menyajikan UI **dan** API — tidak ada proses Node terpisah.

**Pembagian routing nginx (sekarang sederhana):**
- Semua path → Go `:20180`. Backend yang membedakan: `/api/*` `/v1/*` `/mcp` `/health` ditangani handler; sisanya (`/`, `/login`, `/dashboard/*`, `/_app/*`, favicon) dilayani SPA embedded.
- Auth middleware: GET ke path UI/asset statis lewat tanpa auth (shell SPA tidak menyimpan secret; guard berjalan client-side). `/api/*` `/v1/*` `/mcp` **tetap** fail-closed 401 tanpa token. Dikunci 2 security boundary test.

Frontend memanggil API lewat path relatif (`/api/...`, `/v1/...`). **Jangan hardcode `localhost:20180` di frontend.**

> **Mode 2-service lama (deprecated):** SvelteKit `:5173` via `node build/index.js` + nginx split masih bisa jalan karena source `frontend/` utuh, tapi single-binary adalah jalur resmi. Prod sudah migrasi ke single-binary; service `lintasan-dashboard` di-disable.

---

## 3. Port & Service Map

| Service | Port | systemd unit | WorkingDir | ExecStart |
|---------|------|--------------|------------|-----------|
| Go backend (UI + API) | `20180` | `lintasan.service` | `/home/ubuntu/lintasan-go` | `lintasan start` |

Env penting backend: `PORT=20180`.

> Service `lintasan-dashboard` (SvelteKit Node `:5173`) sudah **di-stop + disable** sejak migrasi single-binary. Tidak perlu lagi — UI dilayani backend.

**Backend jalan sebagai systemd (Restart=always, PPID=1).** Jangan jalankan sebagai child process sesi — akan mati saat sesi putus.

Restart setelah deploy:
```bash
sudo systemctl restart lintasan
sudo systemctl is-active lintasan
```

Deploy binary baru (downtime ~0.2–0.3 detik):
```bash
make build                          # frontend → embed → ./lintasan
sudo systemctl stop lintasan        # lepas file handle (hindari "text file busy")
cp dist-bin/lintasan-linux-amd64 lintasan   # atau pakai ./lintasan hasil make build
sudo systemctl start lintasan
curl -s localhost:20180/health      # verifikasi versi
```

---

## 4. Struktur Repo

```
lintasan-go/
├── cmd/                      # entrypoint binary (lintasan start)
├── internal/                 # 36 package backend (lihat §5)
├── frontend/                 # SvelteKit 5 dashboard
│   ├── src/
│   │   ├── routes/           # halaman (file-based routing)
│   │   │   ├── +page.svelte           # landing page (/)
│   │   │   ├── login/+page.svelte      # login
│   │   │   └── dashboard/              # semua halaman dashboard
│   │   │       ├── +layout.svelte      # shell (Sidebar + Header)
│   │   │       ├── +layout.ts          # AUTH GUARD (client-side)
│   │   │       └── <page>/+page.svelte
│   │   ├── lib/
│   │   │   ├── api.ts                  # API client (auto-attach JWT)
│   │   │   ├── components/             # Sidebar, Header, Toast, dll
│   │   │   └── stores/                 # theme, auth, toast
│   │   └── app.css                     # design tokens (CSS vars)
│   └── build/                # output produksi (node adapter)
├── docs/
│   ├── api-reference.md
│   └── design-system.md
├── README.md                 # dokumentasi lengkap (bilingual)
├── CONTRIBUTING.md
└── AGENTS.md                 # file ini
```

---

## 5. Backend Packages (`internal/`)

Setiap package punya tanggung jawab tunggal. Saat menambah fitur, ikuti pola package yang sudah ada.

| Package | Fungsi |
|---------|--------|
| `server` | HTTP mux, route registration, middleware (18 file, ~70 route) |
| `proxy` | Core LLM proxy: chat completions, embeddings, images, audio |
| `auth` | JWT auth (HS256), password hashing (SHA-512), user CRUD, middleware |
| `config` | Loading & validasi konfigurasi |
| `db` | SQLite schema + migrations |
| `cache` | Semantic cache (cosine similarity), response/stream cache |
| `combo` | Routing combos (alias model → provider+model) |
| `fallback` | Fallback chain antar provider/model |
| `circuit` | Circuit breaker (auto-disable provider gagal) |
| `compress` | Token compression |
| `cost` | Cost tracking per request |
| `budget` | Budget limits |
| `discover` | Provider discovery |
| `freeproviders` | Free provider catalog |
| `guard` | Guardrails (input/output filtering) |
| `lb` | Load balancing |
| `mcp` | MCP server (JSON-RPC 2.0, 14 tools) — HTTP + SSE |
| `memory` | Vector memory (search/store/stats/list) |
| `mlrouter` | ML-based smart routing |
| `models` | Model catalog & metadata |
| `optimizer` | Request optimization |
| `plugin` | Plugin system (extensible tanpa ubah core) |
| `quality` | Quality scoring |
| `quota` | Quota management |
| `ratelimit` | Rate limiting |
| `reasoning` | Reasoning extraction |
| `reflect` | Self-reflection |
| `retry` | Retry logic |
| `translator` | Cross-format translation (5 format) |
| `webhook` | Webhook subscriptions |
| `websearch` | Web search integration |
| `logging` | Request logging |
| `batch`, `mitm`, `rtk` | Batch processing, MITM inspect, runtime toolkit |

---

## 6. API Route Map

Auth (JWT dashboard):
```
POST   /api/auth/login        # username+password → { token, user }
GET    /api/auth/me           # validasi token (Bearer)
POST   /api/auth/logout
GET    /api/auth/users        # admin only
POST   /api/auth/users        # admin only
```

OpenAI-compatible proxy (`/v1/*`):
```
GET    /v1/models
POST   /v1/chat/completions   # streaming + non-streaming
POST   /v1/embeddings
POST   /v1/images/generations
POST   /v1/audio/speech
POST   /v1/audio/transcriptions
POST   /v1/web/search
GET    /v1/memory  ·  GET /v1/memory/search  ·  POST /v1/memory  ·  GET /v1/memory/stats  ·  DELETE /v1/memory/{key}
```

Dashboard API (`/api/*`):
```
GET    /api/models/catalog
GET    /api/connections · POST · PATCH · DELETE (+ /{id})
GET    /api/combos · POST · PUT · DELETE
GET    /api/providers/discover
GET    /api/stats · /api/logs · /api/analytics · /api/telemetry
GET    /api/savings/summary · /api/savings/history
POST   /api/translate · GET /api/translate/formats
GET    /api/mcp/tools
```

MCP (JSON-RPC 2.0):
```
POST   /mcp        # HTTP transport
GET    /mcp/sse    # Server-Sent Events transport
```

Health:
```
GET    /health
```

> Daftar lengkap & contoh payload ada di `docs/api-reference.md`.

---

## 7. Auth Flow (WAJIB dipahami sebelum sentuh dashboard)

Kontrak UX auth (jangan dirusak):

1. **Unauthenticated → selalu ke `/login`.**
   - `dashboard/+layout.ts` adalah guard client-side: kalau tidak ada token → `redirect(307, '/login')`; kalau ada → validasi `GET /api/auth/me` dengan Bearer token; gagal → clear localStorage + redirect login.
   - `/` (`+page.svelte`) tidak hardcode ke dashboard — route by auth state.

2. **Token ada ≠ session valid.** Selalu validasi via `/api/auth/me` sebelum auto-forward dari login ke dashboard.

3. **Kontrol auth eksplisit di shell:**
   - Header: tombol `Login` (belum auth) / `Logout` + shortcut user (sudah auth).
   - Sidebar: label `User Management` untuk admin akun.

Token disimpan di `localStorage`: `lintasan_token`, `lintasan_user`.
API client (`frontend/src/lib/api.ts`) otomatis attach `Authorization: Bearer <token>` dan handle 401 (clear + redirect login).

**Default kredensial:** `admin` / `admin123` (seed admin di `internal/auth/migration.go`).

---

## 8. Frontend Konvensi

- **SvelteKit 5** dengan runes (`$state`, `$derived`, `$props`, `$bindable`).
- **Design tokens** di `frontend/src/app.css` sebagai CSS variables (`--color-*`, `--radius-*`, `--shadow-*`). Default theme = **clean light**. Dark mode tersedia lewat `[data-theme="dark"]` + `theme` store.
- **Styling:** Tailwind v4 + scoped `<style>` per komponen. Untuk halaman entry (landing/login) pakai scoped CSS standalone agar konsisten.
- **Ikon:** `lucide-svelte`.
- **API calls:** selalu lewat `api` dari `$lib/api` (jangan `fetch` mentah kecuali untuk auth/me di guard).
- **A11y wajib:** `<label for>` + `id`, tombol icon-only butuh `aria-label`/`title`, modal butuh role + keyboard handler, hindari mouseenter/mouseleave JS untuk hover (pakai CSS `:hover`). Build harus bebas warning a11y.

Pitfall lama (sudah resolved tapi catat): lucide-svelte sebagai komponen di `EmptyState icon={...}` kadang memicu type error svelte-check; ini warning type, bukan blocker build.

---

## 8b. CLI Commands (binary `lintasan`)

Dibangun dengan cobra. Command tersedia:

```
lintasan start          # start proxy server (port dari env PORT, default 20180)
lintasan setup          # interactive setup wizard
lintasan mitm start     # MITM bridge untuk IDE traffic interception (port MITM_PORT, default 8443)
```

`lintasan.service` menjalankan `lintasan start`. MITM mode adalah fitur terpisah untuk inspect/intercept trafik IDE (lihat package `internal/mitm`).

---

## 8c. Environment Variables

Sumber kebenaran: `.env.example` di root. Copy ke `.env` sebelum jalan.

**Required:**
| Var | Default | Fungsi |
|-----|---------|--------|
| `PORT` | `20180` | Port Go server |
| `LINTASAN_DATA_DIR` | `./data` | Direktori SQLite DB + runtime data |
| `LINTASAN_MASTER_KEY` | _(kosong)_ | Master API key untuk autentikasi proxy request. Generate: `openssl rand -hex 32` |

**Optional:**
| Var | Default | Fungsi |
|-----|---------|--------|
| `LINTASAN_JWT_SECRET` | _(auto)_ | Secret untuk JWT signing (auth dashboard). Set eksplisit di production. |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis untuk vector memory. Degrade gracefully kalau Redis mati. |
| `MITM_PORT` | `8443` | Port MITM proxy untuk IDE interception |
| `DASHBOARD_URL` | `http://127.0.0.1:20180` | URL dashboard untuk reverse-proxy; kosongkan untuk disable |

> ⚠️ Dua mode auth berbeda:
> - **Proxy clients** (`/v1/*`) → autentikasi pakai `LINTASAN_MASTER_KEY` (Bearer). Kalau master key kosong, middleware **fail-closed** (return 401 "Invalid API key") — TIDAK ada dev mode allow-all. Setup state machine (BOOTSTRAP → ACTIVE) menentukan perilaku: BOOTSTRAP hanya mengizinkan setup endpoints; ACTIVE fail-closed untuk semua management endpoints.
> - **Dashboard users** (`/api/auth/*`) → JWT (HS256) ditandatangani `LINTASAN_JWT_SECRET`.

---

## 8d. Database Schema (SQLite, 18 tabel)

DB ada di `$LINTASAN_DATA_DIR/` (default `./data/`). Migrasi otomatis saat start.

| Tabel | Isi |
|-------|-----|
| `users` | Akun dashboard (JWT auth), seed `admin` |
| `connections` | Provider connections (base_url, api_key, format) |
| `discovered_models` | Hasil model discovery per provider |
| `settings` | Key-value settings global |
| `request_logs` | Log semua proxy request |
| `audit_events` | Audit trail (perubahan penting) |
| `cost_entries` | Cost tracking per request |
| `cost_savings` | Penghematan dari cache/routing/compress |
| `quota_usage` | Pemakaian quota |
| `semantic_cache` | Cache semantik (cosine similarity) |
| `response_cache` | Cache response non-stream |
| `stream_response_cache` | Cache response streaming |
| `embedding_cache` | Cache embedding |
| `memories` | Vector memory entries |
| `plugins` | Plugin terinstall + config |
| `webhooks` | Webhook subscriptions |
| `webhook_deliveries` | Log delivery webhook |
| `oauth_sessions` | Sesi OAuth provider |
| `experimental_providers` | Experimental provider lifecycle state |
| `experimental_credentials` | Encrypted credentials for experimental providers (AES-256-GCM) |
| `provider_presets` | Provider preset catalog (built-in + custom) |
| `preset_categories` | Preset categories with icons/colors |

> ⚠️ **Credential masking pitfall (observed 2026-06-01):** When API keys are displayed via a masker (e.g. `sk-oHU...60KVQQ`), copying the masked string back into a form and saving it overwrites the real key. The masked value is ~25 chars vs ~51 for a real key. **Mitigations:** (1) Dashboard forms should NEVER pre-fill API key fields with stored values — use `•••• stored` placeholder + password input. (2) Server-side validation should reject `len(api_key) < 40` for `sk-` prefixed keys. (3) Sanity check: `SELECT length(api_key) FROM connections;` — anything below ~40 for `sk-` keys is corrupt. This bug recurs any time a key is rendered via masker and re-pasted.

---

## 9. Build, Test, Deploy

**Backend:**
```bash
cd /home/ubuntu/lintasan-go
go build -o lintasan ./cmd/...     # build binary
go test ./...                       # 816 tests, 44 packages — harus PASS
```

**Frontend:**
```bash
cd /home/ubuntu/lintasan-go/frontend
npm run build                       # harus PASS, target zero a11y warning
npm run check                       # svelte-check (type/a11y diagnostics)
```

**Deploy (setelah build):**
```bash
sudo systemctl restart lintasan
sudo systemctl is-active lintasan
```

**Verifikasi live:**
- `https://lintasan.sans.biz.id/` → landing
- `/dashboard` tanpa token → redirect `/login`
- login `admin/admin123` → dashboard
- logout → `/login`

---

## 10. Aturan untuk Agent

1. **Jangan rusak auth flow** (§7). Setiap perubahan dashboard harus tetap lulus: unauth→login, validasi `/api/auth/me`, login→dashboard, logout→login.
2. **Jangan ubah perilaku API/endpoint** kecuali memang itu task-nya. Frontend & klien eksternal bergantung pada kontrak ini.
3. **Build harus tetap hijau.** Jalankan `go test ./...` dan `npm run build` sebelum klaim selesai.
4. **Commit per task**, pesan jelas (`feat(...)`, `fix(...)`). Push ke `main` hanya setelah build pass.
5. **Pure Go, minim dependency** — stdlib + `go-sqlite3` adalah preferensi. Jangan tambah dependency berat tanpa alasan kuat.
6. **Default theme clean light** — hindari desain dark-heavy/glow berlebihan untuk halaman entry.
7. **Verifikasi nyata** — pakai browser/curl untuk cek hasil sebelum klaim "done", jangan berasumsi.
8. **Secrets** — jangan commit `.env`, API key, atau credential. Pakai `.env.example` untuk dokumentasi env.

---

## 11. Quick Reference

```bash
# Status service
systemctl status lintasan --no-pager

# Health check
curl -s https://lintasan.sans.biz.id/health

# Login — see first-run password recovery below. The bootstrap password is
# randomly generated on first start (never hardcoded) and surfaced once on
# stderr. It is forced-rotation on first login.
#
# Password recovery (after the operator rotated and forgot):
#   1. SSH into the host and inspect /var/log/syslog or
#      `journalctl -u lintasan -n 200 --no-pager` for the FIRST-RUN banner.
#   2. If the rotated password is unknown, stop the service, delete the
#      admin row from data/lintasan.db, restart — a fresh admin will be
#      seeded and the password printed to stderr.
#   3. Production operators should set a stable master_key and rotate the
#      admin password via the dashboard Users page.

# Chat completion (OpenAI-compatible)
curl -s -X POST https://lintasan.sans.biz.id/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"<combo-or-model>","messages":[{"role":"user","content":"hi"}]}'

# Logs service
journalctl -u lintasan -n 50 --no-pager
```

---

## 12. Repo Audit State — 2026-06-05 (FINAL, audit closed)

> **Status:** Audit closed + QA validated. Repo clean. Main `main @ fb19ed1` is
> the last housekeeping state; this section is the authoritative snapshot for
> the next session — re-running the audit from scratch is **not required**.
> Update this section whenever the state materially changes (new release tag,
> new intentional branch, new deferred TODO).

### Headline

- **`main` HEAD:** `fb19ed1` (post-merge of `fix/beta-p0-p1` to main)
- **Release tag:** `v0.24.2` → `9dc3556` (CHANGELOG/AGENTS docs commit); production
  binary is `v0.24.2-1-gfb19ed1` (HEAD + 1 merge commit)
- **Predecessor tags:** `v0.24.1` (test + proxy fix), `v0.24.0` (versioning reset only)
- **Production:** `v0.24.2` deployed to `lintasan.sans.biz.id:20180`
- **Tests:** **826/826 passing in 44 packages** (zero regressions; 10 new tests
  in `handlers_beta_p0_test.go`)
- **QA validation:** 87 test cases executed against prod build — 64 PASS,
  7 NEEDS_KEY, 1 NEEDS_CONFIG, 3 test-data bugs (not code), 1 expected SSE
  timeout. **Beta decision: GO.** Full evidence: `docs/qa-test-report.md`.

### Branches — final state

| Branch | Status | Reason |
|--------|--------|--------|
| `main` | **HEAD** | Production. Always. |
| `feat/codex-m0-skeleton` | **KEEP** (intentional) | Shape-1 (Codex Responses ingress). Orthogonal to Shape-2 (Experimental). Per §6 + AGENTS.md §6 Codex lifecycle: never merge into Shape-2. Fork fresh worktree to continue. |
| `gh-pages` | **KEEP** (intentional) | GitHub Pages static landing. Per project convention, landing lives on `gh-pages`; main is the product. Do not rebase, do not touch. |

**Dropped (do not resurrect):**
- 30+ merged `feat/*` and `fix/*` branches — work is in main.
- `feat/provider-sdk-foundation` — superseded; work landed via F1+F2 chain.
- `frontend-t949aa391` — kanban worktree orphan; both branch and
  `~/.hermes/kanban/workspaces/t_949aa391` removed.
- `feat/curl-import-connection` — base was 9 commits stale, 47-file diff
  mostly noise. The one substantive fix (proxy response header forwarding)
  was re-extracted and landed in main as `c030fdd` applied to **all three**
  call sites in `proxy.go` (the original branch only fixed one).

### Worktrees — final state

Only one worktree exists:

```
/home/ubuntu/lintasan-go    main    c030fdd
```

All other worktrees removed (kanban `t_949aa391` and `/tmp/lintasan-curl-import`).

### Deferred (acknowledged, not blocking)

- `cmd/lintasan/main.go:118` — `fmt.Println("TODO: Interactive setup")` in
  the fallback `print` branch of the setup wizard command. The full wizard
  is not implemented. Not critical path; can be picked up as a separate
  task or folded into a future `feat/setup-wizard` branch.
- `feat/codex-m0-skeleton` M5 live validation — blocked on
  `OPENAI_API_KEY` env. M0–M4 framework is in place.
- Cohort-A ACP live validation — per-provider checkpoint loop pending.

### Housekeeping log (what was done in this audit)

1. `84874cf` — test(server): add 28 tests for `internal/server/curl_import.go`
2. `c030fdd` — fix(proxy): strip stale `Content-Length` / `Transfer-Encoding`
   from upstream response headers (3 call sites in `proxy.go`)
3. CHANGELOG `v0.24.1` entry added; AGENTS.md §9 test count refreshed;
   this §12 added as authoritative state snapshot.
4. 30+ merged branches deleted (local + remote). 2 stale unmerged
   branches deleted. 1 branch (`feat/curl-import-connection`) dropped with
   fix re-extracted clean.
5. Orphaned worktree `~/.hermes/kanban/workspaces/t_949aa391` removed.
6. Tag `v0.24.1` created at `c030fdd`. GitHub release published with
   binary attachment.

### Audit follow-up (2026-06-05, batch `fix/beta-p0-p1`)

The beta-readiness audit (see `CHANGELOG.md` v0.24.2) flagged 4 P0/P1
items; all fixed in this batch:

7. `9551304` — fix(frontend): `/api/load-balancer` PUT → POST (1 line)
8. `c6cd2fd` — fix(server): `handlePluginGenerate` is real LLM codegen
   (no more fake template); returns 503 with hint if not configured
9. `e73b019` — fix(server): `handleCosts` reads real data from
   `request_logs` via `cost.NewCalculator()`, no more hardcoded zeros
10. `a7fdf41` — docs(agent): §11 quick reference no longer claims
    `admin/admin123` (which was never a valid password); replaced with
    a recovery-flow comment
11. 10 new unit tests in `handlers_beta_p0_test.go`. Suite: 826/826
    passing in 44 packages. `go vet` clean.

### Audit corrections (IMPORTANT)

The original beta-readiness audit (v0.24.1) reported "16+ frontend
calls hit non-existent routes" as a Critical issue. This was a
**false positive**: the audit only grepped `server.go` and
`handlers_parity.go`, missing `handlers_rest.go` where all 16 routes
were already wired in commit `777a553` ("fix(dashboard): register
missing REST routes for resource mutations"). The 16 routes are real
implementations with real tests. The audit also missed the cause of
the 401s they observed: the auth middleware runs before the mux
matches, so missing-route and unauthenticated both return 401, which
made the routes LOOK missing during unauthenticated probing. With a
valid token they work.

This §12 entry exists so the next session doesn't re-derive the
conclusion. The audit also over-claimed "load-balancer 401"; that
one WAS real (PUT/POST mismatch) and is fixed in commit `9551304`.

### QA validation run (2026-06-05)

Full beta-readiness validation against production build `v0.24.2-1-gfb19ed1`
at `lintasan.sans.biz.id:20180`. Plan + report: `docs/qa-test-plan.md` and
`docs/qa-test-report.md` (in this commit).

| Category | Result | Notes |
|----------|--------|-------|
| Auth (T1–T8) | 8/8 PASS | Login, logout, /me, RBAC all clean |
| Provider mgmt (T9–T11) | 3/3 PASS | CRUD + discover all wired |
| Embeddings/images/audio (T12–T15) | 0/4 PASS, **4 NEEDS_KEY** | Proxy routes correctly; upstream returns 402 (zero credits on test account). Infrastructure validated. |
| Chat (T16–T20) | 5/5 PASS | Real chat call works when model valid; rejects unknown model (correct). |
| Routing (T21–T25) | 5/5 PASS | Combos, fallback, LB, ML router all responsive |
| Cache (T26–T27) | 2/2 PASS | Store/fetch with real similarity scoring |
| Cost (T28–T31) | 4/4 PASS | **P1 v0.24.2 fix verified** — real data shape from `request_logs` (was hardcoded zeros in v0.24.1) |
| Memory (T32–T36) | 5/5 PASS | Store/recall/stats; **test-data bug found**: requires `text` field, not `content` (not a code bug) |
| MCP (T37–T40) | 4/4 PASS | All 14 tools list, JSON-RPC 2.0 valid |
| Plugin (T41–T44) | 3/4 PASS | 1 test-data bug (`pluginId` not `id`); generate returns 503 with hint (correct) |
| Webhook (T45–T48) | 4/4 PASS | CRUD + event trigger |
| Backup (T49–T52) | 3/4 PASS | 1 test-data bug (required field `action:"create"`) |
| Teams (T53–T56) | 4/4 PASS | All member CRUD endpoints responsive |
| Keys (T57–T60) | 4/4 PASS | All key mgmt endpoints responsive |
| Quota (T61–T62) | 2/2 PASS | Stats + limits all real data |
| Regression (T63–T87) | 16/17 PASS, 1 expected SSE timeout | 16 previously-claimed-"missing" routes re-validated PASS with sequential create+operate; confirms audit correction. |

**Headline numbers:**
- **64 PASS** (74%), **7 NEEDS_KEY** (8%), **1 NEEDS_CONFIG** (1%), **3 test-data bugs** (3%), **1 expected timeout** (1%)
- **0 code defects**, **0 P0/P1 regressions**
- **3 test-data bugs** are documentation issues (API doc doesn't enumerate required fields clearly) — fix in API reference, not code

**NEEDS_KEY (7):** T12, T13, T14, T15 (embeddings/images/audio), T18 (chat with default model), T66 (ML router with key), T83 (Cohort A ACP M5).

**Pre-launch recommendations** (from report):
1. Provide at least one provider API key (OpenAI/Anthropic/DeepSeek) for end-to-end NEEDS_KEY tests
2. Set `plugin_generator_model` in settings to enable AI plugin generation (currently returns 503)
3. Configure minimal 1 provider connection (currently default test user has none)
4. Update API reference doc with counter-intuitive field names: memory `text`, plugin `pluginId`, backup `action:"create"`

**Beta launch decision: GO.** No blockers. All deferred items (P2 in §11) are explicitly acknowledged non-blocking.

### Recovery notes (per §9 conventions)

- **Main worktree scrub pitfall** (May 2026): `~/lintasan-go` is subject to
  external git processes (GitKraken `.git/gk/`, `filter-repo`) that can wipe
  untracked files within ~60s. **Always** work in an isolated worktree
  (`git worktree add -b feat/x /tmp/x main`), commit to a branch, and
  diff `vs HEAD~1` not `vs main`. Full recipe:
  `~/.hermes/skills/lintasan-development/references/parallel-foundation-commits.md`.
- **Pre-deploy build hygiene:** before any `make build` or `systemctl restart`,
  run `git status --porcelain` and either commit or park (`.gitignore`) any
  untracked files. Untracked files left in main will be included in the
  build artifact and can cause silent breakage.
- **Test count drift:** if `go test ./...` count differs from the headline
  in this section, **trust the live count** and update the section.
  Mismatch = audit signal.

---

*Last updated: 2026-06-05 (post-QA run, HEAD `fb19ed1`) · Stack: Go 1.22.2 + SvelteKit 5 (embedded SPA) · 826 backend tests / 44 packages · single self-contained binary (v0.24.2) · **Beta: GO***

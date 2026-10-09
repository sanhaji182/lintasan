# CLIProxyAPI Parity untuk Lintasan

**Tanggal:** 2026-10-09  
**Status:** Implementasi kandidat selesai di tiga branch terisolasi; belum diintegrasikan atau dideploy  
**Pemilik keputusan:** Operator Lintasan

---

## 1. Ringkasan

[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) menyajikan endpoint kompatibel OpenAI, Claude, Gemini, Codex, dan Grok di atas akun CLI/subscription yang masuk lewat OAuth. Nilai utamanya bukan sekadar format API, tetapi pengelolaan beberapa akun, rotasi kredensial, refresh token, streaming, dan failover.

Lintasan tidak perlu mengkloning proyek itu. Audit pada baseline `1c0f7ad` menunjukkan sebagian besar fondasi sudah ada: delapan alur OAuth IDE, refresh token, penyimpanan sesi, overlay token ke connection, translator lintas format, load balancer multi-account, dan ingress Codex Responses API. Pekerjaan parity yang relevan dipersempit menjadi empat celah:

1. pool dan round-robin antar **sesi OAuth** satu provider;
2. satu sumber kebenaran untuk gate OAuth IDE antara dashboard dan hot path;
3. observabilitas akun dan status wire yang jujur di dashboard;
4. ingress native Claude Messages dan Gemini generateContent, default nonaktif.

Semua perubahan dirancang additive. Produksi tidak berubah selama flag terkait nonaktif.

---

## 2. Audit fondasi yang sudah ada

Audit dilakukan terhadap source baseline `1c0f7ad`, bukan berdasarkan README atau asumsi.

| Kapabilitas | Status baseline | Bukti source |
|---|---|---|
| Delapan provider OAuth IDE: Claude, Antigravity, Codex, GitHub, Cursor, xAI, Kilo Code, Cline | Ada | `internal/oauthide/catalog.go:42-62` |
| PKCE, device-code, dan token-import | Ada | `internal/oauthide/pkce_flow.go`, `internal/oauthide/github.go`, `internal/oauthide/kilocode.go`, `internal/oauthide/cursor.go` |
| Refresh token xAI, Claude, Codex, Antigravity | Ada | `internal/auth/oauth_refresh.go:21-71` |
| Penyimpanan sesi OAuth | Ada | schema `oauth_sessions` di `internal/db/db.go:173-183` |
| Token sesi di-overlay ke connection | Ada, tetapi gate baseline divergen | `internal/server/oauth_proxy_wire.go:13-37` |
| Dashboard OAuth IDE | Ada | `frontend/src/routes/dashboard/oauth-ide/+page.svelte` |
| Round-robin multi-account untuk API-key connection pool | Ada pada level connection, bukan sesi OAuth | `internal/lb/multi_account.go`; `internal/server/oauth_proxy_wire.go:53-115` |
| Translasi OpenAI ↔ Anthropic/Gemini/Cohere/Mistral | Ada | `internal/translator/detect.go:8-21`, `anthropic.go`, `gemini.go` |
| Ingress Codex `/v1/responses` | Ada, gated | `internal/server/handlers_responses.go:36-109`; `responses_gate.go:7-32` |
| Ingress native Claude `/v1/messages` | Belum ada | baseline `internal/server/server.go:210-226` hanya mendaftarkan chat, embeddings, responses |
| Ingress native Gemini `/v1beta/...` | Belum ada | tidak ada route pada baseline `internal/server/server.go` |

### Koreksi atas asumsi awal

- Lintasan **sudah** mempunyai multi-account load balancer, tetapi pool tersebut bekerja pada `connections.pool_id`, bukan beberapa baris `oauth_sessions` milik provider yang sama.
- Lintasan **sudah** mempunyai translator Anthropic/Gemini termasuk SSE. Kekurangannya adalah ingress native, bukan mesin translasi baru.
- Ingress Codex bukan scaffolding kosong lagi; baseline `handlers_responses.go` sudah menerjemahkan request, buffered response, dan stream ke Responses API.

---

## 3. Celah P1 — pool akun OAuth

### Masalah baseline

`internal/auth/oauth_proxy.go:91-113` mengambil satu sesi terbaru:

```sql
SELECT ... FROM oauth_sessions
WHERE provider = ? AND status = 'active'
ORDER BY datetime(created_at) DESC LIMIT 1
```

Tiga akun aktif tetap berperilaku seperti satu akun. Jika akun terbaru ditolak upstream, akun sehat lain tidak dipakai.

### Desain

- Ambil semua sesi `active` satu provider dengan urutan deterministik.
- Simpan cursor round-robin per provider di memori.
- Kembalikan `SessionID` bersama token agar respons 401/403 dapat diatribusikan ke akun yang benar.
- 401/403 menandai sesi `restricted` dan mengeluarkannya dari rotasi.
- Status `restricted` tetap tersimpan dan terlihat; sistem tidak menyembunyikan pembatasan upstream.
- Dengan satu akun, perilaku tetap identik dengan baseline.

### Implementasi kandidat

Branch `agent/cliproxy-backend`, commit `0488d99`:

- `internal/auth/oauth_pool.go`
- `internal/auth/oauth_proxy.go`
- `internal/server/oauth_proxy_wire.go`
- `internal/server/proxy.go`

Test membuktikan tiga akun dipilih bergantian, akun restricted tidak dipilih ulang, dan single-account tetap stabil.

---

## 4. Celah P2 — gate wire yang divergen

### Masalah baseline

Dashboard/API memanggil `(*Server).oauthIdeEnabled()`, yang membaca setting DB `oauth_ide_enabled` dan baru fallback ke config (`internal/server/oauth_ide_gate.go:43-53`).

Hot path proxy justru membaca `p.cfg.OAuthIDEEnabled` langsung (`internal/server/oauth_proxy_wire.go:23`). Nilai config dilatch saat boot dari env. Kondisi nyata yang diaudit:

- setting DB `oauth_ide_enabled = true`;
- env `LINTASAN_OAUTH_IDE_ENABLED` tidak ada;
- dashboard mengatakan aktif dan tombol Wire dapat membuat connection;
- hot path menganggap nonaktif dan tidak menempelkan token OAuth.

Akibatnya connection terlihat ter-wire, tetapi request memakai `api_key` kosong.

### Desain

Ekstrak satu resolver gate bersama:

1. boolean valid di setting DB menang;
2. bila DB belum disimpan, gunakan config/env startup;
3. dashboard dan proxy hot path memanggil resolver yang sama.

Implementasi kandidat ada pada commit `0488d99`, terutama `internal/server/oauth_ide_gate.go` dan `oauth_proxy_wire.go`. Test khusus membuktikan DB `true` + env/config `false` tetap menempelkan token, dan DB `false` mengalahkan env/config `true`.

---

## 5. Celah P3 — UI akun, health, dan wire state

### Masalah baseline

Halaman OAuth IDE menampilkan daftar sesi datar dan menyebut POST provision-connection sebagai sukses. Itu tidak menjawab:

- berapa akun sehat per provider;
- akun mana restricted/expired;
- apakah connection benar-benar wired dengan akun aktif;
- apakah quota memang diketahui.

### Kontrak API

Commit backend `0488d99` menambah:

```json
{
  "enabled": true,
  "accounts": [
    {
      "id": "...",
      "provider": "codex",
      "status": "active",
      "health": "healthy",
      "expires_at": "...",
      "masked_token": "abcd…wxyz"
    }
  ],
  "providers": [
    {
      "provider": "codex",
      "name": "OpenAI Codex",
      "active": 2,
      "expiring": 0,
      "restricted": 1,
      "expired": 0,
      "revoked": 0,
      "total": 3,
      "wired": true,
      "connection_id": "...",
      "connection_name": "oauth-codex"
    }
  ]
}
```

Access token dan refresh token tidak pernah dikirim. Hanya hint bertopeng yang tersedia.

### UI kandidat

Branch `agent/cliproxy-frontend`, commit `5834fc7`:

- grup akun per provider;
- status `Healthy`, `Expiring`, `Restricted`, `Expired`, `Revoked` tetap terlihat;
- badge `Not wired`, `Wired & active`, atau `Wired · token error`;
- tombol Wire melakukan reload dan memverifikasi state, bukan langsung memberi toast sukses palsu;
- quota tampil `–` jika backend tidak mempunyai angka nyata.

---

## 6. Celah P4 — ingress native Claude dan Gemini

### Desain

Ingress baru hanya shim di tepi sistem:

```text
request native
    ↓ translator yang sudah ada
OpenAI chat canonical
    ↓ pipeline Lintasan yang sudah ada
routing → combo → fallback → cache → logging
    ↓ translator yang sudah ada
response native
```

Tidak ada provider, registry, atau routing table baru.

### Surface dan gate

| Surface | Setting DB | Default |
|---|---|---|
| `POST /v1/messages` | `claude_messages_api_enabled` | `false` |
| `POST /v1beta/models/{model}:generateContent` | `gemini_native_api_enabled` | `false` |
| `POST /v1beta/models/{model}:streamGenerateContent` | `gemini_native_api_enabled` | `false` |

Gate dibaca per request sehingga operator dapat mematikan surface tanpa restart. Flag nonaktif menghasilkan 404. Semua path tetap melewati middleware auth fail-closed.

Streaming diterjemahkan baris per baris dan di-flush segera; tidak diaggregate menjadi satu body.

Implementasi kandidat: branch `agent/cliproxy-ingress`, commit `8fc1493`, terutama `internal/server/handlers_native_ingress.go` dan `native_ingress_gate.go`.

---

## 7. Keputusan operator

Agent tidak mengaktifkan surface berisiko secara otomatis.

| ID | Keputusan | Risiko | Rekomendasi default |
|---|---|---|---|
| F1 | Izinkan beberapa akun subscription untuk provider yang sama | Multi-account routing dapat bertentangan dengan ketentuan upstream dan meningkatkan risiko pembatasan akun | Tetap experimental; BYO personal; tampilkan `RiskNotice`; operator menambah akun secara sadar |
| F2 | Aktifkan failover otomatis antar sesi OAuth | Failover bisa menyamarkan akun yang sedang dibatasi jika statusnya tidak terlihat | Terima hanya dengan status `restricted` persisten dan terlihat; restricted tidak auto-retry |
| F3 | Buka ingress native Claude/Gemini di produksi | Menambah permukaan API dan kombinasi payload yang harus dipelihara | Tetap OFF hingga operator mengaktifkan masing-masing setting setelah review |
| F4 | Sediakan client ID/secret Antigravity | Secret Google OAuth tidak boleh masuk repository | Tetap lewat env/operator secret; jangan commit; jangan mengaktifkan bila secret belum tersedia |

---

## 8. Batasan operasional

1. Produksi read-only selama implementasi dan review.
2. Tidak menyentuh binary `./lintasan`; build kandidat hanya ke output build/worktree.
3. Tidak restart service atau menjalankan `make deploy` tanpa persetujuan eksplisit.
4. `gofmt` hanya file yang diedit.
5. Snapshot DB harus dibuat dengan `VACUUM INTO`, bukan `cp`, dan permission 700/600.
6. Perubahan additive; flag off harus inert.
7. `oauthide.RiskNotice` tetap dipertahankan.
8. UI tidak boleh mengarang quota, kesehatan, atau keberhasilan wire.

---

## 9. Rencana verifikasi

### Unit dan build

- 3 sesi aktif → 3 resolve berturut-turut memakai 3 session ID berbeda.
- sesi yang kena 401/403 → status `restricted`, resolve berikutnya memakai sesi lain.
- satu sesi → hasil stabil seperti baseline.
- DB gate `true`, env/config `false` → token OAuth terpasang.
- DB gate `false`, env/config `true` → kredensial statis tetap dipakai.
- `/api/oauth/accounts` tidak pernah memuat access/refresh token.
- flag ingress off → 404.
- flip setting DB → surface aktif tanpa restart.
- Anthropic buffered response mempunyai `type: message`, `content[]`, `stop_reason`.
- Gemini buffered response mempunyai `candidates[].content.parts[]`.
- stream Claude/Gemini keluar incremental dan mempunyai terminal event.
- `go build ./...`, `go test ./internal/...`, `npm run check`, dan `npm run build` lulus.

### Runtime review non-prod

- Jalankan binary kandidat pada port terpisah dan `LINTASAN_DATA_DIR` terpisah.
- Gunakan snapshot konsisten: `sqlite3 SRC "VACUUM INTO 'DST'"`.
- Uji tanpa auth → 401 untuk `/v1/*` dan `/v1beta/*`.
- Uji dengan auth + flag off → 404.
- Uji dengan flag on terhadap mock upstream → respons native dan streaming benar.
- Kirim canary secret pada header/path; grep seluruh response/log/metrics untuk memastikan tidak bocor.
- Matikan instance review dan konfirmasi PID/version produksi tidak berubah.

---

## 10. Kriteria selesai

- [x] Backend kandidat: `8ad7ff0` (supersedes `0488d99`), build sukses, 1.525 internal test lulus pada lane tersebut.
- [x] Ingress kandidat: `01f78f9` (supersedes `8fc1493`), 484 server test dan build lulus.
- [x] UI kandidat: `5834fc7`, focused test, check tanpa error, dan production build lulus.
- [ ] Review independen backend selesai tanpa blocker.
- [ ] Tiga branch diintegrasikan dan seluruh test dijalankan ulang dari satu SHA.
- [ ] Runtime review non-prod membuktikan auth, dormansi, format, dan redaksi.
- [ ] Operator menandatangani keputusan F1–F4.
- [ ] Deploy hanya setelah persetujuan eksplisit dan verifikasi kandidat.

---

## 11. Rollback

Karena ketiga surface baru default OFF, rollback awal adalah mematikan setting:

```text
oauth_ide_enabled=false
claude_messages_api_enabled=false
gemini_native_api_enabled=false
```

Jika binary sudah terdeploy dan perlu rollback penuh, gunakan backup yang dibuat target deploy resmi; jangan salin binary secara manual. Verifikasi `/health`, PID/start time, `NRestarts`, dan route protected setelah rollback.

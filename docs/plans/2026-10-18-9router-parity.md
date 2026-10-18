# 9router v0.5.104 → Lintasan Go Parity Plan

**Tanggal:** 2026-10-18
**Status:** DESIGN — menunggu pilihan scope (A / B / C / ABC)
**Sumber:** github.com/decolua/9router @ 87a74e1 (v0.5.104, 2026-10-15), clone di `/tmp/9router-src`
**Target:** `~/lintasan-go` (main @ def8390, sync dengan origin/main; qoder-suite WIP di stash tidak disentuh)

---

## 1. Metode

Setiap item di bawah diverifikasi membaca kode kedua repo, bukan dari CHANGELOG.
Kaidah: hanya item yang sudah terbukti barisnya yang masuk tabel. Yang perlu
probe lebih lanjut ditandai `?`.

---

## 2. Sudah diport (bukan gap — jangan kerjakan dua kali)

| 9router | Lintasan | Bukti |
|---|---|---|
| Registry cross-ref (54 preset) | `internal/server/handlers_presets.go` `seedCatalogue()` | komentar "cross-referenced from the 9router registry" |
| Export/import migrasi | `internal/migrate/ninerouter.go` + `_test.go` | header "parses a 9router backup export" |
| OAuth IDE catalog v0.4.71 (8 prov) | `internal/oauthide/catalog.go` | konstanta `RiskNotice`, 8 entri impl-ready |
| xAI PKCE | `internal/oauthide/xai.go`, `xai_redirect.go` | loopback listener port dari 9router |
| Trae adapter (in-process) | `internal/trae/` (9 file) | `Format == "trae"` memakai adapter |
| WorkBuddy adapter | `internal/workbuddy/` | `experimental_adapters.go` register keduanya |
| CC Alpha / commandcode | `internal/server/proxy.go` branch `conn.Format == "commandcode"` | 6 titik cabang |
| Translator 5 format | `internal/translator/` (openai/anthropic/gemini/cohere/mistral) | pivot OpenAI, mirroring `open-sse/translator` |
| RTK compressor | `internal/compress/rtk.go` + `internal/rtk/` | `CompressRTK`, `Caveman` ada |
| Hedged upstream / stream failover | `internal/server/proxy.go:728` `doHedgedUpstream`, `/server/STREAM_FAILOVER.md` | — |
| Multi-account LB + cooldown auth | `internal/lb/multi_account.go` | `AuthFailed` vs `RateLimited` terpisah |
| Qoder native provider | branch `isQoder(...)` di `doUpstream` + `internal/qoder/` | gate `qoder_enabled` |
| Error classify (status-based) | `internal/lb/multi_account.go` cooldown + `classifyTask` | cukup untuk status; teks-based masih gap (lihat B) |

---

## 3. Gap lens A — Provider executable parity (feature-scope)

Gap = koneksi `openai/claude/gemini format` Lintasan tidak bisa menyediakan
wire-protocol yang sama dengan 9router. Semua masuk lewat pola
`internal/trae` → `experimental_adapters.go` (Track=Experimental, membrane
gated, TIDAK pernah dipilih auto-routing) — zero changes to shared path.

| # | 9router source | Target (proposed) | Kind | Flag/gate |
|---|---|---|---|---|
| A1 | `open-sse/providers/registry/kiro.js` (`format:"kiro"`, EventStream) + `executors/kiro.js` (51k) + `translator/claude-to-kiro.js`,`openai-to-kiro.js` | `internal/kiro/` | EventStream binary ⚠ tidak round-trip OpenAI — seperti precedent commandcode | `kiro_enabled` DB setting, default false |
| A2 | `providers/registry/cursor.js` (52k, protobuf) + `executors/cursor.js` | `internal/cursor/` | protobuf, tidak round-trip | `cursor_enabled`, default false |
| A3 | `providers/registry/windsurf.js` + `executors/windsurf.js` (22k, firebase + codeium gRPC-ish) | `internal/windsurf/` | bukan aman untuk default; perlu research `firebaseSignInUrl` dulu | `windsurf_enabled`, default false |
| A4 | `providers/registry/zed.js` (RSA 2048 callback) + `executors/zed.js` (11k) | `internal/zed/` | RSA keypair + local TCP callback | `zed_enabled`, default false |
| A5 | `providers/registry/devin-cli.js` (authtype none, ACP-stdio 33k executor) + `executors/devin-cli.js` | `internal/expprovider` Cohort-A descriptor (bukan 9r-clone, pakai framework Substrate) | ACP-stdio lifecycles | descriptor + fixture, admit-flow existing |
| A6 | `providers/registry/glm.js` (Z.ai OAuth CLI-poll, 2 lanes openai+claude) + `src/lib/oauth/providers/glm.js` (10.8k) | `internal/glm/` | dual-auth OAuth+apikey, endpoint `api.z.ai/api/anthropic/v1/messages` | `glm_enabled`, OAuth masuk lens C |
| A7 | `providers/registry/opencode-zen.js` + `executors/opencode-zen.js` (12k, 3 transports) | preset sudah ada for `opencode.ai/zen/go/v1`; tambah lane `/zen/v1/{chat,messages,responses}` | kecil, boolean | preset tambah + conn `format` |
| A8 | `providers/registry/mimo-free.js` + `executors/mimo-free.js` | `internal/mimofree/` | fingerprint check | default false |
| A9 | `providers/registry/antigravity.js` + `executors/antigravity.js` (24k native) | Lintasan sudah punya OAuth-lab antigravity; native executor beda | Cukup OAuth → C | C |

Urutan recommended: **A7 → A6 → A5 → A8 → A3 → A4 → A1 → A2**
(kecil/b-safe dulu, protobuf paling akhir).

---

## 4. Gap lens B — Data-layer parity (infra-scope, kalau mau tanpa adapter)

| # | 9router | Lintasan | Gap & aksi |
|---|---|---|---|
| B1 | `open-sse/config/errorConfig.js` — `ERROR_RULES` 16 rule, teks-first lalu status | `internal/lb/multi_account.go` hanya cooldown per status | Port `ERROR_RULES` ke `internal/lb/classify.go` (baru). Teks-regex per provider; fail-open kalau provider tidak dikenal |
| B2 | `open-sse/rtk/` mutator `fail-open` yang menulis ke body in-place, per-tool filters 9 file | `internal/compress/pipeline.go` ada, tidak menyentuh tool_result in-place | `internal/compress/rtk_inplace.go` baru; wajib fail-open (test: rtk throw tidak meneruskan error) |
| B3 | `scripts/migrate-registry.mjs` (auto-gen `registry/index.js`, tidak hand-edit) | preset list hard-coded di `handlers_presets.go` | Buat `scripts/gen-presets.mjs` yang baca `9registry.keys` (CSV statik) dan cek base URL pakai probe (sudah pernah dilakukan manual) |
| B4 | `HEADROOM` external compress proxy (`lib/headroom/`) | `_compress/pipeline.go` internal saja | OPTIONAL; skip kalau scope A sudah besar |
| B5 | 173 Next.js route handler di `src/app/api` | 70+ route di `internal/server` | Tidak 1:1. Beda payung; tidak dijadikan target parity. Sudah diag "gap" oleh arsitektur |

---

## 5. Gap lens C — OAuth-IDE next phase (subscription-account lab)

Status: `docs/oauth-ide-experimental.md` mematok versi 9router **v0.4.71**;
sudah ~7 rilis tertinggal. Perbedaan siap dibawa dengan tetap memelihara
`IsKnownProvider` sebagai single-truth dan `oauth_ide_enabled` sebagai satu gate.

Provider baru di 9router sejak itu:

| 9r id | kind | flow | Kesiapan port | Rencana |
|---|---|---|---|---|
| `muse` | OAuth LLM | device | `ImplReady` baru setelah probe | Masuk catalog, impl=planned dulu |
| `glm` (Z.ai) | dual OAuth+apikey | CLI-poll | butuh poll-engine baru | impl=planned, tulis spec |
| `grok-cli` | OAuth | device | silang-cek vs `xai` yang sudah Ready | `ImplReady` |
| `gemini-cli` | device | device | kandidat porting pasti | `ImplReady` |
| `antigravity` | sudah ada | authcode | sudah ada; update saja bila ada drift | bump |
| `iflow` | OAuth | authcode+cookie | perlu probe | `ImplReady` atau import-only |
| `qoder-cn` | OAuth | device | sudah ada Qoder non-IDE | catalog saja |
| `minimax-code(+global)` | OAuth | device | perlu probe | planned |
| `nous` (Hermes-CLI) | OAuth | device | paling dekat (user running Hermes) | **prioritas tinggi**, impl=planned → ready |
| `cursor` | OAuth | import `state.vscdb` | sudah import-only di Lintasan | sudah ada |
| `kimi`, `kilocode`, `cline`, `clinepass`, `codebuddy-*`, `kimchi` | OAuth | mix | beberapa sudah ported | merge/sync |

Aksi C:
1. Bump `internal/oauthide/catalog.go` versi 8 → ~21 entri dengan status impl yang
   **jujur** (planned kalau belum dikerjakan).
2. Add `internal/auth/oauthide/nous.go` (device flow, NOUS_CONFIG dari 9router).
   Ini bugfix lapisan pratique: Hermes sendiri sudah punya profil Hermes di 9r.
3. Update `docs/oauth-ide-experimental.md` + `frontend/src/routes/dashboard/oauth-ide/`
   list otomatis dari `/api/oauthide` (bukan hard-code 8).

---

## 6. Fase implementasi & urutan

| Fase | Isi | Risk | Flag default |
|---|---|---|---|
| P0 (siap duluan) | Lens B1+B3 (data-layer, tanpa per-port native) | rendah, tanpa runner | semua setting false |
| P1 | Lens A8, A7, A5 (devin via ACB framework), A6 APIkey lane saja | sedang | semua false |
| P2 | Lens C1-C2 (catalog bump + nous device flow) | sedang | `oauth_ide_enabled` tetap gate lama |
| P3 | Lens A3, A4 (windsurf, zed — butuh research) | tinggi | semua false |
| P4 | Lens A1, A2 (kiro, cursor — binar-biner) | tertinggi, jangan sampai ke prod tanpa greenline | semua false; kiro & cursor di eksport hanya bila butuh |

**Semua fase TIDAK MENYENTUH jalur routing default sampai flag di-ON**
- flag semuanya read dari DB `settings` (pola `provider_sdk_enabled` yang sudah ada),
- tidak ada yang register otomatis ke `providerReg` kecuali lewat
  `experimental_adapters.go` (Track=Experimental),
- tidak ada yang jadi `providerSDKEligible` kecuali via `Registry.Resolve` name-based.

---

## 7. Keputusan yang masih terbuka (perlu angka dari Sans)

1. **Kanji antara A5 (devin via ACB framework) atau versi in-process** —
   framework ACB sudah ok, tapi bisa daftar sebagai `TrackExperimental`
   in-process seperti Trae, jadi tidak butuh runner.
2. **Windsurf (A3) dan Zed (A4) pakai flow berbasis RSA-TCP dan firebase** —
   potensi bandel soal trust, mungkin tidak diajalankan sampai butuh.
3. **C seberapa jauh** — tambah katalog saja via `GET /api/oauthide` (murah),
   atau sampai OAuth flow `nous` Ready (butuh OAuth HTTP hook baru)?
4. **Urutan P0-P4** mau dijalankan serentak oleh 3 worktree (sesuai kaidah
   multi-agent Lintasan), atau berurutan di 1 worktree?
5. Semua fase akan ditulis sebagai `kanban_create` di board Lintasan; assignee
   perlu disepakati (nama profil harus sudah ada di server, tidak bisa karang).

---

## 8. Acceptance (umum utk semua fase)

1. `go test ./...` pass (target >= 3000 tests existing).
2. `internal/provider/default_provider.go` tidak berubah semantik.
3. Provider baru mematuhi kontrak `provider.Provider` + `Track()==Experimental`
   (kecuali presety A7).
4. Tidak ada file di bawah `internal/server/proxy.go` yang berubah kecuali:
   - penambahan 1 cabang `conn.Format == "<name>"` + import baru;
   - atau tambah 1 register di `experimental_adapters.go`.
5. Tidak ada perubahan prod tanpa `make deploy` + health check + verifikasi
   binary dari `/health` path (kaidah prod — bukan bagian plan, hanya kon strain).

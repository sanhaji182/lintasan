# Rencana Konsolidasi Menu Dashboard Lintasan

> **Status:** DRAFT untuk review. Tidak ada kode yang diubah. Prod read-only.
> **Tanggal audit:** 11 Aug 2026
> **Sumber bukti:** heading tiap `+page.svelte` + endpoint backend yang dipanggil.
> **Kondisi awal:** 26 item nav (13 MENU / 7 MANAGE / 6 TOOLS).
> **Target:** ~18 item, tanpa menghapus fungsi apa pun (semua jadi tab/sub-section).

---

## 0. Prinsip

- **Tidak ada fitur yang hilang** — halaman yang di-merge menjadi tab/section, bukan dihapus.
- **Live UI tetap sama sampai approval.** Redesign dikerjakan di worktree, expose preview URL, deploy hanya dengan approval eksplisit.
- **Route lama tetap ada** (redirect ke tab baru) selama ≥1 rilis supaya bookmark/deep-link user tidak mati → lihat `orphan-route-guard`.
- Perubahan murni frontend (nav + wrapper tab). **Endpoint backend TIDAK diubah** di fase ini.

---

## 1. Peta duplikat (temuan)

| # | Halaman | Endpoint | Overlap dengan | Kenapa duplikat |
|---|---------|----------|----------------|-----------------|
| 1 | **Usage** | `/api/usage`, `/api/quota/stats` | Analytics, Savings | Provider/Model Breakdown muncul di Usage & Analytics |
| 2 | **Analytics** | `/api/dashboard/stats`, `/api/logs` | Usage, Logs | Aggregasi dari `/api/logs`; breakdown provider/model sama |
| 3 | **Savings** | `/api/savings/*` | Usage/Analytics | Metrik biaya = bagian dari statistik request |
| 4 | **Logs** | `/api/logs` | Analytics | Sumber data identik; Analytics = view teragregasi |
| 5 | **Routing** | aliases, combos, load-balancer, smart-routing | Fallback | Sama-sama "rantai model→provider" |
| 6 | **Fallback** | `/api/fallback/*chains` | Routing | Fallback = jenis routing chain |
| 7 | **Discover** | `/api/discover/free-providers` → bikin connection | Connections | Entry point tambah provider |
| 8 | **OAuth IDE** | `/api/oauth/*`, `/api/oauth/provision-connection` | Connections | Panggil endpoint provision yg SAMA dgn Connections |
| 9 | **Experimental** | `/api/experimental/*` | Connections | Jenis provider lain, entry point kelola provider |
| 10 | **Observability** | `/v1/memory/stats` (+scan/HTTP) | Memory | Satu-satunya EP-nya milik Memory |
| 11 | **MCP** vs **Plugins** | beda EP | (ikon) | Ikon `Puzzle` identik → terlihat duplikat |

---

## 2. Target struktur nav (26 → 18)

### MENU
1. Overview (`/dashboard`) — tetap
2. **Connections** — jadi hub provider, dgn tab:
   - *Accounts* (existing connections)
   - *Discover* (free providers) ← dari `Discover`
   - *OAuth IDE* ← dari `oauth-ide` (LAB)
   - *Experimental* ← dari `experimental` (LAB)
3. Providers (`/dashboard/providers`) — tetap (katalog model, beda dari Connections)
4. **Routing** — dgn tab:
   - *Aliases / Combos / Load Balancer / Smart Routing* (existing)
   - *Fallback* ← dari `fallback`
5. **Analytics** — dgn tab:
   - *Requests* (status/provider/model breakdown) ← dari `analytics`
   - *Usage & Quota* ← dari `usage`
   - *Savings* ← dari `savings`
   - *Logs* (raw drill-down) ← dari `logs`
6. Memory (`/dashboard/memory`) — serap stats dari `observability` (lihat §3)
7. Observability — **KEEP hanya jika** scan-load & HTTP traffic memang fitur nyata; kalau isinya cuma memory stats → gabung ke Memory & hapus dari nav

### MANAGE (tetap 7)
API Keys · Teams · User Management · Webhooks · Backup · Migrate · Settings

### TOOLS (6 → 6, hanya ganti ikon)
MCP Server (ganti ikon, mis. `Server`/`Plug`) · Savings *(pindah ke Analytics — hapus di sini)* · Translator · Plugins (`Puzzle`) · Playground · Docs

> Catatan: `Savings` di TOOLS dan sebagai tab Analytics jangan dobel — pilih Analytics.

**Hasil:** MENU 13→7, MANAGE 7, TOOLS 6→5 (Savings pindah) ⇒ **±18 item.**

---

## 3. Keputusan yang butuh input Sonickk

- [ ] **Observability**: hapus (merge stats ke Memory) atau pertahankan sebagai halaman infra (scan-load, HTTP traffic)? Perlu lihat apakah ada user yang pakai.
- [ ] **Experimental & OAuth IDE**: sebagai tab di Connections tetap ber-badge LAB? (default OFF, admin-only, copy Experimental jujur — sesuai preferensi ToS-gray).
- [ ] **Savings**: sebagai tab Analytics atau tetap TOOLS? (rekomendasi: Analytics).

---

## 4. Eksekusi (kalau plan disetujui) — frontend-only

1. Worktree `~/worktrees/hermes-eng` (multi-agent isolation), branch baru.
2. Bikin komponen `TabbedPage` reusable (URL-hash driven, deep-link aman).
3. Untuk tiap merge: pindahkan konten `+page.svelte` lama → komponen tab; route lama jadi redirect ke `?tab=`.
4. Update `Sidebar.svelte` (`menuItems`/`manageItems`/`toolItems`).
5. Ganti ikon MCP.
6. `make build` → `dist-bin/` → expose preview instance (isolated smoke) → kirim preview URL.
7. **Deploy hanya setelah approval eksplisit** (`make deploy`, backup otomatis).
8. Guard: `orphan-route-guard` (tak ada halaman jadi orphan) + 2 security boundary test tetap hijau.

---

## 5. Yang TIDAK dilakukan

- Tidak mengubah/menghapus endpoint backend.
- Tidak menyentuh path prod `./lintasan` (semua build ke `dist-bin/`).
- Tidak deploy tanpa approval.

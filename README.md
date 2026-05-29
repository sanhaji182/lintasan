# ⚠️ Lintasan (v1 — Node.js) — DEPRECATED

> **Proyek ini tidak dilanjutkan lagi. Seluruh pengembangan telah dipindahkan ke versi Go.**
> *This project is no longer maintained. All development has moved to the Go version.*

---

## 🔄 Migrasi ke Go / Migration to Go

Versi Node.js Lintasan telah digantikan oleh **Lintasan Go (v2)** — rewrite penuh dalam Go dengan performa 35x lebih cepat, RAM 30x lebih hemat, dan feature parity penuh.

➡️ **[lintasan-go →](https://github.com/sanhaji182/lintasan-go)**

| Metric | Node.js (v1) | Go (v2) |
|--------|-------------|---------|
| Binary size | 513MB (node_modules) | 24MB (single binary) |
| Memory | ~500MB | ~14MB |
| Cold start | 3-5s | <50ms |
| Provider presets | 27 | 113 (all LiteLLM) |
| Tests | JS (manual) | 373 passing |
| Dependencies | 800+ npm packages | go-sqlite3 only |

---

## 📦 Riwayat / History

Lintasan Node.js adalah proof-of-concept awal yang membuktikan konsep multi-provider LLM proxy gateway dengan 34+ fitur optimasi. Versi ini sukses mencapai **feature parity** dan berfungsi sebagai blueprint untuk rewrite Go.

**Pencapaian utama versi Node:**
- 25+ fitur optimasi (embedding cache, semantic cache, stream cache, circuit breaker, dll)
- Dashboard interaktif dengan real-time analytics
- Plugin system dengan AI generator
- Dual-mode CommandCode
- Benchmark: Lintasan 5 — 9Router 3

---

## 🛑 Status Akhir / Final Status

- Commit terakhir: `af1bd9d` — real provider favicons via Go proxy
- Status build: ✅ 0 errors
- Database: File SQLite terpisah (tidak kompatibel dengan Go v2)

---

<p align="center">
  <b>Lintasan</b> — Setiap Koneksi Punya Jalannya<br>
  <i>Every Connection Has Its Path</i><br><br>
  🔄 <b>Lanjut ke <a href="https://github.com/sanhaji182/lintasan-go">Lintasan Go (v2)</a></b>
</p>

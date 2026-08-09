# Migrasi 9router → Lintasan

**Tanggal:** 2026-08-09
**Status:** Design — menunggu implementasi
**Penulis:** hermes-cc (brainstorming bersama Sonickk)

---

## 1. Masalah

9router dan OmniRouter punya basis pengguna yang aktif. Pengguna yang ingin pindah
ke Lintasan saat ini harus memasukkan ulang setiap koneksi provider satu per satu
lewat dashboard. Untuk setup dengan puluhan koneksi dan belasan combo, ini
penghalang adopsi yang nyata — bukan karena Lintasan kurang mampu, tapi karena
biaya pindahnya terlalu mahal.

9router sudah punya tombol export yang menghasilkan satu file JSON berisi seluruh
konfigurasi. File itu adalah jalan masuknya.

## 2. Apa yang sebenarnya ada di file export

Analisis dilakukan terhadap file export asli
(`9router-backup-2026-08-09T08-25-57-402Z.json`, 1.18 MB, 973 koneksi), bukan
terhadap dokumentasi atau tebakan.

Struktur top-level:

| Key | Isi | Relevan? |
|---|---|---|
| `providerConnections` | 973 koneksi (kredensial + status) | ✅ inti |
| `providerNodes` | 13 endpoint custom `openai-compatible` | ✅ inti |
| `combos` | 12 alias multi-model | ✅ inti |
| `modelAliases` | peta alias model | ⬜ nanti |
| `customModels` | 1265 entri katalog model | ❌ abaikan |
| `apiKeys` | 9 kunci akses **ke** 9router | ❌ abaikan |
| `settings`, `proxyPools`, `mitmAlias`, `pricing` | konfigurasi internal 9router | ❌ abaikan |

Temuan penting: koneksi custom **membawa `baseUrl` sendiri** di
`providerSpecificData`, jadi importer tidak perlu melakukan join ke
`providerNodes`. Tiap baris koneksi berdiri sendiri.

### Bentuk data

```jsonc
// providerConnections[] — koneksi ke endpoint custom
{
  "id": "acb01e44-…",
  "provider": "openai-compatible-chat-f864e547-…",  // → id di providerNodes
  "authType": "apikey",
  "name": "Key 1",
  "priority": 1,
  "isActive": true,
  "apiKey": "sk-…",
  "errorCode": 402,                    // sinyal kesehatan
  "testStatus": "unavailable",         // sinyal kesehatan
  "providerSpecificData": {
    "baseUrl": "https://ai.genfity.com/v1",   // ← kuncinya di sini
    "prefix": "genfity",
    "apiType": "chat"
  }
}
```

## 3. Tiga kelompok koneksi

Klasifikasi ini yang menentukan seluruh desain:

**A. Custom `openai-compatible`** — membawa `baseUrl` sendiri. Portabel penuh.

**B. Built-in apikey** — 9router tahu endpoint-nya dari adapter bawaan; file
export tidak memuat URL-nya. Portabel **jika** Lintasan punya preset untuk
provider tersebut. Lintasan sudah punya tabel `provider_presets` (22 entri).

**C. OAuth** — `codex`, `qoder`, `grok-cli`, `cline`, `clinepass`. Butuh alur
login, refresh token, dan penyimpanan sesi. **Di luar cakupan** (keputusan
Sonickk, 2026-08-09). Mengetahui URL-nya tidak cukup; ini pekerjaan adapter, dan
mengejarnya akan menjadikan Lintasan kloning 9router.

### Preset baru yang perlu ditambahkan

Endpoint ditemukan dari bundle 9router, semuanya provider apikey biasa:

| Provider | base_url |
|---|---|
| `xiaomi-mimo` | `https://api.xiaomimimo.com/v1` |
| `poolside` | `https://inference.poolside.ai/v1` |
| `kilo-gateway` | `https://api.kilo.ai/api/gateway` |
| `nvidia` | `https://integrate.api.nvidia.com/v1` |

Hanya penambahan baris data ke `provider_presets` — tanpa kode adapter.

## 4. Masalah sebenarnya: sebagian besar data tidak layak pindah

Kesehatan koneksi pada file export nyata:

| Kelompok | Sehat | Mati / nonaktif | Total |
|---|---|---|---|
| Custom | 6 | 10 | 16 |
| Preset (sudah ada) | 6 | 3 | 9 |
| Preset (baru) | 4 | **921** | 925 |
| Tanpa endpoint | 1 | 4 | 5 |
| OAuth | — | — | 18 |

920 dari 922 koneksi `xiaomi-mimo` sudah mati (`errorCode: 404`,
`testStatus: unavailable`) — jelas kumpulan key hasil farming yang sudah habis.

**Konsekuensinya:** importer yang memindahkan semuanya mentah-mentah akan
menyuguhkan 950+ koneksi merah pada layar pertama pengguna baru. Kesan yang
timbul: "Lintasan rusak", padahal kerusakan itu warisan dari sumbernya.

Karena itu masalah desain di sini **bukan** "bagaimana memindahkan data", tapi
"bagaimana pengguna tidak salah paham ketika sebagian besar setup-nya memang
tidak layak dipindah".

## 5. Desain

### Prinsip

> Importer harus jujur, bukan mengesankan berhasil. Pengguna melihat persis apa
> yang akan masuk dan apa yang tidak, **sebelum** apa pun ditulis ke database.

### Alur

```
1. Pengguna unggah 9router-backup-*.json
2. Parse DI MEMORI — file tidak pernah menyentuh disk (berisi API key polos)
3. Klasifikasi tiap koneksi: kelompok (A/B/C) × kesehatan (sehat/mati)
4. Tampilkan PREVIEW — belum ada penulisan
5. Pengguna konfirmasi → tulis ke connections + combos
```

### Preview (contoh dengan data nyata)

```
Akan diimpor          16 koneksi  ·  10 combo
  ├─  6 endpoint custom   (sumopod, genfity, gnrt, srbyte, …)
  └─ 10 lewat preset      (deepseek, openrouter, xai, commandcode, …)

Dilewati             957
  ├─ 934 gagal / nonaktif di 9router   (920 xiaomi-mimo error 404)
  ├─  18 OAuth — butuh login, tidak portabel  (codex, qoder, grok-cli)
  └─   5 provider tanpa endpoint yang diketahui

Combo                 3 utuh · 7 sebagian · 2 dilewati

☐ Ikutkan juga koneksi yang mati (+934)
```

Opsi "ikutkan yang mati" **default nonaktif**, tapi tersedia bagi pengguna yang
memang ingin memindahkan semuanya.

### Penanganan combo separuh-jalan

7 dari 12 combo mencampur model portabel dan non-portabel. Contoh: combo `hemat`
memuat `sr/glm-5.2` (portabel) dan `qd/kimi-k2.6` (Qoder, OAuth).

**Keputusan: impor anggota yang portabel saja, laporkan sisanya.**

```
hemat  →  4 dari 7 model diimpor
          dilewati: qd/kimi-k2.6, cx/gpt-5.5, cbai/… (provider tidak portabel)
```

Alternatif yang ditolak:
- *Lewati seluruh combo yang tidak utuh* — terlalu galak; combo dengan 4 model
  masih berguna.
- *Impor utuh apa adanya* — menyimpan referensi model yang tidak ada di
  database; pengguna baru sadar saat request-nya gagal.

### Keamanan

- File diproses **di memori**, tidak pernah ditulis ke disk atau ke log.
- API key koneksi **ikut diimpor** (tanpa itu pengguna tidak bisa langsung
  jalan), disimpan lewat jalur yang sama dengan koneksi biasa.
- `apiKeys` milik 9router (kunci akses ke gateway lama) **tidak** diimpor —
  kredensial sistem lain, tidak relevan di Lintasan.
- Preview tidak menampilkan nilai key, hanya nama dan endpoint.

## 6. Cakupan teknis

Fondasinya sudah ada, jadi perubahannya kecil:

| Komponen | Sifat |
|---|---|
| `internal/migrate` (paket baru) | parser + mapper, **fungsi murni** — mudah diuji |
| `provider_presets` | +4 baris data |
| `POST /api/migrate/9router/preview` | klasifikasi, tanpa efek samping |
| `POST /api/migrate/9router/import` | tulis koneksi + combo |
| UI satu halaman | unggah → preview → konfirmasi |

Mapping ke skema `connections` Lintasan:

| 9router | → | Lintasan |
|---|---|---|
| `providerSpecificData.baseUrl` (A) / preset (B) | → | `base_url` |
| `apiKey` | → | `api_key` |
| `name` (+ nama node) | → | `name` |
| `isActive` | → | `is_active` |
| `priority` | → | `priority` |
| `apiType: "chat"` | → | `format: "openai"` |

### Pluggable untuk OmniRouter

`internal/migrate` mendefinisikan satu antarmuka:

```go
type Source interface {
    Name() string
    Detect([]byte) bool          // kenali format dari isi file
    Parse([]byte) (Plan, error)  // → rencana impor yang seragam
}
```

`Plan` bersifat netral terhadap sumber. Menambah OmniRouter nanti = menulis satu
`Source` baru, tanpa menyentuh handler, UI, maupun logika penulisan.

## 7. Di luar cakupan

- Adapter OAuth (Qoder, Codex, Grok, Cline) — keputusan eksplisit Sonickk.
- Impor `customModels` (1265 entri) — biarkan `discover` Lintasan mengisi
  sendiri; hasilnya lebih akurat karena diverifikasi ke endpoint hidup.
- Impor `settings`, `proxyPools`, `pricing` — spesifik 9router.
- OmniRouter — arsitektur disiapkan, mapper menyusul saat contoh file tersedia.

## 8. Rencana pengujian

- Fixture: file export asli (**API key diredaksi**) masuk ke `testdata/`.
  Pelajaran dari M5: fixture buatan tangan hanya menguji pemahaman kita terhadap
  format, bukan formatnya.
- Uji klasifikasi: tiap kelompok (A/B/C) × kesehatan.
- Uji combo separuh-jalan: anggota portabel masuk, sisanya dilaporkan.
- Uji preview bebas efek samping: database tidak berubah setelah preview.
- Uji idempotensi: impor dua kali tidak menghasilkan duplikat.

## 9. Kriteria selesai

- [ ] Preview menampilkan angka yang benar untuk file export nyata
- [ ] Impor menghasilkan koneksi yang **benar-benar bisa dipakai** (bukan sekadar
      baris database — diverifikasi dengan satu request nyata)
- [ ] Koneksi OAuth dilaporkan jelas sebagai tidak portabel, bukan didiamkan
- [ ] Combo separuh-jalan tidak meninggalkan referensi model yang tidak ada
- [ ] File export tidak pernah tersentuh disk

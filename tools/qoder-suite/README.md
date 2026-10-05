# Qoder Unified Suite

Toolkit otomasi **Qoder** end-to-end: pembuatan akun, generate Personal Access Token (PAT),
klaim bonus/kuota, dan injeksi koneksi otomatis ke **9router** (mode API key/PAT).

Dibangun dengan Python + Playwright (stealth Chromium), solver captcha slider Aliyun lokal
(tanpa layanan captcha eksternal), dan temp-mail disposable.

> ⚠️ **Disclaimer**: Gunakan hanya untuk akun milik Anda sendiri dan sesuai Terms of Service
> layanan terkait. Penulis tidak bertanggung jawab atas penyalahgunaan.

---

## Fitur

- **`pipeline`** — All-in-one: buat akun → auto-create PAT → auto-claim bonus → injeksi + test ke 9router.
- **`create`** — Buat akun batch (temp-mail + captcha slider + OTP + PAT), simpan ke `accounts.txt`.
- **`claim`** — Exchange PAT (`pt-*`) → Job Token (`jt-*`) & cek/klaim kuota via COSY engine.
- **`login-9r`** — Login batch akun (OAuth) ke dashboard 9router.
- **`sync`** — Injeksi ulang akun di `accounts.txt` yang belum terdaftar di 9router.
- **`report`** — Ringkasan status akun lokal vs koneksi di 9router.
- **`retest`** — Test ulang koneksi non-active agar kembali `active`.
- **`usage`** — Tampilkan kuota & pemakaian tiap koneksi Qoder.
- **`doctor`** — Preflight check mandiri (temp-mail, 9router, login).
- **`health`** — Health check end-to-end: kirim test chat nyata & verifikasi respons model.
- **`progress`** — Status batch terakhir (state resume).
- **`verify [--renew]`** — Verifikasi semua PAT; renew yang mati via login Qoder.

### Ketahanan batch besar

- **Retry per akun** (`-r N`) — ulangi akun yang gagal (captcha/OTP kadang gagal).
- **Resume** (`--resume`) — lanjutkan batch yang terputus dari titik terakhir (`data/progress.json`).
- **Adaptive backoff** — jeda otomatis membesar setelah beberapa gagal berturut-turut.
- **Isolasi kegagalan** — satu akun gagal tidak menghentikan sisanya.
- **Dedup** — email duplikat tidak ditulis dua kali.
- **Atomic file writes** — aman saat concurrency > 1 (file lock + fsync).
- **Preflight** — cek temp-mail + 9router + login sebelum batch panjang dimulai.
- **Auto-sync** — menutup celah akun yang gagal ter-inject di akhir pipeline.
- **Concurrency** (`-c N`) — worker paralel untuk mempercepat batch.
- **Rotasi proxy** (`-p file`) — hindari rate-limit/blokir IP.
- **JSON logging** (`QODER_JSON_LOG=1`) — log terstruktur untuk monitoring.

---

## Format Akun

```
email:password:pat
```

Parser memakai **bagian terakhir sebagai PAT** dan **middle-join sebagai password**,
sehingga password yang mengandung `:` tetap aman. Format lama `email:password` tetap didukung.

---

## Instalasi

```bash
git clone https://github.com/<user>/qoder-suite.git
cd qoder-suite
chmod +x setup.sh run.sh
./setup.sh          # membuat .venv + install dependencies + playwright chromium
```

### Konfigurasi

Salin template lalu isi sesuai lingkungan Anda:

```bash
cp config.example.toml config.toml
```

```toml
[general]
headless = true
captcha_attempts = 5

[router]
url = "http://localhost:20127/dashboard/providers/qoder"
password = ""

[api]
tempmail_base = "https://your-tempmail.example/api"
```

Nilai juga bisa di-override lewat environment variable:
`NINEROUTER_URL`, `NINEROUTER_PASS`, `NINEROUTER_DB`, `TEMPIK_BASE`, `QODER_HEADLESS`, `CAPTCHA_ATTEMPTS`, `QODER_WEBHOOK_URL`.

### Notifikasi batch (opsional)

Set `QODER_WEBHOOK_URL` (atau `[notify] webhook_url` di config) untuk menerima ringkasan
akhir pipeline — kompatibel dengan Slack/Discord/Telegram gateway atau endpoint HTTP apa pun.

---

## Penggunaan

```bash
# 1 akun end-to-end
./run.sh pipeline -n 1

# 100 akun, maksimal 3 percobaan per akun
./run.sh pipeline -n 100 -r 3

# Top-up sampai TOTAL 100 akun di accounts.txt
./run.sh pipeline -t 100

# 100 akun, 3 worker paralel
./run.sh pipeline -n 100 -c 3

# Pakai rotasi proxy
./run.sh pipeline -n 100 -p proxies.txt

# Buat akun saja
./run.sh create -n 5

# Login akun yang ada ke 9router
./run.sh login-9r -b 2

# Cek / klaim kuota sebuah PAT
./run.sh claim --pat pt-xxxxxxxx

# Sinkron & status
./run.sh sync
./run.sh report
./run.sh retest
./run.sh usage            # kuota per koneksi
./run.sh doctor           # cek kesiapan sistem
./run.sh health           # health check end-to-end (test chat nyata)
./run.sh progress         # status batch terakhir
./run.sh verify --renew   # cek & renew PAT yang invalid

# Lanjutkan batch yang terputus
./run.sh pipeline --resume
```

### Opsi `pipeline`

| Opsi | Keterangan |
|------|------------|
| `-n, --count` | Jumlah akun yang dibuat |
| `-r, --retries` | Maksimum percobaan per akun (default 3) |
| `-t, --target` | Top-up sampai total N akun di `accounts.txt` |
| `-c, --concurrency` | Jumlah worker paralel (default 1) |
| `-p, --proxy-file` | File daftar proxy (satu per baris) |
| `--resume` | Lanjutkan batch yang terputus dari state terakhir |

---

## Struktur

```
qoder-suite/
├── main.py                 # CLI
├── run.sh / setup.sh
├── config.example.toml
├── requirements.txt
└── src/
    ├── config.py           # Loader config
    ├── creator.py          # Orkestrasi signup → OTP → PAT → claim
    ├── tempmail.py         # Klien temp-mail (API tempik)
    ├── captcha.py          # Solver captcha slider Aliyun lokal
    ├── stealth.py          # Anti-deteksi Chromium (Playwright)
    ├── pat.py              # Pembuatan PAT via sesi web
    ├── claim.py            # COSY engine (exchange token + claim kuota)
    ├── login.py            # Injeksi OAuth ke 9router
    ├── proxy.py            # Rotasi proxy
    ├── progress.py         # State resume batch
    ├── health.py           # Health check end-to-end
    ├── renew.py            # Verify & renew PAT
    └── utils.py            # Logging, penyimpanan akun, helper
```

---

## Lisensi

MIT

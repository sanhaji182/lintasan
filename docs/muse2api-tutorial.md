# Tutorial: Menghubungkan muse.ai ke Lintasan via muse2api

Dokumen ini menjelaskan alur lengkap:

```
Browser muse.ai
   │  cookie sesi (hatch_sess, hatch_gw, hatch_vml, hatch_native_auth_device)
   ▼
Panel Muse (Lintasan)  ──admin API──►  muse2api  ──Chromium/CDP──►  muse.ai
   ▲                                      │
   │                                      │ OpenAI-compatible /v1
Client / SDK  ─────────►  Lintasan  ──────┘
```

muse2api adalah sidecar terpisah. Lintasan **tidak** membawa Chromium: Lintasan
hanya mem-proxy admin API muse2api (di balik auth admin Lintasan) dan merutekan
chat/image ke endpoint OpenAI-compatible muse2api.

> Cookie muse.ai bersifat sensitif (akses penuh ke akun). Jangan pernah
> menempelkannya di chat, issue, commit, atau screenshot. Gunakan hanya akun yang
> Anda berwenang menghubungkan.

---

## 1. Prasyarat

- Lintasan sudah berjalan (contoh: `https://lintasan.sans.biz.id`).
- Python 3.10+ dan `python3.x-venv` di host yang sama dengan Lintasan.
- Chromium/Google Chrome terpasang.
- Akses admin ke dashboard Lintasan.
- Akun muse.ai yang bisa login di browser.

Contoh instalasi paket venv di Ubuntu:

```bash
sudo apt-get update
sudo apt-get install -y python3.12-venv
```

---

## 2. Instal muse2api

```bash
git clone --depth 1 https://github.com/www222fff/muse2api.git ~/muse2api
cd ~/muse2api
python3 -m venv .venv
.venv/bin/pip install --upgrade pip
.venv/bin/pip install -e .
```

Verifikasi:

```bash
.venv/bin/python -c 'import muse2api; print(muse2api.__version__)'
google-chrome --version   # atau chromium
```

Catatan: ekstra `[matting]` (BiRefNet, ~930MB) hanya diperlukan untuk
`"background":"transparent"` pada image generation. Tidak wajib.

---

## 3. Konfigurasi muse2api

Buat dua key acak yang berbeda — **API key** (untuk connection Lintasan) dan
**admin key** (untuk panel/proxy Lintasan). Jangan memakai key yang sama dengan
master key Lintasan.

```bash
cd ~/muse2api
umask 077
API_KEY="m2a-$(openssl rand -hex 24)"
ADMIN_KEY="m2a-admin-$(openssl rand -hex 24)"

cat > .env <<EOF
MUSE2API_HOST=127.0.0.1
MUSE2API_PORT=18610
MUSE2API_LOG_LEVEL=INFO
MUSE2API_DRIVER=browser
MUSE2API_SITE_URL=https://muse.ai
MUSE2API_DATA_DIR=/home/ubuntu/muse2api/data
MUSE2API_API_KEY=$API_KEY
MUSE2API_ADMIN_KEY=$ADMIN_KEY
MUSE2API_CHROMIUM_PATH=/usr/bin/google-chrome
MUSE2API_HEADLESS=true
MUSE2API_CDP_PORT=19210
MUSE2API_POOL_STRATEGY=affinity
MUSE2API_ACCOUNT_MAX_CONCURRENCY=1
MUSE2API_KEEPALIVE_ENABLED=true
MUSE2API_PUBLIC_BASE=https://lintasan.sans.biz.id
EOF

chmod 600 .env
mkdir -p data
```

Simpan `API_KEY` untuk langkah connection. `ADMIN_KEY` akan dipakai Lintasan
secara server-side dan tidak perlu dimasukkan ke dashboard.

Data muse2api (termasuk cookie dan profil Chrome) berada di `data/`; direktori
ini harus hanya bisa dibaca user service (`700`).

---

## 4. Systemd service muse2api

Buat `/etc/systemd/system/muse2api.service`:

```ini
[Unit]
Description=muse2api — muse.ai OpenAI-compatible sidecar
Documentation=https://github.com/www222fff/muse2api
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
User=ubuntu
Group=ubuntu
WorkingDirectory=/home/ubuntu/muse2api
Environment=HOME=/home/ubuntu
Environment=PYTHONUNBUFFERED=1
EnvironmentFile=/home/ubuntu/muse2api/.env
ExecStart=/home/ubuntu/muse2api/.venv/bin/muse2api
Restart=always
RestartSec=5
TimeoutStopSec=20
StandardOutput=journal
StandardError=journal
SyslogIdentifier=muse2api
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full
ReadWritePaths=/home/ubuntu/muse2api/data
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

Aktifkan:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now muse2api.service
systemctl status muse2api --no-pager
```

Verifikasi:

```bash
curl -s http://127.0.0.1:18610/healthz
curl -s http://127.0.0.1:18610/readyz
journalctl -u muse2api -n 30 --no-pager
```

`/readyz` akan `ready:false` selama pool masih 0 akun; itu normal sebelum
cookie diimpor. Driver browser harus `ok:true`.

---

## 5. Konfigurasi Lintasan

Lintasan membaca URL dan admin key muse2api dari environment. Secret disimpan
di file root-only:

```bash
sudo mkdir -p /etc/lintasan
sudo tee /etc/lintasan/muse2api.env >/dev/null <<'EOF'
LINTASAN_MUSE2API_URL=http://127.0.0.1:18610
LINTASAN_MUSE2API_ADMIN_KEY=<ADMIN_KEY_DARI_LANGKAH_3>
EOF
sudo chmod 600 /etc/lintasan/muse2api.env
sudo chown root:root /etc/lintasan/muse2api.env
```

Daftarkan lewat drop-in systemd:

```bash
sudo tee /etc/systemd/system/lintasan.service.d/muse2api.conf >/dev/null <<'EOF'
[Service]
EnvironmentFile=/etc/lintasan/muse2api.env
EOF
sudo systemctl daemon-reload
sudo systemctl restart lintasan
```

Fallback yang didukung: jika `LINTASAN_MUSE2API_*` kosong, Lintasan membaca
`MUSE2API_ADMIN_KEY` / `MUSE2API_URL` dari environment.

Remote browser login memakai port CDP Chromium sidecar. Default-nya
`http://127.0.0.1:19210` (sama dengan `MUSE2API_CDP_PORT`). Jika berbeda, set:

```bash
LINTASAN_MUSE2API_CDP_URL=http://127.0.0.1:19210
```

Endpoint CDP hanya boleh loopback; Lintasan menolak URL non-loopback.

Verifikasi di dashboard:

- **Settings → Account Connection** menampilkan pintasan Muse Accounts,
  Connections, dan OAuth Providers.
- **Muse Accounts** menampilkan `muse2api v… · driver browser`.
- Jika belum dikonfigurasi, panel menampilkan instruksi env, bukan error.

---

## 6. Ambil cookie muse.ai

1. Login ke `https://muse.ai` di browser Anda.
2. Buka DevTools → **Application/Storage → Cookies → https://muse.ai**.
3. Salin nilai cookie berikut:
   - `hatch_sess`
   - `hatch_gw`
   - `hatch_vml`
   - `hatch_native_auth_device`

Alternatif: tab **Network** → pilih request ke muse.ai → salin seluruh header
`Cookie:` (pastikan keempat nama di atas ada), atau export JSON dengan ekstensi
EditThisCookie.

Jangan menutup sesi muse.ai sebelum cookie terbukti berfungsi.

---

## 7. Import cookie di panel Muse

Buka **Settings → Account Connection → Muse Accounts** atau langsung
`/dashboard/muse`.

1. Isi **Label** (mis. `muse-main`).
2. Paste cookie ke field **Cookies** — panel menerima:
   - raw header: `hatch_sess=...; hatch_gw=...; hatch_vml=...; hatch_native_auth_device=...`
   - JSON object: `{"hatch_sess":"...","hatch_gw":"...",...}`
   - JSON list hasil export (EditThisCookie)
3. Klik **Import account**.
4. Jika muncul `missing cookies`, tambahkan cookie yang disebut.
5. Klik **Renew session** untuk memvalidasi dan merotasi sesi.
6. Target: status akun **Active**, counter **Available** minimal 1.

Di balik layar, panel memanggil `POST /api/muse/accounts` di Lintasan, yang
meneruskan ke muse2api `POST /admin/accounts` dengan admin key server-side.
Cookie tidak disimpan oleh Lintasan; muse2api menyimpannya di `data/accounts.json`
dan profil Chrome.

### Login otomatis lewat remote browser

Cara paling sedikit langkah: login langsung dari panel.

1. Di **Muse Accounts**, klik **Open remote login**.
2. Browser sementara (Chromium headless, context incognito terisolasi) terbuka di modal.
3. Login ke muse.ai di dalam modal: email/telepon, lalu kode OTP. Klik area browser
   dan ketik seperti biasa; klik tombol di halaman lewat mouse.
4. Saat cookie `hatch_*` terdeteksi, Lintasan otomatis membuat akun di
   muse2api, menutup context, dan memuat daftar akun.
5. Jika muse.ai hanya memberi tiga cookie (`hatch_vml` belum ada), panel
   menampilkan "Login detected — completing muse.ai session…" lalu memanggil
   `renew` di muse2api agar `hatch_vml` terbit dan akun langsung aktif. Tidak
   perlu menunggu atau mengulang login.
6. Klik **Renew session** jika ingin memvalidasi ulang sesi.

Catatan:

- Password/OTP diketik langsung ke halaman muse.ai di browser server; Lintasan
  tidak menerima atau menyimpannya.
- Context bersifat sementara dan dibuang setelah import; cookie tidak masuk ke
  profil Chrome default muse2api.
- Sesi login dibatasi 15 menit dan satu sesi aktif per admin.
- Modal menampilkan frame screenshot (~2.5 fps), bukan video; input mouse dan
  keyboard diteruskan ke halaman.
- Jika panel melaporkan CDP tidak terjangkau, pastikan `muse2api` (dan Chromium-nya)
  berjalan serta `LINTASAN_MUSE2API_CDP_URL` benar.

### Import otomatis dengan ekstensi Chrome

Jika tidak ingin menyalin cookie manual:

1. Di panel Muse, klik **Generate import token**. Token berlaku ±10 menit,
   sekali pakai, dan **hanya** bisa dipakai untuk import cookie.
2. Muat ekstensi dari folder [`tools/muse-cookie-extension`](../tools/muse-cookie-extension):
   buka `chrome://extensions` → aktifkan **Developer mode** → **Load unpacked**
   → pilih folder tersebut.
3. Login ke muse.ai di browser yang sama.
4. Buka ekstensi, isi URL Lintasan, paste token, dan klik **Capture & import**.
   Browser mungkin meminta izin akses ke origin Lintasan.
5. Kembali ke panel Muse dan klik **Renew session**.

Ekstensi hanya membaca empat cookie `hatch_*`, mengirimnya langsung ke Lintasan
dengan token, dan tidak pernah melihat admin key muse2api. Token tidak disimpan
di extension; jika import gagal, generate token baru.

---

## 8. Buat connection di Lintasan

1. Buka **Connections → Add → Provider preset → Muse (muse2api)**.
2. Base URL: `http://127.0.0.1:18610/v1` (sudah terisi preset).
3. API key: `MUSE2API_API_KEY` dari `~/muse2api/.env` — **bukan** admin key:
   ```bash
   grep '^MUSE2API_API_KEY=' /home/ubuntu/muse2api/.env
   ```
4. Klik **Test Connection**, lalu **Save Connection**.
5. Klik **Sync Models** agar `muse-chat`, `muse-image`, `muse-video`, dan alias
   (`gpt-4o`, `dall-e-3`, dll.) terdaftar.

Jika routing menemukan beberapa connection yang mendukung model yang sama,
gunakan combo/alias atau nonaktifkan connection lain untuk pengujian.

---

## 9. Uji alur end-to-end

Chat melalui Lintasan (gunakan master key Lintasan, bukan key muse2api):

```bash
curl -s -X POST https://lintasan.sans.biz.id/v1/chat/completions \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "muse-chat",
    "messages": [{"role": "user", "content": "Balas dengan satu kalimat tentang muse.ai"}],
    "stream": false
  }'
```

Streaming:

```bash
curl -N -X POST https://lintasan.sans.biz.id/v1/chat/completions \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"muse-chat","messages":[{"role":"user","content":"Halo"}],"stream":true}'
```

Image:

```bash
curl -s -X POST https://lintasan.sans.biz.id/v1/images/generations \
  -H "Authorization: Bearer $LINTASAN_MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"muse-image","prompt":"A red fox in autumn","size":"16:9"}'
```

Catatan image: set `MUSE2API_PUBLIC_BASE` ke origin publik Lintasan
(`https://lintasan.sans.biz.id`). Lintasan mem-proxy `GET /v1/media/{name}` ke
sidecar, jadi URL media dapat diakses klien eksternal tanpa auth (nama file
acak). Tanpa public base, URL akan menunjuk `127.0.0.1:18610`. Chat tidak
terpengaruh.

Keepalive: `MUSE2API_KEEPALIVE_ENABLED=true` direkomendasikan. Renew manual
(panel **Renew** atau `POST /admin/accounts/{id}/renew`) terbukti memperpanjang
`hatch_vml` sekitar 48 jam; loop keepalive default berjalan setiap 6 jam.

Video: muse2api punya `/v1/videos`, tetapi Lintasan belum punya route padanan,
jadi video belum bisa dipakai lewat Lintasan.

Verifikasi pendukung:

```bash
# Pool & driver
ADMIN=$(grep '^MUSE2API_ADMIN_KEY=' ~/muse2api/.env | cut -d= -f2-)
curl -s -H "Authorization: Bearer $ADMIN" http://127.0.0.1:18610/admin/status

# Log saat request
journalctl -u muse2api -f

# Status Lintasan
journalctl -u lintasan -n 50 --no-pager
```

---

## 10. Troubleshooting

| Gejala | Penyebab / solusi |
|---|---|
| Panel: “Connect the muse2api sidecar” | Env `LINTASAN_MUSE2API_ADMIN_KEY` belum ada / Lintasan belum restart. Periksa `/etc/lintasan/muse2api.env` dan drop-in, lalu restart. |
| Panel: sidecar unreachable | `systemctl status muse2api`; pastikan listen `127.0.0.1:18610`; cek `journalctl -u muse2api`. |
| Panel: “muse2api rejected the admin key” | `LINTASAN_MUSE2API_ADMIN_KEY` tidak sama dengan `MUSE2API_ADMIN_KEY` di `~/muse2api/.env`. Perbaiki lalu restart Lintasan. |
| `missing cookies` saat import | Cookie wajib belum lengkap: `hatch_sess`, `hatch_gw`, `hatch_vml`, `hatch_native_auth_device`. |
| Akun **Invalid session** / HTTP 403 | Sesi muse.ai kedaluwarsa atau dicabut. Login ulang dan export cookie baru. |
| Akun Active tapi chat gagal | Lihat `journalctl -u muse2api -f`; bisa quota/cooldown, tab macet, atau session perlu **Renew**. |
| Test Connection 401 | API key yang dimasukkan adalah admin key atau key Lintasan. Gunakan `MUSE2API_API_KEY`. |
| `/readyz` false | Pool belum punya akun available; import cookie terlebih dahulu. |
| Image URL `127.0.0.1` | Set `MUSE2API_PUBLIC_BASE=https://lintasan.sans.biz.id` lalu restart sidecar. Lintasan sudah mem-proxy `/v1/media/`. |
| Chrome kehabisan memori | `systemctl status muse2api`; kurangi `MUSE2API_ACCOUNT_MAX_CONCURRENCY`, tambah RAM/swap, atau restart service. |

---

## 11. Keamanan dan perawatan

- **Admin key muse2api** hanya ada di `~/muse2api/.env` dan
  `/etc/lintasan/muse2api.env` (root, `600`). Jangan membukanya ke dashboard
  publik atau mengirimkannya ke klien.
- **Cookie muse.ai** = sesi akun penuh. Simpan hanya di muse2api; jangan commit
  ke git, jangan tempel di chat.
- Gunakan akun yang Anda berwenang menghubungkan; pahami ToS muse.ai.
- Rotasi key: ubah `MUSE2API_API_KEY`/`MUSE2API_ADMIN_KEY` di `.env`, salin admin
  key baru ke `/etc/lintasan/muse2api.env`, lalu restart **kedua** service. Perbarui
  juga API key pada connection Lintasan jika API key berubah.
- Cookie berkala perlu diperbarui. Panel menyediakan **Renew session**; opsi
  `MUSE2API_KEEPALIVE_ENABLED=true` dapat mengaktifkan renew otomatis.
- `data/` muse2api berisi cookie dan profil Chrome; direktori harus `700`.
- Setelah upgrade muse2api, jalankan ulang `pip install -e .` dan restart service.

---

## 12. Referensi cepat

```bash
# Service
sudo systemctl enable --now muse2api
sudo systemctl restart muse2api
journalctl -u muse2api -f

# Health
curl -s http://127.0.0.1:18610/healthz
curl -s http://127.0.0.1:18610/readyz

# Key locations
grep '^MUSE2API_API_KEY='   ~/muse2api/.env   # connection Lintasan
grep '^MUSE2API_ADMIN_KEY=' ~/muse2api/.env   # dipakai proxy panel

# Lintasan
sudo systemctl restart lintasan
curl -s https://lintasan.sans.biz.id/health
```

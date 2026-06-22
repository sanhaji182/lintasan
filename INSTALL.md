# 🇮🇩 Lintasan — Instalasi

> Panduan lengkap untuk menginstal Lintasan Go di berbagai platform.

> ⚡ **Single binary ~24MB** — dashboard sudah termasuk. Tidak perlu Node.js, Redis, atau database terpisah.

---

## 🇬🇧 Lintasan — Installation

> Complete installation guide for Lintasan Go on any platform.

---

## 📋 Prasyarat / Prerequisites

| Metode / Method | Minimal | Catatan |
|----------------|---------|---------|
| **Binary (pre-built)** | Linux x86_64 | Zero dependencies |
| **Build dari source** | Go 1.22+, Node.js 20+ | `make build` |
| **Docker** | Docker + Docker Compose | Single container |

---

## 🚀 Opsi 1: Pre-built Binary (Recommended)

Satu perintah, langsung jalan. Dashboard sudah embedded di dalam binary.

```bash
# Download latest release
curl -L -o lintasan https://github.com/sanhaji182/lintasan/releases/latest/download/lintasan-linux-amd64

# Beri izin eksekusi
chmod +x lintasan

# Start server
./lintasan start
```

**Dashboard** → `http://localhost:20180/dashboard`
**API** → `http://localhost:20180/v1/chat/completions`

Pada first run, Lintasan akan:
1. Membuat database SQLite otomatis di `./data/lintasan.db`
2. Membuat akun admin default dengan one-time password (ditampilkan di console)
3. Menunggu Anda login dan mengganti password

> 💡 Untuk production, set `LINTASAN_MASTER_KEY` sebelum start:
> ```bash
> export LINTASAN_MASTER_KEY=$(openssl rand -hex 32)
> ./lintasan start
> ```

---

## 🔧 Opsi 2: Build dari Source

Butuh **Go 1.22+** dan **Node.js 20+** (untuk build dashboard SvelteKit).

```bash
# Clone repo
git clone https://github.com/sanhaji182/lintasan.git
cd lintasan

# Build frontend → embed → single binary
make build

# Start
./lintasan start
```

**Tanpa Node.js?** Jika hanya butuh API proxy (tanpa dashboard UI):

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o lintasan ./cmd/lintasan
./lintasan start
```

> Hasilnya server **API-only** di `:20180` — cocok untuk headless/server deployment.

### Build cross-platform

```bash
# Build untuk Linux x86_64
make release

# Atau manual untuk target lain
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o lintasan-linux-amd64 ./cmd/lintasan
```

> CGO cross-compile butuh toolchain spesifik platform. Untuk target non-Linux, lihat [go-sqlite3 cross compile guide](https://github.com/mattn/go-sqlite3#cross-compile).

---

## 🐳 Opsi 3: Docker (Single Container)

Container tunggal dengan health check, volume persistensi, dan restart policy.

```bash
git clone https://github.com/sanhaji182/lintasan.git
cd lintasan

# Generate master key dan start
LINTASAN_MASTER_KEY=$(openssl rand -hex 32) docker compose up --build

# UI + API → http://localhost:20180
```

### docker-compose.yml

```yaml
services:
  lintasan:
    build:
      context: .
      dockerfile: Dockerfile
    image: lintasan-go:latest
    container_name: lintasan
    ports:
      - "20180:20180"
    environment:
      PORT: "20180"
      LINTASAN_DATA_DIR: /app/data
      LINTASAN_MASTER_KEY: "${LINTASAN_MASTER_KEY}"
    volumes:
      - lintasan-data:/app/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:20180/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

volumes:
  lintasan-data:
```

### Environment variables untuk Docker

| Variable | Contoh | Keterangan |
|----------|--------|------------|
| `LINTASAN_MASTER_KEY` | `openssl rand -hex 32` | **WAJIB** untuk production |
| `PORT` | `20180` | Port container |

---

## 📋 CLI Commands

```bash
# Start server (UI + API)
./lintasan start

# Start dengan port kustom
PORT=8080 ./lintasan start

# Inisialisasi database (jika perlu reset)
./lintasan setup

# MITM HTTPS bridge (opsional — untuk LocalAI/LM Studio)
./lintasan mitm start

# Cek versi
./lintasan version

# Semua perintah
./lintasan help
```

---

## 🔄 Update / Upgrade

### Binary pre-built

```bash
# Download ulang binary terbaru
./lintasan version                          # cek versi lama
curl -L -o lintasan-new https://github.com/sanhaji182/lintasan/releases/latest/download/lintasan-linux-amd64
chmod +x lintasan-new

# Ganti binary saat service berjalan
sudo systemctl stop lintasan
mv lintasan-new lintasan
sudo systemctl start lintasan

# Verifikasi
./lintasan version
curl -s localhost:20180/health
```

### Build dari source

```bash
git pull origin main
make build
sudo systemctl stop lintasan
cp lintasan /opt/lintasan/lintasan
sudo systemctl start lintasan
```

---

## 🏗 Production Deployment

### Systemd service

```ini
[Unit]
Description=Lintasan LLM Proxy Gateway
Documentation=https://github.com/sanhaji182/lintasan
After=network.target

[Service]
Type=simple
User=lintasan
WorkingDirectory=/opt/lintasan
ExecStart=/opt/lintasan/lintasan start
Restart=on-failure
RestartSec=5
Environment=LINTASAN_MASTER_KEY=<your-master-key>

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now lintasan
sudo systemctl status lintasan
```

### Nginx reverse proxy

```nginx
server {
    listen 443 ssl;
    server_name lintasan.example.com;

    ssl_certificate     /etc/letsencrypt/live/lintasan.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/lintasan.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:20180;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
        proxy_buffering off;  # penting untuk SSE streaming
    }
}
```

---

## 🐛 Troubleshooting

| Masalah | Solusi |
|---------|--------|
| **"CGO_ENABLED required"** | go-sqlite3 butuh CGO. Set `CGO_ENABLED=1` |
| **Port 20180 sudah dipakai** | Ganti port: `PORT=8080 ./lintasan start` |
| **"text file busy"** | Binary sedang running. Stop service dulu |
| **Dashboard blank** | Pastikan binary hasil `make build` (bukan `go build` saja) |
| **Database error** | Hapus `data/lintasan.db` lalu restart (data akan hilang) |

---

## 📦 File Size Reference

| Komponen | Ukuran |
|----------|--------|
| Binary (single, embedded) | ~24MB |
| SQLite database (kosong) | ~80KB |
| RAM (idle) | ~14MB |
| RAM (10 concurrent req) | ~28MB |

---

> **🇮🇩** Ada masalah instalasi? Buka [issue](https://github.com/sanhaji182/lintasan/issues).
>
> **🇬🇧** Having installation issues? Open an [issue](https://github.com/sanhaji182/lintasan/issues).

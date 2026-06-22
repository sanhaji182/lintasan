# 🇮🇩 Lintasan — Quickstart

> **Dari nol ke request LLM pertama dalam <5 menit.**

Panduan ini mengasumsikan Anda menggunakan Linux x86_64. Butuh platform lain? Lihat [INSTALL.md](INSTALL.md).

---

## 🇬🇧 Lintasan — Quickstart

> **From zero to your first LLM request in under 5 minutes.**

---

## ⏱ 5-Minute Setup

### Menit 1: Download & Start

```bash
# Download binary (Linux x86_64)
curl -L -o lintasan https://github.com/sanhaji182/lintasan/releases/latest/download/lintasan-linux-amd64

# Beri izin eksekusi
chmod +x lintasan

# Start server
./lintasan start
```

Output akan terlihat seperti:

```
✓ Lintasan Go v0.24.2
✓ SQLite database initialized at ./data/lintasan.db
✓ Admin account created — one-time password: aB3x...K9m2
  → Login at http://localhost:20180/login
✓ Server listening on :20180
```

---

### Menit 2: Login & Ambil One-Time Password

Buka **http://localhost:20180** di browser.

1. Anda akan diarahkan ke halaman **Login**
2. Cari satu **one-time password** di console output server (lihat langkah 1)
3. Username default: `admin`
4. Masukkan OTP sebagai password
5. Sistem akan meminta Anda mengganti password

> 💡 **Kehilangan OTP?** Restart server — OTP baru akan digenerate.

---

### Menit 3: Set Master API Key

Setelah login, Anda akan melihat halaman **Settings** dengan prompt untuk mengatur master key.

**Via dashboard:**
- Buka `/dashboard/settings`
- Generate key: klik tombol **Generate** (atau buat manual)
- Copy dan simpan key ini — ini adalah Bearer token untuk API calls

**Via CLI (alternative):**
```bash
export LINTASAN_MASTER_KEY=$(openssl rand -hex 32)
./lintasan start
```

---

### Menit 4: Tambahkan Provider Connection

1. Buka **Accounts** → **+ Add Connection**
2. Pilih preset provider atau isi manual:
   - **Name**: `OpenAI Test`
   - **Base URL**: `https://api.openai.com/v1`
   - **API Key**: `sk-...` (API key Anda)
   - **Format**: `openai`
3. Klik **Test** — pastikan koneksi berhasil
4. Klik **Save**

> 🆓 Punya akun Groq? Coba preset **Groq** — gratis dan cepat.
>
> 🇮🇩 Punya akun **Sumopod**? Pilih dari **Providers** → **Sumopod**.

---

### Menit 5: Kirim Request LLM Pertama

**Via curl:**
```bash
curl http://localhost:20180/v1/chat/completions \
  -H "Authorization: Bearer <master-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello! Who are you?"}],
    "stream": true
  }'
```

**Via Playground (dashboard):**
- Buka **Playground** (`/dashboard/playground`)
- Pilih model dari dropdown
- Ketik pesan
- Klik **Send**

**Via Python:**
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:20180/v1",
    api_key="<master-key>"
)

response = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)
```

**Via langchain:**
```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    base_url="http://localhost:20180/v1",
    api_key="<master-key>",
    model="gpt-4o-mini"
)
print(llm.invoke("Hello!"))
```

---

## ✅ Selesai! Apa Selanjutnya?

| Ingin... | Baca |
|----------|------|
| 📖 Dokumentasi lengkap | [README.md](README.md) |
| 📦 Instalasi production | [INSTALL.md](INSTALL.md) |
| ⚙ Semua opsi konfigurasi | [CONFIGURATION.md](CONFIGURATION.md) |
| 📡 API Reference | [docs/api-reference.md](docs/api-reference.md) |
| 🔌 Tambah provider | Dashboard → **Providers** |
| 🔀 Set routing rules | Dashboard → **Routing** |
| 📊 Cek statistics | Dashboard → **Overview** |

---

## 🏃‍♂️ Commands Cepat

```bash
# Start
./lintasan start

# Test health
curl http://localhost:20180/health

# List models
curl http://localhost:20180/v1/models -H "Authorization: Bearer <key>"

# Embeddings
curl http://localhost:20180/v1/embeddings \
  -H "Authorization: Bearer <key>" \
  -H "Content-Type: application/json" \
  -d '{"model": "text-embedding-3-small", "input": "Hello world"}'

# Cek versi
./lintasan version
```

---

## 🔍 Troubleshooting Cepat

| Problem | Solution |
|---------|----------|
| `curl: command not found` | `sudo apt install curl` |
| `Permission denied` | `chmod +x lintasan` |
| `Bind :20180: address already` | `PORT=8080 ./lintasan start` |
| `401 Unauthorized` | Cek `Authorization: Bearer <master-key>` |
| `404 model not found` | Belum add connection dengan model itu |
| `502 All providers failed` | Cek koneksi provider di Accounts page |

---

> **🇮🇩** Butuh bantuan? [Buka issue](https://github.com/sanhaji182/lintasan/issues)
>
> **🇬🇧** Need help? [Open an issue](https://github.com/sanhaji182/lintasan/issues)

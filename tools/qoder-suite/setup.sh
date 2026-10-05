#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"

if [ ! -d ".venv" ]; then
    echo "[*] Membuat virtual environment .venv..."
    python3 -m venv .venv
fi

echo "[*] Install dependency..."
.venv/bin/pip install --upgrade pip
.venv/bin/pip install -r requirements.txt
.venv/bin/python -m playwright install chromium

[ -f config.toml ] || cp config.example.toml config.toml
[ -f input_login.txt ] || echo "# Format: email:password:pat atau email:password" > input_login.txt

echo "[+] Setup selesai. Jalankan: ./run.sh"

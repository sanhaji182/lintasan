#!/usr/bin/env bash
cd "$(dirname "$0")"

if [ ! -d ".venv" ]; then
    echo "[!] .venv belum ada, menjalankan setup.sh..."
    ./setup.sh
fi

.venv/bin/python main.py "$@"

"""
Qoder Suite - Progress State (Resume dari crash)

Menyimpan state batch ke data/progress.json agar pipeline bisa dilanjutkan
dari titik terakhir jika proses mati (VPS restart, OOM, Ctrl+C, dll).

State berisi:
- run_id     : id unik run
- target     : jumlah akun yang diinginkan
- done       : jumlah akun sukses
- failed     : jumlah akun gagal (setelah semua retry)
- started_at : waktu mulai
- updated_at : waktu update terakhir
"""

import json
import os
import time
import uuid
from pathlib import Path
from typing import Dict, Any, Optional

from .config import DATA_DIR
from .utils import write_log

PROGRESS_FILE = DATA_DIR / "progress.json"


def new_state(target: int, extra: Dict[str, Any] = None) -> Dict[str, Any]:
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    st = {
        "run_id": uuid.uuid4().hex[:12],
        "target": target,
        "done": 0,
        "failed": 0,
        "started_at": now,
        "updated_at": now,
        "status": "running",
    }
    if extra:
        st.update(extra)
    return st


def save_state(state: Dict[str, Any], path: Path = None) -> bool:
    path = path or PROGRESS_FILE
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        state["updated_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        tmp = path.with_suffix(".tmp")
        with open(tmp, "w", encoding="utf-8") as f:
            json.dump(state, f, ensure_ascii=False, indent=2)
        os.replace(tmp, path)  # atomic
        return True
    except Exception as e:
        write_log(f"progress save failed: {e}", "WARNING")
        return False


def load_state(path: Path = None) -> Optional[Dict[str, Any]]:
    path = path or PROGRESS_FILE
    if not path.exists():
        return None
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception as e:
        write_log(f"progress load failed: {e}", "WARNING")
        return None


def clear_state(path: Path = None) -> bool:
    path = path or PROGRESS_FILE
    try:
        if path.exists():
            path.unlink()
        return True
    except Exception:
        return False


def mark_done(state: Dict[str, Any], success: bool = True, path: Path = None):
    if success:
        state["done"] = state.get("done", 0) + 1
    else:
        state["failed"] = state.get("failed", 0) + 1
    save_state(state, path)


def mark_finished(state: Dict[str, Any], path: Path = None):
    state["status"] = "finished"
    save_state(state, path)

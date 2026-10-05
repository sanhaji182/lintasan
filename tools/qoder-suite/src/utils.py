import json
import os
import string
import random
import uuid
import hashlib
from pathlib import Path
from datetime import datetime, timezone
from typing import Dict, Any, Optional
from .config import LOG_FILE

# Lock per-file (thread/async-safe via O_EXCL) untuk mencegah race saat concurrency > 1.
try:
    import fcntl  # POSIX
    _HAS_FCNTL = True
except ImportError:  # pragma: no cover (Windows)
    _HAS_FCNTL = False

def setup_logging():
    LOG_FILE.parent.mkdir(parents=True, exist_ok=True)
    if not LOG_FILE.exists():
        LOG_FILE.write_text(f"# Log started at {datetime.now(timezone.utc).isoformat()}\n", encoding="utf-8")

def write_log(message: str, level: str = "INFO"):
    setup_logging()
    timestamp = datetime.now(timezone.utc).isoformat()
    try:
        with open(LOG_FILE, "a", encoding="utf-8") as f:
            if _HAS_FCNTL:
                fcntl.flock(f.fileno(), fcntl.LOCK_EX)
            f.write(f"[{timestamp}] [{level}] {message}\n")
            if _HAS_FCNTL:
                fcntl.flock(f.fileno(), fcntl.LOCK_UN)
    except Exception:
        pass
    # Structured JSON log (opsional, untuk parsing/monitoring)
    _write_json_log(message, level, timestamp)

_JSON_LOG_ENABLED = None

def _json_log_enabled() -> bool:
    global _JSON_LOG_ENABLED
    if _JSON_LOG_ENABLED is None:
        try:
            from .config import JSON_LOG
            _JSON_LOG_ENABLED = bool(JSON_LOG)
        except Exception:
            _JSON_LOG_ENABLED = False
    return _JSON_LOG_ENABLED

def _write_json_log(message: str, level: str, timestamp: str):
    """Tulis satu baris JSON terstruktur ke data/app.jsonl (jika diaktifkan)."""
    if not _json_log_enabled():
        return
    try:
        import json as _json
        rec = {"ts": timestamp, "level": level, "msg": message}
        path = LOG_FILE.with_name("app.jsonl")
        path.parent.mkdir(parents=True, exist_ok=True)
        with open(path, "a", encoding="utf-8") as f:
            if _HAS_FCNTL:
                fcntl.flock(f.fileno(), fcntl.LOCK_EX)
            f.write(_json.dumps(rec, ensure_ascii=False) + "\n")
            if _HAS_FCNTL:
                fcntl.flock(f.fileno(), fcntl.LOCK_UN)
    except Exception:
        pass

def log_event(event: str, **fields):
    """Catat event terstruktur (level=EVENT) dengan field tambahan.

    Contoh: log_event("account_created", email="a@b.com", pat=True)
    """
    payload = {"event": event}
    payload.update(fields)
    import json as _json
    write_log(_json.dumps(payload, ensure_ascii=False), "EVENT")

def save_jsonl(filepath: Path, data: Dict[str, Any]):
    filepath.parent.mkdir(parents=True, exist_ok=True)
    with open(filepath, "a", encoding="utf-8") as f:
        if _HAS_FCNTL:
            fcntl.flock(f.fileno(), fcntl.LOCK_EX)
        f.write(json.dumps(data, ensure_ascii=False) + "\n")
        if _HAS_FCNTL:
            fcntl.flock(f.fileno(), fcntl.LOCK_UN)

def append_account_txt(filepath: Path, email: str, password: str, pat: str):
    """Save in format email:password:pat (dedup by email, atomic under concurrency)."""
    filepath.parent.mkdir(parents=True, exist_ok=True)
    # Buka dengan mode append dan kunci file agar cek-dedup + tulis menjadi atomik.
    with open(filepath, "a+", encoding="utf-8") as f:
        if _HAS_FCNTL:
            fcntl.flock(f.fileno(), fcntl.LOCK_EX)
        try:
            f.seek(0)
            existing_emails = set()
            for line in f.read().splitlines():
                line = line.strip()
                if line and not line.startswith("#"):
                    existing_emails.add(line.split(":", 1)[0].strip())
            if email in existing_emails:
                write_log(f"Skip duplicate account: {email}", "WARNING")
                return False
            f.seek(0, os.SEEK_END)
            f.write(f"{email}:{password}:{pat}\n")
            f.flush()
            os.fsync(f.fileno())
            return True
        finally:
            if _HAS_FCNTL:
                fcntl.flock(f.fileno(), fcntl.LOCK_UN)

def remove_account_txt(filepath: Path, raw_line: str):
    if not filepath.exists():
        return
    with open(filepath, "r+", encoding="utf-8") as f:
        if _HAS_FCNTL:
            fcntl.flock(f.fileno(), fcntl.LOCK_EX)
        lines = f.read().splitlines(keepends=True)
        new_lines = [l for l in lines if l.strip() != raw_line.strip()]
        f.seek(0)
        f.truncate()
        f.write("".join(new_lines))
        f.flush()
        if _HAS_FCNTL:
            fcntl.flock(f.fileno(), fcntl.LOCK_UN)

def generate_password(length: int = 14) -> str:
    chars = string.ascii_letters + string.digits + "!@#$%^&*"
    return "".join(random.choices(chars, k=length))

def generate_machine_id() -> str:
    return hashlib.md5(str(random.randint(1000000, 9999999)).encode()).hexdigest()[:32]

def send_notification(message: str) -> bool:
    """Kirim notifikasi ke WEBHOOK_URL (jika di-set). Return True jika terkirim."""
    try:
        from .config import WEBHOOK_URL
    except Exception:
        return False
    if not WEBHOOK_URL:
        return False
    try:
        import urllib.request
        payload = json.dumps({"text": message, "content": message}).encode("utf-8")
        req = urllib.request.Request(WEBHOOK_URL, data=payload,
                                     headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            ok = 200 <= resp.status < 300
        write_log(f"Notification sent: {ok}", "INFO")
        return ok
    except Exception as e:
        write_log(f"Notification failed: {e}", "WARNING")
        return False

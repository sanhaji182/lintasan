"""
Qoder Suite - Health Check (End-to-End)

Membuktikan pipeline benar-benar menghasilkan koneksi yang BERGUNA, bukan sekadar
"active" di dashboard. Kuota 0 pada akun free tier adalah NORMAL (akses via queue),
jadi indikator satu-satunya yang valid adalah: apakah model benar-benar membalas.

Cek:
1. 9router hidup (/api/health)
2. API key tersedia (dari DB atau env)
3. Model qd/qfmodel terekspos (/v1/models)
4. Test chat nyata ke model -> verifikasi respons
"""

import json
import sqlite3
import urllib.request
import urllib.error
import time
from typing import Dict, Any, Optional

from .config import NINE_ROUTER_URL, TEMPIK_BASE
from .utils import write_log


def _base_router(url: str) -> str:
    """Normalisasi URL dashboard -> base origin."""
    if "://" not in url:
        url = "http://" + url
    # buang path
    from urllib.parse import urlparse
    u = urlparse(url)
    return f"{u.scheme}://{u.netloc}"


def _get_api_key(db_path: str = None, model: str = "qd/qfmodel") -> Optional[str]:
    """Ambil API key 9router dari DB (prioritas yang allowedModels cocok)."""
    import os
    db_path = db_path or os.getenv("NINEROUTER_DB", os.path.expanduser("~/.9router/db/data.sqlite"))
    try:
        conn = sqlite3.connect(db_path)
        rows = conn.execute(
            "SELECT key, allowedModels, isActive FROM apiKeys WHERE isActive=1").fetchall()
        conn.close()
    except Exception as e:
        write_log(f"health: gagal baca API key: {e}", "WARNING")
        return None

    # Prioritas: key yang secara eksplisit mengizinkan model ini
    for key, allowed, _ in rows:
        if allowed and model in str(allowed):
            return key
    # Fallback: key dengan akses '*'
    for key, allowed, _ in rows:
        if allowed and "*" in str(allowed):
            return key
    # Fallback terakhir: key pertama
    return rows[0][0] if rows else None


def check_router(base_router: str) -> Dict[str, Any]:
    try:
        req = urllib.request.Request(f"{base_router}/api/health",
                                     headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=10) as r:
            data = json.loads(r.read().decode())
        return {"ok": bool(data.get("ok")), "raw": data}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def check_model(base_router: str, model: str = "qd/qfmodel", api_key: str = None) -> Dict[str, Any]:
    try:
        headers = {"User-Agent": "Mozilla/5.0"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        req = urllib.request.Request(f"{base_router}/v1/models", headers=headers)
        with urllib.request.urlopen(req, timeout=15) as r:
            data = json.loads(r.read().decode())
        ids = [m.get("id") for m in (data.get("data") or [])]
        return {"ok": model in ids, "available": ids}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def check_chat(base_router: str, api_key: str, model: str = "qd/qfmodel",
               timeout: int = 120) -> Dict[str, Any]:
    """Kirim test chat minimal & verifikasi respons nyata dari model."""
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": "Reply with exactly: PONG"}],
        "max_tokens": 16,
        "stream": False,
    }).encode("utf-8")
    req = urllib.request.Request(
        f"{base_router}/v1/chat/completions",
        data=body,
        headers={"Content-Type": "application/json",
                 "Authorization": f"Bearer {api_key}",
                 "User-Agent": "Mozilla/5.0"},
    )
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            data = json.loads(r.read().decode())
        latency = round(time.time() - t0, 2)
        content = ""
        try:
            content = data["choices"][0]["message"]["content"]
        except Exception:
            pass
        # Deteksi balasan "queue" (bukan error, tapi belum benar-benar dilayani)
        queued = isinstance(data, dict) and data.get("isQueued")
        return {
            "ok": bool(content) and not queued,
            "latency_s": latency,
            "content": content[:80],
            "queued": bool(queued),
            "usage": data.get("usage"),
        }
    except urllib.error.HTTPError as e:
        return {"ok": False, "error": f"HTTP {e.code}: {e.read().decode()[:200]}",
                "latency_s": round(time.time() - t0, 2)}
    except Exception as e:
        return {"ok": False, "error": str(e), "latency_s": round(time.time() - t0, 2)}


def run_health(base_router: str = None, model: str = "qd/qfmodel",
               db_path: str = None, api_key: str = None) -> Dict[str, Any]:
    """Jalankan seluruh rangkaian health check. Return dict hasil."""
    base = _base_router(base_router or NINE_ROUTER_URL)
    result: Dict[str, Any] = {"base": base, "model": model, "steps": {}}

    # 1. Router hidup
    r1 = check_router(base)
    result["steps"]["router"] = r1

    # 2. API key
    key = api_key or _get_api_key(db_path, model)
    result["steps"]["api_key"] = {"ok": bool(key), "found": bool(key)}
    if not key:
        result["ok"] = False
        return result

    # 3. Model terekspos
    r3 = check_model(base, model, key)
    result["steps"]["model"] = r3

    # 4. Test chat nyata
    r4 = check_chat(base, key, model)
    result["steps"]["chat"] = r4

    result["ok"] = bool(r1.get("ok") and r3.get("ok") and r4.get("ok"))
    return result

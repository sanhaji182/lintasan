#!/usr/bin/env python3
"""Which ACTIVE connections can actually serve a given model right now?

Read-only and free: one GET {base_url}{models_path} per active connection — no
chat completion, no token spend. Reports, per connection:
  - reachability + HTTP status
  - whether the model id is genuinely served upstream
  - credential presence

Usage:
    python3 scripts/find_live_provider_for_model.py gemini-2.5-flash
    python3 scripts/find_live_provider_for_model.py <model> --all   # every active conn, not just gemini ones
"""
import json
import os
import sqlite3
import sys
import urllib.error
import urllib.request

DB = os.environ.get("LINTASAN_DB", os.path.expanduser("~/lintasan-go/data/lintasan.db"))


def resolve_token(conn, conn_id, api_key, oauth_provider):
    if (api_key or "").strip():
        return api_key.strip(), "api_key"
    if oauth_provider:
        row = conn.execute(
            "SELECT access_token FROM oauth_sessions WHERE provider=? "
            "AND status IN ('active','expiring') ORDER BY datetime(created_at) DESC LIMIT 1",
            (oauth_provider,)).fetchone()
        if row and (row[0] or "").strip():
            return row[0].strip(), f"oauth:{oauth_provider}"
    return "", "MISSING"


def probe(url, token):
    # A browser-like User-Agent is REQUIRED: several providers sit behind
    # Cloudflare, which answers plain script UAs with `403 error code: 1010`.
    # That is bot protection, not a dead endpoint — reporting it as "down" hides
    # perfectly healthy providers. curl is used because it is what the gateway's
    # own probes use and it produced HTTP 200 where urllib got 1010.
    import subprocess
    base = ["curl", "-s", "--max-time", "20",
            "-A", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
                  "(KHTML, like Gecko) Chrome/125.0 Safari/537.36"]
    auth = ["-H", f"Authorization: Bearer {token}"] if token else []
    try:
        body = subprocess.run(base + auth + [url],
                              capture_output=True, text=True, timeout=30).stdout
        # Status is fetched separately so the body stays uncorrupted JSON.
        code = int(subprocess.run(base + auth + ["-o", "/dev/null", "-w", "%{http_code}", url],
                                  capture_output=True, text=True, timeout=30).stdout.strip() or 0)
    except Exception as e:
        return 0, f"{type(e).__name__}: {e}"
    return code, body


def extract_ids(body):
    try:
        d = json.loads(body)
    except Exception:
        return []
    data = d.get("data") or d.get("models") or []
    out = []
    for m in data if isinstance(data, list) else []:
        if isinstance(m, dict):
            mid = m.get("id") or m.get("name") or ""
            if mid:
                out.append(mid.split("/")[-1] if mid.startswith("models/") else mid)
    return out


def main(argv):
    if len(argv) < 2:
        print(__doc__)
        return 2
    model = argv[1]
    everywhere = "--all" in argv

    conn = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    if everywhere:
        rows = conn.execute("""
            SELECT id, name, base_url, COALESCE(models_path,'/v1/models'), api_key,
                   COALESCE(oauth_provider,''), format
            FROM connections WHERE is_active=1 ORDER BY name""").fetchall()
    else:
        rows = conn.execute("""
            SELECT c.id, c.name, c.base_url, COALESCE(c.models_path,'/v1/models'), c.api_key,
                   COALESCE(c.oauth_provider,''), c.format
            FROM connections c
            WHERE c.is_active=1 AND EXISTS (
                SELECT 1 FROM discovered_models d
                WHERE d.connection_id=c.id AND d.is_active=1 AND d.model_id LIKE '%gemini%')
            ORDER BY c.name""").fetchall()

    print(f"probing {len(rows)} active connection(s) for model: {model}\n")
    served, healthy = [], []
    for cid, name, base, path, api_key, oauth, fmt in rows:
        token, src = resolve_token(conn, cid, api_key, oauth)
        url = f"{(base or '').rstrip('/')}{path or '/v1/models'}"
        code, body = probe(url, token)
        ids = extract_ids(body)
        label = (name or base or cid)[:34]
        if code == 200:
            healthy.append((label, len(ids)))
            hit = model in ids
            if hit:
                served.append((label, cid, src))
            mark = "SERVES" if hit else "no    "
            print(f"  [{code}] {mark} {label:<34} models={len(ids):<4} cred={src}")
        else:
            snippet = body.replace("\n", " ")[:70]
            print(f"  [{code or 'ERR'}] ------ {label:<34} cred={src:<22} {snippet}")
    conn.close()

    print(f"\nreachable (HTTP 200): {len(healthy)}/{len(rows)}")
    print(f"actually serves {model}: {len(served)}")
    for label, cid, src in served:
        print(f"  - {label}  id={cid}  cred={src}")
    return 0 if served else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))

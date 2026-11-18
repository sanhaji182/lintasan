#!/usr/bin/env python3
"""Which candidate models can the gateway actually SERVE right now?

/models returning 200 does not prove a provider has credit — clouvia serves
gemini-2.5-flash in its catalogue yet answers every chat with 402. The only
honest test is a real (tiny) chat completion through the gateway, which also
exercises routing and failover. max_tokens=1 keeps the cost negligible.

Usage:
    python3 scripts/probe_serve_models.py                       # default candidate list
    python3 scripts/probe_serve_models.py modelA modelB ...
    python3 scripts/probe_serve_models.py --gemini              # every gemini* in catalogue

Exit 0 if at least one candidate was served.
"""
import json
import os
import sqlite3
import sys
import time
import urllib.error
import urllib.request

DB = os.environ.get("LINTASAN_DB", os.path.expanduser("~/lintasan-go/data/lintasan.db"))
BASE = os.environ.get("LINTASAN_BASE", "http://localhost:20180")

DEFAULT_CANDIDATES = [
    "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-2.5-pro",
    "gemini-3-flash", "gemini-3.1-flash-lite", "gemini-3.5-flash-lite",
    "gemini-3.8-flash", "gemini-3.8-flash-low", "gemini-3.8-flash-medium",
    "gemini-3.8-flash-high", "gemini-3.8-flash-tiered",
]


def catalogue_models(conn, prefix):
    rows = conn.execute(
        "SELECT DISTINCT model_id FROM discovered_models WHERE is_active=1 AND model_id LIKE ?",
        (f"%{prefix}%",))
    return sorted({r[0] for r in rows})


def probe(base, master, model):
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": f"ping-{time.time()}"}],
        "max_tokens": 1,
    }).encode()
    req = urllib.request.Request(
        f"{base}/v1/chat/completions", data=body,
        headers={"Content-Type": "application/json",
                 "Authorization": f"Bearer {master}"})
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=90) as r:
            txt = r.read().decode()
            return r.status, round(time.time() - t0, 2), txt
    except urllib.error.HTTPError as e:
        return e.code, round(time.time() - t0, 2), e.read().decode()
    except Exception as e:
        return 0, round(time.time() - t0, 2), f"{type(e).__name__}: {e}"


def main(argv):
    args = [a for a in argv[1:] if not a.startswith("--")]
    conn = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    master = conn.execute("SELECT value FROM settings WHERE key='master_key'").fetchone()[0]
    if "--gemini" in argv:
        models = catalogue_models(conn, "gemini")
    else:
        models = args or DEFAULT_CANDIDATES
    conn.close()

    print(f"probing {len(models)} model(s) with max_tokens=1\n")
    served, failed = [], []
    for m in models:
        code, elapsed, txt = probe(BASE, master, m)
        if code == 200:
            served.append(m)
            print(f"  [200] OK      {m:<32} {elapsed:>5}s")
        else:
            try:
                err = json.loads(txt)
                detail = (err.get("error") or {}).get("details") or (err.get("error") or {}).get("message") or ""
            except Exception:
                detail = txt
            failed.append(m)
            print(f"  [{code}] FAIL    {m:<32} {elapsed:>5}s  {str(detail)[:80]}")

    print(f"\nserved: {len(served)}/{len(models)}")
    for m in served:
        print(f"  + {m}")
    return 0 if served else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))

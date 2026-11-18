#!/usr/bin/env python3
"""Show which connection served a single request, and how the attempt moved.

Read-only against production: it issues ONE chat completion and then reads the
request_logs rows that request produced. Useful after a deploy to prove that a
dead OAuth account no longer black-holes a model — you should see the dead
connection fail fast and a healthy one answer, rather than a single 401 after
several seconds of retries.

    python3 scripts/verify_failover_prod.py                 # default model
    python3 scripts/verify_failover_prod.py gemini-2.5-flash
    python3 scripts/verify_failover_prod.py <model> --json

Exit code is 0 whenever the request was actually dispatched (whatever the
upstream answered), because the point is observing the routing, not asserting
that a specific provider has credit.
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
DEFAULT_MODEL = "gemini-2.5-flash"


def main(argv):
    args = [a for a in argv[1:] if not a.startswith("--")]
    as_json = "--json" in argv
    model = args[0] if args else DEFAULT_MODEL

    conn = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    master = conn.execute("SELECT value FROM settings WHERE key='master_key'").fetchone()[0]
    # request_logs.id is NOT monotonic, so anchor on the timestamp instead.
    anchor = conn.execute("SELECT datetime('now','localtime')").fetchone()[0]

    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": f"failover-probe-{time.time()}"}],
        "max_tokens": 4,
    }).encode()
    req = urllib.request.Request(
        f"{BASE}/v1/chat/completions", data=body,
        headers={"Content-Type": "application/json",
                 "Authorization": f"Bearer {master}"})

    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=90) as r:
            code, text = r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        code, text = e.code, e.read().decode()
    except Exception as e:
        print(f"request failed: {e}", file=sys.stderr)
        conn.close()
        return 2
    elapsed = time.time() - t0

    rows = list(conn.execute("""
        SELECT connection_id, model, status, latency_ms, COALESCE(error,'')
        FROM request_logs
        WHERE created_at >= datetime(?, '-10 seconds')
          AND model LIKE ?
        ORDER BY created_at, id
    """, (anchor, f"%{model}%")))
    conn.close()

    if as_json:
        print(json.dumps({"http": code, "elapsed_s": round(elapsed, 3),
                          "body": text, "attempts": [
                              {"connection_id": r[0], "model": r[1], "status": r[2],
                               "latency_ms": r[3], "error": r[4][:160]} for r in rows]},
                         indent=2))
        return 0

    print(f"POST /v1/chat/completions  model={model}")
    print(f"  HTTP {code} in {elapsed:.2f}s")
    print(f"  body: {text[:200].replace(chr(10), ' ')}")
    print()
    if not rows:
        print("  no request_logs rows matched — check the model name")
    else:
        print("  attempts (in order):")
        for cid, m, status, lat, err in rows:
            note = err[:100] if err else ""
            print(f"    {status}  conn={cid[:8]}  {lat:>5}ms  {note}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))

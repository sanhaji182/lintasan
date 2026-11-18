#!/usr/bin/env python3
"""List the gemini models each REACHABLE connection really serves upstream, then
test them through the gateway.

Two distinct facts, often confused:
  - a connection lists a model in its own /models  (advertised)
  - the model answers a chat completion            (serveable, has credit)

Read-only except the tiny max_tokens=1 completions.
"""
import json
import os
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request

DB = os.environ.get("LINTASAN_DB", os.path.expanduser("~/lintasan-go/data/lintasan.db"))
BASE = os.environ.get("LINTASAN_BASE", "http://localhost:20180")
UA = ("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/125.0 Safari/537.36")


def curl_json(url, token):
    cmd = ["curl", "-s", "--max-time", "20", "-A", UA]
    if token:
        cmd += ["-H", f"Authorization: Bearer {token}"]
    out = subprocess.run(cmd + [url], capture_output=True, text=True).stdout
    try:
        return json.loads(out)
    except Exception:
        return None


def probe_chat(master, model):
    body = json.dumps({"model": model,
                       "messages": [{"role": "user", "content": f"p{time.time()}"}],
                       "max_tokens": 1}).encode()
    req = urllib.request.Request(f"{BASE}/v1/chat/completions", data=body,
                                 headers={"Content-Type": "application/json",
                                          "Authorization": f"Bearer {master}"})
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, ""
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()[:100]
    except Exception as e:
        return 0, str(e)[:100]


def main():
    conn = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    master = conn.execute("SELECT value FROM settings WHERE key='master_key'").fetchone()[0]
    conns = conn.execute("""
        SELECT name, base_url, COALESCE(models_path,'/v1/models'), api_key
        FROM connections WHERE is_active=1 AND COALESCE(oauth_provider,'')=''
    """).fetchall()
    conn.close()

    found = {}
    for name, base, path, key in conns:
        url = f"{(base or '').rstrip('/')}{path}"
        d = curl_json(url, (key or "").strip())
        if not d:
            continue
        data = d.get("data") or d.get("models") or []
        ids = [(m.get("id") or m.get("name") or "") for m in data if isinstance(m, dict)]
        gem = [i for i in ids if "gemini" in i.lower()]
        if gem:
            print(f"\n{name[:40]}  -> {len(gem)} gemini model(s)")
            for g in gem[:12]:
                print(f"    {g}")
            found[name[:40]] = gem

    if not found:
        print("no reachable connection advertises a gemini model")
        return 1

    # Test the gateway's own routing for a sample: does the bare model id work?
    print("\n=== gateway routing test for UNPREFIXED gemini ids")
    uniq = sorted({g.split("/")[-1] for gs in found.values() for g in gs})
    ok = 0
    for m in uniq[:12]:
        code, detail = probe_chat(master, m)
        print(f"  [{code}] {m:<34} {detail}")
        if code == 200:
            ok += 1
    print(f"\nserved via gateway: {ok}/{min(len(uniq),12)}")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())

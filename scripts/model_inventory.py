#!/usr/bin/env python3
"""Model inventory + price report for one connection.

Lists every model the connection advertises with its catalogue price, context
window, and — with --probe — whether it actually serves right now. "/models
lists it" is not the same as "it answers": a plan-restricted Claude model shows
up and then 403s with MODEL_NOT_IN_PLAN.

Prices come from the embedded Go catalogue (internal/models/catalog.go), which is
what the dashboard and /api/models/catalog serve. A model missing there is
reported as unpriced rather than guessed.

    python3 scripts/model_inventory.py                 # default: oauth-antigravity
    python3 scripts/model_inventory.py <conn-name-or-id> --probe
"""
import json
import os
import re
import sqlite3
import subprocess
import sys
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DB = os.environ.get("LINTASAN_DB", os.path.join(ROOT, "data", "lintasan.db"))
BASE = os.environ.get("LINTASAN_BASE", "http://localhost:20180")
CATALOG = os.path.join(ROOT, "internal", "models", "catalog.go")
DEFAULT_TARGET = "oauth-antigravity"


def catalog_prices():
    """id -> (input$, output$, context, maxtokens) per 1M tokens."""
    out = {}
    if not os.path.exists(CATALOG):
        return out
    src = open(CATALOG, encoding="utf-8").read()
    for m in re.finditer(
            r'ID:\s*"([^"]+)".*?ContextWindow:\s*(\d+).*?MaxTokens:\s*(\d+)'
            r'.*?InputPrice:\s*([\d.]+).*?OutputPrice:\s*([\d.]+)', src):
        mid, ctx, mx, inp, outp = m.groups()
        out[mid] = (float(inp), float(outp), int(ctx), int(mx))
    # Variants the catalogue stores under a base id (e.g. "-thinking" suffixes and
    # tiered speed variants share the parent model's price).
    for mid, (inp, outp, ctx, mx) in list(out.items()):
        for suffix in ("-thinking", "-high", "-medium", "-low", "-tiered"):
            out.setdefault(mid + suffix, (inp, outp, ctx, mx))
    out.setdefault("gemini-4-hydrogen-sonar", out.get("gemini-4-hydrogen", (1.50, 7.50, 1048576, 8192)))
    out.setdefault("gpt-oss-120b-medium", (0.15, 0.60, 128000, 8192))
    return out


def resolve_conn(target):
    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    row = c.execute("""
        SELECT id, name, COALESCE(oauth_provider,'') FROM connections
        WHERE id = ? OR name LIKE ? ORDER BY is_active DESC LIMIT 1""",
        (target, f"%{target}%")).fetchone()
    c.close()
    return row


def models_of(conn_id):
    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    rows = list(c.execute("""
        SELECT model_id, COALESCE(max_input_tokens,0), COALESCE(max_output_tokens,0)
        FROM discovered_models WHERE connection_id = ? AND is_active = 1
        ORDER BY model_id""", (conn_id,)))
    c.close()
    return rows


def spend_of(conn_id):
    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    rows = {r[0]: r[1:] for r in c.execute("""
        SELECT model, COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
        FROM request_logs WHERE connection_id = ? GROUP BY model""", (conn_id,))}
    c.close()
    return rows


def probe(master, model, max_tokens=1):
    body = json.dumps({"model": model,
                       "messages": [{"role": "user", "content": f"p{time.time()}"}],
                       "max_tokens": max_tokens}).encode()
    cmd = ["curl", "-s", "--max-time", "90", "-o", "/dev/null",
           "-w", "%{http_code}", "-X", "POST", f"{BASE}/v1/chat/completions",
           "-H", "Content-Type: application/json",
           "-H", f"Authorization: Bearer {master}", "-d", body]
    try:
        return int(subprocess.run(cmd, capture_output=True, text=True,
                                  timeout=100).stdout.strip() or 0)
    except Exception:
        return 0


def main(argv):
    args = [a for a in argv[1:] if not a.startswith("--")]
    do_probe = "--probe" in argv
    target = args[0] if args else DEFAULT_TARGET

    conn = resolve_conn(target)
    if not conn:
        print(f"no connection matched: {target}")
        return 1
    cid, name, oauth = conn
    prices = catalog_prices()
    models = models_of(cid)
    spend = spend_of(cid)

    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    master = c.execute("SELECT value FROM settings WHERE key='master_key'").fetchone()[0]
    c.close()

    print(f"connection : {name}  ({cid[:8]})  oauth={oauth or '-'}")
    print(f"models     : {len(models)}"
          f"{'  (probing, max_tokens=1)' if do_probe else '  (pass --probe to test serving)'}\n")
    print(f"{'model':<30} {'in $/1M':>8} {'out $/1M':>9} {'ctx':>9}  {'used':>5}  serve")
    print("-" * 78)

    served = 0
    for mid, mx_in, mx_out in models:
        p = prices.get(mid)
        inp = f"{p[0]:.2f}" if p else "?"
        outp = f"{p[1]:.2f}" if p else "?"
        ctx = f"{mx_in:,}" if mx_in else (f"{p[2]:,}" if p else "?")
        used = spend.get(mid, (0, 0, 0))
        code = probe(master, mid) if do_probe else None
        if code == 200:
            served += 1
        tag = "" if code is None else f"{code}"
        print(f"{mid:<30} {inp:>8} {outp:>9} {ctx:>9}  {used[0]:>5}  {tag}")

    if do_probe:
        print(f"\nserving now: {served}/{len(models)}")
    unpriced = [m[0] for m in models if m[0] not in prices]
    if unpriced:
        print(f"\nnot in catalogue (no price published): {', '.join(unpriced)}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))

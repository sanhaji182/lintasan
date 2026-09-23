#!/usr/bin/env python3
"""Qoder burn-rate watchdog — alerts when the pool drains faster than WE spent.

Why this exists (decided 2026-09-23, operator choice "option 2"): the nine Qoder
PATs in Lintasan's pool are SHARED with 9router on baby, which holds byte-identical
copies and calls openapi.qoder.sh directly. That sharing is accepted, so the pool is
shared capacity and sizing it by absolute balance is wrong — what matters is the burn
RATE, and specifically the part of it we cannot account for.

The detector is a comparison, and either half alone says nothing:

    pool drop (measured, all nine accounts)
      vs
    SUM(request_logs.credits) for our own qoder connections over the same window

  * pool drops, our spend matches        -> normal, we did it. Silent.
  * pool drops, our spend does NOT match -> someone else is spending it. ALERT.

On 2026-09-23 the ratio was 280 -> 27 pool vs 5.49 recorded: ~98% unaccounted,
which is exactly the shape this reports.

Quiet by design: no output unless a threshold is crossed, so the cron stays silent
while sharing behaves. Exit 0 in every normal case; a non-zero exit means the
watchdog itself broke, which the delayed-alert path surfaces.

Usage:  qoder_burn_watch.py [--verbose] [--interval-seconds N] [--reset-state]
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

DB = Path("/home/ubuntu/lintasan-go/data/lintasan.db")
BASE = "http://localhost:20180"
STATE = Path.home() / ".hermes/state/qoder_burn_watch.json"

# Thresholds. Deliberately conservative: the pool is 2700 credits when full, so a
# 20-credit move in 5 minutes is ~0.7% of capacity. Tuned to catch the 2026-09-23
# shape (95 requests / 267 credits over ~2.4h = ~9 credits/min) with margin, without
# firing on a single heavy turn (a cold 13K-token turn is ~4.6 credits).
MIN_POOL_DROP = 20.0      # credits lost between two readings
MIN_UNACCOUNTED = 15.0    # of which this much is unattributable to us
UNACCOUNTED_RATIO = 0.50  # and it is at least this share of the drop

# Cache TTL of the quota endpoint is 5 minutes, so readings must be forced fresh or
# the delta is measured against stale numbers (a trap that produced a false "0 spent"
# answer during the original investigation).
DEFAULT_INTERVAL = 300


def sh(*args: str, timeout: int = 60) -> str:
    r = subprocess.run(args, capture_output=True, text=True, timeout=timeout)
    if r.returncode != 0:
        raise RuntimeError(f"{args[0]} failed: {r.stderr.strip()[:200]}")
    return r.stdout.strip()


def master_key() -> str:
    return sh("sqlite3", str(DB), "SELECT value FROM settings WHERE key='master_key';")


def qoder_conns() -> list[tuple[str, int, str]]:
    out = sh("sqlite3", str(DB),
             "SELECT id||'|'||priority||'|'||name FROM connections "
             "WHERE LOWER(format)='qoder' ORDER BY priority DESC;")
    rows = []
    for line in out.splitlines():
        cid, prio, name = line.split("|", 2)
        rows.append((cid, int(prio), name))
    return rows


def api_get(path: str, mk: str, timeout: int = 120) -> dict | None:
    req = urllib.request.Request(BASE + path, headers={"Authorization": f"Bearer {mk}"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return json.loads(resp.read())
    except (urllib.error.URLError, json.JSONDecodeError, TimeoutError):
        return None


def read_pool(mk: str, conns: list[tuple[str, int, str]]) -> dict:
    """Force-fresh per-account read, then the rollup.

    Returns {"total": float|None, "accounts": {cid: remaining|None}, "states": {...}}.
    A missing `remaining` is recorded as None, never 0 — an account whose fetch
    failed and an account that is genuinely empty must not look the same, which is
    the bug that printed "left=0" for a failed fetch mid-investigation.
    """
    for cid, _, _ in conns:
        api_get(f"/api/qoder/quota/{cid}?refresh=1", mk, timeout=90)

    data = api_get("/api/qoder/quota", mk, timeout=240)
    if not data:
        return {"total": None, "accounts": {}, "states": {}}

    total = 0.0
    accounts: dict[str, float | None] = {}
    states: dict[str, str] = {}
    for row in data.get("data") or []:
        cid = row.get("connection_id") or ""
        if row.get("error"):
            states[cid] = "ERROR"
            accounts[cid] = None
            continue
        q = row.get("quota")
        if not q:
            states[cid] = "NULL"
            accounts[cid] = None
            continue
        uq = q.get("user_quota")
        if not uq:
            states[cid] = "EMPTY"
            accounts[cid] = None
            continue
        rem = uq.get("remaining")
        if isinstance(rem, (int, float)):
            states[cid] = "OK"
            accounts[cid] = float(rem)
            total += float(rem)
        else:
            states[cid] = "NOMINAL"
            accounts[cid] = None
    return {"total": total, "accounts": accounts, "states": states}


def our_spend(since_iso: str) -> tuple[float, int]:
    """Credits WE recorded for qoder in the window, and how many rows carried one."""
    out = sh("sqlite3", str(DB),
             "SELECT COALESCE(SUM(credits),0)||'|'||COUNT(credits) FROM request_logs "
             "WHERE connection_id IN (SELECT id FROM connections WHERE LOWER(format)='qoder') "
             f"AND created_at >= '{since_iso}';")
    credits, rows = out.split("|")
    return float(credits), int(rows)


def load_state() -> dict | None:
    try:
        return json.loads(STATE.read_text())
    except (OSError, json.JSONDecodeError):
        return None


def save_state(payload: dict) -> None:
    STATE.parent.mkdir(parents=True, exist_ok=True)
    tmp = STATE.with_suffix(".json.tmp")
    tmp.write_text(json.dumps(payload, indent=2))
    os.chmod(tmp, 0o600)
    tmp.replace(STATE)


def fmt(v) -> str:
    return "?" if v is None else f"{v:,.1f}"


def main() -> int:
    verbose = "--verbose" in sys.argv
    if "--reset-state" in sys.argv:
        STATE.unlink(missing_ok=True)
        print("burn-watchdog state cleared")
        return 0

    interval = DEFAULT_INTERVAL
    if "--interval-seconds" in sys.argv:
        interval = int(sys.argv[sys.argv.index("--interval-seconds") + 1])

    now = time.time()
    mk = master_key()
    conns = qoder_conns()
    pool = read_pool(mk, conns)

    prev = load_state()

    # No baseline yet: record one and stay silent. The FIRST run cannot compute a
    # delta, and reporting one would mean comparing against nothing.
    if not prev or pool["total"] is None:
        save_state({"at": now, "total": pool["total"], "accounts": pool["accounts"],
                    "states": pool["states"], "t0_local": time.strftime("%Y-%m-%d %H:%M:%S")})
        if verbose:
            print(f"baseline recorded: total={fmt(pool['total'])} at {time.strftime('%H:%M:%S')}")
        return 0

    elapsed = now - prev.get("at", now)
    pool_drop = None
    if prev.get("total") is not None and pool["total"] is not None:
        pool_drop = prev["total"] - pool["total"]

    spent, spend_rows = our_spend(prev.get("t0_local", "1970-01-01 00:00:00"))

    save_state({"at": now, "total": pool["total"], "accounts": pool["accounts"],
                "states": pool["states"], "t0_local": time.strftime("%Y-%m-%d %H:%M:%S")})

    if pool_drop is None:
        if verbose:
            print("pool total unreadable this cycle; baseline updated, no verdict")
        return 0

    unaccounted = pool_drop - spent

    if verbose:
        print(f"elapsed={elapsed:.0f}s pool_drop={pool_drop:.3f} our_spend={spent:.3f} "
              f"unaccounted={unaccounted:.3f} rows={spend_rows}")

    # Alert condition: a real drop, of which most is not attributable to us.
    if (pool_drop >= MIN_POOL_DROP
            and unaccounted >= MIN_UNACCOUNTED
            and pool_drop > 0
            and (unaccounted / pool_drop) >= UNACCOUNTED_RATIO):

        # Per-account movers, so the alert says WHERE the credits went.
        movers = []
        for cid, prio, name in conns:
            before = (prev.get("accounts") or {}).get(cid)
            after = (pool["accounts"] or {}).get(cid)
            if isinstance(before, (int, float)) and isinstance(after, (int, float)):
                d = before - after
                if abs(d) >= 1.0:
                    movers.append((d, name, before, after, (pool["states"] or {}).get(cid, "?")))
        movers.sort(reverse=True)

        lines = [
            f"🔥 Qoder pool burning off-gateway",
            f"",
            f"Pool: {fmt(prev['total'])} → {fmt(pool['total'])}  (drop {pool_drop:,.1f})",
            f"Ours: {spent:,.1f} credits over {spend_rows} request(s) in request_logs",
            f"Unaccounted: {unaccounted:,.1f}  ({(unaccounted / pool_drop) * 100:.0f}% of the drop)",
            f"",
        ]
        if movers:
            lines.append("Who moved:")
            for d, name, before, after, state in movers[:8]:
                lines.append(f"  {name}: {before:,.1f} → {after:,.1f}  (−{d:,.1f})")
            lines.append("")
        read_states = {s for s in (pool["states"] or {}).values()}
        if read_states - {"OK"}:
            lines.append(f"⚠️ some accounts did not read cleanly: {sorted(read_states - {'OK'})}")
            lines.append("")
        lines += [
            "A drop we cannot attribute to our own request_logs means another consumer",
            "on the shared PATs (9router on baby holds all nine). Shared capacity is",
            "accepted — this is the burn-rate signal, not a fault.",
            "",
            "Verify: python3 /home/ubuntu/lintasan-go/scripts/pool_quota.py --refresh",
        ]
        print("\n".join(lines))
        return 0

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:  # noqa: BLE001 — a broken watchdog must be visible
        print(f"⚠️ qoder burn watchdog FAILED: {type(exc).__name__}: {exc}")
        sys.exit(1)

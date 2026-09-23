#!/usr/bin/env python3
"""Read the Qoder pool quota and report honestly.

The first pass at this reported `left=0` for every account because the reader
defaulted a missing `user_quota.remaining` to 0 — which is indistinguishable from
a genuine zero balance. A failed fetch and an empty account must never print the
same number, so this distinguishes four states per account:

  OK        quota present, buckets read
  EMPTY     quota present but carries no user_quota bucket
  NULL      no quota object returned at all (fetch failed or was refused)
  ERROR     the handler reported an error string for this connection

Usage: pool_quota.py [--refresh]
"""
import json
import subprocess
import sys
import time

DB = "/home/ubuntu/lintasan-go/data/lintasan.db"
BASE = "http://localhost:20180"


def master_key() -> str:
    out = subprocess.run(
        ["sqlite3", DB, "SELECT value FROM settings WHERE key='master_key';"],
        capture_output=True, text=True, check=True,
    ).stdout.strip()
    if not out:
        sys.exit("master_key not found")
    return out


def qoder_conns():
    out = subprocess.run(
        ["sqlite3", DB,
         "SELECT id||'|'||priority||'|'||name FROM connections "
         "WHERE LOWER(format)='qoder' ORDER BY priority DESC;"],
        capture_output=True, text=True, check=True,
    ).stdout.strip()
    rows = []
    for line in out.splitlines():
        cid, prio, name = line.split("|", 2)
        rows.append((cid, int(prio), name))
    return rows


def curl(url: str, mk: str, timeout: int = 120):
    r = subprocess.run(
        ["curl", "-s", "-m", str(timeout), url, "-H", f"Authorization: Bearer {mk}"],
        capture_output=True, text=True,
    )
    if r.returncode != 0:
        return None, f"curl exit {r.returncode}"
    try:
        return json.loads(r.stdout), None
    except json.JSONDecodeError as e:
        return None, f"bad JSON: {e}"


def main():
    refresh = "--refresh" in sys.argv
    mk = master_key()
    conns = qoder_conns()
    if refresh:
        for cid, _, _ in conns:
            curl(f"{BASE}/api/qoder/quota/{cid}?refresh=1", mk, timeout=90)

    data, err = curl(f"{BASE}/api/qoder/quota", mk, timeout=240)
    if err:
        sys.exit(f"pool read failed: {err}")

    rows = data.get("data") or []
    total = 0
    zero_real, null_ct, err_ct, ok_ct = [], 0, 0, 0

    print(f"  {'prio':>4} {'account':30} {'state':6} {'used':>6} {'total':>6} {'left':>6} {'exc':>5}")
    print("  " + "-" * 74)
    for r in rows:
        name = (r.get("name") or "")[:30]
        q = r.get("quota")
        e = r.get("error")
        if e:
            state, err_ct = "ERROR", err_ct + 1
            print(f"  {r.get('priority'):>4} {name:30} {state:6} {'-':>6} {'-':>6} {'-':>6} {'-':>5}  {str(e)[:40]}")
            continue
        if not q:
            null_ct += 1
            print(f"  {r.get('priority'):>4} {name:30} {'NULL':6} {'-':>6} {'-':>6} {'-':>6} {'-':>5}")
            continue
        uq = q.get("user_quota")
        if not uq:
            print(f"  {r.get('priority'):>4} {name:30} {'EMPTY':6} {'-':>6} {'-':>6} {'-':>6} {'-':>5}")
            continue
        ok_ct += 1
        used = uq.get("used")
        tot = uq.get("total")
        rem = uq.get("remaining")
        left = rem if isinstance(rem, (int, float)) else 0
        total += left
        if left <= 0:
            zero_real.append(name)
        print(f"  {r.get('priority'):>4} {name:30} {'OK':6} {str(used):>6} {str(tot):>6} {str(rem):>6} {str(q.get('is_quota_exceeded')):>5}")

    print()
    print(f"  read OK={ok_ct}  NULL(fetch failed)={null_ct}  ERROR={err_ct}")
    print(f"  genuinely zero balance: {len(zero_real)} -> {', '.join(zero_real) if zero_real else 'none'}")
    print(f"  POOL TOTAL (sum of accounts read OK) = {total}")
    print(f"  snapshot at {time.strftime('%Y-%m-%d %H:%M:%S')} {'(force-refreshed)' if refresh else '(cache)'}")


if __name__ == "__main__":
    main()

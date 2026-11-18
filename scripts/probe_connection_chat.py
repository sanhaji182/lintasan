#!/usr/bin/env python3
"""Does a specific connection actually COMPLETE a chat for a model it lists?

MAX_TOKENS=1 so the cost is effectively zero. This distinguishes "listed in
/models" from "will answer" (credit, entitlement, upstream capacity).

    python3 scripts/probe_connection_chat.py <conn-name-or-id> <model-id> [model2 ...]
"""
import json
import os
import sqlite3
import subprocess
import sys

DB = os.environ.get("LINTASAN_DB", os.path.expanduser("~/lintasan-go/data/lintasan.db"))
UA = ("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/125.0 Safari/537.36")


def main(argv):
    if len(argv) < 3:
        print(__doc__)
        return 2
    target, models = argv[1], argv[2:]

    conn = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    row = conn.execute("""
        SELECT id, name, base_url, COALESCE(chat_path,'/v1/chat/completions'), api_key
        FROM connections WHERE id = ? OR name LIKE ? ORDER BY is_active DESC LIMIT 1
    """, (target, f"%{target}%")).fetchone()
    conn.close()
    if not row:
        print(f"no connection matched: {target}")
        return 1
    cid, name, base, chat_path, key = row
    url = f"{(base or '').rstrip('/')}{chat_path}"
    print(f"connection: {name[:44]}  ({cid[:8]})\nPOST {url}\n")

    ok = 0
    for m in models:
        payload = {"model": m,
                   "messages": [{"role": "user", "content": "say ok"}],
                   "max_tokens": 20}
        cmd = ["curl", "-s", "--max-time", "60", "-A", UA,
               "-H", "Content-Type: application/json",
               "-H", f"Authorization: Bearer {(key or '').strip()}",
               "-d", json.dumps(payload), url]
        out = subprocess.run(cmd, capture_output=True, text=True).stdout
        served = False
        detail = out[:110].replace("\n", " ")
        try:
            d = json.loads(out)
            # Some providers nest the OpenAI envelope under "data"
            # (Clinepass answers {"data":{"choices":[...]}}); accept both shapes
            # or a working provider looks broken.
            env = d.get("data") if isinstance(d.get("data"), dict) else d
            choices = (env or {}).get("choices") or d.get("choices")
            if choices:
                served = True
                msg = (choices[0].get("message") or {}).get("content")
                if msg is None:
                    msg = (choices[0].get("message") or {}).get("reasoning")
                detail = f"content={str(msg)[:40]!r}"
        except Exception:
            pass
        ok += served
        print(f"  [{'SERVED' if served else 'FAIL  '}] {m:<34} {detail}")
    print(f"\nserved {ok}/{len(models)}")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))

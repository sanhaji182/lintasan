#!/usr/bin/env python3
"""Read-only post-deploy check: the catalogue must not advertise models that no
credentialed connection can serve.

Run after a deploy (or any time) to catch the "OAuth provider wired but dead"
class without touching production state:

    python3 scripts/verify_catalog_prod.py

Exits 0 when healthy, 1 when it finds advertised-but-unservable models.

What it checks
--------------
For every model in /v1/models, at least one connection exposing it must be able
to mint a credential. An OAuth connection whose provider has no active/restricted
session cannot — nothing mints a token without a human sign-in — so any model
advertised exclusively by such a connection is a route guaranteed to 401.

Read-only: opens the DB with mode=ro and calls the gateway's GET endpoints only.
"""
import json
import os
import sqlite3
import sys
import urllib.request

DB = os.environ.get("LINTASAN_DB", os.path.expanduser("~/lintasan-go/data/lintasan.db"))
BASE = os.environ.get("LINTASAN_BASE", "http://localhost:20180")


def die(msg):
    print(f"ERROR: {msg}", file=sys.stderr)
    return 1


def main():
    if not os.path.exists(DB):
        return die(f"database not found: {DB}")

    conn = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    try:
        row = conn.execute("SELECT value FROM settings WHERE key='master_key'").fetchone()
        if not row or not row[0]:
            return die("master_key missing from settings")
        master = row[0]

        # Connections that cannot serve anything: OAuth providers with no usable
        # session. Mirrors the rule the server enforces (auth.pickOAuthSession):
        # 'expired' with a refresh token is revivable, so only presence of an
        # active/restricted row counts here; anything else is dead.
        dead_conns = {
            r[0]
            for r in conn.execute("""
                SELECT c.id FROM connections c
                WHERE COALESCE(c.oauth_provider,'') != ''
                  AND NOT EXISTS (
                    SELECT 1 FROM oauth_sessions s
                    WHERE s.provider = c.oauth_provider
                      AND s.status IN ('active','restricted'))
            """)
        }

        servable = set()
        for conn_id, model_id in conn.execute("""
                SELECT c.id, d.model_id
                FROM discovered_models d JOIN connections c ON c.id = d.connection_id
                WHERE d.is_active = 1 AND c.is_active = 1"""):
            if conn_id not in dead_conns:
                servable.add(model_id)

        advertised_db = {
            m for (m,) in conn.execute("""
                SELECT DISTINCT d.model_id FROM discovered_models d
                JOIN connections c ON c.id = d.connection_id
                WHERE d.is_active = 1 AND c.is_active = 1""")
        }

        oauth_conns = list(conn.execute("""
            SELECT c.id, c.oauth_provider, c.is_active,
                   (SELECT COUNT(*) FROM oauth_sessions s
                     WHERE s.provider = c.oauth_provider
                       AND s.status IN ('active','restricted'))
            FROM connections c WHERE COALESCE(c.oauth_provider,'') != ''"""))
    finally:
        conn.close()

    # Ask the running gateway what it actually advertises.
    req = urllib.request.Request(f"{BASE}/v1/models",
                                headers={"Authorization": f"Bearer {master}"})
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            data = json.loads(r.read())["data"]
    except Exception as e:
        return die(f"GET {BASE}/v1/models failed: {e}")
    live = {m["id"] for m in data}

    print(f"catalogue (live)        : {len(live)} models")
    print(f"OAuth connections       : {len(oauth_conns)}")
    for cid, provider, active, sessions in oauth_conns:
        state = "DEAD (no session)" if cid in dead_conns else "ok"
        print(f"  {provider:<14} active={active} sessions={sessions}  {state}")
    if not oauth_conns:
        print("  (none — every provider here uses an API key)")

    unservable = sorted(advertised_db - servable)
    leaked = sorted(live & set(unservable))

    print(f"models only dead providers expose : {len(unservable)}")
    if unservable:
        for m in unservable[:20]:
            print(f"  - {m}")
    print(f"still advertised in /v1/models    : {leaked or 'none'}")

    if leaked:
        print("\nFAIL: the catalogue advertises models that cannot be served.")
        return 1
    print("\nOK: every advertised model has a credentialed provider.")
    return 0


if __name__ == "__main__":
    sys.exit(main())

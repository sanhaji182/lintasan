#!/usr/bin/env bash
# Check that a provider connection is ALIVE without spending tokens.
#
# GET {base_url}/models costs nothing, so this verifies the endpoint + credential
# before you blame routing for a failure.
#
#   scripts/provider_models_probe.sh <connection-id|name-substring>
#   scripts/provider_models_probe.sh                     # list candidates
#
# Read-only: reads the local DB, one HTTP GET to the provider. Never writes.
set -uo pipefail
cd "$(dirname "$0")/.." || exit 1

DB="${LINTASAN_DB:-$HOME/lintasan-go/data/lintasan.db}"
TARGET="${1:-}"

if [ -z "$TARGET" ]; then
  echo "usage: $0 <connection-id|name-substring>"
  echo
  echo "connections (id, oauth_provider, active, name):"
  python3 - "$DB" <<'PY'
import sqlite3, sys
c = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
for cid, name, oauth, active in c.execute(
        "SELECT id, name, COALESCE(oauth_provider,''), is_active FROM connections "
        "ORDER BY is_active DESC, name LIMIT 60"):
    print(f"  {cid}  oauth={oauth or '-':<12} active={active}  {name[:50]}")
PY
  exit 0
fi

read -r BASE MODELS_PATH KEY OAUTH NAME < <(python3 - "$DB" "$TARGET" <<'PY'
import sqlite3, sys
db, target = sys.argv[1], sys.argv[2]
c = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
row = c.execute(
    "SELECT base_url, COALESCE(models_path,'/v1/models'), api_key, "
    "COALESCE(oauth_provider,''), name FROM connections WHERE id = ?", (target,)).fetchone()
if not row:
    row = c.execute(
        "SELECT base_url, COALESCE(models_path,'/v1/models'), api_key, "
        "COALESCE(oauth_provider,''), name FROM connections "
        "WHERE name LIKE ? ORDER BY is_active DESC LIMIT 1", (f"%{target}%",)).fetchone()
if not row:
    print("", "", "", "", "")
else:
    base, path, key, oauth, name = row
    # OAuth connections keep api_key empty; the token lives in oauth_sessions.
    if not (key or "").strip() and oauth:
        s = c.execute("SELECT access_token FROM oauth_sessions WHERE provider = ? "
                      "AND status IN ('active','expiring') ORDER BY datetime(created_at) DESC LIMIT 1",
                      (oauth,)).fetchone()
        key = (s[0] if s else "")
    print(base or "", path or "/v1/models", key or "", oauth or "-", (name or "").replace(" ", "_"))
PY
)

if [ -z "${BASE:-}" ]; then
  echo "no connection matched: $TARGET"
  exit 1
fi

URL="${BASE%/}${MODELS_PATH}"
echo "connection : $NAME"
echo "oauth      : $OAUTH"
echo "GET        : $URL"
if [ -z "${KEY:-}" ]; then
  echo "credential : MISSING — cannot authenticate (OAuth account absent, or key empty)"
  code=$(curl -s --max-time 20 -o /tmp/pm.json -w '%{http_code}' "$URL")
else
  echo "credential : present (${#KEY} chars)"
  code=$(curl -s --max-time 20 -o /tmp/pm.json -w '%{http_code}' \
    "$URL" -H "Authorization: Bearer ${KEY}")
fi
echo "HTTP       : $code"

python3 - <<'PY'
import json
try:
    d = json.load(open('/tmp/pm.json'))
except Exception:
    raw = open('/tmp/pm.json', 'rb').read()[:200]
    print("body       :", raw)
    raise SystemExit
data = d.get('data') or d.get('models') or []
if isinstance(data, list) and data:
    ids = [m.get('id') or m.get('name') or '' for m in data if isinstance(m, dict)]
    print(f"models     : {len(ids)}")
    for m in ids[:8]:
        print("   ", m)
else:
    print("body       :", json.dumps(d)[:200])
PY

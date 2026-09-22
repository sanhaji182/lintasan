#!/bin/bash
# Deploy a candidate binary to the Lintasan production path.
#
#   Usage: scripts/deploy.sh [path-to-candidate]
#   Default candidate: <repo>/dist-bin/lintasan  (the `make build` output)
#
# Why this exists instead of an ad-hoc script: the previous deploy script
# hardcoded NEW_BIN to a worktree's dist-bin. After `feat/qoder-prod` was merged
# into main, running it re-deployed the WORKTREE's binary — a redundant restart
# serving the same old version, reported as success. This script takes the
# candidate as an argument and refuses to report success unless the version it
# read from the candidate is the version /health reports after the restart.
#
# AGENTS.md §3: only this path — never `make build` — may write ./lintasan.
# `make build` lands in dist-bin/ and cannot stage code into production.
set -euo pipefail

PROD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NEW_BIN="${1:-$PROD_DIR/dist-bin/lintasan}"
HEALTH_URL="http://localhost:20180/health"
PUBLIC_URL="https://lintasan.sans.biz.id"
TS=$(date +%Y%m%d-%H%M%S)

echo "=== 0. candidate pre-flight ==="
if [ ! -x "$NEW_BIN" ]; then
  echo "ABORT: candidate not executable: $NEW_BIN" >&2
  exit 1
fi

# NOTE: do NOT use `strings "$BIN" | grep -q <pat>` with pipefail — grep -q exits
# on first match, strings takes SIGPIPE (141), and pipefail turns that into a
# false "not found". Count instead; grep -c consumes the whole stream.
CAND_VER=$(strings "$NEW_BIN" | grep -oE 'Version=v[0-9][0-9.]*(-[0-9]+-g[0-9a-f]+)?' | head -1 | cut -d= -f2)
if [ -z "$CAND_VER" ]; then
  echo "ABORT: could not read a version string from $NEW_BIN" >&2
  exit 1
fi
EMBED_HITS=$(strings "$NEW_BIN" | grep -c 'dist/_app/immutable' || true)
if [ "${EMBED_HITS:-0}" -lt 1 ]; then
  echo "ABORT: candidate embeds no dashboard assets (built without the frontend?)" >&2
  exit 1
fi

echo "  candidate : $NEW_BIN"
echo "  version   : $CAND_VER"
echo "  md5       : $(md5sum "$NEW_BIN" | cut -d' ' -f1)"
echo "  embed hits: $EMBED_HITS asset paths"
echo "  running   : $(curl -s -m 10 "$HEALTH_URL" 2>/dev/null | grep -oE '"version":"[^"]*"' || echo '(unreachable)')"

echo
echo "=== 1. backup + swap + restart ==="
cp -p "$PROD_DIR/lintasan" "$PROD_DIR/lintasan.bak-$TS"
echo "  backup    : $PROD_DIR/lintasan.bak-$TS"
sudo systemctl stop lintasan
cp "$NEW_BIN" "$PROD_DIR/lintasan"
sudo systemctl start lintasan

echo
echo "=== 2. wait for health ==="
for _ in $(seq 1 30); do
  sleep 1
  if curl -s -m 2 -o /dev/null "$HEALTH_URL"; then break; fi
done

echo
echo "=== 3. running image must equal the candidate ==="
LIVE_VER=$(curl -s -m 10 "$HEALTH_URL" | grep -oE '"version":"[^"]*"' | cut -d'"' -f4)
PID=$(systemctl show lintasan -p MainPID --value)
RUN_MD5=$(md5sum "/proc/$PID/exe" 2>/dev/null | cut -d' ' -f1)
DISK_MD5=$(md5sum "$PROD_DIR/lintasan" | cut -d' ' -f1)
CAND_MD5=$(md5sum "$NEW_BIN" | cut -d' ' -f1)

echo "  active     : $(systemctl is-active lintasan)   NRestarts: $(systemctl show lintasan -p NRestarts --value)"
echo "  /health    : $LIVE_VER"
echo "  candidate  : $CAND_VER"
echo "  running md5: $RUN_MD5"
echo "  on-disk md5: $DISK_MD5"
echo "  cand md5   : $CAND_MD5"

FAIL=0
[ "$LIVE_VER" = "$CAND_VER" ] || { echo "  ✗ VERSION MISMATCH — deployed the wrong binary"; FAIL=1; }
[ "$RUN_MD5" = "$DISK_MD5" ]  || { echo "  ✗ running inode != on-disk (stale process)"; FAIL=1; }
[ "$DISK_MD5" = "$CAND_MD5" ] || { echo "  ✗ on-disk != candidate"; FAIL=1; }
[ "$FAIL" = "0" ] && echo "  ✓ candidate == installed == running, version $LIVE_VER"

echo
echo "=== 4. auth boundary (public domain — nginx in the path) ==="
V1=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$PUBLIC_URL/v1/models")
API=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$PUBLIC_URL/api/stats")
echo "  /v1/models : $V1  (expect 401)"
echo "  /api/stats : $API  (expect 401)"
[ "$V1" = "401" ] && [ "$API" = "401" ] || { echo "  ✗ auth boundary not fail-closed"; FAIL=1; }

echo
if [ "$FAIL" = "0" ]; then
  echo "✓ deploy verified: $CAND_VER"
else
  echo "✗ deploy FAILED verification — rollback:"
  echo "    sudo systemctl stop lintasan && cp $PROD_DIR/lintasan.bak-$TS $PROD_DIR/lintasan && sudo systemctl start lintasan"
fi
exit "$FAIL"

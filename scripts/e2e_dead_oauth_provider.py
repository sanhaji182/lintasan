#!/usr/bin/env python3
"""Isolated end-to-end proof: a tokenless OAuth connection must NOT black-hole a
model that a healthy provider also serves.

Runs against a COPY of the production DB (python sqlite3.backup, not cp) on a
separate port, with one extra httptest-style stub upstream standing in for the
"healthy alternate provider". Production is never contacted for writes and the
prod process is never touched.

Assertions:
  1. BASELINE (pre-fix binary): POST /v1/chat/completions for the shared model
     returns 401 oauth_reauth_required even though a healthy provider serves it.
  2. FIXED binary: the same call returns 200 with content from the healthy
     provider.

Build the baseline once (a binary WITHOUT the fix):

    git -C ~/lintasan-go archive <pre-fix-sha> | tar -x -C /tmp --one-top-level=ag-baseline-src
    cp -r ~/lintasan-go/frontend/build /tmp/ag-baseline-src/frontend/build   # go:embed needs it
    cd /tmp/ag-baseline-src && CGO_ENABLED=1 go build \\
        -ldflags="-s -w -X github.com/sanhaji182/lintasan-go/internal/version.Version=baseline" \\
        -o /tmp/ag-baseline-lintasan ./cmd/lintasan

Override paths with E2E_BASE_BIN / E2E_FIXED_BIN.
"""
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WORK = "/tmp/ag-e2e"
PROD_DB = os.path.expanduser("~/lintasan-go/data/lintasan.db")

FIXED_BIN = os.environ.get("E2E_FIXED_BIN", os.path.join(ROOT, "dist-bin", "lintasan"))
# Baseline must be a binary WITHOUT the fix (see BUILD_BASELINE below).
BASE_BIN = os.environ.get("E2E_BASE_BIN", "/tmp/ag-baseline-lintasan")

STUB_PORT = 25099
SHARED_MODEL = "gemini-2.5-flash"


def log(msg):
    print(msg, flush=True)


def snapshot_db(dest):
    src = sqlite3.connect(f"file:{PROD_DB}?mode=ro", uri=True)
    out = sqlite3.connect(dest)
    with out:
        src.backup(out)
    out.close()
    src.close()
    os.chmod(dest, 0o600)


def b64url(raw):
    import base64
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def mint_jwt(secret):
    import hashlib
    import hmac
    header = b64url(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    now = int(time.time())
    payload = b64url(json.dumps({
        "sub": "e2e-admin", "username": "e2e-admin", "role": "admin",
        "iat": now, "exp": now + 3600,
    }, separators=(",", ":")).encode())
    signing = f"{header}.{payload}"
    sig = b64url(hmac.new(secret.encode(), signing.encode(), hashlib.sha256).digest())
    return f"{signing}.{sig}"


def prepare_data_dir(port_key):
    """Fresh copy of prod data + the healthy alternate provider row."""
    data_dir = os.path.join(WORK, f"data-{port_key}")
    shutil.rmtree(data_dir, ignore_errors=True)
    os.makedirs(data_dir, exist_ok=True)
    db_path = os.path.join(data_dir, "lintasan.db")
    snapshot_db(db_path)

    conn = sqlite3.connect(db_path)
    cur = conn.cursor()
    stub_base = f"http://127.0.0.1:{STUB_PORT}"
    cur.execute("""
        INSERT OR REPLACE INTO connections
          (id, name, base_url, api_key, format, chat_path, models_path,
           auth_header, auth_prefix, is_active, priority, oauth_provider)
        VALUES ('e2e-healthy', 'e2e-healthy', ?, 'sk-e2e', 'openai',
                '/v1/chat/completions', '/v1/models', 'Authorization', 'Bearer ', 1, 1, '')
    """, (stub_base,))
    # It serves the same model the dead OAuth row advertises.
    cur.execute("""
        INSERT OR REPLACE INTO discovered_models (id, connection_id, model_id, model_name, is_active)
        VALUES ('e2e-dm', 'e2e-healthy', ?, ?, 1)
    """, (SHARED_MODEL, SHARED_MODEL))
    conn.commit()

    state = {
        "dead_conn": cur.execute(
            "SELECT id, is_active FROM connections WHERE oauth_provider='antigravity' LIMIT 1"
        ).fetchone(),
        "sessions": cur.execute(
            "SELECT COUNT(*) FROM oauth_sessions WHERE provider='antigravity'"
        ).fetchone()[0],
        "master_key": cur.execute(
            "SELECT value FROM settings WHERE key='master_key'"
        ).fetchone()[0],
        "jwt_secret": cur.execute(
            "SELECT value FROM settings WHERE key='jwt_secret'"
        ).fetchone()[0],
    }
    conn.close()
    return data_dir, db_path, state


def start_stub():
    code = f'''
import json, http.server, socketserver

class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        self.rfile.read(n)
        body = json.dumps({{
            "id": "chatcmpl-e2e", "object": "chat.completion", "model": "{SHARED_MODEL}",
            "choices": [{{"index": 0, "message": {{"role": "assistant", "content": "SERVED-BY-HEALTHY"}},
                        "finish_reason": "stop"}}],
            "usage": {{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}},
        }}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a):
        pass

socketserver.TCPServer.allow_reuse_address = True
with socketserver.TCPServer(("127.0.0.1", {STUB_PORT}), H) as s:
    s.serve_forever()
'''
    p = subprocess.Popen([sys.executable, "-c", code])
    time.sleep(1.0)
    return p


def wait_health(port, timeout=60):
    end = time.time() + timeout
    while time.time() < end:
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{port}/health", timeout=2) as r:
                if r.status == 200:
                    return json.loads(r.read().decode())
        except Exception:
            time.sleep(0.5)
    return None


def run_instance(binary, tag, port):
    data_dir, db_path, state = prepare_data_dir(tag)
    env = dict(os.environ)
    env.update({
        "PORT": str(port),
        "LINTASAN_DATA_DIR": data_dir,
        "LINTASAN_OAUTH_IDE_ENABLED": "true",
        # Deliberately unset: no OAuth client -> no token can ever be minted,
        # which is precisely the production state for antigravity.
        "LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID": "",
        "LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET": "",
    })
    logf = open(os.path.join(WORK, f"instance-{tag}.log"), "wb")
    proc = subprocess.Popen([binary, "start"], cwd=data_dir, env=env,
                            stdout=logf, stderr=subprocess.STDOUT)
    health = wait_health(port)
    if health is None:
        proc.kill()
        logf.close()
        raise RuntimeError(f"{tag}: instance never became healthy (see {WORK}/instance-{tag}.log)")
    return proc, logf, db_path, state, health


def probe(port, token, model):
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": "e2e-probe" + str(time.time())}],
        "max_tokens": 8,
    }).encode()
    req = urllib.request.Request(
        f"http://127.0.0.1:{port}/v1/chat/completions",
        data=body, method="POST",
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {token}"},
    )
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return r.status, r.read().decode(), time.time() - t0
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode(), time.time() - t0


def stop(proc, logf):
    proc.terminate()
    try:
        proc.wait(timeout=15)
    except subprocess.TimeoutExpired:
        proc.kill()
    logf.close()


def main():
    shutil.rmtree(WORK, ignore_errors=True)
    os.makedirs(WORK, exist_ok=True)

    log("=== production DB state being reproduced (read-only snapshot)")
    conn = sqlite3.connect(f"file:{PROD_DB}?mode=ro", uri=True)
    dead = conn.execute(
        "SELECT id, name, is_active, COALESCE(last_error_code,'') FROM connections "
        "WHERE oauth_provider='antigravity'"
    ).fetchone()
    sessions = conn.execute(
        "SELECT COUNT(*) FROM oauth_sessions WHERE provider='antigravity'"
    ).fetchone()[0]
    shared = conn.execute(
        "SELECT COUNT(*) FROM discovered_models WHERE model_id=?", (SHARED_MODEL,)
    ).fetchone()[0]
    conn.close()
    log(f"  dead connection : {dead}")
    log(f"  antigravity sessions: {sessions}")
    log(f"  providers exposing {SHARED_MODEL}: {shared}")

    stub = start_stub()
    results = {}
    try:
        for tag, binary, port in (("baseline", BASE_BIN, 25181), ("fixed", FIXED_BIN, 25182)):
            if not os.path.exists(binary):
                raise RuntimeError(f"missing binary: {binary}")
            log(f"\n=== {tag}: {binary}")
            proc, logf, db_path, state, health = run_instance(binary, tag, port)
            try:
                log(f"  version         : {health.get('version')}")
                log(f"  seed: dead_conn={state['dead_conn']} antigravity_sessions={state['sessions']}")
                # /v1/* is gated by the master key, NOT a dashboard JWT
                # (a JWT there answers 401 "Invalid API key").
                status, body, elapsed = probe(port, state["master_key"], SHARED_MODEL)
                log(f"  POST /v1/chat/completions model={SHARED_MODEL}")
                log(f"    HTTP {status}  ({elapsed:.2f}s)")
                log(f"    body: {body[:260]}")
                results[tag] = {"status": status, "body": body, "elapsed": round(elapsed, 2),
                                "version": health.get("version")}

                # Catalogue check on the fixed build.
                req = urllib.request.Request(
                    f"http://127.0.0.1:{port}/v1/models",
                    headers={"Authorization": f"Bearer {state['master_key']}"},
                )
                try:
                    with urllib.request.urlopen(req, timeout=20) as r:
                        cat = json.loads(r.read().decode())
                    ids = {m["id"] for m in cat.get("data", [])}
                    ag_only = ids & {"claude-opus-4-6-thinking", "gemini-2.5-flash-thinking",
                                     "gemini-3.5-flash-lite", "gpt-oss-120b-medium"}
                    results[tag]["catalog_model_count"] = len(ids)
                    results[tag]["dead_only_models_listed"] = sorted(ag_only)
                    results[tag]["shared_model_listed"] = SHARED_MODEL in ids
                    log(f"  /v1/models: {len(ids)} models; "
                        f"antigravity-exclusive still listed = {sorted(ag_only) or 'none'}; "
                        f"{SHARED_MODEL} listed = {SHARED_MODEL in ids}")
                except Exception as e:
                    log(f"  /v1/models read failed: {e}")
            finally:
                stop(proc, logf)
    finally:
        stub.terminate()
        try:
            stub.wait(timeout=5)
        except subprocess.TimeoutExpired:
            stub.kill()

    log("\n=== VERDICT")
    b, f = results.get("baseline", {}), results.get("fixed", {})
    log(f"  baseline: HTTP {b.get('status')}  catalog={b.get('catalog_model_count')} "
        f"dead-only listed={b.get('dead_only_models_listed')}")
    log(f"  fixed   : HTTP {f.get('status')}  catalog={f.get('catalog_model_count')} "
        f"dead-only listed={f.get('dead_only_models_listed')}")
    ok = (b.get("status") == 401 and f.get("status") == 200
          and not f.get("dead_only_models_listed")
          and "SERVED-BY-HEALTHY" in f.get("body", ""))
    log(f"  failover 401->200 with healthy content : "
        f"{'PASS' if b.get('status') == 401 and f.get('status') == 200 else 'FAIL'}")
    log(f"  dead-only models hidden in fixed build : "
        f"{'PASS' if not f.get('dead_only_models_listed') else 'FAIL'}")
    log(f"  overall: {'PASS' if ok else 'FAIL'}")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())

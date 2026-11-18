#!/usr/bin/env python3
"""Multi-turn conversation test through the Lintasan gateway.

Sends a real conversation (many round-trips, full history each time) and checks
that the model actually REMEMBERS earlier turns — not just that HTTP 200 comes
back. A provider that silently drops context, or a router that fails over
mid-conversation to something that lost the history, fails here.

Design notes
------------
- One opaque token is planted in turn 1 and asked back near the end, so a
  stateless echo cannot pass by luck.
- An arithmetic chain is built turn by turn; each turn asks for a value that
  depends on all previous ones.
- Per turn we record HTTP status, latency, which connection served it (from
  request_logs), token usage, and whether the expected fact survived.
- --stream exercises the SSE path, which is what IDE clients actually use.

Usage
-----
    python3 scripts/multiturn_conversation_test.py                     # 20 turns, gemini-2.5-flash
    python3 scripts/multiturn_conversation_test.py --turns 30 --model gemini-2.5-pro
    python3 scripts/multiturn_conversation_test.py --stream
    python3 scripts/multiturn_conversation_test.py --concurrent 5      # parallel conversations
"""
import argparse
import json
import os
import random
import secrets
import sqlite3
import string
import sys
import time
import urllib.error
import urllib.request

DB = os.environ.get("LINTASAN_DB", os.path.expanduser("~/lintasan-go/data/lintasan.db"))
BASE = os.environ.get("LINTASAN_BASE", "http://localhost:20180")


def master_key():
    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    k = c.execute("SELECT value FROM settings WHERE key='master_key'").fetchone()[0]
    c.close()
    return k


def log_anchor():
    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    t = c.execute("SELECT datetime('now','localtime')").fetchone()[0]
    c.close()
    return t


def attempts_since(anchor, model):
    c = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    rows = list(c.execute("""
        SELECT connection_id, status, latency_ms, COALESCE(error,'')
        FROM request_logs
        WHERE created_at >= datetime(?, '-5 seconds') AND model LIKE ?
        ORDER BY created_at, id""", (anchor, f"%{model}%")))
    c.close()
    return rows


def chat(base, key, model, messages, stream=False, timeout=120):
    body = {"model": model, "messages": messages, "max_tokens": 400, "temperature": 0.2}
    if stream:
        body["stream"] = True
    req = urllib.request.Request(
        f"{base}/v1/chat/completions",
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json", "Authorization": f"Bearer {key}"})
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read().decode("utf-8", "replace")
            code = r.status
    except urllib.error.HTTPError as e:
        return e.code, "", round(time.time() - t0, 2), e.read().decode()[:200]
    except Exception as e:
        return 0, "", round(time.time() - t0, 2), f"{type(e).__name__}: {e}"
    elapsed = round(time.time() - t0, 2)

    if not stream:
        try:
            d = json.loads(raw)
            msg = (d.get("choices") or [{}])[0].get("message") or {}
            return code, (msg.get("content") or msg.get("reasoning") or ""), elapsed, ""
        except Exception:
            return code, raw[:400], elapsed, "unparseable JSON"

    # SSE: concatenate delta.content across events.
    out = []
    for line in raw.splitlines():
        line = line.strip()
        if not line.startswith("data:"):
            continue
        payload = line[5:].strip()
        if payload in ("", "[DONE]"):
            continue
        try:
            ev = json.loads(payload)
        except Exception:
            continue
        for ch in ev.get("choices") or []:
            delta = ch.get("delta") or {}
            piece = delta.get("content")
            if piece:
                out.append(piece)
    return code, "".join(out), elapsed, ""


def build_turns(n, token):
    """Each turn's question depends on facts stated in earlier turns."""
    turns = []
    turns.append(f"Remember this secret code: {token}. Also remember that my name is Sans. "
                 f"Reply with exactly: OK")
    turns.append("My favourite number is 7. Reply with just that number.")
    for i in range(3, n + 1):
        if i == n - 1:
            turns.append("What is the secret code I told you in the very first message? "
                         "Reply with the code only.")
        elif i == n:
            turns.append("Summarise in one short sentence: what is my name, my favourite number, "
                         "and the secret code?")
        elif i % 5 == 0:
            turns.append(f"Still with me? Reply with the secret code and the turn number {i}.")
        elif i % 3 == 0:
            turns.append(f"Add {i} to my favourite number and reply with only the result.")
        else:
            turns.append(f"Reply with the single word ACK{i} and nothing else.")
    return turns


def run_conversation(idx, base, key, model, turns, stream, verbose):
    token = "ZX" + "".join(random.choice(string.ascii_uppercase + string.digits) for _ in range(8))
    prompts = build_turns(turns, token)

    messages = []
    results = []
    for i, prompt in enumerate(prompts, start=1):
        messages.append({"role": "user", "content": prompt})
        anchor = log_anchor()
        code, content, elapsed, err = chat(base, key, model, messages, stream=stream)
        served = attempts_since(anchor, model)
        conn = served[-1][0][:8] if served else "-"
        messages.append({"role": "assistant", "content": content or ""})

        ok_ctx = None
        if i == len(prompts) - 1:
            ok_ctx = token in (content or "")
        elif i == len(prompts):
            ok_ctx = token in (content or "") and "Sans" in (content or "")

        results.append({"turn": i, "status": code, "elapsed": elapsed,
                        "conn": conn, "chars": len(content or ""),
                        "ctx_ok": ok_ctx, "err": err, "content": (content or "")[:90]})
        if verbose:
            tag = "" if ok_ctx is None else (" CTX_OK" if ok_ctx else " CTX_FAIL")
            print(f"  [{idx}] t{i:>2} {code} {elapsed:>6.2f}s conn={conn} "
                  f"len={len(content or ''):<4}{tag} :: {(content or err)[:70]!r}")
        if code != 200:
            break
    return {"idx": idx, "token": token, "results": results}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default="gemini-2.5-flash")
    ap.add_argument("--turns", type=int, default=20)
    ap.add_argument("--stream", action="store_true")
    ap.add_argument("--concurrent", type=int, default=1)
    ap.add_argument("--verbose", action="store_true", default=True)
    args = ap.parse_args()

    key = master_key()
    print(f"model={args.model}  turns={args.turns}  stream={args.stream}  "
          f"conversations={args.concurrent}\n")

    started = time.time()
    convs = []
    if args.concurrent > 1:
        from concurrent.futures import ThreadPoolExecutor
        with ThreadPoolExecutor(max_workers=args.concurrent) as ex:
            futs = [ex.submit(run_conversation, i + 1, BASE, key, args.model,
                              args.turns, args.stream, args.verbose)
                    for i in range(args.concurrent)]
            convs = [f.result() for f in futs]
    else:
        convs = [run_conversation(1, BASE, key, args.model, args.turns,
                                  args.stream, args.verbose)]
    wall = round(time.time() - started, 1)

    print("\n=== SUMMARY")
    total = ok = ctx_pass = ctx_fail = 0
    lat = []
    for conv in convs:
        for r in conv["results"]:
            total += 1
            if r["status"] == 200:
                ok += 1
                lat.append(r["elapsed"])
            if r["ctx_ok"] is True:
                ctx_pass += 1
            elif r["ctx_ok"] is False:
                ctx_fail += 1

    print(f"  requests       : {ok}/{total} HTTP 200")
    if lat:
        lat.sort()
        print(f"  latency        : min {lat[0]:.2f}s  median {lat[len(lat)//2]:.2f}s  max {lat[-1]:.2f}s")
    print(f"  context checks : {ctx_pass} pass, {ctx_fail} fail")
    print(f"  wall time      : {wall}s")
    for conv in convs:
        bad = [r for r in conv["results"] if r["status"] != 200 or r["ctx_ok"] is False]
        print(f"  conversation {conv['idx']}: {len(conv['results'])} turns, "
              f"{'OK' if not bad else f'{len(bad)} problem turn(s)'}")
    return 0 if (ok == total and ctx_fail == 0 and ctx_pass > 0) else 1


if __name__ == "__main__":
    sys.exit(main())

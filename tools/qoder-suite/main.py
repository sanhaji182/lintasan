"""
Qoder Suite - Unified CLI
Features:
1. create      : Buat akun otomatis (tempmail + captcha + OTP + PAT + auto-claim)
2. claim       : Verifikasi PAT & klaim bonus kuota via COSY engine
3. login-9r    : Batch login OAuth ke 9router dashboard
4. pipeline    : Buat akun baru -> klaim kuota -> langsung inject login ke 9router
"""

import asyncio
import os
import sys
import argparse
from typing import Optional
from rich import box
from rich.console import Console
from rich.panel import Panel
from rich.prompt import IntPrompt, Confirm
from rich.table import Table
from rich.text import Text

from src.config import (
    NINE_ROUTER_URL, HEADLESS, OUTPUT_TXT, INPUT_LOGIN_TXT, DATA_DIR
)
from src.creator import CreatorManager
from src.claim import CosyEngine
from src.login import run_login_batch, load_accounts
from src.utils import setup_logging, write_log

console = Console()

def banner():
    body = Text.assemble(
        Text("QODER UNIFIED SUITE", style="bold cyan"), "\n",
        Text("Account Creator + Auto-Claim + 9router OAuth Ingestion", style="dim"), "\n\n",
        Text(f"9router Endpoint: {NINE_ROUTER_URL}", style="bright_black"),
    )
    console.print(Panel(body, box=box.HEAVY, border_style="cyan"))

async def cmd_create(count: int, headless: bool, auto_claim: bool):
    manager = CreatorManager(headless=headless, console=console)
    console.print(f"[bold cyan]Membuat {count} akun Qoder...[/]")
    for i in range(count):
        await manager.create_account(idx=i+1, auto_claim_bonus=auto_claim)
        if i < count - 1:
            await asyncio.sleep(3)

async def cmd_claim(pat: str):
    engine = CosyEngine()
    console.print(f"[cyan]Memeriksa & mengklaim kuota untuk PAT:[/] {pat[:15]}...")
    res = engine.auto_claim(pat)
    import json
    console.print("\n[bold green]Hasil Claim & Quota Engine:[/]")
    console.print(json.dumps(res, indent=2))

async def cmd_login(input_path: str, batch_size: int, headless: bool):
    from pathlib import Path
    p = Path(input_path) if input_path else OUTPUT_TXT
    if not p.exists() and p != INPUT_LOGIN_TXT and INPUT_LOGIN_TXT.exists():
        p = INPUT_LOGIN_TXT

    accounts = load_accounts(p)
    if not accounts:
        console.print(f"[yellow]Tidak ada akun yang ditemukan di {p}[/]")
        return

    console.print(f"[green]Ditemukan {len(accounts)} akun di {p.name}[/]")
    ok, fail = await run_login_batch(
        accounts=accounts,
        batch_size=batch_size,
        headless=headless,
        console=console,
        input_file=p
    )
    console.print(f"\n[bold]Selesai: [green]{ok} sukses[/], [red]{fail} gagal[/][/]")

import os

def _router_db_path() -> str:
    """Path DB SQLite 9router (bisa dioverride lewat env NINEROUTER_DB)."""
    return os.getenv("NINEROUTER_DB", os.path.expanduser("~/.9router/db/data.sqlite"))

def _router_login(base_router: str):
    """Login ke 9router, return cookie auth_token atau None."""
    import urllib.request, json
    from src.config import NINE_ROUTER_PASS
    try:
        req = urllib.request.Request(
            f"{base_router}/api/auth/login",
            data=json.dumps({"password": NINE_ROUTER_PASS}).encode("utf-8"),
            headers={"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"}
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            cookie = resp.headers.get("Set-Cookie")
            return [c for c in cookie.split(";") if "auth_token=" in c][0].strip()
    except Exception as e:
        console.print(f"[yellow]Login 9router gagal: {e}[/]")
        return None

def _router_add_and_test(base_router: str, auth_token: str, email: str, pat: str):
    """Tambah koneksi qoder ke 9router + test. Return (ok, conn_id)."""
    import urllib.request, json
    headers = {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"}
    if auth_token:
        headers["Cookie"] = auth_token
    add_req = urllib.request.Request(
        f"{base_router}/api/providers",
        data=json.dumps({"provider": "qoder", "name": email, "apiKey": pat, "priority": 1}).encode("utf-8"),
        headers=headers
    )
    with urllib.request.urlopen(add_req, timeout=20) as resp:
        res_data = json.loads(resp.read().decode())
    conn_id = res_data.get("connection", {}).get("id")
    valid = None
    if conn_id:
        test_req = urllib.request.Request(
            f"{base_router}/api/providers/{conn_id}/test",
            data=b"{}",
            headers=headers
        )
        with urllib.request.urlopen(test_req, timeout=20) as t_resp:
            valid = json.loads(t_resp.read().decode()).get("valid")
    return conn_id, valid

async def cmd_sync(base_router: str = "http://127.0.0.1:20127"):
    """Scan accounts.txt dan injeksi ke 9router semua akun yang belum terdaftar."""
    import sqlite3, json, urllib.request
    from pathlib import Path

    db_path = _router_db_path()
    existing = set()
    try:
        conn = sqlite3.connect(db_path)
        existing = set(r[0] for r in conn.execute(
            "SELECT name FROM providerConnections WHERE provider=\"qoder\"").fetchall())
        conn.close()
    except Exception as e:
        console.print(f"[yellow]Tidak bisa baca DB 9router: {e}[/]")

    accounts = load_accounts(OUTPUT_TXT)
    pending = [a for a in accounts if a["email"] not in existing and a.get("pat")]
    console.print(f"[cyan]Akun lokal:[/] {len(accounts)} | [cyan]Sudah di 9router:[/] {len(existing)} | [yellow]Perlu sync:[/] {len(pending)}")

    if not pending:
        console.print("[green]Semua akun sudah tersinkron ke 9router.[/]")
        return

    auth_token = _router_login(base_router)
    ok = 0
    fail = 0
    for a in pending:
        try:
            cid, valid = _router_add_and_test(base_router, auth_token, a["email"], a["pat"])
            if valid:
                ok += 1
                console.print(f"  [green]OK[/] {a['email']} (ID: {cid})")
            else:
                fail += 1
                console.print(f"  [yellow]WARN[/] {a['email']} ditambahkan tapi test valid={valid}")
        except Exception as e:
            fail += 1
            console.print(f"  [red]FAIL[/] {a['email']}: {e}")
    console.print(f"\n[bold]Sync selesai: [green]{ok} OK[/], [red]{fail} gagal[/][/]")

async def cmd_sync_lintasan(url: Optional[str] = None, api_key: Optional[str] = None, db_path: Optional[str] = None):
    """Scan accounts.txt dan injeksi ke Lintasan gateway (via API /api/qoder/credentials atau SQLite DB)."""
    from src.config import LINTASAN_URL, LINTASAN_KEY, LINTASAN_DB
    from src.login import load_accounts
    import urllib.request, json, sqlite3, os, uuid

    target_url = url or LINTASAN_URL
    db_file = db_path or LINTASAN_DB

    accounts = load_accounts(OUTPUT_TXT)
    valid_pats = [a["pat"] for a in accounts if a.get("pat") and a["pat"].startswith("pt-")]

    console.print(f"[cyan]Akun lokal di accounts.txt:[/] {len(accounts)} | [cyan]PAT valid:[/] {len(valid_pats)}")
    if not valid_pats:
        console.print("[yellow]Tidak ada PAT yang valid untuk di-sync.[/]")
        return

    # 1. Coba via Lintasan API
    try:
        endpoint = f"{target_url.rstrip('/')}/api/qoder/credentials"
        payload = json.dumps({
            "pats": "\n".join(valid_pats),
            "validate": True
        }).encode("utf-8")
        headers = {"Content-Type": "application/json", "User-Agent": "qoder-suite/1.0"}
        if api_key or LINTASAN_KEY:
            headers["Authorization"] = f"Bearer {api_key or LINTASAN_KEY}"

        req = urllib.request.Request(endpoint, data=payload, headers=headers, method="POST")
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode())
            console.print("[bold green]Sync ke Lintasan API berhasil:[/]")
            summary = data.get("summary", {})
            console.print(f"  • Ditambahkan : [green]{summary.get('added', 0)}[/]")
            console.print(f"  • Duplikat    : [yellow]{summary.get('duplicates', 0)}[/]")
            console.print(f"  • Invalid     : [red]{summary.get('invalid', 0)}[/]")
            return
    except Exception as e:
        console.print(f"[yellow]Sync via API ({target_url}) gagal: {e}. Mencoba direct SQLite...[/]")

    # 2. Fallback: direct SQLite DB injection
    try:
        if os.path.exists(db_file):
            conn = sqlite3.connect(db_file)
            c = conn.cursor()
            existing = set(r[0] for r in c.execute("SELECT api_key FROM connections WHERE LOWER(format)='qoder'").fetchall())
            added = 0
            for a in accounts:
                pat = a.get("pat", "")
                if pat and pat.startswith("pt-") and pat not in existing:
                    cid = str(uuid.uuid4())
                    name = f"Qoder {a['email']}"
                    c.execute("""
                        INSERT INTO connections (id, name, base_url, format, api_key, is_active, priority)
                        VALUES (?, ?, 'https://api.qoder.com/v1', 'qoder', ?, 1, 60)
                    """, (cid, name, pat))
                    added += 1
                    existing.add(pat)
            conn.commit()
            conn.close()
            console.print(f"[bold green]Sync via DB {db_file} selesai: [green]{added} akun ditambahkan.[/]")
        else:
            console.print(f"[red]DB Lintasan tidak ditemukan di {db_file}[/]")
    except Exception as e:
        console.print(f"[red]Sync via DB gagal: {e}[/]")

async def cmd_report():
    """Tampilkan ringkasan status akun lokal vs 9router."""
    import sqlite3, json
    from rich.table import Table
    from rich import box

    db_path = _router_db_path()
    conns = {}
    try:
        conn = sqlite3.connect(db_path)
        for r in conn.execute(
            "SELECT name, data FROM providerConnections WHERE provider=\"qoder\"").fetchall():
            try:
                d = json.loads(r[1])
            except Exception:
                d = {}
            conns[r[0]] = d.get("testStatus", "unknown")
        conn.close()
    except Exception as e:
        console.print(f"[yellow]Tidak bisa baca DB 9router: {e}[/]")

    accounts = load_accounts(OUTPUT_TXT)
    active = sum(1 for s in conns.values() if s == "active")
    unavailable = sum(1 for s in conns.values() if s != "active")

    t = Table(box=box.ROUNDED, title="Qoder Suite Report")
    t.add_column("Metrik", style="bold cyan")
    t.add_column("Nilai", style="white")
    t.add_row("Akun lokal (accounts.txt)", str(len(accounts)))
    t.add_row("Koneksi Qoder di 9router", str(len(conns)))
    t.add_row("  • active", f"[green]{active}[/]")
    t.add_row("  • non-active", f"[yellow]{unavailable}[/]")
    t.add_row("Akun belum ter-inject", str(len([a for a in accounts if a["email"] not in conns])))
    t.add_row("Akun tanpa PAT", str(len([a for a in accounts if not a.get("pat")])))
    console.print(t)

def cmd_report_sync():
    asyncio.run(cmd_report())

async def cmd_usage(base_router: str = "http://127.0.0.1:20127", limit: int = None):
    """Tampilkan kuota tiap koneksi Qoder via API 9router (/api/usage/{id})."""
    import sqlite3, json, urllib.request
    from rich.table import Table
    from rich import box

    db_path = _router_db_path()
    conns = []
    try:
        conn = sqlite3.connect(db_path)
        for r in conn.execute(
            "SELECT id, name, data FROM providerConnections WHERE provider=\"qoder\" ORDER BY createdAt").fetchall():
            try:
                d = json.loads(r[2])
            except Exception:
                d = {}
            conns.append((r[0], r[1], d.get("testStatus", "unknown")))
        conn.close()
    except Exception as e:
        console.print(f"[red]Tidak bisa baca DB 9router: {e}[/]")
        return

    if limit:
        conns = conns[-limit:]

    auth_token = _router_login(base_router)
    headers = {"User-Agent": "Mozilla/5.0"}
    if auth_token:
        headers["Cookie"] = auth_token

    t = Table(box=box.ROUNDED, title="Qoder Quota / Usage")
    t.add_column("Email", style="cyan", overflow="fold")
    t.add_column("Status", style="white")
    t.add_column("Kuota", style="white")
    t.add_column("Terpakai", style="white")
    t.add_column("Sisa", style="green")
    t.add_column("Reset", style="dim")

    total_remaining = 0.0
    for cid, name, status in conns:
        try:
            req = urllib.request.Request(f"{base_router}/api/usage/{cid}", headers=headers)
            with urllib.request.urlopen(req, timeout=20) as resp:
                u = json.loads(resp.read().decode())
            q = (u.get("quotas") or {}).get("user") or {}
            total = q.get("total", 0) or 0
            used = q.get("used", 0) or 0
            rem = q.get("remaining", 0) or 0
            unit = q.get("unit", "")
            reset = (q.get("resetAt") or "")[:10]
            total_remaining += rem
            st = f"[green]{status}[/]" if status == "active" else f"[yellow]{status}[/]"
            t.add_row(name, st, f"{total:g} {unit}", f"{used:g}", f"{rem:g}", reset)
        except Exception as e:
            t.add_row(name, f"[yellow]{status}[/]", "[red]err[/]", "-", "-", str(e)[:20])
    console.print(t)
    console.print(f"[bold]Total sisa kuota: [green]{total_remaining:g}[/] credits[/]")

def cmd_usage_sync(base_router="http://127.0.0.1:20127", limit=None):
    asyncio.run(cmd_usage(base_router, limit))

async def cmd_doctor():
    """Preflight standalone: cek temp-mail, 9router, login, dan DB."""
    console.print("[bold cyan]=== Doctor: Preflight Check ===[/]")
    ok = _preflight()
    if ok:
        console.print("[bold green]Semua sistem siap. ✓[/]")
    else:
        console.print("[bold red]Ada masalah pada infrastruktur. ✗[/]")
    return ok

def cmd_doctor_sync():
    ok = asyncio.run(cmd_doctor())
    import sys
    sys.exit(0 if ok else 1)

def cmd_progress_sync():
    """Tampilkan status batch terakhir dari state resume."""
    from src import progress as _prog
    st = _prog.load_state()
    if not st:
        console.print(f"[yellow]Tidak ada state batch di {_prog.PROGRESS_FILE}[/]")
        return
    from rich.table import Table
    from rich import box
    t = Table(box=box.ROUNDED, title="Batch Progress")
    t.add_column("Field", style="bold cyan")
    t.add_column("Nilai", style="white")
    for k in ("run_id", "status", "target", "done", "failed", "started_at", "updated_at"):
        t.add_row(k, str(st.get(k, "-")))
    console.print(t)
    total = (st.get("done", 0) or 0) + (st.get("failed", 0) or 0)
    console.print(f"[dim]Diproses: {total}/{st.get('target','?')} | "
                  f"Resume dengan: ./run.sh pipeline --resume[/]")

async def cmd_verify_renew(renew: bool = False, headless: bool = True):
    """Verifikasi semua PAT; opsional renew yang mati."""
    from src.renew import verify_pat, renew_pat
    from src.login import load_accounts
    from src.utils import append_account_txt
    from rich.table import Table
    from rich import box

    accounts = load_accounts(OUTPUT_TXT)
    if not accounts:
        console.print("[yellow]Tidak ada akun di accounts.txt[/]")
        return

    console.print(f"[cyan]Memverifikasi {len(accounts)} PAT...[/]")
    t = Table(box=box.ROUNDED, title="PAT Verification")
    t.add_column("Email", style="cyan", overflow="fold")
    t.add_column("Status", style="white")
    t.add_column("Detail", style="dim")

    valid_n = invalid_n = renewed_n = 0
    for a in accounts:
        email, pat = a["email"], a.get("pat", "")
        res = verify_pat(pat)
        if res.get("valid"):
            valid_n += 1
            t.add_row(email, "[green]valid[/]", str(res.get("expires_at", ""))[:10])
            continue

        invalid_n += 1
        if renew:
            new_pat = await renew_pat(email, a.get("password", ""), headless=headless, console=console)
            if new_pat:
                renewed_n += 1
                # Update accounts.txt (hapus baris lama, tulis baru) + jsonl
                try:
                    from src.utils import remove_account_txt
                    raw = f"{email}:{a.get('password','')}:{pat}"
                    remove_account_txt(OUTPUT_TXT, raw)
                    append_account_txt(OUTPUT_TXT, email, a.get("password", ""), new_pat)
                except Exception as e:
                    console.print(f"  [yellow]! gagal update file: {e}[/]")
                t.add_row(email, "[green]renewed[/]", res.get("reason", "")[:40])
            else:
                t.add_row(email, "[red]gagal renew[/]", res.get("reason", "")[:40])
        else:
            t.add_row(email, "[red]invalid[/]", res.get("reason", "")[:50])

    console.print(t)
    console.print(f"[bold]Valid: [green]{valid_n}[/] | Invalid: [red]{invalid_n}[/] | Renewed: [green]{renewed_n}[/][/]")

def cmd_verify_renew_sync(renew: bool = False, headless: bool = True):
    asyncio.run(cmd_verify_renew(renew, headless))

def cmd_health_sync(model: str = "qd/qfmodel"):
    """Health check end-to-end: router -> model -> test chat nyata."""
    from src.health import run_health
    from rich.table import Table
    from rich import box

    console.print("[bold cyan]=== End-to-End Health Check ===[/]")
    r = run_health(model=model)
    s = r["steps"]

    t = Table(box=box.ROUNDED, title=f"Health: {model}")
    t.add_column("Tahap", style="bold cyan")
    t.add_column("Status", style="white")
    t.add_column("Detail", style="dim")

    def _mark(d):
        return "[green]✓ OK[/]" if d.get("ok") else "[red]✗ GAGAL[/]"

    t.add_row("9router hidup", _mark(s.get("router", {})), str(s.get("router", {}).get("raw", "")))
    t.add_row("API key ditemukan", _mark(s.get("api_key", {})), "")
    md = s.get("model", {})
    t.add_row("Model terekspos", _mark(md), str(md.get("available", md.get("error", "")))[:60])
    ch = s.get("chat", {})
    detail = ch.get("content") or ch.get("error", "")
    if ch.get("queued"):
        detail = "queued (free tier)"
    t.add_row("Test chat nyata", _mark(ch), f"{detail} ({ch.get('latency_s','?')}s)")
    console.print(t)

    if r["ok"]:
        console.print("[bold green]Pipeline terbukti menghasilkan koneksi yang BERGUNA. ✓[/]")
    else:
        console.print("[bold red]Pipeline TIDAK menghasilkan koneksi yang bisa dipakai. ✗[/]")
    import sys
    sys.exit(0 if r["ok"] else 1)

async def cmd_retest(base_router: str = "http://127.0.0.1:20127", only_failed: bool = True):
    """Test ulang koneksi Qoder di 9router (default: hanya yang non-active)."""
    import sqlite3, json, urllib.request

    db_path = _router_db_path()
    targets = []
    try:
        conn = sqlite3.connect(db_path)
        for r in conn.execute(
            "SELECT id, name, data FROM providerConnections WHERE provider=\"qoder\"").fetchall():
            try:
                d = json.loads(r[2])
            except Exception:
                d = {}
            status = d.get("testStatus", "unknown")
            if not only_failed or status != "active":
                targets.append((r[0], r[1], status))
        conn.close()
    except Exception as e:
        console.print(f"[red]Tidak bisa baca DB 9router: {e}[/]")
        return

    console.print(f"[cyan]Koneksi akan di-retest:[/] {len(targets)}" + (" (hanya non-active)" if only_failed else " (semua)"))
    if not targets:
        console.print("[green]Semua koneksi sudah active.[/]")
        return

    auth_token = _router_login(base_router)
    headers = {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"}
    if auth_token:
        headers["Cookie"] = auth_token

    ok = 0
    fail = 0
    for cid, name, old_status in targets:
        try:
            req = urllib.request.Request(
                f"{base_router}/api/providers/{cid}/test", data=b"{}", headers=headers)
            with urllib.request.urlopen(req, timeout=25) as resp:
                td = json.loads(resp.read().decode())
            if td.get("valid"):
                ok += 1
                console.print(f"  [green]OK[/] {name} ({old_status} → active)")
            else:
                fail += 1
                console.print(f"  [yellow]WARN[/] {name}: {td.get('error')}")
        except Exception as e:
            fail += 1
            console.print(f"  [red]FAIL[/] {name}: {e}")
    console.print(f"\n[bold]Retest selesai: [green]{ok} OK[/], [red]{fail} gagal[/][/]")

def _preflight() -> bool:
    """Cek konektivitas temp-mail & 9router sebelum batch panjang."""
    import urllib.request, json
    from src.config import TEMPIK_BASE
    ok = True

    # Temp-mail
    try:
        req = urllib.request.Request(f"{TEMPIK_BASE}/config",
                                     headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=10) as r:
            data = json.loads(r.read().decode())
        domains = data.get("mailDomains", [])
        console.print(f"[green]✓[/] Temp-mail OK ({len(domains)} domain)")
    except Exception as e:
        console.print(f"[red]✗[/] Temp-mail tidak dapat diakses: {e}")
        ok = False

    # 9router
    try:
        req = urllib.request.Request("http://127.0.0.1:20127/api/health",
                                     headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req, timeout=10) as r:
            h = json.loads(r.read().decode())
        if h.get("ok"):
            console.print("[green]✓[/] 9router OK")
        else:
            console.print(f"[yellow]![/] 9router health: {h}")
    except Exception as e:
        console.print(f"[red]✗[/] 9router tidak dapat diakses: {e}")
        ok = False

    # Login 9router
    if ok:
        if _router_login("http://127.0.0.1:20127"):
            console.print("[green]✓[/] Login 9router OK")
        else:
            console.print("[red]✗[/] Login 9router gagal (cek password di config.toml)")
            ok = False

    return ok

async def cmd_pipeline(count: int, headless: bool, max_retries: int = 3, proxy_file: str = None,
                       target: int = None, concurrency: int = 1, resume: bool = False):
    # Preflight: cek temp-mail & 9router sebelum batch panjang
    if not _preflight():
        console.print("[red]Preflight gagal. Batch dibatalkan untuk menghemat waktu.[/]")
        return

    from src import progress as _prog

    # Mode target: hitung kebutuhan dari total akun yang sudah ada di accounts.txt
    if target:
        from src.login import load_accounts as _load
        existing = _load(OUTPUT_TXT)
        need = max(0, target - len(existing))
        console.print(f"[cyan]Mode target:[/] {len(existing)} akun ada, target {target} → buat {need} akun baru")
        if need == 0:
            console.print("[green]Target sudah tercapai, tidak ada akun baru yang dibuat.[/]")
            console.print("[dim]Menjalankan auto-sync...[/]")
            await cmd_sync("http://127.0.0.1:20127")
            return
        count = need

    # Resume: lanjutkan dari state sebelumnya jika ada
    state = None
    already_done = 0
    if resume:
        state = _prog.load_state()
        if state and state.get("status") == "running":
            already_done = state.get("done", 0) + state.get("failed", 0)
            console.print(f"[cyan]Resume:[/] run {state.get('run_id')} — {state.get('done',0)} sukses, "
                          f"{state.get('failed',0)} gagal dari target {state.get('target')}")
            if state.get("target"):
                count = state["target"]
        elif state and state.get("status") == "finished":
            console.print("[yellow]State sebelumnya sudah 'finished'; memulai run baru.[/]")
            state = None
        else:
            console.print("[dim]Tidak ada state untuk di-resume; memulai run baru.[/]")

    remaining = max(0, count - already_done)
    console.print(f"\n[bold yellow]=== Menjalankan Full Pipeline ({count} Akun, sisa {remaining}) ===[/]")

    if remaining == 0:
        console.print("[green]Semua akun pada state ini sudah diproses.[/]")
        if state:
            _prog.mark_finished(state)
        return

    if not state:
        state = _prog.new_state(target=count)
        _prog.save_state(state)
    console.print(f"[dim]State: {_prog.PROGRESS_FILE} (run_id={state['run_id']})[/]")

    proxy_pool = None
    if proxy_file:
        from src.proxy import ProxyPool
        proxy_pool = ProxyPool(pool_path=proxy_file)
        console.print(f"[cyan]Proxy pool:[/] {proxy_pool.count} proxy dimuat dari {proxy_file}")
        if proxy_pool.count == 0:
            console.print("[yellow]! Proxy file kosong, lanjut tanpa proxy[/]")
            proxy_pool = None

    manager = CreatorManager(proxy_pool=proxy_pool, headless=headless, console=console)
    created_list = []

    # Adaptive backoff: jika gagal berturut-turut, jeda makin panjang (hindari rate-limit/blokir).
    fail_streak = {"n": 0}

    async def _backoff_if_needed(lock):
        n = fail_streak["n"]
        if n >= 3:
            delay = min(90, 10 * n)
            async with lock:
                console.print(f"  [yellow]! {n} gagal berturut-turut → backoff {delay}s[/]")
            write_log(f"Adaptive backoff {delay}s after {n} consecutive failures", "WARNING")
            await asyncio.sleep(delay)

    if concurrency and concurrency > 1:
        console.print(f"[cyan]Concurrency:[/] {concurrency} worker paralel")
        sem = asyncio.Semaphore(concurrency)
        print_lock = asyncio.Lock()
        results = [None] * remaining

        async def _worker(slot: int, idx: int):
            async with sem:
                await _backoff_if_needed(print_lock)
                for attempt in range(1, max_retries + 1):
                    try:
                        acc = await manager.create_account(idx=idx, auto_claim_bonus=True)
                    except Exception as e:
                        async with print_lock:
                            console.print(f"  [red]EXC[/] Account #{idx} attempt {attempt}: {e}")
                        write_log(f"Pipeline exception #{idx} attempt {attempt}: {e}", "ERROR")
                        acc = None
                    if acc:
                        results[slot] = acc
                        fail_streak["n"] = 0
                        _prog.mark_done(state, True)
                        return
                    if attempt < max_retries:
                        async with print_lock:
                            console.print(f"  [yellow]![/] Retry #{idx} ({attempt}/{max_retries}) dalam 8s...")
                        await asyncio.sleep(8)
                async with print_lock:
                    console.print(f"  [red]GAGAL[/] Account #{idx} setelah {max_retries} percobaan, dilewati.")
                write_log(f"Pipeline account #{idx} failed after {max_retries} attempts", "ERROR")
                fail_streak["n"] += 1
                _prog.mark_done(state, False)

        await asyncio.gather(*[_worker(s, already_done + s + 1) for s in range(remaining)])
        created_list = [r for r in results if r]
    else:
        for i in range(remaining):
            idx = already_done + i + 1
            await _backoff_if_needed(asyncio.Lock())
            acc = None
            for attempt in range(1, max_retries + 1):
                try:
                    acc = await manager.create_account(idx=idx, auto_claim_bonus=True)
                except Exception as e:
                    console.print(f"  [red]EXC[/] Account #{idx} attempt {attempt}: {e}")
                    write_log(f"Pipeline exception #{idx} attempt {attempt}: {e}", "ERROR")
                    acc = None
                if acc:
                    break
                if attempt < max_retries:
                    console.print(f"  [yellow]![/] Retry #{idx} ({attempt}/{max_retries}) dalam 8s...")
                    await asyncio.sleep(8)
            if acc:
                created_list.append(acc)
                fail_streak["n"] = 0
                _prog.mark_done(state, True)
            else:
                console.print(f"  [red]GAGAL[/] Account #{idx} setelah {max_retries} percobaan, dilewati.")
                write_log(f"Pipeline account #{idx} failed after {max_retries} attempts", "ERROR")
                fail_streak["n"] += 1
                _prog.mark_done(state, False)
            if i < remaining - 1:
                await asyncio.sleep(3)

    console.print(f"\n[bold green]Selesai membuat: {len(created_list)}/{remaining} akun (run ini)[/]")

    if not created_list:
        console.print("[red]Tidak ada akun yang berhasil dibuat dalam pipeline.[/]")
        _prog.mark_finished(state)
        return

    console.print(f"\n[bold cyan]Menginjeksi {len(created_list)} akun ke 9router...[/]")
    base_router = "http://127.0.0.1:20127"
    auth_token = _router_login(base_router)

    ok = 0
    fail = 0
    for acc in created_list:
        email = acc["email"]
        pat = acc.get("pat_token")
        if not pat:
            console.print(f"  [yellow]Skipping {email}: PAT kosong[/]")
            continue
        try:
            conn_id, valid = _router_add_and_test(base_router, auth_token, email, pat)
            if valid:
                ok += 1
                console.print(f"  [green]+ Terhubung 9router:[/] [bold cyan]{email}[/] (ID: {conn_id}) [green]✓ valid[/]")
            else:
                console.print(f"  [yellow]+ Terhubung 9router:[/] {email} (ID: {conn_id}) [yellow]! valid={valid}[/]")
        except Exception as e:
            fail += 1
            console.print(f"  [red]FAIL[/] Gagal menambahkan koneksi {email}: {e}")
            write_log(f"Ingest fail {email}: {e}", "ERROR")
    console.print(f"\n[bold]Ingest 9router: [green]{ok} OK[/], [red]{fail} gagal[/][/]")

    # Auto-sync: pastikan semua akun di accounts.txt (termasuk dari run sebelumnya) sudah masuk
    console.print("\n[dim]Auto-sync: memeriksa akun yang belum ter-inject...[/]")
    try:
        await cmd_sync(base_router)
    except Exception as e:
        console.print(f"[yellow]Auto-sync dilewati: {e}[/]")

    # Notifikasi ringkasan (jika webhook di-set)
    from src.utils import send_notification
    summary = (f"[Qoder Suite] Pipeline selesai: {len(created_list)}/{count} akun dibuat, "
               f"ingest 9router {ok} OK / {fail} gagal.")
    if send_notification(summary):
        console.print("[dim]Notifikasi terkirim.[/]")

    # Tandai state selesai (agar tidak di-resume lagi)
    _prog.mark_finished(state)
    console.print(f"[dim]State ditandai finished ({state['run_id']}).[/]")

def main():
    setup_logging()
    parser = argparse.ArgumentParser(description="Qoder Unified Suite CLI")
    subparsers = parser.add_subparsers(dest="command")

    # create
    p_create = subparsers.add_parser("create", help="Generate accounts with PAT and auto-claim")
    p_create.add_argument("-n", "--count", type=int, default=1, help="Number of accounts")
    p_create.add_argument("--headless", action="store_true", default=True, help="Headless browser")
    p_create.add_argument("--no-claim", action="store_true", help="Skip bonus claiming")

    # claim
    p_claim = subparsers.add_parser("claim", help="Inspect quota and claim bonus for a PAT")
    p_claim.add_argument("--pat", type=str, required=True, help="Personal Access Token (pt-*)")

    # login
    p_login = subparsers.add_parser("login-9r", help="Login existing accounts to 9router via OAuth")
    p_login.add_argument("-f", "--file", type=str, default=None, help="Path to account list file")
    p_login.add_argument("-b", "--batch", type=int, default=1, help="Concurrent batch size")
    p_login.add_argument("--headless", action="store_true", default=True, help="Headless browser")

    # pipeline
    p_pipe = subparsers.add_parser("pipeline", help="End-to-End: Create -> Auto-Claim -> 9router Ingestion")
    p_pipe.add_argument("-n", "--count", type=int, default=1, help="Number of accounts")
    p_pipe.add_argument("--headless", action="store_true", default=True, help="Headless browser")
    p_pipe.add_argument("-r", "--retries", type=int, default=3, help="Max retries per account")
    p_pipe.add_argument("-p", "--proxy-file", type=str, default=None, help="File daftar proxy (satu per baris)")
    p_pipe.add_argument("-t", "--target", type=int, default=None, help="Top-up sampai total N akun di accounts.txt")
    p_pipe.add_argument("-c", "--concurrency", type=int, default=1, help="Jumlah worker paralel (default 1)")
    p_pipe.add_argument("--resume", action="store_true", help="Lanjutkan batch yang terputus dari state terakhir")

    # progress
    subparsers.add_parser("progress", help="Tampilkan status batch terakhir (state resume)")

    # sync
    p_sync = subparsers.add_parser("sync", help="Injeksi akun accounts.txt yang belum ada di 9router")
    p_sync.add_argument("--router", type=str, default="http://127.0.0.1:20127", help="Base URL 9router")

    # sync-lintasan
    p_sync_lintasan = subparsers.add_parser("sync-lintasan", help="Injeksi akun accounts.txt ke Lintasan Gateway / DB")
    p_sync_lintasan.add_argument("--url", type=str, default=None, help="Base URL Lintasan")
    p_sync_lintasan.add_argument("--api-key", type=str, default=None, help="API key Lintasan")
    p_sync_lintasan.add_argument("--db", type=str, default=None, help="Path DB SQLite Lintasan")

    # report
    subparsers.add_parser("report", help="Ringkasan status akun lokal vs 9router")

    # retest
    p_retest = subparsers.add_parser("retest", help="Test ulang koneksi Qoder di 9router")
    p_retest.add_argument("--router", type=str, default="http://127.0.0.1:20127", help="Base URL 9router")
    p_retest.add_argument("--all", action="store_true", help="Retest semua (default hanya non-active)")

    # usage
    p_usage = subparsers.add_parser("usage", help="Tampilkan kuota tiap koneksi Qoder")
    p_usage.add_argument("--router", type=str, default="http://127.0.0.1:20127", help="Base URL 9router")
    p_usage.add_argument("-n", "--limit", type=int, default=None, help="Tampilkan N koneksi terakhir saja")

    # doctor
    subparsers.add_parser("doctor", help="Preflight check: temp-mail, 9router, login")

    # health
    p_health = subparsers.add_parser("health", help="Health check end-to-end (test chat nyata)")
    p_health.add_argument("-m", "--model", type=str, default="qd/qfmodel", help="Model untuk test chat")

    # verify / renew PAT
    p_verify = subparsers.add_parser("verify", help="Verifikasi semua PAT (opsional renew yang mati)")
    p_verify.add_argument("--renew", action="store_true", help="Renew PAT yang invalid via login Qoder")

    args = parser.parse_args()

    banner()

    if args.command == "create":
        asyncio.run(cmd_create(args.count, args.headless, not args.no_claim))
    elif args.command == "claim":
        asyncio.run(cmd_claim(args.pat))
    elif args.command == "login-9r":
        asyncio.run(cmd_login(args.file, args.batch, args.headless))
    elif args.command == "pipeline":
        asyncio.run(cmd_pipeline(args.count, args.headless, args.retries, args.proxy_file,
                                 args.target, args.concurrency, args.resume))
    elif args.command == "progress":
        cmd_progress_sync()
    elif args.command == "sync":
        asyncio.run(cmd_sync(args.router))
    elif args.command == "sync-lintasan":
        asyncio.run(cmd_sync_lintasan(args.url, args.api_key, args.db))
    elif args.command == "report":
        cmd_report_sync()
    elif args.command == "retest":
        asyncio.run(cmd_retest(args.router, not args.all))
    elif args.command == "usage":
        cmd_usage_sync(args.router, args.limit)
    elif args.command == "doctor":
        cmd_doctor_sync()
    elif args.command == "health":
        cmd_health_sync(args.model)
    elif args.command == "verify":
        cmd_verify_renew_sync(args.renew)
    else:
        # Tampilkan menu interaktif
        table = Table(box=box.ROUNDED)
        table.add_column("Perintah", style="bold cyan")
        table.add_column("Deskripsi", style="white")
        table.add_row("python main.py create -n 1", "Buat akun baru + PAT + auto claim kuota")
        table.add_row("python main.py claim --pat pt-...", "Verifikasi PAT & klaim bonus kuota")
        table.add_row("python main.py login-9r", "Login akun di file (email:pass:pat) ke 9router")
        table.add_row("python main.py pipeline -n 1", "Full Auto: Create -> Claim -> Direct 9router Login")
        table.add_row("python main.py sync", "Injeksi ulang akun accounts.txt yang belum masuk 9router")
        table.add_row("python main.py sync-lintasan", "Injeksi akun accounts.txt ke Lintasan Gateway / DB")
        table.add_row("python main.py report", "Ringkasan status akun lokal vs 9router")
        table.add_row("python main.py retest", "Test ulang koneksi Qoder non-active di 9router")
        table.add_row("python main.py usage", "Tampilkan kuota tiap koneksi Qoder")
        table.add_row("python main.py doctor", "Preflight check: temp-mail, 9router, login")
        table.add_row("python main.py health", "Health check end-to-end (test chat nyata)")
        table.add_row("python main.py progress", "Status batch terakhir (resume state)")
        table.add_row("python main.py verify --renew", "Cek & renew PAT yang invalid")
        console.print(table)

if __name__ == "__main__":
    main()

import asyncio
import random
from pathlib import Path
from typing import List, Dict, Any, Tuple
from rich.console import Console

from .captcha import solve_slider_local
from .config import (
    NINE_ROUTER_URL, NINE_ROUTER_PASS, CAPTCHA_ATTEMPTS, CALLBACK_WAIT_MS
)
from .stealth import launch_stealth_browser, create_stealth_context
from .utils import write_log, remove_account_txt

def parse_line(line: str):
    """
    Parse format:
    email:password:pat  atau  email:password
    """
    if ":" not in line:
        return None
    parts = line.split(":")
    if len(parts) >= 3:
        email = parts[0].strip()
        pat = parts[-1].strip()
        password = ":".join(parts[1:-1]).strip()
        if not email or not password or "@" not in email:
            return None
        return email, password, pat
    elif len(parts) == 2:
        email, password = parts[0].strip(), parts[1].strip()
        if not email or not password or "@" not in email:
            return None
        return email, password, ""
    return None

def load_accounts(filepath: Path) -> List[Dict[str, Any]]:
    accounts = []
    if not filepath.exists():
        return accounts
    for line in filepath.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parsed = parse_line(line)
        if parsed:
            accounts.append({
                "email": parsed[0],
                "password": parsed[1],
                "pat": parsed[2],
                "raw_line": line
            })
    return accounts

async def _page_says_success(page) -> bool:
    try:
        body = await page.evaluate("() => document.body?.innerText || ''")
        return "sign in success" in body.lower()
    except Exception:
        return False

class LoginManager:
    def __init__(self, router_url: str = None, router_pass: str = None, headless: bool = True, console: Console = None):
        self.router_url = router_url or NINE_ROUTER_URL
        self.router_pass = router_pass or NINE_ROUTER_PASS
        self.headless = headless
        self.console = console or Console()

    async def _wait_success(self, page, state: dict, seconds: float) -> bool:
        for _ in range(int(seconds * 2)):
            if state["done"] or await _page_says_success(page):
                state["done"] = True
                return True
            await page.wait_for_timeout(500)
        return state["done"]

    async def _finish_success(self, page, email: str, note: str = "") -> bool:
        self.console.print(f"  [green]OK[/] Login sukses{note}: [bold]{email}[/]")
        write_log(f"Login success{note}: {email}", "SUCCESS")
        # Jeda 20 detik agar 9router (halaman utama) menyelesaikan polling deviceToken ke openapi.qoder.sh
        self.console.print("  [dim]Menunggu 9router menyelesaikan polling token & menyimpan kredensial (20s)...[/]")
        await page.wait_for_timeout(20000)
        return True

    async def login_account(self, account: dict, playwright) -> bool:
        email = account["email"]
        password = account["password"]
        c = self.console

        c.print(f"  [cyan]->[/] [bold]{email}[/]")
        write_log(f"Login attempt: {email}", "INFO")

        browser = None
        try:
            browser = await launch_stealth_browser(playwright, headless=self.headless)
            context = await create_stealth_context(browser)

            state = {"done": False, "signed_in": False}

            async def on_response(response):
                if state["signed_in"] and "selectAccounts" in response.url:
                    state["done"] = True

            page = await context.new_page()

            # 1. Open 9router
            try:
                await page.goto(self.router_url, wait_until="networkidle", timeout=30000)
            except Exception:
                c.print(f"  [red]FAIL[/] 9router tidak bisa dibuka: {self.router_url}")
                return False
            await page.wait_for_timeout(1000)

            # Cek jika diarahkan ke halaman login 9router
            if "/login" in page.url:
                c.print("  [dim]9router terproteksi password, melakukan login dashboard...[/]")
                pw_input = await page.wait_for_selector("input[type='password']", timeout=10000)
                if pw_input:
                    await pw_input.click()
                    await page.keyboard.type(self.router_pass, delay=40)
                    await page.keyboard.press("Enter")
                    await page.wait_for_load_state("networkidle", timeout=15000)
                    await page.wait_for_timeout(2000)
                # Kembali arahkan ke provider url jika belum di sana
                if "/providers/qoder" not in page.url:
                    await page.goto(self.router_url, wait_until="networkidle", timeout=30000)
                    await page.wait_for_timeout(2000)

            # 2. Klik OAuth
            new_pages = []
            context.on("page", lambda pg: new_pages.append(pg))
            oauth_btn = await page.query_selector("button:has-text('OAuth')")
            if not oauth_btn:
                # Coba cari tombol Add Connection jika belum ada tombol OAuth langsung
                add_btn = await page.query_selector("button:has-text('Add Connection')")
                if add_btn:
                    await add_btn.click()
                    await page.wait_for_timeout(1000)
                    oauth_btn = await page.query_selector("button:has-text('OAuth')")

            if not oauth_btn:
                c.print("  [red]FAIL[/] Tombol OAuth tidak ditemukan di 9router")
                return False
            await oauth_btn.click()
            await page.wait_for_timeout(3000)

            # Jika browser tidak membuka tab baru otomatis (misal popup terblokir atau flow modal),
            # periksa apakah muncul link/tombol Open di modal
            if not new_pages:
                open_btn = await page.query_selector("button:has-text('Open'), a:has-text('Open')")
                if open_btn:
                    await open_btn.click()
                    await page.wait_for_timeout(3000)

            if not new_pages:
                c.print("  [red]FAIL[/] Tab sign-in OAuth tidak terbuka")
                return False

            sp = new_pages[-1]
            sp.on("response", on_response)
            await sp.wait_for_load_state("domcontentloaded", timeout=15000)
            await sp.wait_for_timeout(1000)

            # 3. Email
            email_input = await sp.wait_for_selector("#basic_email", timeout=10000)
            await email_input.fill(email)
            await sp.wait_for_timeout(300)
            await (await sp.query_selector("button:has-text('Continue')")).click()
            try:
                await sp.wait_for_selector("#password_password", state="visible", timeout=8000)
            except Exception:
                c.print("  [red]FAIL[/] Field password tidak muncul (email tidak terdaftar?)")
                return False
            await sp.wait_for_timeout(300)

            # 4. Password
            await (await sp.query_selector("#password_password")).fill(password)
            await sp.wait_for_timeout(300)
            await (await sp.query_selector("button:has-text('Sign in')")).click()
            state["signed_in"] = True

            # [5] Cek apakah butuh Captcha
            verify_el = await sp.query_selector("#aliyunCaptcha-captcha-body")
            if not verify_el:
                verify_el = await sp.query_selector("button:has-text('Click to verify')")

            if verify_el and await verify_el.is_visible():
                c.print("  [yellow]![/] Captcha terdeteksi pada login, solving...")
                await verify_el.click()
                await sp.wait_for_timeout(2000)
                await solve_slider_local(sp, max_attempts=CAPTCHA_ATTEMPTS, console=c)
                await sp.wait_for_timeout(2000)

            # [6] Tunggu respon sukses atau callback selectAccounts
            if await self._wait_success(sp, state, 15):
                return await self._finish_success(sp, email)

            # Re-check captcha jika muncul terlambat
            verify_el2 = await sp.query_selector("#aliyunCaptcha-captcha-body")
            if not verify_el2:
                verify_el2 = await sp.query_selector("button:has-text('Click to verify')")

            if verify_el2 and await verify_el2.is_visible():
                c.print("  [yellow]![/] Captcha muncul kembali, solving attempt 2...")
                await verify_el2.click()
                await sp.wait_for_timeout(2000)
                await solve_slider_local(sp, max_attempts=CAPTCHA_ATTEMPTS, console=c)
                if await self._wait_success(sp, state, 15):
                    return await self._finish_success(sp, email)

            # Check error body
            body = (await sp.evaluate("() => document.body?.innerText || ''")).lower()
            if any(w in body for w in ("incorrect", "invalid", "wrong")):
                c.print("  [red]FAIL[/] Password salah")
            else:
                c.print("  [red]FAIL[/] Login gagal")
            return False

        except Exception as e:
            c.print(f"  [red]FAIL[/] Error: {str(e)[:80]}")
            return False
        finally:
            if browser:
                try:
                    await browser.close()
                except Exception:
                    pass

async def run_login_batch(
    accounts: List[dict],
    batch_size: int = 1,
    router_url: str = None,
    headless: bool = True,
    console: Console = None,
    input_file: Path = None,
) -> Tuple[int, int]:
    from playwright.async_api import async_playwright

    c = console or Console()
    manager = LoginManager(router_url=router_url, headless=headless, console=c)

    total = len(accounts)
    ok = fail = 0
    c.print(f"\n  [dim]Total: {total} akun | Batch: {batch_size} | Headless: {headless}[/]\n")

    async with async_playwright() as p:
        for start in range(0, total, batch_size):
            batch = accounts[start:start + batch_size]
            c.print(f"  [dim]-- Batch {start // batch_size + 1} ({len(batch)} akun) --[/]")

            results = await asyncio.gather(
                *(manager.login_account(acc, p) for acc in batch),
                return_exceptions=True,
            )
            for acc, result in zip(batch, results):
                if result is True:
                    ok += 1
                    if input_file:
                        remove_account_txt(input_file, acc["raw_line"])
                else:
                    fail += 1

            if start + batch_size < total:
                await asyncio.sleep(random.uniform(1.5, 3.0))

    return ok, fail

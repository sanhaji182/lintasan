import asyncio
import time
from typing import Optional, Dict, Any
from playwright.async_api import Page, async_playwright
from rich.console import Console

from .config import QODER_BASE, ACCOUNTS_JSONL, OUTPUT_TXT, CAPTCHA_ATTEMPTS
from .utils import write_log, generate_password, save_jsonl, append_account_txt, generate_machine_id, log_event
from .tempmail import TempikClient
from .proxy import ProxyPool
from .stealth import create_stealth_context, launch_stealth_browser
from .captcha import solve_slider_local
from .pat import PATManager
from .claim import CosyEngine

class CreatorManager:
    """Manages full signup -> OTP -> PAT -> Claim flow."""

    def __init__(self, proxy_pool: Optional[ProxyPool] = None, headless: bool = True, console: Optional[Console] = None):
        self.proxy_pool = proxy_pool
        self.headless = headless
        self.console = console or Console()
        self.cosy = CosyEngine()

    async def create_account(self, idx: int = 1, auto_claim_bonus: bool = True) -> Optional[Dict[str, Any]]:
        proxy = self.proxy_pool.next() if self.proxy_pool else None
        c = self.console

        c.print(f"\n[bold cyan]=== [Account #{idx}] Creating Qoder Account ===[/]")
        c.print("  [dim]Step 1: Mengambil disposable email dari Tempik...[/]")
        tempmail = TempikClient()
        try:
            email = tempmail.create_inbox()
        except Exception as e:
            c.print(f"  [red]FAIL[/] Gagal mengambil temp mail: {e}")
            write_log(f"[{idx}] Tempmail error: {e}", "ERROR")
            return None

        password = generate_password()
        c.print(f"  [green]OK[/] Email   : [bold cyan]{email}[/]")
        c.print(f"  [green]OK[/] Password: [bold yellow]{password}[/]")

        async with async_playwright() as p:
            c.print("  [dim]Step 2: Menjalankan stealth Chromium...[/]")
            browser = await launch_stealth_browser(p, proxy, self.headless)
            context = await create_stealth_context(browser, proxy)
            page = await context.new_page()

            try:
                # 3. Open signup
                c.print("  [dim]Step 3: Membuka form registrasi...[/]")
                await page.goto(f"{QODER_BASE}/users/sign-up", wait_until="domcontentloaded", timeout=60000)
                await page.wait_for_timeout(4000)

                # 4. Fill name + email
                await page.fill("#basic_firstName", "Dev")
                await page.fill("#basic_lastName", "User")
                await page.fill("#basic_email", email)

                cb = await page.query_selector("input[type=checkbox]")
                if cb:
                    try:
                        await cb.check(force=True)
                    except Exception:
                        pass

                for cbox in await page.query_selector_all("input[type=checkbox]"):
                    try:
                        await cbox.check(force=True)
                    except Exception:
                        pass

                continue_btn = await page.query_selector("button:has-text('Continue')")
                if continue_btn:
                    await continue_btn.click()
                await page.wait_for_timeout(3000)

                # 5. Fill password
                pw = await page.query_selector("#basic_password")
                if pw:
                    await pw.click(force=True)
                    await page.keyboard.type(password, delay=20)
                    btn2 = await page.query_selector("button:has-text('Continue')")
                    if btn2:
                        await btn2.click()
                await page.wait_for_timeout(3000)

                # 6. Solve captcha
                verify_el = await page.query_selector("#aliyunCaptcha-captcha-body, button:has-text('Click to verify')")
                if verify_el and await verify_el.is_visible():
                    c.print("  [yellow]![/] Slider captcha terdeteksi, memecahkan lokal...")
                    await verify_el.click()
                    await page.wait_for_timeout(2000)
                    solved = await solve_slider_local(page, max_attempts=CAPTCHA_ATTEMPTS, console=c)
                    if not solved:
                        c.print("  [red]FAIL[/] Gagal menyelesaikan captcha")
                        return None
                    await page.wait_for_timeout(4000)

                # 7. OTP
                c.print("  [dim]Step 4: Menunggu kode OTP dari inbox...[/]")
                messages = await tempmail.wait_for_messages(email, max_wait=120, interval=4)
                if not messages:
                    c.print("  [red]FAIL[/] Timeout menunggu OTP")
                    return None

                otp = tempmail.extract_otp(messages)
                if not otp:
                    c.print("  [red]FAIL[/] OTP tidak ditemukan di email")
                    return None
                c.print(f"  [green]OK[/] Kode OTP didapat: [bold green]{otp}[/]")

                # Fill OTP
                otp_inputs = await page.query_selector_all("input.ant-otp-input")
                if len(otp_inputs) >= 6:
                    await otp_inputs[0].click()
                    await page.wait_for_timeout(200)
                    await page.keyboard.type(otp, delay=80)
                    await page.wait_for_timeout(1500)
                else:
                    all_inputs = await page.query_selector_all('input:not([type="hidden"])')
                    if all_inputs:
                        await all_inputs[0].click()
                        await page.keyboard.type(otp, delay=80)
                    else:
                        await page.keyboard.type(otp, delay=80)

                # Submit OTP
                for sel in ['button:has-text("Create account")', 'button:has-text("Verify")', 'button:has-text("Sign up")', 'button[type="submit"]']:
                    el = await page.query_selector(sel)
                    if el and await el.is_visible():
                        await el.click()
                        break
                await page.wait_for_timeout(8000)

                # 8. Create PAT
                c.print("  [dim]Step 5: Generate Personal Access Token (PAT)...[/]")
                pat_response = await PATManager.create(page, "suite")
                pat_token = PATManager.extract_token(pat_response)
                if not pat_token:
                    c.print("  [yellow]![/] PAT gagal dibuat via session cookie, melanjutkan tanpa PAT...")
                    pat_token = ""

                c.print(f"  [green]OK[/] PAT Berhasil: [bold green]{pat_token[:16]}...[/]" if pat_token else "  [dim]PAT: (kosong)[/]")

                # Step 5b: Coba trigger claim trial onboarding via web session jika ada button / endpoint
                try:
                    c.print("  [dim]Step 5b: Triggering web onboarding trial / claim...[/]")
                    await page.evaluate('''async () => {
                        try {
                            await fetch('/api/v1/me/trial', { method: 'POST', credentials: 'include' });
                            await fetch('/api/v2/activity/claim?activityId=new_user_trial', { method: 'POST', credentials: 'include' });
                        } catch(e) {}
                    }''')
                except Exception:
                    pass

            finally:
                await context.close()
                await browser.close()

        # Step 6: Claim bonus jika aktif & PAT ada
        claim_data = None
        if auto_claim_bonus and pat_token:
            c.print("  [dim]Step 6: Auto-Claim Bonus Kuota Event (Cosy Engine)...[/]")
            mid = generate_machine_id()
            claim_data = self.cosy.auto_claim(pat_token, machine_id=mid)
            if claim_data.get("success"):
                c.print("  [green]OK[/] Auto-Claim & kuota scan selesai")
            else:
                c.print(f"  [dim]Auto-claim notice: {claim_data.get('error')}[/]")

        record = {
            "email": email,
            "password": password,
            "pat_token": pat_token,
            "pat_valid": bool(pat_token),
            "claim_data": claim_data,
            "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        }

        save_jsonl(ACCOUNTS_JSONL, record)
        append_account_txt(OUTPUT_TXT, email, password, pat_token)
        log_event("account_created", email=email, has_pat=bool(pat_token),
                  domain=email.split("@")[-1] if "@" in email else "",
                  claim_ok=bool((claim_data or {}).get("success")))
        c.print(f"  [bold green]SUCCESS:[/] Akun tersimpan di [cyan]{OUTPUT_TXT.name}[/] [dim](format email:pass:pat)[/]")
        return record

"""
Qoder Suite - PAT Verify & Renew

- verify_pat(pat)   : cek apakah PAT masih valid (exchange ke job token).
- renew_pat(account): login ulang ke Qoder via browser, buat PAT baru,
                      kembalikan token baru.

PAT Qoder berlaku ~12 bulan, jadi renew umumnya hanya perlu untuk akun lama
atau saat token dicabut/di-revoke.
"""

import asyncio
import json
from typing import Dict, Any, Optional

from .config import QODER_BASE, CAPTCHA_ATTEMPTS
from .utils import write_log, log_event
from .claim import CosyEngine
from .stealth import launch_stealth_browser, create_stealth_context
from .captcha import solve_slider_local
from .pat import PATManager


def verify_pat(pat: str) -> Dict[str, Any]:
    """Cek validitas PAT via exchange. Return {valid, reason, expires_at}."""
    if not pat or not pat.startswith("pt-"):
        return {"valid": False, "reason": "format PAT tidak valid"}
    e = CosyEngine()
    r = e.exchange_pat_to_job_token(pat)
    if r.get("success"):
        raw = r.get("raw") or {}
        return {"valid": True, "expires_at": raw.get("refresh_token_expires_at"),
                "job_expires_at": raw.get("expires_at")}
    return {"valid": False, "reason": str(r.get("error"))[:150]}


async def renew_pat(email: str, password: str, headless: bool = True,
                    console=None) -> Optional[str]:
    """Login ulang ke Qoder & buat PAT baru. Return token baru atau None."""
    from rich.console import Console
    c = console or Console()

    async with __import__("playwright.async_api", fromlist=["async_playwright"]).async_playwright() as p:
        browser = await launch_stealth_browser(p, headless=headless)
        context = await create_stealth_context(browser)
        page = await context.new_page()
        try:
            c.print(f"  [dim]Login Qoder untuk {email}...[/]")
            await page.goto(f"{QODER_BASE}/users/sign-in", wait_until="domcontentloaded", timeout=60000)
            await page.wait_for_timeout(3000)

            # Email
            try:
                await page.wait_for_selector("#basic_email", timeout=15000)
            except Exception:
                c.print("  [red]FAIL[/] Form login Qoder tidak muncul")
                return None
            await page.fill("#basic_email", email)
            await page.wait_for_timeout(300)
            btn = await page.query_selector("button:has-text('Continue')")
            if btn:
                await btn.click()
            await page.wait_for_timeout(2500)

            # Password
            try:
                await page.wait_for_selector("#password_password", state="visible", timeout=10000)
            except Exception:
                c.print("  [red]FAIL[/] Field password tidak muncul (email/kredensial salah?)")
                return None
            await page.fill("#password_password", password)
            await page.wait_for_timeout(300)
            sbtn = await page.query_selector("button:has-text('Sign in')")
            if sbtn:
                await sbtn.click()
            await page.wait_for_timeout(2500)

            # Captcha (jika muncul)
            verify_el = await page.query_selector("#aliyunCaptcha-captcha-body")
            if not verify_el:
                verify_el = await page.query_selector("button:has-text('Click to verify')")
            if verify_el and await verify_el.is_visible():
                c.print("  [yellow]![/] Captcha pada login, solving...")
                await verify_el.click()
                await page.wait_for_timeout(2000)
                await solve_slider_local(page, max_attempts=CAPTCHA_ATTEMPTS, console=c)
                await page.wait_for_timeout(3000)

            await page.wait_for_timeout(4000)

            # Buat PAT baru via sesi web
            c.print("  [dim]Membuat PAT baru...[/]")
            pat_response = await PATManager.create(page, "renew")
            token = PATManager.extract_token(pat_response)
            if token:
                c.print(f"  [green]OK[/] PAT baru: {token[:16]}...")
                log_event("pat_renewed", email=email)
                return token
            c.print(f"  [red]FAIL[/] Gagal membuat PAT baru (status={pat_response.get('status')})")
            return None
        except Exception as e:
            c.print(f"  [red]FAIL[/] renew error: {e}")
            write_log(f"renew_pat error {email}: {e}", "ERROR")
            return None
        finally:
            try:
                await context.close()
                await browser.close()
            except Exception:
                pass

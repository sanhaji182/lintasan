"""
Qoder Creator - PAT Manager
Create Personal Access Tokens via Qoder web session.
"""

import asyncio
import json
import re
from typing import Optional, Dict, Any

from .utils import write_log


class PATManager:
    """Manage PAT creation via browser session."""

    @staticmethod
    async def create(page, name: str = "farm") -> Dict[str, Any]:
        """Create a PAT via the Qoder web session (browser cookies)."""
        try:
            pat = await page.evaluate(
                """async ({name}) => {
                    const exp = new Date();
                    exp.setHours(23, 59, 59, 999);
                    exp.setMonth(exp.getMonth() + 12);
                    const csrf = window.csrfToken || '';
                    const r = await fetch('/api/v1/me/personal-access-tokens', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            'X-CSRF-TOKEN': csrf,
                            'x-requested-with': 'XMLHttpRequest'
                        },
                        credentials: 'include',
                        body: JSON.stringify({
                            name: name + '-' + Date.now(),
                            expires_at: exp.valueOf()
                        })
                    });
                    return {status: r.status, body: await r.text()};
                }""",
                {"name": name},
            )

            write_log(f"PAT created: status={pat.get('status')}", "INFO")
            return pat

        except Exception as e:
            write_log(f"PAT creation error: {e}", "ERROR")
            return {"status": 0, "body": "", "err": str(e)}

    @staticmethod
    def extract_token(pat_response: Dict[str, Any]) -> Optional[str]:
        """Extract the PAT token from the API response."""
        if pat_response.get("status") != 201:
            return None

        body = pat_response.get("body", "")
        try:
            data = json.loads(body)
            token = data.get("token")
            if token and len(token) >= 64:
                return token
        except json.JSONDecodeError:
            pass

        # Try regex
        match = re.search(r'"token"\s*:\s*"(pt-[A-Za-z0-9_\-]+)"', body)
        if match and len(match.group(1)) >= 64:
            return match.group(1)

        return None

    @staticmethod
    def is_valid(pat_response: Dict[str, Any]) -> bool:
        """Check if PAT response is valid."""
        return PATManager.extract_token(pat_response) is not None
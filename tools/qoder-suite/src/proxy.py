"""
Qoder Creator - Proxy Manager
Rotating proxy pool dengan format: http://user:pass@host:port atau host:port:user:pass
"""

import os
import random
from typing import Optional, Dict, List
from urllib.parse import urlparse

from .utils import write_log


class ProxyPool:
    """Rotating proxy pool manager."""

    def __init__(self, proxies: List[str] = None, pool_path: str = None):
        self._proxies: List[str] = []
        self._idx = 0

        if proxies:
            self._proxies = proxies
        elif pool_path and os.path.exists(pool_path):
            self.load(pool_path)

    def load(self, path: str):
        """Load proxies from file (one per line)."""
        with open(path, "r") as f:
            self._proxies = [
                line.strip() for line in f if line.strip() and not line.startswith("#")
            ]
        write_log(f"Loaded {len(self._proxies)} proxies from {path}", "INFO")

    @property
    def count(self) -> int:
        return len(self._proxies)

    def next(self) -> Optional[Dict[str, str]]:
        """Get next proxy in round-robin. Returns {server, username, password} or None."""
        if not self._proxies:
            return None

        raw = self._proxies[self._idx % len(self._proxies)]
        self._idx += 1
        return self._parse(raw)

    def random(self) -> Optional[Dict[str, str]]:
        """Get a random proxy."""
        if not self._proxies:
            return None
        return self._parse(random.choice(self._proxies))

    @staticmethod
    def _parse(raw: str) -> Dict[str, str]:
        """Parse proxy string into structured dict."""
        # Format: http://user:pass@host:port
        if raw.startswith("http"):
            u = urlparse(raw)
            return {
                "server": f"{u.scheme}://{u.hostname}:{u.port}",
                "username": u.username or "",
                "password": u.password or "",
            }

        # Format: host:port:user:pass
        parts = raw.split(":")
        if len(parts) == 4:
            return {
                "server": f"http://{parts[0]}:{parts[1]}",
                "username": parts[2],
                "password": parts[3],
            }

        # Format: host:port
        if len(parts) == 2:
            return {"server": f"http://{parts[0]}:{parts[1]}"}

        raise ValueError(f"Invalid proxy format: {raw[:30]}...")
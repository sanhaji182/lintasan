"""
Cosy Claim & Quota Engine (Ported & Enhanced from qoder-workflow)
Handles:
1. PAT -> Job Token (jt-*) direct exchange via openapi.qoder.sh
2. Dynamic MD5 signature generation
3. Quota checking
4. Activity claim / bonus checks
"""

import hashlib
import json
import urllib.request
import urllib.parse
from datetime import datetime, timezone
from typing import Dict, Any, Optional

class CosyEngine:
    def __init__(self, base_url="https://openapi.qoder.sh", appcode="cosy", secret="cosy&war, war never changes"):
        self.base_url = base_url.rstrip("/")
        self.appcode = appcode
        self.secret = secret

    def exchange_pat_to_job_token(self, pat_token: str) -> Dict[str, Any]:
        """
        Exchange PAT (pt-...) to short-lived Job Token (jt-...) directly via API.
        No local binary required!
        """
        url = f"{self.base_url}/api/v1/jobToken/exchange"
        payload = json.dumps({"personal_token": pat_token}).encode("utf-8")
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json",
            "User-Agent": "qodercli/1.0.0",
            "Cosy-Version": "1.1.5",
            "Cosy-ClientType": "5"
        }
        req = urllib.request.Request(url, data=payload, headers=headers, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=20) as resp:
                data = json.loads(resp.read().decode())
                return {
                    "success": True,
                    "job_token": data.get("token"),
                    "refresh_token": data.get("refresh_token"),
                    "raw": data
                }
        except urllib.error.HTTPError as e:
            err_body = e.read().decode() if hasattr(e, "read") else ""
            return {"success": False, "error": f"HTTP {e.code}: {err_body}"}
        except Exception as e:
            return {"success": False, "error": str(e)}

    def generate_signature(self):
        now_gmt = datetime.now(timezone.utc).strftime('%a, %d %b %Y %H:%M:%S GMT')
        raw_str = f"{self.secret}&{now_gmt}"
        sig = hashlib.md5(raw_str.encode("utf-8")).hexdigest()
        return now_gmt, sig

    def get_headers(self, token: str, machine_id: str, runtime_info: Optional[Dict[str, Any]] = None):
        runtime_info = runtime_info or {}
        now_gmt, sig = self.generate_signature()
        
        return {
            "Authorization": f"Bearer {token}",
            "Cosy-Version": "1.1.5",
            "Cosy-ClientType": "5",
            "Cosy-MachineOS": "arm64_darwin",
            "Cosy-MachineId": machine_id,
            "Cosy-MachineToken": runtime_info.get("machineToken", ""),
            "Cosy-MachineType": runtime_info.get("machineType", ""),
            "Cosy-MachineCode": runtime_info.get("machineCode", ""),
            "appcode": self.appcode,
            "login-version": "v2",
            "date": now_gmt,
            "signature": sig,
            "User-Agent": "Go-http-client/2.0",
            "Accept": "application/json",
            "Content-Type": "application/json"
        }

    def get_quota_usage(self, token: str, machine_id: str) -> Dict[str, Any]:
        """Fetch quota usage."""
        url = f"{self.base_url}/api/v2/quota/usage"
        headers = self.get_headers(token, machine_id)
        req = urllib.request.Request(url, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=20) as res:
                return json.loads(res.read().decode())
        except Exception as e:
            return {"code": -1, "error": str(e)}

    def check_eligibility(self, token: str, machine_id: str, runtime_info: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Check activity/event claim eligibility."""
        url = f"{self.base_url}/api/v2/activity/claim/eligibility"
        headers = self.get_headers(token, machine_id, runtime_info)
        req = urllib.request.Request(url, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=20) as res:
                return json.loads(res.read().decode())
        except Exception as e:
            return {"code": -1, "error": str(e)}

    def claim_activity(self, activity_id: str, token: str, machine_id: str, runtime_info: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Claim activity / trial bonus."""
        encoded_id = urllib.parse.quote(activity_id, safe="")
        url = f"{self.base_url}/api/v2/activity/claim?activityId={encoded_id}"
        headers = self.get_headers(token, machine_id, runtime_info)
        req = urllib.request.Request(url, headers=headers, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=20) as res:
                return json.loads(res.read().decode())
        except Exception as e:
            return {"code": -1, "error": str(e)}

    def auto_claim(self, pat_or_token: str, machine_id: str = "default-mid-001") -> Dict[str, Any]:
        """
        Auto claim runner:
        1. If token starts with pt-, exchange to jt-
        2. Check quota
        3. Check eligibility and claim any available activities
        """
        eff_token = pat_or_token
        if pat_or_token.startswith("pt-"):
            res = self.exchange_pat_to_job_token(pat_or_token)
            if res.get("success"):
                eff_token = res.get("job_token")
            else:
                return {"success": False, "error": f"Exchange PAT failed: {res.get('error')}"}

        quota = self.get_quota_usage(eff_token, machine_id)
        elig = self.check_eligibility(eff_token, machine_id)
        
        claims = []
        # Parse elig activities if present
        data = elig.get("data") or {}
        activities = data.get("activities") or []
        for act in activities:
            act_id = act.get("activityId") or act.get("id")
            if act_id and act.get("status") in ("available", 1, True):
                c_res = self.claim_activity(str(act_id), eff_token, machine_id)
                claims.append({"activityId": act_id, "result": c_res})

        return {
            "success": True,
            "quota": quota,
            "eligibility": elig,
            "claims": claims
        }

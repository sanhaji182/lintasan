# OAuth IDE — 9router parity (Go rewrite)

Experimental lab (`LINTASAN_OAUTH_IDE_ENABLED=false` by default).

## Catalog (8 providers)

Mirrors `9router` `OAUTH_PROVIDERS` @ v0.4.71:

| id | flow | implementation |
|----|------|----------------|
| **claude** | PKCE | **ready** |
| antigravity | PKCE | planned |
| **codex** | PKCE | **ready** |
| **github** | device_code | **ready** |
| cursor | import_token | import_only |
| **xai** | PKCE | **ready** |
| kilocode | custom_device | planned |
| cline | local_app_callback | planned |

All eight appear in `/api/oauth/status` → `catalog`. Only `implementation=ready` accepts **Authorize** today.

## Enable

```bash
export LINTASAN_OAUTH_IDE_ENABLED=true
export LINTASAN_OAUTH_PUBLIC_BASE_URL=https://your-lintasan-host
```

**xAI (Grok):** uses public client id ported from 9router — no env client secret. Redirect URI must match `.../api/oauth/callback/xai`.

## Next ports (from `/tmp/9router-decolua-fresh`)

1. github — `GITHUB_CONFIG` device flow + copilot_internal token  
2. claude / codex — PKCE (`oauth.js` constants)  
3. kilocode / cline — device/custom flows  
4. cursor — import from vscdb  
5. Proxy wire — `GetActiveToken` in connection resolver  

## ToS

Same as dashboard disclaimer — personal BYO only.
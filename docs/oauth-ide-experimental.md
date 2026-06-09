# OAuth IDE — 9router parity (Go rewrite)

Experimental lab (`LINTASAN_OAUTH_IDE_ENABLED=false` by default).

## Catalog (8 providers)

Mirrors `9router` `OAUTH_PROVIDERS` @ v0.4.71:

| id | flow | implementation |
|----|------|----------------|
| **claude** | PKCE | **ready** |
| **antigravity** | authorization_code | **ready** (env client id/secret) |
| **codex** | PKCE | **ready** |
| **github** | device_code | **ready** |
| **cursor** | import_token | **import_only** |
| **xai** | PKCE | **ready** |
| **kilocode** | custom_device | **ready** |
| **cline** | local_app_callback | **ready** |

Only `implementation=ready` (or cursor import) is actionable from the dashboard.

## Enable

```bash
export LINTASAN_OAUTH_IDE_ENABLED=true
export LINTASAN_OAUTH_PUBLIC_BASE_URL=https://your-lintasan-host
```

**Antigravity:** Google OAuth credentials are not in the repository. Use your own Google Cloud OAuth client or lab copy of 9router `ANTIGRAVITY_CONFIG`:

```bash
export LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID=your-client-id.apps.googleusercontent.com
export LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET=your-client-secret
```

**xAI (Grok):** public client id ported from 9router — redirect `.../api/oauth/callback/xai`.

## Next

- Proxy wire — `GetActiveToken` in connection resolver

## ToS

Personal BYO only — same as dashboard disclaimer.
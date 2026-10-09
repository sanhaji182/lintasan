# OAuth IDE — 9router parity (Go rewrite)

Experimental lab (off by default).

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

**Preferred:** Dashboard → **Settings** → **Experimental** → **OAuth IDE (lab)** → Save. Takes effect immediately (stored as `oauth_ide_enabled` in DB).

**Fallback:** if you never saved that setting, env applies:

```bash
export LINTASAN_OAUTH_IDE_ENABLED=true
export LINTASAN_OAUTH_PUBLIC_BASE_URL=https://your-lintasan-host
```

**Antigravity:** Google OAuth credentials are not in the repository. Use an operator-owned Google Cloud OAuth client when possible. The authorization endpoint fails closed before creating a pending session when either variable is missing; Lintasan never redirects configuration errors to Google.

```bash
export LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_ID=your-client-id.apps.googleusercontent.com
export LINTASAN_OAUTH_IDE_ANTIGRAVITY_CLIENT_SECRET=your-client-secret
```

The configured Google client must register the exact callback `${LINTASAN_OAUTH_PUBLIC_BASE_URL}/api/oauth/callback/antigravity`. CLIProxyAPI uses a desktop installed-app client with `http://localhost:51121/oauth-callback`; do not reuse those public source credentials with Lintasan's HTTPS callback unless the Google client explicitly accepts it. If compatibility testing requires that published client, keep it in a root-readable systemd EnvironmentFile outside this repository and review the shared-client/account-risk implications first. Reference behavior was checked against CLIProxyAPI commit `67465884ca179a8f9098328d03a361b50003fdd7`.

**xAI (Grok):** public client id ported from 9router — redirect must be **`http://127.0.0.1:56121/callback`** (loopback; not your public URL). Lintasan opens a listener on the server when you authorize.

## Next

- Proxy wire — set `oauth_provider` on a connection (dashboard PATCH) to one of the 8 catalog ids; proxy uses active `oauth_sessions` when OAuth IDE is enabled. GitHub uses `flow_meta.copilot.token`.

## ToS

Personal BYO only — same as dashboard disclaimer.
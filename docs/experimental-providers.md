# Experimental Providers

Experimental Providers are ACP-based coding agents (Codex, Claude Code, Gemini CLI, Copilot) managed through Lintasan's Generic Provider Framework. They operate in a separate track from Official providers and are never accessible via default routing.

## Overview

- **Track:** Experimental (isolated from Official routing)
- **Protocol:** ACP (Agent Communication Protocol) over stdio
- **Access:** Explicit opt-in only (`experimental/<name>` model prefix or `X-Lintasan-Track: experimental` header)
- **Membrane:** Experimental providers can NEVER be selected by production/auto/smart routing

## Lifecycle

```
Proposed → Admitted → Active → Deprecated → Retired
```

- **Proposed:** Named in the catalog, not yet pursued
- **Admitted:** Passed admission harness (isolation + protocol gates)
- **Active:** Live behind the experimental flag, opt-in only
- **Deprecated:** Flagged for removal
- **Retired:** Removed, can be re-proposed

## Credential Configuration

### Priority Order

Credentials are resolved in this order:

1. **Dashboard credential** (encrypted in DB) — highest priority
2. **Environment variable** — fallback
3. **Missing** — no credential available

### Setting Credentials via Dashboard

1. Navigate to **Dashboard → Experimental Providers**
2. Find the provider you want to configure
3. Click **Set Credential** (or **Update** if already configured)
4. Enter the API key/token
5. Click **Save**

The credential is stored encrypted (AES-256-GCM) and never displayed in full. Only a masked version is shown (e.g., `sk-abc****xyz`).

### Credential Sources

| Source | Description |
|--------|-------------|
| `dashboard` | Set via the dashboard UI, stored encrypted in DB |
| `environment` | Read from the server's environment variable |
| `none` | No credential configured |

### Removing Credentials

Click the trash icon next to a dashboard-stored credential to remove it. The system will fall back to the environment variable if one is set.

## Provider Setup

### Codex

- **Binary:** `codex-acp` (npm: `@anthropic/codex-acp`)
- **Auth Env Var:** `OPENAI_API_KEY`
- **Auth Method:** `openai-api-key`
- **Capabilities:** coding, tool_calling, reasoning, streaming

```bash
# Install binary
npm install -g @anthropic/codex-acp

# Set credential via dashboard OR environment
# Dashboard: Experimental Providers → Codex → Set Credential
# Environment: add OPENAI_API_KEY to systemd service
```

### Claude Code

- **Binary:** `cc-acp` (npm: `claude-code-acp`)
- **Auth Env Var:** `ANTHROPIC_API_KEY`
- **Auth Method:** `claude-code-subscription`
- **Capabilities:** coding, tool_calling, reasoning, streaming

```bash
# Install binary
npm install -g claude-code-acp

# Set credential via dashboard OR environment
```

### Gemini CLI

- **Binary:** `gemini` (@google/gemini-cli)
- **Auth Env Var:** `GEMINI_API_KEY`
- **Auth Method:** `gemini-api-key`
- **Capabilities:** coding, tool_calling, reasoning, streaming
- **Note:** Key stored in system keychain in ACP mode; env var is for G4 injection

```bash
# Install binary
npm install -g @google/gemini-cli

# Set credential via dashboard OR environment
```

### Copilot

- **Binary:** `copilot` (@github/copilot)
- **Auth Env Var:** `COPILOT_GITHUB_TOKEN`
- **Auth Method:** `copilot-login`
- **Capabilities:** coding, tool_calling, streaming
- **Note:** Requires fine-grained PAT with "Copilot Requests" permission

```bash
# Install binary
npm install -g @github/copilot

# Set credential via dashboard OR environment
# Token: fine-grained PAT (not classic ghp_ tokens)
```

## Hoplite Cloud Agent

Hoplite is integrated as a REST-based **Cloud Agent**, not as an ACP provider and not as an OpenAI-compatible model connection. It is always isolated from Official routing, smart routing, fallbacks, and `/v1/chat/completions`.

- **Dashboard:** Experimental Providers → Hoplite
- **Credential:** encrypted dashboard credential `hoplite`, with `HOPLITE_API_KEY` environment fallback
- **Operations:** persistent connection diagnostics, list projects, create/list/read threads, read messages and resulting pull requests
- **Models:** Hoplite has no model-catalog endpoint. Lintasan shows the project's `defaultModel` when available and accepts an exact custom model ID; custom IDs are validated only by creating a real thread.
- **Agent test:** explicitly creates a real thread, may consume quota, and polls status/messages. The fixed prompt asks for read-only inspection, while `autoFix=false` and `autoMerge=false`; Hoplite exposes no dry-run guarantee.
- **Safe defaults:** `autoFix=false`, `autoMerge=false`; thread creation is always an explicit operator action
- **Upstream:** `https://api.hoplite.sh`

Management routes:

```
GET  /api/experimental/cloud-agents/hoplite/status
POST /api/experimental/cloud-agents/hoplite/test
GET  /api/experimental/cloud-agents/hoplite/projects
GET  /api/experimental/cloud-agents/hoplite/threads?projectId=...
POST /api/experimental/cloud-agents/hoplite/threads
GET  /api/experimental/cloud-agents/hoplite/threads/{id}
GET  /api/experimental/cloud-agents/hoplite/threads/{id}/messages
```

All routes inherit the dashboard's fail-closed authentication boundary. The Hoplite key is resolved and attached server-side and is never returned to browser code.

## Activation Flow

### From Dashboard (recommended)

1. **Configure credential** → Experimental Providers → Set Credential
2. **Admit** → Click "Admit" button (runs admission harness)
3. **Activate** → Click "Activate" button (transitions to active state)
4. **Use** → Send requests with `model=experimental/<name>`

### API Endpoints

```
GET    /api/experimental/providers              — list all providers + state
GET    /api/experimental/providers/{name}       — detail one provider
POST   /api/experimental/providers/{name}/admit — run admission
POST   /api/experimental/providers/{name}/activate   — admitted → active
POST   /api/experimental/providers/{name}/deactivate — active → deprecated

GET    /api/experimental/credentials            — credential status (all)
GET    /api/experimental/credentials/{name}     — credential status (one)
PUT    /api/experimental/credentials/{name}     — set credential
DELETE /api/experimental/credentials/{name}     — remove credential
```

### Making Requests

```bash
# Via model prefix (recommended)
curl -X POST http://localhost:20180/v1/chat/completions \
  -H "Authorization: Bearer <your-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"experimental/codex","messages":[{"role":"user","content":"hello"}]}'

# Via headers
curl -X POST http://localhost:20180/v1/chat/completions \
  -H "Authorization: Bearer <your-key>" \
  -H "Content-Type: application/json" \
  -H "X-Lintasan-Track: experimental" \
  -H "X-Lintasan-Provider: codex" \
  -d '{"model":"any","messages":[{"role":"user","content":"hello"}]}'
```

## Troubleshooting

### "credential not available" (412)

No credential found for the provider. Solutions:
- Set credential via Dashboard → Experimental Providers → Set Credential
- Or set the environment variable in the systemd service

### "experimental provider not found or not active" (404)

The provider is not registered in the runtime registry. Causes:
- Provider not yet admitted/activated
- Server restarted but provider state is not "active" in DB

### "execution failed: no credential available" (502)

Credential was available at admission time but is now missing. Solutions:
- Re-set the credential via dashboard
- Check if the environment variable was removed

### "execution failed: exec: codex-acp: not found" (502)

The ACP binary is not installed or not on PATH. Solutions:
- Install the binary (see Provider Setup above)
- Ensure it's on the system PATH accessible to the lintasan service

### Official routing affected?

Never. Experimental providers are structurally isolated:
- They never appear in `/v1/models`
- `ResolveRoutable()` can never return them
- The membrane is enforced at compile time via source-scan tests

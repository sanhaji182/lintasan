# Hoplite Cloud Agent Integration Design

## Decision

Integrate Hoplite as an Experimental **Cloud Agent**, not as an OpenAI-compatible connection or ACP subprocess. It remains structurally outside Lintasan official model routing, smart routing, fallback, and `/v1/chat/completions`.

## Architecture

```text
Dashboard / Experimental
  -> authenticated Lintasan management routes
  -> encrypted experimental credential store (`hoplite` / `HOPLITE_API_KEY`)
  -> bounded Go REST client
  -> https://api.hoplite.sh
```

The browser never receives the Hoplite API key. Lintasan resolves a dashboard-encrypted key first and falls back to `HOPLITE_API_KEY`.

## Scope

- Credential status, set, update, and delete through the existing encrypted store.
- Read-only connection test and project discovery.
- Project-scoped thread listing.
- Explicit thread creation with idempotency key.
- Thread detail and message retrieval for polling.
- PR links surfaced from durable Hoplite thread state.
- `autoFix` and `autoMerge` default to false and remain explicit operator choices.

## API surface

- `GET /api/experimental/cloud-agents/hoplite/status`
- `POST /api/experimental/cloud-agents/hoplite/test`
- `GET /api/experimental/cloud-agents/hoplite/projects`
- `GET /api/experimental/cloud-agents/hoplite/threads?projectId=...`
- `POST /api/experimental/cloud-agents/hoplite/threads`
- `GET /api/experimental/cloud-agents/hoplite/threads/{id}`
- `GET /api/experimental/cloud-agents/hoplite/threads/{id}/messages`

All routes inherit Lintasan's fail-closed authentication middleware.

## Failure handling

- Missing credential: `412 Precondition Failed` without contacting Hoplite.
- Invalid input: `400 Bad Request` before upstream access.
- Hoplite errors preserve the upstream status when safe and include `x-request-id` / `Retry-After` metadata, never credentials or request authorization headers.
- Client timeouts are bounded.

## Verification

- Client contract tests with `httptest.Server` verify headers, paths, payloads, response parsing, and error sanitization.
- Full middleware tests verify anonymous requests are rejected.
- Handler integration tests use a fake Hoplite server and encrypted test credential.
- Frontend contract checks plus Svelte type-check/build.
- No production deploy without a separate explicit approval.

## Sources

- Hoplite API overview: https://hoplite.sh/docs/api
- Authentication and rate limits: https://hoplite.sh/docs/api/authentication
- List projects: https://hoplite.sh/docs/api/listProjects
- Create thread: https://hoplite.sh/docs/api/createThread
- Health endpoint: https://hoplite.sh/docs/api/getHealth

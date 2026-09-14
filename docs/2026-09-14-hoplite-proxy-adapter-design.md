# Hoplite OpenAI Proxy Adapter Design

Date: 2026-09-14

## Decision

Hoplite is integrated as an explicit coding-agent adapter, not represented as a native low-latency inference provider. The public API documents projects, threads, messages, and repository operations; it does not document `/v1/models` or `/v1/chat/completions`.

Configured projects are exposed as `hoplite-agent/<project-id>`. These IDs are generated only after an authenticated `GET /api/projects` succeeds. No Hoplite-native model catalogue is fabricated.

## Request flow

1. Lintasan accepts an authenticated, non-streaming OpenAI chat-completion request whose model is `hoplite-agent/<project-id>`.
2. The adapter extracts the final non-empty user message and creates one Hoplite thread using `POST /api/threads`.
3. `autoFix` and `autoMerge` are always false. A caller `Idempotency-Key` is passed as `clientOperationId`; oversized keys are deterministically SHA-256-normalized. Without a key, Lintasan creates one.
4. The adapter polls `GET /api/threads/{id}` every two seconds for at most four minutes. The inbound request context cancels upstream calls when the client disconnects.
5. On `ready`/`succeeded`, it reads durable messages and returns the latest assistant `chat` message in an OpenAI response. Thinking/tool/status messages are excluded. Thread status and pull-request links are returned under `x_lintasan`.
6. Failed/blocked/cancelled/archived states map deterministically to a 502 OpenAI-shaped error. Timeout maps to 504. Streaming maps to 400 without contacting Hoplite.

Normal model routing remains unchanged because only the reserved `hoplite-agent/` prefix is intercepted.

## Security and limits

- Credentials remain in Lintasan's encrypted server-side credential store (or server environment); the browser receives only masked status.
- All `/v1/*` and management routes retain fail-closed Lintasan authentication.
- Request body: 1 MiB maximum. Existing Hoplite client response bounds remain 10 MiB success / 32 KiB error.
- Upstream authorization data is never included in logs or errors.
- `/v1/models` fails closed for Hoplite entries: no credential or failed project lookup means no adapter models.
- No production data migration is required.

## Dashboard

The Hoplite page remains the configuration surface for encrypted credentials and project selection. It now lists the exact usable adapter model IDs and explains that every request starts a coding-agent thread, is non-streaming, can take minutes, and never enables automatic fixes or merging.

## Verification

- Focused fake-upstream tests cover conditional model advertisement, creation/polling/result mapping, exclusion of thinking messages, safe flags/idempotency, streaming rejection, and terminal failure mapping.
- Existing Hoplite REST handler/client tests remain authoritative for credential encryption, bounded bodies, auth boundary, and error sanitization.
- Fresh full Go tests, Svelte check, frontend build, and full embedded binary build are required before deployment.

## Official sources

- API overview and supported resources: https://hoplite.sh/docs/api
- Authentication, server-side key handling, permissions, and rate limits: https://hoplite.sh/docs/api/authentication
- Create thread and idempotency semantics: https://hoplite.sh/docs/api/createThread
- Durable thread state: https://hoplite.sh/docs/api/getThread
- Durable message history and message kinds: https://hoplite.sh/docs/api/listThreadMessages

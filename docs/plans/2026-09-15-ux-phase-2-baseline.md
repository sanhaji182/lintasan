# UX Phase 2 implementation baseline

Date: 2026-09-15
Scope: Models, Playground, Connections, Routing, and observability
Source audit: Kanban task `t_988747d8`, attached `report.md`
Prerequisite: Phase 1 task `t_809fca21` is complete and independently approved.

This document is the contract map for Phase 2 follow-on work. It describes the repository state at the time of this audit; it does not authorize production mutations.

## Code map

### Frontend routes and shared components

- `/dashboard/models`: `frontend/src/routes/dashboard/models/+page.svelte`
  - Builds a client-side callable inventory from `/v1/models`, `/api/combos`, `/api/aliases`, and `/api/connections`.
  - Supports text/type filtering, Copy ID, safe per-connection model test, and a Playground link.
- `/dashboard/playground`: `frontend/src/routes/dashboard/playground/+page.svelte`
  - Uses `frontend/src/lib/components/ModelCombobox.svelte` and the same callable-catalog helper.
  - Persists `lintasan.lastModel` and `lintasan.recentModels` in browser `localStorage` only.
  - Uses non-streaming requests when the selected catalog row reports `supportsStreaming === false` or is a Cloud Agent.
- `/dashboard/connections`: `frontend/src/routes/dashboard/connections/+page.svelte`
  - Configured provider accounts and virtual Cloud Agent accounts, compact provider groups, filters, explicit bulk mode, contextual Add menu, per-account actions, model discovery/testing, balance/limit display.
- `/dashboard/routing`: `frontend/src/routes/dashboard/routing/+page.svelte`
  - Sections: Policies, Combos, Quotas.
  - Stages policy, load-balancer, quota, combo-strategy, and combo-order changes locally; only explicit Save actions call mutation APIs.
- `/dashboard/analytics`: `frontend/src/routes/dashboard/analytics/+page.svelte`
  - Combines global dashboard counters with a retained `/api/logs` snapshot, labels the mismatch, records retrieval time, and separately reports SSE connection/update freshness.
- `/dashboard/observability`: `frontend/src/routes/dashboard/observability/+page.svelte`
  - Observability landing/navigation surface.
- `/dashboard/usage` and `/dashboard/logs`: `frontend/src/routes/dashboard/usage/+page.svelte` and `frontend/src/routes/dashboard/logs/+page.svelte`.
- Navigation registry: `frontend/src/lib/navigation.ts`; rendered by `frontend/src/lib/components/Sidebar.svelte`.
- Shared Phase 2 derivation and save-scope helpers: `frontend/src/lib/workflow-consolidation.ts`.

### Backend routes and implementations

Registration starts in `internal/server/server.go`; REST-style patch/reorder routes are additionally registered by `internal/server/handlers_rest.go`.

Read contracts:

- `GET /v1/models` -> `internal/server/handlers.go:handleModels`
- `GET /api/models/catalog` -> embedded static provider catalog with pricing/capabilities
- `GET /api/connections` -> `internal/server/handlers.go:handleGetConnections`
- `GET /api/combos` -> JSON setting `combos`
- `GET /api/aliases` -> JSON setting `aliases`
- `GET /api/load-balancer` -> setting `lb_strategy`
- `GET /api/smart-routing` -> scalar smart-routing settings plus JSON `quota_limits`
- `GET /api/dashboard/stats`, `GET /api/logs`, `GET /api/analytics`, `GET /api/usage`, `GET /api/analytics/stream` -> dashboard/observability data

Mutation contracts used by these screens:

- Connections: `POST/PATCH/DELETE /api/connections`, bulk enable/disable/delete/test, model sync/toggle/test endpoints, plus virtual-provider dispatch in the Hoplite handlers.
- Combos: `POST/PUT/DELETE /api/combos`; `PATCH /api/routing/combos/{id}`; `PUT /api/routing/combos/reorder`.
- Aliases: `POST /api/routing/aliases`; `DELETE /api/routing/aliases/{id}` (legacy whole-map endpoints also exist).
- Policies: `POST /api/load-balancer` and `POST /api/smart-routing` with policy-only fields.
- Quotas: `POST /api/smart-routing` with only `quota_limits`.
- Model test: `POST /api/models/test`; this is an explicit upstream probe, not a passive health read.

All `/api/*` and `/v1/*` routes are behind fail-closed auth middleware. An anonymous 401 proves the auth boundary, not route existence or endpoint behavior.

## Authoritative data and callable IDs

### Source precedence

1. **What is callable now:** `GET /v1/models` is authoritative for active advertised provider and Cloud Agent model IDs. For ordinary providers it reads active `discovered_models` joined to active `connections`. Only when there are no active discovered rows at all does it fall back to the embedded catalog.
2. **Configured aliases and combos:** `/api/aliases` and `/api/combos` are authoritative for route names. Their persisted values are JSON in the `settings` table.
3. **Connection/account identity and state:** `/api/connections` is authoritative. Standard accounts come from `connections`; Hoplite accounts are virtual rows backed by encrypted credentials/account state and deliberately expose no plaintext credential.
4. **Reference metadata:** `/api/models/catalog` and `internal/models/catalog.go` are authoritative only for the embedded static provider catalog. They are not proof that a model is configured, healthy, or callable on a current account.
5. **Observed health:** connection/Hoplite health and explicit model-test results are observations. `is_active` means enabled, not necessarily recently healthy.

### Callable ID formation

- Discovered provider model: exact `discovered_models.model_id`; `connection_id` is provenance and routing eligibility, not part of the callable ID.
- Alias: exact key in the `aliases` setting.
- Combo: `combo.name` (the frontend currently falls back to `combo.provider` if a legacy row lacks `name`; the persistent object also has an internal `id`).
- Default Hoplite account/project: `hoplite-agent/<projectID>`.
- Additional Hoplite account/project: `hoplite-agent/v2/<encodedAccountID>/<encodedProjectID>`.
- Default Hoplite account/project/model: `hoplite-model/v1/<encodedProjectID>/<encodedModelID>`.
- Additional Hoplite account/project/model: account-qualified `hoplite-model/v2/...` form produced by `hopliteSelectedModelIDForAccount`.

Treat all IDs as opaque, exact strings. Never reconstruct account-qualified IDs in the frontend; consume `/v1/models` or the Hoplite discovery APIs.

### Metadata that may be shown

Only show metadata present in its authoritative response:

- `/v1/models` ordinary discovered rows: `id`, `owned_by`, `connection_id`, `source=discovered`.
- `/v1/models` embedded fallback rows: `id`, `owned_by`, `source=catalog`.
- `/v1/models` Cloud Agent rows additionally report display/project/account/model/provider identity, context window, catalog eligibility/revision, `provider_kind=cloud_agent`, `supports_streaming=false`, `long_running=true`, and `project_scoped=true`.
- `/api/models/catalog`: reference model name, provider, context window, max tokens, input/output price, and capabilities for hardcoded catalog entries.
- `/api/connections`: account identity, enabled state, priority, format, masked credential representation, model count, pool, and Cloud Agent capability flags.

### Unsupported/non-authoritative metadata — do not fabricate

For ordinary discovered rows, `/v1/models` does **not** report model health, capabilities, context window, pricing, max output tokens, or streaming support. `/api/connections` does not make `is_active` equivalent to healthy. Static catalog metadata must not be attached to an arbitrary discovered row merely because its model ID text matches. A missing value must render as `Not reported`/unknown.

The Phase 2 helper currently follows this rule for context, price, capabilities, and health. Note the schema spelling: Cloud Agent responses expose `context_window_tokens`, while the helper currently reads `context_window`; downstream work must normalize this deliberately rather than silently inventing a value.

## Persistence and save semantics

- Standard connections and discovered models are SQLite rows (`connections`, `discovered_models`). Connection PATCH is field-selective. Mask placeholders must never overwrite stored credentials.
- Hoplite Cloud Agent accounts are virtual connections backed by encrypted credential/account storage; they are dispatched through dedicated handlers, not treated as plaintext standard connection rows.
- Combos and aliases are JSON documents in the SQLite `settings` table. Combo writes reload the in-memory combo router after persistence.
- `lb_strategy` and smart-routing scalar values are individual settings. `quota_limits` is a JSON setting. Smart-routing writes live-reload the proxy configuration.
- Routing UI save scopes are intentionally separate:
  - Policies: load-balancer strategy plus only ML/cost policy fields.
  - Combos: staged strategy patches, then reorder only if every strategy patch succeeded.
  - Quotas: only `quota_limits`.
- Alias create/delete remains an explicit immediate action, separate from the three scoped Save controls.
- Playground recent/last selections are local browser preferences, not server configuration.
- Model catalog Copy and Playground links are read-only. `POST /api/models/test` performs an upstream probe and must remain an explicit user action.

## Cloud Agent constraint

Any combo containing a Cloud Agent target must use `strategy = "priority"`. This is enforced both in UI controls and server validation (`internal/server/cloud_agent_routing.go`) for create/update/patch. Cloud Agent calls are long-running, project-scoped, and non-streaming. The restriction prevents duplicate jobs from round-robin/random/latency strategies and preserves failover semantics: fallback is allowed only before a job is known to be accepted; ambiguous or post-acceptance failures must not launch a duplicate fallback job.

## Current `/dashboard/models` behavior

The route exists and is served by the embedded SPA rather than returning the audit's former bare 404. It builds one deduplicated client-side list in this order: aliases, combos, Cloud Agent rows, ordinary provider rows. Duplicate IDs keep the first row, so configured route names win over raw provider rows with the same ID. The page supports fuzzy search, type filters, Copy ID, a safe explicit test when a concrete connection is known, and a link to Playground with `?model=<exact ID>`.

Compatibility caution: because deduplication is by callable ID alone, same-text IDs from multiple ordinary connections collapse to one display row even though backend routing may have several eligible connections. This is suitable for answering “what string can I call?” but not for a complete per-account model matrix.

## Tests relevant to this baseline

Frontend:

- `frontend/tests/ux-phase-2.test.mjs`: catalog dedupe/truthfulness/search/grouping, Cloud Agent streaming, routing payload isolation/dirty state, observability reconciliation.
- `frontend/tests/model-combobox.test.ts`: real Svelte DOM tests for Enter/Space/mouse opening, focus transfer, and immediate search filtering.
- `frontend/tests/ux-phase-1.test.mjs`: navigation/Quickstart/landing regressions.
- `frontend/tests/hoplite-model-id.test.mjs`: Cloud Agent ID handling.

Backend:

- `internal/server/models_quickstart_test.go`: stable `/v1/models` discovered provenance.
- `internal/server/hoplite_connection_combo_test.go`, `hoplite_accounts*_test.go`, `model_test_hoplite_test.go`, `cloud_agent_review_regression_test.go`: virtual connection/model IDs, masking, priority-only validation, exact account dispatch, and duplicate-job-safe fallback.
- `internal/server/handlers_rest_test.go`: combo patch/reorder, alias persistence, model activation and sync contracts.
- `internal/server/analytics_stream_test.go`: SSE transport contract.
- `internal/server/dashboard_nav_test.go`, `public_ui_path_test.go`, `security_boundary_test.go`: route/nav and auth boundaries.
- `internal/server/proxy_auth_failover_test.go`: deterministic priority/failover behavior.

Canonical verification commands:

```bash
cd /home/ubuntu/lintasan-go/frontend
npm test
npm run check
npm run build

cd /home/ubuntu/lintasan-go
go test ./...
git diff --check
```

`make build` is the production-form single-binary build (frontend static output plus Go embed) but is not needed to validate this documentation-only baseline. It writes `dist-bin/lintasan`; it must never overwrite the live `./lintasan` binary. Production deploy/restart requires separate explicit approval.

## Compatibility and regression risks

- Do not treat the embedded static catalog as configured availability; `/v1/models` fallback behavior can otherwise make unavailable reference models look callable on an empty install.
- Preserve exact aliases/combo names and opaque Cloud Agent IDs; normalization or prefix stripping can route to the wrong account/project.
- Keep Cloud Agent and Cloud Agent-containing combos non-streaming and priority-only across Models and Playground.
- Preserve per-account routing even when the Models page deduplicates the user-facing callable string.
- Do not merge policy and quota payloads: the server's partial-update behavior is useful, but a broad frontend payload can overwrite another scope.
- Combo save ordering is fail-closed: do not reorder after a strategy PATCH failure.
- `POST /api/models/test` may contact an upstream; never run it as passive page-load health discovery.
- Keep credential masking and virtual-provider dispatch intact when compacting Connections.
- Global counters, retained request-log snapshots, and SSE freshness are different scopes; differences must be explained rather than forced to reconcile.
- Existing source-contract tests can become stale after refactors; behavioral DOM/API tests are the acceptance evidence.

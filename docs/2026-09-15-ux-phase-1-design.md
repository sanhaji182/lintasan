# UX Phase 1 — Quickstart and Simplified Navigation Design

Status: approved via parent UX audit `t_988747d8` and implementation card `t_809fca21`.

## Goal

Give a first-time API developer one truthful path from provider connection to a tested OpenAI-compatible request, while preserving every existing route and operator capability.

## Approach

1. Add pure frontend domain helpers for Quickstart readiness, recommended callable-model selection, secret masking, snippets, intent-based navigation, and landing metric state. Components consume these helpers so behavior is directly unit-tested without brittle source-marker assertions.
2. Add `/dashboard/quickstart` as a five-step flow. It reads `/api/connections`, `/api/keys`, and `/v1/models`; it never invents values. Existing secrets are masked, newly created keys are shown only from the create response, and empty/error states point to the owning page.
3. Keep Overview, Quickstart, Playground, Gateway, and Observability visible. Gateway is an inline group of existing routes; Manage and Advanced are collapsed by default but auto-open when the active route is inside them. No Models link is introduced before Phase 2.
4. Replace public animated placeholder numbers with verified public health/catalog values when available. While loading, render skeletons; when unavailable, render nonnumeric proof points rather than `0+` or fabricated counts.
5. Add a keyboard-visible skip link and global focus-visible treatment. Verify desktop and mobile layouts in a real browser viewport.

## Data and safety

- Public base URL is derived from `window.location.origin` plus `/v1`.
- A connection is ready only when the API reports it active and not unhealthy.
- A callable model is selected only from `/v1/models`; aliases/combos are already represented by that gateway contract when callable.
- Existing API key values are never displayed or copied from list responses. Only a masked prefix/status is shown. Creating a key is an explicit mutation initiated by the user.
- Live QA remains read-only: no provider, credential, or routing mutations.

## Error handling

Each source has an explicit unavailable state. Partial success remains useful: for example, endpoint/snippets can render while provider or key steps remain incomplete. Snippets use placeholders until a freshly created key is available.

## Tests

Node behavioral tests exercise pure data transformations and route/group invariants. Svelte checks and production build validate components. Go route/contracts and `go test ./...` guard embedded SPA and auth boundaries. Browser QA covers authenticated Quickstart/sidebar at desktop/mobile plus unauthenticated landing and protected API behavior.

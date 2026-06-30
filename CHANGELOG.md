# Changelog

All notable changes to Lintasan Go are documented here. The format is loosely
based on [Keep a Changelog](https://keepachangelog.com/), and this project uses
semantic-ish versioning.

> **Versioning note (2026-06-05):** Lintasan is now numbered as **0.x.x** (pre-1.0)
> to honestly reflect that the project is still in active development — the API
> and on-disk formats may still change. The previous `v2.4.0` tag remains in
> history as a reference point; `v0.24.0` is the first release of the new
> numbering scheme (the `.24` keeps continuity with the prior work).

## [0.26.1] — 2026-06-30

### Changed
- **Savings page redesign:** migrated from hardcoded dark-theme Tailwind to
  design system CSS variables — 4-column summary grid, breakdown cards with
  icons, proper table styling matching the rest of the dashboard.

### Tests
- 5 new connection pool CRUD tests (create, multi-pool, patch, delete, empty state).

### Housekeeping
- `.gitignore`: added coverage reports, dev utility commands, and dist-bin.

## [0.26.0] — 2026-06-30

### Added
- **Connection Pool Management with Multi-Account Load Balancing.** Group
  multiple API keys under a `pool_id` to distribute requests across accounts,
  avoiding per-key rate limits. Pool health stats (success rate, availability,
  rate-limited count) are visible in the dashboard Connections page.
- **`GET /api/connections/pools`** — new endpoint returning pool health stats
  augmented with runtime data from in-memory load balancers.
- **Pool-aware proxy routing:** `pickMultiAccountAPIKey()` rotates API keys per
  request; `dedupCandidatesByPool()` ensures only one connection per pool is
  considered during route resolution; `recordMultiAccountResult()` tracks
  success/failure per account for health scoring.
- **DB migration:** `pool_id` column + index on connections table.

### Changed
- **Connections page UX:** collapsible sections for OAuth IDE and Provider
  Presets (click to expand/collapse), 4-column summary grid (Total/Active/
  Formats/Pools), pool health cards with color-coded availability indicators.

### Tests
- All 44 packages PASS (937+ tests).

## [0.25.0] — 2026-06-23

### Added
- **Experimental IDE OAuth (off by default).** A new `/api/oauth/ide/*` subsystem
  that lets AI coding agents (Claude Code, Codex CLI, Copilot, Cursor, Cline,
  Kilocode, Antigravity, xAI) authenticate via OAuth 2.0 PKCE or device flow —
  no manual API key management. Gated behind `--oauth-ide-enable=false` and
  admin-only dashboard controls.
- **8-provider OAuth IDE catalog** (`internal/oauthide/`): Claude (PKCE), Codex
  (PKCE), GitHub Copilot (device flow), Cursor (device flow), Cline (PKCE),
  Kilocode (PKCE), Antigravity (PKCE), xAI (PKCE+device flow).
- **9-router port:** the IDE OAuth layer reuses the 9router routing substrate,
  inheriting its multi-provider connection management infrastructure.
- **Dashboard OAuth IDE page** (`/dashboard/oauth-ide`): status card per IDE,
  self-service toggle, credentials display, connection health check.
- **QA documentation:** `docs/qa-test-plan.md` (336 lines) and
  `docs/qa-test-report.md` (277 lines) covering the beta-readiness P0+P1 batch
  and the OAuth IDE experimental feature.

### Changed
- **Repo renamed** from `lintasan-go` → `lintasan` across all docs, badges,
  and remote URLs.
- **README badges** updated: version → v0.25.0, test count → 900+
- **AGENTS.md** reconciled to reflect the repo rename, removed stale binary
  references.

### Housekeeping
- Removed stale binary artifacts (`lintasan-go.bak`, `lintasan-go2`) from repo
  root.
- `feat/oauth-ide-experimental` branch tracking the experimental work kept as
  active feature branch.

### Tests
- 3 new test files: `internal/oauthide/catalog_test.go`,
  `internal/oauthide/github_test.go`, `internal/server/oauth_ide_test.go`.
- Suite: **43/43 packages PASS**, `go vet` clean, all 900+ tests pass.

## [0.24.2] — 2026-06-05

### Fixed
- **`handlePluginGenerate` is no longer a stub.** The "AI Generate" tab in
  the Plugins page used to return a hardcoded template regardless of the
  prompt, which was misleading. New behavior: if `plugin_generator_model`
  is unset, return `503` with a clear "set this setting" hint instead of a
  fake template. If set, self-call `/v1/chat/completions` with the master
  key, pass a system prompt that anchors the output to a single
  JavaScript plugin module, and return the model-generated code.
- **`handleCosts` reads real data.** Previously returned hardcoded zeros.
  Now aggregates `request_logs` by model for "today" and "this month" and
  computes cost per row via `cost.NewCalculator()` with the built-in
  pricing table. Returns the full shape (totals, by-model breakdown
  sorted by cost desc, request counts, input/output token counts).
- **Frontend/backend method mismatch on `/api/load-balancer`.** The
  routing page sent `PUT` but the server registered `POST`. Aligned the
  frontend to `POST` (1-line change, no server-side handler added).

### Changed
- **AGENTS.md §11 quick reference.** Removed the stale `admin/admin123`
  login example (the password is randomly generated on first start and
  forced to rotate). Replaced with a recovery-flow comment block.

### Tests
- 10 new unit tests in `internal/server/handlers_beta_p0_test.go`:
  - `TestHandlePluginGenerate_*` (5 cases) — bad JSON, empty prompt, no
    model, test mode (port=0), no master key
  - `TestHandleCosts_*` (2 cases) — empty-DB shape, real aggregation
    from seeded `request_logs` rows
  - `TestStripCodeFences`, `TestSanitizeName`, `TestRound2` — helpers
- Suite: 35/35 packages pass, `go vet` clean.

## [0.24.1] — 2026-06-05

### Added
- **Test coverage for `internal/server/curl_import.go`.** The endpoint
  `POST /api/connections/import-curl` was active in production with zero unit
  tests; the gap is now closed with 28 tests covering `parseCurlCommand`
  (header/body extraction, quoting, multiline), `inferNameFromHost`
  (provider hints, edge cases), `tokenizeCurl` / `stripQuotes` helpers, and
  `handleCurlImport` HTTP error paths (bad JSON, missing URL, unsupported
  method). Commit `84874cf`.

### Fixed
- **Proxy response header forwarding.** The Go `http.Transport` auto-decompresses
  `Content-Encoding: gzip` and resets the response `Content-Length` and
  `Transfer-Encoding` headers on read. The proxy was forwarding the upstream's
  stale `Content-Length` and `Transfer-Encoding: chunked` to clients, which
  caused empty bodies when the client tried to read the declared length.
  Now stripping those two hop-by-hop-style headers at all three forwarding
  sites in `internal/proxy/proxy.go` (chat completions + 2 streaming paths).
  Commit `c030fdd`.

### Housekeeping
- Dropped 30+ merged `feat/*` and `fix/*` branches (lokal + remote).
- Removed two stale unmerged branches: `feat/provider-sdk-foundation`
  (work already in main via F1+F2 chain) and `frontend-t949aa391`
  (kanban worktree orphan, worktree dir removed).
- Dropped `feat/curl-import-connection` (2-ahead branch with noisy base).
  The one substantive fix it contained was re-extracted and merged
  properly to all three call sites (see "Fixed" above).
- 5 intentional branches preserved: `feat/codex-m0-skeleton`,
  `gh-pages` (kept per project convention), plus the working
  `main`.

## [0.24.0] — 2026-06-05

### Changed
- **Versioning scheme reset to 0.x.x.** The first release of the pre-1.0 series
  is `v0.24.0`, continuing from the `v2.4.0` work. Semver signal: API and config
  formats are not yet frozen.

### Notes
- No code or behavior change vs the prior `main @ 6b29d79`. Only version strings,
  docs, and CHANGELOG were updated.

## [2.4.0] — 2026-05-30

### Added
- **Single self-contained binary.** The SvelteKit dashboard is now compiled to a
  static SPA and embedded into the Go binary via `go:embed`. `lintasan start`
  serves the full UI **and** the API from one executable on `:20180` — no
  separate Node process, no nginx required for a basic deployment.
- `make build` orchestrates the full build (frontend → embed → binary).
- Multi-stage `Dockerfile` (builds frontend + backend) and `docker-compose.yml`
  so `docker compose up --build` produces a single working container.
- Pre-built `lintasan-linux-amd64` binary attached to the release.

### Changed
- Frontend switched from `adapter-node` to `adapter-static` (pure client-rendered
  SPA; the dashboard already ran with `ssr=false` everywhere).
- `authMiddleware` now serves the embedded SPA + static assets publicly (no
  secrets there) while keeping every `/api`, `/v1`, and `/mcp` endpoint gated.
  Guarded by an explicit allowlist (`isPublicUIPath`) with dedicated tests.
- README Quick Start / Installation rewritten to match reality (the old binary
  download URL 404'd and the Docker instructions referenced a compose file that
  didn't build the frontend).

### Notes
- Building from source now requires **Node 20+** in addition to **Go 1.22+** to
  compile the dashboard. `go build` alone still works but yields an API-only
  server (UI reports as unavailable).

## [2.3.7] — 2026-05-30

### Fixed
- Registered 23 RESTful dashboard routes (`DELETE /api/keys/{id}`,
  `PATCH /api/plugins/{id}`, etc) that were never wired into the mux. Go 1.22's
  strict ServeMux made every edit/delete/toggle button silently return 405.
- Three pre-existing no-op stubs now persist: team delete, team member add, and
  webhook create (the form posts `{url, events}` with no `action` field).

## [2.3.6] — 2026-05-30

### Fixed
- User Management full parity: list rendering (response-shape mismatch), real
  create/delete/role-change, admin password reset, self-service change-password,
  and `must_change_password` now surfaced by `ListUsers`. Added last-admin guards.

## [2.3.5] — 2026-05-30

### Fixed
- Login UX: a wrong password now surfaces "invalid credentials" instead of the
  misleading "Session expired" message.

## [2.3.4] — 2026-05-30

### Fixed
- Unified auth transport (raw `fetch` → `api.*` wrapper) to eliminate a
  split-brain where some requests carried the JWT and others didn't.

## [2.3.3] — 2026-05-30

### Fixed
- 403 `password_change_required` handling and secret-masking hardening.

## [2.3.1] — 2026-05-30

### Added
- Security & Reliability release: fail-closed auth, bootstrap/active state
  machine, first-run setup redesign.

[0.25.0]: https://github.com/sanhaji182/lintasan/releases/tag/v0.25.0
[0.24.0]: https://github.com/sanhaji182/lintasan/releases/tag/v0.24.0
[2.4.0]: https://github.com/sanhaji182/lintasan/releases/tag/v2.4.0
[2.3.7]: https://github.com/sanhaji182/lintasan-go/releases/tag/v2.3.7
[2.3.6]: https://github.com/sanhaji182/lintasan-go/releases/tag/v2.3.6
[2.3.5]: https://github.com/sanhaji182/lintasan-go/releases/tag/v2.3.5
[2.3.4]: https://github.com/sanhaji182/lintasan-go/releases/tag/v2.3.4
[2.3.3]: https://github.com/sanhaji182/lintasan-go/releases/tag/v2.3.3
[2.3.1]: https://github.com/sanhaji182/lintasan-go/releases/tag/v2.3.1

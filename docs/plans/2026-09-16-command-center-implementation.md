# Lintasan Command Center Phase 1 Implementation Plan

> **For Hermes:** Execute this plan task-by-task with strict red-green-refactor cycles.

**Goal:** Deliver the approved Command Center shell, truthful Overview, and Connections hierarchy baseline without changing backend contracts.

**Architecture:** Keep route files and API boundaries intact. Centralize navigation/search behavior in typed library functions, isolate overview source-state derivation in a pure module, and compose the Svelte shell from Sidebar, Header, and a new CommandPalette component. Apply shared visual behavior through app tokens and component-scoped responsive CSS.

**Tech Stack:** SvelteKit 5, TypeScript, Svelte runes, Testing Library/Vitest, Node test runner, Lucide Svelte, Go HTTP server.

---

### Task 1: Lock the intent navigation contract

**Files:**
- Modify: `frontend/tests/ux-phase-1.test.mjs`
- Modify: `frontend/src/lib/navigation.ts`

**Steps:**
1. Change the test expectation to Operate / Build / Observe / Configure and add assertions for searchable route metadata and related route activation.
2. Run `node --experimental-strip-types --test tests/ux-phase-1.test.mjs`; expect failure on old group labels/functions.
3. Implement the typed intent groups and pure search function while preserving all canonical routes.
4. Re-run the focused test; expect pass.

### Task 2: Build accessible navigation command palette

**Files:**
- Create: `frontend/tests/command-palette.test.ts`
- Create: `frontend/src/lib/components/CommandPalette.svelte`
- Modify: `frontend/src/lib/components/Header.svelte`

**Steps:**
1. Add DOM tests for Ctrl/Cmd+K, slash, input focus, route filtering, Escape, focus restoration, and real href results.
2. Run `npx vitest run tests/command-palette.test.ts`; expect component/import failure.
3. Implement the navigation-only dialog and header trigger using Lucide icons and shared navigation data.
4. Re-run focused tests; expect pass.

### Task 3: Make Overview source truth explicit

**Files:**
- Create: `frontend/src/lib/overview-state.ts`
- Create: `frontend/tests/overview-state.test.mjs`
- Create: `frontend/tests/overview-page.test.ts`
- Modify: `frontend/src/routes/dashboard/+page.svelte`

**Steps:**
1. Add pure tests for independent source states and derived metrics; add DOM tests that reject fabricated zeroes when sources fail.
2. Run focused Node/Vitest tests; expect missing module and old UI failures.
3. Implement source envelopes and redesign Overview around `/health`, `/api/dashboard/stats`, `/api/logs`, and `/api/connections` without fallback fabrication.
4. Re-run focused tests; expect pass.

### Task 4: Apply the Command Center visual system and responsive shell

**Files:**
- Modify: `frontend/src/app.css`
- Modify: `frontend/src/lib/components/Sidebar.svelte`
- Modify: `frontend/src/lib/components/Header.svelte`
- Modify: `frontend/src/routes/dashboard/+layout.svelte`
- Modify: `frontend/tests/command-palette.test.ts`

**Steps:**
1. Add DOM/static assertions for labels, landmarks, touch-safe controls, and reduced-motion hooks.
2. Run focused tests; expect failure.
3. Implement light/dark tokens, sidebar/header hierarchy, width containment, focus, reduced motion, and mobile behavior.
4. Run focused tests and `npm run check`; expect pass.

### Task 5: Align Connections with canonical lifecycle hierarchy

**Files:**
- Modify: `frontend/tests/connections-workflow.test.ts`
- Modify: `frontend/src/routes/dashboard/connections/+page.svelte`

**Steps:**
1. Add assertions for lifecycle context, a single primary Add action, truthful API-derived summaries, and collapsed provider details.
2. Run `npx vitest run tests/connections-workflow.test.ts`; expect copy/hierarchy failure.
3. Adjust page header, summary styling, progressive disclosure, and responsive controls without changing existing handlers or API calls.
4. Re-run Connections, model Safe-test, and toast diagnostics tests; expect pass.

### Task 6: Verify, browser-QA, and hand off

**Files:**
- Update: `docs/plans/2026-09-16-command-center-design.md` only if implementation decisions differ.

**Steps:**
1. Run `npm test`, `npm run check`, and `npm run build` in `frontend/`.
2. Run relevant Go dashboard/server contract tests, then the full `go test ./...` if time permits.
3. Start a non-production local server/preview, establish a safe local auth session if available, and inspect desktop plus mobile with browser tools. Verify no horizontal shell overflow and inspect console errors.
4. Review `git diff`, commit focused changes on `main`, do not push or deploy.
5. Add structured evidence as a Kanban comment and block with `review-required`.

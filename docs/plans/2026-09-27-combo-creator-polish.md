# Combo Creator UI/UX Polish Implementation Plan

> **For Hermes:** Execute this plan task-by-task with strict red-green-refactor.

**Goal:** Make the Routing Combo creator clearly communicate provider, account, and model identity; expose truthful async/empty states; and remain usable by keyboard and on narrow screens without changing routing payload semantics.

**Architecture:** Keep the existing unified inline Combo form and existing catalog APIs. Add presentation-only helpers to the combo selector module, then upgrade the inline Svelte selector into explicit provider cards, a searchable model list, and a compact pre-save route summary. Persist the exact same `provider_id` or `connection_id` payload shape already shipped.

**Tech Stack:** Svelte 5, TypeScript, lucide-svelte, Vitest, Testing Library, existing CSS design tokens.

---

### Task 1: Provider metadata and truthful availability

**Files:**
- Modify: `frontend/src/lib/combo-entry-selector.ts`
- Test: `frontend/tests/combo-entry-selector.test.mjs`

1. Write failing selector tests for provider model counts, account labels, filtering, and inactive-only catalog state.
2. Run the focused node test and confirm expected failures.
3. Add the smallest pure helpers/metadata needed by the UI.
4. Re-run the focused test and confirm pass.

### Task 2: Accessible searchable Combo picker

**Files:**
- Modify: `frontend/src/routes/dashboard/routing/+page.svelte`
- Test: `frontend/tests/routing-workflow.test.ts`

1. Write failing UI tests for canonical-name hierarchy, explicit selected indicator, no-model/inactive/API-failure states, model search, Escape reset, and pre-save Provider → Account → Model summary.
2. Run the focused Vitest file and confirm expected failures.
3. Replace native provider/model selects with token-based provider cards and a compact searchable model picker; retain the advanced account pin and exact save payload.
4. Ensure loading/sync/create actions disable duplicate submission and announce status.
5. Re-run focused tests and refactor only while green.

### Task 3: Responsive and visual verification

**Files:**
- Modify: `frontend/src/routes/dashboard/routing/+page.svelte`

1. Add responsive CSS with `min-width: 0`, wrapping, no horizontal overflow, visible `:focus-visible`, and 44px interactive targets.
2. Run selector tests, routing tests, full frontend tests, `npm run check`, and production build.
3. Run a local non-production build against a safe copied data/session if authentication can be established without exposing credentials; capture desktop and mobile screenshots. Otherwise record the authenticated-inspection limitation explicitly.
4. Commit the task branch and leave a review-required handoff with exact evidence.

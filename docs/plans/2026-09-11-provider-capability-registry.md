# Provider Capability Registry Implementation Plan

> **For Hermes:** Implement task-by-task with strict RED-GREEN-REFACTOR.

**Goal:** Replace the broad URL-only preset list with a curated, capability-aware catalogue whose metadata drives connection test, save, model sync, and honest usage display.

**Architecture:** Extend the preset schema and Go catalogue with connection-contract fields. Keep one DB-backed API for the dashboard, preserve custom presets, and make frontend form state carry the full selected contract. Usage capability is descriptive and separate from runtime provider errors.

**Tech Stack:** Go, SQLite, SvelteKit 5, TypeScript.

---

### Task 1: Schema and migration

**Files:** `internal/db/db.go`, DB migration tests.

1. Write a failing migration test for new preset columns and legacy-row backfill.
2. Run the focused test and confirm expected missing-column failure.
3. Add additive columns: chat/models paths, auth metadata, extra headers, model/usage capabilities, verification status/date.
4. Rerun focused test.

### Task 2: Typed curated catalogue

**Files:** `internal/server/handlers_presets.go`, catalogue tests.

1. Write failing contracts requiring every curated built-in to have valid model metadata and verification evidence.
2. Add fields to `Preset`; replace unverified primary entries with a conservative curated cohort.
3. Encode provider-specific contracts for Anthropic, Gemini OpenAI-compatible endpoint, and CommandCode Alpha.
4. Update seeding to refresh all built-in metadata while preserving manual rows.
5. Rerun focused tests.

### Task 3: Preset CRUD/API parity

**Files:** `internal/server/handlers_presets.go`, handler tests.

1. Add failing JSON round-trip and CRUD tests for capability fields.
2. Update SELECT/INSERT/UPDATE payloads.
3. Verify API response and manual preset defaults.

### Task 4: Connection test and save parity

**Files:** `frontend/src/routes/dashboard/connections/+page.svelte`, relevant Go handler tests.

1. Add failing source/handler tests proving complete preset metadata reaches test and save.
2. Expand form state and `pickPreset`.
3. Send snake/camel-compatible path/auth/header metadata to test and save.
4. After successful creation, sync models automatically; report sync failure as warning while preserving connection.
5. Verify focused tests.

### Task 5: Capability UX

**Files:** `frontend/src/routes/dashboard/connections/+page.svelte`.

1. Add source-level regression tests for filtering and labels.
2. Show only verified/model-supported built-ins plus manual presets.
3. Render Models and Usage capability badges.
4. Render `Usage not provided by provider` as neutral state rather than failure.
5. Build frontend.

### Task 6: Review and verification

1. Run adversarial fresh-context review of schema compatibility, capability truthfulness, and failure modes.
2. Reconcile valid findings.
3. Run focused tests, `go test ./...`, frontend checks/build, and `git diff --check`.
4. Run isolated binary smoke and browser QA without modifying production.
5. Commit, rebase on `origin/main`, rerun verification, push branch, and submit for review before any production deployment.

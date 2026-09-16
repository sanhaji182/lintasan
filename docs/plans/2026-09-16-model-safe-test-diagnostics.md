# Model Safe-Test Diagnostics Implementation Plan

> **For Hermes:** Execute directly with strict RED→GREEN→REFACTOR cycles.

**Goal:** Make `/dashboard/models` Safe test failures concise, actionable, and safe for both HTTP-200 `success:false` responses and thrown non-2xx API envelopes.

**Architecture:** Add a small pure formatter/sanitizer in `frontend/src/lib/model-test-result.ts`, then let the Models page render its structured output and emit a rich error toast. Keep raw upstream payloads out of the DOM: strip markup/control characters, redact credential-shaped data, normalize whitespace, and cap detail length.

**Tech Stack:** SvelteKit 5, TypeScript, Vitest, Testing Library.

---

### Task 1: Define executable formatter behavior
- Create `frontend/tests/model-safe-test.test.ts` with table-driven assertions for auth errors, rate limits, missing models, thrown ApiError envelopes, network/non-JSON unsafe bodies, truncation, and success.
- Run the focused test and confirm RED because the formatter module does not exist.

### Task 2: Implement the formatter
- Create `frontend/src/lib/model-test-result.ts` with typed normalization and sanitization.
- Run the focused test and confirm GREEN.

### Task 3: Wire the Models page
- Extend the test with rendered-page interactions covering HTTP-200 failure, thrown envelope, and success UI/toast behavior; confirm RED.
- Modify `frontend/src/routes/dashboard/models/+page.svelte` to use structured results, render status/HTTP/latency/message/detail/hint, and show rich failure toasts.
- Run focused tests and confirm GREEN.

### Task 4: Verify and commit
- Run canonical `npm test`, `npm run check`, and `npm run build` in `frontend/`.
- Visually inspect the page using the browser harness against a local preview fixture if feasible.
- Review the diff for credential leakage, commit the focused change, comment structured evidence, and block `review-required` without deploying.

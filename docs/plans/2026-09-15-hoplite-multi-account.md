# Hoplite Multi-Account Implementation Plan

> **For Hermes:** Execute directly with strict red-green-refactor cycles; credential-sensitive work is not delegated.

**Goal:** Add backward-compatible, independently managed and safely routed Hoplite accounts.

**Architecture:** Persist non-secret account metadata separately from encrypted credentials. Preserve the legacy account and v1 model namespace while adding account-qualified v2 IDs. Route every management and proxy operation through a resolved account object and fail closed on inactive/unhealthy/exhausted/expired state.

**Tech Stack:** Go, SQLite, net/http, SvelteKit 5, TypeScript.

---

### Task 1: Account persistence and legacy migration
- Test: add backend tests proving existing encrypted `hoplite` credential is adopted without ciphertext changes and multiple accounts have stable IDs.
- Run focused tests and verify RED.
- Add idempotent schema, account repository helpers, and migration.
- Run focused tests and verify GREEN.

### Task 2: Account CRUD and account-scoped management
- Test: admin-only add/list/patch/delete, masking, secret preservation, and per-account test/project dispatch.
- Run focused tests and verify RED.
- Add routes and account-aware client resolution while keeping omitted `account_id` mapped to default.
- Run focused tests and verify GREEN.

### Task 3: Exact model identity and routing eligibility
- Test: same projects/models across accounts produce distinct IDs; direct/combo routing uses the intended secret; disabled, unhealthy, exhausted, and expired accounts are skipped/rejected.
- Run focused tests and verify RED.
- Add v2 ID codec, account-aware catalogs, combo dispatch, and eligibility checks.
- Run focused tests and verify GREEN.

### Task 4: Connections UI lifecycle
- Extend frontend contract tests first for add/edit/test/models/usage/disable/delete controls and prohibition on credential copy/pooling.
- Run test and verify RED.
- Implement account modal/actions and account-specific labels/IDs.
- Run frontend checks and contract tests until GREEN.

### Task 5: Regression and production gates
- Run focused backend tests.
- Run `go test ./...`.
- Run frontend check and build.
- Run `git diff --check`, inspect complete diff, and scan for secret leakage.
- Commit and push main.
- Record runtime hash and database/binary backup, then run `make build && make deploy`.
- Verify health/version, service status/restarts, anonymous 401 boundaries, authenticated accounts/models/balances, logs, and rollback artifact.

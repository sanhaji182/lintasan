# Hoplite Cloud Agent Implementation Plan

> **For Hermes:** Execute with strict TDD and verify each layer before claiming completion.

**Goal:** Add Hoplite as a secure Experimental Cloud Agent in Lintasan without exposing it to official LLM routing.

**Architecture:** A dedicated Go REST client talks to Hoplite using a server-side organization API key from Lintasan's encrypted credential store. Authenticated management handlers expose bounded project/thread operations to a dedicated dashboard section.

**Tech Stack:** Go `net/http`, Lintasan auth/credential store, SvelteKit 5, TypeScript.

---

### Task 1: Hoplite REST client

**Files:**
- Create: `internal/hoplite/client.go`
- Create: `internal/hoplite/client_test.go`

Write failing contract tests for authentication, project retrieval, thread create/list/detail/messages, idempotency payload, timeout/error metadata, and secret-safe errors. Implement the minimum typed client, then rerun package tests.

### Task 2: Secure server routes

**Files:**
- Create: `internal/server/handlers_hoplite.go`
- Create: `internal/server/handlers_hoplite_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/handlers_credentials.go`

Write failing middleware/handler tests for anonymous denial, missing credentials, read-only connection test, project listing, input validation, default-safe thread creation, and upstream status mapping. Register dedicated cloud-agent routes and include Hoplite in the encrypted credential catalog without treating it as an ACP descriptor.

### Task 3: Experimental Cloud Agent UI

**Files:**
- Create: `frontend/src/routes/dashboard/experimental/hoplite/+page.svelte`
- Modify: `frontend/src/routes/dashboard/experimental/+page.svelte`

Add a Hoplite cloud-agent card and operations screen: credential control, connection test, project selector, explicit new-thread form, thread list, and refreshable detail/messages. Keep `autoFix` and `autoMerge` off by default; do not offer automatic routing.

### Task 4: Documentation and verification

**Files:**
- Modify: `docs/experimental-providers.md`

Run focused Go tests with `-v -count=1`, full Go tests, `go vet`, frontend `svelte-check`, frontend production build, staged whitespace/secret scan, and inspect the final diff. Do not deploy or mutate production.

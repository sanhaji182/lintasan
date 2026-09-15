# Hoplite Multi-Account Design

Status: approved by Kanban acceptance criteria t_56237bd3

## Identity and compatibility

Each Hoplite account is represented by a `hoplite_accounts` metadata row. Its stable connection ID is `hoplite-cloud-agent` for the migrated/default account and `hoplite-cloud-agent/<uuid>` for subsequent accounts. Secrets remain solely in `experimental_credentials`; account rows reference credential names (`hoplite` for the legacy/default account and `hoplite-account:<uuid>` for new accounts). Migration adopts the existing encrypted `hoplite` row by reference, so no plaintext or re-entry is required.

The default account retains legacy model IDs (`hoplite-agent/<project>` and `hoplite-model/v1/<project>/<model>`). Additional accounts use collision-free IDs (`hoplite-agent/v2/<account>/<project>` and `hoplite-model/v2/<account>/<project>/<model>`, with URL-safe base64 path components). Existing clients and combos therefore continue to work while every new account routes exactly.

## API and lifecycle

Admin-only account CRUD lives under `/api/experimental/cloud-agents/hoplite/accounts`. Create requires a display name and unmasked secret, generates stable IDs, and encrypts through the existing credential store. Patch supports rename, activation, and optional secret replacement; masked placeholders are rejected. Delete removes account metadata and its encrypted credential transactionally, with default-account deletion also removing the legacy credential. Existing Hoplite status/test/projects/thread endpoints accept `account_id`, defaulting to the legacy account for compatibility.

`/api/connections`, model discovery, `/v1/models`, connection testing, and balances enumerate accounts independently. Account health and usage snapshots are stored as metadata only (never secrets).

## Routing safety

Direct model IDs resolve an account from their namespace; combo targets use their exact connection ID. Before thread creation, routing rejects/skips inactive accounts, accounts with a persisted unhealthy test result, exhausted credit, or expired entitlement. It refreshes billing eligibility immediately before autonomous work where billing is available. Pre-acceptance failures may continue to the next priority target; after a thread is accepted fallback remains forbidden.

Cloud-agent combos remain priority-only. Standard connection behavior and database rows are unchanged.

## UI

Connections renders one card per account, supports add, rename/key update, test, models, usage, enable/disable, and delete confirmation. Cloud-agent cards never offer plaintext-copy or pool actions. The Hoplite diagnostics page remains compatible with the default account.

## Verification and rollback

TDD covers migration, isolation, exact IDs, CRUD/auth/masking, routing eligibility, and frontend contracts. Required gates: focused tests, `go test ./...`, frontend check/build, `git diff --check`. Deployment uses `make build && make deploy`, whose timestamped `lintasan.bak-*` artifact is the rollback source.

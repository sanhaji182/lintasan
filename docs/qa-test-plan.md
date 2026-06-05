# QA Automation Test Plan — Lintasan Beta

**Server:** `http://lintasan.sans.biz.id:20180`
**Version:** `v0.24.2-1-gfb19ed1`
**Beta Status:** GO (stabilization complete)
**Test Suite Base:** 826/826 passing

---

## 1. Input Required

Sebelum menjalankan test, pastikan kamu punya:

| Input | Default | Required |
|-------|---------|----------|
| `ADMIN_USER_EMAIL` | `admin@example.com` | ✅ Yes |
| `ADMIN_PASSWORD` | (random generated) | ✅ Yes |
| `API_KEY_PROVIDER` | (OpenAI/Anthropic/DeepSeek) | ❌ Opsional (skip test yang butuh provider) |
| `SERVER_URL` | `http://lintasan.sans.biz.id:20180` | ❌ Optional |
| `EXISTING_CONNECTION_ID` | (auto-create if missing) | ❌ Optional |

---

## 2. Test Coverage Summary

| Area | Test Cases | Priority |
|------|------------|----------|
| Auth & Setup | 5 | P0 |
| Provider Connection | 4 | P0 |
| Chat Completion | 2 | P0 |
| Embeddings/Images/Audio | 4 | P1 |
| Routing (combos/fallback/alias/lb) | 12 | P0 |
| Cache | 3 | P1 |
| Cost & Analytics | 6 | P1 |
| Memory | 3 | P1 |
| MCP | 3 | P2 |
| Plugin | 7 | P1 |
| Webhook | 5 | P1 |
| Backup/Restore | 6 | P1 |
| Teams & Users | 6 | P1 |
| Keys | 3 | P1 |
| Quota | 1 | P2 |
| Regression (after fixes) | 16 | P0 |
| **Total** | **82** | - |

---

## 3. Test Cases

### 3.1 Auth & Setup (P0)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 1 | Login valid | POST | `/api/auth/login` | 200 + JWT token + admin info | ⏳ |
| 2 | Get me | GET | `/api/auth/me` | 200 + user data | ⏳ |
| 3 | Logout | POST | `/api/auth/logout` | 200 + token invalid | ⏳ |
| 4 | Setup status | GET | `/api/setup/status` | `{state: "active", has_admin: true, has_master_key: true}` | ⏳ |
| 5 | Health check | GET | `/health` | 200 + `{status: "ok", version: "v0.24.2"}` | ⏳ |

---

### 3.2 Provider Connection (P0)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 6 | List connections | GET | `/api/connections` | 200 + array | ⏳ |
| 7 | Create connection | POST | `/api/connections` | 200 + connection_id | ⏳ |
| 8 | Test connection | POST | `/api/connections/test` | 200 + success/fail | ⏳ NEEDS_KEY |
| 9 | List presets | GET | `/api/providers/presets` | 200 + 118+ presets | ⏳ |

---

### 3.3 Chat Completion (P0)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 10 | Non-stream chat | POST | `/v1/chat/completions` | 200 + response.content | ⏳ NEEDS_KEY |
| 11 | Streaming chat | POST | `/v1/chat/completions` (stream: true) | chunk count > 1 | ⏳ NEEDS_KEY |

---

### 3.4 Embeddings / Images / Audio (P1)

| # | Test | Method | Expected | Status |
|---|------|--------|----------|--------|
| 12 | Embeddings | POST | 200 + embeddings array | ⏳ NEEDS_KEY |
| 13 | Images | POST | 200 + image URL | ⏳ NEEDS_KEY |
| 14 | Speech | POST | 200 + audio binary | ⏳ NEEDS_KEY |
| 15 | Transcription | POST | 200 + text | ⏳ NEEDS_KEY |

---

### 3.5 Routing (P0)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 16 | Smart routing get | GET | `/api/smart-routing` | 200 + config | ⏳ |
| 17 | Smart routing update | POST | `/api/smart-routing` | 200 + updated | ⏳ |
| 18 | List combos | GET | `/api/combos` | 200 + array | ⏳ |
| 19 | Create combo | POST | `/api/combos` | 200 + combo_id | ⏳ |
| 20 | Update combo strategy | PATCH | `/api/routing/combos/{id}` | 200 + updated (**VERIFY FIX P0**) | ⏳ |
| 21 | Reorder combo | POST | `/api/routing/reorder` | 200 + updated | ⏳ |
| 22 | Delete alias | DELETE | `/api/routing/aliases/{id}` | 200 (**VERIFY FIX**) | ⏳ |
| 23 | Load balancer get | GET | `/api/load-balancer` | 200 + strategy | ⏳ |
| 24 | Load balancer update | POST | `/api/load-balancer` | 200 + updated (**VERIFY FIX**) | ⏳ |
| 25 | List fallback | GET | `/api/fallback` | 200 + chains | ⏳ |
| 26 | Create fallback | POST | `/api/fallback` | 200 + chain_id | ⏳ |
| 27 | Delete fallback chain | DELETE | `/api/fallback/model-chains/{id}` | 200 (**VERIFY FIX**) | ⏳ |

---

### 3.6 Cache (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 28 | Clear cache | POST | `/api/cache` | 200 + cleared | ⏳ |
| 29 | Get cache stats | GET | `/api/cache` | 200 + stats | ⏳ |
| 30 | Cache hit | 2x chat | Same prompt → 2nd from cache | ⏳ |

---

### 3.7 Cost & Analytics (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 31 | Get costs | GET | `/api/costs` | 200 + today/month > 0 (**VERIFY FIX P1**) | ⏳ |
| 32 | Savings summary | GET | `/api/savings/summary` | 200 + savings data | ⏳ |
| 33 | Analytics | GET | `/api/analytics` | 200 + data | ⏳ |
| 34 | Analytics realtime | GET | `/api/analytics/realtime` | 200 + data not empty | ⏳ |
| 35 | Analytics stream | GET | `/api/analytics/stream` | SSE stream + data (**Major stub**) | ⏳ |
| 36 | Analytics combos | GET | `/api/analytics/combos` | 200 + stats (may be empty) | ⏳ |

---

### 3.8 Memory (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 37 | Store memory | POST | `/v1/memory` | 200 + memory_id | ⏳ |
| 38 | Search memory | GET | `/v1/memory/search` | 200 + results | ⏳ |
| 39 | Memory stats | GET | `/v1/memory/stats` | 200 + stats | ⏳ |

---

### 3.9 MCP (P2)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 40 | MCP initialize | POST | `/mcp` | 200 + jsonrpc response | ⏳ |
| 41 | SSE transport | GET | `/mcp/sse` | SSE stream | ⏳ |
| 42 | List tools | GET | `/api/mcp/tools` | 200 + 14 tools | ⏳ |

---

### 3.10 Plugin (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 43 | List plugins | GET | `/api/plugins` | 200 + array | ⏳ |
| 44 | Create plugin | POST | `/api/plugins` | 200 + plugin_id | ⏳ |
| 45 | AI generate plugin | POST | `/api/plugins/generate` | 200 + code != stub (**VERIFY FIX P0**) | ⏳ |
| 46 | Update plugin config | PATCH | `/api/plugins/{id}/config` | 200 (**VERIFY FIX**) | ⏳ |
| 47 | Install plugin | POST | `/api/plugins/install` | 200 (**VERIFY FIX**) | ⏳ |
| 48 | Delete plugin | DELETE | `/api/plugins/{id}` | 200 (**VERIFY FIX**) | ⏳ |
| 49 | Plugin store | GET | `/api/plugins/store` | 200 + items | ⏳ |

---

### 3.11 Webhook (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 50 | List webhooks | GET | `/api/webhooks` | 200 + array | ⏳ |
| 51 | Create webhook | POST | `/api/webhooks` | 200 + webhook_id | ⏳ |
| 52 | Update webhook | PATCH | `/api/webhooks/{id}` | 200 (**VERIFY FIX**) | ⏳ |
| 53 | Test webhook | POST | `/api/webhooks/{id}/test` | 200 (**VERIFY FIX**) | ⏳ |
| 54 | Delete webhook | DELETE | `/api/webhooks/{id}` | 200 (**VERIFY FIX**) | ⏳ |

---

### 3.12 Backup/Restore (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 55 | Create backup | POST | `/api/backup` | 200 + backup_id | ⏳ |
| 56 | List backup | GET | `/api/backup` | 200 + list | ⏳ |
| 57 | Import backup | POST | `/api/backup/import` | 200 (**VERIFY FIX**) | ⏳ |
| 58 | Restore backup | POST | `/api/backup/{id}/restore` | 200 (**VERIFY FIX**) | ⏳ |
| 59 | Delete backup | DELETE | `/api/backup/{id}` | 200 (**VERIFY FIX**) | ⏳ |
| 60 | Export | GET | `/api/export` | 200 + settings | ⏳ |

---

### 3.13 Teams & Users (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 61 | List users | GET | `/api/auth/users` | 200 + array | ⏳ |
| 62 | Create user | POST | `/api/auth/users` | 200 + user_id | ⏳ |
| 63 | List teams | GET | `/api/teams` | 200 + array | ⏳ |
| 64 | Create team | POST | `/api/teams` | 200 + team_id | ⏳ |
| 65 | Add member | POST | `/api/teams/{id}/members` | 200 | ⏳ |
| 66 | Remove member | DELETE | `/api/teams/{id}/members/{name}` | 200 (**VERIFY FIX**) | ⏳ |

---

### 3.14 Keys (P1)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 67 | List keys | GET | `/api/keys` | 200 + array | ⏳ |
| 68 | Create key | POST | `/api/keys` | 200 + key_id | ⏳ |
| 69 | Delete key | DELETE | `/api/keys/{id}` | 200 (**VERIFY FIX**) | ⏳ |

---

### 3.15 Quota (P2)

| # | Test | Method | Endpoint | Expected | Status |
|---|------|--------|----------|----------|--------|
| 70 | Quota stats | GET | `/api/quota/stats` | 200 + real data | ⏳ |

---

### 3.16 Regression After Fixes (P0)

Re-test semua route yang di-fix di P0/P1:

| # | Route | Method | Status |
|---|-------|--------|--------|
| 71 | `/api/routing/combos/{id}` | PATCH | ⏳ |
| 72 | `/api/routing/aliases/{id}` | DELETE | ⏳ |
| 73 | `/api/fallback/model-chains/{id}` | DELETE | ⏳ |
| 74 | `/api/fallback/connection-chains/{id}` | DELETE | ⏳ |
| 75 | `/api/webhooks/{id}` | DELETE | ⏳ |
| 76 | `/api/webhooks/{id}` | PATCH | ⏳ |
| 77 | `/api/webhooks/{id}/test` | POST | ⏳ |
| 78 | `/api/backup/import` | POST | ⏳ |
| 79 | `/api/backup/{id}/restore` | POST | ⏳ |
| 80 | `/api/backup/{id}` | DELETE | ⏳ |
| 81 | `/api/plugins/{id}/config` | PATCH | ⏳ |
| 82 | `/api/plugins/install` | POST | ⏳ |
| 83 | `/api/plugins/{id}` | PATCH | ⏳ |
| 84 | `/api/plugins/{id}` | DELETE | ⏳ |
| 85 | `/api/teams/{id}/members/{name}` | DELETE | ⏳ |
| 86 | `/api/keys/{id}` | DELETE | ⏳ |
| 87 | `/api/load-balancer` | POST | ⏳ |

---

## 4. Execution Flow

### Phase 1: Auth & Setup
1. Test login
2. Store JWT token
3. Test all protected endpoints with token

### Phase 2: Core Flow
1. Provider connection
2. Chat completion
3. Routing
4. Cache

### Phase 3: Data & Analytics
1. Cost tracking
2. Analytics
3. Memory

### Phase 4: Integrations
1. MCP
2. Plugin
3. Webhook

### Phase 5: Operations
1. Backup/restore
2. Teams/users
3. Keys

### Phase 6: Regression
1. Re-test semua route yang di-fix

---

## 5. Status Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | PASS |
| ❌ | FAIL |
| ⏳ | Not tested yet |
| ⏸️ | NEEDS_KEY (skip if no API key) |
| ⏭️ | SKIP (no UI consumer) |

---

## 6. Report Template

### Test Summary

| Metric | Value |
|--------|-------|
| Total test cases | 87 |
| PASS | _FILL_ |
| FAIL | _FILL_ |
| SKIP | _FILL_ |
| NEEDS_KEY | _FILL_ |

### Blocking Issues

| # | Bug | Severity | Endpoint | Status |
|---|-----|----------|----------|--------|
| - | - | - | - | - |

### Beta Decision

| Criteria | Status |
|----------|--------|
| All P0 selesai | ✅ / ❌ |
| No critical regression | ✅ / ❌ |
| Test suite green | ✅ / ❌ |
| **Final decision** | **GO / CONDITIONAL GO / NO-GO** |

---

## 7. Notes

- Audit correction: 16 "missing routes" sebenarnya udah ada di `handlers_rest.go` [AGENTS.md §12]
- P0 fixes: `handlePluginGenerate` (real LLM), `handleCosts` (real data), `load-balancer` method fix
- Non-blocking issues: SSE stream stub, prompt routing stub, OAuth stub, interactive setup wizard stub
- Beta status: GO (stabilization complete v0.24.2)

---

**Template version:** 1.0
**Generated:** 2026-06-05
**For:** Lintasan Beta QA Automation

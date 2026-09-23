# Qoder credit pool — accounting, and the external consumer that was found

> **Conclusion up front.** The nine Qoder PATs in this gateway's pool are **shared
> with 9router, which runs on a different host** (`baby`, `43.133.136.147`,
> `9router.service`, `127.0.0.1:20128`). 9router holds byte-identical copies of all
> nine credentials and round-robins chat traffic across them. That traffic — not
> anything in this gateway — drained the pool. It is still wired up and will drain
> the pool again when the plan resets.

This document exists because the pool drop was originally attributed to an
unidentified external consumer and the honest answer at the time was "we cannot
find it inside our estate". It has now been found, and it *is* inside our estate.

---

## 1. The measurement that needed explaining

| observation | value |
|---|---|
| pool at 2026-09-23 05:11 WIB | **280** credits across 9 accounts |
| pool at 2026-09-23 08:16 WIB | **13** credits (8 of 9 accounts at zero) |
| credits recorded by this gateway in that window | **5.493** across 4 turns |
| unexplained | **~267 credits, ~98% of the drain** |

The gateway's own `request_logs.credits` column is the reference for "what did
*we* spend". Anything above it is, by definition, another holder of the same
credentials.

## 2. The consumer: 9router on `baby`, driven by the `pi` CLI

`9router.service` runs on **baby** (`43.133.136.147`, hostname
`VM-0-5-ubuntu`) from `/opt/9router-runtime/node_modules/9router` (v0.5.75),
listening on `127.0.0.1:20128`, published at `https://9r.sans.biz.id`.

Its data root is `/home/ubuntu/.9router/db/data.sqlite`. Two facts settle it:

**(a) The credentials are byte-identical.** `providerConnections` holds **20**
`qoder` rows. Comparing the `data.apiKey` values against
`connections.api_key` in `data/lintasan.db`:

```
9router qoder PATs : 20
Lintasan qoder PATs:  9
byte-identical overlap: 9   (i.e. ALL of them)
```

Those nine rows are named `Key 2, Key 4, Key 6, Key 1, Key 3, Key 5, Key 7,
Key 8, Key 9, Key 10`. The remaining ten (`Akun [41]` … `Akun [50]`) were
created 2026-09-22T23:17Z — *mid-drain*, at 06:17 WIB — and are **not** in this
gateway's pool; they look like replacements bought once the shared ones ran dry.

**(b) Its usage history covers the window exactly.** `usageHistory` is
per-request. For `provider='qoder'` between 2026-09-22T22:11Z and
2026-09-23T00:54Z (= 05:11–07:54 WIB, the measured drain window):

| grouping | requests | prompt tokens |
|---|---|---|
| on PATs shared with Lintasan (`Key 2`–`Key 10`) | **95** | 6.69 M |
| on 9router-owned PATs (`Akun [41]`–`[50]`) | 151 | 10.94 M |
| **total in window** | **246** | 17.63 M |

Every one of those 246 requests carries the same 9router API key — the row in
`apiKeys` named **`pi cli`** (`sk-661c…904e`). All `model = ultimate`, all
`status = ok`, all `endpoint = /v1/chat/completions`.

## 3. The correlation that makes it conclusive

Per-account, using the 05:11 WIB snapshot from `scripts/pool_quota.py` against
the current reading, and 9router's request count per shared PAT:

| account (Lintasan) | prio | left @05:11 | left @08:16 | **burned** | 9router rows named | **reqs in window** |
|---|---|---|---|---|---|---|
| `b46ac2eeef5aRyanPerry` | 70 | 0 | 0 | 0 | Key 6 | 0 |
| `4003adcfb7b8BrendaBrown` | 69 | 0 | 0 | 0 | Key 4 | 0 |
| `84f7923552e2DennisGarcia` | 68 | 0 | 0 | 0 | Key 2 | 0 |
| `076b02ada93aBwOod` | 67 | 15 | 0 | **15** | Key 7 | **7** |
| `6e8e999dfc11PamelaH` | 66 | 0 | 0 | 0 | Key 5 | 0 |
| `bf42b3d9f407GaryThompson` | 65 | 45 | 0 | **45** | Key 3 | **19** |
| `eabe7292bb5eEvelynLong` | 64 | 23 | 0 | **23** | Key 10 | **5** |
| `cd66a79242d7VtaYlor` | 63 | 78 | 0 | **78** | Key 9 | **31** |
| `b8259b0188ccRuthMoore` | 62 | 119 | 13 | **106** | Key 8 | **33** |
| **pool** | | **280** | **13** | **267** | | **95** |

**5 of 5 accounts with a positive balance were hit by 9router. 4 of 4 accounts
already at zero were not.** The mapping is 1:1 and admits no third party: if any
other consumer existed, the four exhausted accounts would have shown burn too,
and they did not.

Aggregate rate: **267 credits / 95 requests = 2.81 credits per request** on
9router's shared-PAT traffic. The per-account rates corroborate this rather than
contradicting it — `RuthMoore` 106/33 = 3.2, `VtaYlor` 78/31 = 2.5,
`GaryThompson` 45/19 = 2.4, `EvelynLong` 23/5 = 4.6, `BwOod` 15/7 = 2.1 — all
within a factor of two of each other and none near zero or near 300. (Do not
extrapolate this into a per-token model: `ultimate` at ~71 K prompt tokens per
request is the shape, and prefix caching varies per request. The aggregate is
the measurement; a derived per-token rate is not.)

**This window is the final leg, not the whole drain.** Four accounts
(`RyanPerry`, `BrendaBrown`, `DennisGarcia`, `PamelaH`) were *already* at zero
at 05:11 and show zero 9router requests in the window — because 9router had
finished with them earlier. `Key 4`/`Key 2` were last used 2026-09-18, `Key 6` on
2026-09-18; their 300 credits went well before this snapshot. The lifetime
figures in the next subsection are what cover that earlier phase.

### Lifetime exposure, not just this window

Since the plan window opened on 2026-09-14, 9router has served, on the nine
shared PATs alone:

```
2583 requests   349.0 M prompt tokens   1257 K completion tokens
2026-09-15T14:42:39Z -> 2026-09-23T00:51:40Z
```

By client key, over 2026-09-21 → now: `pi cli` 1584 reqs / 138.0 M tokens;
`Hermes Phase 4 rotation 2026-07-27` 289 reqs / 43.24 M; `codex` 2 reqs. The
consumer is the `pi` CLI (User-Agent `pi (darwin 25.6.0; arm64)`, i.e. macOS
ARM), reaching `9r.sans.biz.id` from Indonesian consumer IPs — predominantly
`103.82.15.219` (Jakarta, PT Mora Telematika) with `112.215.x` / `140.213.x`
mobile ranges.

## 4. Why this was not obvious

- **Two separate SQLite databases on two separate hosts.** Neither gateway can
  see the other's spend; the shared credential is the only join key, and the
  credential is not written down anywhere as *shared*.
- **9router's schema hides it behind JSON.** `providerConnections.apiKey` does
  not exist as a column — the credential lives at
  `json_extract(data, '$.apiKey')`. A column-level look finds nothing.
- **The names are uninformative.** Lintasan calls them `Qoder <prefix><Name>`;
  9router calls the same credentials `Key 2`, `Key 8`. Only the PAT value links them.
- **`qoder2api` was the obvious suspect and is genuinely dead** — which made
  "we cannot find an internal consumer" feel settled. The real consumer was a
  different proxy entirely.
- **The drain shape looks like a bug in routing.** It is not. See §6.

## 5. Current state — the exposure is STILL LIVE

As of 2026-09-23 08:18 WIB:

- `9router.service` is `active` and `enabled`.
- It has **12** `qoder` connections with `isActive = 1`, including `Key 8`
  (`pt-IlxkpY36NOM`, → `RuthMoore`) and `Key 9` (`pt-s93cVg1936y`, → `VtaYlor`).
- The last shared-PAT request was `2026-09-23T00:51:53Z` (07:51 WIB).

The traffic stopped because **the pool is empty**, not because the consumer
stopped. `nextResetAt` is **2026-09-28T04:53:32+07:00**; when the plan resets,
the same nine credentials are handed 300 credits each and 9router will consume
them again. Nothing about the drain mechanism is self-limiting.

- **The consumer is NOT a hop through this gateway.** 9router bundles its own
  native Qoder client (`app/src/lib/oauth/services/qoder.js`) and calls
  `openapi.qoder.sh` directly (24 references; `api3.qoder.sh`,
  `center.qoder.sh` also present). No reference to Lintasan exists in its
  runtime. The traffic is genuinely off-gateway, which is why our
  `request_logs` could never account for it.

## 6. What NOT to do about it

**Do not "optimise" routing by skipping `isQuotaExceeded` connections.** Measured
2026-09-23: a live account at `used=300 / total=300`, `isQuotaExceeded: true`,
`remaining: 0` still answers chat normally — HTTP 200, real content, ~400 ms —
via the vendor's daily basic-model allowance. Skipping those connections would
remove three working accounts from a nine-account pool. `Quota.IsQuotaExceeded`
is parsed, surfaced in `/api/qoder/quota`, read by the dashboard, and
**deliberately not consulted on the request path**. `grep -rn IsQuotaExceeded
--include='*.go' | grep -v _test` returning only `quota.go` is correct.

The pool drain and the routing behaviour are unrelated problems. Fixing the
routing causes an outage; fixing the sharing stops the drain.

## 7. Mitigation options (none applied by this document)

This is a finding, not a change. The decision belongs to the operator:

1. **De-share the credentials (root fix).** Either remove the nine shared PATs
   from 9router's `providerConnections`, or give 9router its own accounts
   (`Akun [41]`–`[50]` already are, but the shared rows are still active and
   still take round-robin turns). Two proxies must not present the same PAT:
   the device fingerprint derives from `account_id + credential`, so both
   collide on one device identity as well as one quota.
2. **Treat the pool as shared capacity (acceptance path).** If sharing is
   intended, size the pool for the combined load and alert on burn rate rather
   than on absolute balance — 2.81 credits/request × 9router's rate is the
   number that matters, and it is not visible from either side alone.
3. **Per-account alerting.** A drop of >N credits between two 5-minute windows
   with no matching `request_logs.credits` is the signature. That comparison is
   the detector; either half alone tells you nothing.

## 8. How to re-verify

```bash
# What WE spent (core)
cd /home/ubuntu/lintasan-go
sqlite3 -header -column data/lintasan.db \
  "SELECT COUNT(*), ROUND(SUM(COALESCE(credits,0)),3) FROM request_logs
   WHERE connection_id IN (SELECT id FROM connections WHERE LOWER(format)='qoder')
     AND created_at >= '<window start>';"

# Pool balance (core) — distinguishes OK/EMPTY/NULL/ERROR per account
python3 scripts/pool_quota.py --refresh

# Who ELSE is spending it (baby)
ssh ubuntu@43.133.136.147 \
  'sqlite3 ~/.9router/db/data.sqlite \
   "SELECT p.name, substr(json_extract(p.data,\"\$.apiKey\"),1,14) pat,
           COUNT(u.id) reqs, ROUND(SUM(u.promptTokens)/1000000.0,2) mtok
    FROM providerConnections p
    LEFT JOIN usageHistory u ON u.connectionId = p.id
    WHERE p.provider = \"qoder\" GROUP BY p.id ORDER BY reqs DESC;"'

# Are the two sets still the same credentials?
ssh ubuntu@43.133.136.147 \
  'sqlite3 ~/.9router/db/data.sqlite \
     "SELECT json_extract(data,\"\$.apiKey\") FROM providerConnections WHERE provider=\"qoder\";"' \
  | sort > /tmp/9r_keys.txt
sqlite3 data/lintasan.db \
  "SELECT api_key FROM connections WHERE LOWER(format)='qoder';" \
  | sort > /tmp/lt_keys.txt
comm -12 /tmp/9r_keys.txt /tmp/lt_keys.txt | wc -l   # >0 means still shared
```

### Measurement traps that apply to this check

- **Never measure consumption with a repeated prompt.** The exact-response cache
  answers identical prompts in ~3 ms with `cached=1` and
  `connection_id=exact-ca*`, costing **0 credits**. Use distinct content, or read
  per-connection rows instead of the pool total.
- **`QuotaCache` TTL is 5 minutes.** Use
  `GET /api/qoder/quota/{connection_id}?refresh=1`; the list endpoint does not
  force a re-read.
- **A missing `user_quota.remaining` defaulted to 0 is indistinguishable from a
  real zero balance.** Use `scripts/pool_quota.py`, which reports
  OK / EMPTY / NULL / ERROR per account. A failed fetch must never print `left=0`.

## 9. Method, generalised

When a metered resource drains faster than the gateway's own logs account for,
the gateway's logs have told you the delta but not the answer. The credential is
the only join key across independent systems, so:

1. Compute the delta from the vendor's number minus your own recorded spend.
2. Search the estate for **other holders of the same credential**, not for other
   copies of your own code. Compare by value, not by name or prefix.
3. Check the *correlation*, not the total: if accounts the suspect did **not**
   touch also drained, you have not found it yet.
4. Expect the other holder to be a different product with a different schema.
   JSON-embedded secrets and renamed rows are the normal case, not an edge case.

Found this way: 9router on baby, all nine credentials, 95 requests, 267 credits,
matching the burn account-for-account.

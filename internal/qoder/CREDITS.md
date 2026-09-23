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

## 7. Decision taken: shared capacity, alerted on burn rate

**Operator decision, 2026-09-23: option 2 — the sharing is ACCEPTED.** The nine
PATs stay in both Lintasan and 9router. De-sharing was considered and rejected;
the options are kept below because the reasoning still matters if the decision is
ever revisited.

The consequences of accepting it, spelled out so nobody rediscovers them as bugs:

- **The pool is shared capacity, so its absolute balance is not ours to read.**
  A low balance is not an incident. `summary.available` counting accounts at zero
  as available is correct for the same reason — a `credits=0` account still serves
  chat (see the VENDOR ALLOWANCE section).
- **The number that matters is the burn RATE, and specifically the part of it we
  cannot account for.** Ours is `SUM(request_logs.credits)` for qoder connections;
  the pool delta is everyone's. The difference is the signal.
- **Sizing must assume both consumers.** Anything sized against our own traffic
  alone will be wrong by roughly an order of magnitude — on 2026-09-23 our share was
  ~2% of the drain (5.49 of 267 credits).
- **Two proxies present the same PAT**, and the device fingerprint derives from
  `account_id + credential`, so both also collide on one device identity. A symptom
  that looks like "impossible" concurrency or rotation on one account is expected
  under sharing, not a bug.

### The detector is live

`~/.hermes/scripts/qoder_burn_watch.py`, cron **`aa722a156bbb`** every 30 minutes,
`no_agent`, delivering to the DevOps topic. It compares the pool delta against
`request_logs.credits` over the same window and is **silent unless the drop is
unattributable to us**:

```
alert when  pool_drop >= 20 credits
      AND   unaccounted >= 15 credits
      AND   unaccounted / pool_drop >= 50%
```

Verified before arming, because an alerting rule nobody has seen fire is not a
verified rule:

| case | expected | observed |
|---|---|---|
| 280 -> 13 with 5.5 ours | alert, 98% unaccounted | alert, "Unaccounted: 261.5 (98%)" |
| drop below 20 credits | silent | silent |
| big drop, spend matches (7 unaccounted) | silent | silent |
| pool unreadable this cycle | silent, no false report | silent |

Also verified: **polling is free.** 30 forced quota reads moved the pool by exactly
0.0000 and added 0 rows to `request_logs`, so the watchdog cannot consume what it
measures. It forces `?refresh=1` per account because `QuotaCache` TTL is 5 minutes
and a stale read would report a delta of zero.

State lives in `~/.hermes/state/qoder_burn_watch.json` (mode 600, self-resetting so
a reboot does not produce a bogus delta). `--verbose` prints every cycle's numbers;
`--reset-state` clears the baseline after a pool reset.

## 7a. Options considered (kept for the record)

1. **De-share the credentials (root fix) — NOT taken.** Either remove the nine
   shared PATs from 9router's `providerConnections`, or give 9router its own
   accounts (`Akun [41]`–`[50]` already are, but the shared rows are still active
   and still take round-robin turns). Rejected because it changes 9router's routing
   behaviour, which is a system the operator relies on. Note for any future revisit:
   9router has **ten Qoder accounts of its own** whose credentials match none of
   ours, so de-sharing may cost it little.
2. **Treat the pool as shared capacity — TAKEN.** See above.
3. **Per-account alerting.** Implemented as part of the watchdog: the alert names
   the per-account movers, so it says where the credits went as well as how many.

## 8. How to re-verify

```bash
# Is the watchdog armed, and did its last run stay silent or alert?
hermes cron list | grep -A9 aa722a156bbb

# What is it seeing right now? (prints every cycle's numbers, never alerts)
python3 ~/.hermes/scripts/qoder_burn_watch.py --verbose

# Force a fresh baseline after the pool resets (next reset: check nextResetAt)
python3 ~/.hermes/scripts/qoder_burn_watch.py --reset-state
```

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

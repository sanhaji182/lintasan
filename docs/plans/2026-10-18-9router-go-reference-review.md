# Recheck B1 against luqman-v1/9router-go (Native Go port)

Date: 2026-10-18
Target: https://github.com/luqman-v1/9router-go @ v1.9.14-exp.5, CHANGELOG latest
        v1.9.13, parity upstream v0.5.85 (out of source | github.com/decolua/9router)
Repo:   /tmp/9router-go-ref — 128 internal/proxy/executor/*.go files + >=64
        translator/*.go files, >=56 change-fragments (.changes/<pr>.md)
License: MIT, same family as Lintasan.

---

## 1. Kesimpulan

luqman-v1/9router-go = Native Go port yang **integritas roadmap is high-grade**;
dia punya 56 .changes/<pr>.md per-fragment + per-fragment tests, semua
diproduksi lapor vs upstream v0.5.85..v0.5.104 In Tanpa "say its theirs"
caveat (see COMPARISON.md §"Current differences and accepted limitations").
Tiga area**: (a) opencode-zen, (b) openCode fingerprint v2, (c) B1
errorclassify**. Semua WITHOUT  RE-INVENTING the wheel — ini referensi
produksi yang telah dijalankan upstream.

Value: luqman code **verified live against decolua/9router / gate** —
pertamanya `TestConcealOpenCodeV2CollidingNamesStillAppendExact`,
`TestZenShapedBodyFollowsTheDestinationGate`, `classifyLive`. Tapi B1:
perbapnya lebih tipis, karena Lintasan punya pool model selain糙 relays
("44 pool model". Go-syntax bogs; saya hanya menghitung per 9router-go.

---

## 2. Perbandingan langsung — B1 / errorclassify

### 2.1 9router-go's `internal/providers/errorclassify.go`

- 12 text rules (Lintasan B1 = 9). Ekstra empat:
  - `resource_exhausted` (Backoff)
  - `resource has been exhausted` (Backoff)
  - `model_capacity_exhausted` (Backoff)
  - `server is temporarily unavailable` (Backoff)
- Status rules: 401, 402, 403, 404, 429 (Backoff) + **502, 503, 504
  (all Backoff)** — Lintasan B1 punya 429 Backoff saja, 502/503/504 unmatched
  → no-fallback caller-default.
- CooldownMs semantics: Backoff+1 → 2s/4s/8s... capped 5 min. Sama persis
  dengan Lintasan (`BackoffBase=2s`, `BackoffMax=5m`).
- TransientCooldownMs = 30s untuk unmatched **bukan request-scoped 4xx**
  ( ada explicit 4xx-only "skip" branch. Lintasan B1 failopen untuk semua
  unmatched — ini DESAIN SAMA (fail-open), tetapi punya parameter berbeda:
  9router-go default 30s cooldown kalau status >= 500 (server transient);
  Lintasan B1 memilih caller default — di proxy.go itu 60s pool default,
  bukan 30s.

Probed: confirmed by 9router-go HEAD, 2026-10-18.

### 2.2 Patch ke Lintasan B1 commit f724d4b

Adopted 4 rules + 3 status in-place:

  - `resource_exhausted` / `resource has been exhausted` /
    `model_capacity_exhausted` / `server is temporarily unavailable` →
    Backoff (Bump b/Backoff B1 hit "rate limit" as the only backoff text rule
    → upstream's "resource_exhausted" beta endpoints actually back out).
  - 502/503/504 Backoff — the 9router-go convention. Production Lintasan
    hot-path (internal/server/proxy.go:998) already backoff hints from
    Retry-After; adding these gives the same shape when the upstream never
    sends that header (just 503 from a broken gateway — no more parking a
    key for the fixed 60s default).

  B1's fail-open invirtual: unmatched NON-request-scoped 4xx still falls
  through to the caller's timed default (internal/server/proxy.go recordMultiAccountResultClassified
  runs the B1 table, and only matches the classifier's own cooldown) — so
  production is NOT affected beyond adopting the upstream's extra table
  entries. Difference stays in favor of fail-open.

### 2.3 Not adopted (kept Lintasan's own)

  1. TransientCooldownMs=30s — 9router-go covers unmatched NON-4xx with it.
     Lintasan B1 fail-open instead. Rationale: Lintasan's record-
     result is the *pool pipeline*, where a 60-second pool default already
     has a working answer directly (429-Retry-After header + Qoder 10605
     hint). Falling back prematurely ALSO burns a request per transient
     blip, which is Lintasan's existing design guard. This is a documented
     DEVIATION, not an oversight.
  2. `TransientCooldownMs` UpdateStrategy (single collector in 9router-go)
     — Lintasan hasn't needed this yet; a separate card when we ship a
     second client for the classifier.

---

## 3. Perbandingan langsung — opencode-zen (lens A7 review of A7)

### 3.1 Di 9router-go

- `internal/proxy/executor/opencode_zen.go` 607 lines, THREE-LANE executor:
  forwardZenChat / forwardZenMessages / forwardZenResponses; routes
  by `zenTargetFormat(ctx, model)` from `model_formats.go`.
- Free-tier vs keyed lane decided by `IsOpencodeFreeTierModel(provider,
  baseModelID)` (both curated list + `strings.HasSuffix(modelID, "-free")`),
  → free requests travel to `keyless /inference/v1` with `Bearer public`,
  keyed requests go to `zenBaseURL` (registry default) with the user key.
  This matches 9router v0.5.104's free tier for `/zen/v1` (#4842.
- `internal/translator/opencode_v2_fingerprint.go` 421 lines — emulates
  upstream's /inference/v1 fingerprint, aliases caller tools named
  `read`/`shell` to `read_client`/`shell_client` + injects exact definitions
  + makes tool_choice deterministic.
- `internal/translator/fingerprint.go` 630 lines — legacy /zen/v1 quartet
  `ConcealFingerprintTools`: only injects `read` (basis: live probe proves
  `read` alone passes), renames case-variants (Bash→bash etc.), dedupe
  identical names, retarget tool_choice & prior-toolcall references.
- Tests: `internal/proxy/executor/opencode_zen_test.go` 28.1K + `.../relay_test.go`
  3.2K; `internal/translator/fingerprint_test.go` 17.6K + `opencode_v2_fingerprint_test.go`
  16.9K. Integration coverage: `internal/integration/opencode_live_keyless_test.go`
  (runs only with `-tags=integration`, calls upstream LIVE).

### 3.2 Di Lintasan

Lintasan `internal/opencodezen/` = 9 tests / ~290 lines HEAD. Ide bagus
karena (a) test yang saya punya covers fingerprint bahkan (deterministic
bytes, counter shape, UA rule) tapi (b) file saya tidak handle:
  - v2 openCode /inference gate (rename-alias path, not just inject);
  - messages lane (native Claude body passthrough) → dia packages a
    `zenShapedBody` by destination gate;
  - responses lane (`/zen/v1/responses`) → also needed by 9router #4842.

Keputusan: keep Lintasan's current pure-function helpers (valuable in
itself), but mark `internal/opencodezen` as A7-partial in the parity
plan doc and add the 3 missing behaviours as next follow-up cards, NOT
a re-implementation of someone else's executor.

### 3.3 Yang jelas ikut dari ref (copied-style, no modification)

  - `HasValidOpencodeVersion` — pin UA acceptance `opencode/1.x` minor>=17.
  - `IsOpencodeFreeTierModel(provider, baseModelID)` from
    `model_formats.go` — worth porting as-is_yellow (list-based, not a
    static slice — but fits Lintasan's `internal/models` surface).

Not directly adopted:
  - Direction: 9router-go's `zenShapedBody` switching by gate is a nicer
    shape than my inject-then-transmute path, but changing it would not
    produce different observable behaviour — defer.

---

## 4. Apa yang harus dilakukan setelah ini (rubri 5 keputusan lama)

1. B1 : updated notes in-place (this file) — already adopted in
   `internal/lb/classify.go` tested.
2. A6 (glm): two preset lanes shipped (commit 44f5c05) + oauthide catalog.
   SAME SHAPE as 9router-go's `internal/proxy/executor/zai.go` (6.7K) —
   worth reading it for the dual-auth OAuth+apikey pattern before Lintasan
   invests in GLM-codegen adapter.
3. A7-note: keep `internal/opencodezen/` pure-fn; mark "A7 partial" and
   defer `zenShapedBody`-style gate dispatch until we have a messages
   caller worth the interpreter overhead.
4. A5 (devin-cli): ported via descriptors (commit fc0d50b) — no overlap
   with 9router-go (they use stdio/metadata; Lintasan ships via Cohort-A
   ACP).
5. Lens C (nous refresh): `9router-go` v1.9.14 does NOT have a 9router
   nous port (searched — only portal oauth base without mint); skip.

---

## 5. Bukti (evidence pointers)

- 9router-go source: /tmp/9router-go-ref (shallow clone, v1.9.14-exp.5).
- B1 table updated here: internal/lb/classify.go (richer text + status
  rules) — pins 9router-go's table in its own test suite.
- `.changes/*.md` convention: worth copying as Lintasan's own practice
  per PR (56 fragments as of v1.9.14-exp.5) — already documented in
  docs/plans/2026-10-18-9router-parity.md.
- `TestConcealOpenCodeV2CollidingNamesStillAppendExact` (upstream #320) and
  `TestZenShapedBodyFollowsTheDestinationGate` are the two tests that
  make their opencode-zen port RECORDABLE — Lintasan tests currently
  pin the same shape via the internal/opencodezen suite.
- License: MIT, both upstream (decolua/9router) and this fork — attribute
  in our file headers for any code that is a straight port.

## 6. Tidak di-commit (berhenti apa yang di atas)

Only the B1 table tuning is a behavioural change from this reference; the
rest (2), (3), (5) are READ-ONLY reviews and were NOT committed — the
dispatch intentionally does not cross the "no silent re-implementation"
safety rule.

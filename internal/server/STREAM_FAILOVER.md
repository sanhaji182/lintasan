# Streaming failover for in-stream provider failures

**Fixed 2026-09-23** across five commits (`5bb6106`, `2408df5`, `f976433`, `66372fd`,
`da91500`), deployed as `v0.29.3-174-gda91500`.

## The defect

The streaming branch committed the response before reading a single upstream frame:

```go
w.WriteHeader(resp.StatusCode)   // committed here
... read frames ...
```

Once that runs, the status is on the wire and every `continue` in the candidate loop is
unreachable — Go cannot take a status back. That is harmless for a provider that reports
failure in the status line, because those failures are handled before the commit. It is
fatal for a provider that reports failure **inside a 200**, which this codebase already
documented in a comment:

> Qoder reports a refused credential, a busy model and content moderation all INSIDE a
> stream that opened with HTTP 200, so a caller that only inspects the status code sees
> a healthy stream that never produces content.

Observed live, on an account refusing with code 105:

```
data: {"error":{"message":"upstream refused this credential ... (code 105)"}}
data: {"error":{...the same frame, twice...}}
data: [DONE]
```

Two defects in that transcript: the error frame was duplicated, and no retry happened
even though `code 105` is classified retryable ("7 of 9 credentials served normally
again after reporting it").

## Why it took four attempts

Each of these was a real bug, and each was ONLY visible from a live request — the test
suite was green after every one of them:

1. **`5bb6106` deferred the header but the Qoder streamer still wrote to the client
   directly**, so the gate never saw a write and `Retryable()` stayed true while the
   client had already been served. Fix: route every Qoder write through the gate.
2. **The 2xx status was still written eagerly at the branch head**, which set
   `statusWritten` and made the gate report "not retryable" — the fix was present and
   did nothing. Fix: a 2xx is committed only by a client-visible write.
3. **The candidate list holds exactly one connection** for a plain model route, so the
   retry had nowhere to go: `all routes failed`, `candidates=1`. Fix: widen via
   `findAlternateConnectionsForModel` on the discarded-attempt branch, exactly as the
   401/403 path already did.
4. **A stalled non-streaming read had no deadline** (pre-existing, found while
   verifying): HTTP 200 then no body held the request until the *client* cancelled —
   120,002 ms recorded as "context canceled". Fix: `readUpstreamBodyBounded`.

The lesson worth keeping: **verify the behaviour, not the code path.** Each time the
implementation looked right in review and the tests passed; only a real request against
a real refusing account showed it did nothing. A fix whose effect is "a retry happens"
must be observed retrying.

## What now happens

Measured live on the code-105 account, after `66372fd`:

```
5ec756d7  502   533ms  qoder upstream error (status 403)   attempt 1 refused
7cfe0584  502   887ms  qoder upstream error (status 403)   attempt 2 refused
d8eddefa  502  1246ms  qoder upstream error (status 403)   attempt 3 refused
a8cf81b6  200  4357ms                                     attempt 4 served
```

The client received 11 OpenAI chunks, `finish_reason` set, `[DONE]`, real content and
reasoning — and **no error frame**, because it never saw a failed attempt.

## Invariants

* A 2xx response is committed **only** by a client-visible write through
  `streamCommit`. Never by the branch head, never by a provider's own bookkeeping.
* An attempt may be discarded **only** while nothing has been written. Retryability in
  principle comes from `qoder.DescribeStreamError`; retryability in practice comes from
  the commit state. The two are deliberately not merged — an unclassified error must not
  walk the pool, and `Retryable()` alone must not decide failover.
* Held frames belong to the attempt that produced them. `Abandon` drops them on retry;
  `Flush` releases them on success. Flushing a failed candidate's frames into the next
  candidate's stream would splice two upstreams together.
* `commandcode` is deliberately ungated: its translator owns the client writes and never
  reports a retryable failure. It writes its own status because the old single eager
  `WriteHeader` supplied it and set no SSE headers — removed by mistake in `5bb6106` and
  caught by enumerating the format branches rather than by a test.

## Tests

`stream_commit_test.go` (15), `stream_commit_qoder_test.go` (5),
`stream_commit_timeout_test.go` (4). The ones that matter:

* `TestQoderRolePreambleDoesNotCommit` — drives the exact observed refusal shape through
  the real frame parser and asserts the attempt is still retryable and nothing was
  written. This is the regression pin for the whole change.
* `TestAbandonDropsTheFailedCandidatesHeldFrames` — the correctness half of the gate.
* `TestPumpKeepAlivesDoNotCommit` — a chatty upstream must not re-create the bug.
* `TestReadUpstreamBodyBoundedTimesOutAndReleases` — proves the goroutine is released by
  closing the body, not leaked.
* `TestHandleStreamFailureDoesNotRetryUnclassifiedError` — the pool must not be walked.

Note what is NOT covered by tests: that a retry actually reaches a second account. That
needs a live refusing credential, which is why the measurement above is part of the
record rather than a unit test.

# Provider SDK (v2.4 foundation)

> **Status: FOUNDATION COMMIT.** This package is planted *parallel* to the live
> implementation. Nothing in Lintasan imports it yet, so it cannot change any
> runtime path or behavior — the Go linker strips it from the binary as dead
> code. It exists so v2.4 has a stable, reviewed surface to build on, with zero
> risk to the system that is already live.

## Why this exists

Today the proxy decides how to talk to each upstream by switching on a
connection's `Format` field inside `internal/server/proxy.go` (`doUpstream`):

```go
if conn.Format == "commandcode" {
    body = transformForCommandCode(body, thinkingMode)
    // ...special headers, special URL, special response translation...
}
```

Every new upstream shape means another branch in the router. The Provider SDK
replaces that format-switch with a small per-provider contract: adding a
provider becomes *a new file + one `Register()` call*, with **zero router edits**.

## The contract in one breath

- A `Provider` turns a canonical (OpenAI-shaped) `Request` into an
  `UpstreamRequest` (`Prepare`), and turns raw upstream bytes back into a
  canonical `Response` (`Translate`). **It does not make the HTTP call itself.**
- The **router owns the HTTP call**, so reliability (circuit/retry/fallback/
  hedge) and shared post-processing (reasoning extraction, normalization) wrap
  the provider from the *outside*. Providers stay thin; the reliability layer
  stays reusable. This is the decorator model.
- Providers self-register in a `Registry`. Unknown names fall back to a generic
  OpenAI-compatible `DefaultProvider` — the migration safety net that lets the
  SDK coexist with unmigrated connections.
- Optional capabilities (`Embedder`, `CredentialRefresher`, `StreamTranslator`)
  are separate small interfaces a provider *may* additionally implement. No
  god-interface; the router type-asserts.

## Files

| File | Responsibility |
|------|----------------|
| `doc.go` | Package overview (godoc) |
| `types.go` | `Provider` interface, `Request`/`UpstreamRequest`/`Response`/`ConnConfig`, `Track`, optional interfaces |
| `capability.go` | `Capability` constants, `CapabilitySet` (declaration surface only) |
| `registry.go` | `Registry` + package-level `Register`/`Get`/`Resolve`/`Names` |
| `default_provider.go` | `DefaultProvider` — generic OpenAI-compatible fallback (faithful to live router) |
| `dispatch.go` | `Dispatch()` — router-facing entrypoint that injects the HTTP transport |
| `errors.go` | Sentinel errors (`errors.Is`-matchable) |
| `provider_test.go` | 20 unit tests |
| `example_test.go` | Runnable `Example()` (godoc-verified output) |

## What this is NOT (yet)

Not wired into the proxy. No migrated providers, no new providers, no feature
flag, no schema change. Capability-based routing and experimental-provider
isolation are *declared* in the contract (so the shape is stable) but are
deliberately **not implemented** here — those are later, separately-approved
steps. Wiring this into the router is its own change with its own review.

## Mapping to the live router (for the future wiring step)

| SDK surface | Lives today in |
|-------------|----------------|
| `Provider.Prepare` | `doUpstream` URL/auth/header build + `transformForCommandCode` branch |
| `Provider.Translate` | `translateCCAlphaToOpenAI` + per-format response massaging |
| `Dispatch`'s injected `httpDo` | the `retry.Do` / circuit-breaker wrapping around `doUpstream` |
| router post-processing after `Translate` | `reasoning.ExtractReasoningContent` + `normalizeOpenAIResponseBody` (stay shared) |
| `ConnConfig` | the `Connection` row (identical fields — no schema change) |
| `Registry.Resolve(name, fallback)` | the implicit "no special handling" default path |

## Run it

```bash
go test ./internal/provider/        # 20 unit tests + 1 example
go vet  ./internal/provider/
```

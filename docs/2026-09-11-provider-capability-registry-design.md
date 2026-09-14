# Curated Provider Capability Registry

**Status:** Approved 2026-09-11

## Goal

Make Add Connection trustworthy: only verified providers appear in the primary catalogue, every preset carries its real connection metadata, and model/usage capabilities are stated honestly.

## Product decisions

1. The primary catalogue is curated. Existing connections are never removed or disabled by catalogue changes.
2. A provider qualifies when its endpoint/auth/model-discovery contract has evidence and can be exercised by the connection test.
3. Real usage is optional. Providers without a public usage API remain eligible, but display `Usage not provided by provider` rather than an error or fabricated usage.
4. Save remains gated by a successful test. A successful save automatically syncs models and reports partial post-save failures clearly.

## Architecture

Extend `provider_presets` with connection metadata and capability/evidence fields. A typed catalogue in Go is the source for built-ins; the seeder refreshes all built-in metadata in place while preserving manual presets. The dashboard copies the complete preset contract into the connection form and sends it to both Test and Save.

Capability values:

- `models`: `supported` or `unsupported`
- `usage`: `full`, `rate_limits`, or `not_provided`
- `verification_status`: `verified` or `unverified`
- `verified_at`: date of latest endpoint/contract verification

The primary Add Connection list shows only built-ins marked verified with model discovery supported, plus user-created custom presets. Capability badges explain what is available.

## Initial curated cohort

The initial cohort is intentionally conservative and based on current implementation/evidence: OpenAI, Anthropic, Google AI, xAI, Mistral, DeepSeek, Qwen, Moonshot, Zhipu, Groq, Together, Fireworks, OpenRouter, Perplexity, Ollama, Xiaomi MiMo, NVIDIA, Cerebras, Kilo Gateway, TokenRouter, SambaNova, SiliconFlow, DeepInfra, Jina AI, Baseten, Novita AI, CommandCode Alpha, and existing custom presets whose owner explicitly created them.

Provider-specific metadata must override generic path derivation. Anthropic uses `x-api-key` plus `anthropic-version`; Gemini uses its OpenAI-compatible surface for this connection flow; CommandCode Alpha uses `/alpha/generate` and `/provider/v1/models`.

## Error handling

- Unsupported usage is a successful capability result, not an HTTP/provider failure.
- Authentication, malformed response, unavailable endpoint, and timeout remain errors.
- If connection creation succeeds but model sync fails, keep the connection and show a warning with a retry action; never roll back or delete credentials silently.
- Unknown custom providers use generic OpenAI-compatible paths and `usage=not_provided`.

## Verification

- Migration/backfill tests for existing databases.
- Catalogue contract tests: every curated preset has paths/auth/capabilities/evidence.
- Provider-specific tests for Anthropic, Gemini/OpenAI-compatible, CommandCode Alpha, and standard `/v1` providers.
- Handler tests proving test and save consume identical metadata.
- Frontend build and source/DOM behavior checks.
- Full Go suite and production-like isolated smoke before any deployment.

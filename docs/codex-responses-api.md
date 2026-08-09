# Codex Responses API (`/v1/responses`)

Experimental lab. **Off by default.**

Lintasan speaks OpenAI's `/v1/responses` shape (used by Codex CLI / the Codex
IDE plugin) by translating it to/from the existing `/v1/chat/completions`
pipeline. No new provider integration, no new routing table — every model
Lintasan already serves via chat completions is reachable through this
endpoint too.

## Enable

Dashboard: **Settings → Experimental → Codex Responses API**. Flips instantly,
no restart — the gate is read from the DB on every request
(`internal/server/responses_gate.go`).

Equivalent via API — `PUT /api/settings` takes a `{key: value}` object and
accepts a JSON bool:

```bash
curl -X PUT localhost:20180/api/settings \
  -H "Authorization: Bearer $MASTER_KEY" \
  -H "Content-Type: application/json" \
  -d '{"responses_api_enabled": true}'
```

With the flag off, `POST /v1/responses` returns `404` regardless of request
shape — including `stream=false` (M6 is not a way around the gate).

## Request

```
POST /v1/responses
Authorization: Bearer <master_key>
Content-Type: application/json
```

| field | required | notes |
|---|---|---|
| `model` | yes | any model Lintasan already routes for chat completions |
| `input` | yes | string, or array of input items |
| `stream` | no | `true` (default) → SSE. `false` → single JSON body (M6). Any non-bool value is treated as absent. |
| `tools` | no | OpenAI function-tool shape; Codex's heterogeneous array (function + provider built-ins) is normalized — see Tools below |

Input items (`input: [...]`):

| `type` | handling |
|---|---|
| `message` (or omitted) | `content` may be a string, or an array of `input_text`/`output_text`/`text` and `input_image`/`image_url`/`image` parts |
| `function_call` | requires `call_id` |
| `function_call_output` | requires `call_id`, matched back to the corresponding `function_call` |

A malformed item, a request missing `model`/`input`, or a `function_call*`
item without `call_id` is a `400` before anything reaches the upstream
(`internal/translator/responses.go: ValidateResponsesRequest`).

### Tools

Codex sends a heterogeneous array — real callable functions alongside
provider built-ins the chat API can't represent (e.g. `web_search`). Those
built-ins are dropped, not forwarded as broken function defs; each drop
increments `lintasan_responses_tools_dropped_total` so a shrunk tool list is
visible in `/metrics` instead of silently changing model behavior.

## Response

**Streaming** (`stream: true` or absent) — SSE, Codex's own event vocabulary:
`response.created`, `response.output_item.added`, `response.output_text.delta`,
`response.function_call_arguments.delta`, `response.output_item.done`,
terminal `response.completed` / `response.failed` / `response.incomplete`.

**Non-streaming** (`stream: false`, M6) — single JSON body, `Content-Type:
application/json`, shaped identically to the `response` object carried by the
streaming path's terminal event:

```json
{
  "id": "resp_...",
  "object": "response",
  "status": "completed",
  "model": "gpt-oss-120b",
  "output": [
    {"id": "msg_...", "type": "message", "status": "completed", "role": "assistant",
     "content": [{"type": "output_text", "text": "...", "annotations": []}]},
    {"id": "fc_...", "type": "function_call", "status": "completed",
     "call_id": "...", "name": "...", "arguments": "{...}"}
  ],
  "usage": {"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
}
```

Rules that hold for both paths:

- a `message` item exists **only** when there is assistant text — a pure
  tool-call turn emits no empty message item;
- `call_id` is preserved verbatim from the upstream tool call;
- `arguments` is always a JSON string, `"{}"` when the upstream omits it;
- a tool call with no `id` is dropped (it could never be answered with a
  matching `function_call_output`), not fabricated.

A non-2xx upstream response is passed through verbatim on the non-streaming
path (same status, same body) — an auth failure or rate limit reaches the
client as itself, not as a translation error. An upstream body with no usable
choice is `502` (`ErrResponsesBadUpstream`), never a hollow `"completed"` turn.

## Metrics

All counters gated by the standard `/metrics` switch (no prompt content, no
`call_id` values):

| metric | meaning |
|---|---|
| `lintasan_responses_streams_started_total` | requests where `response.created` was emitted (streaming + non-streaming both count here) |
| `lintasan_responses_streams_completed_total` | terminated successfully |
| `lintasan_responses_streams_failed_total` | terminated by upstream/translation error |
| `lintasan_responses_streams_incomplete_total` | truncated/partial |
| `lintasan_responses_tool_calls_total` | `function_call` items emitted |
| `lintasan_responses_text_streams_total` | turns that produced any assistant text |
| `lintasan_responses_tools_dropped_total` | unrepresentable `tools[]` entries dropped |

Structured stderr log, one line per request:

```
lintasan.responses ingress=responses           model="..." started=true terminal=completed tool_calls=0 had_text=true   # streaming
lintasan.responses ingress=responses-nonstream model="..." started=true terminal=completed tool_calls=0 had_text=true   # M6
```

## Status

| milestone | scope | state |
|---|---|---|
| M0 | route skeleton, request validation | shipped |
| M1 | request translation (Responses → chat) | shipped |
| M2 | response translation contract, streaming | shipped |
| M3 | SSE event emitter | shipped |
| M4 | metrics + structured logging | shipped |
| M5 | heterogeneous `tools[]` translation | shipped |
| M6 | non-streaming (`stream: false`) JSON path | shipped |

Everything above is behind the kill switch and OFF in production by default.

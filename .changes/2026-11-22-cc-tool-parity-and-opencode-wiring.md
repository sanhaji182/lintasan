# 2026-11-22 — CommandCode tool round-trip parity + OpenCode lane wiring

## Problem

`core` (CommandCode Alpha alias via provider `lintasan`) failed with an empty-body
HTTP 400 on any Hermes session carrying tool history. Fixing that exposed two
further silent failures: outbound tool declarations were dropped, and inbound
`tool-call` SSE events were discarded, so `finish_reason=tool-calls` arrived
with `tool_calls: null`.

Separately, `internal/opencodezen` and `internal/zed`-era pure-function ports
were referenced by zero production files — code existed but no connection could
benefit.

## Why the naive fix is wrong

- Treating the empty 400 as an outage leads to switching aliases (`hemat`) —
  that only masks a request-shape bug. The real body (visible with
  `stream:false`) is `expected one of "user"|"assistant" at params.messages[N].role`.
- The 9router-go fork comment says `role:"tool"` + a `tool-result` block is the
  correct current shape and that fold-to-user was "superseded". A direct probe
  of the live endpoint disagreed: fold-to-user returned 200, the fork's shape
  produced an in-band `server_error`. Live behaviour wins over a stale comment.
- Sending native OpenAI `tools` verbatim to CommandCode is rejected
  (`expected string at params.tools[0].name`). It must be projected to
  `{name, description, input_schema}`.

## What changed

| Commit | Change |
|---|---|
| `ce62f1b` | Fold OpenAI `role:tool` results into a user-role message in `transformForCommandCode`; tests `cc_tool_roles_test.go` |
| `2f6acde` | Port 9router `convertTools`: project OpenAI tools to CommandCode schema; `cc_tools_test.go` |
| `dd66965` | Translate CC `tool-call` events into OpenAI `tool_calls` (streaming + non-streaming); `cc_tool_calls_response_test.go` |
| `2414575` | `opencode_zen_enabled` (default OFF) wires `PrepareHeaders` + fingerprint tools for `format=opencodezen`; `opencodezen_wiring*.go` |
| `7d624eb` | `opencode_go_enabled` (default OFF) attaches the required `x-opencode-session` on `opencode.ai` connections; `opencode_go_wiring*.go` |
| (subsequent docs-only commits) | This fragment + the `AGENTS.md` §10c pointer; no Go changes, so the binary stamp legitimately stays `v0.29.3-299-g7d624eb` (`7d624eb`). |

## Tests that pin it

- `TestTransformForCommandCode_ToolRolesFoldedIntoUser` — no `tool` role leaks into `params.messages`
- `TestTransformForCommandCode_ProjectsOpenAITools` — OpenAI wrapper becomes `{name,description,input_schema}`
- `TestTranslateCCAlphaToOpenAI_ToolCalls` / `TestPipeCCAlphaStreamToOpenAI_ToolCalls` — `tool_calls` with JSON arguments
- `TestPrepareOpencodeZenRequest`, `TestOpencodeZenFingerprintToolsInjected` — header/fingerprint contract
- `TestOpenCodeGoSessionIDShape`, `TestOpenCodeGoHeadersApplied` — `ses_` + 32 hex, deterministic, native reuse

Full internal suite: 1864 pass, 0 fail.

## Reproduction commands

```bash
# The original 400 (old binary) → 200 (new), 41 msgs / 18 tool-role msgs
curl -s localhost:20180/health   # expect v0.29.3-299-g7d624eb

# Native tool call with tool history
python3 - <<'PY'
import sqlite3, json, urllib.request, time
db='/home/ubuntu/lintasan-go/data/lintasan.db'
c=sqlite3.connect('file:'+db+'?mode=ro',uri=True,timeout=3)
key=c.execute("select value from settings where key='master_key'").fetchone()[0]
tools=[{'type':'function','function':{'name':'get_weather','description':'Get weather',
  'parameters':{'type':'object','properties':{'city':{'type':'string'}}}}}]
d={'model':'core','messages':[{'role':'user','content':f'Call get_weather for Jakarta. {time.time_ns()}'}],
   'max_tokens':160,'tools':tools}
r=urllib.request.Request('http://127.0.0.1:20180/v1/chat/completions',
  data=json.dumps(d).encode(), headers={'Authorization':'Bearer '+key,'Content-Type':'application/json'})
with urllib.request.urlopen(r,timeout=90) as resp:
    b=json.loads(resp.read().decode()); print(b['choices'][0]['finish_reason'], b['choices'][0]['message']['tool_calls'])
PY
```

Expected: `tool-calls` with `function.name=get_weather` and JSON arguments.

## Deployment evidence

Live `v0.29.3-299-g7d624eb`; PID PPid=1, NRestarts=0, disk sha == `/proc/PID/exe` sha
(`656a0dcb8b1d0cc61c2e768e`), `main == origin/main`, tree clean, `vcs.modified` clean.
Both new flags are absent from `settings` (0 rows) — wiring is inert until opted in.
50/50 sequential probes of the original failing payload: 100% success (p50 2.07s, p95 4.3s).

## Not proven / deliberate non-actions

- OpenCode Zen / Go live 200 blocked by credentials: Zen free lane 429
  `FreeUsageLimitError`, Go key 401 `api_key_expired`. The Go-specific 400
  `MissingSessionID` was reproduced and the fix targets exactly that.
- A8 `mimo-free` was NOT wired: upstream registry now says the free channel
  ended and marks it `hidden: true`; bootstrap endpoint returns 404.
- A3 Windsurf / A4 Zed require real OAuth tokens; pure encoders alone cannot be
  validated, so they were left unwired.
- Rollback binary: `lintasan.bak-20261122-161904`; DB snapshot:
  `data/lintasan-pre-cc-toolroles-20261122_160001.db`.

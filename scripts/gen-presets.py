#!/usr/bin/env python3
"""gen-presets.py — B3 of the 9router parity plan (docs/plans/2026-10-18-9router-parity.md).

Cross-references Lintasan's built-in provider presets against the 9router
provider registry (open-sse/providers/registry/*.js) (v0.5.104 source tree)
and prints:
  - which OpenAI-compatible chat providers 9router knows that Lintasan's
    seedCatalogue() does NOT yet have (missing presets, one line each, in a
    Go-literal-ready form),
  - which Lintasan presets no longer correspond to anything in 9router
    (stale, informational only — Lintasan's preset table is hand-curated and
    may legitimately carry endpoints 9router dropped).

Run mode:
  ./scripts/gen-presets.py                # against /tmp/9router-src
  REGISTRY_DIR=path ./scripts/gen-presets.py

Design note: this is a CHECKER (seed diff), not a generator — a wrong preset
that quietly lands in the DB breaks routing. A real "generate" step would
still require the operator to probe each base URL (GET /models must answer)
before inclusion; that probe gate lives in parity-plan §4 B3 and is enforced
below by only printing candidates, never writing files.
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path

# Default: upstream clone this plan was written against.
REGISTRY_DIR = Path(os.environ.get("REGISTRY_DIR", "/tmp/9router-src/open-sse/providers/registry"))

# Hand-curated current seedCatalogue() (internal/server/handlers_presets.go).
# Keep in sync with that file manually — this script READS it, never writes it.
LINTASAN_BUILTIN_HOSTS = {
    # foundation
    "api.openai.com", "api.anthropic.com", "generativelanguage.googleapis.com", "api.x.ai", "api.mistral.ai",
    # open (cn + intl)
    "api.deepseek.com", "dashscope.aliyuncs.com", "dashscope-intl.aliyuncs.com", "coding.dashscope.aliyuncs.com",
    "coding-intl.dashscope.aliyuncs.com", "api.moonshot.cn", "open.bigmodel.cn", "api.ai21.com", "api.cohere.ai",
    # inference
    "api.groq.com", "api.together.xyz", "api.fireworks.ai", "integrate.api.nvidia.com", "api.sambanova.ai",
    "api.deepinfra.com", "api.jina.ai", "inference.baseten.co", "api.novita.ai", "api.venice.ai",
    # aggregator
    "openrouter.ai", "api.replicate.com", "api.tokenrouter.com", "ai-gateway.vercel.sh",
    # search
    "api.perplexity.ai",
    # local
    "localhost",
    # coding
    "api.xiaomimimo.com", "token-plan-sgp.xiaomimimo.com", "api.qoder.com", "opencode.ai", "llm.kimchi.dev",
    "inference.poolside.ai", "api.siliconflow.com", "api.hunyuan.cloud.tencent.com", "ark.cn-beijing.volces.com",
}

# Providers that 9router routes to over a NON-OpenAI transport, an OAuth-only
# flow, a protobuf/EventStream wire, or stdio — a preset alone cannot represent
# them, they need the adapter work in lens A. Listed so the missing-preset
# output doesn't flag them as actionable.
NON_PRESET_KINDS = {
    "kiro", "cursor", "trae", "windsurf", "zed", "devin-cli", "glm",
    "antigravity", "gemini-cli", "grok-cli", "grok-web", "perplexity-web",
    "qoder", "qoder-cn", "minimax-code", "minimax-code-global",
    "muse", "nous", "iflow", "ollama-local", "commandcode",
    # media-only providers (tts/stt/search/image) — not chat
    "assemblyai", "deepgram", "elevenlabs", "aws-polly", "edge-tts",
    "google-tts", "cartesia", "inworld", "playht", "fish-audio", "coqui",
    "brave-search", "exa", "firecrawl", "google-pse", "jina-reader",
    "linkup", "searchapi", "searxng", "serper", "tavily", "ollama-search",
    # image / video generation
    "black-forest-labs", "comfyui", "fal-ai", "recraft", "sdwebui",
    "stability-ai", "nanobanana", "runwayml",
    # webhook / relay-specific, not a preset
    "netlify-relay", "bazaarlink", "dahl", "local-device", "mmf",
    "perplexity-agent", "kilo-gateway",
    # curated-list entries already in Lintasan (skip duplicates here)
    "xiaomi-mimo", "opencode-go", "opencode-zen", "kimchi", "mimo-free",
    "groq", "openai", "anthropic", "deepseek", "xai", "mistral",
    "perplexity", "openrouter", "ollama",

    # Media/sub-kind providers that use a *different endpoint family* from
    # chat; a generic "openai chat preset" would route by accident rather
    # than by model — same reason Lintasan excludes Voyage AI today.
    "selfhosted-embedding", "selfhosted-stt", "selfhosted-tts",
    "tortoise", "xquik", "youcom", "tinyfish",

    # Endpoint templates with literal {region} / {accountId} substitution in
    # the baseUrl — Lintasan's connection form does not support templates
    # yet, so a preset would carry an unusable literal. Lens A work only.
    "bedrock", "bedrock-xai", "vertex", "vertex-partner", "cloudflare-ai",

    # OpenAI's OWN Responses-only Codex endpoint is not a generic chat
    # preset; Lintasan exposes the Responses surface via responses_api_enabled.
    "codex",

    # Copilot/GitLab chat live behind OAuth device flows — they need the
    # internal/oauthide side (already ported for github/copilot).
    "github", "gitlab", "kilocode", "cline", "clinepass",

    # Minimax family speaks Anthropic wire on /anthropic/v1/messages; the
    # preset would need Format:"anthropic" and has no OpenAI lane.
    "minimax", "minimax-cn",

    # Kimi coding-plan endpoint is /coding/v1/messages (Claude wire).
    "kimi",
}

ENTRY_RE = re.compile(r"^export default \{|^\s*id:\s*\"([^\"]+)\"|^\s*alias:\s*\"([^\"]+)\"", re.M)
ID_RE = re.compile(r'^\s*id:\s*"([^"]+)"', re.M)
BASEURL_RE = re.compile(r'^\s*baseUrl:\s*"([^"]+)"', re.M)
FORMAT_RE = re.compile(r'^\s*format:\s*"([^"]+)"', re.M)


def host_of(url: str) -> str:
    m = re.match(r"^https?://([^/]+)/", url + "/")
    return m.group(1) if m else url


def inspect_registry() -> list[dict]:
    if not REGISTRY_DIR.is_dir():
        sys.exit(f"registry dir not found: {REGISTRY_DIR} (clone github.com/decolua/9router to /tmp/9router-src)")
    out = []
    for path in sorted(REGISTRY_DIR.glob("*.js")):
        if path.name in ("index.js", "REGISTRY_TEMPLATE.js"):
            continue
        text = path.read_text()
        entry = {
            "file": path.name,
            "id": (ID_RE.search(text) or [None, None])[1],
            "baseUrl": (BASEURL_RE.search(text) or [None, None])[1],
            "format": (FORMAT_RE.search(text) or [None, None])[1],
        }
        if not entry["id"]:
            continue
        out.append(entry)
    return out


def main() -> None:
    entries = inspect_registry()
    missing, stale = [], []
    lintasan_hosts = LINTASAN_BUILTIN_HOSTS
    seen_lintasan = set()
    for e in entries:
        eid, url = e["id"], e["baseUrl"]
        if not url or url.startswith("devin://"):
            continue
        # Preserve domain (host) of the baseUrl so any provider port can be
        # matched against what Lintasan already ships.
        host = host_of(url)
        in_lintasan = any(h and host == h for h in lintasan_hosts)
        if eid in NON_PRESET_KINDS:
            if in_lintasan:
                seen_lintasan.add(eid)
            continue
        if in_lintasan:
            seen_lintasan.add(eid)
            continue
        # Actionable: OpenAI-compatible chat provider with a baseUrl that
        # Lintasan is missing. Print Go literal form so the operator can 1:1
        # paste into seedCatalogue() after the manual GET /models probe.
        name = eid.replace("-", " ").title() if eid else ""
        missing.append(
            f'\t\t{{Name: "{name}", Domain: "{host_of(url)}", BaseURL: "{url}", Format: "openai", KeyLabel: "API Key", Category: "open"}},'
        )
    for h in lintasan_hosts:
        if not any(host_of(e["baseUrl"]) == h for e in entries if e["baseUrl"]):
            # not in 9router at all — stale for THIS source version, informational
            stale.append(h)

    print(f"# source: {REGISTRY_DIR} ({len(entries)} entries)")
    print(f"# presets 9router has that seedCatalogue() does not (probe before adding):")
    for m in missing:
        print(m)
    if stale:
        print(f"\n# Lintasan hosts with no 9router v0.5.104 counterpart (informational):")
        for s in stale:
            print(f"#   {s}")


if __name__ == "__main__":
    main()

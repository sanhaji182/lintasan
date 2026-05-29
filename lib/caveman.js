// Caveman Mode — inject terse system prompt to reduce output tokens
// Three levels: lite (drop filler), full (fragments), ultra (abbreviations)
// Saves 20-65% output tokens depending on level
// Preserves: code blocks, file paths, commands, errors, URLs, security warnings

import { getSetting } from "./db/index.js";

export function getCavemanLevel() {
  return getSetting("caveman_mode", "off"); // off | lite | full | ultra
}

export function isCavemanEnabled() {
  return getCavemanLevel() !== "off";
}

const CAVEMAN_PROMPTS = {
  lite: `Response style: Be concise. Drop filler words, hedging, and pleasantries. Keep full sentences but make them short and direct. No "I think", "It seems like", "Let me", "Sure!", "Great question". Just answer.`,

  full: `Response style: Ultra-concise. Drop articles (a/an/the), filler, hedging. Use fragments over sentences. Short synonyms ("big" not "extensive", "use" not "utilize", "fix" not "resolve"). One sentence where others use a paragraph. Lists over prose. No pleasantries, no transitions, no meta-commentary.`,

  ultra: `Response style: Maximum compression. Abbreviations: DB/auth/config/req/res/fn/impl/deps/env/pkg/repo/dir/cmd/arg/param/val/err/msg/info/dev/prod/srv/lib/util/spec/doc. Arrows for causality (→). Fragments. One word when sufficient. No articles/filler/hedging/transitions. Code-like brevity for prose. Example: "Add auth middleware → check JWT → reject if expired. Config in env.AUTH_SECRET."

PRESERVE ALWAYS: code blocks, file paths, commands, error messages, URLs, security warnings, irreversible action confirmations.`,
};

// Inject caveman prompt into messages array
export function injectCavemanPrompt(messages) {
  const level = getCavemanLevel();
  if (level === "off" || !CAVEMAN_PROMPTS[level]) return messages;

  const cavemanText = CAVEMAN_PROMPTS[level];

  // Check if there's already a system message
  const hasSystem = messages.some(m => m.role === "system");

  if (hasSystem) {
    // Append to existing system message
    return messages.map(m => {
      if (m.role === "system") {
        return { ...m, content: m.content + "\n\n" + cavemanText };
      }
      return m;
    });
  } else {
    // Prepend new system message
    return [{ role: "system", content: cavemanText }, ...messages];
  }
}

// For Anthropic format (system is separate string)
export function getCavemanSystemAppend() {
  const level = getCavemanLevel();
  if (level === "off" || !CAVEMAN_PROMPTS[level]) return "";
  return "\n\n" + CAVEMAN_PROMPTS[level];
}

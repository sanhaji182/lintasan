/**
 * Content Filter Plugin
 * Blocks requests containing configurable banned words.
 */

import { shortCircuit } from "../lib/plugins.js";

// Configurable banned words list
const BANNED_WORDS = (process.env.CONTENT_FILTER_BANNED_WORDS || "")
  .split(",")
  .map((w) => w.trim().toLowerCase())
  .filter(Boolean);

// Default banned words if env var is not set
const DEFAULT_BANNED = ["hack-this-system", "ignore-all-instructions"];

const bannedList = BANNED_WORDS.length > 0 ? BANNED_WORDS : DEFAULT_BANNED;

function containsBannedWord(text) {
  const lower = text.toLowerCase();
  for (const word of bannedList) {
    if (lower.includes(word)) {
      return word;
    }
  }
  return null;
}

function checkMessages(messages) {
  if (!Array.isArray(messages)) return null;
  for (const msg of messages) {
    const content =
      typeof msg.content === "string"
        ? msg.content
        : Array.isArray(msg.content)
          ? msg.content.map((p) => (typeof p === "string" ? p : p.text || "")).join(" ")
          : "";
    const found = containsBannedWord(content);
    if (found) return found;
  }
  return null;
}

const contentFilter = {
  name: "content-filter",
  version: "1.0.0",
  enabled: true,
  priority: 5,

  hooks: {
    beforeRequest(ctx) {
      const found = checkMessages(ctx.messages);
      if (found) {
        console.warn(`[content-filter] Blocked request: banned word "${found}" detected.`);
        return shortCircuit({
          status: 403,
          body: {
            error: {
              message: "Request blocked by content filter.",
              type: "content_policy_violation",
              code: "content_filtered",
            },
          },
        });
      }
      return null;
    },

    afterRequest(ctx, response) {
      return response;
    },

    onError(ctx, error) {
      return null;
    },

    onStream(ctx, chunk) {
      return chunk;
    },
  },
};

export default contentFilter;

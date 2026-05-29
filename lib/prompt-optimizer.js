// Prompt Optimizer - reduce token usage by cleaning up messages
import { getSetting } from "./db/index.js";

/**
 * Check if prompt optimizer is enabled
 */
export function isPromptOptimizerEnabled() {
  return getSetting("prompt_optimizer_enabled", "false") === "true";
}

// Common verbose phrases and their concise replacements
const PHRASE_REPLACEMENTS = [
  [/I would like you to /gi, ""],
  [/I want you to /gi, ""],
  [/Can you please /gi, ""],
  [/Could you please /gi, ""],
  [/Please make sure to /gi, ""],
  [/Please ensure that /gi, "Ensure "],
  [/It is important that you /gi, ""],
  [/I need you to /gi, ""],
  [/You should make sure to /gi, ""],
  [/You are required to /gi, ""],
  [/In order to /gi, "To "],
  [/Due to the fact that /gi, "Because "],
  [/At this point in time /gi, "Now "],
  [/In the event that /gi, "If "],
  [/For the purpose of /gi, "For "],
  [/With regard to /gi, "Regarding "],
  [/In addition to that /gi, "Also "],
  [/As a matter of fact /gi, ""],
  [/It should be noted that /gi, ""],
  [/Please note that /gi, "Note: "],
  [/Keep in mind that /gi, ""],
  [/It goes without saying that /gi, ""],
  [/Needless to say /gi, ""],
  [/As previously mentioned /gi, ""],
  [/As I mentioned before /gi, ""],
  [/To summarize /gi, ""],
  [/In summary /gi, ""],
  [/Basically /gi, ""],
  [/Essentially /gi, ""],
  [/Actually /gi, ""],
  [/Obviously /gi, ""],
  [/Clearly /gi, ""],
];

// Filler words to remove from system prompts
const FILLER_PATTERNS = [
  /\b(very|really|quite|rather|somewhat|fairly|pretty much|just|simply)\b/gi,
];

/**
 * Remove redundant whitespace from text
 */
function cleanWhitespace(text) {
  return text
    .replace(/[ \t]+/g, " ")        // Multiple spaces/tabs to single space
    .replace(/\n{3,}/g, "\n\n")     // 3+ newlines to 2
    .replace(/^\s+$/gm, "")         // Empty lines with whitespace
    .trim();
}

/**
 * Apply phrase replacements to shorten verbose instructions
 */
function shortenPhrases(text) {
  let result = text;
  for (const [pattern, replacement] of PHRASE_REPLACEMENTS) {
    result = result.replace(pattern, replacement);
  }
  return result;
}

/**
 * Remove filler words from system prompts
 */
function removeFiller(text) {
  let result = text;
  for (const pattern of FILLER_PATTERNS) {
    result = result.replace(pattern, "");
  }
  // Clean up double spaces left by removals
  result = result.replace(/  +/g, " ");
  return result;
}

/**
 * Detect and remove duplicate/near-duplicate messages
 */
function deduplicateMessages(messages) {
  if (messages.length <= 2) return messages;

  const seen = new Map();
  const result = [];

  for (const msg of messages) {
    const content = typeof msg.content === "string" ? msg.content : JSON.stringify(msg.content);
    // Normalize for comparison
    const normalized = content.toLowerCase().replace(/\s+/g, " ").trim();

    // Check for exact or near duplicates
    let isDuplicate = false;
    for (const [key, _] of seen) {
      if (normalized === key) {
        isDuplicate = true;
        break;
      }
      // Check similarity (simple: same first 100 chars and same role)
      if (normalized.length > 50 && key.length > 50) {
        const prefix = normalized.slice(0, 100);
        if (key.startsWith(prefix)) {
          isDuplicate = true;
          break;
        }
      }
    }

    if (!isDuplicate || msg.role === "system") {
      seen.set(normalized, true);
      result.push(msg);
    }
  }

  return result;
}

/**
 * Optimize a single message content
 */
function optimizeContent(content, role) {
  if (typeof content !== "string") return content;

  let optimized = content;

  // Clean whitespace
  optimized = cleanWhitespace(optimized);

  // Shorten verbose phrases
  optimized = shortenPhrases(optimized);

  // Remove filler from system prompts (more aggressive)
  if (role === "system") {
    optimized = removeFiller(optimized);
  }

  // Final whitespace cleanup
  optimized = cleanWhitespace(optimized);

  return optimized;
}

/**
 * Calculate approximate token count (rough: 1 token ≈ 4 chars)
 */
function estimateTokens(messages) {
  let chars = 0;
  for (const msg of messages) {
    const content = typeof msg.content === "string" ? msg.content : JSON.stringify(msg.content || "");
    chars += content.length + (msg.role?.length || 0) + 4; // role + formatting overhead
  }
  return Math.ceil(chars / 4);
}

/**
 * Optimize messages array - main entry point
 * @param {Array} messages - OpenAI-format messages
 * @returns {{ messages: Array, savings: number, originalTokens: number, optimizedTokens: number }}
 */
export function optimizePrompt(messages) {
  if (!Array.isArray(messages) || messages.length === 0) {
    return { messages, savings: 0, originalTokens: 0, optimizedTokens: 0 };
  }

  const originalTokens = estimateTokens(messages);

  // Step 1: Deduplicate messages
  let optimized = deduplicateMessages(messages);

  // Step 2: Optimize each message content
  optimized = optimized.map(msg => ({
    ...msg,
    content: optimizeContent(msg.content, msg.role),
  }));

  // Step 3: Remove empty messages (except system)
  optimized = optimized.filter(msg => {
    if (msg.role === "system") return true;
    const content = typeof msg.content === "string" ? msg.content : "";
    return content.trim().length > 0;
  });

  const optimizedTokens = estimateTokens(optimized);
  const savings = originalTokens > 0 ? Math.round(((originalTokens - optimizedTokens) / originalTokens) * 100) : 0;

  return {
    messages: optimized,
    savings: Math.max(0, savings),
    originalTokens,
    optimizedTokens,
  };
}

// Context Compression v2 — intelligent prompt compression to reduce noise
import { getDb, getSetting } from "./db/index.js";

// Main compression function
export function compressContext(messages, options = {}) {
  const enabled = getSetting("context_compression_v2_enabled", "true") === "true";
  if (!enabled) return { messages, compressed: false, savedTokens: 0 };

  const maxInputTokens = parseInt(getSetting("context_compression_max_input", "6000"));
  const estimatedTokens = estimateTokens(messages);

  // Only compress if over threshold
  if (estimatedTokens <= maxInputTokens) {
    return { messages, compressed: false, savedTokens: 0 };
  }

  const compressed = [...messages];
  let saved = 0;

  // Strategy 1: Deduplicate repeated context blocks
  saved += deduplicateContext(compressed);

  // Strategy 2: Trim verbose system messages (keep first 500 words + last 200 words)
  saved += trimSystemMessages(compressed);

  // Strategy 3: Summarize long conversation history (keep last 2 user messages intact)
  saved += summarizeHistory(compressed);

  // Strategy 4: Remove redundant whitespace and formatting
  saved += cleanFormatting(compressed);

  const afterTokens = estimateTokens(compressed);

  return {
    messages: compressed,
    compressed: true,
    savedTokens: estimatedTokens - afterTokens,
    originalTokens: estimatedTokens,
    compressedTokens: afterTokens,
    ratio: afterTokens > 0 ? (1 - afterTokens / estimatedTokens).toFixed(2) : 0,
  };
}

// Strategy 1: Find and deduplicate repeated text blocks
function deduplicateContext(messages) {
  let saved = 0;

  // Find repeated blocks across messages (common in multi-turn with context)
  const contentMap = new Map();

  for (let i = 0; i < messages.length; i++) {
    const content = getContent(messages[i]);
    if (!content || content.length < 200) continue;

    // Split into paragraphs and find duplicates
    const paragraphs = content.split(/\n{2,}/);
    const seen = new Set();
    const unique = [];

    for (const para of paragraphs) {
      const normalized = para.trim().toLowerCase().slice(0, 100);
      if (normalized.length < 50) {
        unique.push(para);
        continue;
      }

      // Check if this paragraph appeared in earlier messages
      const key = normalized;
      if (contentMap.has(key) && contentMap.get(key) !== i) {
        saved += estimateTokensStr(para);
        unique.push("[...context repeated from above...]");
      } else {
        contentMap.set(key, i);
        unique.push(para);
      }
    }

    if (unique.length !== paragraphs.length) {
      setContent(messages[i], unique.join("\n\n"));
    }
  }

  return saved;
}

// Strategy 2: Trim verbose system messages
function trimSystemMessages(messages) {
  let saved = 0;
  const maxSystemWords = parseInt(getSetting("context_compression_max_system", "800"));

  for (let i = 0; i < messages.length; i++) {
    if (messages[i].role !== "system") continue;

    const content = getContent(messages[i]);
    if (!content) continue;

    const words = content.split(/\s+/);
    if (words.length <= maxSystemWords) continue;

    // Keep first 60% and last 25% (middle is usually examples/verbose docs)
    const keepStart = Math.floor(maxSystemWords * 0.6);
    const keepEnd = Math.floor(maxSystemWords * 0.25);

    const trimmed = [
      ...words.slice(0, keepStart),
      "\n[...compressed...]\n",
      ...words.slice(-keepEnd),
    ].join(" ");

    saved += estimateTokensStr(content) - estimateTokensStr(trimmed);
    setContent(messages[i], trimmed);
  }

  return saved;
}

// Strategy 3: Summarize old conversation history
function summarizeHistory(messages) {
  let saved = 0;

  // Keep last 2 user messages and last assistant message intact
  // Compress older messages to brief summaries
  const userMsgIndices = messages
    .map((m, i) => (m.role === "user" ? i : -1))
    .filter((i) => i >= 0);

  if (userMsgIndices.length <= 2) return 0; // Not enough history to compress

  const keepFrom = userMsgIndices[userMsgIndices.length - 2]; // Keep last 2 user turns

  for (let i = 0; i < keepFrom; i++) {
    if (messages[i].role === "system") continue; // Don't touch system

    const content = getContent(messages[i]);
    if (!content || content.length < 300) continue;

    // Compress to first 2 sentences + key info
    const sentences = content.match(/[^.!?\n]+[.!?\n]+/g) || [content];
    const summary = sentences.slice(0, 2).join("").trim();

    if (summary.length < content.length * 0.5) {
      saved += estimateTokensStr(content) - estimateTokensStr(summary);
      setContent(messages[i], summary + " [...]");
    }
  }

  return saved;
}

// Strategy 4: Clean excessive formatting
function cleanFormatting(messages) {
  let saved = 0;

  for (let i = 0; i < messages.length; i++) {
    const content = getContent(messages[i]);
    if (!content) continue;

    let cleaned = content;

    // Remove excessive blank lines (3+ → 2)
    cleaned = cleaned.replace(/\n{4,}/g, "\n\n\n");

    // Remove trailing spaces
    cleaned = cleaned.replace(/[ \t]+$/gm, "");

    // Collapse multiple spaces
    cleaned = cleaned.replace(/  +/g, " ");

    if (cleaned.length < content.length) {
      saved += estimateTokensStr(content) - estimateTokensStr(cleaned);
      setContent(messages[i], cleaned);
    }
  }

  return saved;
}

// Helpers
function getContent(msg) {
  if (typeof msg.content === "string") return msg.content;
  if (Array.isArray(msg.content)) {
    return msg.content.filter((p) => p.type === "text").map((p) => p.text).join("\n");
  }
  return "";
}

function setContent(msg, text) {
  if (typeof msg.content === "string") {
    msg.content = text;
  } else if (Array.isArray(msg.content)) {
    const textParts = msg.content.filter((p) => p.type === "text");
    if (textParts.length > 0) {
      textParts[0].text = text;
      // Remove other text parts
      msg.content = [textParts[0], ...msg.content.filter((p) => p.type !== "text")];
    }
  }
}

function estimateTokens(messages) {
  return messages.reduce((sum, m) => sum + estimateTokensStr(getContent(m)), 0);
}

function estimateTokensStr(text) {
  if (!text) return 0;
  // Rough estimate: 1 token ≈ 4 chars for English, 2 chars for CJK
  return Math.ceil(text.length / 3.5);
}

// Get compression stats (for dashboard)
export function getCompressionStats() {
  return {
    enabled: getSetting("context_compression_v2_enabled", "true") === "true",
    maxInput: parseInt(getSetting("context_compression_max_input", "6000")),
    maxSystem: parseInt(getSetting("context_compression_max_system", "800")),
  };
}

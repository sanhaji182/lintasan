// Chat Summary Mode - condense conversation history for better caching
import { getSetting } from "./db/index.js";

/**
 * Get current chat mode setting
 * @returns {"smart"|"summary"|"full"}
 */
export function getChatMode() {
  return getSetting("chat_mode", "full");
}

/**
 * Check if chat summary is active (not "full" mode)
 */
export function isChatSummaryEnabled() {
  const mode = getChatMode();
  return mode === "smart" || mode === "summary";
}

/**
 * Summarize conversation history based on mode
 * - "smart": single-turn only (system + last user message) — max cache hits
 * - "summary": condense history into a summary system message + last user message
 * - "full": pass through unchanged
 * @param {Array} messages - OpenAI-format messages array
 * @returns {Array} condensed messages array
 */
export function summarizeHistory(messages) {
  const mode = getChatMode();

  if (mode === "full" || !Array.isArray(messages) || messages.length <= 2) {
    return messages;
  }

  const systemMsg = messages.find(m => m.role === "system");
  const lastUserMsg = [...messages].reverse().find(m => m.role === "user");

  if (!lastUserMsg) return messages;

  if (mode === "smart") {
    // Single-turn: only system + last user message (maximum cache potential)
    const result = [];
    if (systemMsg) result.push(systemMsg);
    result.push(lastUserMsg);
    return result;
  }

  if (mode === "summary") {
    // Summarize: condense all prior messages into a context summary
    const priorMessages = messages.filter(m => m !== lastUserMsg && m !== systemMsg);

    if (priorMessages.length === 0) {
      const result = [];
      if (systemMsg) result.push(systemMsg);
      result.push(lastUserMsg);
      return result;
    }

    const summary = condensePriorMessages(priorMessages);
    const result = [];

    if (systemMsg) {
      // Append summary to system message
      result.push({
        role: "system",
        content: systemMsg.content + "\n\n[Conversation Summary]\n" + summary,
      });
    } else {
      result.push({
        role: "system",
        content: "[Conversation Summary]\n" + summary,
      });
    }

    result.push(lastUserMsg);
    return result;
  }

  return messages;
}

/**
 * Condense prior messages into a brief summary string
 */
function condensePriorMessages(messages) {
  const parts = [];
  let turnCount = 0;

  for (const msg of messages) {
    const content = typeof msg.content === "string" ? msg.content : "";
    if (!content.trim()) continue;

    turnCount++;
    const role = msg.role === "assistant" ? "AI" : msg.role === "user" ? "User" : msg.role;
    // Truncate long messages to key points
    const truncated = truncateContent(content, 150);
    parts.push(`${role}: ${truncated}`);
  }

  if (parts.length === 0) return "No prior context.";

  const header = `Prior conversation (${turnCount} turns):`;
  return header + "\n" + parts.join("\n");
}

/**
 * Truncate content to maxLen chars, preserving sentence boundaries
 */
function truncateContent(text, maxLen) {
  if (text.length <= maxLen) return text;

  // Try to cut at sentence boundary
  const cut = text.slice(0, maxLen);
  const lastPeriod = cut.lastIndexOf(".");
  const lastNewline = cut.lastIndexOf("\n");
  const breakPoint = Math.max(lastPeriod, lastNewline);

  if (breakPoint > maxLen * 0.5) {
    return text.slice(0, breakPoint + 1).trim() + "...";
  }

  return cut.trim() + "...";
}

// Context Compressor — reduces token usage for long conversations
// Summarizes older messages while keeping recent ones intact
import { getDb, getSetting } from "./db/index.js";

// Settings with defaults
function getCompressionConfig() {
  return {
    enabled: getSetting("compression_enabled", "true") === "true",
    // Max messages to keep in full (most recent)
    keepRecent: parseInt(getSetting("compression_keep_recent", "4")),
    // Threshold: only compress if messages exceed this count
    triggerThreshold: parseInt(getSetting("compression_trigger", "6")),
    // Max chars for summary of compressed messages
    summaryMaxChars: parseInt(getSetting("compression_summary_chars", "800")),
  };
}

// Extract text content from a message
function getMessageText(msg) {
  if (typeof msg.content === "string") return msg.content;
  if (Array.isArray(msg.content)) {
    return msg.content
      .filter((p) => p.type === "text")
      .map((p) => p.text || "")
      .join("\n");
  }
  return "";
}

// Compress older messages into a summary
export function compressContext(messages) {
  const config = getCompressionConfig();

  if (!config.enabled) return messages;
  if (!Array.isArray(messages)) return messages;

  // Separate system messages from conversation
  const systemMsgs = messages.filter(
    (m) => m.role === "system" || m.role === "developer"
  );
  const convMsgs = messages.filter(
    (m) => m.role !== "system" && m.role !== "developer"
  );

  // Don't compress if below threshold
  if (convMsgs.length <= config.triggerThreshold) return messages;

  // Split: older messages to compress, recent to keep
  const keepCount = config.keepRecent;
  const olderMsgs = convMsgs.slice(0, -keepCount);
  const recentMsgs = convMsgs.slice(-keepCount);

  // Build summary of older messages
  const summaryParts = [];
  for (const msg of olderMsgs) {
    const text = getMessageText(msg);
    if (!text) continue;

    const role = msg.role === "assistant" ? "A" : msg.role === "user" ? "U" : msg.role;
    // Truncate individual messages in summary
    const truncated =
      text.length > 200 ? text.slice(0, 200) + "..." : text;
    summaryParts.push(`[${role}]: ${truncated}`);
  }

  let summary = summaryParts.join("\n");
  // Cap total summary length
  if (summary.length > config.summaryMaxChars) {
    summary = summary.slice(0, config.summaryMaxChars) + "\n[...truncated]";
  }

  // Create compressed message set
  const compressedConv = [
    {
      role: "user",
      content: `[Previous conversation summary]\n${summary}\n[End of summary — recent messages follow]`,
    },
    {
      role: "assistant",
      content: "Understood. I have the context from our previous conversation. Continuing from where we left off.",
    },
    ...recentMsgs,
  ];

  return [...systemMsgs, ...compressedConv];
}

// Calculate token savings estimate (rough: 1 token ≈ 4 chars)
export function estimateSavings(original, compressed) {
  const origChars = original.reduce(
    (sum, m) => sum + getMessageText(m).length,
    0
  );
  const compChars = compressed.reduce(
    (sum, m) => sum + getMessageText(m).length,
    0
  );
  return {
    originalEstTokens: Math.ceil(origChars / 4),
    compressedEstTokens: Math.ceil(compChars / 4),
    savedEstTokens: Math.ceil((origChars - compChars) / 4),
    reductionPercent: origChars > 0 ? Math.round(((origChars - compChars) / origChars) * 100) : 0,
  };
}

// Prompt Reordering — optimize message order for provider-side prompt caching
// Providers like CC/Anthropic cache prompt prefixes server-side.
// By ensuring system prompt + static context is always first and stable,
// we maximize server-side cache hits (free optimization).

// Reorder messages to maximize provider cache hit rate:
// 1. System/developer messages first (most stable, cached by provider)
// 2. Static context (tool definitions, etc.)
// 3. Conversation messages in order (dynamic, changes each turn)
export function reorderForCaching(messages) {
  if (!Array.isArray(messages) || messages.length <= 1) return messages;

  const system = [];
  const developer = [];
  const conversation = [];

  for (const msg of messages) {
    if (msg.role === "system") {
      system.push(msg);
    } else if (msg.role === "developer") {
      developer.push(msg);
    } else {
      conversation.push(msg);
    }
  }

  // System first, then developer, then conversation
  // This ensures the prefix (system+developer) is stable across requests
  return [...system, ...developer, ...conversation];
}

// Merge multiple system messages into one (reduces token overhead from repeated role tags)
export function consolidateSystemMessages(messages) {
  if (!Array.isArray(messages)) return messages;

  const systemMsgs = messages.filter((m) => m.role === "system");
  if (systemMsgs.length <= 1) return messages;

  // Merge all system messages into one
  const mergedSystem = {
    role: "system",
    content: systemMsgs
      .map((m) => (typeof m.content === "string" ? m.content : ""))
      .filter(Boolean)
      .join("\n\n"),
  };

  const nonSystem = messages.filter((m) => m.role !== "system");
  return [mergedSystem, ...nonSystem];
}

// Full optimization pipeline
export function optimizePromptOrder(messages) {
  let optimized = consolidateSystemMessages(messages);
  optimized = reorderForCaching(optimized);
  return optimized;
}

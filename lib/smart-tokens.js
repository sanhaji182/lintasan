// Smart Max Tokens — dynamically adjust max_tokens based on prompt complexity
import { getSetting } from "./db/index.js";

function getSmartTokenConfig() {
  return {
    enabled: getSetting("smart_tokens_enabled", "true") === "true",
    // Minimum max_tokens (never go below this)
    floor: parseInt(getSetting("smart_tokens_floor", "150")),
    // Minimum for reasoning models (reasoning_content eats into budget)
    reasoningFloor: parseInt(getSetting("smart_tokens_reasoning_floor", "8192")),
    // Maximum max_tokens cap — let model decide, stop at provider limit
    ceiling: parseInt(getSetting("smart_tokens_ceiling", "65536")),
  };
}

// Classify prompt complexity
function classifyComplexity(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return "medium";

  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  if (!lastUser) return "medium";

  const text = typeof lastUser.content === "string"
    ? lastUser.content
    : Array.isArray(lastUser.content)
      ? lastUser.content.filter((p) => p.type === "text").map((p) => p.text).join(" ")
      : "";

  const lower = text.toLowerCase();
  const wordCount = text.split(/\s+/).length;

  // Simple/short questions
  const simplePatterns = [
    /^(hi|hello|hey|thanks|ok|yes|no|sure)\b/i,
    /^what is \w+\??$/i,
    /^(translate|convert|fix)\b/i,
  ];
  if (wordCount <= 5 || simplePatterns.some((p) => p.test(lower))) {
    return "simple";
  }

  // Complex indicators
  const complexIndicators = [
    /explain.*detail/i,
    /compare.*and/i,
    /step.by.step/i,
    /write.*code/i,
    /implement/i,
    /create.*function/i,
    /build.*app/i,
    /design.*system/i,
    /architecture/i,
    /comprehensive/i,
    /thorough/i,
    /full.*example/i,
    /production.ready/i,
    /best.practice/i,
  ];

  const complexScore = complexIndicators.filter((p) => p.test(lower)).length;

  // Also consider: long prompts with multiple questions tend to need long answers
  const questionMarks = (text.match(/\?/g) || []).length;
  const hasMultipleQuestions = questionMarks >= 2;
  const hasCodeRequest = /```|code|function|class|script|program/i.test(lower);

  if (complexScore >= 2 || (hasMultipleQuestions && wordCount > 30) || hasCodeRequest) {
    return "complex";
  }

  if (complexScore >= 1 || wordCount > 20) {
    return "medium";
  }

  return "simple";
}

// Determine optimal max_tokens
export function smartMaxTokens(messages, requestedMax) {
  const config = getSmartTokenConfig();

  if (!config.enabled) return requestedMax || config.ceiling;

  // If user explicitly set max_tokens, respect it but cap
  if (requestedMax && requestedMax > 0) {
    return Math.min(requestedMax, config.ceiling);
  }

  // Detect explicit word limits in prompt and cap tokens accordingly
  const lastUser = [...(messages || [])].reverse().find((m) => m.role === "user");
  if (lastUser) {
    const text = typeof lastUser.content === "string" ? lastUser.content : "";
    const wordLimitMatch = text.match(/(?:under|max(?:imum)?|kurang dari|maksimal?)\s*(\d+)\s*(?:words?|kata)/i);
    if (wordLimitMatch) {
      const wordLimit = parseInt(wordLimitMatch[1]);
      // ~1.5 tokens per word, add buffer for formatting
      const tokenCap = Math.max(config.floor, Math.ceil(wordLimit * 2));
      return Math.min(tokenCap, config.ceiling);
    }
  }

  // If no max_tokens provided, DON'T inject a limit — let model decide
  // This matches pass-through behavior (like 9Router) for best quality
  // Only inject if setting forces it
  const forceInject = getSetting("smart_tokens_force_inject", "false") === "true";
  if (!forceInject) {
    return undefined; // No max_tokens = model decides
  }

  const complexity = classifyComplexity(messages);

  let suggested;
  switch (complexity) {
    case "simple":
      suggested = 300;
      break;
    case "medium":
      suggested = 2000;
      break;
    case "complex":
      suggested = 16384; // Reasoning models need room after 7k+ system prompt injection
      break;
    default:
      suggested = 1000;
  }

  return Math.max(config.floor, Math.min(suggested, config.ceiling));
}

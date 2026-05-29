// Prompt Routing - auto-detect complexity and route to appropriate model
import { getSetting, getDb } from "./db/index.js";

// Complexity scoring heuristics
function estimateComplexity(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return "low";

  const lastMessage = messages[messages.length - 1];
  const content = typeof lastMessage.content === "string" ? lastMessage.content : "";
  const totalContent = messages.map(m => typeof m.content === "string" ? m.content : "").join(" ");

  let score = 0;

  // Length-based
  if (content.length > 2000) score += 3;
  else if (content.length > 500) score += 2;
  else if (content.length > 100) score += 1;

  // Conversation depth
  if (messages.length > 10) score += 2;
  else if (messages.length > 5) score += 1;

  // Code indicators
  const codePatterns = /```|function |class |import |def |const |let |var |async |await |return /gi;
  const codeMatches = (totalContent.match(codePatterns) || []).length;
  if (codeMatches > 10) score += 3;
  else if (codeMatches > 3) score += 2;

  // Reasoning indicators
  const reasoningPatterns = /explain|analyze|compare|evaluate|design|architect|debug|optimize|refactor|implement/gi;
  const reasoningMatches = (content.match(reasoningPatterns) || []).length;
  if (reasoningMatches > 3) score += 3;
  else if (reasoningMatches > 1) score += 2;

  // Math/logic indicators
  const mathPatterns = /calculate|equation|formula|proof|theorem|algorithm|complexity/gi;
  if ((content.match(mathPatterns) || []).length > 0) score += 2;

  // System prompt complexity
  const systemMsg = messages.find(m => m.role === "system");
  if (systemMsg && typeof systemMsg.content === "string" && systemMsg.content.length > 1000) score += 2;

  // Classify
  if (score >= 7) return "high";
  if (score >= 4) return "medium";
  return "low";
}

// Get routing rules
export function getPromptRoutingRules() {
  try {
    const rulesJson = getSetting("prompt_routing_rules", "{}");
    return JSON.parse(rulesJson);
  } catch {
    return {};
  }
}

export function setPromptRoutingRules(rules) {
  const db = getDb();
  db.prepare("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)").run("prompt_routing_rules", JSON.stringify(rules));
}

export function isPromptRoutingEnabled() {
  return getSetting("prompt_routing_enabled", "false") === "true";
}

// Route based on complexity
export function routeByComplexity(messages) {
  if (!isPromptRoutingEnabled()) return null;

  const complexity = estimateComplexity(messages);
  const rules = getPromptRoutingRules();

  // Rules format: { high: { model, provider }, medium: { model, provider }, low: { model, provider } }
  const rule = rules[complexity];
  if (rule && rule.model && rule.provider) {
    return { model: rule.model, provider: rule.provider, complexity };
  }

  return null;
}

export { estimateComplexity };

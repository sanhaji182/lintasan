// Quality-Aware Model Routing — route queries to appropriate model tier
// Simple queries → cheap model, complex queries → expensive model
// Based on RouteLLM research: 40-60% cost reduction without quality loss
import { getSetting } from "./db/index.js";

function getQualityRoutingConfig() {
  return {
    enabled: getSetting("quality_routing_enabled", "true") === "true",
    // Model tiers
    cheapModel: getSetting("quality_routing_cheap_model", "minimax/minimax-m2.7"),
    expensiveModel: getSetting("quality_routing_expensive_model", "deepseek/deepseek-v4-pro"),
    // Provider for cheap model
    cheapProvider: getSetting("quality_routing_cheap_provider", "commandcode"),
    expensiveProvider: getSetting("quality_routing_expensive_provider", "commandcode"),
    // Threshold score (0-1): below = cheap, above = expensive
    complexityThreshold: parseFloat(getSetting("quality_routing_threshold", "0.5")),
    // Force expensive for these patterns (override)
    forceExpensivePatterns: getSetting("quality_routing_force_expensive", "code,implement,debug,architect,design system,production").split(","),
  };
}

// Complexity scoring (0-1)
function scoreComplexity(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return 0.5;

  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  if (!lastUser) return 0.5;

  const text = typeof lastUser.content === "string"
    ? lastUser.content
    : Array.isArray(lastUser.content)
      ? lastUser.content.filter((p) => p.type === "text").map((p) => p.text).join(" ")
      : "";

  const lower = text.toLowerCase();
  const wordCount = text.split(/\s+/).length;
  let score = 0;

  // Length factor (longer = more complex)
  if (wordCount <= 5) score += 0;
  else if (wordCount <= 15) score += 0.1;
  else if (wordCount <= 40) score += 0.2;
  else if (wordCount <= 80) score += 0.3;
  else score += 0.4;

  // Complexity indicators
  const complexPatterns = [
    { pattern: /\b(implement|build|create|develop|write)\b.*\b(app|application|system|service|api)\b/i, weight: 0.3 },
    { pattern: /\b(debug|fix|troubleshoot|diagnose)\b/i, weight: 0.25 },
    { pattern: /\b(architect|design|plan|strategy)\b/i, weight: 0.3 },
    { pattern: /\b(compare|analyze|evaluate|trade.?off)\b/i, weight: 0.2 },
    { pattern: /\b(step.by.step|detailed|comprehensive|thorough)\b/i, weight: 0.15 },
    { pattern: /\b(production|scalab|performance|optimi)\b/i, weight: 0.2 },
    { pattern: /\b(security|vulnerab|exploit|attack)\b/i, weight: 0.2 },
    { pattern: /```|code block/i, weight: 0.25 },
    { pattern: /\b(algorithm|data structure|complexity)\b/i, weight: 0.25 },
    { pattern: /\b(kubernetes|docker|terraform|aws|gcp|azure)\b/i, weight: 0.15 },
    { pattern: /\b(machine learning|neural|model|training)\b/i, weight: 0.2 },
  ];

  for (const { pattern, weight } of complexPatterns) {
    if (pattern.test(lower)) score += weight;
  }

  // Simple indicators (reduce score)
  const simplePatterns = [
    { pattern: /^(hi|hello|hey|thanks|ok|yes|no|sure|bye)\b/i, weight: -0.4 },
    { pattern: /^(what is|who is|when|where)\b.{0,30}\??$/i, weight: -0.2 },
    { pattern: /^(translate|convert|summarize in one)\b/i, weight: -0.15 },
    { pattern: /\b(joke|fun fact|trivia)\b/i, weight: -0.2 },
  ];

  for (const { pattern, weight } of simplePatterns) {
    if (pattern.test(lower)) score += weight;
  }

  // Multi-turn context adds complexity
  const turnCount = messages.filter((m) => m.role === "user").length;
  if (turnCount > 3) score += 0.1;
  if (turnCount > 6) score += 0.1;

  // Multiple questions in one message
  const questionMarks = (text.match(/\?/g) || []).length;
  if (questionMarks >= 3) score += 0.15;

  // Clamp to 0-1
  return Math.max(0, Math.min(1, score));
}

// Determine which model tier to use
export function routeByQuality(messages, requestedModel) {
  const config = getQualityRoutingConfig();

  if (!config.enabled) return null; // null = don't override

  // Only route if using the default/expensive model
  // Don't override if user explicitly chose a specific model
  if (requestedModel && requestedModel !== config.expensiveModel) return null;

  const score = scoreComplexity(messages);

  // Force expensive for certain patterns
  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  if (lastUser) {
    const text = typeof lastUser.content === "string" ? lastUser.content : "";
    const lower = text.toLowerCase();
    for (const pattern of config.forceExpensivePatterns) {
      if (pattern.trim() && lower.includes(pattern.trim())) {
        return { model: config.expensiveModel, provider: config.expensiveProvider, score, tier: "expensive", reason: "force_pattern" };
      }
    }
  }

  if (score < config.complexityThreshold) {
    return { model: config.cheapModel, provider: config.cheapProvider, score, tier: "cheap", reason: "low_complexity" };
  }

  return { model: config.expensiveModel, provider: config.expensiveProvider, score, tier: "expensive", reason: "high_complexity" };
}

// Get complexity score for debugging/monitoring
export function getComplexityScore(messages) {
  return scoreComplexity(messages);
}

// Response Quality Filter — scores responses and triggers retry if quality is too low
import { getDb, getSetting } from "./db/index.js";

// Quality dimensions and their weights
const QUALITY_WEIGHTS = {
  completeness: 0.30,   // Did the model finish? Not truncated?
  relevance: 0.25,      // Does response address the question?
  format: 0.20,        // Does it follow requested format?
  coherence: 0.10,      // Is it well-structured and readable?
  verbosity: 0.15,     // Is response appropriately concise?
};

// Score a response on multiple dimensions
export function scoreResponse(response, originalMessages, finishReason) {
  const enabled = getSetting("quality_filter_enabled", "true") === "true";
  if (!enabled) return { score: 1.0, pass: true, dimensions: {} };

  const content = extractContent(response);
  if (!content) return { score: 0, pass: false, reason: "empty_response", dimensions: {} };

  const lastUser = getLastUserMessage(originalMessages);
  const dimensions = {};

  // 1. Completeness
  dimensions.completeness = scoreCompleteness(content, finishReason);

  // 2. Relevance
  dimensions.relevance = scoreRelevance(content, lastUser);

  // 3. Format compliance
  dimensions.format = scoreFormat(content, lastUser);

  // 4. Coherence
  dimensions.coherence = scoreCoherence(content);

  // 5. Verbosity — penalize unnecessarily long responses
  dimensions.verbosity = scoreVerbosity(content, lastUser);

  // Weighted total
  const totalScore = Object.entries(QUALITY_WEIGHTS).reduce((sum, [dim, weight]) => {
    return sum + (dimensions[dim] || 0) * weight;
  }, 0);

  const threshold = parseFloat(getSetting("quality_filter_threshold", "0.4"));
  const pass = totalScore >= threshold;

  return {
    score: Math.round(totalScore * 100) / 100,
    pass,
    reason: pass ? null : `quality_below_threshold (${totalScore.toFixed(2)} < ${threshold})`,
    dimensions,
    threshold,
  };
}

// Completeness: penalize truncated responses
function scoreCompleteness(content, finishReason) {
  let score = 1.0;

  // Truncated by max_tokens
  if (finishReason === "length") score -= 0.5;

  // Ends mid-sentence (no period, no code block close)
  const trimmed = content.trim();
  const lastChar = trimmed[trimmed.length - 1];
  const endsClean = /[.!?}\])`"'\n]$/.test(trimmed) || trimmed.endsWith("```");
  if (!endsClean && finishReason === "length") score -= 0.2;

  // Very short response for complex question
  if (content.split(/\s+/).length < 10) score -= 0.3;

  return Math.max(0, Math.min(1, score));
}

// Relevance: does response address the question?
function scoreRelevance(content, userMessage) {
  if (!userMessage) return 0.7; // Can't assess without user message

  let score = 0.5; // Base score

  // Extract key terms from user message
  const userTerms = extractKeyTerms(userMessage);
  const responseTerms = extractKeyTerms(content);

  // Check overlap of key terms
  const overlap = userTerms.filter((t) => responseTerms.includes(t));
  const overlapRatio = userTerms.length > 0 ? overlap.length / userTerms.length : 0;

  score += overlapRatio * 0.5; // Up to +0.5 for term overlap

  return Math.max(0, Math.min(1, score));
}

// Format: does it follow requested format?
function scoreFormat(content, userMessage) {
  if (!userMessage) return 0.8;

  let score = 0.8; // Default: assume format is fine

  // Check if user asked for specific format
  const wantsCode = /\b(code|implement|write|function|class)\b/i.test(userMessage);
  const wantsList = /\b(list|enumerate|steps|numbered)\b/i.test(userMessage);
  const wantsBrief = /\b(brief|concise|short|under \d+ words|max \d+ words)\b/i.test(userMessage);
  const wantsLang = /\b(bahasa indonesia|indonesian|in indonesian)\b/i.test(userMessage);

  // Verify format compliance
  if (wantsCode && !content.includes("```") && !/\b(function|class|const|let|def|import)\b/.test(content)) {
    score -= 0.3; // Asked for code but no code block
  }

  if (wantsList && !/(\d+\.|[-*•])\s/.test(content)) {
    score -= 0.2; // Asked for list but no list markers
  }

  if (wantsBrief) {
    const wordCount = content.split(/\s+/).length;
    const limitMatch = userMessage.match(/(\d+)\s*words/i);
    const limit = limitMatch ? parseInt(limitMatch[1]) : 200;
    if (wordCount > limit * 1.5) score -= 0.3; // Way over word limit
  }

  if (wantsLang) {
    // Simple check: Indonesian common words
    const idWords = /\b(dan|yang|untuk|dengan|dari|ini|itu|adalah|tidak|akan|sudah|bisa)\b/i;
    if (!idWords.test(content)) score -= 0.2;
  }

  return Math.max(0, Math.min(1, score));
}

// Coherence: well-structured and readable
function scoreCoherence(content) {
  let score = 0.8;

  // Penalize very repetitive content
  const sentences = content.split(/[.!?\n]+/).filter((s) => s.trim().length > 10);
  if (sentences.length > 3) {
    const unique = new Set(sentences.map((s) => s.trim().toLowerCase()));
    const uniqueRatio = unique.size / sentences.length;
    if (uniqueRatio < 0.5) score -= 0.4; // Very repetitive
  }

  // Penalize gibberish (high ratio of non-word characters)
  const alphaRatio = (content.match(/[a-zA-Z\u00C0-\u024F\u0400-\u04FF\u4E00-\u9FFF]/g) || []).length / content.length;
  if (alphaRatio < 0.3 && !content.includes("```")) score -= 0.3;

  // Bonus for structured content (headers, lists, code blocks)
  if (/^#{1,3}\s/m.test(content) || /^\d+\.\s/m.test(content) || content.includes("```")) {
    score += 0.1;
  }

  return Math.max(0, Math.min(1, score));
}

// Verbosity: is response appropriately concise?
function scoreVerbosity(content, userMessage) {
  const wordCount = content.split(/\s+/).length;
  let score = 1.0;

  if (!userMessage) return 0.8;

  // Baseline: reasonable response is ~50-200 words for most tasks
  const userWordCount = userMessage.split(/\s+/).length;

  // Ratio of response length to prompt length
  const ratio = wordCount / Math.max(userWordCount, 1);

  // Penalize if response is more than 5x the prompt length (very verbose)
  if (ratio > 5) score -= 0.5;
  else if (ratio > 3) score -= 0.3;
  else if (ratio > 2) score -= 0.1;

  // Bonus if response is appropriately sized (1-2x prompt length)
  if (ratio >= 0.5 && ratio <= 2) score += 0.1;

  // Penalize very long responses (>1000 words) regardless of prompt
  if (wordCount > 1000) score -= 0.3;

  // Penalize if user asked for "brief" or "short"
  if (/\b(brief|short|concise|minimal|under \d+|max \d+|no explanation|just |only )\b/i.test(userMessage)) {
    if (ratio > 1.5) score -= 0.3;
    if (wordCount > 300) score -= 0.2;
  }

  // Penalize if response starts with obvious filler phrases
  const filler = /^(ya|yes|ok|oke|baik|sure|tentu|bisa|jadi|nah|so |well |actually |basically )/i;
  if (filler.test(content.trim())) score -= 0.1;

  return Math.max(0, Math.min(1, score));
}

// Helper: extract content from response object
function extractContent(response) {
  if (typeof response === "string") return response;
  if (response?.choices?.[0]?.message?.content) return response.choices[0].message.content;
  if (response?.content) return response.content;
  return "";
}

// Helper: get last user message text
function getLastUserMessage(messages) {
  if (!Array.isArray(messages)) return "";
  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  if (!lastUser) return "";
  return typeof lastUser.content === "string"
    ? lastUser.content
    : Array.isArray(lastUser.content)
      ? lastUser.content.filter((p) => p.type === "text").map((p) => p.text).join(" ")
      : "";
}

// Helper: extract key terms from text
function extractKeyTerms(text) {
  const stopWords = new Set([
    "the", "a", "an", "is", "are", "was", "were", "be", "been", "being",
    "have", "has", "had", "do", "does", "did", "will", "would", "could",
    "should", "may", "might", "shall", "can", "need", "dare", "ought",
    "used", "to", "of", "in", "for", "on", "with", "at", "by", "from",
    "as", "into", "through", "during", "before", "after", "above", "below",
    "between", "out", "off", "over", "under", "again", "further", "then",
    "once", "here", "there", "when", "where", "why", "how", "all", "each",
    "every", "both", "few", "more", "most", "other", "some", "such", "no",
    "nor", "not", "only", "own", "same", "so", "than", "too", "very",
    "just", "because", "but", "and", "or", "if", "while", "that", "this",
    "yang", "dan", "di", "ke", "dari", "untuk", "dengan", "ini", "itu",
  ]);

  return text
    .toLowerCase()
    .replace(/[^a-z0-9\s]/g, " ")
    .split(/\s+/)
    .filter((w) => w.length > 2 && !stopWords.has(w))
    .slice(0, 30);
}

// Get quality filter status (for dashboard)
export function getQualityFilterStatus() {
  return {
    enabled: getSetting("quality_filter_enabled", "true") === "true",
    threshold: parseFloat(getSetting("quality_filter_threshold", "0.4")),
    maxRetries: parseInt(getSetting("quality_filter_max_retries", "1")),
  };
}

// Web Search Integration - detect queries needing web search and inject context
import { getSetting } from "./db/index.js";

const SEARCH_KEYWORDS = [
  "latest", "current", "today", "yesterday", "recent",
  "2024", "2025", "2026", "news", "price", "stock",
  "weather", "update", "released", "announced", "launched",
  "trending", "now", "this week", "this month",
];

const URL_PATTERN = /https?:\/\/[^\s]+|www\.[^\s]+/i;

/**
 * Check if web search is enabled
 */
export function isWebSearchEnabled() {
  return getSetting("web_search_enabled", "false") === "true";
}

/**
 * Detect if a query likely needs web search results
 * @param {string} query - The user's message content
 * @returns {boolean}
 */
export function needsWebSearch(query) {
  if (!query || typeof query !== "string") return false;

  const lower = query.toLowerCase();

  // Check for URL references
  if (URL_PATTERN.test(query)) return true;

  // Check for search keywords
  for (const keyword of SEARCH_KEYWORDS) {
    if (lower.includes(keyword)) return true;
  }

  // Question patterns that often need current info
  const questionPatterns = /what is the .*(price|status|version|score)|who (won|is leading)|when (does|did|will|is)/i;
  if (questionPatterns.test(query)) return true;

  return false;
}

/**
 * Extract the search query from user message
 * @param {string} content - User message content
 * @returns {string} search query
 */
function extractSearchQuery(content) {
  // Remove common prefixes
  let query = content
    .replace(/^(please |can you |could you |help me |tell me |find |search |look up )/i, "")
    .replace(/[?!.]+$/, "")
    .trim();

  // Limit query length for search
  if (query.length > 150) {
    query = query.slice(0, 150);
  }

  return query;
}

/**
 * Perform DuckDuckGo HTML search and parse results
 * @param {string} query - Search query
 * @returns {Promise<Array<{title: string, snippet: string, url: string}>>}
 */
export async function searchDuckDuckGo(query) {
  try {
    const encoded = encodeURIComponent(query);
    const url = `https://html.duckduckgo.com/html/?q=${encoded}`;

    const response = await fetch(url, {
      headers: {
        "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
      },
      signal: AbortSignal.timeout(5000),
    });

    if (!response.ok) return [];

    const html = await response.text();
    return parseSearchResults(html);
  } catch (error) {
    console.error("[web-search] DuckDuckGo search failed:", error.message);
    return [];
  }
}

/**
 * Parse DuckDuckGo HTML results
 */
function parseSearchResults(html) {
  const results = [];

  // Match result blocks - DuckDuckGo HTML format
  const resultPattern = /<a[^>]*class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)<\/a>/gs;
  const snippetPattern = /<a[^>]*class="result__snippet"[^>]*>(.*?)<\/a>/gs;

  const titles = [];
  const urls = [];
  let match;

  while ((match = resultPattern.exec(html)) !== null) {
    urls.push(decodeURIComponent(match[1].replace(/\/\/duckduckgo\.com\/l\/\?uddg=/, "").split("&")[0]));
    titles.push(stripHtml(match[2]));
  }

  const snippets = [];
  while ((match = snippetPattern.exec(html)) !== null) {
    snippets.push(stripHtml(match[1]));
  }

  for (let i = 0; i < Math.min(titles.length, 5); i++) {
    if (titles[i] && (snippets[i] || urls[i])) {
      results.push({
        title: titles[i],
        snippet: snippets[i] || "",
        url: urls[i] || "",
      });
    }
  }

  return results;
}

/**
 * Strip HTML tags from string
 */
function stripHtml(html) {
  return html.replace(/<[^>]*>/g, "").replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, "\"").replace(/&#x27;/g, "'").trim();
}

/**
 * Format search results as context for LLM
 * @param {Array} results - Search results
 * @returns {string} formatted context
 */
export function formatSearchContext(results) {
  if (!results || results.length === 0) return "";

  let context = "[Web Search Results]\n";
  for (let i = 0; i < results.length; i++) {
    const r = results[i];
    context += `${i + 1}. ${r.title}\n`;
    if (r.snippet) context += `   ${r.snippet}\n`;
    if (r.url) context += `   Source: ${r.url}\n`;
  }
  context += "\nUse the above search results as context to answer the user's question. Cite sources when relevant.\n";
  return context;
}

/**
 * Inject web search results into messages array
 * @param {Array} messages - OpenAI-format messages
 * @returns {Promise<Array>} messages with search context prepended
 */
export async function injectWebSearch(messages) {
  if (!isWebSearchEnabled()) return messages;
  if (!Array.isArray(messages) || messages.length === 0) return messages;

  // Get last user message
  const lastUserMsg = [...messages].reverse().find(m => m.role === "user");
  if (!lastUserMsg) return messages;

  const content = typeof lastUserMsg.content === "string" ? lastUserMsg.content : "";
  if (!needsWebSearch(content)) return messages;

  const query = extractSearchQuery(content);
  const results = await searchDuckDuckGo(query);

  if (results.length === 0) return messages;

  const searchContext = formatSearchContext(results);

  // Inject as system message at the beginning (after existing system message)
  const newMessages = [...messages];
  const systemIdx = newMessages.findIndex(m => m.role === "system");

  const searchMsg = {
    role: "system",
    content: searchContext,
  };

  if (systemIdx >= 0) {
    // Insert after existing system message
    newMessages.splice(systemIdx + 1, 0, searchMsg);
  } else {
    // Insert at beginning
    newMessages.unshift(searchMsg);
  }

  return newMessages;
}

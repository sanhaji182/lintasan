// Semantic Cache — similarity-based response caching
// Uses simple TF-IDF cosine similarity (no external embedding API needed)
import { getDb, getSetting } from "./db/index.js";

// Initialize semantic cache table
function initSemanticCache() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS semantic_cache (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      model TEXT NOT NULL,
      fingerprint TEXT NOT NULL,
      messages_hash TEXT NOT NULL,
      response TEXT NOT NULL,
      token_count INTEGER DEFAULT 0,
      hits INTEGER DEFAULT 0,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      expires_at DATETIME NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_semantic_model ON semantic_cache(model);
    CREATE INDEX IF NOT EXISTS idx_semantic_expires ON semantic_cache(expires_at);
  `);
}

let initialized = false;
function ensureInit() {
  if (!initialized) {
    initSemanticCache();
    initialized = true;
  }
}

// Config
function getSemanticConfig() {
  return {
    enabled: getSetting("semantic_cache_enabled", "true") === "true",
    similarityThreshold: parseFloat(getSetting("semantic_cache_threshold", "0.92")),
    maxEntries: parseInt(getSetting("semantic_cache_max_entries", "500")),
    ttlSeconds: parseInt(getSetting("semantic_cache_ttl", "3600")),
  };
}

export function isSemanticCacheEnabled() {
  return getSemanticConfig().enabled;
}

// Simple tokenizer — split into lowercase words, remove stopwords, basic stemming
const STOPWORDS = new Set([
  "the", "a", "an", "is", "are", "was", "were", "be", "been", "being",
  "have", "has", "had", "do", "does", "did", "will", "would", "could",
  "should", "may", "might", "shall", "can", "need", "dare", "ought",
  "used", "to", "of", "in", "for", "on", "with", "at", "by", "from",
  "as", "into", "through", "during", "before", "after", "above", "below",
  "between", "out", "off", "over", "under", "again", "further", "then",
  "once", "here", "there", "when", "where", "why", "how", "all", "each",
  "every", "both", "few", "more", "most", "other", "some", "such", "no",
  "nor", "not", "only", "own", "same", "so", "than", "too", "very",
  "just", "because", "but", "and", "or", "if", "while", "about",
  "what", "which", "who", "whom", "this", "that", "these", "those",
  "am", "it", "its", "my", "your", "his", "her", "our", "their",
  "me", "him", "us", "them", "you", "she", "he", "we", "they",
  "tell", "can", "please", "explain", "describe", "want", "know",
]);

// Basic suffix stemmer (covers most English inflections)
function stem(word) {
  if (word.length <= 4) return word;
  if (word.endsWith("ation")) return word.slice(0, -5);
  if (word.endsWith("ment")) return word.slice(0, -4);
  if (word.endsWith("ness")) return word.slice(0, -4);
  if (word.endsWith("ing")) return word.slice(0, -3);
  if (word.endsWith("tion")) return word.slice(0, -4);
  if (word.endsWith("sion")) return word.slice(0, -4);
  if (word.endsWith("ies")) return word.slice(0, -3) + "y";
  if (word.endsWith("ous")) return word.slice(0, -3);
  if (word.endsWith("ive")) return word.slice(0, -3);
  if (word.endsWith("ful")) return word.slice(0, -3);
  if (word.endsWith("less")) return word.slice(0, -4);
  if (word.endsWith("able")) return word.slice(0, -4);
  if (word.endsWith("ible")) return word.slice(0, -4);
  if (word.endsWith("ally")) return word.slice(0, -4);
  if (word.endsWith("ly")) return word.slice(0, -2);
  if (word.endsWith("er")) return word.slice(0, -2);
  if (word.endsWith("ed")) return word.slice(0, -2);
  if (word.endsWith("es")) return word.slice(0, -2);
  if (word.endsWith("s") && !word.endsWith("ss")) return word.slice(0, -1);
  return word;
}

function tokenize(text) {
  return text
    .toLowerCase()
    .replace(/[^\w\s]/g, " ")
    .split(/\s+/)
    .map((w) => stem(w))
    .filter((w) => w.length > 2 && !STOPWORDS.has(w));
}

// Build TF vector from tokens
function buildTF(tokens) {
  const tf = {};
  for (const t of tokens) {
    tf[t] = (tf[t] || 0) + 1;
  }
  // Normalize
  const len = tokens.length || 1;
  for (const t in tf) tf[t] /= len;
  return tf;
}

// Cosine similarity between two TF vectors
function cosineSimilarity(a, b) {
  const keys = new Set([...Object.keys(a), ...Object.keys(b)]);
  let dot = 0, magA = 0, magB = 0;
  for (const k of keys) {
    const va = a[k] || 0;
    const vb = b[k] || 0;
    dot += va * vb;
    magA += va * va;
    magB += vb * vb;
  }
  const denom = Math.sqrt(magA) * Math.sqrt(magB);
  return denom === 0 ? 0 : dot / denom;
}

// Extract fingerprint from messages (last user message + context)
function extractFingerprint(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return "";
  
  // Focus on last user message (most important for semantic matching)
  const userMsgs = messages.filter((m) => m.role === "user");
  const lastUser = userMsgs[userMsgs.length - 1];
  if (!lastUser) return "";

  const content = typeof lastUser.content === "string"
    ? lastUser.content
    : Array.isArray(lastUser.content)
      ? lastUser.content.filter((p) => p.type === "text").map((p) => p.text).join(" ")
      : "";

  // Weight the TAIL of the message more heavily (task-specific instructions)
  // This prevents false cache hits when prompts share large context blocks
  const words = content.split(/\s+/);
  
  // Strategy: extract ONLY the task-specific part
  // Look for common delimiters that separate context from task
  const delimiters = [
    /\n(?:task|tugas|instruksi|instruction|question|pertanyaan|phase|fase)\s*[:：]/i,
    /\n(?:---+|===+|___+)\s*\n/,
    /\n(?:now|sekarang|berdasarkan|based on)\s/i,
  ];
  
  let taskPart = "";
  for (const delim of delimiters) {
    const match = content.search(delim);
    if (match > 0 && match < content.length * 0.9) {
      taskPart = content.slice(match).trim();
      break;
    }
  }
  
  // Fallback: use last 100 words (most likely the actual task)
  if (!taskPart || taskPart.split(/\s+/).length < 5) {
    const tailStart = Math.max(0, words.length - 100);
    taskPart = words.slice(tailStart).join(" ");
  }
  
  // Use ONLY the task part for fingerprinting — no context contamination
  return taskPart.trim();
}

// Search for semantically similar cached response
export function findSemanticMatch(model, messages) {
  ensureInit();
  const config = getSemanticConfig();
  if (!config.enabled) return null;

  const db = getDb();
  const fingerprint = extractFingerprint(messages);
  if (!fingerprint) return null;

  const queryTokens = tokenize(fingerprint);
  const queryTF = buildTF(queryTokens);

  // Get recent entries for this model
  const rows = db
    .prepare(
      `SELECT id, fingerprint, response, hits FROM semantic_cache 
       WHERE model = ? AND expires_at > datetime('now')
       ORDER BY hits DESC, created_at DESC LIMIT 100`
    )
    .all(model);

  let bestMatch = null;
  let bestScore = 0;

  for (const row of rows) {
    const cachedTokens = tokenize(row.fingerprint);
    const cachedTF = buildTF(cachedTokens);
    const score = cosineSimilarity(queryTF, cachedTF);

    if (score > bestScore && score >= config.similarityThreshold) {
      bestScore = score;
      bestMatch = row;
    }
  }

  if (bestMatch) {
    // Increment hit count
    db.prepare("UPDATE semantic_cache SET hits = hits + 1 WHERE id = ?").run(bestMatch.id);
    try {
      return { response: JSON.parse(bestMatch.response), score: bestScore };
    } catch {
      return null;
    }
  }

  return null;
}

// Store response in semantic cache
export function storeSemanticCache(model, messages, response) {
  ensureInit();
  const config = getSemanticConfig();
  if (!config.enabled) return;

  const db = getDb();
  const fingerprint = extractFingerprint(messages);
  if (!fingerprint || fingerprint.length < 10) return;

  // Simple hash for dedup
  const msgHash = simpleHash(JSON.stringify(messages.slice(-3)));

  // Check if exact hash already exists
  const existing = db
    .prepare("SELECT id FROM semantic_cache WHERE messages_hash = ? AND model = ?")
    .get(msgHash, model);
  if (existing) return;

  // Enforce max entries (evict oldest low-hit entries)
  const count = db.prepare("SELECT COUNT(*) as cnt FROM semantic_cache WHERE model = ?").get(model);
  if (count && count.cnt >= config.maxEntries) {
    db.prepare(
      `DELETE FROM semantic_cache WHERE id IN (
        SELECT id FROM semantic_cache WHERE model = ? ORDER BY hits ASC, created_at ASC LIMIT 50
      )`
    ).run(model);
  }

  // Clean expired
  db.prepare("DELETE FROM semantic_cache WHERE expires_at < datetime('now')").run();

  const tokenCount = (response.usage?.prompt_tokens || 0) + (response.usage?.completion_tokens || 0);
  const expiresAt = new Date(Date.now() + config.ttlSeconds * 1000).toISOString();

  db.prepare(
    `INSERT INTO semantic_cache (model, fingerprint, messages_hash, response, token_count, expires_at)
     VALUES (?, ?, ?, ?, ?, ?)`
  ).run(model, fingerprint, msgHash, JSON.stringify(response), tokenCount, expiresAt);
}

function simpleHash(str) {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash; // Convert to 32bit integer
  }
  return hash.toString(36);
}

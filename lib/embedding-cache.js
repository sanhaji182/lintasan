// Embedding Cache — real vector similarity using configurable embedding provider
// Falls back gracefully if embedding API is unavailable
import { getDb, getSetting } from "./db/index.js";

// Initialize embedding cache table
function initEmbeddingCache() {
  const db = getDb();
  db.exec(`
    CREATE TABLE IF NOT EXISTS embedding_cache (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      model TEXT NOT NULL,
      embedding TEXT NOT NULL,
      messages_hash TEXT NOT NULL,
      response TEXT NOT NULL,
      hits INTEGER DEFAULT 0,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      expires_at DATETIME NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_embedding_cache_model ON embedding_cache(model);
    CREATE INDEX IF NOT EXISTS idx_embedding_cache_expires ON embedding_cache(expires_at);
  `);
}

let initialized = false;
function ensureInit() {
  if (!initialized) {
    initEmbeddingCache();
    initialized = true;
  }
}

// Config
function getEmbeddingConfig() {
  return {
    enabled: getSetting("embedding_cache_enabled", "false") === "true",
    providerUrl: getSetting("embedding_provider_url", "https://api.openai.com/v1"),
    apiKey: getSetting("embedding_api_key", ""),
    model: getSetting("embedding_model", "text-embedding-3-small"),
    similarityThreshold: parseFloat(getSetting("embedding_similarity_threshold", "0.88")),
    ttlSeconds: parseInt(getSetting("semantic_cache_ttl", "3600")),
    maxEntries: parseInt(getSetting("semantic_cache_max_entries", "500")),
  };
}

export function isEmbeddingCacheEnabled() {
  const config = getEmbeddingConfig();
  return config.enabled && config.apiKey.length > 0;
}

// Cosine similarity between two float arrays
function cosineSimilarity(a, b) {
  if (a.length !== b.length) return 0;
  let dot = 0, magA = 0, magB = 0;
  for (let i = 0; i < a.length; i++) {
    dot += a[i] * b[i];
    magA += a[i] * a[i];
    magB += b[i] * b[i];
  }
  const denom = Math.sqrt(magA) * Math.sqrt(magB);
  return denom === 0 ? 0 : dot / denom;
}

// Extract text for embedding from messages (last user message)
function extractTextForEmbedding(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return "";

  const userMsgs = messages.filter((m) => m.role === "user");
  const lastUser = userMsgs[userMsgs.length - 1];
  if (!lastUser) return "";

  const content = typeof lastUser.content === "string"
    ? lastUser.content
    : Array.isArray(lastUser.content)
      ? lastUser.content.filter((p) => p.type === "text").map((p) => p.text).join(" ")
      : "";

  return content.trim();
}

// Call embedding API
async function getEmbedding(text, config) {
  const url = `${config.providerUrl}/embeddings`;
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${config.apiKey}`,
    },
    body: JSON.stringify({
      model: config.model,
      input: text,
    }),
    signal: AbortSignal.timeout(10000), // 10s timeout
  });

  if (!response.ok) {
    throw new Error(`Embedding API error: ${response.status}`);
  }

  const data = await response.json();
  if (!data.data || !data.data[0] || !data.data[0].embedding) {
    throw new Error("Invalid embedding response format");
  }

  return data.data[0].embedding;
}

// Simple hash for dedup
function simpleHash(str) {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }
  return hash.toString(36);
}

// Search for semantically similar cached response using embeddings
export async function findEmbeddingMatch(model, messages) {
  ensureInit();
  const config = getEmbeddingConfig();
  if (!config.enabled || !config.apiKey) return null;

  const text = extractTextForEmbedding(messages);
  if (!text || text.length < 10) return null;

  try {
    const queryEmbedding = await getEmbedding(text, config);

    const db = getDb();
    const rows = db
      .prepare(
        `SELECT id, embedding, response, hits FROM embedding_cache
         WHERE model = ? AND expires_at > datetime('now')
         ORDER BY hits DESC, created_at DESC LIMIT 100`
      )
      .all(model);

    let bestMatch = null;
    let bestScore = 0;

    for (const row of rows) {
      let cachedEmbedding;
      try {
        cachedEmbedding = JSON.parse(row.embedding);
      } catch {
        continue;
      }

      const score = cosineSimilarity(queryEmbedding, cachedEmbedding);

      if (score > bestScore && score >= config.similarityThreshold) {
        bestScore = score;
        bestMatch = row;
      }
    }

    if (bestMatch) {
      db.prepare("UPDATE embedding_cache SET hits = hits + 1 WHERE id = ?").run(bestMatch.id);
      try {
        return { response: JSON.parse(bestMatch.response), score: bestScore };
      } catch {
        return null;
      }
    }

    return null;
  } catch (error) {
    // Silently fail — don't break the request
    console.error("[embedding-cache] lookup error:", error.message);
    return null;
  }
}

// Store response in embedding cache
export async function storeEmbeddingCache(model, messages, response) {
  ensureInit();
  const config = getEmbeddingConfig();
  if (!config.enabled || !config.apiKey) return;

  const text = extractTextForEmbedding(messages);
  if (!text || text.length < 10) return;

  try {
    const db = getDb();

    // Simple hash for dedup
    const msgHash = simpleHash(JSON.stringify(messages.slice(-3)));

    // Check if exact hash already exists
    const existing = db
      .prepare("SELECT id FROM embedding_cache WHERE messages_hash = ? AND model = ?")
      .get(msgHash, model);
    if (existing) return;

    // Get embedding
    const embedding = await getEmbedding(text, config);

    // Enforce max entries (evict oldest low-hit entries)
    const count = db.prepare("SELECT COUNT(*) as cnt FROM embedding_cache WHERE model = ?").get(model);
    if (count && count.cnt >= config.maxEntries) {
      db.prepare(
        `DELETE FROM embedding_cache WHERE id IN (
          SELECT id FROM embedding_cache WHERE model = ? ORDER BY hits ASC, created_at ASC LIMIT 50
        )`
      ).run(model);
    }

    // Clean expired
    db.prepare("DELETE FROM embedding_cache WHERE expires_at < datetime('now')").run();

    const expiresAt = new Date(Date.now() + config.ttlSeconds * 1000).toISOString();

    db.prepare(
      `INSERT INTO embedding_cache (model, embedding, messages_hash, response, expires_at)
       VALUES (?, ?, ?, ?, ?)`
    ).run(model, JSON.stringify(embedding), msgHash, JSON.stringify(response), expiresAt);
  } catch (error) {
    // Silently fail — don't break the request
    console.error("[embedding-cache] store error:", error.message);
  }
}

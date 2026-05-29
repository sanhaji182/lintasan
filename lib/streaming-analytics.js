/**
 * Streaming Analytics - In-memory metrics collector for real-time analytics
 * Ring buffer of last 1000 requests with time-series bucketing
 */

const RING_BUFFER_SIZE = 1000;
const ONE_HOUR_MS = 60 * 60 * 1000;
const TWENTY_FOUR_HOURS_MS = 24 * 60 * 60 * 1000;
const ONE_MIN_MS = 60 * 1000;
const FIFTEEN_MIN_MS = 15 * 60 * 1000;

// Ring buffer for recent requests
const ringBuffer = [];
let ringIndex = 0;
let totalRecorded = 0;

// Time-series buckets: Map<bucketKey, aggregatedData>
// 1-min buckets for last hour
const oneMinBuckets = new Map();
// 15-min buckets for last 24h
const fifteenMinBuckets = new Map();

// Cleanup interval reference
let cleanupInterval = null;

function getBucketKey(timestamp, intervalMs) {
  return Math.floor(timestamp / intervalMs) * intervalMs;
}

function ensureCleanup() {
  if (cleanupInterval) return;
  cleanupInterval = setInterval(() => {
    const now = Date.now();

    // Clean 1-min buckets older than 1 hour
    for (const [key] of oneMinBuckets) {
      if (now - key > ONE_HOUR_MS + ONE_MIN_MS) {
        oneMinBuckets.delete(key);
      }
    }

    // Clean 15-min buckets older than 24 hours
    for (const [key] of fifteenMinBuckets) {
      if (now - key > TWENTY_FOUR_HOURS_MS + FIFTEEN_MIN_MS) {
        fifteenMinBuckets.delete(key);
      }
    }
  }, ONE_MIN_MS);

  // Allow process to exit without waiting for this interval
  if (cleanupInterval.unref) {
    cleanupInterval.unref();
  }
}

function createEmptyBucket() {
  return {
    count: 0,
    errors: 0,
    totalLatency: 0,
    latencies: [],
    totalInputTokens: 0,
    totalOutputTokens: 0,
    cacheHits: { exact: 0, semantic: 0, stream: 0 },
    models: {},
    providers: {},
  };
}

function addToBucket(bucket, data) {
  bucket.count++;
  if (data.status && data.status !== 200) {
    bucket.errors++;
  }
  if (data.latencyMs) {
    bucket.totalLatency += data.latencyMs;
    bucket.latencies.push(data.latencyMs);
  }
  bucket.totalInputTokens += data.inputTokens || 0;
  bucket.totalOutputTokens += data.outputTokens || 0;

  // Cache hit tracking
  if (data.provider === 'cache') bucket.cacheHits.exact++;
  else if (data.provider === 'semantic-cache') bucket.cacheHits.semantic++;
  else if (data.provider === 'stream-cache') bucket.cacheHits.stream++;

  // Model tracking
  if (data.model) {
    if (!bucket.models[data.model]) {
      bucket.models[data.model] = { count: 0, totalLatency: 0, tokens: 0 };
    }
    bucket.models[data.model].count++;
    bucket.models[data.model].totalLatency += data.latencyMs || 0;
    bucket.models[data.model].tokens += (data.inputTokens || 0) + (data.outputTokens || 0);
  }

  // Provider tracking
  if (data.provider) {
    if (!bucket.providers[data.provider]) {
      bucket.providers[data.provider] = { count: 0, totalLatency: 0, errors: 0, tokens: 0 };
    }
    bucket.providers[data.provider].count++;
    bucket.providers[data.provider].totalLatency += data.latencyMs || 0;
    bucket.providers[data.provider].tokens += (data.inputTokens || 0) + (data.outputTokens || 0);
    if (data.status && data.status !== 200) {
      bucket.providers[data.provider].errors++;
    }
  }
}

/**
 * Record a metric data point
 * @param {Object} data - { connectionId, provider, model, status, inputTokens, outputTokens, latencyMs, error, timestamp }
 */
export function recordMetric(data) {
  ensureCleanup();

  const timestamp = data.timestamp || Date.now();
  const entry = { ...data, timestamp };

  // Add to ring buffer
  if (ringBuffer.length < RING_BUFFER_SIZE) {
    ringBuffer.push(entry);
  } else {
    ringBuffer[ringIndex] = entry;
  }
  ringIndex = (ringIndex + 1) % RING_BUFFER_SIZE;
  totalRecorded++;

  // Add to 1-min bucket
  const oneMinKey = getBucketKey(timestamp, ONE_MIN_MS);
  if (!oneMinBuckets.has(oneMinKey)) {
    oneMinBuckets.set(oneMinKey, createEmptyBucket());
  }
  addToBucket(oneMinBuckets.get(oneMinKey), entry);

  // Add to 15-min bucket
  const fifteenMinKey = getBucketKey(timestamp, FIFTEEN_MIN_MS);
  if (!fifteenMinBuckets.has(fifteenMinKey)) {
    fifteenMinBuckets.set(fifteenMinKey, createEmptyBucket());
  }
  addToBucket(fifteenMinBuckets.get(fifteenMinKey), entry);
}

function percentile(sortedArr, p) {
  if (sortedArr.length === 0) return 0;
  const idx = Math.ceil((p / 100) * sortedArr.length) - 1;
  return sortedArr[Math.max(0, idx)];
}

/**
 * Get real-time stats computed from the ring buffer
 */
export function getRealtimeStats() {
  const now = Date.now();
  const entries = ringBuffer.filter(e => e !== undefined);

  if (entries.length === 0) {
    return {
      totalRequests: 0,
      requestsPerSec: 0,
      latency: { p50: 0, p95: 0, p99: 0, avg: 0 },
      throughput: { tokensPerSec: 0, totalInputTokens: 0, totalOutputTokens: 0 },
      cacheHitRate: { exact: 0, semantic: 0, stream: 0, total: 0 },
      errorRate: 0,
      bufferSize: 0,
      totalRecorded,
    };
  }

  // Calculate time window from buffer entries
  const timestamps = entries.map(e => e.timestamp).sort((a, b) => a - b);
  const windowMs = Math.max(now - timestamps[0], 1000); // at least 1 second
  const windowSec = windowMs / 1000;

  // Latency percentiles
  const latencies = entries
    .map(e => e.latencyMs)
    .filter(l => l !== undefined && l !== null)
    .sort((a, b) => a - b);

  const totalLatency = latencies.reduce((sum, l) => sum + l, 0);

  // Token throughput
  const totalInputTokens = entries.reduce((sum, e) => sum + (e.inputTokens || 0), 0);
  const totalOutputTokens = entries.reduce((sum, e) => sum + (e.outputTokens || 0), 0);
  const totalTokens = totalInputTokens + totalOutputTokens;

  // Cache hits
  const exactHits = entries.filter(e => e.provider === 'cache').length;
  const semanticHits = entries.filter(e => e.provider === 'semantic-cache').length;
  const streamHits = entries.filter(e => e.provider === 'stream-cache').length;
  const totalCacheHits = exactHits + semanticHits + streamHits;

  // Errors
  const errors = entries.filter(e => e.status && e.status !== 200).length;

  return {
    totalRequests: entries.length,
    requestsPerSec: Math.round((entries.length / windowSec) * 100) / 100,
    latency: {
      p50: percentile(latencies, 50),
      p95: percentile(latencies, 95),
      p99: percentile(latencies, 99),
      avg: latencies.length > 0 ? Math.round(totalLatency / latencies.length) : 0,
    },
    throughput: {
      tokensPerSec: Math.round((totalTokens / windowSec) * 100) / 100,
      totalInputTokens,
      totalOutputTokens,
    },
    cacheHitRate: {
      exact: entries.length > 0 ? Math.round((exactHits / entries.length) * 10000) / 100 : 0,
      semantic: entries.length > 0 ? Math.round((semanticHits / entries.length) * 10000) / 100 : 0,
      stream: entries.length > 0 ? Math.round((streamHits / entries.length) * 10000) / 100 : 0,
      total: entries.length > 0 ? Math.round((totalCacheHits / entries.length) * 10000) / 100 : 0,
    },
    errorRate: entries.length > 0 ? Math.round((errors / entries.length) * 10000) / 100 : 0,
    bufferSize: entries.length,
    totalRecorded,
  };
}

/**
 * Get time-series data for a given period
 * @param {'1h'|'24h'} period
 */
export function getTimeSeries(period = '1h') {
  const now = Date.now();
  const series = [];

  if (period === '1h') {
    const startTime = now - ONE_HOUR_MS;
    const bucketKeys = [...oneMinBuckets.keys()]
      .filter(k => k >= startTime)
      .sort((a, b) => a - b);

    for (const key of bucketKeys) {
      const bucket = oneMinBuckets.get(key);
      const latencies = bucket.latencies.slice().sort((a, b) => a - b);
      series.push({
        timestamp: key,
        time: new Date(key).toISOString(),
        requests: bucket.count,
        errors: bucket.errors,
        avgLatency: bucket.count > 0 ? Math.round(bucket.totalLatency / bucket.count) : 0,
        p95Latency: percentile(latencies, 95),
        inputTokens: bucket.totalInputTokens,
        outputTokens: bucket.totalOutputTokens,
        cacheHits: bucket.cacheHits,
        errorRate: bucket.count > 0 ? Math.round((bucket.errors / bucket.count) * 10000) / 100 : 0,
      });
    }
  } else if (period === '24h') {
    const startTime = now - TWENTY_FOUR_HOURS_MS;
    const bucketKeys = [...fifteenMinBuckets.keys()]
      .filter(k => k >= startTime)
      .sort((a, b) => a - b);

    for (const key of bucketKeys) {
      const bucket = fifteenMinBuckets.get(key);
      const latencies = bucket.latencies.slice().sort((a, b) => a - b);
      series.push({
        timestamp: key,
        time: new Date(key).toISOString(),
        requests: bucket.count,
        errors: bucket.errors,
        avgLatency: bucket.count > 0 ? Math.round(bucket.totalLatency / bucket.count) : 0,
        p95Latency: percentile(latencies, 95),
        inputTokens: bucket.totalInputTokens,
        outputTokens: bucket.totalOutputTokens,
        cacheHits: bucket.cacheHits,
        errorRate: bucket.count > 0 ? Math.round((bucket.errors / bucket.count) * 10000) / 100 : 0,
      });
    }
  }

  return series;
}

/**
 * Get top models by request count
 * @param {number} limit
 */
export function getTopModels(limit = 10) {
  const modelMap = new Map();

  for (const entry of ringBuffer) {
    if (!entry || !entry.model) continue;
    if (!modelMap.has(entry.model)) {
      modelMap.set(entry.model, { model: entry.model, count: 0, totalLatency: 0, tokens: 0, errors: 0 });
    }
    const m = modelMap.get(entry.model);
    m.count++;
    m.totalLatency += entry.latencyMs || 0;
    m.tokens += (entry.inputTokens || 0) + (entry.outputTokens || 0);
    if (entry.status && entry.status !== 200) m.errors++;
  }

  return [...modelMap.values()]
    .sort((a, b) => b.count - a.count)
    .slice(0, limit)
    .map(m => ({
      model: m.model,
      requests: m.count,
      avgLatency: m.count > 0 ? Math.round(m.totalLatency / m.count) : 0,
      totalTokens: m.tokens,
      errorRate: m.count > 0 ? Math.round((m.errors / m.count) * 10000) / 100 : 0,
    }));
}

/**
 * Get top providers by request count
 * @param {number} limit
 */
export function getTopProviders(limit = 10) {
  const providerMap = new Map();

  for (const entry of ringBuffer) {
    if (!entry || !entry.provider) continue;
    if (!providerMap.has(entry.provider)) {
      providerMap.set(entry.provider, { provider: entry.provider, count: 0, totalLatency: 0, tokens: 0, errors: 0 });
    }
    const p = providerMap.get(entry.provider);
    p.count++;
    p.totalLatency += entry.latencyMs || 0;
    p.tokens += (entry.inputTokens || 0) + (entry.outputTokens || 0);
    if (entry.status && entry.status !== 200) p.errors++;
  }

  return [...providerMap.values()]
    .sort((a, b) => b.count - a.count)
    .slice(0, limit)
    .map(p => ({
      provider: p.provider,
      requests: p.count,
      avgLatency: p.count > 0 ? Math.round(p.totalLatency / p.count) : 0,
      totalTokens: p.tokens,
      errorRate: p.count > 0 ? Math.round((p.errors / p.count) * 10000) / 100 : 0,
    }));
}

/**
 * Stop the cleanup interval (for graceful shutdown)
 */
export function stopCleanup() {
  if (cleanupInterval) {
    clearInterval(cleanupInterval);
    cleanupInterval = null;
  }
}

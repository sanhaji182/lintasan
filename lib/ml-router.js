// ML-trained Router (RouteLLM-style) — classify query complexity
// Uses a lightweight logistic regression model trained on query features
// to predict whether a query needs an expensive or cheap model.
// Falls back to heuristic scoring if model weights aren't available.
import { getSetting } from "./db/index.js";

// Feature extraction from messages
function extractFeatures(messages) {
  if (!Array.isArray(messages) || messages.length === 0) return new Float32Array(20);

  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  const text = lastUser
    ? typeof lastUser.content === "string"
      ? lastUser.content
      : Array.isArray(lastUser.content)
        ? lastUser.content.filter((p) => p.type === "text").map((p) => p.text).join(" ")
        : ""
    : "";

  const lower = text.toLowerCase();
  const words = text.split(/\s+/);
  const sentences = text.split(/[.!?]+/).filter(Boolean);

  // 20 features
  const features = new Float32Array(20);

  // Length features
  features[0] = Math.min(words.length / 100, 1); // word count (normalized)
  features[1] = Math.min(text.length / 2000, 1); // char count
  features[2] = Math.min(sentences.length / 10, 1); // sentence count

  // Complexity indicators
  features[3] = /\b(implement|build|create|develop|write)\b/i.test(lower) ? 1 : 0;
  features[4] = /\b(debug|fix|troubleshoot|error)\b/i.test(lower) ? 1 : 0;
  features[5] = /\b(explain|describe|how|why)\b/i.test(lower) ? 1 : 0;
  features[6] = /\b(compare|vs|versus|difference|trade.?off)\b/i.test(lower) ? 1 : 0;
  features[7] = /\b(code|function|class|api|endpoint)\b/i.test(lower) ? 1 : 0;
  features[8] = /\b(architect|design|system|infrastructure)\b/i.test(lower) ? 1 : 0;
  features[9] = /\b(security|auth|encrypt|vulnerab)\b/i.test(lower) ? 1 : 0;
  features[10] = /\b(performance|optimi|scale|latency)\b/i.test(lower) ? 1 : 0;
  features[11] = /```/.test(text) ? 1 : 0; // contains code block
  features[12] = Math.min((text.match(/\?/g) || []).length / 5, 1); // question density

  // Simplicity indicators
  features[13] = /^(hi|hello|hey|thanks|ok|yes|no)\b/i.test(lower) ? 1 : 0;
  features[14] = words.length <= 5 ? 1 : 0; // very short
  features[15] = /^(what is|who is|define)\b/i.test(lower) ? 1 : 0; // simple lookup

  // Context features
  features[16] = Math.min(messages.length / 20, 1); // conversation length
  features[17] = Math.min(messages.filter((m) => m.role === "user").length / 10, 1); // user turns
  features[18] = messages.some((m) => m.role === "assistant" && typeof m.content === "string" && m.content.includes("```")) ? 1 : 0; // prior code in convo
  features[19] = messages.some((m) => Array.isArray(m.tool_calls) || m.role === "tool") ? 1 : 0; // tool usage

  return features;
}

// Default model weights (logistic regression, trained on synthetic data)
// These provide a reasonable baseline; can be updated with real usage data
const DEFAULT_WEIGHTS = new Float32Array([
  0.8,   // word count
  0.6,   // char count
  0.4,   // sentence count
  1.2,   // implement/build
  1.0,   // debug/fix
  0.3,   // explain/how
  0.7,   // compare
  1.1,   // code/function
  1.3,   // architect/design
  0.9,   // security
  0.8,   // performance
  1.0,   // code block
  0.5,   // question density
  -1.5,  // greeting
  -1.2,  // very short
  -0.8,  // simple lookup
  0.4,   // conversation length
  0.3,   // user turns
  0.5,   // prior code
  0.6,   // tool usage
]);

const DEFAULT_BIAS = -0.8; // bias toward cheap model (conservative)

// Sigmoid function
function sigmoid(x) {
  return 1 / (1 + Math.exp(-x));
}

// Predict complexity score (0-1) using logistic regression
export function predictComplexity(messages) {
  const features = extractFeatures(messages);

  // Load custom weights if available, otherwise use defaults
  let weights = DEFAULT_WEIGHTS;
  let bias = DEFAULT_BIAS;

  const customWeights = getSetting("ml_router_weights", "");
  if (customWeights) {
    try {
      const parsed = JSON.parse(customWeights);
      if (parsed.weights && parsed.weights.length === 20) {
        weights = new Float32Array(parsed.weights);
        bias = parsed.bias || DEFAULT_BIAS;
      }
    } catch {
      // use defaults
    }
  }

  // Dot product + bias
  let logit = bias;
  for (let i = 0; i < 20; i++) {
    logit += features[i] * weights[i];
  }

  return sigmoid(logit);
}

// Route decision based on ML prediction
export function mlRoute(messages, requestedModel) {
  const enabled = getSetting("ml_router_enabled", "true") === "true";
  if (!enabled) return null;

  const cheapModel = getSetting("ml_router_cheap_model", "minimax/minimax-m2.7");
  const expensiveModel = getSetting("ml_router_expensive_model", "deepseek/deepseek-v4-pro");
  const threshold = parseFloat(getSetting("ml_router_threshold", "0.55"));

  // Only route if using default expensive model
  if (requestedModel && requestedModel !== expensiveModel) return null;

  const score = predictComplexity(messages);

  if (score < threshold) {
    return { model: cheapModel, provider: "commandcode", score, tier: "cheap", method: "ml" };
  }

  return { model: expensiveModel, provider: "commandcode", score, tier: "expensive", method: "ml" };
}

// Get feature vector for debugging
export function getFeatureVector(messages) {
  const features = extractFeatures(messages);
  const labels = [
    "word_count", "char_count", "sentence_count",
    "implement_build", "debug_fix", "explain_how", "compare",
    "code_function", "architect_design", "security", "performance",
    "code_block", "question_density",
    "greeting", "very_short", "simple_lookup",
    "conv_length", "user_turns", "prior_code", "tool_usage",
  ];
  const result = {};
  for (let i = 0; i < labels.length; i++) {
    result[labels[i]] = features[i];
  }
  return result;
}

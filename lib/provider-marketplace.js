// Community-contributed provider presets marketplace
// Users can submit presets via GitHub PR to this catalog

export const MARKETPLACE_PROVIDERS = [
  // === Free / No-Key Providers ===
  {
    id: "ollama",
    name: "Ollama",
    category: "local",
    author: "community",
    description: "Run LLMs locally. Supports Llama, Mistral, Gemma, Phi, and more.",
    baseUrl: "http://127.0.0.1:11434",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/api/tags",
    authRequired: false,
    tags: ["local", "free", "privacy"],
    popularity: 95,
  },
  {
    id: "lmstudio",
    name: "LM Studio",
    category: "local",
    author: "community",
    description: "Desktop app for running local LLMs with OpenAI-compatible API.",
    baseUrl: "http://127.0.0.1:1234",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: false,
    tags: ["local", "free", "gui"],
    popularity: 88,
  },

  // === Aggregators ===
  {
    id: "openrouter",
    name: "OpenRouter",
    category: "aggregator",
    author: "official",
    description: "200+ models from all providers. Pay-per-token, no subscriptions.",
    baseUrl: "https://openrouter.ai/api",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["aggregator", "multi-model", "pay-per-use"],
    popularity: 92,
  },
  {
    id: "together",
    name: "Together AI",
    category: "aggregator",
    author: "official",
    description: "Fast inference for open-source models. Free tier available.",
    baseUrl: "https://api.together.xyz",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["fast", "open-source", "free-tier"],
    popularity: 78,
  },

  // === Major Providers ===
  {
    id: "deepseek",
    name: "DeepSeek",
    category: "major",
    author: "official",
    description: "DeepSeek V4 Pro, R1. High quality reasoning at low cost.",
    baseUrl: "https://api.deepseek.com",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["reasoning", "coding", "cheap"],
    popularity: 90,
  },
  {
    id: "groq",
    name: "Groq",
    category: "fast-inference",
    author: "official",
    description: "Ultra-fast inference. Llama, Mixtral at 500+ tok/s. Free tier.",
    baseUrl: "https://api.groq.com/openai",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["fast", "free-tier", "open-source"],
    popularity: 85,
  },
  {
    id: "cerebras",
    name: "Cerebras",
    category: "fast-inference",
    author: "official",
    description: "Fastest inference in the world. 2000+ tok/s. Free tier.",
    baseUrl: "https://api.cerebras.ai",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["fastest", "free-tier"],
    popularity: 75,
  },
  {
    id: "sambanova",
    name: "SambaNova",
    category: "fast-inference",
    author: "official",
    description: "Fast open-source model inference. Free tier with generous limits.",
    baseUrl: "https://api.sambanova.ai",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["fast", "free-tier"],
    popularity: 70,
  },

  // === Chinese Providers ===
  {
    id: "kimi",
    name: "Kimi (Moonshot)",
    category: "chinese",
    author: "community",
    description: "Kimi K2.6 — strong coding and reasoning. Competitive pricing.",
    baseUrl: "https://api.moonshot.cn",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["coding", "chinese", "cheap"],
    popularity: 72,
  },
  {
    id: "qwen",
    name: "Qwen (Alibaba)",
    category: "chinese",
    author: "community",
    description: "Qwen3 Coder — excellent for code generation and analysis.",
    baseUrl: "https://dashscope.aliyuncs.com/compatible-mode",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["coding", "chinese", "multilingual"],
    popularity: 74,
  },
  {
    id: "siliconflow",
    name: "SiliconFlow",
    category: "chinese",
    author: "community",
    description: "Chinese model aggregator. DeepSeek, Qwen, GLM at low cost.",
    baseUrl: "https://api.siliconflow.cn",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["aggregator", "chinese", "cheap"],
    popularity: 68,
  },

  // === Indonesia ===
  {
    id: "sumopod",
    name: "Sumopod",
    category: "indonesia",
    author: "official",
    description: "Indonesian AI gateway. 53+ models including GPT-5, Claude, DeepSeek.",
    baseUrl: "https://ai.sumopod.com",
    format: "openai",
    chatPath: "/v1/chat/completions",
    modelsPath: "/v1/models",
    authRequired: true,
    tags: ["indonesia", "aggregator", "multi-model"],
    popularity: 65,
  },
];

// Get all marketplace providers
export function getMarketplaceProviders({ category, search, sort = "popularity" } = {}) {
  let results = [...MARKETPLACE_PROVIDERS];

  if (category && category !== "all") {
    results = results.filter((p) => p.category === category);
  }

  if (search) {
    const q = search.toLowerCase();
    results = results.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.description.toLowerCase().includes(q) ||
        p.tags.some((t) => t.includes(q))
    );
  }

  if (sort === "popularity") {
    results.sort((a, b) => b.popularity - a.popularity);
  } else if (sort === "name") {
    results.sort((a, b) => a.name.localeCompare(b.name));
  }

  return results;
}

// Get categories
export function getMarketplaceCategories() {
  return [
    { id: "all", name: "All", count: MARKETPLACE_PROVIDERS.length },
    { id: "local", name: "Local / Free", count: MARKETPLACE_PROVIDERS.filter((p) => p.category === "local").length },
    { id: "aggregator", name: "Aggregators", count: MARKETPLACE_PROVIDERS.filter((p) => p.category === "aggregator").length },
    { id: "major", name: "Major", count: MARKETPLACE_PROVIDERS.filter((p) => p.category === "major").length },
    { id: "fast-inference", name: "Fast Inference", count: MARKETPLACE_PROVIDERS.filter((p) => p.category === "fast-inference").length },
    { id: "chinese", name: "Chinese", count: MARKETPLACE_PROVIDERS.filter((p) => p.category === "chinese").length },
    { id: "indonesia", name: "Indonesia", count: MARKETPLACE_PROVIDERS.filter((p) => p.category === "indonesia").length },
  ];
}

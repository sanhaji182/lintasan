// Provider Registry - built-in + custom providers from DB
import { listCustomProviders } from "../db/index.js";

// Built-in providers
const BUILTIN_PROVIDERS = {
  commandcode: {
    id: "commandcode",
    name: "CommandCode",
    format: "commandcode",
    baseUrl: "https://api.commandcode.ai",
    chatPath: "/alpha/generate",
    modelsPath: null,
    authType: "apikey",
    authHeader: "Authorization",
    authPrefix: "Bearer ",
    headers: { "Content-Type": "application/json" },
    defaultModels: [
      "deepseek/deepseek-v4-pro",
      "kimi/kimi-k2.6",
      "glm/glm-4.7",
      "minimax/minimax-m2.7",
      "qwen/qwen3-coder",
    ],
    builtin: true,
  },
  openai: {
    id: "openai",
    name: "OpenAI",
    format: "openai",
    baseUrl: "https://api.openai.com/v1",
    chatPath: "/chat/completions",
    modelsPath: "/models",
    authType: "apikey",
    authHeader: "Authorization",
    authPrefix: "Bearer ",
    headers: { "Content-Type": "application/json" },
    defaultModels: ["gpt-4o", "gpt-4o-mini", "gpt-4.1", "o3-mini"],
    builtin: true,
  },
  anthropic: {
    id: "anthropic",
    name: "Anthropic",
    format: "anthropic",
    baseUrl: "https://api.anthropic.com/v1",
    chatPath: "/messages",
    modelsPath: null,
    authType: "apikey",
    authHeader: "x-api-key",
    authPrefix: "",
    headers: {
      "Content-Type": "application/json",
      "anthropic-version": "2023-06-01",
    },
    defaultModels: ["claude-sonnet-4-20250514", "claude-haiku-35-20241022", "claude-opus-4-20250514"],
    builtin: true,
  },
  openrouter: {
    id: "openrouter",
    name: "OpenRouter",
    format: "openai",
    baseUrl: "https://openrouter.ai/api/v1",
    chatPath: "/chat/completions",
    modelsPath: "/models",
    authType: "apikey",
    authHeader: "Authorization",
    authPrefix: "Bearer ",
    headers: {
      "Content-Type": "application/json",
      "HTTP-Referer": "https://lintasan.dev",
      "X-Title": "Lintasan",
    },
    defaultModels: [],
    builtin: true,
  },
  deepseek: {
    id: "deepseek",
    name: "DeepSeek",
    format: "openai",
    baseUrl: "https://api.deepseek.com",
    chatPath: "/chat/completions",
    modelsPath: "/models",
    authType: "apikey",
    authHeader: "Authorization",
    authPrefix: "Bearer ",
    headers: { "Content-Type": "application/json" },
    defaultModels: ["deepseek-chat", "deepseek-reasoner"],
    builtin: true,
  },
  groq: {
    id: "groq",
    name: "Groq",
    format: "openai",
    baseUrl: "https://api.groq.com/openai/v1",
    chatPath: "/chat/completions",
    modelsPath: "/models",
    authType: "apikey",
    authHeader: "Authorization",
    authPrefix: "Bearer ",
    headers: { "Content-Type": "application/json" },
    defaultModels: ["llama-3.3-70b-versatile", "mixtral-8x7b-32768"],
    builtin: true,
  },
  together: {
    id: "together",
    name: "Together AI",
    format: "openai",
    baseUrl: "https://api.together.xyz/v1",
    chatPath: "/chat/completions",
    modelsPath: "/models",
    authType: "apikey",
    authHeader: "Authorization",
    authPrefix: "Bearer ",
    headers: { "Content-Type": "application/json" },
    defaultModels: [],
    builtin: true,
  },
};

export function getProvider(id) {
  // Check built-in first
  if (BUILTIN_PROVIDERS[id]) return BUILTIN_PROVIDERS[id];

  // Check custom providers from DB
  try {
    const customs = listCustomProviders();
    const custom = customs.find((p) => p.id === id);
    if (custom) {
      return {
        id: custom.id,
        name: custom.name,
        format: custom.format,
        baseUrl: custom.base_url,
        chatPath: custom.chat_path,
        modelsPath: custom.models_path,
        authType: custom.auth_type,
        authHeader: custom.auth_header,
        authPrefix: custom.auth_prefix,
        headers: JSON.parse(custom.headers || "{}"),
        defaultModels: JSON.parse(custom.default_models || "[]"),
        builtin: false,
      };
    }
  } catch (e) {}

  return null;
}

export function listProviders() {
  const builtins = Object.values(BUILTIN_PROVIDERS);

  try {
    const customs = listCustomProviders().map((p) => ({
      id: p.id,
      name: p.name,
      format: p.format,
      baseUrl: p.base_url,
      chatPath: p.chat_path,
      modelsPath: p.models_path,
      authType: p.auth_type,
      authHeader: p.auth_header,
      authPrefix: p.auth_prefix,
      headers: JSON.parse(p.headers || "{}"),
      defaultModels: JSON.parse(p.default_models || "[]"),
      builtin: false,
    }));
    return [...builtins, ...customs];
  } catch (e) {
    return builtins;
  }
}

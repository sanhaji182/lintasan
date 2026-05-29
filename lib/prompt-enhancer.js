// Prompt Enhancer — inject configurable meta-instructions to improve response quality
import { getDb, getSetting } from "./db/index.js";

const ENHANCEMENT_PROFILES = {
  // Default: minimal injection for conciseness
  balanced: {
    instruction: `Be concise. No unnecessary preamble or filler. Follow instructions precisely. Respect any word/token limits in the prompt.`,
    position: "system_prepend",
  },
  // Coding: optimized for SWE tasks — minimal, just format enforcement
  coding: {
    instruction: `Output only what's asked. No preamble, no summary, no explanation unless asked. Exact format. Minimal tokens. If asked for code, output ONLY code.`,
    position: "system_prepend",
  },
  // Analytical: for reasoning tasks
  analytical: {
    instruction: `Be extremely concise. Answer in minimal words. Use bullet points, not paragraphs. No filler. Match requested format exactly. Respect word/token limits strictly.`,
    position: "system_prepend",
  },
  // Minimal: just format enforcement
  minimal: {
    instruction: `Follow instructions precisely. Match requested format exactly.`,
    position: "system_prepend",
  },
  // Strict concise: when prompt has explicit word limits
  strict_concise: {
    instruction: `CRITICAL: The user specified a word/token limit. You MUST stay within it. Be extremely brief. No filler, no preamble, no repetition. Every word must earn its place.`,
    position: "system_prepend",
  },
  // Custom: user-defined in settings
  custom: null,
};

// Detect task type from messages
function detectTaskType(messages) {
  if (!messages || messages.length === 0) return "balanced";

  const lastUser = [...messages].reverse().find((m) => m.role === "user");
  if (!lastUser) return "balanced";

  const content =
    typeof lastUser.content === "string"
      ? lastUser.content.toLowerCase()
      : Array.isArray(lastUser.content)
        ? lastUser.content
            .filter((p) => p.type === "text")
            .map((p) => p.text)
            .join(" ")
            .toLowerCase()
        : "";

  // Detect explicit word/token limits in prompt
  const wordLimitMatch = content.match(/(?:under|max(?:imum)?|kurang dari|maksimal?)\s*(\d+)\s*(?:words?|kata)/i);
  if (wordLimitMatch) {
    return "strict_concise";
  }

  // Coding indicators
  const codingPatterns = [
    /\b(code|function|class|implement|fix|bug|error|debug|refactor|test|api|endpoint)\b/,
    /\b(typescript|javascript|python|java|rust|go|sql|html|css)\b/,
    /```/,
    /\b(file|module|package|import|export|return)\b/,
    /\.(ts|js|py|java|rs|go|sql|json|yaml|toml)\b/,
  ];

  // Analytical indicators
  const analyticalPatterns = [
    /\b(analyze|compare|explain|why|how|trace|identify|diagnose)\b/,
    /\b(step by step|phase|breakdown|evaluate|assess)\b/,
    /\b(race condition|vulnerability|performance|bottleneck)\b/,
  ];

  const codingScore = codingPatterns.filter((p) => p.test(content)).length;
  const analyticalScore = analyticalPatterns.filter((p) =>
    p.test(content)
  ).length;

  if (codingScore >= 2) return "coding";
  if (analyticalScore >= 2) return "analytical";
  return "balanced";
}

// Main enhancement function
export function enhancePrompt(messages, options = {}) {
  const enabled = getSetting("prompt_enhancer_enabled", "true") === "true";
  if (!enabled) return messages;

  const profileName =
    options.profile || getSetting("prompt_enhancer_profile", "auto");
  let profile;

  if (profileName === "auto") {
    const taskType = detectTaskType(messages);
    profile = ENHANCEMENT_PROFILES[taskType];
  } else if (profileName === "custom") {
    const customInstruction = getSetting("prompt_enhancer_custom", "");
    if (!customInstruction) return messages;
    profile = { instruction: customInstruction, position: "system_prepend" };
  } else {
    profile = ENHANCEMENT_PROFILES[profileName] || ENHANCEMENT_PROFILES.balanced;
  }

  if (!profile || !profile.instruction) return messages;

  // Clone messages to avoid mutation
  const enhanced = [...messages];

  // Find existing system message
  const systemIdx = enhanced.findIndex((m) => m.role === "system");

  if (profile.position === "system_prepend") {
    if (systemIdx >= 0) {
      // Prepend to existing system message
      enhanced[systemIdx] = {
        ...enhanced[systemIdx],
        content: `${profile.instruction}\n\n${enhanced[systemIdx].content}`,
      };
    } else {
      // Insert new system message at the beginning
      enhanced.unshift({ role: "system", content: profile.instruction });
    }
  } else if (profile.position === "system_append") {
    if (systemIdx >= 0) {
      enhanced[systemIdx] = {
        ...enhanced[systemIdx],
        content: `${enhanced[systemIdx].content}\n\n${profile.instruction}`,
      };
    } else {
      enhanced.unshift({ role: "system", content: profile.instruction });
    }
  }

  return enhanced;
}

// Get current profile info (for dashboard)
export function getEnhancerStatus() {
  return {
    enabled: getSetting("prompt_enhancer_enabled", "true") === "true",
    profile: getSetting("prompt_enhancer_profile", "auto"),
    profiles: Object.keys(ENHANCEMENT_PROFILES).filter((k) => k !== "custom"),
    customInstruction: getSetting("prompt_enhancer_custom", ""),
  };
}

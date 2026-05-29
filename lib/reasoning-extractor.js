/**
 * Reasoning Extractor — Extracts code/content from reasoning_content
 * 
 * Some models (DeepSeek V4 Pro, etc.) return their actual answer inside
 * `reasoning_content` field instead of `content`. This middleware detects
 * that pattern and moves the final answer into `content` so IDEs can read it.
 */

/**
 * Check if a response has reasoning_content but empty content, and fix it
 * @param {object} data - The response JSON from upstream
 * @returns {object} - Fixed response with content populated
 */
export function extractReasoningContent(data) {
  if (!data?.choices?.[0]?.message) return data;

  const msg = data.choices[0].message;
  const content = msg.content || "";
  const reasoning = msg.reasoning_content || "";

  // If neither has data, skip
  if (!content.trim() && !reasoning.trim()) return data;

  // Check if content already has a code block (>50 chars) — if so, leave it
  const contentCodeBlocks = [...content.matchAll(/```(?:\w*)\n?([\s\S]*?)```/g)];
  const hasContentCode = contentCodeBlocks.some(m => m[1].trim().length > 50);
  if (hasContentCode) return data;

  // Content has no code — try extracting from reasoning
  if (reasoning.trim()) {
    const extracted = extractFinalAnswer(reasoning);
    if (extracted) {
      data.choices[0].message.content = extracted;
      data._reasoning_extracted = true;
    }
  }

  return data;
}

/**
 * Extract the final code block or answer from reasoning text
 * Reasoning typically contains thinking + final code block
 */
function extractFinalAnswer(reasoning) {
  // Strategy 0: Check if reasoning itself starts with code (no prose prefix)
  // Some models put the full answer as raw code at the start without code fences
  const trimmed = reasoning.trim();
  if (trimmed.startsWith("import ") || trimmed.startsWith("def ") || trimmed.startsWith("class ")) {
    return trimTrailingProse(trimmed);
  }

  // Strategy 1: Collect ALL code blocks and score them for completeness.
  // DeepSeek V4 Pro scatters code in reasoning — the LAST block is rarely the answer.
  // We want the block with the most requirement coverage.
  const codeBlocks = [...reasoning.matchAll(/```(?:python|javascript|typescript|go|rust|java|cpp|c|ruby|php|swift|kotlin|scala|sql|bash|sh|yaml|json|toml|xml|html|css|markdown|text|plaintext)?\s*\n(.*?)```/gs)];
  
  if (codeBlocks.length > 0) {
    // Score each block by completeness signals
    const scored = codeBlocks.map(m => ({ code: m[1].trim(), score: 0 }));
    for (const entry of scored) {
      const c = entry.code;
      // Import/definition completeness
      if (/^import\s/.test(c)) entry.score += 3;
      if (/def\s+\w+/.test(c)) entry.score += 2;
      if (/class\s+\w+/.test(c)) entry.score += 2;
      // Requirement-specific signals
      if (/Condition/.test(c)) entry.score += 2;
      if (/\.wait\(/.test(c)) entry.score += 2;
      if (/\.notify/.test(c)) entry.score += 2;
      if (/finally:/.test(c)) entry.score += 2;
      if (/__enter__/.test(c) && /__exit__/.test(c)) entry.score += 2;
      if (/release_connection/.test(c)) entry.score += 1;
      if (/execute_query/.test(c)) entry.score += 1;
      if (/is_stale/.test(c)) entry.score += 1;
      // Size bonus — bigger blocks are usually the full answer
      entry.score += Math.min(Math.floor(c.length / 200), 5);
    }
    scored.sort((a, b) => b.score - a.score);
    const best = scored[0];
    if (best && best.code.length > 50) return trimTrailingProse(best.code);
  }

  // Strategy 2: Find the largest standalone code section (no fences)
  // Look for "import" or "def" after a blank line — typical final-answer pattern
  const importMarker = /\n\n(import\s+[\s\S]+)$/;
  const im = reasoning.match(importMarker);
  if (im) {
    const code = im[1].trim();
    if (code.length > 100) return trimTrailingProse(code);
  }

  // Strategy 3: Find the last occurrence of "import " or "def " followed by the rest
  const lastImport = reasoning.lastIndexOf("\nimport ");
  const lastDef = reasoning.lastIndexOf("\ndef ");
  const start = Math.max(lastImport, lastDef);
  if (start > 0) {
    const code = reasoning.substring(start).trim();
    return trimTrailingProse(code);
  }

  // Strategy 4: Old marker-based fallback
  const finalMarkers = [
    /(?:final|complete|corrected|fixed|updated|full)\s+(?:code|solution|version|implementation)[:\s]*\n([\s\S]+)$/i,
    /(?:here(?:'s| is) the|so the)\s+(?:final|complete|corrected|fixed|full)?\s*(?:code|solution|implementation)[:\s]*\n([\s\S]+)$/i,
  ];
  for (const marker of finalMarkers) {
    const match = reasoning.match(marker);
    if (match) {
      let code = match[1].trim();
      const nextExplanation = code.search(/\n\n(?:This|Note|The|In |So |Now |Let|We )/);
      if (nextExplanation > 200) {
        code = code.substring(0, nextExplanation).trim();
      }
      if (code.length > 50) return code;
    }
  }

  return null;
}

/** Trim trailing prose lines from a code block */
function trimTrailingProse(code) {
  const lines = code.split("\n");
  let lastCodeLine = lines.length;
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i].trim();
    if (
      line === "" ||
      line.startsWith("import ") ||
      line.startsWith("from ") ||
      line.startsWith("def ") ||
      line.startsWith("class ") ||
      line.startsWith("return ") ||
      line.startsWith("self.") ||
      line.startsWith("if ") ||
      line.startsWith("for ") ||
      line.startsWith("while ") ||
      line.startsWith("try:") ||
      line.startsWith("except") ||
      line.startsWith("finally:") ||
      line.startsWith("with ") ||
      line.startsWith("raise ") ||
      line.startsWith("#") ||
      line.startsWith("@") ||
      /^\s/.test(line) ||
      /^[}\]):]/.test(line) ||
      /^\w+\s*[=(]/.test(line) ||
      /^\w+\.\w+/.test(line)
    ) {
      lastCodeLine = i + 1;
      break;
    }
  }
  code = lines.slice(0, lastCodeLine).join("\n").trim();
  return code.length > 50 ? code : null;
}

/**
 * Check if a streaming response needs reasoning extraction
 * For SSE streams, we need to buffer and post-process
 */
export function isReasoningModel(data) {
  if (!data?.choices?.[0]?.message) return false;
  const msg = data.choices[0].message;
  return (!msg.content || !msg.content.trim()) && msg.reasoning_content && msg.reasoning_content.trim().length > 0;
}

#!/usr/bin/env python3
"""
SWE-Bench Monorepo Challenge v2 — Split into focused sub-tasks
Each sub-task tests one specific SWE skill
"""
import json, time, requests, subprocess, tempfile, os, sys, re, statistics

# Config
LINTASAN = {"url": "http://127.0.0.1:20180/api/v1/chat/completions", "key": "sk-sans-test-key", "name": "Lintasan"}
NINER_KEY = os.environ.get("NINER_API_KEY", "your-9router-key-here")
NINER = {"url": "http://127.0.0.1:20128/api/v1/chat/completions", "key": NINER_KEY, "name": "9Router"}

# Buggy source code shared across tasks
AUTH_CODE = '''const jwt = require('jsonwebtoken');
const JWT_SECRET = 'dev-secret';
const REFRESH_SECRET = 'refresh-secret';

class SessionStore {
  constructor() {
    this.sessions = new Map();
    this.refreshTokens = new Set(); // track valid refresh tokens
  }
  async createSession(userId) {
    const id = `sess_${Date.now()}_${Math.random().toString(36).slice(2)}`;
    const session = { id, userId, createdAt: Date.now(), isActive: true };
    this.sessions.set(id, session);
    return session;
  }
  async getSession(id) { return this.sessions.get(id) || null; }
  async getActiveSessionsByUser(userId) {
    return [...this.sessions.values()].filter(s => s.userId === userId && s.isActive);
  }
  async invalidateSession(id) {
    const s = this.sessions.get(id);
    if (s) { s.isActive = false; return true; }
    return false;
  }
}'''

TASKS = [
    {
        "id": "MONO-1",
        "name": "Identify & explain race condition",
        "category": "Code Analysis",
        "difficulty": "Medium",
        "prompt": f"""Analyze this authentication code and identify the race condition:

```javascript
{AUTH_CODE}

class AuthService {{
  constructor() {{
    this.store = new SessionStore();
  }}

  async refreshToken(refreshToken) {{
    const decoded = jwt.verify(refreshToken, REFRESH_SECRET);
    const session = await this.store.getSession(decoded.sessionId);
    if (!session || !session.isActive) throw new Error('Invalid session');
    
    // Generate new tokens
    const newAccess = jwt.sign({{ userId: decoded.userId, sessionId: session.id }}, JWT_SECRET, {{ expiresIn: '15m' }});
    const newRefresh = jwt.sign({{ userId: decoded.userId, sessionId: session.id }}, REFRESH_SECRET, {{ expiresIn: '7d' }});
    
    return {{ accessToken: newAccess, refreshToken: newRefresh }};
  }}
}}
```

Questions:
1. What is the race condition? (2 sentences max)
2. What attack does this enable? (1 sentence)
3. What's the fix in 1 sentence?

Output ONLY the 3 numbered answers, nothing else.""",
        "verify": lambda text: (
            sum([
                15 if re.search(r"(concurrent|simultaneous|parallel|multiple.*request|race)", text, re.I) else 0,
                15 if re.search(r"(reuse|replay|reused|old.*token|same.*token)", text, re.I) else 0,
                10 if re.search(r"(lock|mutex|atomic|invalidat|revok|one-time|single.use)", text, re.I) else 0,
            ]),
            40
        ),
    },
    {
        "id": "MONO-2",
        "name": "Fix token reuse vulnerability",
        "category": "Security Fix",
        "difficulty": "Hard",
        "prompt": f"""Fix this refresh token function to prevent token reuse attacks. The old refresh token must be invalidated after use.

```javascript
class TokenStore {{
  constructor() {{
    this.usedTokens = new Set();
  }}
  isUsed(token) {{ return this.usedTokens.has(token); }}
  markUsed(token) {{ this.usedTokens.add(token); }}
}}

// BUGGY - fix this function
async function refreshToken(token, tokenStore) {{
  // No check if token was already used!
  const decoded = jwt.verify(token, 'secret');
  const newToken = jwt.sign({{ userId: decoded.userId }}, 'secret', {{ expiresIn: '7d' }});
  return {{ newToken }};
}}
```

Fix the function to:
1. Check if token was already used (throw Error if so)
2. Mark token as used before generating new one
3. Return the new token

Output ONLY the fixed `refreshToken` function, no explanation.""",
        "verify": lambda text: (
            sum([
                15 if re.search(r"isUsed|has\(token\)|usedTokens", text) else 0,
                15 if re.search(r"markUsed|add\(token\)|usedTokens\.add", text) else 0,
                10 if re.search(r"throw|Error|reject", text) else 0,
                10 if "newToken" in text and "return" in text else 0,
            ]),
            50
        ),
    },
    {
        "id": "MONO-3",
        "name": "Implement session deduplication",
        "category": "Race Condition Fix",
        "difficulty": "Hard",
        "prompt": """Write a JavaScript function `acquireSessionLock` that prevents duplicate session creation using a simple in-memory lock map.

Requirements:
- Takes userId and a lockMap (Map object)
- If userId already has a pending lock, wait for it (return existing promise)
- If no lock exists, create one and return it
- Lock should auto-release after the promise resolves
- Must handle concurrent calls for same userId

```javascript
// Usage:
// const lockMap = new Map();
// const release = await acquireSessionLock('user1', lockMap);
// try { /* create session */ } finally { release(); }
```

Output ONLY the function code, no explanation.""",
        "verify": lambda text: (
            sum([
                10 if "Map" in text or "lockMap" in text or "map" in text else 0,
                10 if re.search(r"(has|get)\(userId\)", text) else 0,
                10 if re.search(r"(set|Map).*Promise|promise|resolve", text, re.I) else 0,
                10 if re.search(r"(delete|remove|release|finally)", text, re.I) else 0,
                10 if "async" in text or "Promise" in text else 0,
            ]),
            50
        ),
    },
    {
        "id": "MONO-4",
        "name": "Write session invalidation tests",
        "category": "Test Writing",
        "difficulty": "Medium",
        "prompt": """Write Jest test cases for this session invalidation function:

```javascript
class SessionManager {
  constructor() { this.sessions = new Map(); }
  
  create(userId) {
    const id = 'sess_' + Math.random().toString(36).slice(2);
    this.sessions.set(id, { id, userId, active: true });
    return id;
  }
  
  invalidate(sessionId) {
    const s = this.sessions.get(sessionId);
    if (!s) return false;
    s.active = false;
    return true;
  }
  
  invalidateAll(userId) {
    let count = 0;
    for (const [id, s] of this.sessions) {
      if (s.userId === userId && s.active) { s.active = false; count++; }
    }
    return count;
  }
  
  getActive(userId) {
    return [...this.sessions.values()].filter(s => s.userId === userId && s.active);
  }
}
```

Write 5 test cases covering: create, invalidate single, invalidate non-existent, invalidateAll, getActive after invalidation. Use Jest describe/test/expect syntax. Output ONLY the test code.""",
        "verify": lambda text: (
            sum([
                8 if "describe" in text else 0,
                8 if text.count("test(") >= 4 or text.count("it(") >= 4 else 0,
                8 if "expect" in text else 0,
                8 if "invalidate" in text else 0,
                8 if re.search(r"(toBe|toEqual|toHaveLength|toBeTruthy|toBeFalsy)", text) else 0,
            ]),
            40
        ),
    },
    {
        "id": "MONO-5",
        "name": "Backward-compatible API migration",
        "category": "Refactoring",
        "difficulty": "Hard",
        "prompt": """Refactor this login endpoint to add session management WITHOUT changing the API response format:

CURRENT (must keep same response shape):
```javascript
async function login(username, password) {
  const user = await findUser(username);
  if (!user || user.password !== hashPassword(password)) {
    return { success: false, error: 'Invalid credentials' };
  }
  const token = jwt.sign({ userId: user.id }, SECRET, { expiresIn: '1h' });
  return { success: true, token, userId: user.id };
}
```

ADD these features while keeping the EXACT same response format:
1. Create a session on successful login
2. Include sessionId in the JWT payload (alongside userId)
3. Limit max 3 active sessions per user — if exceeded, invalidate oldest
4. Log session creation (console.log with format: `[auth] Session created: ${sessionId} for user ${userId}`)

Assume `sessionStore` is available with methods: createSession(userId), getActiveSessionsByUser(userId), invalidateSession(sessionId).

Output ONLY the refactored function.""",
        "verify": lambda text: (
            sum([
                10 if "createSession" in text else 0,
                10 if "sessionId" in text and "jwt.sign" in text.lower().replace(" ", "") or "sessionId" in text else 0,
                10 if re.search(r"(length|\.length\s*[>>=]|slice|splice|sort|shift)", text) else 0,
                10 if re.search(r"console\.log.*[Ss]ession", text) else 0,
                10 if "success" in text and "token" in text and "userId" in text else 0,
            ]),
            50
        ),
    },
]

def run_llm(endpoint, model, prompt, timeout=60):
    headers = {"Content-Type": "application/json", "Authorization": f"Bearer {endpoint['key']}"}
    body = {"model": model, "messages": [{"role": "user", "content": prompt}], "stream": False, "temperature": 0}
    start = time.time()
    try:
        resp = requests.post(endpoint["url"], json=body, headers=headers, timeout=timeout)
        latency = time.time() - start
        if resp.status_code != 200:
            return {"ok": False, "error": f"HTTP {resp.status_code}", "latency": latency}
        data = resp.json()
        # Get content - check both content and reasoning_content
        choice = data.get("choices", [{}])[0].get("message", {})
        content = choice.get("content", "")
        reasoning = choice.get("reasoning_content", "")
        usage = data.get("usage", {})
        return {"ok": True, "content": content, "reasoning": reasoning, "latency": latency,
                "input_tokens": usage.get("prompt_tokens", 0),
                "output_tokens": usage.get("completion_tokens", 0)}
    except requests.Timeout:
        return {"ok": False, "error": "TIMEOUT", "latency": time.time() - start}
    except Exception as e:
        return {"ok": False, "error": str(e)[:80], "latency": time.time() - start}

# === MAIN ===
print("╔═══════════════════════════════════════════════════════════════════════╗")
print("║     SWE-BENCH MONOREPO CHALLENGE v2                                 ║")
print("║     5 focused sub-tasks simulating real monorepo work               ║")
print("║     Lintasan (deepseek-v4-pro) vs 9Router (hemat)                   ║")
print("╚═══════════════════════════════════════════════════════════════════════╝")
print()
print(f"Tasks: {len(TASKS)} | Categories: Analysis, Security, Race Condition, Testing, Refactoring")
print("─" * 71)
print()

all_results = {"Lintasan": [], "9Router": []}

for i, task in enumerate(TASKS, 1):
    print(f"[{i}/{len(TASKS)}] {task['id']} | {task['name']} ({task['difficulty']})")
    print(f"         Category: {task['category']}")
    
    for endpoint, model, name in [(LINTASAN, "deepseek/deepseek-v4-pro", "Lintasan"), (NINER, "hemat", "9Router")]:
        r = run_llm(endpoint, model, task["prompt"])
        
        if not r["ok"]:
            all_results[name].append({"score": 0, "max": task["verify"]("")[1], "latency": r["latency"], "tokens": 0, "pass": False})
            print(f"  {name:>10}: ❌ {r['error']} | {r['latency']:.1f}s")
            continue
        
        content = r["content"]
        if not content and r.get("reasoning"):
            content = r["reasoning"]  # fallback to reasoning if content empty
        
        score, max_score = task["verify"](content)
        passed = score >= max_score * 0.6  # 60% threshold to pass
        
        all_results[name].append({
            "score": score, "max": max_score, "latency": r["latency"],
            "tokens": r["output_tokens"], "pass": passed
        })
        
        icon = "✅" if passed else "⚠️" if score > 0 else "❌"
        print(f"  {name:>10}: {icon} {score}/{max_score}pts | {r['latency']:.1f}s | {r['output_tokens']}tok")
    
    print()

# === FINAL REPORT ===
print("═" * 71)
print("  SWE-BENCH MONOREPO RESULTS")
print("═" * 71)
print()

print(f"{'Task':<40} {'Lintasan':>12} {'9Router':>12}")
print("─" * 66)

l_total = 0; n_total = 0; l_max = 0; n_max = 0
l_pass = 0; n_pass = 0

for i, task in enumerate(TASKS):
    lr = all_results["Lintasan"][i]
    nr = all_results["9Router"][i]
    l_total += lr["score"]; n_total += nr["score"]
    l_max += lr["max"]; n_max += nr["max"]
    if lr["pass"]: l_pass += 1
    if nr["pass"]: n_pass += 1
    
    l_str = f"{'✅' if lr['pass'] else '❌'} {lr['score']}/{lr['max']}"
    n_str = f"{'✅' if nr['pass'] else '❌'} {nr['score']}/{nr['max']}"
    print(f"  {task['id']} {task['name']:<34} {l_str:>12} {n_str:>12}")

print("─" * 66)
print(f"  {'TOTAL':<38} {l_total:>8}/{l_max} {n_total:>8}/{n_max}")
print()

# Metrics
l_times = [r["latency"] for r in all_results["Lintasan"] if r["score"] > 0]
n_times = [r["latency"] for r in all_results["9Router"] if r["score"] > 0]
l_tok = [r["tokens"] for r in all_results["Lintasan"] if r["score"] > 0]
n_tok = [r["tokens"] for r in all_results["9Router"] if r["score"] > 0]

print(f"{'Metric':<30} {'Lintasan':>15} {'9Router':>15}")
print("─" * 62)
print(f"{'Pass Rate (≥60%)':<30} {l_pass:>12}/{len(TASKS)} {n_pass:>12}/{len(TASKS)}")
print(f"{'Score %':<30} {l_total/l_max*100:>13.0f}% {n_total/n_max*100:>13.0f}%")
if l_times:
    print(f"{'Avg Latency':<30} {statistics.mean(l_times):>12.1f}s {statistics.mean(n_times) if n_times else 0:>12.1f}s")
if l_tok:
    print(f"{'Avg Output Tokens':<30} {statistics.mean(l_tok):>12.0f} {statistics.mean(n_tok) if n_tok else 0:>12.0f}")
print()

# Winner
print("═" * 71)
l_pct = l_total / l_max * 100
n_pct = n_total / n_max * 100
print(f"  FINAL: Lintasan {l_total}/{l_max} ({l_pct:.0f}%) vs 9Router {n_total}/{n_max} ({n_pct:.0f}%)")
if l_total > n_total:
    print(f"  🏆 WINNER: Lintasan (+{l_total-n_total}pts)")
elif n_total > l_total:
    print(f"  🏆 WINNER: 9Router (+{n_total-l_total}pts)")
else:
    print(f"  🤝 TIE")
print("═" * 71)

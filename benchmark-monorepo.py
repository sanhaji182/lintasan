#!/usr/bin/env python3
"""
SWE-Bench Monorepo Task — Real-world multi-step coding challenge
Simulates a large codebase with auth middleware, session management, race conditions
Both routers get the SAME task and their output is verified by running tests
"""
import json, time, requests, subprocess, tempfile, os, sys

# Config
LINTASAN = {"url": "http://127.0.0.1:20180/api/v1/chat/completions", "key": "sk-sans-test-key", "name": "Lintasan"}
NINER_KEY = os.environ.get("NINER_API_KEY", "your-9router-key-here")
NINER = {"url": "http://127.0.0.1:20128/api/v1/chat/completions", "key": NINER_KEY, "name": "9Router"}

# === MONOREPO SIMULATION ===
# Create a realistic codebase with bugs that need fixing

MONOREPO_FILES = {
    "src/middleware/auth.js": """// Authentication middleware
const jwt = require('jsonwebtoken');
const { SessionStore } = require('../services/session');
const { TokenService } = require('../services/token');

const JWT_SECRET = process.env.JWT_SECRET || 'dev-secret';

class AuthMiddleware {
  constructor() {
    this.sessionStore = new SessionStore();
    this.tokenService = new TokenService();
  }

  async authenticate(req, res, next) {
    const authHeader = req.headers.authorization;
    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return res.status(401).json({ error: 'Missing token' });
    }

    const token = authHeader.slice(7);
    
    try {
      const decoded = jwt.verify(token, JWT_SECRET);
      req.user = decoded;
      req.sessionId = decoded.sessionId;
      next();
    } catch (err) {
      if (err.name === 'TokenExpiredError') {
        return res.status(401).json({ error: 'Token expired', code: 'TOKEN_EXPIRED' });
      }
      return res.status(401).json({ error: 'Invalid token' });
    }
  }

  async refreshToken(req, res) {
    const { refreshToken } = req.body;
    if (!refreshToken) {
      return res.status(400).json({ error: 'Refresh token required' });
    }

    try {
      // BUG: No validation that refresh token belongs to the session
      const decoded = this.tokenService.verifyRefreshToken(refreshToken);
      
      // BUG: Race condition - multiple refresh requests can create duplicate sessions
      const session = await this.sessionStore.getSession(decoded.sessionId);
      if (!session) {
        return res.status(401).json({ error: 'Session not found' });
      }

      // Create new tokens
      const newAccessToken = this.tokenService.createAccessToken(decoded.userId, session.id);
      const newRefreshToken = this.tokenService.createRefreshToken(decoded.userId, session.id);
      
      // BUG: Old refresh token not invalidated (token reuse attack)
      await this.sessionStore.updateSession(session.id, { lastRefresh: Date.now() });

      return res.json({ accessToken: newAccessToken, refreshToken: newRefreshToken });
    } catch (err) {
      return res.status(401).json({ error: 'Invalid refresh token' });
    }
  }
}

module.exports = { AuthMiddleware };
""",
    "src/services/session.js": """// Session management service
class SessionStore {
  constructor() {
    this.sessions = new Map();
    this.userSessions = new Map(); // userId -> Set<sessionId>
  }

  async createSession(userId, metadata = {}) {
    // BUG: No check for existing active session - causes duplicates
    const sessionId = `sess_${Date.now()}_${Math.random().toString(36).slice(2)}`;
    const session = {
      id: sessionId,
      userId,
      createdAt: Date.now(),
      lastRefresh: Date.now(),
      isActive: true,
      metadata,
    };
    
    this.sessions.set(sessionId, session);
    
    if (!this.userSessions.has(userId)) {
      this.userSessions.set(userId, new Set());
    }
    this.userSessions.get(userId).add(sessionId);
    
    return session;
  }

  async getSession(sessionId) {
    return this.sessions.get(sessionId) || null;
  }

  async updateSession(sessionId, updates) {
    const session = this.sessions.get(sessionId);
    if (!session) return null;
    Object.assign(session, updates);
    return session;
  }

  async invalidateSession(sessionId) {
    const session = this.sessions.get(sessionId);
    if (!session) return false;
    session.isActive = false;
    return true;
  }

  async invalidateAllUserSessions(userId) {
    const sessionIds = this.userSessions.get(userId);
    if (!sessionIds) return 0;
    let count = 0;
    for (const id of sessionIds) {
      const session = this.sessions.get(id);
      if (session && session.isActive) {
        session.isActive = false;
        count++;
      }
    }
    return count;
  }

  async getActiveSessions(userId) {
    const sessionIds = this.userSessions.get(userId);
    if (!sessionIds) return [];
    return [...sessionIds]
      .map(id => this.sessions.get(id))
      .filter(s => s && s.isActive);
  }
}

module.exports = { SessionStore };
""",
    "src/services/token.js": """// Token service
const jwt = require('jsonwebtoken');

const JWT_SECRET = process.env.JWT_SECRET || 'dev-secret';
const REFRESH_SECRET = process.env.REFRESH_SECRET || 'refresh-dev-secret';

class TokenService {
  createAccessToken(userId, sessionId) {
    return jwt.sign(
      { userId, sessionId, type: 'access' },
      JWT_SECRET,
      { expiresIn: '15m' }
    );
  }

  createRefreshToken(userId, sessionId) {
    return jwt.sign(
      { userId, sessionId, type: 'refresh' },
      REFRESH_SECRET,
      { expiresIn: '7d' }
    );
  }

  verifyRefreshToken(token) {
    return jwt.verify(token, REFRESH_SECRET);
  }

  verifyAccessToken(token) {
    return jwt.verify(token, JWT_SECRET);
  }
}

module.exports = { TokenService };
""",
    "src/services/session-invalidation.js": """// Session invalidation handlers
const { SessionStore } = require('./session');

class SessionInvalidationService {
  constructor(sessionStore) {
    this.sessionStore = sessionStore;
    this.logger = console;
  }

  async logoutUser(userId) {
    this.logger.log(`[session] Logging out user ${userId}`);
    const count = await this.sessionStore.invalidateAllUserSessions(userId);
    this.logger.log(`[session] Invalidated ${count} sessions for user ${userId}`);
    return { invalidated: count };
  }

  async logoutSession(sessionId) {
    this.logger.log(`[session] Invalidating session ${sessionId}`);
    const result = await this.sessionStore.invalidateSession(sessionId);
    return { success: result };
  }

  async cleanupExpiredSessions(maxAgeMs = 7 * 24 * 60 * 60 * 1000) {
    // Not implemented yet - placeholder
    return { cleaned: 0 };
  }
}

module.exports = { SessionInvalidationService };
""",
    "tests/auth.test.js": """// Auth middleware tests
const { AuthMiddleware } = require('../src/middleware/auth');
const { SessionStore } = require('../src/services/session');
const { TokenService } = require('../src/services/token');

describe('AuthMiddleware', () => {
  let auth, sessionStore, tokenService;

  beforeEach(() => {
    auth = new AuthMiddleware();
    sessionStore = auth.sessionStore;
    tokenService = auth.tokenService;
  });

  test('should reject missing token', async () => {
    const req = { headers: {} };
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn() };
    await auth.authenticate(req, res, jest.fn());
    expect(res.status).toHaveBeenCalledWith(401);
  });

  test('should validate access token', async () => {
    const session = await sessionStore.createSession('user1');
    const token = tokenService.createAccessToken('user1', session.id);
    const req = { headers: { authorization: `Bearer ${token}` } };
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn() };
    const next = jest.fn();
    await auth.authenticate(req, res, next);
    expect(next).toHaveBeenCalled();
    expect(req.user.userId).toBe('user1');
  });

  test('should refresh token and invalidate old one', async () => {
    const session = await sessionStore.createSession('user1');
    const refreshToken = tokenService.createRefreshToken('user1', session.id);
    const req = { body: { refreshToken } };
    const res = { status: jest.fn().mockReturnThis(), json: jest.fn() };
    
    await auth.refreshToken(req, res);
    expect(res.json).toHaveBeenCalled();
    const response = res.json.mock.calls[0][0];
    expect(response.accessToken).toBeDefined();
    expect(response.refreshToken).toBeDefined();
    
    // Old refresh token should be invalidated (currently fails - this is the bug)
    // After fix: reusing old refresh token should fail
  });

  test('should prevent duplicate sessions on concurrent refresh', async () => {
    const session = await sessionStore.createSession('user1');
    const refreshToken = tokenService.createRefreshToken('user1', session.id);
    
    // Simulate concurrent refresh requests
    const req1 = { body: { refreshToken } };
    const req2 = { body: { refreshToken } };
    const res1 = { status: jest.fn().mockReturnThis(), json: jest.fn() };
    const res2 = { status: jest.fn().mockReturnThis(), json: jest.fn() };
    
    await Promise.all([
      auth.refreshToken(req1, res1),
      auth.refreshToken(req2, res2),
    ]);
    
    // Should not create duplicate sessions
    const activeSessions = await sessionStore.getActiveSessions('user1');
    expect(activeSessions.length).toBe(1);
  });
});
""",
}

# The PROMPT given to both LLMs
MONOREPO_PROMPT = """You are working in a Node.js monorepo. Below are the relevant files.

## Files

### src/middleware/auth.js
```javascript
""" + MONOREPO_FILES["src/middleware/auth.js"] + """```

### src/services/session.js
```javascript
""" + MONOREPO_FILES["src/services/session.js"] + """```

### src/services/token.js
```javascript
""" + MONOREPO_FILES["src/services/token.js"] + """```

### src/services/session-invalidation.js
```javascript
""" + MONOREPO_FILES["src/services/session-invalidation.js"] + """```

### tests/auth.test.js
```javascript
""" + MONOREPO_FILES["tests/auth.test.js"] + """```

## Tasks

1. Find the authentication middleware and explain how refresh tokens are validated.
2. Identify the race condition causing duplicate session creation during concurrent token refresh.
3. Fix the race condition in the refresh token flow.
4. Invalidate old refresh tokens after refresh (prevent token reuse attacks).
5. Update the session store to prevent duplicate active sessions per user.
6. Ensure backward compatibility — keep API responses unchanged, preserve logging behavior.

## Requirements
- Use minimal edits (only change what's necessary)
- Do not rewrite unrelated files
- Keep API response format unchanged
- Preserve existing logging behavior

## Output Format
For each file you modify, output the COMPLETE updated file content in a code block with the filename as header. Only include files you actually changed. Format:

### filename.js
```javascript
// complete file content
```

Also provide a brief explanation of each fix."""

# === Python-based verification (since we can't run Jest easily) ===
VERIFY_SCRIPT = '''
import re, sys

def check_solution(solution_text):
    """Verify the LLM's solution addresses all requirements"""
    score = 0
    max_score = 100
    details = []
    
    # 1. Did they modify auth.js? (required for race condition fix)
    if "auth.js" in solution_text or "AuthMiddleware" in solution_text:
        score += 10
        details.append("+ Found auth middleware modifications")
    else:
        details.append("- Missing auth middleware fix")
    
    # 2. Race condition fix: mutex/lock/atomic check in refreshToken
    race_patterns = [
        r"(lock|mutex|semaphore|atomic|synchronized)",
        r"(refreshing|isRefreshing|refreshLock|pendingRefresh)",
        r"(Map|Set).*refresh",
        r"await.*lock",
    ]
    race_fixed = any(re.search(p, solution_text, re.IGNORECASE) for p in race_patterns)
    if race_fixed:
        score += 20
        details.append("+ Race condition addressed (lock/mutex pattern found)")
    else:
        details.append("- No clear race condition fix (no lock/mutex pattern)")
    
    # 3. Old refresh token invalidation
    invalidation_patterns = [
        r"(invalidate|revoke|blacklist|used).*refresh",
        r"refresh.*(invalidat|revok|blacklist|used)",
        r"(usedTokens|revokedTokens|tokenBlacklist|refreshTokens)",
        r"delete.*refresh|remove.*refresh",
    ]
    token_invalidated = any(re.search(p, solution_text, re.IGNORECASE) for p in invalidation_patterns)
    if token_invalidated:
        score += 20
        details.append("+ Old refresh token invalidation implemented")
    else:
        details.append("- Missing refresh token invalidation")
    
    # 4. Duplicate session prevention
    dedup_patterns = [
        r"(getActiveSessions|existingSession|activeSession)",
        r"(findSession|checkSession|hasActiveSession)",
        r"activeSessions.*length|sessions.*filter",
    ]
    dedup_fixed = any(re.search(p, solution_text, re.IGNORECASE) for p in dedup_patterns)
    if dedup_fixed:
        score += 15
        details.append("+ Duplicate session prevention found")
    else:
        details.append("- No duplicate session prevention")
    
    # 5. Session store modified
    if "SessionStore" in solution_text and ("createSession" in solution_text or "session.js" in solution_text):
        score += 10
        details.append("+ Session store modifications present")
    else:
        details.append("- Session store not modified")
    
    # 6. Backward compatibility - API response format preserved
    if "accessToken" in solution_text and "refreshToken" in solution_text:
        score += 10
        details.append("+ API response format preserved")
    else:
        details.append("- API response format may be broken")
    
    # 7. Explanation provided
    explanation_patterns = [
        r"(race condition|concurrent|simultaneous)",
        r"(fix|solution|approach|change)",
    ]
    has_explanation = sum(1 for p in explanation_patterns if re.search(p, solution_text, re.IGNORECASE))
    if has_explanation >= 2:
        score += 10
        details.append("+ Clear explanation provided")
    elif has_explanation >= 1:
        score += 5
        details.append("~ Partial explanation")
    else:
        details.append("- No explanation")
    
    # 8. Minimal edits (penalize if they rewrote everything)
    file_count = solution_text.count("```javascript") + solution_text.count("```js")
    if 1 <= file_count <= 3:
        score += 5
        details.append(f"+ Minimal edits ({file_count} files changed)")
    elif file_count == 4:
        score += 3
        details.append(f"~ Moderate edits ({file_count} files changed)")
    elif file_count >= 5:
        details.append(f"- Too many files changed ({file_count})")
    
    return score, details

# Read solution from stdin
solution = sys.stdin.read()
score, details = check_solution(solution)
print(f"SCORE: {score}/100")
for d in details:
    print(f"  {d}")
'''

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
        content = data.get("choices", [{}])[0].get("message", {}).get("content", "")
        usage = data.get("usage", {})
        return {"ok": True, "content": content, "latency": latency,
                "input_tokens": usage.get("prompt_tokens", 0),
                "output_tokens": usage.get("completion_tokens", 0)}
    except requests.Timeout:
        return {"ok": False, "error": "TIMEOUT", "latency": time.time() - start}
    except Exception as e:
        return {"ok": False, "error": str(e)[:80], "latency": time.time() - start}

def verify_solution(content):
    """Run verification script on solution"""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.py', delete=False) as f:
        f.write(VERIFY_SCRIPT)
        verify_path = f.name
    
    try:
        result = subprocess.run(
            ["python3", verify_path],
            input=content, capture_output=True, text=True, timeout=10
        )
        os.unlink(verify_path)
        return result.stdout.strip()
    except Exception as e:
        os.unlink(verify_path)
        return f"ERROR: {e}"

# === MAIN ===
print("╔═══════════════════════════════════════════════════════════════════════╗")
print("║     SWE-BENCH MONOREPO CHALLENGE                                    ║")
print("║     Real-world multi-file bug fix + feature implementation           ║")
print("║     Lintasan vs 9Router                                              ║")
print("╚═══════════════════════════════════════════════════════════════════════╝")
print()
print("Task: Fix auth race condition + token reuse + duplicate sessions")
print("Files: 5 (middleware, services, tests)")
print("Scoring: Code quality verification (pattern matching + structure)")
print("─" * 71)
print()

results = {}

for endpoint, model, name in [(LINTASAN, "deepseek/deepseek-v4-pro", "Lintasan"), (NINER, "hemat", "9Router")]:
    print(f"▶ Running {name}...")
    sys.stdout.flush()
    
    r = run_llm(endpoint, model, MONOREPO_PROMPT, timeout=90)
    
    if not r["ok"]:
        print(f"  ❌ FAILED: {r['error']}")
        results[name] = {"score": 0, "latency": r["latency"], "error": r["error"]}
        continue
    
    # Verify solution
    verification = verify_solution(r["content"])
    
    # Extract score
    score_line = [l for l in verification.split("\n") if l.startswith("SCORE:")]
    score = int(score_line[0].split("/")[0].replace("SCORE: ", "")) if score_line else 0
    
    results[name] = {
        "score": score,
        "latency": r["latency"],
        "input_tokens": r["input_tokens"],
        "output_tokens": r["output_tokens"],
        "content_length": len(r["content"]),
    }
    
    print(f"  ⏱️  Latency: {r['latency']:.1f}s")
    print(f"  📊 Tokens: {r['input_tokens']} in / {r['output_tokens']} out")
    print(f"  📝 Response: {len(r['content'])} chars")
    print(f"  🔍 Verification:")
    for line in verification.split("\n"):
        print(f"     {line}")
    print()

# === COMPARISON ===
print("═" * 71)
print("  MONOREPO CHALLENGE RESULTS")
print("═" * 71)
print()
print(f"{'Metric':<30} {'Lintasan':>18} {'9Router':>18}")
print("─" * 68)

l = results.get("Lintasan", {})
n = results.get("9Router", {})

print(f"{'SWE Score':<30} {l.get('score',0):>15}/100 {n.get('score',0):>15}/100")
print(f"{'Latency':<30} {l.get('latency',0):>16.1f}s {n.get('latency',0):>16.1f}s")
print(f"{'Output Tokens':<30} {l.get('output_tokens',0):>18} {n.get('output_tokens',0):>18}")
print(f"{'Response Length':<30} {l.get('content_length',0):>14} chr {n.get('content_length',0):>14} chr")
print()

ls = l.get('score', 0)
ns = n.get('score', 0)
print("═" * 71)
if ls > ns:
    print(f"  🏆 WINNER: Lintasan ({ls}/100 vs {ns}/100, +{ls-ns}pts)")
elif ns > ls:
    print(f"  🏆 WINNER: 9Router ({ns}/100 vs {ls}/100, +{ns-ls}pts)")
else:
    print(f"  🤝 TIE ({ls}/100 vs {ns}/100)")
print("═" * 71)

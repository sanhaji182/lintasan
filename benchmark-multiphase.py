#!/usr/bin/env python3
"""
SWE-Bench Multi-Phase Monorepo Challenge
Tests: phased reasoning, token efficiency, instruction following
"""
import json, time, requests, re, sys, statistics

LINTASAN = {"url": "http://127.0.0.1:20180/api/v1/chat/completions", "key": "sk-sans-test-key", "name": "Lintasan"}
NINER_KEY = os.environ.get("NINER_API_KEY", "your-9router-key-here")
NINER = {"url": "http://127.0.0.1:20128/api/v1/chat/completions", "key": NINER_KEY, "name": "9Router"}

# Monorepo structure for context
MONOREPO_CONTEXT = """
Project structure (partial):
```
/repo
├── packages/
│   ├── auth/
│   │   ├── src/
│   │   │   ├── middleware/
│   │   │   │   ├── authenticate.ts
│   │   │   │   ├── rateLimit.ts
│   │   │   │   └── cors.ts
│   │   │   ├── services/
│   │   │   │   ├── tokenService.ts
│   │   │   │   ├── sessionService.ts
│   │   │   │   └── refreshService.ts
│   │   │   ├── models/
│   │   │   │   ├── Session.ts
│   │   │   │   └── RefreshToken.ts
│   │   │   └── index.ts
│   │   └── tests/
│   │       ├── auth.test.ts
│   │       ├── session.test.ts
│   │       └── refresh.test.ts
│   ├── api-gateway/
│   │   └── src/routes/auth.ts
│   ├── user-service/
│   │   └── src/handlers/login.ts
│   └── shared/
│       └── src/types/auth.ts
├── libs/
│   ├── redis-client/
│   └── postgres-client/
└── docker-compose.yml
```

Key file contents:

### packages/auth/src/services/refreshService.ts
```typescript
import { Redis } from '@libs/redis-client';
import { TokenService } from './tokenService';
import { SessionService } from './sessionService';

export class RefreshService {
  constructor(
    private redis: Redis,
    private tokenService: TokenService,
    private sessionService: SessionService
  ) {}

  async refresh(refreshToken: string): Promise<TokenPair> {
    // Step 1: Verify token
    const payload = this.tokenService.verifyRefresh(refreshToken);
    
    // Step 2: Check session exists
    const session = await this.sessionService.getById(payload.sessionId);
    if (!session || !session.isActive) {
      throw new AuthError('SESSION_INVALID');
    }

    // Step 3: Check token not revoked
    const isRevoked = await this.redis.sismember('revoked_tokens', refreshToken);
    if (isRevoked) {
      // Potential token reuse - invalidate all sessions
      await this.sessionService.invalidateAllForUser(payload.userId);
      throw new AuthError('TOKEN_REUSE_DETECTED');
    }

    // BUG: Race condition between check (step 3) and revoke (step 4)
    // Two concurrent requests can both pass step 3 before either reaches step 4

    // Step 4: Revoke old token
    await this.redis.sadd('revoked_tokens', refreshToken);

    // Step 5: Generate new pair
    const newTokens = this.tokenService.generatePair(payload.userId, session.id);
    
    // Step 6: Update session
    await this.sessionService.touch(session.id);

    return newTokens;
  }
}
```

### packages/auth/src/services/sessionService.ts
```typescript
import { Pool } from '@libs/postgres-client';

export class SessionService {
  constructor(private db: Pool) {}

  async create(userId: string, metadata: SessionMeta): Promise<Session> {
    // BUG: No uniqueness check - concurrent logins create duplicate sessions
    const result = await this.db.query(
      'INSERT INTO sessions (user_id, metadata, is_active, created_at) VALUES ($1, $2, true, NOW()) RETURNING *',
      [userId, JSON.stringify(metadata)]
    );
    return result.rows[0];
  }

  async getById(id: string): Promise<Session | null> {
    const result = await this.db.query('SELECT * FROM sessions WHERE id = $1', [id]);
    return result.rows[0] || null;
  }

  async getActiveForUser(userId: string): Promise<Session[]> {
    const result = await this.db.query(
      'SELECT * FROM sessions WHERE user_id = $1 AND is_active = true ORDER BY created_at DESC',
      [userId]
    );
    return result.rows;
  }

  async invalidateAllForUser(userId: string): Promise<number> {
    const result = await this.db.query(
      'UPDATE sessions SET is_active = false WHERE user_id = $1 AND is_active = true',
      [userId]
    );
    return result.rowCount;
  }

  async touch(id: string): Promise<void> {
    await this.db.query('UPDATE sessions SET last_active = NOW() WHERE id = $1', [id]);
  }
}
```
"""

PHASES = [
    {
        "id": "Phase-1",
        "name": "Identify auth files & dependency map",
        "prompt": MONOREPO_CONTEXT + """

## Phase 1 Task
Identify only the authentication-related directories and files from the structure above.
Return a concise dependency map showing how auth components depend on each other.

Rules:
- Keep response under 200 words
- Use Bahasa Indonesia for explanations
- Only list auth-relevant files
- Show dependency arrows (A → B means A depends on B)""",
        "verify": lambda text: (
            sum([
                10 if "refreshService" in text.lower() or "refresh" in text.lower() else 0,
                10 if "sessionService" in text.lower() or "session" in text.lower() else 0,
                10 if "tokenService" in text.lower() or "token" in text.lower() else 0,
                5 if "redis" in text.lower() else 0,
                5 if "postgres" in text.lower() or "db" in text.lower() else 0,
                5 if "→" in text or "->" in text or "depend" in text.lower() else 0,
                5 if len(text.split()) <= 250 else 0,  # conciseness bonus
            ]),
            50
        ),
    },
    {
        "id": "Phase-2",
        "name": "Trace refresh token flow",
        "prompt": MONOREPO_CONTEXT + """

## Phase 2 Task
Trace the refresh token creation and invalidation flow in refreshService.ts.
Explain step by step what happens when a refresh request comes in.
Do NOT modify code yet.

Rules:
- Keep response under 150 words
- Use Bahasa Indonesia
- Number each step
- Highlight where the vulnerability is""",
        "verify": lambda text: (
            sum([
                10 if re.search(r"(verify|validasi|verifikasi)", text, re.I) else 0,
                10 if re.search(r"(session|sesi)", text, re.I) else 0,
                10 if re.search(r"(revok|revoke|cek.*revok)", text, re.I) else 0,
                10 if re.search(r"(race|concurrent|bersamaan|paralel)", text, re.I) else 0,
                5 if re.search(r"(step|langkah|1\.|2\.|3\.)", text, re.I) else 0,
                5 if len(text.split()) <= 200 else 0,
            ]),
            50
        ),
    },
    {
        "id": "Phase-3",
        "name": "Identify race condition source",
        "prompt": MONOREPO_CONTEXT + """

## Phase 3 Task
Identify the MOST LIKELY race condition source in the refresh flow.
Explain:
1. What two operations race against each other?
2. What's the time window of vulnerability?
3. What's the impact?

Rules:
- Maximum 100 words
- Use Bahasa Indonesia
- Be specific about line/step numbers""",
        "verify": lambda text: (
            sum([
                15 if re.search(r"(step 3|langkah 3|check|cek|sismember)", text, re.I) else 0,
                15 if re.search(r"(step 4|langkah 4|sadd|revoke|revok)", text, re.I) else 0,
                10 if re.search(r"(concurrent|bersamaan|dua request|two request|paralel)", text, re.I) else 0,
                10 if re.search(r"(duplicate|duplikat|token.*reuse|replay)", text, re.I) else 0,
            ]),
            50
        ),
    },
    {
        "id": "Phase-4",
        "name": "Implement minimal fix",
        "prompt": MONOREPO_CONTEXT + """

## Phase 4 Task
Implement the SMALLEST possible fix for the race condition in refreshService.ts.
The fix should use Redis atomic operations (SETNX or similar) to make the check-and-revoke atomic.

Rules:
- Output ONLY the changed method/lines, not the full file
- Use a Redis lock or SETNX pattern
- Keep the fix under 15 lines of code
- Use Bahasa Indonesia for any explanation (max 2 sentences)""",
        "verify": lambda text: (
            sum([
                15 if re.search(r"(setnx|SET.*NX|setNX|sismember.*sadd|atomic)", text, re.I) else 0,
                10 if re.search(r"(lock|kunci|mutex)", text, re.I) else 0,
                10 if re.search(r"(await|async)", text) else 0,
                10 if "redis" in text.lower() else 0,
                5 if re.search(r"```", text) else 0,  # has code block
            ]),
            50
        ),
    },
    {
        "id": "Phase-5",
        "name": "Targeted test commands",
        "prompt": MONOREPO_CONTEXT + """

## Phase 5 Task
What commands would you run to test ONLY the authentication-related changes?
Assume the monorepo uses:
- pnpm workspaces
- vitest for testing
- packages/auth/tests/ contains the test files

Rules:
- List exact commands (max 3 commands)
- Use Bahasa Indonesia for explanation (max 1 sentence per command)
- Do NOT suggest running all tests""",
        "verify": lambda text: (
            sum([
                15 if re.search(r"(pnpm|npx).*vitest|vitest.*auth", text, re.I) else 0,
                10 if re.search(r"(packages/auth|--filter.*auth)", text) else 0,
                10 if re.search(r"(refresh|session)", text, re.I) else 0,
                5 if text.count("```") >= 2 or re.search(r"^\s*\$|^\s*pnpm|^\s*npx", text, re.M) else 0,
                5 if len(text.split()) <= 150 else 0,
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
        choice = data.get("choices", [{}])[0].get("message", {})
        content = choice.get("content", "")
        if not content:
            content = choice.get("reasoning_content", "")
        usage = data.get("usage", {})
        return {"ok": True, "content": content, "latency": latency,
                "output_tokens": usage.get("completion_tokens", 0),
                "finish_reason": choice.get("finish_reason", "N/A")}
    except requests.Timeout:
        return {"ok": False, "error": "TIMEOUT", "latency": time.time() - start}
    except Exception as e:
        return {"ok": False, "error": str(e)[:80], "latency": time.time() - start}

# === MAIN ===
print("╔═══════════════════════════════════════════════════════════════════════╗")
print("║     SWE-BENCH MULTI-PHASE MONOREPO CHALLENGE                        ║")
print("║     5 Phases: Identify → Trace → Diagnose → Fix → Test              ║")
print("║     Scoring: Correctness + Conciseness + Instruction Following       ║")
print("╚═══════════════════════════════════════════════════════════════════════╝")
print()
print("Model: deepseek-v4-pro | Rules: Bahasa Indonesia, minimal tokens")
print("─" * 71)
print()

all_results = {"Lintasan": [], "9Router": []}

for i, phase in enumerate(PHASES, 1):
    print(f"[{i}/{len(PHASES)}] {phase['id']} | {phase['name']}")
    
    for endpoint, model, name in [(LINTASAN, "deepseek/deepseek-v4-pro", "Lintasan"), (NINER, "hemat", "9Router")]:
        r = run_llm(endpoint, model, phase["prompt"], timeout=90)
        
        if not r["ok"]:
            all_results[name].append({"score": 0, "max": 50, "latency": r["latency"], "tokens": 0})
            print(f"  {name:>10}: ❌ {r['error']} | {r['latency']:.1f}s")
            continue
        
        score, max_score = phase["verify"](r["content"])
        passed = score >= max_score * 0.6
        
        # Token efficiency bonus (max 10pts)
        tok = r["output_tokens"]
        if tok <= 200: eff_bonus = 10
        elif tok <= 400: eff_bonus = 7
        elif tok <= 600: eff_bonus = 4
        elif tok <= 1000: eff_bonus = 2
        else: eff_bonus = 0
        
        total = score + eff_bonus
        total_max = max_score + 10
        
        all_results[name].append({
            "score": total, "max": total_max, "latency": r["latency"],
            "tokens": tok, "pass": passed, "finish": r["finish_reason"]
        })
        
        icon = "✅" if passed else "⚠️" if score > 0 else "❌"
        print(f"  {name:>10}: {icon} {total}/{total_max}pts | {r['latency']:.1f}s | {tok}tok | fin:{r['finish_reason']}")
    
    print()

# === FINAL REPORT ===
print("═" * 71)
print("  MULTI-PHASE MONOREPO RESULTS")
print("═" * 71)
print()

print(f"{'Phase':<42} {'Lintasan':>12} {'9Router':>12}")
print("─" * 68)

l_total = 0; n_total = 0; l_max = 0; n_max = 0
l_pass = 0; n_pass = 0

for i, phase in enumerate(PHASES):
    lr = all_results["Lintasan"][i] if i < len(all_results["Lintasan"]) else {"score": 0, "max": 60, "pass": False}
    nr = all_results["9Router"][i] if i < len(all_results["9Router"]) else {"score": 0, "max": 60, "pass": False}
    l_total += lr["score"]; n_total += nr["score"]
    l_max += lr["max"]; n_max += nr["max"]
    if lr.get("pass"): l_pass += 1
    if nr.get("pass"): n_pass += 1
    
    l_str = f"{'✅' if lr.get('pass') else '❌'} {lr['score']}/{lr['max']}"
    n_str = f"{'✅' if nr.get('pass') else '❌'} {nr['score']}/{nr['max']}"
    print(f"  {phase['id']} {phase['name']:<36} {l_str:>12} {n_str:>12}")

print("─" * 68)
print(f"  {'TOTAL':<40} {l_total:>8}/{l_max} {n_total:>8}/{n_max}")
print()

l_times = [r["latency"] for r in all_results["Lintasan"] if r.get("score", 0) > 0]
n_times = [r["latency"] for r in all_results["9Router"] if r.get("score", 0) > 0]
l_tok = [r["tokens"] for r in all_results["Lintasan"] if r.get("score", 0) > 0]
n_tok = [r["tokens"] for r in all_results["9Router"] if r.get("score", 0) > 0]

print(f"{'Metric':<30} {'Lintasan':>15} {'9Router':>15}")
print("─" * 62)
print(f"{'Pass Rate (≥60%)':<30} {l_pass:>12}/{len(PHASES)} {n_pass:>12}/{len(PHASES)}")
print(f"{'Score %':<30} {l_total/l_max*100 if l_max else 0:>13.0f}% {n_total/n_max*100 if n_max else 0:>13.0f}%")
if l_times:
    print(f"{'Avg Latency':<30} {statistics.mean(l_times):>12.1f}s {statistics.mean(n_times) if n_times else 0:>12.1f}s")
if l_tok:
    print(f"{'Avg Output Tokens':<30} {statistics.mean(l_tok):>12.0f} {statistics.mean(n_tok) if n_tok else 0:>12.0f}")
    print(f"{'Total Tokens Used':<30} {sum(l_tok):>12} {sum(n_tok) if n_tok else 0:>12}")
print()

print("═" * 71)
l_pct = l_total / l_max * 100 if l_max else 0
n_pct = n_total / n_max * 100 if n_max else 0
if l_total > n_total:
    print(f"  🏆 WINNER: Lintasan ({l_total}/{l_max} = {l_pct:.0f}%) vs 9Router ({n_total}/{n_max} = {n_pct:.0f}%)")
elif n_total > l_total:
    print(f"  🏆 WINNER: 9Router ({n_total}/{n_max} = {n_pct:.0f}%) vs Lintasan ({l_total}/{l_max} = {l_pct:.0f}%)")
else:
    print(f"  🤝 TIE ({l_total}/{l_max} = {l_pct:.0f}%)")
print("═" * 71)

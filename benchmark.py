#!/usr/bin/env python3
"""
Comprehensive LLM Router Benchmark Suite v2.0
Inspired by: SWE-bench, OpenHands, Aider, Terminal-Bench
Tests real-world coding ability across 4 dimensions
"""
import json, time, requests, subprocess, tempfile, os, sys, statistics

# Config
LINTASAN = {"url": "http://127.0.0.1:20180/api/v1/chat/completions", "key": "sk-sans-test-key", "name": "Lintasan"}
NINER_KEY = os.environ.get("NINER_API_KEY", "your-9router-key-here")
NINER = {"url": "http://127.0.0.1:20128/api/v1/chat/completions", "key": NINER_KEY, "name": "9Router"}

# ═══════════════════════════════════════════════════════════════
# BENCHMARK 1: SWE-bench Style (Fix real bugs in existing code)
# ═══════════════════════════════════════════════════════════════
SWE_TASKS = [
    {
        "id": "SWE-1", "name": "Fix pagination off-by-one",
        "difficulty": "Medium", "category": "Bug Fix",
        "prompt": """Fix this buggy pagination function. It should return correct page of items:

```python
def paginate(items, page, per_page):
    start = page * per_page
    end = start + per_page
    total_pages = len(items) // per_page
    return {
        'data': items[start:end],
        'page': page,
        'total_pages': total_pages,
        'has_next': page < total_pages
    }
```

Bugs: 1) page is 1-indexed but code treats as 0-indexed, 2) total_pages wrong when items don't divide evenly, 3) has_next logic wrong.
Output only the corrected function.""",
        "test_code": """
from solution import paginate
r = paginate([1,2,3,4,5,6,7], 1, 3)
assert r['data'] == [1,2,3], f"Got {r['data']}"
assert r['page'] == 1
assert r['total_pages'] == 3
assert r['has_next'] == True
r2 = paginate([1,2,3,4,5,6,7], 3, 3)
assert r2['data'] == [7], f"Got {r2['data']}"
assert r2['has_next'] == False
r3 = paginate([1,2,3,4,5,6], 2, 3)
assert r3['data'] == [4,5,6]
assert r3['total_pages'] == 2
print("PASS")
""",
    },
    {
        "id": "SWE-2", "name": "Fix race condition in counter",
        "difficulty": "Hard", "category": "Bug Fix",
        "prompt": """Fix this thread-safe counter class. The increment should be atomic:

```python
import threading

class SafeCounter:
    def __init__(self):
        self.count = 0
    
    def increment(self):
        current = self.count
        self.count = current + 1
    
    def get(self):
        return self.count
```

The bug: increment is not atomic (read-modify-write without lock). Fix it using threading.Lock. Output only the corrected class with import.""",
        "test_code": """
import threading
from solution import SafeCounter
c = SafeCounter()
threads = []
for _ in range(100):
    t = threading.Thread(target=lambda: [c.increment() for _ in range(100)])
    threads.append(t)
    t.start()
for t in threads:
    t.join()
assert c.get() == 10000, f"Expected 10000, got {c.get()}"
print("PASS")
""",
    },
    {
        "id": "SWE-3", "name": "Fix memory leak in cache",
        "difficulty": "Hard", "category": "Bug Fix",
        "prompt": """Fix this cache that has unbounded growth (memory leak). Add max_size with LRU eviction:

```python
class Cache:
    def __init__(self):
        self.store = {}
    
    def get(self, key):
        return self.store.get(key)
    
    def set(self, key, value):
        self.store[key] = value
```

Fix: Add max_size parameter (default 100). When full, evict least recently accessed key. Track access order. Output only the corrected class.""",
        "test_code": """
from solution import Cache
c = Cache(max_size=3)
c.set("a", 1)
c.set("b", 2)
c.set("c", 3)
assert c.get("a") == 1
c.set("d", 4)  # should evict "b" (least recently used, "a" was accessed by get)
assert c.get("b") is None, f"b should be evicted, got {c.get('b')}"
assert c.get("a") == 1
assert c.get("c") == 3
assert c.get("d") == 4
print("PASS")
""",
    },
]

# ═══════════════════════════════════════════════════════════════
# BENCHMARK 2: OpenHands Style (Multi-step task execution)
# ═══════════════════════════════════════════════════════════════
OPENHANDS_TASKS = [
    {
        "id": "OH-1", "name": "Build REST API handler",
        "difficulty": "Medium", "category": "Full Implementation",
        "prompt": """Write a Python class SimpleAPI that simulates a REST API for a todo list:
- __init__(self): initialize empty todo list
- add(self, title) -> dict: add todo, return {"id": int, "title": str, "done": False}
- get(self, id) -> dict or None: get todo by id
- complete(self, id) -> bool: mark as done, return True if found
- list_all(self) -> list: return all todos
- delete(self, id) -> bool: delete todo, return True if found

IDs should auto-increment starting from 1. Output only the class.""",
        "test_code": """
from solution import SimpleAPI
api = SimpleAPI()
t1 = api.add("Buy milk")
assert t1 == {"id": 1, "title": "Buy milk", "done": False}
t2 = api.add("Write code")
assert t2["id"] == 2
assert api.get(1) == {"id": 1, "title": "Buy milk", "done": False}
assert api.get(99) is None
assert api.complete(1) == True
assert api.get(1)["done"] == True
assert api.complete(99) == False
assert len(api.list_all()) == 2
assert api.delete(1) == True
assert len(api.list_all()) == 1
assert api.delete(1) == False
print("PASS")
""",
    },
    {
        "id": "OH-2", "name": "Implement event emitter",
        "difficulty": "Medium", "category": "Full Implementation",
        "prompt": """Write a Python class EventEmitter with:
- on(event, callback): register callback for event
- off(event, callback): remove specific callback
- emit(event, *args): call all callbacks for event with args
- once(event, callback): register callback that fires only once then auto-removes

Output only the class.""",
        "test_code": """
from solution import EventEmitter
ee = EventEmitter()
results = []
def handler1(x): results.append(f"h1:{x}")
def handler2(x): results.append(f"h2:{x}")

ee.on("data", handler1)
ee.on("data", handler2)
ee.emit("data", "hello")
assert results == ["h1:hello", "h2:hello"], f"Got {results}"

ee.off("data", handler1)
results.clear()
ee.emit("data", "world")
assert results == ["h2:world"]

# Test once
results.clear()
ee.once("ping", lambda x: results.append(f"once:{x}"))
ee.emit("ping", "1")
ee.emit("ping", "2")
assert results == ["once:1"], f"Got {results}"
print("PASS")
""",
    },
    {
        "id": "OH-3", "name": "Build middleware pipeline",
        "difficulty": "Hard", "category": "Full Implementation",
        "prompt": """Write a Python class Pipeline that chains middleware functions:
- use(middleware): add middleware function. Middleware signature: fn(context, next) where next() calls the next middleware
- execute(context): run all middleware in order, return final context

Each middleware can modify context (a dict) and must call next() to continue the chain. If next() is not called, chain stops.

Output only the class.""",
        "test_code": """
from solution import Pipeline
p = Pipeline()

def add_timestamp(ctx, next):
    ctx['timestamp'] = '2026-01-01'
    next()

def add_user(ctx, next):
    ctx['user'] = 'admin'
    next()

def validate(ctx, next):
    if 'user' in ctx:
        ctx['valid'] = True
        next()
    # else: don't call next, stops chain

p.use(add_timestamp)
p.use(add_user)
p.use(validate)

ctx = {}
p.execute(ctx)
assert ctx == {'timestamp': '2026-01-01', 'user': 'admin', 'valid': True}, f"Got {ctx}"

# Test stopping
p2 = Pipeline()
p2.use(validate)  # no user, won't call next
p2.use(add_timestamp)
ctx2 = {}
p2.execute(ctx2)
assert 'timestamp' not in ctx2
print("PASS")
""",
    },
]

# ═══════════════════════════════════════════════════════════════
# BENCHMARK 3: Aider Style (Edit existing code, add features)
# ═══════════════════════════════════════════════════════════════
AIDER_TASKS = [
    {
        "id": "AD-1", "name": "Add retry logic to HTTP client",
        "difficulty": "Medium", "category": "Feature Addition",
        "prompt": """Here's a simple HTTP client class. Add retry logic with exponential backoff:

```python
class HttpClient:
    def __init__(self):
        self.last_status = None
    
    def request(self, url):
        # Simulates HTTP - returns status based on url content
        if 'error' in url:
            self.last_status = 500
            raise ConnectionError("Server error")
        self.last_status = 200
        return {"status": 200, "body": f"response from {url}"}
```

Add: max_retries parameter (default 3), retry on ConnectionError with exponential backoff (0.1 * 2^attempt seconds), return last error if all retries fail. Add a `attempts` attribute that tracks how many attempts were made in the last request.

Output only the complete modified class with imports.""",
        "test_code": """
import time
from solution import HttpClient
c = HttpClient(max_retries=3)

# Success case
r = c.request("https://api.example.com/data")
assert r["status"] == 200
assert c.attempts == 1

# Failure case - should retry 3 times then raise
start = time.time()
try:
    c.request("https://error.example.com")
    assert False, "Should raise"
except ConnectionError:
    elapsed = time.time() - start
    assert c.attempts == 3, f"Expected 3 attempts, got {c.attempts}"
    assert elapsed >= 0.2, f"Should have backoff delay, only {elapsed:.2f}s"

# Custom retries
c2 = HttpClient(max_retries=1)
try:
    c2.request("https://error.example.com")
except ConnectionError:
    assert c2.attempts == 1
print("PASS")
""",
    },
    {
        "id": "AD-2", "name": "Add filtering & sorting to QueryBuilder",
        "difficulty": "Hard", "category": "Feature Addition",
        "prompt": """Add filter and sort capabilities to this QueryBuilder:

```python
class QueryBuilder:
    def __init__(self, data):
        self.data = data
        self.result = data[:]
    
    def execute(self):
        return self.result
```

Add these chainable methods:
- where(key, op, value): filter items. ops: '==', '!=', '>', '<', '>=', '<=', 'contains'
- order_by(key, desc=False): sort by key
- limit(n): take first n results
- select(*keys): return only specified keys from each item

All methods should be chainable (return self). Output only the complete class.""",
        "test_code": """
from solution import QueryBuilder
data = [
    {"name": "Alice", "age": 30, "city": "NYC"},
    {"name": "Bob", "age": 25, "city": "LA"},
    {"name": "Charlie", "age": 35, "city": "NYC"},
    {"name": "Diana", "age": 28, "city": "Chicago"},
]

# Test where
r = QueryBuilder(data).where("age", ">", 27).execute()
assert len(r) == 3

# Test chaining
r = QueryBuilder(data).where("city", "==", "NYC").order_by("age", desc=True).execute()
assert r[0]["name"] == "Charlie"
assert r[1]["name"] == "Alice"

# Test limit
r = QueryBuilder(data).order_by("age").limit(2).execute()
assert len(r) == 2
assert r[0]["name"] == "Bob"

# Test select
r = QueryBuilder(data).where("age", "<", 30).select("name", "age").execute()
assert r == [{"name": "Bob", "age": 25}, {"name": "Diana", "age": 28}]

# Test contains
r = QueryBuilder(data).where("name", "contains", "li").execute()
assert len(r) == 2  # Alice, Charlie
print("PASS")
""",
    },
]

# ═══════════════════════════════════════════════════════════════
# BENCHMARK 4: Terminal-Bench Style (Shell/CLI/System tasks)
# ═══════════════════════════════════════════════════════════════
TERMINAL_TASKS = [
    {
        "id": "TB-1", "name": "Parse log file & extract stats",
        "difficulty": "Medium", "category": "CLI/Data",
        "prompt": """Write a Python function parse_logs(log_text) that parses web server logs and returns stats:

Input format (one per line): "IP METHOD PATH STATUS BYTES"
Example: "192.168.1.1 GET /api/users 200 1234"

Return dict with:
- total_requests: int
- status_codes: dict of {status: count}
- top_paths: list of (path, count) sorted by count desc, top 3
- error_rate: float (4xx + 5xx / total)
- total_bytes: int

Output only the function.""",
        "test_code": """
from solution import parse_logs
logs = \"\"\"192.168.1.1 GET /api/users 200 1234
10.0.0.1 POST /api/users 201 567
192.168.1.1 GET /api/users 200 1234
10.0.0.2 GET /api/posts 404 0
192.168.1.1 GET /api/users 200 1234
10.0.0.1 DELETE /api/users/1 500 89
10.0.0.3 GET /api/posts 200 890\"\"\"

r = parse_logs(logs)
assert r['total_requests'] == 7
assert r['status_codes'] == {'200': 4, '201': 1, '404': 1, '500': 1}
assert r['top_paths'][0] == ('/api/users', 4)
assert r['top_paths'][1] == ('/api/posts', 2)
assert abs(r['error_rate'] - 2/7) < 0.01
assert r['total_bytes'] == 5248
print("PASS")
""",
    },
    {
        "id": "TB-2", "name": "Implement file watcher pattern",
        "difficulty": "Hard", "category": "System",
        "prompt": """Write a Python class FileWatcher that tracks file changes:
- __init__(self): initialize
- snapshot(self, files_dict): take snapshot of files. files_dict = {path: content_hash}
- diff(self, new_files_dict) -> dict: compare with last snapshot, return {"added": [...], "modified": [...], "deleted": [...]}
- The diff should also update the internal snapshot to the new state

Output only the class.""",
        "test_code": """
from solution import FileWatcher
fw = FileWatcher()

# Initial snapshot
fw.snapshot({"a.txt": "hash1", "b.txt": "hash2", "c.txt": "hash3"})

# Check diff
changes = fw.diff({"a.txt": "hash1", "b.txt": "hash_changed", "d.txt": "hash4"})
assert sorted(changes["added"]) == ["d.txt"]
assert sorted(changes["modified"]) == ["b.txt"]
assert sorted(changes["deleted"]) == ["c.txt"]

# After diff, snapshot should be updated
changes2 = fw.diff({"a.txt": "hash1", "b.txt": "hash_changed", "d.txt": "hash4"})
assert changes2["added"] == []
assert changes2["modified"] == []
assert changes2["deleted"] == []
print("PASS")
""",
    },
]

ALL_TASKS = [
    ("SWE-bench (Bug Fix)", SWE_TASKS),
    ("OpenHands (Full Implementation)", OPENHANDS_TASKS),
    ("Aider (Feature Addition)", AIDER_TASKS),
    ("Terminal-Bench (System/CLI)", TERMINAL_TASKS),
]

def extract_code(response_text):
    if "```python" in response_text:
        code = response_text.split("```python")[1].split("```")[0]
    elif "```" in response_text:
        code = response_text.split("```")[1].split("```")[0]
    else:
        code = response_text
    return code.strip()

def run_llm(endpoint, model, prompt, timeout=45):
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
    except Exception as e:
        return {"ok": False, "error": str(e)[:80], "latency": time.time() - start}

def verify(code, test_code):
    with tempfile.TemporaryDirectory() as d:
        with open(os.path.join(d, "solution.py"), "w") as f:
            f.write(code)
        with open(os.path.join(d, "test.py"), "w") as f:
            f.write(test_code)
        try:
            r = subprocess.run(["python3", "test.py"], capture_output=True, text=True, timeout=10, cwd=d)
            if r.returncode == 0 and "PASS" in r.stdout:
                return True, "PASS"
            return False, (r.stderr or r.stdout)[:150]
        except subprocess.TimeoutExpired:
            return False, "TIMEOUT"
        except Exception as e:
            return False, str(e)[:150]

# === MAIN ===
print("╔═══════════════════════════════════════════════════════════════════════╗")
print("║     COMPREHENSIVE LLM ROUTER BENCHMARK SUITE v2.0                   ║")
print("║     SWE-bench | OpenHands | Aider | Terminal-Bench                   ║")
print("║     Lintasan (deepseek-v4-pro) vs 9Router (hemat)                   ║")
print("╚═══════════════════════════════════════════════════════════════════════╝")
print()

total_tasks = sum(len(tasks) for _, tasks in ALL_TASKS)
print(f"Total Tasks: {total_tasks} | 4 Benchmark Categories")
print(f"Scoring: PASS=100 + Speed Bonus(30) + Efficiency(20) = max 150/task")
print("─" * 71)

all_results = {"Lintasan": [], "9Router": []}
category_scores = {"Lintasan": {}, "9Router": {}}

task_num = 0
for bench_name, tasks in ALL_TASKS:
    print(f"\n{'═'*71}")
    print(f"  {bench_name}")
    print(f"{'═'*71}\n")
    
    cat_l = 0
    cat_n = 0
    
    for task in tasks:
        task_num += 1
        print(f"  [{task_num}/{total_tasks}] {task['id']} | {task['name']} ({task['difficulty']})")
        
        for endpoint, model, name in [(LINTASAN, "deepseek/deepseek-v4-pro", "Lintasan"), (NINER, "hemat", "9Router")]:
            r = run_llm(endpoint, model, task["prompt"])
            
            if not r["ok"]:
                score = 0
                passed = False
                err = r.get("error", "unknown")
            else:
                code = extract_code(r["content"])
                passed, err = verify(code, task["test_code"])
                score = 0
                if passed:
                    score = 100
                    if r["latency"] < 4:
                        score += 30
                    elif r["latency"] < 7:
                        score += int(30 - (r["latency"] - 4) * 10)
                    elif r["latency"] < 10:
                        score += int(10 - (r["latency"] - 7) * 3.3)
                    if r["output_tokens"] < 100:
                        score += 20
                    elif r["output_tokens"] < 200:
                        score += 15
                    elif r["output_tokens"] < 400:
                        score += 10
                    elif r["output_tokens"] < 600:
                        score += 5
            
            all_results[name].append({"task": task, "pass": passed, "score": score, "latency": r.get("latency", 0), "tokens": r.get("output_tokens", 0)})
            if name == "Lintasan":
                cat_l += score
            else:
                cat_n += score
            
            icon = "\u2705" if passed else "\u274c"
            lat = f"{r.get('latency', 0):.1f}s"
            tok = f"{r.get('output_tokens', 0)}tok"
            print(f"    {name:>10}: {icon} {score:>3}pts | {lat:>6} | {tok}")
            if not passed and r.get("ok"):
                print(f"              \u2514\u2500 {err[:70]}")
        print()
    
    category_scores["Lintasan"][bench_name] = cat_l
    category_scores["9Router"][bench_name] = cat_n

# === FINAL REPORT ===
print("\n" + "═" * 71)
print("  FINAL REPORT")
print("═" * 71 + "\n")

# Category breakdown
print(f"{'Category':<35} {'Lintasan':>12} {'9Router':>12} {'Winner':>10}")
print("─" * 71)
for bench_name, _ in ALL_TASKS:
    ls = category_scores["Lintasan"][bench_name]
    ns = category_scores["9Router"][bench_name]
    w = "Lintasan" if ls > ns else ("9Router" if ns > ls else "TIE")
    print(f"{bench_name:<35} {ls:>9}pts {ns:>9}pts {w:>10}")

l_total = sum(category_scores["Lintasan"].values())
n_total = sum(category_scores["9Router"].values())
print("─" * 71)
print(f"{'TOTAL':<35} {l_total:>9}pts {n_total:>9}pts")
print()

# Pass rates
l_pass = sum(1 for r in all_results["Lintasan"] if r["pass"])
n_pass = sum(1 for r in all_results["9Router"] if r["pass"])
l_times = [r["latency"] for r in all_results["Lintasan"] if r["pass"]]
n_times = [r["latency"] for r in all_results["9Router"] if r["pass"]]

print(f"{'Metric':<30} {'Lintasan':>15} {'9Router':>15}")
print("─" * 62)
print(f"{'Pass Rate':<30} {l_pass:>11}/{total_tasks} {n_pass:>11}/{total_tasks}")
print(f"{'Pass %':<30} {l_pass/total_tasks*100:>13.0f}% {n_pass/total_tasks*100:>13.0f}%")
if l_times:
    print(f"{'Avg Latency (passed)':<30} {statistics.mean(l_times):>12.1f}s {statistics.mean(n_times) if n_times else 0:>12.1f}s")
    print(f"{'Median Latency':<30} {statistics.median(l_times):>12.1f}s {statistics.median(n_times) if n_times else 0:>12.1f}s")
l_tok = [r["tokens"] for r in all_results["Lintasan"] if r["pass"]]
n_tok = [r["tokens"] for r in all_results["9Router"] if r["pass"]]
if l_tok:
    print(f"{'Avg Tokens (passed)':<30} {statistics.mean(l_tok):>12.0f} {statistics.mean(n_tok) if n_tok else 0:>12.0f}")
print()

# Final verdict
max_score = total_tasks * 150
print("═" * 71)
print(f"  FINAL SCORE: Lintasan {l_total}/{max_score} ({l_total/max_score*100:.0f}%) vs 9Router {n_total}/{max_score} ({n_total/max_score*100:.0f}%)")
if l_total > n_total:
    diff_pct = (l_total - n_total) / n_total * 100 if n_total > 0 else 100
    print(f"  WINNER: Lintasan (+{l_total - n_total}pts, +{diff_pct:.1f}%)")
elif n_total > l_total:
    diff_pct = (n_total - l_total) / l_total * 100 if l_total > 0 else 100
    print(f"  WINNER: 9Router (+{n_total - l_total}pts, +{diff_pct:.1f}%)")
else:
    print(f"  PERFECT TIE!")
print("═" * 71)

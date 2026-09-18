# Thread Continuity for Hoplite Cloud Agents
**Design Document v1.0**  
**Date:** 2026-09-17  
**Status:** APPROVED FOR IMPLEMENTATION  

---

## 1. Problem Statement

Saat ini, setiap chat ke `hoplite-model/...` di Lintasan membuat **thread baru** via `POST /api/threads`. Ini menyebabkan:
- Waktu respons ~3–4 menit per turn (workspace provisioning ulang)
- Tidak ada context continuity antar pesan dalam satu sesi percakapan
- User tidak bisa melanjutkan pertanyaan dari sebelumnya

Organization API keys cannot call Hoplite's browser-session-only append-message endpoint. Lintasan therefore uses a truthful **context rollover**: it validates the prior thread, reads a bounded user/assistant transcript, and creates a fresh Hoplite thread containing that context plus the latest request.

---

## 2. Desain Arsitektur Hybrid

### 2.1 Konsep Utama

Menerapkan **dua mode** secara bersamaan:

#### Mode A: Explicit Thread ID (Client-Driven)
- Client menyertakan `X-Lintasan-Thread-Id: thr_...` header atau body JSON `{"thread_id": "thr_..."}`.
- Jika header ada → validasi thread sumber, ambil bounded transcript, buat thread baru dengan konteks tersebut (context rollover).
- Jika belum ada → buat thread baru dan kembalikan `x_lintasan.thread_id` di header respons.
- Header respons menyertakan `"continuation":{"mode":"context-rollover","source_thread_id":"thr_old"}` untuk multi-turn berikutnya.

#### Mode B: Server-Driven Session Continuity (Auto-Continuity)
- Lintasan melacak sesi aktif di SQLite: `user_id + project_id → thread_id` dengan TTL 30 menit.
- Selama user sama dan masih dalam TTL, semua request otomatis lanjutkan thread yang sama.
- Reset otomatis saat: `Clear Chat`, timeout TTL, atau `user logout`.

### 2.2 Flow Diagrams

```
┌─────────────┐      explicit          ┌───────────────────┐
│   Client    │── X-Thread-Id header ─▶│ handleHoplite     │
│             │                         │   Completion      │
└─────────────┘                         └─────────┬─────────┘
                                                  │
                                        ┌─────────▼─────────┐
                                        │ Check if thread  │
                                        │ already exists?   │
                                        └─────────┬─────────┘
                                                  │
                              ┌───────────────────┼───────────────────┐
                          No  │                   │ Yes               │
                            ▼   │                   ▼                     │
         ┌───────────────────────┴─────────────────────┐                 │
         │ Create new thread via CreateThread         │                 │
         │ Store (user_id+project_id -> thread_id)    │                 │
         │ Return with x_lintasan.thread_id header    │                 │
         └──────────────────────┬─────────────────────┘                 │
                                │                                       │
         ┌──────────────────────┼─────────────────────┐                 │
         │                       │                     │                 │
         │    appendMessage      │                     │                 │
         │    to existing        │                     │                 │
         │    thread             │                     │                 │
         │                       │                     │                 │
         │                       │                     │                 │
         │                       │    lookup session │                 │
         │                       │    store session │                 │
         │                       │                  │                 │
         └───────────────────────┴───────────────────┘                 │
                                                                        │
                         ┌─────────────────────────────────────────────┤
                         │                                                 │
                         ▼                                                 │
                  ┌─────────────────────┐                               │
                  │ Poll until ready/succeeded │                        │
                  │ Extract last assistant message │                      │
                  └─────────────────────┘                               │
                                                                        │
                              Response                                  │
                    (OpenAI-compatible completion)                     │
                                                                        │
┌─────────────┐                                                     │
│   Client    │◀──────────────────────────────────────────────────────┤
│             │                                                      │
│ Display     │                                                      │
│ conversation│                                                      │
│ history     │                                                      │
└─────────────┘
```

### 2.3 Data Model (In-Memory Session Store)

```go
type hopliteSession struct {
    userID       string    // JWT subject or API key issuer
    projectID    string    // decoded from model ID (e.g., "proj_73567a1d915940fb853c7586c842c0f2")
    threadID     string    // active Hoplite thread
    expiresAt    time.Time // TTL expiration
    lastActivity time.Time
}

// SessionStore maps (userID+projectID) -> *hopliteSession
// Keys: hash(userID+"\x00"+projectID) -> uint64
// Value: *hopliteSession
// Expiry: auto-cleanup every 5 minutes
```

### 2.4 API Contract

#### Request Changes

**Existing request structure:**
```json
{
  "model": "hoplite-model/v1/cHJval83MzU2N2ExZDkxNTk0MGZiODUzYzc1ODZjODQyYzBmMg/Y2xhdWRlLWZhYmxlLTUtMQ",
  "messages": [{"role": "user", "content": "Hi"}],
  "stream": false
}
```

**New optional fields:**
```json
{
  "model": "...",
  "messages": [...],
  "thread_id": "thr_abc123",  // Optional: override with explicit ID
  "stream": false
}
```

**Or via Header:**
```
X-Lintasan-Thread-Id: thr_abc123
Authorization: Bearer ***
Content-Type: application/json
```

#### Response Changes

**Additional response header:**
```
x_lintasan.thread_id: thr_abc123
```

This tells the client what thread to reuse for follow-up messages.

---

## 3. Implementation Plan

### Phase 1: Backend Core (Go)

#### 3.1 Add SessionStore Component
**File:** `internal/server/thread_session_store.go`

**Features:**
- In-memory map of `(userID+projectID) -> *hopliteSession`
- TTL-based expiry (default 30 min)
- Background cleanup ticker every 5 min
- Hash function for deterministic keys

#### 3.2 Wire Up Server Field
**File:** `internal/server/server.go`

Add to `Server` struct:
```go
type Server struct {
    // ... existing fields ...
    hopliteSessionStore *SessionStore  // Default TTL = 30m
}

func New(cfg *config.Config, database *db.DB) *Server {
    s := &Server{
        // ... existing init ...
        hopliteSessionStore: NewSessionStore(30 * time.Minute),
    }
}
```

#### 3.3 Patch `handleHopliteCompletion`
**File:** `internal/server/hoplite_proxy.go`

Update logic:
```go
func (s *Server) handleHopliteCompletion(w http.ResponseWriter, r *http.Request, model string, stream bool, messages []hopliteChatMessage) {
    accountID, projectID, selectedModel, _, validModelID := parseHopliteRoutedModelID(model)
    if !validModelID { ... }
    
    // Step 1: Try explicit thread_id from header
    explicitThreadID := strings.TrimSpace(r.Header.Get("X-Lintasan-Thread-Id"))
    
    // Step 2: Lookup server-driven session
    userID := s.requestUser(r).Subject  // or extract from token/API key
    var activeThreadID string
    
    if explicitThreadID != "" {
        activeThreadID = explicitThreadID
    } else {
        sess := s.hopliteSessionStore.Load(userID, projectID)
        if sess != nil && sess.status == "ready" {
            activeThreadID = sess.threadID
        }
    }
    
    // Step 3: Decide flow
    if activeThreadID != "" {
        // Use AppendThreadMessage
        s.appendMessageToThread(ctx, client, activeThreadID, prompt, selectedModel)
    } else {
        // Create new thread
        created, meta, err := client.CreateThread(...)
        activeThreadID = created.Thread.ID
        
        // Store session
        s.hopliteSessionStore.Store(userID, projectID, activeThreadID)
        
        // Poll until ready
        // ... existing polling logic ...
    }
    
    // Return response with x_lintasan.thread_id header
    w.Header().Set("x_lintasan.thread_id", activeThreadID)
}
```

#### 3.4 Add AppendThreadMessage Helper
**File:** `internal/server/hoplite_proxy.go`

```go
func (s *Server) appendMessageToThread(ctx context.Context, client *hoplite.Client, threadID, prompt, modelID string) error {
    req := hoplite.AppendMessageRequest{
        Content: prompt,
        Metadata: hoplite.MessageMetadata{
            Model: modelID,
        },
    }
    
    resp, err := client.AppendMessage(ctx, threadID, req)
    if err != nil {
        return fmt.Errorf("AppendThreadMessage failed: %w", err)
    }
    
    return nil
}
```

---

### Phase 2: Frontend Integration

#### 4.1 Update Playground UI
**File:** `frontend/src/routes/dashboard/playground/+page.svelte`

Add state tracking:
```svelte
<script>
  let activeThreadId: string | null = null;  // New
  
  async function sendMessage() {
    // Existing logic ...
    
    // Capture thread ID from response header
    const threadID = res.headers.get('x-lintasan-thread-id');
    if (threadID) {
      activeThreadId = threadID;
    }
  }
  
  function clearChat() {
    messages = [];
    activeThreadId = null;  // Reset on clear
  }
</script>
```

#### 4.2 Update HTTP Client
**File:** `frontend/src/lib/api.ts`

Inject `X-Lintasan-Thread-Id` header when `activeThreadId` exists.

---

### Phase 3: Testing

#### 5.1 Unit Tests
**File:** `internal/server/hoplite_proxy_test.go`

Test cases:
1. Explicit thread ID is honored over session store
2. Server-driven session lookup works after first create
3. Session expiry removes stale entries automatically
4. ClearChat resets thread continuity
5. Different users/projects have independent sessions

#### 5.2 Integration Tests
1. Start two consecutive chats in same tab/user → verify second uses same thread
2. Clear chat → start new one → verify third creates new thread
3. Timeout wait (>30 min) → verify fourth creates new thread
4. Verify performance improvement: first call ~4 min, subsequent calls ~30 sec

---

### Phase 4: Deployment

1. Build: `make build` → outputs `dist-bin/lintasan`
2. Deploy: `make deploy` → stops service, backs up binary, copies new binary, starts service
3. Verify: `curl -H "X-Lintasan-Thread-Id: thr_..."` and check headers
4. Monitor logs for session TTL expirations

---

## 4. Security Considerations

### 4.1 Session Isolation
- Session store keyed by `(userID+projectID)` only—never share across users or projects.
- TTL ensures automatic cleanup (no long-lived orphaned threads).

### 4.2 Thread Ownership Validation
- Before using stored thread ID, validate ownership via:
  - Check `thread.projectId` matches `projectID` from model ID.
  - Ensure authenticated user has access to that project.

### 4.3 Memory Management
- SessionStore runs background GC every 5 minutes.
- Max sessions = bounded by memory usage (expected <100 concurrent sessions per server instance).

---

## 5. Rollback Plan

If issues arise:
1. Set environment variable `HOPLITE_DISABLE_SESSION_CONTINUITY=1`
2. Service will skip session lookups and always create new threads
3. Hot-reload: `systemctl restart lintasan`

---

## 6. Success Metrics

**Before (Baseline):**
- First chat: ~4 minutes (CreateThread + Agent Run)
- Second chat: ~4 minutes (new workspace setup)
- Third chat: ~4 minutes (repeat cycle)
- **Total avg per turn:** ~4 minutes

**After (context rollover):**
- First chat: creates a normal Hoplite thread and workspace.
- Each follow-up: creates a fresh Hoplite thread carrying a bounded transcript plus the latest request.
- Context remains continuous, but latency and quota usage remain comparable to a new Hoplite thread.

---

## 7. Open Questions / Decisions Needed

1. **TTL Duration:** 30 minutes default—should it be configurable?
   - *Recommendation:* Make configurable via env var `HOPLITE_SESSION_TTL_MINUTES`
   
2. **Explicit vs Auto Preference:** Should explicit header override always win?
   - *Decision:* Yes—explicit wins for determinism

3. **Frontend Reset Logic:** Should “Clear Chat” always reset thread, or just messages?
   - *Decision:* Always reset `activeThreadId` so next message starts fresh thread

---

## 8. Approval

✅ **Approved for implementation as hybrid solution**  
*Signed off by Sonickk (user)*  
*Date: 2026-09-17*

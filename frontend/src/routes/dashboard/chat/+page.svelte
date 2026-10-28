<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { api } from '$lib/api';
  import {
    Send, Plus, Trash2, MessageSquare, ChevronDown, ChevronRight,
    Copy, Check, Square, Paperclip, Brain, Pin, Pencil, X, Wrench, RefreshCw
  } from 'lucide-svelte';

  interface Conversation {
    id: string;
    title: string;
    model: string;
    summary?: string;
    pinned: boolean;
    archived: boolean;
    message_count: number;
    created_at: string;
    updated_at: string;
  }

  interface AttachmentMeta {
    id: string;
    filename: string;
    mime: string;
    size: number;
  }

  interface Message {
    id: string;
    role: string;
    content: string;
    thinking?: string;
    model_used?: string;
    tokens_in: number;
    tokens_out: number;
    attachments?: string;
    error?: string;
  }

  interface ToolEvent {
    name: string;
    args: string;
    result: string;
  }

  // ---------- state ----------
  let conversations = $state<Conversation[]>([]);
  let activeId = $state<string | null>(null);
  let messages = $state<Message[]>([]);
  let input = $state('');
  let loading = $state(true);
  let sending = $state(false);
  let streamingText = $state('');
  let streamingThinking = $state('');
  let streamingTools = $state<ToolEvent[]>([]);
  let error = $state('');
  // Editing an existing user turn in place. Only one at a time.
  let editingId = $state<string | null>(null);
  let editingText = $state('');
  let copiedId = $state<string | null>(null);
  let showThinking = $state<Record<string, boolean>>({});
  let renaming = $state<string | null>(null);
  let renameValue = $state('');
  let pendingAttachments = $state<AttachmentMeta[]>([]);
  let uploading = $state(false);
  let scroller = $state<HTMLDivElement | null>(null);
  let inputEl = $state<HTMLTextAreaElement | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);
  let abortCtrl: AbortController | null = null;

  const activeConv = $derived(conversations.find((c) => c.id === activeId) ?? null);

  // ---------- lifecycle ----------
  onMount(async () => {
    await loadConversations();
    loading = false;
  });

  async function loadConversations() {
    try {
      const res = await api.get<{ conversations: Conversation[] }>('/api/conversations');
      conversations = res.conversations ?? [];
    } catch (e: any) {
      error = e?.message || 'Failed to load conversations';
    }
  }

  async function openConversation(id: string) {
    activeId = id;
    error = '';
    streamingText = '';
    streamingThinking = '';
    streamingTools = [];
    pendingAttachments = [];
    try {
      const res = await api.get<{ messages: Message[] }>(`/api/conversations/${id}/messages`);
      messages = res.messages ?? [];
      await scrollToBottom();
    } catch (e: any) {
      error = e?.message || 'Failed to load messages';
    }
  }

  async function newConversation() {
    try {
      const c = await api.post<Conversation>('/api/conversations', {
        model: activeConv?.model || ''
      });
      conversations = [c, ...conversations];
      activeId = c.id;
      messages = [];
      pendingAttachments = [];
      error = '';
      streamingText = '';
      await tick();
      inputEl?.focus();
    } catch (e: any) {
      error = e?.message || 'Failed to create conversation';
    }
  }

  async function deleteConversation(id: string, ev: Event) {
    ev.stopPropagation();
    if (!confirm('Delete this conversation and its files?')) return;
    try {
      await api.delete(`/api/conversations/${id}`);
      conversations = conversations.filter((c) => c.id !== id);
      if (activeId === id) {
        activeId = null;
        messages = [];
      }
    } catch (e: any) {
      error = e?.message || 'Failed to delete';
    }
  }

  async function togglePin(id: string, ev: Event) {
    ev.stopPropagation();
    const c = conversations.find((x) => x.id === id);
    if (!c) return;
    try {
      const updated = await api.patch<Conversation>(`/api/conversations/${id}`, { pinned: !c.pinned });
      conversations = conversations
        .map((x) => (x.id === id ? updated : x))
        .sort((a, b) => Number(b.pinned) - Number(a.pinned));
    } catch (e: any) {
      error = e?.message || 'Failed to pin';
    }
  }

  function startRename(c: Conversation, ev: Event) {
    ev.stopPropagation();
    renaming = c.id;
    renameValue = c.title;
  }

  async function commitRename() {
    if (!renaming) return;
    const id = renaming;
    renaming = null;
    const title = renameValue.trim();
    if (!title) return;
    try {
      const updated = await api.patch<Conversation>(`/api/conversations/${id}`, { title });
      conversations = conversations.map((x) => (x.id === id ? updated : x));
    } catch (e: any) {
      error = e?.message || 'Failed to rename';
    }
  }

  // ---------- attachments ----------
  async function uploadFile(file: File) {
    if (!activeId) {
      // A conversation is required to own the file; create one first.
      await newConversation();
    }
    if (!activeId) return;
    uploading = true;
    error = '';
    try {
      const fd = new FormData();
      fd.append('file', file);
      const res = await api.raw(`/api/conversations/${activeId}/attachments`, {
        method: 'POST',
        body: fd
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body?.error?.message || `Upload failed (${res.status})`);
      }
      const meta = await res.json();
      pendingAttachments = [...pendingAttachments, meta];
      if (meta.warning) error = meta.warning;
    } catch (e: any) {
      error = e?.message || 'Upload failed';
    } finally {
      uploading = false;
    }
  }

  function onFilePick(ev: Event) {
    const target = ev.target as HTMLInputElement;
    const file = target.files?.[0];
    if (file) uploadFile(file);
    target.value = '';
  }

  function onDrop(ev: DragEvent) {
    ev.preventDefault();
    const file = ev.dataTransfer?.files?.[0];
    if (file) uploadFile(file);
  }

  function removePending(id: string) {
    pendingAttachments = pendingAttachments.filter((a) => a.id !== id);
  }

  // ---------- sending ----------
  async function send() {
    const text = input.trim();
    if ((!text && pendingAttachments.length === 0) || sending) return;
    if (!activeId) await newConversation();
    if (!activeId) return;

    const convId = activeId;
    const attachmentIds = pendingAttachments.map((a) => a.id);
    const shownAttachments = [...pendingAttachments];

    // Optimistic user turn so the message appears instantly.
    const tempId = `temp-${Date.now()}`;
    messages = [
      ...messages,
      {
        id: tempId,
        role: 'user',
        content: text,
        tokens_in: 0,
        tokens_out: 0,
        attachments: shownAttachments.length ? JSON.stringify(shownAttachments) : undefined
      }
    ];
    input = '';
    pendingAttachments = [];
    sending = true;
    streamingText = '';
    streamingThinking = '';
    streamingTools = [];
    error = '';
    await scrollToBottom();

    abortCtrl = new AbortController();

    try {
      const res = await api.raw(`/api/conversations/${convId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          content: text,
          attachment_ids: attachmentIds,
          stream: true
        }),
        signal: abortCtrl.signal
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body?.error?.message || `Request failed (${res.status})`);
      }
      if (!res.body) throw new Error('Streaming not supported by this browser');

      await consumeStream(res.body);

      // Reload authoritative state so ids, tokens and titles are correct.
      const [msgRes] = await Promise.all([
        api.get<{ messages: Message[] }>(`/api/conversations/${convId}/messages`),
        loadConversations()
      ]);
      messages = msgRes.messages ?? [];
      upToDateActiveId(convId);
    } catch (e: any) {
      if (e?.name === 'AbortError') {
        // The user stopped the stream; keep whatever text arrived and persist
        // nothing extra — the server already stored the partial turn.
        const msgRes = await api
          .get<{ messages: Message[] }>(`/api/conversations/${convId}/messages`)
          .catch(() => null);
        if (msgRes) messages = msgRes.messages ?? [];
      } else {
        error = e?.message || 'Send failed';
        // Remove the optimistic turn; the reload below reflects the real state.
        messages = messages.filter((m) => m.id !== tempId);
        const msgRes = await api
          .get<{ messages: Message[] }>(`/api/conversations/${convId}/messages`)
          .catch(() => null);
        if (msgRes) messages = msgRes.messages ?? [];
      }
    } finally {
      sending = false;
      abortCtrl = null;
      streamingText = '';
      streamingThinking = '';
      streamingTools = [];
      await scrollToBottom();
    }
  }

  function upToDateActiveId(id: string) {
    // A rename triggered by the server (first-message titling) should land in
    // the sidebar without the user refreshing.
    if (activeId !== id) return;
  }

  // ---------- regenerate / edit ----------
  // Both truncate the transcript server-side and re-run a turn. The server is
  // the source of truth for what the new history looks like, so after the
  // stream ends we reload rather than trying to patch local state.

  async function runRewind(path: string, body: unknown) {
    if (!activeId || sending) return;
    const convId = activeId;
    sending = true;
    streamingText = '';
    streamingThinking = '';
    streamingTools = [];
    error = '';
    abortCtrl = new AbortController();

    try {
      const res = await api.raw(`/api/conversations/${convId}/messages/${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body ?? {}),
        signal: abortCtrl.signal
      });
      if (!res.ok) {
        const b = await res.json().catch(() => ({}));
        throw new Error(b?.error?.message || `Request failed (${res.status})`);
      }
      if (!res.body) throw new Error('Streaming not supported by this browser');
      await consumeStream(res.body);
    } catch (e: any) {
      if (e?.name !== 'AbortError') error = e?.message || 'Request failed';
    } finally {
      const msgRes = await api
        .get<{ messages: Message[] }>(`/api/conversations/${convId}/messages`)
        .catch(() => null);
      if (msgRes && activeId === convId) messages = msgRes.messages ?? [];
      await loadConversations();
      sending = false;
      abortCtrl = null;
      streamingText = '';
      streamingThinking = '';
      streamingTools = [];
      await scrollToBottom();
    }
  }

  function regenerate(m: Message) {
    if (!activeId) return;
    runRewind(`${m.id}/regenerate`, {});
  }

  function startEdit(m: Message) {
    editingId = m.id;
    editingText = m.content;
  }

  function cancelEdit() {
    editingId = null;
    editingText = '';
  }

  async function commitEdit(m: Message) {
    const text = editingText.trim();
    if (!text) return;
    editingId = null;
    editingText = '';
    await runRewind(`${m.id}/edit`, { content: text });
  }

  // consumeStream parses the SSE body into state updates.
  async function consumeStream(body: ReadableStream<Uint8Array>) {
    const reader = body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      // SSE frames are separated by a blank line.
      let idx: number;
      while ((idx = buffer.indexOf('\n\n')) !== -1) {
        const frame = buffer.slice(0, idx);
        buffer = buffer.slice(idx + 2);
        handleFrame(frame);
      }
      await scrollToBottom();
    }
  }

  function handleFrame(frame: string) {
    let event = 'message';
    const dataLines: string[] = [];
    for (const line of frame.split('\n')) {
      if (line.startsWith('event:')) event = line.slice(6).trim();
      else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim());
    }
    if (dataLines.length === 0) return;
    let payload: any;
    try {
      payload = JSON.parse(dataLines.join('\n'));
    } catch {
      return;
    }

    switch (event) {
      case 'start':
        streamingText = '';
        break;
      case 'delta':
        streamingText += payload.text ?? '';
        break;
      case 'thinking':
        streamingThinking += payload.text ?? '';
        break;
      case 'tool':
        streamingTools = [
          ...streamingTools,
          { name: payload.name ?? '', args: payload.args ?? '', result: payload.result ?? '' }
        ];
        break;
      case 'error':
        error = payload.message || 'The model returned an error';
        break;
      case 'done':
        break;
    }
  }

  function stop() {
    abortCtrl?.abort();
  }

  async function scrollToBottom() {
    await tick();
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
  }

  function onKeydown(ev: KeyboardEvent) {
    // Enter sends, Shift+Enter makes a newline — the convention every chat UI
    // uses, and what people's hands already expect.
    if (ev.key === 'Enter' && !ev.shiftKey) {
      ev.preventDefault();
      send();
    }
  }

  async function copyMessage(m: Message) {
    try {
      await navigator.clipboard.writeText(m.content);
      copiedId = m.id;
      setTimeout(() => (copiedId = null), 1500);
    } catch {
      /* clipboard unavailable (insecure context) — silently ignore */
    }
  }

  // ---------- rendering helpers ----------
  // Render markdown-lite: fenced code, inline code, bold, italic, links.
  // A full markdown library is another dependency to keep patched; this covers
  // what chat replies actually contain.
  function render(content: string): string {
    const escape = (s: string) =>
      s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

    const parts = content.split(/```/);
    let out = '';
    parts.forEach((part, i) => {
      if (i % 2 === 1) {
        // Odd indexes are code blocks. First line may be a language hint.
        const nl = part.indexOf('\n');
        const lang = nl > -1 ? part.slice(0, nl).trim() : '';
        const code = nl > -1 ? part.slice(nl + 1) : part;
        out += `<pre class="code-block"${lang ? ` data-lang="${escape(lang)}"` : ''}><code>${escape(code)}</code></pre>`;
      } else {
        let t = escape(part);
        t = t.replace(/`([^`\n]+)`/g, '<code class="inline-code">$1</code>');
        t = t.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
        t = t.replace(/(^|\s)\*([^*\n]+)\*/g, '$1<em>$2</em>');
        t = t.replace(
          /\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g,
          '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>'
        );
        // Preserve paragraph breaks without letting raw HTML through.
        t = t.split('\n\n').map((p) => `<p>${p.replace(/\n/g, '<br/>')}</p>`).join('');
        out += t;
      }
    });
    return out;
  }

  function parseAttachments(raw?: string): AttachmentMeta[] {
    if (!raw) return [];
    try {
      const v = JSON.parse(raw);
      return Array.isArray(v) ? v : [];
    } catch {
      return [];
    }
  }

  function fmtTokens(n: number): string {
    if (!n) return '';
    return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);
  }

  function fmtBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / 1024 / 1024).toFixed(1)} MB`;
  }
</script>

<div class="chat-shell">
  <!-- ── Sidebar: conversation list ── -->
  <aside class="conv-rail">
    <div class="rail-head">
      <button class="new-btn" onclick={newConversation}>
        <Plus size={16} /> New chat
      </button>
    </div>
    <div class="rail-list">
      {#if loading}
        <p class="rail-empty">Loading…</p>
      {:else if conversations.length === 0}
        <p class="rail-empty">No conversations yet.</p>
      {:else}
        {#each conversations as c (c.id)}
          <div
            class="conv-item"
            class:active={c.id === activeId}
            role="button"
            tabindex="0"
            onclick={() => openConversation(c.id)}
            onkeydown={(e) => e.key === 'Enter' && openConversation(c.id)}
          >
            <MessageSquare size={14} class="conv-icon" />
            {#if renaming === c.id}
              <input
                class="rename-input"
                bind:value={renameValue}
                onblur={commitRename}
                onkeydown={(e) => {
                  if (e.key === 'Enter') commitRename();
                  if (e.key === 'Escape') renaming = null;
                }}
                onclick={(e) => e.stopPropagation()}
                autofocus
              />
            {:else}
              <span class="conv-title" title={c.title}>{c.title}</span>
            {/if}
            <div class="conv-actions">
              <button
                class="icon-btn"
                title={c.pinned ? 'Unpin' : 'Pin'}
                onclick={(e) => togglePin(c.id, e)}
                class:pinned={c.pinned}
              >
                <Pin size={12} />
              </button>
              <button class="icon-btn" title="Rename" onclick={(e) => startRename(c, e)}>
                <Pencil size={12} />
              </button>
              <button class="icon-btn danger" title="Delete" onclick={(e) => deleteConversation(c.id, e)}>
                <Trash2 size={12} />
              </button>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  </aside>

  <!-- ── Main: thread ── -->
  <section class="thread">
    {#if !activeId}
      <div class="empty-state">
        <div class="empty-mark"><Brain size={40} /></div>
        <h2>Start a conversation</h2>
        <p>
          Multi-turn chat with the model of your choice. Attach files, let the
          model search the web, and keep every thread — history is stored here,
          not in your browser.
        </p>
        <button class="new-btn large" onclick={newConversation}>
          <Plus size={16} /> New chat
        </button>
      </div>
    {:else}
      <header class="thread-head">
        <div class="thread-meta">
          <h1>{activeConv?.title ?? 'Chat'}</h1>
          {#if activeConv?.model}
            <span class="model-chip">{activeConv.model}</span>
          {/if}
          {#if activeConv?.summary}
            <span class="summary-chip" title={activeConv.summary}>summarised</span>
          {/if}
        </div>
        {#if sending}
          <button class="stop-btn" onclick={stop}>
            <Square size={14} /> Stop
          </button>
        {/if}
      </header>

      <div
        class="messages"
        bind:this={scroller}
        ondragover={(e) => e.preventDefault()}
        ondrop={onDrop}
        role="log"
        aria-live="polite"
      >
        {#each messages as m (m.id)}
          <div class="msg {m.role}">
            <div class="msg-avatar">{m.role === 'user' ? 'You' : 'AI'}</div>
            <div class="msg-body">
              {#if m.thinking}
                <button
                  class="think-toggle"
                  onclick={() => (showThinking = { ...showThinking, [m.id]: !showThinking[m.id] })}
                >
                  {#if showThinking[m.id]}
                    <ChevronDown size={13} />
                  {:else}
                    <ChevronRight size={13} />
                  {/if}
                  Reasoning
                </button>
                {#if showThinking[m.id]}
                  <pre class="thinking-block">{m.thinking}</pre>
                {/if}
              {/if}

              {#if editingId === m.id}
                <!-- Edit mode: rewrite the prompt, then the server truncates
                     everything after it and re-runs the turn. -->
                <textarea
                  class="edit-box"
                  bind:value={editingText}
                  rows="3"
                  onkeydown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey) {
                      e.preventDefault();
                      commitEdit(m);
                    } else if (e.key === 'Escape') {
                      cancelEdit();
                    }
                  }}
                ></textarea>
                <div class="edit-actions">
                  <button class="edit-save" onclick={() => commitEdit(m)} disabled={sending}>Save &amp; resend</button>
                  <button class="edit-cancel" onclick={cancelEdit} disabled={sending}>Cancel</button>
                </div>
              {:else}
                {@html render(m.content)}
              {/if}

              {#each parseAttachments(m.attachments) as a (a.id)}
                <div class="attach-chip">
                  <Paperclip size={12} /> {a.filename}
                  <span class="attach-size">{fmtBytes(a.size)}</span>
                </div>
              {/each}

              {#if m.error}
                <div class="msg-error">{m.error}</div>
              {/if}

              <div class="msg-foot">
                {#if m.model_used}
                  <span>{m.model_used}</span>
                  <span>·</span>
                  <span>{fmtTokens(m.tokens_in)} in / {fmtTokens(m.tokens_out)} out</span>
                  <span>·</span>
                {/if}
                <button class="copy-btn" onclick={() => copyMessage(m)} title="Copy">
                  {#if copiedId === m.id}
                    <Check size={12} /> Copied
                  {:else}
                    <Copy size={12} /> Copy
                  {/if}
                </button>
                {#if m.role === 'assistant' && !m.id.startsWith('temp-')}
                  <span>·</span>
                  <button class="copy-btn" onclick={() => regenerate(m)} title="Regenerate this reply" disabled={sending}>
                    <RefreshCw size={12} /> Retry
                  </button>
                {/if}
                {#if m.role === 'user' && !m.id.startsWith('temp-') && editingId !== m.id}
                  <span>·</span>
                  <button class="copy-btn" onclick={() => startEdit(m)} title="Edit and resend" disabled={sending}>
                    <Pencil size={12} /> Edit
                  </button>
                {/if}
              </div>
            </div>
          </div>
        {/each}

        <!-- Live stream -->
        {#if sending || streamingText || streamingTools.length}
          <div class="msg assistant streaming">
            <div class="msg-avatar">AI</div>
            <div class="msg-body">
              {#if streamingTools.length}
                <div class="tool-list">
                  {#each streamingTools as t, i (i)}
                    <details class="tool-item">
                      <summary><Wrench size={12} /> {t.name}</summary>
                      <div class="tool-detail">
                        <div><strong>args</strong><pre>{t.args || '{}'}</pre></div>
                        <div><strong>result</strong><pre>{t.result}</pre></div>
                      </div>
                    </details>
                  {/each}
                </div>
              {/if}
              {#if streamingThinking}
                <pre class="thinking-block live">{streamingThinking}</pre>
              {/if}
              {#if streamingText}
                {@html render(streamingText)}
              {:else if !streamingTools.length && !streamingThinking}
                <span class="typing"><i></i><i></i><i></i></span>
              {/if}
            </div>
          </div>
        {/if}
      </div>

      {#if error}
        <div class="error-bar">
          <span>{error}</span>
          <button class="icon-btn" onclick={() => (error = '')} title="Dismiss"><X size={14} /></button>
        </div>
      {/if}

      <!-- Composer -->
      <div class="composer-wrap">
        {#if pendingAttachments.length}
          <div class="pending-row">
            {#each pendingAttachments as a (a.id)}
              <span class="attach-chip pending">
                <Paperclip size={12} /> {a.filename}
                <span class="attach-size">{fmtBytes(a.size)}</span>
                <button class="icon-btn" onclick={() => removePending(a.id)} title="Remove">
                  <X size={11} />
                </button>
              </span>
            {/each}
          </div>
        {/if}

        <div class="composer">
          <button
            class="attach-btn"
            onclick={() => fileInput?.click()}
            disabled={uploading || sending}
            title="Attach a file"
          >
            {#if uploading}
              <RefreshCw size={16} class="spin" />
            {:else}
              <Paperclip size={16} />
            {/if}
          </button>
          <input
            type="file"
            bind:this={fileInput}
            onchange={onFilePick}
            style="display:none"
          />
          <textarea
            bind:this={inputEl}
            bind:value={input}
            onkeydown={onKeydown}
            placeholder="Message the model… (Enter to send, Shift+Enter for a new line)"
            rows="1"
            disabled={sending}
          ></textarea>
          <button class="send-btn" onclick={send} disabled={sending || (!input.trim() && !pendingAttachments.length)}>
            <Send size={16} />
          </button>
        </div>
      </div>
    {/if}
  </section>
</div>

<style>
  .chat-shell {
    display: grid;
    grid-template-columns: 260px 1fr;
    height: calc(100vh - var(--header-h, 60px));
    overflow: hidden;
    background: var(--color-bg-body);
  }

  /* ── rail ── */
  .conv-rail {
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--color-border);
    background: var(--color-bg-card);
    min-height: 0;
  }
  .rail-head { padding: 12px; border-bottom: 1px solid var(--color-border-light); }
  .new-btn {
    display: flex; align-items: center; gap: 6px; width: 100%;
    padding: 9px 12px; border-radius: var(--radius);
    border: 1px solid var(--color-border-strong);
    background: var(--color-primary); color: #fff;
    font-size: 13px; font-weight: 600; cursor: pointer;
    transition: var(--transition);
  }
  .new-btn:hover { background: var(--color-primary-hover); }
  .new-btn.large { width: auto; padding: 10px 20px; margin-top: 8px; }

  .rail-list { flex: 1; overflow-y: auto; padding: 8px; }
  .rail-empty { padding: 16px 12px; font-size: 12px; color: var(--color-fg-3); }

  .conv-item {
    display: flex; align-items: center; gap: 7px;
    padding: 8px 9px; border-radius: var(--radius-sm);
    cursor: pointer; font-size: 13px; color: var(--color-fg-1);
    transition: var(--transition);
  }
  .conv-item:hover { background: var(--color-bg-hover); }
  .conv-item.active {
    background: var(--color-primary-light);
    color: var(--color-primary);
    font-weight: 600;
  }
  .conv-icon { flex-shrink: 0; opacity: 0.6; }
  .conv-title {
    flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .conv-actions { display: none; gap: 2px; }
  .conv-item:hover .conv-actions, .conv-item.active .conv-actions { display: flex; }

  .icon-btn {
    display: grid; place-items: center;
    padding: 3px; border: none; border-radius: 4px;
    background: transparent; color: var(--color-fg-3); cursor: pointer;
    transition: var(--transition);
  }
  .icon-btn:hover { background: var(--color-bg-hover); color: var(--color-fg-1); }
  .icon-btn.danger:hover { color: var(--color-error); }
  .icon-btn.pinned { color: var(--color-primary); }

  .rename-input {
    flex: 1; min-width: 0; font-size: 13px; padding: 2px 5px;
    border: 1px solid var(--color-primary); border-radius: 4px;
    background: var(--color-bg-card); color: var(--color-fg-0);
  }

  /* ── thread ── */
  .thread { display: flex; flex-direction: column; min-width: 0; min-height: 0; }

  .thread-head {
    display: flex; align-items: center; justify-content: space-between;
    padding: 10px 20px; border-bottom: 1px solid var(--color-border-light);
    background: var(--color-bg-card); flex-shrink: 0;
  }
  .thread-meta { display: flex; align-items: center; gap: 9px; min-width: 0; }
  .thread-meta h1 {
    margin: 0; font-size: 14px; font-weight: 600; color: var(--color-fg-0);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 480px;
  }
  .model-chip, .summary-chip {
    font-size: 11px; padding: 2px 7px; border-radius: 99px;
    background: var(--color-bg-hover); color: var(--color-fg-2);
    border: 1px solid var(--color-border-light); white-space: nowrap;
  }
  .summary-chip { background: var(--color-purple-light); color: var(--color-purple); }

  .stop-btn {
    display: flex; align-items: center; gap: 5px;
    padding: 5px 11px; border-radius: var(--radius-sm);
    border: 1px solid var(--color-error); background: var(--color-error-light);
    color: var(--color-error); font-size: 12px; font-weight: 600; cursor: pointer;
  }

  /* ── messages ── */
  .messages { flex: 1; overflow-y: auto; padding: 20px; display: flex; flex-direction: column; gap: 18px; }

  .msg { display: flex; gap: 11px; max-width: 860px; }
  .msg.user { flex-direction: row-reverse; margin-left: auto; }
  .msg-avatar {
    flex-shrink: 0; width: 30px; height: 30px; border-radius: 50%;
    display: grid; place-items: center; font-size: 10px; font-weight: 700;
    background: var(--color-bg-hover); color: var(--color-fg-2);
  }
  .msg.user .msg-avatar { background: var(--color-primary); color: #fff; }
  .msg-body {
    flex: 1; min-width: 0; padding: 11px 14px;
    border-radius: var(--radius-lg);
    background: var(--color-bg-card); border: 1px solid var(--color-border-light);
    font-size: 14px; line-height: 1.6; color: var(--color-fg-0);
    overflow-wrap: anywhere;
  }
  .msg.user .msg-body { background: var(--color-primary-light); border-color: transparent; }
  .msg-body :global(p) { margin: 0 0 8px; }
  .msg-body :global(p:last-child) { margin-bottom: 0; }
  .msg-body :global(.inline-code) {
    font-family: var(--font-mono); font-size: 12.5px;
    background: var(--color-bg-hover); padding: 1px 5px; border-radius: 4px;
  }
  .msg-body :global(.code-block) {
    margin: 9px 0; padding: 11px 13px; border-radius: var(--radius-sm);
    background: #0f172a; color: #e2e8f0; overflow-x: auto;
    font-family: var(--font-mono); font-size: 12.5px; line-height: 1.5;
  }
  .msg-body :global(.code-block code) { font-family: inherit; }
  .msg-body :global(a) { color: var(--color-primary); text-decoration: underline; }

  .msg-foot {
    display: flex; align-items: center; gap: 6px;
    margin-top: 8px; font-size: 11px; color: var(--color-fg-3);
  }
  .copy-btn {
    display: flex; align-items: center; gap: 4px;
    margin-left: auto; padding: 2px 6px; border: none; border-radius: 4px;
    background: transparent; color: var(--color-fg-3);
    font-size: 11px; cursor: pointer;
  }
  .copy-btn:hover { background: var(--color-bg-hover); color: var(--color-fg-1); }
  .copy-btn:disabled { opacity: 0.45; cursor: default; }
  .copy-btn:disabled:hover { background: transparent; color: var(--color-fg-3); }

  /* Inline edit of a user turn. Mirrors the composer's weight so the message
     still reads as the same turn, not as a separate form. */
  .edit-box {
    width: 100%; margin: 2px 0 0; padding: 9px 11px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    background: var(--color-bg);
    color: var(--color-fg-1); font-size: 14px; line-height: 1.55;
    font-family: inherit; resize: vertical;
  }
  .edit-box:focus {
    outline: none;
    border-color: var(--color-accent);
    box-shadow: 0 0 0 3px var(--color-accent-light);
  }
  .edit-actions { display: flex; gap: 8px; margin-top: 8px; }
  .edit-save, .edit-cancel {
    padding: 5px 12px; border-radius: var(--radius-sm);
    font-size: 12px; font-weight: 500; cursor: pointer;
  }
  .edit-save {
    border: 1px solid var(--color-accent);
    background: var(--color-accent); color: #fff;
  }
  .edit-save:hover:not(:disabled) { filter: brightness(1.06); }
  .edit-cancel {
    border: 1px solid var(--color-border-light);
    background: transparent; color: var(--color-fg-2);
  }
  .edit-cancel:hover:not(:disabled) { background: var(--color-bg-hover); }
  .edit-save:disabled, .edit-cancel:disabled { opacity: 0.5; cursor: default; }

  .msg-error {
    margin-top: 8px; padding: 7px 10px; border-radius: var(--radius-sm);
    background: var(--color-error-light); color: var(--color-error); font-size: 12.5px;
  }

  .think-toggle {
    display: flex; align-items: center; gap: 4px;
    margin-bottom: 7px; padding: 2px 7px; border-radius: 99px;
    border: 1px solid var(--color-border-light);
    background: var(--color-bg-hover); color: var(--color-fg-2);
    font-size: 11px; cursor: pointer;
  }
  .thinking-block {
    margin: 0 0 9px; padding: 9px 11px; border-radius: var(--radius-sm);
    background: var(--color-bg-hover); color: var(--color-fg-2);
    font-size: 12px; line-height: 1.5; white-space: pre-wrap;
    max-height: 260px; overflow-y: auto;
  }
  .thinking-block.live { margin-bottom: 9px; border-left: 2px solid var(--color-purple); }

  .tool-list { display: flex; flex-direction: column; gap: 6px; margin-bottom: 9px; }
  .tool-item {
    border: 1px solid var(--color-border-light); border-radius: var(--radius-sm);
    background: var(--color-bg-hover); font-size: 12px;
  }
  .tool-item summary {
    display: flex; align-items: center; gap: 6px;
    padding: 6px 10px; cursor: pointer; color: var(--color-fg-2); font-weight: 600;
  }
  .tool-detail { padding: 0 10px 9px; }
  .tool-detail pre {
    margin: 3px 0 7px; padding: 7px; border-radius: 4px;
    background: var(--color-bg-card); font-size: 11px;
    font-family: var(--font-mono); white-space: pre-wrap;
    max-height: 160px; overflow-y: auto;
  }

  .attach-chip {
    display: inline-flex; align-items: center; gap: 5px;
    margin-top: 7px; margin-right: 6px; padding: 3px 9px;
    border-radius: 99px; background: var(--color-bg-hover);
    border: 1px solid var(--color-border-light);
    font-size: 11.5px; color: var(--color-fg-1);
  }
  .attach-size { color: var(--color-fg-3); }
  .attach-chip.pending { margin: 0; }

  .pending-row { display: flex; flex-wrap: wrap; gap: 6px; padding: 0 20px 8px; }

  .typing { display: inline-flex; gap: 4px; padding: 3px 0; }
  .typing i {
    width: 6px; height: 6px; border-radius: 50%;
    background: var(--color-fg-3); animation: blink 1.2s infinite;
  }
  .typing i:nth-child(2) { animation-delay: 0.2s; }
  .typing i:nth-child(3) { animation-delay: 0.4s; }
  @keyframes blink { 0%, 60%, 100% { opacity: 0.25; } 30% { opacity: 1; } }

  /* ── composer ── */
  .composer-wrap { padding: 12px 20px 18px; background: var(--color-bg-body); flex-shrink: 0; }
  .composer {
    display: flex; align-items: flex-end; gap: 8px;
    padding: 8px 10px; border-radius: var(--radius-lg);
    background: var(--color-bg-card);
    border: 1px solid var(--color-border-strong);
    box-shadow: var(--shadow-sm);
  }
  .composer:focus-within { border-color: var(--color-primary); }
  .composer textarea {
    flex: 1; border: none; outline: none; resize: none;
    background: transparent; color: var(--color-fg-0);
    font-family: var(--font-sans); font-size: 14px; line-height: 1.5;
    max-height: 180px; padding: 5px 0;
  }
  .attach-btn, .send-btn {
    display: grid; place-items: center; flex-shrink: 0;
    width: 32px; height: 32px; border-radius: var(--radius-sm);
    border: none; cursor: pointer; transition: var(--transition);
  }
  .attach-btn { background: transparent; color: var(--color-fg-2); }
  .attach-btn:hover:not(:disabled) { background: var(--color-bg-hover); color: var(--color-fg-0); }
  .send-btn { background: var(--color-primary); color: #fff; }
  .send-btn:hover:not(:disabled) { background: var(--color-primary-hover); }
  .send-btn:disabled, .attach-btn:disabled { opacity: 0.45; cursor: not-allowed; }

  .spin { animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .error-bar {
    display: flex; align-items: center; justify-content: space-between; gap: 10px;
    margin: 0 20px 8px; padding: 8px 12px; border-radius: var(--radius-sm);
    background: var(--color-error-light); color: var(--color-error); font-size: 12.5px;
  }

  .empty-state {
    flex: 1; display: flex; flex-direction: column;
    align-items: center; justify-content: center; gap: 14px;
    padding: 40px; text-align: center;
  }
  .empty-mark {
    display: grid; place-items: center; width: 76px; height: 76px;
    border-radius: 50%; background: var(--color-primary-light); color: var(--color-primary);
  }
  .empty-state h2 { margin: 0; font-size: 19px; color: var(--color-fg-0); }
  .empty-state p { margin: 0; max-width: 430px; font-size: 13.5px; line-height: 1.6; color: var(--color-fg-2); }

  @media (max-width: 860px) {
    .chat-shell { grid-template-columns: 1fr; }
    .conv-rail { display: none; }
  }
</style>

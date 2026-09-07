<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import {
    Send, Bot, User, Settings2, Thermometer, Hash,
    Copy, Trash2, ChevronDown, ChevronUp, Brain, Sparkles
  } from 'lucide-svelte';

  interface ChatMessage {
    role: 'user' | 'assistant' | 'system';
    content: string;
    reasoning?: string;
    timestamp: number;
    tokens?: number;
  }

  let messages = $state<ChatMessage[]>([]);
  let userInput = $state('');
  let streaming = $state(false);
  let streamAbort: AbortController | null = null;

  // Settings
  let settingsOpen = $state(false);
  let selectedModel = $state('gpt-4o');
  let temperature = $state(0.7);
  let systemPrompt = $state('You are a helpful assistant.');

  let availableModels = $state<string[]>([
    'gpt-4o', 'gpt-4o-mini', 'claude-3.5-sonnet', 'deepseek-chat', 'gemini-2.5-flash'
  ]);

  async function loadModelsAndCombos() {
    try {
      const [modelsRes, combosRes] = await Promise.all([
        api.get<any>('/v1/models').catch(() => null),
        api.get<any>('/api/combos').catch(() => null),
      ]);
      const set = new Set<string>();
      if (combosRes) {
        const cList = combosRes.data || combosRes.combos || (Array.isArray(combosRes) ? combosRes : []);
        for (const c of cList) {
          if (c.name) set.add(c.name);
          if (c.provider) set.add(c.provider);
        }
      }
      if (modelsRes?.data && Array.isArray(modelsRes.data)) {
        for (const m of modelsRes.data) {
          if (m.id) set.add(m.id);
        }
      }
      if (set.size > 0) {
        availableModels = Array.from(set).sort();
        if (!availableModels.includes(selectedModel)) {
          selectedModel = availableModels[0];
        }
      }
    } catch {}
  }

  onMount(loadModelsAndCombos);

  let totalTokens = $derived(
    messages.reduce((sum, m) => sum + (m.tokens || 0), 0)
  );

  let messageCount = $derived(
    messages.filter(m => m.role !== 'system').length
  );

  let chatContainer: HTMLDivElement | null = $state(null);

  function scrollToBottom() {
    if (chatContainer) {
      requestAnimationFrame(() => {
        chatContainer!.scrollTop = chatContainer!.scrollHeight;
      });
    }
  }

  function estimateTokens(text: string): number {
    return Math.ceil(text.length / 4);
  }

  function escapeHtml(text: string): string {
    return text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }

  function renderMessage(content: string): string {
    let html = escapeHtml(content);

    // Code blocks
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_: string, lang: string, code: string) => {
      return `<pre class="code-block"><code class="language-${lang}">${code.trim()}</code></pre>`;
    });

    // Inline code
    html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>');
    // Bold
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    // Italic
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>');
    // Line breaks
    html = html.replace(/\n/g, '<br>');

    return html;
  }

  async function sendMessage() {
    const content = userInput.trim();
    if (!content || streaming) return;

    const userMsg: ChatMessage = {
      role: 'user', content, timestamp: Date.now(), tokens: estimateTokens(content),
    };
    messages = [...messages, userMsg];
    userInput = '';
    streaming = true;
    scrollToBottom();

    const assistantMsg: ChatMessage = {
      role: 'assistant', content: '', timestamp: Date.now(), tokens: 0,
    };
    messages = [...messages, assistantMsg];

    try {
      streamAbort = new AbortController();

      const apiMessages: { role: string; content: string }[] = [];
      if (systemPrompt.trim()) {
        apiMessages.push({ role: 'system', content: systemPrompt.trim() });
      }
      for (const m of messages) {
        if (m !== assistantMsg) {
          apiMessages.push({ role: m.role, content: m.content });
        }
      }

      const res = await api.raw('/v1/chat/completions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: selectedModel,
          messages: apiMessages,
          temperature,
          stream: true,
        }),
        signal: streamAbort.signal,
      });

      if (!res.ok) {
        const errBody = await res.text();
        throw new Error(errBody || `HTTP ${res.status}`);
      }

      const reader = res.body?.getReader();
      if (!reader) throw new Error('No response stream');

      const decoder = new TextDecoder();
      let buffer = '';
      let fullContent = '';
      let fullReasoning = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed || !trimmed.startsWith('data: ')) continue;
          const data = trimmed.slice(6);
          if (data === '[DONE]') continue;

          try {
            const parsed = JSON.parse(data);
            const delta = parsed.choices?.[0]?.delta?.content || '';
            const rDelta = parsed.choices?.[0]?.delta?.reasoning_content || '';
            if (rDelta) {
              fullReasoning += rDelta;
            }
            if (delta) {
              fullContent += delta;
            }

            // Extract <think> if embedded inside content
            let displayContent = fullContent;
            let displayReasoning = fullReasoning;
            const thinkMatch = fullContent.match(/<think>([\s\S]*?)<\/think>/);
            if (thinkMatch) {
              if (!displayReasoning) displayReasoning = thinkMatch[1].trim();
              displayContent = fullContent.replace(/<think>[\s\S]*?<\/think>/, '').trim();
            }

            const lastIdx = messages.length - 1;
            messages[lastIdx] = {
              ...messages[lastIdx],
              content: displayContent,
              reasoning: displayReasoning,
              tokens: estimateTokens(displayContent + displayReasoning),
            };
            messages = [...messages];
            scrollToBottom();
          } catch {
            // Skip malformed JSON
          }
        }
      }

      const lastIdx = messages.length - 1;
      messages[lastIdx] = { ...messages[lastIdx], tokens: estimateTokens(fullContent) };
      messages = [...messages];
    } catch (e: any) {
      if (e.name === 'AbortError') return;
      const lastIdx = messages.length - 1;
      messages[lastIdx] = {
        ...messages[lastIdx],
        content: messages[lastIdx].content || `Error: ${e.message || 'Request failed'}`,
        tokens: 0,
      };
      messages = [...messages];
    } finally {
      streaming = false;
      streamAbort = null;
      scrollToBottom();
    }
  }

  function stopStreaming() {
    streamAbort?.abort();
  }

  function clearChat() {
    messages = [];
  }

  function copyMessage(content: string) {
    navigator.clipboard.writeText(content);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  }

  function formatTime(ts: number): string {
    return new Date(ts).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  onMount(() => { scrollToBottom(); });
</script>

<svelte:head>
  <title>Playground — Lintasan</title>
</svelte:head>

<div style="display: flex; flex-direction: column; height: calc(100vh - var(--header-h) - 48px);">
  <!-- Header bar -->
  <div class="flex items-center justify-between" style="margin-bottom: 16px;">
    <div>
      <h2 style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); letter-spacing: -0.3px;">
        Chat Playground
      </h2>
      <p style="font-size: 13px; color: var(--color-fg-2); margin-top: 2px;">
        Test models and prompts with streaming responses
      </p>
    </div>
    <div class="flex items-center gap-2">
      <!-- Token counter -->
      <div
        class="flex items-center gap-1.5 px-3 py-1.5 rounded-md"
        style="background: var(--color-bg-card); border: 1px solid var(--color-border); font-size: 12px; color: var(--color-fg-2);"
      >
        <Hash size={12} />
        <span style="font-family: var(--font-mono); color: var(--color-fg-1); font-weight: 500;">
          {totalTokens.toLocaleString()}
        </span>
        tokens · {messageCount} msgs
      </div>

      <button
        class="btn-secondary"
        onclick={() => settingsOpen = !settingsOpen}
        style="display: flex; align-items: center; gap: 6px;"
      >
        <Settings2 size={14} />
        Settings
        {#if settingsOpen}
          <ChevronUp size={12} />
        {:else}
          <ChevronDown size={12} />
        {/if}
      </button>

      <button
        class="btn-secondary"
        onclick={clearChat}
        style="display: flex; align-items: center; gap: 6px;"
      >
        <Trash2 size={14} />
        Clear
      </button>
    </div>
  </div>

  <!-- Settings panel -->
  {#if settingsOpen}
    <div class="card" style="padding: 16px 20px; margin-bottom: 16px; animation: fadeInUp 0.3s ease-out;">
      <div class="flex items-center gap-6 flex-wrap">
        <!-- Model selector -->
        <div style="flex: 1; min-width: 180px;">
          <label
            for="model-select"
            style="display: block; font-size: 11px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 6px;"
          >Model</label>
          <select
            id="model-select"
            class="input-field"
            style="font-size: 13px; padding: 7px 10px;"
            bind:value={selectedModel}
          >
            {#each availableModels as model}
              <option value={model}>{model}</option>
            {/each}
          </select>
        </div>

        <!-- Temperature -->
        <div style="flex: 1; min-width: 200px;">
          <div class="flex items-center justify-between" style="margin-bottom: 6px;">
            <span style="font-size: 11px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">
              <span class="flex items-center gap-1">
                <Thermometer size={12} /> Temperature
              </span>
            </span>
            <span style="font-size: 12px; font-family: var(--font-mono); color: var(--color-primary); font-weight: 600;">
              {temperature.toFixed(1)}
            </span>
          </div>
          <input type="range" min="0" max="2" step="0.1" bind:value={temperature} style="width: 100%; accent-color: var(--color-primary);" />
          <div class="flex justify-between" style="font-size: 10px; color: var(--color-fg-3); margin-top: 2px;">
            <span>Precise</span>
            <span>Creative</span>
          </div>
        </div>
      </div>

      <!-- System prompt -->
      <div style="margin-top: 14px;">
        <label
          for="system-prompt"
          style="display: block; font-size: 11px; font-weight: 600; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 6px;"
        >System Prompt</label>
        <textarea
          id="system-prompt"
          class="input-field"
          rows="2"
          bind:value={systemPrompt}
          placeholder="You are a helpful assistant..."
          style="font-size: 13px; resize: vertical;"
        ></textarea>
      </div>
    </div>
  {/if}

  <!-- Chat messages -->
  <div class="card" style="flex: 1; display: flex; flex-direction: column; padding: 0; overflow: hidden;">
    <div bind:this={chatContainer} style="flex: 1; overflow-y: auto; padding: 20px;">
      {#if messages.length === 0}
        <div class="flex flex-col items-center justify-center" style="height: 100%; opacity: 0.5;">
          <Bot size={48} style="color: var(--color-fg-3); margin-bottom: 12px;" stroke-width={1.2} />
          <div style="font-size: 14px; font-weight: 500; color: var(--color-fg-2);">Start a conversation</div>
          <div style="font-size: 13px; color: var(--color-fg-3); margin-top: 4px;">Type a message below to begin chatting</div>
        </div>
      {:else}
        <div style="display: flex; flex-direction: column; gap: 16px;">
          {#each messages as msg, i}
            <div
              class="message-row"
              class:user={msg.role === 'user'}
              class:assistant={msg.role === 'assistant'}
              style="animation: fadeInUp 0.3s ease-out;"
            >
              <div class="message-avatar" class:user-avatar={msg.role === 'user'} class:bot-avatar={msg.role === 'assistant'}>
                {#if msg.role === 'user'}
                  <User size={16} />
                {:else}
                  <Bot size={16} />
                {/if}
              </div>

              <div class="message-content">
                <div class="flex items-center gap-2" style="margin-bottom: 4px;">
                  <span style="font-size: 12px; font-weight: 600; color: var(--color-fg-0);">
                    {msg.role === 'user' ? 'You' : 'Assistant'}
                  </span>
                  <span style="font-size: 10px; color: var(--color-fg-3);">{formatTime(msg.timestamp)}</span>
                  {#if msg.tokens}
                    <span style="font-size: 10px; color: var(--color-fg-3); font-family: var(--font-mono);">
                      {msg.tokens} tokens
                    </span>
                  {/if}
                </div>

                <div class="message-bubble" class:user-bubble={msg.role === 'user'} class:assistant-bubble={msg.role === 'assistant'}>
                  {#if msg.reasoning}
                    <details class="reasoning-block" open={streaming && i === messages.length - 1}>
                      <summary class="reasoning-summary">
                        <Brain size={13} style="color: var(--color-purple);" />
                        <span>Thinking Process</span>
                      </summary>
                      <div class="reasoning-text">
                        {msg.reasoning}
                      </div>
                    </details>
                  {/if}

                  {#if msg.content}
                    {@html renderMessage(msg.content)}
                  {:else if streaming && i === messages.length - 1}
                    <span class="typing-indicator">
                      <span></span><span></span><span></span>
                    </span>
                  {/if}
                </div>

                {#if msg.content && msg.role === 'assistant'}
                  <button class="copy-btn" onclick={() => copyMessage(msg.content)} title="Copy message">
                    <Copy size={12} />
                  </button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Input area -->
    <div style="border-top: 1px solid var(--color-border); padding: 16px 20px; background: var(--color-bg-card);">
      <div class="flex items-end gap-3">
        <textarea
          class="input-field"
          rows="2"
          bind:value={userInput}
          onkeydown={handleKeydown}
          placeholder="Type your message... (Shift+Enter for new line)"
          disabled={streaming}
          style="resize: none; font-size: 13px;"
        ></textarea>
        {#if streaming}
          <button
            class="btn-secondary"
            onclick={stopStreaming}
            style="padding: 10px 16px; display: flex; align-items: center; gap: 6px; white-space: nowrap; flex-shrink: 0;"
          >Stop</button>
        {:else}
          <button
            class="btn-primary"
            onclick={sendMessage}
            disabled={!userInput.trim()}
            style="padding: 10px 16px; display: flex; align-items: center; gap: 6px; white-space: nowrap; flex-shrink: 0;"
          >
            <Send size={14} /> Send
          </button>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .message-row {
    display: flex;
    gap: 12px;
    max-width: 85%;
  }
  .message-row.user {
    align-self: flex-end;
    flex-direction: row-reverse;
  }
  .message-row.assistant {
    align-self: flex-start;
  }

  .message-avatar {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .user-avatar {
    background: var(--color-primary-light);
    color: var(--color-primary);
  }
  .bot-avatar {
    background: var(--color-purple-light);
    color: var(--color-purple);
  }

  .message-content {
    display: flex;
    flex-direction: column;
    min-width: 0;
    position: relative;
  }

  .message-bubble {
    padding: 12px 16px;
    border-radius: var(--radius);
    font-size: 13px;
    line-height: 1.6;
    word-wrap: break-word;
    overflow-wrap: break-word;
  }
  .user-bubble {
    background: var(--color-primary);
    color: #fff;
    border-bottom-right-radius: 4px;
  }
  .assistant-bubble {
    background: var(--color-bg-body);
    color: var(--color-fg-1);
    border: 1px solid var(--color-border);
    border-bottom-left-radius: 4px;
  }

  .message-bubble :global(.code-block) {
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 14px 16px;
    margin: 8px 0;
    overflow-x: auto;
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.6;
  }
  .message-bubble :global(.inline-code) {
    background: rgba(0, 0, 0, 0.06);
    padding: 2px 6px;
    border-radius: 4px;
    font-family: var(--font-mono);
    font-size: 12px;
  }
  .user-bubble :global(.code-block) {
    background: rgba(255, 255, 255, 0.15);
    border-color: rgba(255, 255, 255, 0.2);
    color: #fff;
  }
  .user-bubble :global(.inline-code) {
    background: rgba(255, 255, 255, 0.15);
    color: #fff;
  }

  .reasoning-block {
    margin-bottom: 10px;
    padding: 8px 12px;
    background: rgba(139, 92, 246, 0.06);
    border: 1px solid rgba(139, 92, 246, 0.2);
    border-radius: 8px;
    font-size: 12px;
  }
  .reasoning-summary {
    display: flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    font-weight: 600;
    color: var(--color-purple);
    user-select: none;
  }
  .reasoning-text {
    margin-top: 8px;
    color: var(--color-fg-2);
    font-size: 11px;
    line-height: 1.5;
    white-space: pre-wrap;
    font-family: var(--font-mono);
    max-height: 220px;
    overflow-y: auto;
  }

  .copy-btn {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    margin-top: 4px;
    border: none;
    background: transparent;
    color: var(--color-fg-3);
    font-size: 11px;
    cursor: pointer;
    border-radius: 4px;
    transition: var(--transition);
    opacity: 0;
  }
  .message-content:hover .copy-btn {
    opacity: 1;
  }
  .copy-btn:hover {
    background: var(--color-bg-body);
    color: var(--color-fg-1);
  }

  .typing-indicator {
    display: inline-flex;
    gap: 4px;
    align-items: center;
  }
  .typing-indicator span {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-fg-3);
    animation: typing 1.4s infinite ease-in-out;
  }
  .typing-indicator span:nth-child(2) { animation-delay: 0.2s; }
  .typing-indicator span:nth-child(3) { animation-delay: 0.4s; }

  @keyframes typing {
    0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
    30% { transform: translateY(-4px); opacity: 1; }
  }
</style>

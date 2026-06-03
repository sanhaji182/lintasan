<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { Brain, Search, Plus, Trash2, X, Tag, Database } from 'lucide-svelte/icons';

  interface MemoryItem {
    key: string;
    text: string;
    metadata?: Record<string, string>;
    tags?: string[];
    score: number;
    hits: number;
    similarity?: number;
    created_at: string;
  }

  interface MemoryStats {
    total_memories: number;
    available: boolean;
    backend: string;
    avg_score: number;
  }

  let memories = $state<MemoryItem[]>([]);
  let stats = $state<MemoryStats>({ total_memories: 0, available: false, backend: 'none', avg_score: 0 });
  let loading = $state(true);
  let searchQuery = $state('');
  let searching = $state(false);
  let showForm = $state(false);
  let newText = $state('');
  let newTags = $state('');
  let total = $state(0);
  let offset = $state(0);
  const limit = 20;

  onMount(async () => {
    await Promise.all([loadMemories(), loadStats()]);
    loading = false;
  });

  async function loadMemories() {
    try {
      const res = await api.get<any>(`/v1/memory?limit=${limit}&offset=${offset}`);
      memories = res.memories || [];
      total = res.total || 0;
    } catch { memories = []; }
  }

  async function loadStats() {
    try {
      stats = await api.get<MemoryStats>('/v1/memory/stats');
    } catch {}
  }

  async function searchMemories() {
    if (!searchQuery.trim()) {
      await loadMemories();
      return;
    }
    searching = true;
    try {
      const res = await api.get<any>(`/v1/memory/search?q=${encodeURIComponent(searchQuery)}&top_k=20`);
      memories = res.results || [];
      total = res.count || 0;
    } catch { memories = []; }
    finally { searching = false; }
  }

  async function storeMemory() {
    if (!newText.trim()) return;
    const tags = newTags.split(',').map(t => t.trim()).filter(Boolean);
    try {
      await api.post('/v1/memory', { text: newText, tags });
      newText = '';
      newTags = '';
      showForm = false;
      await Promise.all([loadMemories(), loadStats()]);
    } catch {}
  }

  async function deleteMemory(key: string) {
    if (!confirm('Delete this memory?')) return;
    try {
      await api.delete(`/v1/memory/${key}`);
      memories = memories.filter(m => m.key !== key);
      await loadStats();
    } catch {}
  }

  function formatDate(dateStr: string): string {
    if (!dateStr) return '—';
    const d = new Date(dateStr);
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  function truncate(text: string, max = 120): string {
    if (text.length <= max) return text;
    return text.slice(0, max) + '…';
  }

  async function nextPage() {
    offset += limit;
    await loadMemories();
  }

  async function prevPage() {
    offset = Math.max(0, offset - limit);
    await loadMemories();
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Stats strip -->
  <div class="card mb-5" style="padding: 0; overflow: hidden;">
    <div class="grid grid-cols-3" style="gap: 1px; background: var(--color-border);">
      {#each [
        { label: 'TOTAL MEMORIES', value: stats.total_memories },
        { label: 'BACKEND', value: stats.backend?.toUpperCase() || 'NONE' },
        { label: 'AVG SCORE', value: stats.avg_score?.toFixed(1) || '0' }
      ] as stat}
        <div class="text-center" style="padding: 16px; background: var(--color-bg-card);">
          <div style="font-size: 11px; font-weight: 500; color: var(--color-fg-3); text-transform: uppercase; letter-spacing: 0.5px;">{stat.label}</div>
          <div style="font-size: 20px; font-weight: 700; color: var(--color-fg-0); font-family: var(--font-mono);">{stat.value}</div>
        </div>
      {/each}
    </div>
  </div>

  <!-- Search + Actions -->
  <div class="flex items-center justify-between mb-5 gap-3">
    <div class="flex items-center gap-2" style="flex: 1;">
      <div style="position: relative; flex: 1;">
        <Search size={16} style="position: absolute; left: 12px; top: 50%; transform: translateY(-50%); color: var(--color-fg-3);" />
        <input
          class="input-field"
          style="padding-left: 36px; width: 100%;"
          bind:value={searchQuery}
          placeholder="Search memories..."
          onkeydown={(e) => e.key === 'Enter' && searchMemories()}
        />
      </div>
      <button class="btn-secondary" onclick={searchMemories} disabled={searching}>
        {searching ? 'Searching...' : 'Search'}
      </button>
    </div>
    <button class="btn-primary flex items-center gap-2" onclick={() => showForm = !showForm}>
      {#if showForm}<X size={16} />{:else}<Plus size={16} />{/if}
      {showForm ? 'Cancel' : 'Store'}
    </button>
  </div>

  <!-- Store form -->
  {#if showForm}
    <div class="card mb-5" style="animation: fadeInScale 0.3s ease-out;">
      <h3 style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px;">Store New Memory</h3>
      <textarea
        class="input-field"
        style="width: 100%; min-height: 80px; resize: vertical; margin-bottom: 12px;"
        bind:value={newText}
        placeholder="Memory text..."
      ></textarea>
      <div class="flex items-center gap-3">
        <input
          class="input-field"
          style="flex: 1;"
          bind:value={newTags}
          placeholder="Tags (comma-separated)..."
        />
        <button class="btn-primary" onclick={storeMemory}>Save</button>
      </div>
    </div>
  {/if}

  <!-- Loading -->
  {#if loading}
    <div class="flex justify-center" style="padding: 60px 0;">
      <Spinner />
    </div>
  {:else if !stats.available}
    <EmptyState icon={Database} title="Memory Unavailable" description="No memory backend connected. Start the server with Redis or SQLite will be used automatically." />
  {:else if memories.length === 0}
    <EmptyState icon={Brain} title="No Memories" description="Store your first memory or let auto-indexing capture completions." />
  {:else}
    <!-- Memory list -->
    <div class="flex flex-col gap-3">
      {#each memories as mem}
        <div class="card" style="padding: 16px;">
          <div class="flex items-start justify-between gap-3">
            <div style="flex: 1; min-width: 0;">
              <p style="font-size: 13px; color: var(--color-fg-1); line-height: 1.5; word-break: break-word;">
                {truncate(mem.text, 200)}
              </p>
              <div class="flex items-center gap-3 mt-2" style="flex-wrap: wrap;">
                {#if mem.tags && mem.tags.length > 0}
                  {#each mem.tags as tag}
                    <span class="flex items-center gap-1" style="font-size: 11px; padding: 2px 8px; border-radius: 4px; background: var(--color-bg-hover); color: var(--color-fg-2);">
                      <Tag size={10} />{tag}
                    </span>
                  {/each}
                {/if}
                <span style="font-size: 11px; color: var(--color-fg-3);">
                  {formatDate(mem.created_at)}
                </span>
                {#if mem.similarity && mem.similarity < 1}
                  <span style="font-size: 11px; color: var(--color-primary); font-family: var(--font-mono);">
                    sim: {mem.similarity.toFixed(3)}
                  </span>
                {/if}
                {#if mem.hits > 0}
                  <span style="font-size: 11px; color: var(--color-fg-3); font-family: var(--font-mono);">
                    hits: {mem.hits}
                  </span>
                {/if}
              </div>
            </div>
            <button
              class="btn-icon"
              style="color: var(--color-danger); flex-shrink: 0;"
              onclick={() => deleteMemory(mem.key)}
              title="Delete"
            >
              <Trash2 size={16} />
            </button>
          </div>
        </div>
      {/each}
    </div>

    <!-- Pagination -->
    {#if total > limit}
      <div class="flex items-center justify-between mt-5">
        <button class="btn-secondary" onclick={prevPage} disabled={offset === 0}>← Previous</button>
        <span style="font-size: 12px; color: var(--color-fg-3);">
          {offset + 1}–{Math.min(offset + limit, total)} of {total}
        </span>
        <button class="btn-secondary" onclick={nextPage} disabled={offset + limit >= total}>Next →</button>
      </div>
    {/if}
  {/if}
</div>

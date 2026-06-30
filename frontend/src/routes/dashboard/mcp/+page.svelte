<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { Plug, Play, Link } from 'lucide-svelte/icons';
  import { showToast } from '$lib/toast';

  let tools = $state<any[]>([]);
  let loading = $state(true);
  let error = $state('');
  let testResult = $state('');
  let selectedTool = $state('');
  let testInput = $state('{}');
  let running = $state(false);

  async function loadTools() {
    loading = true;
    error = '';
    try {
      const data: any = await api.get('/api/mcp/tools');
      tools = data.tools || [];
    } catch (e: any) {
      console.error('Failed to load MCP tools:', e);
      error = 'Failed to load MCP tools. Please try again.';
      showToast('Failed to load MCP tools', 'error');
    } finally {
      loading = false;
    }
  }

  onMount(loadTools);

  async function testTool() {
    if (!selectedTool) return;
    running = true;
    testResult = '';
    try {
      const res = await api.raw('/mcp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: {
            name: selectedTool,
            arguments: JSON.parse(testInput)
          }
        })
      });
      const data = await res.json();
      testResult = JSON.stringify(data, null, 2);
    } catch (e: any) {
      testResult = `Error: ${e.message}`;
      showToast(`Tool execution failed: ${e.message}`, 'error');
    } finally {
      running = false;
    }
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 4px;">🔌 MCP Server</h2>
  <p style="font-size: 13px; color: var(--color-fg-3); margin-bottom: 20px;">Model Context Protocol — {tools.length} tools exposed for AI agents</p>

  {#if loading}
    <div role="status" aria-label="Loading MCP tools">
      <Spinner />
    </div>
  {:else if error}
    <div class="card">
      <EmptyState icon={Plug} title="Failed to load tools" description={error} action={loadTools} actionLabel="Retry" />
    </div>
  {:else}
    <div class="grid grid-cols-1 lg:grid-cols-2" style="gap: 20px;">
      <!-- Tools List -->
      <div class="card" style="padding: 16px;">
        <h3 style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px;">📋 Registered Tools ({tools.length})</h3>
        {#if tools.length === 0}
          <EmptyState icon={Plug} title="No tools registered" description="No MCP tools are currently available." />
        {:else}
          <div style="max-height: 384px; overflow-y: auto; display: flex; flex-direction: column; gap: 8px;">
            {#each tools as tool}
              <button
                style="
                  width: 100%;
                  text-align: left;
                  padding: 10px 12px;
                  border-radius: var(--radius-sm);
                  background: {selectedTool === tool.name ? 'var(--color-primary-light, #eef2ff)' : 'var(--color-bg-body)'};
                  border: 1px solid {selectedTool === tool.name ? 'var(--color-primary)' : 'var(--color-border)'};
                  cursor: pointer;
                  transition: var(--transition);
                "
                onclick={() => selectedTool = tool.name}
              >
                <div style="font-family: var(--font-mono); font-size: 13px; color: var(--color-primary); font-weight: 500;">{tool.name}</div>
                <div style="font-size: 12px; color: var(--color-fg-3); margin-top: 4px;">{tool.description}</div>
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Test Panel -->
      <div class="card" style="padding: 16px;">
        <h3 style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px;">🧪 Test Tool</h3>
        <div style="margin-bottom: 12px;">
          <label for="mcp-selected-tool" style="display: block; font-size: 12px; font-weight: 500; color: var(--color-fg-3); margin-bottom: 6px;">Selected Tool</label>
          <input
            id="mcp-selected-tool"
            type="text"
            bind:value={selectedTool}
            class="input-field"
            style="font-family: var(--font-mono); width: 100%;"
            readonly
            placeholder="Click a tool from the list..."
          />
        </div>
        <div style="margin-bottom: 12px;">
          <label for="mcp-tool-args" style="display: block; font-size: 12px; font-weight: 500; color: var(--color-fg-3); margin-bottom: 6px;">Arguments (JSON)</label>
          <textarea
            id="mcp-tool-args"
            bind:value={testInput}
            class="input-field"
            style="font-family: var(--font-mono); width: 100%; min-height: 96px; resize: vertical;"
          ></textarea>
        </div>
        <button
          onclick={testTool}
          class="btn-primary flex items-center gap-2"
          disabled={!selectedTool || running}
        >
          {#if running}
            <Spinner />
            Running...
          {:else}
            <Play size={14} />
            Run Tool
          {/if}
        </button>

        {#if testResult}
          <div style="margin-top: 16px;">
            <p style="font-size: 12px; font-weight: 500; color: var(--color-fg-3); margin-bottom: 6px;">Result</p>
            <pre style="
              padding: 12px;
              background: var(--color-bg-body);
              border: 1px solid var(--color-border);
              border-radius: var(--radius-sm);
              font-family: var(--font-mono);
              font-size: 12px;
              color: var(--color-fg-1);
              overflow: auto;
              max-height: 256px;
              white-space: pre-wrap;
              word-break: break-word;
            ">{testResult}</pre>
          </div>
        {/if}
      </div>
    </div>

    <!-- Connection Info -->
    <div class="card" style="padding: 16px; margin-top: 20px;">
      <h3 style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px;">
        <Link size={16} style="display: inline; vertical-align: text-bottom; margin-right: 4px;" />
        Connection Info
      </h3>
      <div class="grid grid-cols-2" style="gap: 12px; font-size: 13px;">
        <div>
          <span style="color: var(--color-fg-3);">HTTP Endpoint:</span>
          <code style="margin-left: 8px; font-family: var(--font-mono); color: var(--color-success); background: var(--color-success-light, #ecfdf5); padding: 2px 6px; border-radius: var(--radius-sm); font-size: 12px;">POST /mcp</code>
        </div>
        <div>
          <span style="color: var(--color-fg-3);">SSE Endpoint:</span>
          <code style="margin-left: 8px; font-family: var(--font-mono); color: var(--color-success); background: var(--color-success-light, #ecfdf5); padding: 2px 6px; border-radius: var(--radius-sm); font-size: 12px;">GET /mcp/sse</code>
        </div>
        <div>
          <span style="color: var(--color-fg-3);">Protocol:</span>
          <code style="margin-left: 8px; font-family: var(--font-mono); color: var(--color-success); background: var(--color-success-light, #ecfdf5); padding: 2px 6px; border-radius: var(--radius-sm); font-size: 12px;">JSON-RPC 2.0</code>
        </div>
        <div>
          <span style="color: var(--color-fg-3);">Version:</span>
          <code style="margin-left: 8px; font-family: var(--font-mono); color: var(--color-success); background: var(--color-success-light, #ecfdf5); padding: 2px 6px; border-radius: var(--radius-sm); font-size: 12px;">2024-11-05</code>
        </div>
      </div>
    </div>
  {/if}
</div>

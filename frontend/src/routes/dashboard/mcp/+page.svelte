<script>
  import { onMount } from 'svelte';
  let tools = [];
  let loading = true;
  let testResult = '';
  let selectedTool = '';
  let testInput = '{}';

  onMount(async () => {
    try {
      const res = await fetch('/api/mcp/tools');
      const data = await res.json();
      tools = data.tools || [];
    } catch (e) {
      console.error('Failed to load MCP tools:', e);
    }
    loading = false;
  });

  async function testTool() {
    if (!selectedTool) return;
    try {
      const res = await fetch('/mcp', {
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
    } catch (e) {
      testResult = `Error: ${e.message}`;
    }
  }
</script>

<div class="p-6 bg-gray-900 min-h-screen text-white">
  <h1 class="text-2xl font-bold mb-6">🔌 MCP Server</h1>
  <p class="text-gray-400 mb-6">Model Context Protocol — 14 tools exposed for AI agents</p>

  {#if loading}
    <div class="text-center py-8">Loading tools...</div>
  {:else}
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Tools List -->
      <div class="bg-gray-800 rounded-lg p-4">
        <h2 class="text-lg font-semibold mb-4">📋 Registered Tools ({tools.length})</h2>
        <div class="space-y-2 max-h-96 overflow-y-auto">
          {#each tools as tool}
            <button
              class="w-full text-left p-3 rounded bg-gray-700 hover:bg-gray-600 transition {selectedTool === tool.name ? 'ring-2 ring-blue-500' : ''}"
              on:click={() => selectedTool = tool.name}
            >
              <div class="font-mono text-sm text-blue-400">{tool.name}</div>
              <div class="text-gray-400 text-xs mt-1">{tool.description}</div>
            </button>
          {/each}
        </div>
      </div>

      <!-- Test Panel -->
      <div class="bg-gray-800 rounded-lg p-4">
        <h2 class="text-lg font-semibold mb-4">🧪 Test Tool</h2>
        <div class="mb-4">
          <label class="block text-sm text-gray-400 mb-2">Selected Tool</label>
          <input
            type="text"
            bind:value={selectedTool}
            class="w-full p-2 bg-gray-700 rounded text-white font-mono"
            readonly
          />
        </div>
        <div class="mb-4">
          <label class="block text-sm text-gray-400 mb-2">Arguments (JSON)</label>
          <textarea
            bind:value={testInput}
            class="w-full p-2 bg-gray-700 rounded text-white font-mono h-24"
          ></textarea>
        </div>
        <button
          on:click={testTool}
          class="px-4 py-2 bg-blue-600 rounded hover:bg-blue-500 transition"
          disabled={!selectedTool}
        >
          ▶️ Run Tool
        </button>

        {#if testResult}
          <div class="mt-4">
            <label class="block text-sm text-gray-400 mb-2">Result</label>
            <pre class="p-3 bg-gray-900 rounded text-sm overflow-auto max-h-64">{testResult}</pre>
          </div>
        {/if}
      </div>
    </div>

    <!-- SSE Info -->
    <div class="mt-6 bg-gray-800 rounded-lg p-4">
      <h2 class="text-lg font-semibold mb-2">🔗 Connection Info</h2>
      <div class="grid grid-cols-2 gap-4 text-sm">
        <div>
          <span class="text-gray-400">HTTP Endpoint:</span>
          <code class="ml-2 text-green-400">POST /mcp</code>
        </div>
        <div>
          <span class="text-gray-400">SSE Endpoint:</span>
          <code class="ml-2 text-green-400">GET /mcp/sse</code>
        </div>
        <div>
          <span class="text-gray-400">Protocol:</span>
          <code class="ml-2 text-green-400">JSON-RPC 2.0</code>
        </div>
        <div>
          <span class="text-gray-400">Version:</span>
          <code class="ml-2 text-green-400">2024-11-05</code>
        </div>
      </div>
    </div>
  {/if}
</div>

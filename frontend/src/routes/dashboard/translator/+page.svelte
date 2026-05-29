<script>
  import { onMount } from 'svelte';
  let formats = [];
  let sourceFormat = 'openai';
  let targetFormat = 'anthropic';
  let inputJson = JSON.stringify({
    model: 'gpt-4',
    messages: [{ role: 'user', content: 'Hello!' }]
  }, null, 2);
  let outputJson = '';
  let loading = true;
  let translating = false;

  onMount(async () => {
    try {
      const res = await fetch('/api/translate/formats');
      const data = await res.json();
      formats = data.formats || [];
    } catch (e) {
      console.error('Failed to load formats:', e);
    }
    loading = false;
  });

  async function translate() {
    translating = true;
    try {
      const res = await fetch(`/api/translate?to=${targetFormat}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: inputJson
      });
      const data = await res.json();
      outputJson = JSON.stringify(data, null, 2);
    } catch (e) {
      outputJson = `Error: ${e.message}`;
    }
    translating = false;
  }

  function swapFormats() {
    [sourceFormat, targetFormat] = [targetFormat, sourceFormat];
    [inputJson, outputJson] = [outputJson, inputJson];
  }
</script>

<div class="p-6 bg-gray-900 min-h-screen text-white">
  <h1 class="text-2xl font-bold mb-6">🔄 Format Translator</h1>
  <p class="text-gray-400 mb-6">Convert between AI API formats (OpenAI, Anthropic, Gemini, Cohere, Mistral)</p>

  {#if loading}
    <div class="text-center py-8">Loading formats...</div>
  {:else}
    <!-- Format Selection -->
    <div class="flex items-center gap-4 mb-6">
      <select bind:value={sourceFormat} class="p-2 bg-gray-800 rounded text-white">
        {#each formats as fmt}
          <option value={fmt}>{fmt}</option>
        {/each}
      </select>

      <button on:click={swapFormats} class="p-2 bg-gray-700 rounded hover:bg-gray-600">
        ⇄
      </button>

      <select bind:value={targetFormat} class="p-2 bg-gray-800 rounded text-white">
        {#each formats as fmt}
          <option value={fmt}>{fmt}</option>
        {/each}
      </select>

      <button
        on:click={translate}
        class="px-4 py-2 bg-blue-600 rounded hover:bg-blue-500 transition"
        disabled={translating}
      >
        {translating ? 'Translating...' : '🔄 Translate'}
      </button>
    </div>

    <!-- Input/Output -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <div>
        <h2 class="text-lg font-semibold mb-2">📥 Input ({sourceFormat})</h2>
        <textarea
          bind:value={inputJson}
          class="w-full h-96 p-4 bg-gray-800 rounded font-mono text-sm"
          placeholder="Paste JSON here..."
        ></textarea>
      </div>

      <div>
        <h2 class="text-lg font-semibold mb-2">📤 Output ({targetFormat})</h2>
        <textarea
          bind:value={outputJson}
          class="w-full h-96 p-4 bg-gray-800 rounded font-mono text-sm"
          readonly
          placeholder="Translated output will appear here..."
        ></textarea>
      </div>
    </div>

    <!-- Supported Conversions -->
    <div class="mt-6 bg-gray-800 rounded-lg p-4">
      <h2 class="text-lg font-semibold mb-4">📋 Supported Conversions</h2>
      <div class="grid grid-cols-5 gap-2 text-center text-sm">
        <div></div>
        {#each formats as fmt}
          <div class="font-semibold text-gray-300">{fmt}</div>
        {/each}
        {#each formats as from}
          <div class="font-semibold text-gray-300 text-right">{from}</div>
          {#each formats as to}
            <div class="p-2 rounded {from === to ? 'bg-gray-600' : 'bg-green-900 text-green-300'}">
              {from === to ? '—' : '✓'}
            </div>
          {/each}
        {/each}
      </div>
    </div>
  {/if}
</div>

<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import { showToast } from '$lib/toast';
  import { ArrowRightLeft, Play, FileInput, FileOutput, AlertCircle } from 'lucide-svelte/icons';

  let formats = $state<string[]>([]);
  let sourceFormat = $state('openai');
  let targetFormat = $state('anthropic');
  let inputJson = $state(JSON.stringify({
    model: 'gpt-4',
    messages: [{ role: 'user', content: 'Hello!' }]
  }, null, 2));
  let outputJson = $state('');
  let loading = $state(true);
  let translating = $state(false);
  let error = $state('');

  async function loadFormats() {
    loading = true;
    error = '';
    try {
      const data: any = await api.get('/api/translate/formats');
      formats = data.formats || [];
    } catch (e: any) {
      error = e.message || 'Failed to load formats';
    }
    loading = false;
  }

  onMount(loadFormats);

  async function translate() {
    translating = true;
    try {
      const res = await api.raw(`/api/translate?to=${targetFormat}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: inputJson
      });
      const data = await res.json();
      outputJson = JSON.stringify(data, null, 2);
    } catch (e: any) {
      outputJson = `Error: ${e.message}`;
      showToast('Translation failed: ' + e.message, 'error');
    }
    translating = false;
  }

  function swapFormats() {
    const tmp = sourceFormat;
    sourceFormat = targetFormat;
    targetFormat = tmp;
    const tmpJson = inputJson;
    inputJson = outputJson;
    outputJson = tmpJson;
  }
</script>

<div style="animation: fadeInUp 0.4s ease-out;">
  <h2 style="font-size: 18px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 4px;">Format Translator</h2>
  <p style="font-size: 13px; color: var(--color-fg-3); margin-bottom: 20px;">Convert between AI API formats (OpenAI, Anthropic, Gemini, Cohere, Mistral)</p>

  {#if loading}
    <div role="status" aria-label="Loading formats">
      <Spinner />
    </div>
  {:else if error}
    <EmptyState icon={AlertCircle} title="Failed to load formats" description={error} action={loadFormats} actionLabel="Retry" />
  {:else}
    <!-- Format Selection -->
    <div class="card mb-5">
      <div class="flex items-center gap-3 flex-wrap">
        <select bind:value={sourceFormat} class="input-field" style="width: auto; min-width: 140px;">
          {#each formats as fmt}
            <option value={fmt}>{fmt}</option>
          {/each}
        </select>

        <button onclick={swapFormats} class="btn-secondary" style="padding: 9px 12px;" aria-label="Swap formats">
          <ArrowRightLeft size={16} />
        </button>

        <select bind:value={targetFormat} class="input-field" style="width: auto; min-width: 140px;">
          {#each formats as fmt}
            <option value={fmt}>{fmt}</option>
          {/each}
        </select>

        <button
          onclick={translate}
          class="btn-primary flex items-center gap-2"
          disabled={translating}
        >
          {#if translating}
            <span style="animation: spin 0.8s linear infinite; display: inline-block;">⏳</span> Translating...
          {:else}
            <Play size={14} /> Translate
          {/if}
        </button>
      </div>
    </div>

    <!-- Input/Output -->
    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-bottom: 20px;">
      <div class="card" style="padding: 16px;">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px; display: flex; align-items: center; gap: 6px;">
          <FileInput size={16} style="color: var(--color-primary);" />
          Input ({sourceFormat})
        </div>
        <textarea
          bind:value={inputJson}
          class="input-field"
          style="height: 320px; resize: vertical; font-family: var(--font-mono); font-size: 13px; line-height: 1.5;"
          placeholder="Paste JSON here..."
        ></textarea>
      </div>

      <div class="card" style="padding: 16px;">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 12px; display: flex; align-items: center; gap: 6px;">
          <FileOutput size={16} style="color: var(--color-success);" />
          Output ({targetFormat})
        </div>
        <textarea
          bind:value={outputJson}
          class="input-field"
          style="height: 320px; resize: vertical; font-family: var(--font-mono); font-size: 13px; line-height: 1.5;"
          readonly
          placeholder="Translated output will appear here..."
        ></textarea>
      </div>
    </div>

    <!-- Supported Conversions -->
    <div class="card">
      <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0); margin-bottom: 16px;">Supported Conversions</div>
      <div style="overflow-x: auto;">
        <table class="data-table">
          <thead>
            <tr>
              <th style="min-width: 80px;">From \\ To</th>
              {#each formats as fmt}
                <th style="text-align: center;">{fmt}</th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each formats as from}
              <tr>
                <td style="font-weight: 600; color: var(--color-fg-1);">{from}</td>
                {#each formats as to}
                  <td style="text-align: center;">
                    {#if from === to}
                      <span style="color: var(--color-fg-3);">—</span>
                    {:else}
                      <span style="color: var(--color-success); font-weight: 600;">✓</span>
                    {/if}
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>

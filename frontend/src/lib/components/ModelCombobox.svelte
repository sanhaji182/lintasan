<script lang="ts">
  import { Search, ChevronDown, Check, Cloud, Route, Server } from 'lucide-svelte';
  import { filterCallableModels, groupCallableModels, type CallableModel } from '$lib/workflow-consolidation';

  let { models, selected, recent = [], recommended = null, onselect } = $props<{
    models: CallableModel[]; selected: string; recent?: string[]; recommended?: string | null; onselect: (id: string) => void;
  }>();
  let open = $state(false), query = $state(''), activeIndex = $state(0);
  const current = $derived(models.find((model: CallableModel) => model.id === selected));
  const groups = $derived(groupCallableModels(filterCallableModels(models, query), recent, recommended));
  const flat = $derived(groups.flatMap(group => group.items));

  function choose(model: CallableModel) { onselect(model.id); query = ''; open = false; }
  function keydown(event: KeyboardEvent) {
    if (!open && ['ArrowDown', 'Enter', ' '].includes(event.key)) { event.preventDefault(); open = true; return; }
    if (!open) return;
    if (event.key === 'ArrowDown') { event.preventDefault(); activeIndex = Math.min(activeIndex + 1, flat.length - 1); }
    if (event.key === 'ArrowUp') { event.preventDefault(); activeIndex = Math.max(activeIndex - 1, 0); }
    if (event.key === 'Enter' && flat[activeIndex]) { event.preventDefault(); choose(flat[activeIndex]); }
    if (event.key === 'Escape') { event.preventDefault(); open = false; }
  }
</script>

<div class="picker">
  <button type="button" class="trigger" aria-haspopup="listbox" aria-expanded={open} onkeydown={keydown} onclick={() => open = !open}>
    <span><code>{current?.id || selected || 'Choose a model'}</code>{#if current}<small>{current.account || current.provider || current.kind.replace('_', ' ')}</small>{/if}</span><ChevronDown size={15} />
  </button>
  {#if open}
    <div class="popover">
      <label class="search"><Search size={14} /><span class="sr-only">Search models</span><input role="combobox" aria-expanded="true" aria-controls="model-options" aria-activedescendant={flat[activeIndex] ? `model-${activeIndex}` : undefined} bind:value={query} onkeydown={keydown} oninput={() => activeIndex = 0} placeholder="Search callable ID, provider, account…" /></label>
      <div class="options" id="model-options" role="listbox" aria-label="Callable models">
        {#each groups as group}
          <div class="group" role="group" aria-label={group.label}><div class="group-label">{group.label}</div>
            {#each group.items as model}
              {@const index = flat.findIndex(item => item.id === model.id)}
              <button id={`model-${index}`} type="button" role="option" aria-selected={model.id === selected} class:active={index === activeIndex} onclick={() => choose(model)} onmouseenter={() => activeIndex = index}>
                <span class="kind">{#if model.kind === 'route'}<Route size={14} />{:else if model.kind === 'cloud_agent'}<Cloud size={14} />{:else}<Server size={14} />{/if}</span>
                <span class="copy"><code>{model.id}</code><small>{[model.account, model.provider, model.health !== 'unknown' ? model.health : null].filter(Boolean).join(' · ') || 'Metadata not reported'}</small></span>
                {#if model.id === selected}<Check size={14} />{/if}
              </button>
            {/each}
          </div>
        {/each}
        {#if flat.length === 0}<div class="empty">No callable models match “{query}”.</div>{/if}
      </div>
    </div>
  {/if}
</div>

<style>
.picker{position:relative}.trigger{width:100%;display:flex;justify-content:space-between;align-items:center;text-align:left;padding:8px 10px;border:1px solid var(--color-border);border-radius:8px;background:var(--color-bg-card);color:var(--color-fg-1);cursor:pointer}.trigger span,.copy{display:flex;flex-direction:column;min-width:0}.trigger code,.copy code{font:12px var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.trigger small,.copy small{font-size:10px;color:var(--color-fg-3);margin-top:2px}.popover{position:absolute;z-index:50;top:calc(100% + 5px);left:0;right:0;min-width:min(520px,90vw);background:var(--color-bg-card);border:1px solid var(--color-border);border-radius:11px;box-shadow:var(--shadow-lg);overflow:hidden}.search{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid var(--color-border)}.search input{width:100%;border:0;outline:0;background:transparent;color:var(--color-fg-0);font-size:12px}.options{max-height:330px;overflow:auto;padding:6px}.group-label{font-size:9px;font-weight:800;letter-spacing:.08em;text-transform:uppercase;color:var(--color-fg-3);padding:8px 9px 4px}.group button{width:100%;border:0;background:transparent;color:var(--color-fg-1);display:flex;align-items:center;gap:9px;padding:8px 9px;border-radius:7px;text-align:left;cursor:pointer}.group button:hover,.group button.active{background:var(--color-primary-light)}.kind{color:var(--color-primary);display:grid;place-items:center}.copy{flex:1}.empty{padding:20px;text-align:center;font-size:12px;color:var(--color-fg-3)}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}
</style>

<script lang="ts">
  import { Plus, X, ArrowUp, ArrowDown } from 'lucide-svelte/icons';

  type Option = { value: string; label?: string };

  let {
    items = $bindable<string[]>([]),
    mode = 'text',
    options = [],
    placeholder = 'Model ID',
    listId = '',
    addLabel = 'Add step',
  }: {
    items?: string[];
    mode?: 'text' | 'select';
    options?: Option[];
    placeholder?: string;
    listId?: string;
    addLabel?: string;
  } = $props();

  function addStep() {
    items = [...items, ''];
  }

  function removeStep(index: number) {
    items = items.filter((_, itemIndex) => itemIndex !== index);
  }

  function moveStep(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= items.length) return;
    const next = [...items];
    [next[index], next[target]] = [next[target], next[index]];
    items = next;
  }

  function updateStep(index: number, value: string) {
    items = items.map((item, itemIndex) => itemIndex === index ? value : item);
  }
</script>

<div class="chain-builder">
  <ol class="chain-list">
    {#each items as item, i (i)}
      <li class="chain-row">
        <span class="step-num">{i + 1}</span>
        <span class="step-role">{i === 0 ? 'primary' : `fallback ${i}`}</span>
        {#if mode === 'select'}
          <select
            class="input-field chain-input"
            value={item}
            aria-label={i === 0 ? 'Primary connection' : `Fallback ${i} connection`}
            onchange={(event) => updateStep(i, event.currentTarget.value)}
          >
            <option value="">Select a connection…</option>
            {#each options as option}
              <option value={option.value}>{option.label || option.value}</option>
            {/each}
            {#if item && !options.some(option => option.value === item)}
              <option value={item}>{item} (not found)</option>
            {/if}
          </select>
        {:else}
          <input
            class="input-field chain-input"
            {placeholder}
            list={listId || undefined}
            value={item}
            aria-label={i === 0 ? 'Primary model' : `Fallback ${i} model`}
            oninput={(event) => updateStep(i, event.currentTarget.value)}
          />
        {/if}
        <button type="button" class="btn-icon" aria-label={`Move step ${i + 1} up`} disabled={i === 0} onclick={() => moveStep(i, -1)}>
          <ArrowUp size={14} />
        </button>
        <button type="button" class="btn-icon" aria-label={`Move step ${i + 1} down`} disabled={i === items.length - 1} onclick={() => moveStep(i, 1)}>
          <ArrowDown size={14} />
        </button>
        <button type="button" class="btn-icon remove" aria-label={`Remove step ${i + 1}`} onclick={() => removeStep(i)}>
          <X size={14} />
        </button>
      </li>
    {/each}
  </ol>
  <button type="button" class="btn-secondary add-step" onclick={addStep}>
    <Plus size={14} stroke-width={2} /> {addLabel}
  </button>
</div>

{#if mode === 'text' && listId}
  <datalist id={listId}>
    {#each options as option}
      <option value={option.value}>{option.label || option.value}</option>
    {/each}
  </datalist>
{/if}

<style>
  .chain-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .chain-row { display: flex; align-items: center; gap: 8px; }
  .step-num { width: 22px; flex: 0 0 22px; display: grid; place-items: center; font-size: 11px; font-weight: 700; color: var(--color-fg-3); }
  .step-role { width: 74px; flex: 0 0 74px; font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.4px; color: var(--color-fg-3); }
  .chain-row:first-child .step-role { color: var(--color-primary); }
  .chain-input { flex: 1; min-width: 0; }
  .btn-icon {
    display: flex; align-items: center; justify-content: center;
    width: 32px; height: 32px; flex: 0 0 32px;
    border-radius: var(--radius-sm); border: none; background: transparent;
    cursor: pointer; color: var(--color-fg-2); transition: var(--transition);
  }
  .btn-icon:hover:not(:disabled) { background: var(--color-border-light); }
  .btn-icon:disabled { opacity: 0.35; cursor: not-allowed; }
  .btn-icon.remove { color: var(--color-error); }
  .btn-icon.remove:hover { background: var(--color-error-light); }
  .add-step { margin-top: 10px; display: inline-flex; align-items: center; gap: 6px; min-height: 40px; }
  @media (max-width: 640px) {
    .chain-row { flex-wrap: wrap; }
    .step-role { width: auto; flex: 0 0 auto; }
    .chain-input { flex: 1 1 100%; order: 3; }
  }
</style>

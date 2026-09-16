<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { Search, ArrowRight, X } from 'lucide-svelte';
  import { searchNavigation } from '$lib/navigation';

  let open = $state(false);
  let query = $state('');
  let input = $state<HTMLInputElement>();
  let trigger = $state<HTMLButtonElement>();
  let palette = $state<HTMLDivElement>();
  let activeIndex = $state(0);
  const results = $derived(searchNavigation(query));

  $effect(() => {
    query;
    activeIndex = 0;
  });

  async function show() {
    open = true;
    query = '';
    activeIndex = 0;
    await Promise.resolve();
    input?.focus();
  }

  async function close() {
    open = false;
    await Promise.resolve();
    trigger?.focus();
  }

  function isEditable(target: EventTarget | null) {
    return target instanceof Element && !!target.closest('input, textarea, select, [contenteditable="true"]');
  }

  function focusables() {
    return palette ? Array.from(palette.querySelectorAll<HTMLElement>('input, button, a[href], [tabindex]:not([tabindex="-1"])')).filter(element => !element.hasAttribute('disabled')) : [];
  }

  async function openActive() {
    const item = results[activeIndex];
    if (!item) return;
    open = false;
    await goto(item.path);
  }

  function handleDialogKey(event: KeyboardEvent) {
    if (event.key === 'Escape') { event.preventDefault(); close(); return; }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      if (results.length) activeIndex = (activeIndex + (event.key === 'ArrowDown' ? 1 : -1) + results.length) % results.length;
      return;
    }
    if (event.key === 'Enter' && event.target === input) { event.preventDefault(); openActive(); return; }
    if (event.key !== 'Tab') return;
    const items = focusables();
    if (!items.length) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  }

  onMount(() => {
    const handleKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        open ? close() : show();
      } else if (event.key === '/' && !open && !isEditable(event.target)) {
        event.preventDefault();
        show();
      }
    };
    const containFocus = (event: FocusEvent) => {
      if (open && palette && event.target instanceof Node && !palette.contains(event.target)) input?.focus();
    };
    window.addEventListener('keydown', handleKey);
    document.addEventListener('focusin', containFocus);
    return () => {
      window.removeEventListener('keydown', handleKey);
      document.removeEventListener('focusin', containFocus);
    };
  });
</script>

<button bind:this={trigger} class="command-trigger" type="button" aria-label="Search navigation" onclick={show}>
  <Search size={16} />
  <span>Search</span>
  <kbd>⌘K</kbd>
</button>

{#if open}
  <div class="palette-backdrop" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) close(); }}>
    <div bind:this={palette} class="palette" role="dialog" aria-modal="true" aria-label="Command Center navigation" tabindex="-1" onkeydown={handleDialogKey}>
      <div class="palette-search">
        <Search size={18} aria-hidden="true" />
        <input bind:this={input} bind:value={query} type="search" aria-label="Search pages" placeholder="Go to a page…" autocomplete="off" aria-activedescendant={results[activeIndex] ? `command-result-${activeIndex}` : undefined} />
        <button type="button" class="close-button" aria-label="Close navigation" onclick={close}><X size={16} /></button>
      </div>
      <div class="palette-results" aria-live="polite">
        {#if results.length}
          {#each results as item, index}
            <a id={`command-result-${index}`} href={item.path} class="palette-result" class:active={index === activeIndex} aria-current={index === activeIndex ? 'true' : undefined} onclick={() => { open = false; }} onmouseenter={() => activeIndex = index}>
              <span class="result-copy"><strong>{item.label}</strong><small>{item.description}</small></span>
              <span class="result-group">{item.group}</span>
              <ArrowRight size={15} aria-hidden="true" />
            </a>
          {/each}
        {:else}
          <div class="palette-empty">No matching page. Search covers navigation only.</div>
        {/if}
      </div>
      <footer><span><kbd>↑↓</kbd> Select</span><span><kbd>↵</kbd> Open</span><span><kbd>Esc</kbd> Close</span></footer>
    </div>
  </div>
{/if}

<style>
  .command-trigger { min-width:220px; min-height:40px; display:flex; align-items:center; gap:9px; padding:0 10px 0 12px; color:var(--color-fg-2); background:var(--color-bg-elevated); border:1px solid var(--color-border); border-radius:9px; cursor:pointer; }
  .command-trigger span { flex:1; text-align:left; font-size:13px; }
  kbd { font:10px var(--font-mono); color:var(--color-fg-3); border:1px solid var(--color-border); background:var(--color-bg-body); border-radius:5px; padding:2px 5px; }
  .palette-backdrop { position:fixed; inset:0; z-index:100; display:grid; place-items:start center; padding:12vh 16px 24px; background:rgba(7,10,18,.6); backdrop-filter:blur(8px); }
  .palette { width:min(620px,100%); overflow:hidden; border:1px solid var(--color-border-strong); border-radius:14px; background:var(--color-bg-elevated); box-shadow:var(--shadow-lg); }
  .palette-search { display:flex; align-items:center; gap:10px; min-height:58px; padding:0 14px; border-bottom:1px solid var(--color-border); color:var(--color-fg-3); }
  input { flex:1; min-width:0; border:0; outline:0; background:transparent; color:var(--color-fg-0); font-size:16px; }
  .close-button { width:36px; height:36px; display:grid; place-items:center; border:0; border-radius:8px; color:var(--color-fg-2); background:transparent; cursor:pointer; }
  .palette-results { max-height:min(430px,62vh); overflow-y:auto; padding:8px; }
  .palette-result { min-height:58px; display:flex; align-items:center; gap:12px; padding:9px 12px; border-radius:9px; color:var(--color-fg-1); text-decoration:none; }
  .palette-result:hover, .palette-result:focus-visible, .palette-result.active { background:var(--color-primary-light); color:var(--color-primary); }
  .result-copy { flex:1; min-width:0; display:grid; gap:2px; }
  .result-copy strong { font-size:13px; font-weight:650; }
  .result-copy small { overflow:hidden; color:var(--color-fg-3); font-size:12px; text-overflow:ellipsis; white-space:nowrap; }
  .result-group { color:var(--color-fg-3); font-size:11px; }
  .palette-empty { padding:32px 16px; color:var(--color-fg-2); font-size:13px; text-align:center; }
  footer { display:flex; gap:16px; padding:10px 14px; border-top:1px solid var(--color-border); color:var(--color-fg-3); font-size:11px; }
  @media (max-width:640px) { .command-trigger { min-width:44px; width:44px; min-height:44px; padding:0; justify-content:center; } .close-button { width:44px; height:44px; } .command-trigger span, .command-trigger kbd, .result-group { display:none; } .palette-backdrop { padding-top:72px; } }
</style>

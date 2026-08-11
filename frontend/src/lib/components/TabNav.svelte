<script lang="ts">
  import { page } from '$app/state';

  // Reusable tab bar for consolidated dashboard sections.
  // Each tab is a real route (deep-linkable, bookmark-safe); the tab bar just
  // groups sibling pages under one sidebar entry so the nav stays lean without
  // hiding or deleting any feature.
  type Tab = { label: string; path: string; lab?: boolean };
  let { tabs }: { tabs: Tab[] } = $props();

  function isActive(path: string) {
    return page.url.pathname === path || page.url.pathname.startsWith(path + '/');
  }
</script>

<nav class="tabnav" aria-label="Section tabs">
  {#each tabs as t}
    <a href={t.path} class="tab" class:active={isActive(t.path)}>
      <span>{t.label}</span>
      {#if t.lab}<span class="tab-lab">LAB</span>{/if}
    </a>
  {/each}
</nav>

<style>
  .tabnav {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
    border-bottom: 1px solid #e2e8f0;
    margin-bottom: 20px;
  }
  .tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 9px 14px;
    font-size: 13px;
    font-weight: 500;
    color: #475569;
    text-decoration: none;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    transition: color 0.15s, border-color 0.15s;
  }
  .tab:hover { color: #1e293b; }
  .tab.active {
    color: #4f46e5;
    border-bottom-color: #4f46e5;
    font-weight: 600;
  }
  .tab-lab {
    font-size: 8px;
    font-weight: 700;
    letter-spacing: 0.04em;
    padding: 2px 5px;
    border-radius: 4px;
    background: rgba(139, 92, 246, 0.15);
    color: #7c3aed;
  }
</style>

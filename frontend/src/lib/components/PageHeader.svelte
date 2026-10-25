<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    icon: Icon,
    eyebrow,
    title,
    description,
    actions
  }: {
    icon?: any;
    eyebrow?: string;
    title: string;
    description?: string;
    actions?: Snippet;
  } = $props();
</script>

<header class="page-header">
  {#if Icon}
    <div class="page-header-icon" aria-hidden="true">
      <Icon size={22} />
    </div>
  {/if}
  <div class="page-header-text">
    {#if eyebrow}<span class="page-header-eyebrow">{eyebrow}</span>{/if}
    <h1>{title}</h1>
    {#if description}<p>{description}</p>{/if}
  </div>
  {#if actions}
    <div class="page-header-actions">
      {@render actions()}
    </div>
  {/if}
</header>

<style>
  .page-header {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    flex-wrap: wrap;
    margin: 4px 0 20px;
  }
  .page-header-icon {
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    flex-shrink: 0;
    border-radius: 12px;
    background: var(--color-primary-light);
    color: var(--color-primary);
  }
  .page-header-text {
    flex: 1;
    min-width: 0;
  }
  .page-header-eyebrow {
    display: block;
    margin-bottom: 2px;
    color: var(--color-primary);
    font-size: 10px;
    font-weight: 750;
    letter-spacing: 0.09em;
    text-transform: uppercase;
  }
  .page-header h1 {
    margin: 0;
    color: var(--color-fg-0);
    font-size: 20px;
    font-weight: 700;
    letter-spacing: -0.3px;
    line-height: 1.25;
  }
  .page-header p {
    margin: 4px 0 0;
    max-width: 760px;
    color: var(--color-fg-3);
    font-size: 13px;
    line-height: 1.5;
  }
  .page-header-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    flex-wrap: wrap;
    margin-left: auto;
  }
  @media (max-width: 720px) {
    .page-header-actions {
      width: 100%;
      justify-content: flex-start;
      margin-left: 0;
    }
  }
</style>

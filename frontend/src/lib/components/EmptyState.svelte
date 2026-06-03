<script lang="ts">
  import type { Component } from 'svelte';

  let { icon: Icon, title, description }: { icon?: any; title: string; description?: string } = $props();
</script>

<div class="flex flex-col items-center justify-center text-center" style="padding: 48px 32px;">
  {#if Icon}
    <div style="animation: float 3s ease-in-out infinite; opacity: 0.4; margin-bottom: 16px;">
      <Icon size={48} style="color: var(--color-fg-3);" stroke-width={1.2} />
    </div>
  {/if}
  <div style="font-size: 14px; font-weight: 500; color: var(--color-fg-2);">{title}</div>
  {#if description}
    <div style="font-size: 13px; color: var(--color-fg-3); margin-top: 4px;">{description}</div>
  {/if}
</div>

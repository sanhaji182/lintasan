<script lang="ts">
  import type { Component } from 'svelte';

  let {
    label, value, icon: Icon, color = 'var(--color-primary)',
    trend, subtitle
  }: {
    label: string; value: string | number; icon?: any;
    color?: string; trend?: string; subtitle?: string;
  } = $props();
</script>

<div class="card relative overflow-hidden" style="padding: 20px;">
  <!-- Top accent bar -->
  <div
    class="absolute top-0 left-0 right-0"
    style="height: 3px; background: linear-gradient(135deg, {color} 0%, #6366f1 100%);"
  ></div>

  <div class="flex items-start justify-between mb-4">
    {#if Icon}
      <div
        class="flex items-center justify-center rounded-xl"
        style="width: 44px; height: 44px; background: {color}15; border-radius: 12px;"
      >
        <Icon size={22} style="color: {color};" stroke-width={1.8} />
      </div>
    {/if}
    {#if trend}
      <span class="badge badge-success text-xs">{trend}</span>
    {/if}
  </div>

  <div
    class="font-bold font-mono"
    style="font-size: 28px; letter-spacing: -0.5px; color: var(--color-fg-0); line-height: 1;"
  >{value}</div>
  <div class="mt-1.5" style="font-size: 13px; font-weight: 500; color: var(--color-fg-2);">{label}</div>
  {#if subtitle}
    <div style="font-size: 12px; color: var(--color-fg-3);">{subtitle}</div>
  {/if}
</div>

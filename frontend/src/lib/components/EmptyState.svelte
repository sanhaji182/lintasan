<script lang="ts">
  import type { Component } from 'svelte';

  let {
    icon: Icon,
    title,
    description,
    action,
    actionLabel,
    variant = 'default'
  }: {
    icon?: any;
    title: string;
    description?: string;
    action?: () => void;
    actionLabel?: string;
    variant?: 'default' | 'compact' | 'card';
  } = $props();
</script>

<div
  class="empty-state"
  class:empty-compact={variant === 'compact'}
  class:empty-card={variant === 'card'}
  style="animation: fadeInUp 0.4s ease-out;"
>
  {#if Icon}
    <div class="empty-icon-wrap">
      <div class="empty-icon-bg">
        <Icon size={variant === 'compact' ? 22 : 28} stroke-width={1.2} />
      </div>
    </div>
  {:else}
    <div class="empty-icon-wrap">
      <div class="empty-icon-bg empty-icon-empty">
        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2">
          <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
          <circle cx="12" cy="7" r="4"/>
        </svg>
      </div>
    </div>
  {/if}
  <div class="empty-title">{title}</div>
  {#if description}
    <div class="empty-desc">{description}</div>
  {/if}
  {#if action && actionLabel}
    <button class="empty-action" onclick={action}>
      {actionLabel}
    </button>
  {/if}
</div>

<style>
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: 48px 32px;
  }
  .empty-compact {
    padding: 32px 24px;
  }
  .empty-card {
    padding: 56px 32px;
    background: var(--color-bg-card);
    border: 1px dashed var(--color-border);
    border-radius: var(--radius);
  }

  .empty-icon-wrap {
    margin-bottom: 12px;
    animation: float 3s ease-in-out infinite;
  }
  .empty-icon-bg {
    width: 56px;
    height: 56px;
    border-radius: 16px;
    background: var(--color-bg-body);
    border: 1px solid var(--color-border);
    display: grid;
    place-items: center;
    color: var(--color-fg-3);
    transition: all 0.2s;
  }
  .empty-compact .empty-icon-bg {
    width: 44px;
    height: 44px;
    border-radius: 12px;
  }
  .empty-icon-empty {
    background: var(--color-primary-light, #eef2ff);
    border-color: color-mix(in srgb, var(--color-primary) 30%, transparent);
    color: var(--color-primary);
  }
  .empty-state:hover .empty-icon-bg {
    border-color: var(--color-fg-3);
    color: var(--color-fg-2);
  }

  .empty-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--color-fg-1);
    line-height: 1.4;
    margin-bottom: 4px;
  }
  .empty-compact .empty-title {
    font-size: 13px;
  }

  .empty-desc {
    font-size: 13px;
    color: var(--color-fg-3);
    line-height: 1.5;
    max-width: 320px;
    margin-top: 4px;
  }
  .empty-compact .empty-desc {
    font-size: 12px;
  }

  .empty-action {
    margin-top: 16px;
    padding: 8px 16px;
    background: var(--color-primary);
    color: #fff;
    border: none;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }
  .empty-action:hover {
    background: var(--color-primary-hover);
  }

  @keyframes fadeInUp {
    from { opacity: 0; transform: translateY(12px); }
    to { opacity: 1; transform: translateY(0); }
  }
  @keyframes float {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-6px); }
  }
</style>

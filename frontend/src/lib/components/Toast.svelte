<script lang="ts">
  import { toasts, type ToastItem } from '$lib/toast';
  import { CheckCircle2, AlertCircle, Info, X } from 'lucide-svelte';

  const colorMap: Record<string, string> = {
    success: 'var(--color-success)',
    error: 'var(--color-error)',
    info: 'var(--color-info)'
  };
  const bgMap: Record<string, string> = {
    success: 'var(--color-success-light)',
    error: 'var(--color-error-light)',
    info: 'var(--color-info-light)'
  };
</script>

{#if $toasts.length > 0}
  <div class="toast-container">
    {#each $toasts as toast (toast.id)}
      <div
        class="toast"
        class:toast-rich={!!toast.detail}
        style="background: {bgMap[toast.type]}; color: {colorMap[toast.type]}; border: 1px solid {colorMap[toast.type]};"
      >
        <div class="toast-icon">
          {#if toast.type === 'success'}
            <CheckCircle2 size={toast.detail ? 18 : 16} />
          {:else if toast.type === 'error'}
            <AlertCircle size={toast.detail ? 18 : 16} />
          {:else}
            <Info size={toast.detail ? 18 : 16} />
          {/if}
        </div>
        <div class="toast-body">
          <div class="toast-message">{toast.message}</div>
          {#if toast.detail}
            <div class="toast-detail">
              {#if toast.detail.code}
                <span class="toast-badge code">{toast.detail.code}</span>
              {/if}
              {#if toast.detail.type}
                <span class="toast-badge type">{toast.detail.type}</span>
              {/if}
              {#if toast.detail.param}
                <span class="toast-badge param">param: {toast.detail.param}</span>
              {/if}
            </div>
            {#if toast.detail.message}
              <div class="toast-upstream">{toast.detail.message}</div>
            {/if}
            {#if toast.detail.hint}
              <div class="toast-hint">💡 {toast.detail.hint}</div>
            {/if}
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    top: 16px;
    right: 16px;
    z-index: 9999;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-width: 420px;
  }
  .toast {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 16px;
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-md);
    animation: slideIn 0.3s ease-out;
  }
  .toast-icon {
    flex-shrink: 0;
    margin-top: 1px;
  }
  .toast-body {
    flex: 1;
    min-width: 0;
  }
  .toast-message {
    font-size: 13px;
    font-weight: 600;
    line-height: 1.4;
  }
  .toast-detail {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
    margin-top: 6px;
  }
  .toast-badge {
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 9.5px;
    font-weight: 600;
    font-family: ui-monospace, SFMono-Regular, monospace;
    letter-spacing: 0.02em;
    text-transform: lowercase;
  }
  .toast-badge.code {
    background: rgba(0, 0, 0, 0.12);
    color: inherit;
  }
  .toast-badge.type {
    background: rgba(255, 255, 255, 0.5);
    color: inherit;
    opacity: 0.85;
  }
  .toast-badge.param {
    background: rgba(0, 0, 0, 0.08);
    color: inherit;
    opacity: 0.75;
  }
  .toast-upstream {
    margin-top: 6px;
    padding: 6px 8px;
    background: rgba(0, 0, 0, 0.06);
    border-radius: 4px;
    font-family: ui-monospace, SFMono-Regular, monospace;
    font-size: 11px;
    line-height: 1.4;
    word-wrap: break-word;
    max-height: 60px;
    overflow-y: auto;
  }
  .toast-hint {
    margin-top: 6px;
    font-size: 11.5px;
    line-height: 1.4;
    opacity: 0.92;
  }
  @keyframes slideIn {
    from { transform: translateX(100%); opacity: 0; }
    to { transform: translateX(0); opacity: 1; }
  }
</style>

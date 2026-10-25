<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { CircleAlert, Info, X } from 'lucide-svelte';
  import { confirmRequest, resolveConfirm, setConfirmHostMounted, type ConfirmRequest } from '$lib/confirm';

  let request = $state<ConfirmRequest | null>(null);
  let cancelButton = $state<HTMLButtonElement>();
  let confirmButton = $state<HTMLButtonElement>();
  let lastFocused: HTMLElement | null = null;

  onMount(() => {
    setConfirmHostMounted(true);
    const unsubscribe = confirmRequest.subscribe((value) => {
      const wasOpen = request !== null;
      request = value;
      if (value && !wasOpen) {
        lastFocused = document.activeElement as HTMLElement | null;
        tick().then(() => {
          if (value.tone === 'danger') cancelButton?.focus();
          else confirmButton?.focus();
        });
      } else if (!value && wasOpen) {
        lastFocused?.focus?.();
        lastFocused = null;
      }
    });
    return () => {
      unsubscribe();
      setConfirmHostMounted(false);
    };
  });

  $effect(() => {
    document.body.style.overflow = request ? 'hidden' : '';
    return () => {
      document.body.style.overflow = '';
    };
  });

  function onKeydown(event: KeyboardEvent) {
    if (!request) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      resolveConfirm(false);
      return;
    }
    if (event.key === 'Tab') {
      event.preventDefault();
      const next = document.activeElement === cancelButton ? confirmButton : cancelButton;
      next?.focus();
    }
  }
</script>

{#if request}
  <div class="confirm-root">
    <button
      class="confirm-backdrop"
      type="button"
      aria-label="Dismiss dialog"
      onclick={() => resolveConfirm(false)}
    ></button>
    <div
      class="confirm-panel"
      class:confirm-danger={request.tone === 'danger'}
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      aria-describedby={request.message || request.detail ? 'confirm-description' : undefined}
      tabindex="-1"
      onkeydown={onKeydown}
    >
      <div class="confirm-icon" aria-hidden="true">
        {#if request.tone === 'danger'}<CircleAlert size={20} />{:else}<Info size={20} />{/if}
      </div>
      <div class="confirm-body">
        <h2 id="confirm-title">{request.title}</h2>
        {#if request.message || request.detail}
          <p id="confirm-description">
            {request.message}{#if request.message && request.detail}<br /><br />{/if}{request.detail}
          </p>
        {/if}
      </div>
      <div class="confirm-actions">
        <button bind:this={cancelButton} type="button" class="btn-secondary" onclick={() => resolveConfirm(false)}>
          {request.cancelLabel}
        </button>
        <button
          bind:this={confirmButton}
          type="button"
          class="confirm-action"
          class:danger={request.tone === 'danger'}
          onclick={() => resolveConfirm(true)}
        >
          {request.confirmLabel}
        </button>
      </div>
      <button class="confirm-close" type="button" aria-label="Close dialog" onclick={() => resolveConfirm(false)}>
        <X size={15} />
      </button>
    </div>
  </div>
{/if}

<style>
  .confirm-root {
    position: fixed;
    inset: 0;
    z-index: 2000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
  }
  .confirm-backdrop {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    border: 0;
    padding: 0;
    background: rgba(15, 23, 42, 0.45);
    backdrop-filter: blur(3px);
    cursor: default;
  }
  .confirm-panel {
    position: relative;
    z-index: 1;
    display: flex;
    align-items: flex-start;
    gap: 12px;
    flex-wrap: wrap;
    width: 100%;
    max-width: 440px;
    padding: 20px;
    border: 1px solid var(--color-border);
    border-radius: 14px;
    background: var(--color-bg-card);
    box-shadow: 0 24px 60px -18px rgba(15, 23, 42, 0.45);
    animation: confirm-in 0.16s ease-out;
  }
  .confirm-icon {
    display: grid;
    place-items: center;
    width: 38px;
    height: 38px;
    flex-shrink: 0;
    border-radius: 10px;
    background: var(--color-primary-light);
    color: var(--color-primary);
  }
  .confirm-danger .confirm-icon {
    background: var(--color-error-light);
    color: var(--color-error);
  }
  .confirm-body {
    flex: 1;
    min-width: 0;
    padding-right: 22px;
  }
  .confirm-body h2 {
    margin: 2px 0 0;
    font-size: 15px;
    font-weight: 700;
    color: var(--color-fg-0);
    line-height: 1.35;
  }
  .confirm-body p {
    margin: 6px 0 0;
    font-size: 12.5px;
    line-height: 1.55;
    color: var(--color-fg-2);
    white-space: pre-line;
  }
  .confirm-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    width: 100%;
    margin-top: 8px;
  }
  .confirm-action {
    border: 0;
    border-radius: 9px;
    padding: 9px 14px;
    background: var(--color-primary);
    color: #fff;
    font-size: 12px;
    font-weight: 650;
    cursor: pointer;
  }
  .confirm-action:hover {
    background: var(--color-primary-hover);
  }
  .confirm-action.danger {
    background: var(--color-error);
  }
  .confirm-action.danger:hover {
    background: color-mix(in srgb, var(--color-error) 85%, black);
  }
  .confirm-close {
    position: absolute;
    top: 12px;
    right: 12px;
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: var(--color-fg-3);
    cursor: pointer;
  }
  .confirm-close:hover {
    background: var(--color-bg-hover);
    color: var(--color-fg-1);
  }
  @keyframes confirm-in {
    from {
      opacity: 0;
      transform: translateY(6px) scale(0.98);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }
  @media (max-width: 520px) {
    .confirm-panel {
      padding: 16px;
    }
    .confirm-actions {
      flex-direction: column-reverse;
    }
    .confirm-actions :global(button) {
      width: 100%;
    }
  }
</style>

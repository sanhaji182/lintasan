<script lang="ts">
  import { CircleHelp } from 'lucide-svelte';

  let { content = '' }: { content?: string } = $props();
</script>

<div class="help-tooltip-wrapper">
  <button class="help-btn" aria-label="Help" onclick={(e) => e.stopPropagation()}>
    <CircleHelp size={14} stroke-width={1.8} />
  </button>
  <div class="help-tooltip">
    {content}
  </div>
</div>

<style>
  .help-tooltip-wrapper {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .help-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border-radius: 999px;
    border: none;
    background: transparent;
    color: var(--color-fg-3);
    cursor: help;
    transition: var(--transition);
    padding: 0;
  }

  .help-btn:hover {
    color: var(--color-primary);
    background: var(--color-primary-light);
  }

  .help-tooltip {
    display: none;
    position: absolute;
    top: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
    min-width: 240px;
    max-width: 340px;
    padding: 10px 14px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-md);
    font-size: 12px;
    line-height: 1.6;
    color: var(--color-fg-1);
    z-index: 100;
    white-space: normal;
    text-align: left;
  }

  .help-tooltip-wrapper:hover .help-tooltip {
    display: block;
  }

  @media (max-width: 768px) {
    .help-tooltip {
      left: 0;
      transform: none;
    }
  }
</style>

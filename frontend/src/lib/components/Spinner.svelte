<script lang="ts">
  let { size = 32 }: { size?: number } = $props();
</script>

<div class="flex items-center justify-center" style="padding: 44px;" role="status" aria-label="Loading">
  <div
    style="
      width: {size}px; height: {size}px;
      border: 3px solid var(--color-border);
      border-top-color: var(--color-primary);
      border-radius: 50%;
      animation: spin 0.8s linear infinite;
    "
  ></div>
  <span class="sr-only">Loading…</span>
</div>

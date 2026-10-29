<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  onMount(() => {
    goto('/dashboard/connections', { replaceState: true });
  });
</script>

<svelte:head>
  <title>Redirecting — Lintasan</title>
  <meta http-equiv="refresh" content="0;url=/dashboard/connections" />
</svelte:head>

<div class="flex flex-col items-center justify-center p-12 text-center" style="min-height: 200px;">
  <p class="text-sm text-slate-500 mb-2" style="color: var(--color-fg-2, #64748b);">Redirecting to Connections...</p>
  <a href="/dashboard/connections" class="text-xs text-indigo-600 underline" style="color: var(--color-primary, #6366f1);">
    Click here if not redirected automatically
  </a>
</div>

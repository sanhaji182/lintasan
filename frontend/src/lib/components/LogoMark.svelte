<script module lang="ts">
  let nextLogoId = 0;
</script>

<script lang="ts">
  let {
    size = 36,
    title = 'Lintasan',
    variant = 'light',
    decorative = false,
    class: className = ''
  }: {
    size?: number;
    title?: string;
    variant?: 'light' | 'dark';
    decorative?: boolean;
    class?: string;
  } = $props();

  const gradientId = `lintasan-route-${++nextLogoId}`;
</script>

<svg
  class={className}
  width={size}
  height={size}
  viewBox="0 0 64 64"
  fill="none"
  xmlns="http://www.w3.org/2000/svg"
  role={decorative ? undefined : 'img'}
  aria-hidden={decorative ? 'true' : undefined}
  aria-label={decorative ? undefined : title}
>
  {#if !decorative}<title>{title}</title>{/if}
  <defs>
    <linearGradient id={gradientId} x1="8" y1="8" x2="56" y2="56" gradientUnits="userSpaceOnUse">
      <stop stop-color={variant === 'dark' ? '#67E8F9' : '#2563EB'} />
      <stop offset="1" stop-color={variant === 'dark' ? '#38BDF8' : '#06B6D4'} />
    </linearGradient>
  </defs>
  <rect width="64" height="64" rx="16" fill={variant === 'dark' ? '#0F172A' : '#EFF6FF'} />
  <path d="M10 13H19C27 13 33 19 33 27V45C33 50 37 53 42 53H54" stroke={`url(#${gradientId})`} stroke-width="7" stroke-linecap="round" stroke-linejoin="round" />
  <path d="M10 27H21C28 27 33 32 33 39" stroke={`url(#${gradientId})`} stroke-width="7" stroke-linecap="round" stroke-linejoin="round" />
  <path d="M10 41H22C28 41 33 36 33 30" stroke={`url(#${gradientId})`} stroke-width="7" stroke-linecap="round" stroke-linejoin="round" />
</svg>

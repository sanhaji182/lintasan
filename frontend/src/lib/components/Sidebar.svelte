<script lang="ts">
  import { page } from '$app/state';
  import {
    LayoutDashboard, Link2, GitBranch, BarChart3, Key, Users, UserCircle, Webhook,
    Database, Settings, Puzzle, MessageSquare, BookOpen, Brain, Globe, Server,
    Sun, Moon, Upload, Plug, Rocket, FlaskConical, ChevronDown
  } from 'lucide-svelte';
  import { theme } from '$lib/stores/theme';
  import LogoMark from '$lib/components/LogoMark.svelte';
  import { navigationGroups, routeIsActive, groupIsInitiallyOpen } from '$lib/navigation';

  let { open = $bindable(false) }: { open?: boolean } = $props();
  let expanded = $state<Record<string, boolean>>({});
  const pathname = $derived(page.url.pathname);

  const icons: Record<string, any> = {
    overview: LayoutDashboard, quickstart: Rocket, playground: MessageSquare,
    connections: Link2, models: Brain, providers: Server, routing: GitBranch, observability: BarChart3,
    keys: Key, teams: Users, users: UserCircle, webhooks: Webhook, memory: Brain,
    mcp: Plug, translator: Globe, plugins: Puzzle, backup: Database, migrate: Upload,
    labs: FlaskConical, settings: Settings, docs: BookOpen,
  };

  function isGroupOpen(label: (typeof navigationGroups)[number]['label']) {
    return expanded[label] ?? groupIsInitiallyOpen(label, pathname);
  }

  function toggleGroup(label: string) {
    expanded[label] = !isGroupOpen(label as (typeof navigationGroups)[number]['label']);
  }

  let version = $state('');
  $effect(() => {
    fetch('/health').then(r => r.ok ? r.json() : null).then(d => { if (d?.version) version = d.version; }).catch(() => {});
  });
</script>

{#if open}<button class="overlay" onclick={() => open = false} aria-label="Close sidebar"></button>{/if}

<aside class="sidebar" class:open>
  <div class="sidebar-brand">
    <LogoMark size={36} variant={$theme === 'dark' ? 'dark' : 'light'} decorative />
    <div><div class="sb-name">Lintasan</div><div class="sb-context">Command Center</div><div class="sb-version">{version}</div></div>
  </div>

  <nav class="sidebar-nav" aria-label="Dashboard navigation">
    {#each navigationGroups as group}
      <div class="nav-group">
        {#if group.collapsible}
          <button class="nav-group-toggle" aria-expanded={isGroupOpen(group.label)} onclick={() => toggleGroup(group.label)}>
            <span>{group.label}</span><ChevronDown size={14} class={isGroupOpen(group.label) ? 'rotated' : ''} />
          </button>
        {:else}
          <div class="nav-group-label">{group.label}</div>
        {/if}
        {#if !group.collapsible || isGroupOpen(group.label)}
          <div class="nav-items">
            {#each group.items as item}
              {@const active = routeIsActive(item.path, pathname)}
              {@const Icon = icons[item.icon]}
              <a href={item.path} class="nav-item" class:active aria-current={active ? 'page' : undefined} onclick={() => open = false}>
                <Icon size={18} stroke-width={1.6} /><span>{item.label}</span>
                {#if item.label === 'Labs'}<span class="nav-lab">LAB</span>{/if}
              </a>
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  </nav>

  <div class="sidebar-footer">
    <button class="theme-btn" onclick={() => theme.toggle()} aria-label={$theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'}>
      {#if $theme === 'light'}<Moon size={16} /> Dark mode{:else}<Sun size={16} /> Light mode{/if}
    </button>
  </div>
</aside>

<style>
  .overlay { display: none; position: fixed; inset: 0; z-index: 45; background: rgba(15,23,42,.3); backdrop-filter: blur(4px); }
  .sidebar { position: fixed; inset: 0 auto 0 0; z-index: 50; width: var(--sidebar-w); background: var(--color-bg-sidebar); border-right: 1px solid var(--color-sidebar-border); display: flex; flex-direction: column; transition: transform .25s ease; }
  .sidebar-brand { display: flex; align-items: center; gap: 12px; padding: 20px 18px 17px; border-bottom: 1px solid var(--color-border-light); }
  .sb-name { font-size: 15px; font-weight: 680; color: var(--color-fg-0); letter-spacing: -.3px; line-height:1.15; }
  .sb-context { margin-top:2px; font-size:10px; font-weight:650; color:var(--color-primary); letter-spacing:.06em; text-transform:uppercase; }
  .sb-version { min-height: 13px; margin-top:2px; font: 10px var(--font-mono); color: var(--color-fg-3); }
  .sidebar-nav { flex: 1; overflow-y: auto; padding: 12px 10px; }
  .nav-group { margin-bottom: 14px; }
  .nav-group-label, .nav-group-toggle { width: 100%; font-size: 10px; font-weight: 750; letter-spacing: .07em; color: var(--color-fg-3); text-transform: uppercase; padding: 7px 12px; }
  .nav-group-toggle { display: flex; align-items: center; justify-content: space-between; background: none; border: 0; cursor: pointer; border-radius: 7px; }
  .nav-group-toggle:hover { background: var(--color-bg-sidebar-hover); color: var(--color-fg-1); }
  .nav-group-toggle :global(svg) { transition: transform .2s; }
  .nav-group-toggle :global(svg.rotated) { transform: rotate(180deg); }
  .nav-items { animation: reveal .18s ease-out; }
  .nav-item { min-height: 40px; display: flex; align-items: center; gap: 10px; padding: 8px 12px; border-radius: 8px; font-size: 13px; font-weight: 510; color: var(--color-fg-2); text-decoration: none; margin-bottom: 1px; transition: background .15s, color .15s; }
  .nav-item:hover { background: var(--color-bg-sidebar-hover); color: var(--color-fg-0); }
  .nav-item.active { background: var(--color-primary-light); color: var(--color-primary); font-weight: 650; }
  .nav-lab { margin-left: auto; font-size: 8px; font-weight: 800; letter-spacing: .04em; padding: 2px 5px; border-radius: 4px; background: var(--color-purple-light); color: var(--color-purple); }
  .sidebar-footer { padding: 13px 14px; border-top: 1px solid var(--color-border-light); }
  .theme-btn { min-height: 42px; display: flex; align-items: center; gap: 10px; width: 100%; padding: 9px 12px; background: var(--color-bg-elevated); border: 1px solid var(--color-border); border-radius: 9px; font-size: 13px; font-weight: 500; color: var(--color-fg-2); cursor: pointer; }
  .theme-btn:hover { background: var(--color-bg-sidebar-hover); }
  @keyframes reveal { from { opacity: 0; transform: translateY(-3px); } }
  @media (max-width: 768px) {
    .sidebar { transform: translateX(-100%); }
    .sidebar.open { transform: translateX(0); }
    .overlay { display: block; }
  }
</style>

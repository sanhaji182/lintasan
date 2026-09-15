<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { showToast } from '$lib/toast';
  import { deriveQuickstart, buildQuickstartSnippets, type QuickstartConnection, type GatewayKey, type CallableModel } from '$lib/quickstart';
  import { Check, Circle, Link2, Key, Cpu, Terminal, MessageSquare, Copy, ExternalLink, RefreshCw, Plus } from 'lucide-svelte';

  let connections = $state<QuickstartConnection[]>([]);
  let keys = $state<GatewayKey[]>([]);
  let models = $state<CallableModel[]>([]);
  let loading = $state(true);
  let error = $state('');
  let sourceErrors = $state<string[]>([]);
  let creating = $state(false);
  let newKey = $state('');
  let copied = $state('');
  let snippetTab = $state<'curl' | 'python' | 'javascript'>('curl');
  let baseUrl = $state('/v1');

  const quickstartState = $derived(deriveQuickstart({ connections, keys, models, baseUrl }));
  const snippets = $derived(buildQuickstartSnippets({
    baseUrl,
    model: quickstartState.recommendedModel?.id || 'YOUR_CALLABLE_MODEL',
    gatewayKey: newKey || null,
  }));

  async function load() {
    loading = true;
    error = '';
    sourceErrors = [];
    connections = [];
    keys = [];
    models = [];
    const [connectionResult, keyResult, modelResult] = await Promise.allSettled([
      api.get<any>('/api/connections'),
      api.get<any>('/api/keys'),
      api.get<any>('/v1/models'),
    ]);
    if (connectionResult.status === 'fulfilled') connections = connectionResult.value.data || [];
    else sourceErrors.push('connections');
    if (keyResult.status === 'fulfilled') keys = keyResult.value.data || [];
    else sourceErrors.push('gateway keys');
    if (modelResult.status === 'fulfilled') models = modelResult.value.data || [];
    else sourceErrors.push('callable models');
    if (sourceErrors.length === 3) error = 'Quickstart data could not be loaded.';
    else if (sourceErrors.length) error = `Some status is unavailable: ${sourceErrors.join(', ')}.`;
    loading = false;
  }

  onMount(() => {
    baseUrl = `${window.location.origin}/v1`;
    load();
  });

  async function createGatewayKey() {
    if (creating) return;
    creating = true;
    try {
      const created = await api.post<any>('/api/keys', { action: 'create', name: 'Quickstart' });
      newKey = created.key || '';
      keys = [...keys, created];
      showToast('Gateway API key created. Copy it now; keep it secret.', 'success');
    } catch (e: any) {
      showToast(e.message || 'Could not create gateway API key', 'error');
    } finally {
      creating = false;
    }
  }

  async function copy(value: string, label: string) {
    await navigator.clipboard.writeText(value);
    copied = label;
    showToast(`${label} copied`, 'success');
    setTimeout(() => copied = '', 1800);
  }

  const steps = $derived([
    { number: 1, title: 'Connect a provider', description: 'Add a provider credential so Lintasan has an upstream route.', icon: Link2, ...quickstartState.connection, href: '/dashboard/connections', action: quickstartState.connection.ready ? 'Manage connections' : 'Connect provider' },
    { number: 2, title: 'Get a gateway API key', description: 'Use this Lintasan key in your app — never the upstream provider key.', icon: Key, ...quickstartState.key, href: '/dashboard/keys', action: 'Manage API keys' },
    { number: 3, title: 'Choose a callable model', description: 'Recommended from models the gateway currently reports as callable.', icon: Cpu, ...quickstartState.model, href: '/dashboard/connections', action: 'Review connections' },
    { number: 4, title: 'Copy endpoint and code', description: 'Use the public OpenAI-compatible endpoint in curl or an SDK.', icon: Terminal, ...quickstartState.endpoint, href: '', action: '' },
    { number: 5, title: 'Test in Playground', description: 'Handoff the recommended model and make a safe test request.', icon: MessageSquare, ...quickstartState.playground, href: quickstartState.recommendedModel?.id ? `/dashboard/playground?model=${encodeURIComponent(quickstartState.recommendedModel.id)}` : '/dashboard/playground', action: 'Open Playground' },
  ]);
</script>

<svelte:head><title>Quickstart — Lintasan</title></svelte:head>

<div class="quickstart-page">
  <section class="hero-card">
    <div>
      <div class="eyebrow">Developer onboarding</div>
      <h1>Your first request, step by step</h1>
      <p>Connect an upstream, get a gateway key, choose a real callable model, then test it. Values below come from this Lintasan instance.</p>
    </div>
    <div class="progress-card" aria-label={`${quickstartState.completedSteps} of 5 steps ready`}>
      <strong>{quickstartState.completedSteps}/5</strong>
      <span>steps ready</span>
      <div class="progress-track"><span style={`width: ${quickstartState.completedSteps * 20}%`}></span></div>
    </div>
  </section>

  {#if loading}
    <div class="loading-grid" aria-label="Loading Quickstart status">
      {#each Array(5) as _}<div class="skeleton step-skeleton"></div>{/each}
    </div>
  {:else}
    {#if error}
      <div class="notice error-notice"><span>{error}</span><button class="btn-secondary" onclick={load}><RefreshCw size={14} /> Retry</button></div>
    {/if}

    <div class="steps">
      {#each steps as step}
        <article class:ready={step.ready} class="step-card">
          <div class="status-icon">{#if step.ready}<Check size={18} />{:else}<Circle size={18} />{/if}</div>
          <div class="step-body">
            <div class="step-meta">Step {step.number}</div>
            <div class="step-heading"><step.icon size={20} /><h2>{step.title}</h2></div>
            <p>{step.description}</p>
            <div class="truth-value">{step.label}</div>

            {#if step.number === 2 && !step.ready}
              <div class="step-actions">
                <button class="btn-primary" onclick={createGatewayKey} disabled={creating}><Plus size={15} /> {creating ? 'Creating…' : 'Create gateway key'}</button>
                <a class="text-link" href={step.href}>{step.action}</a>
              </div>
            {:else if step.number === 2 && newKey}
              <div class="secret-once">
                <div><strong>Copy this new key now</strong><span>It is shown only for this session.</span></div>
                <code>{newKey}</code>
                <button class="btn-secondary" onclick={() => copy(newKey, 'Gateway key')}><Copy size={14} /> {copied === 'Gateway key' ? 'Copied' : 'Copy key'}</button>
              </div>
            {:else if step.href}
              <a class="text-link" href={step.href}>{step.action} <ExternalLink size={13} /></a>
            {/if}

            {#if step.number === 4}
              <div class="endpoint-row">
                <code>{baseUrl}</code>
                <button class="icon-button" aria-label="Copy public base URL" onclick={() => copy(baseUrl, 'Base URL')}><Copy size={15} /></button>
              </div>
              <div class="snippet-box">
                <div class="snippet-tabs" role="tablist" aria-label="Code example language">
                  {#each ['curl', 'python', 'javascript'] as tab}
                    <button role="tab" aria-selected={snippetTab === tab} class:active={snippetTab === tab} onclick={() => snippetTab = tab as typeof snippetTab}>{tab === 'javascript' ? 'JavaScript' : tab[0].toUpperCase() + tab.slice(1)}</button>
                  {/each}
                  <button class="copy-snippet" aria-label={`Copy ${snippetTab} example`} onclick={() => copy(snippets[snippetTab], `${snippetTab} snippet`)}><Copy size={14} /> Copy</button>
                </div>
                <pre><code>{snippets[snippetTab]}</code></pre>
              </div>
              {#if !newKey}<p class="safe-note">The example uses a placeholder. For safety, existing gateway keys are never revealed here.</p>{/if}
            {/if}
          </div>
        </article>
      {/each}
    </div>
  {/if}
</div>

<style>
  .quickstart-page { max-width: 980px; margin: 0 auto; }
  .hero-card { display: flex; justify-content: space-between; gap: 28px; padding: 30px; border-radius: 20px; background: linear-gradient(135deg, var(--color-primary-light), var(--color-purple-light)); border: 1px solid color-mix(in srgb, var(--color-primary) 22%, var(--color-border)); margin-bottom: 20px; }
  .eyebrow, .step-meta { color: var(--color-primary); text-transform: uppercase; letter-spacing: .08em; font-size: 10px; font-weight: 800; }
  h1 { margin: 5px 0 8px; font-size: clamp(25px, 4vw, 34px); line-height: 1.16; letter-spacing: -.035em; }
  .hero-card p { margin: 0; max-width: 650px; color: var(--color-fg-2); }
  .progress-card { min-width: 150px; align-self: center; background: var(--color-bg-card); border: 1px solid var(--color-border); border-radius: 16px; padding: 16px; box-shadow: var(--shadow-sm); }
  .progress-card strong { display: block; font: 700 25px var(--font-mono); }
  .progress-card span { color: var(--color-fg-2); font-size: 12px; }
  .progress-track { height: 6px; background: var(--color-border-light); border-radius: 99px; margin-top: 12px; overflow: hidden; }
  .progress-track span { display: block; height: 100%; background: var(--color-success); transition: width .35s ease; }
  .steps { display: grid; gap: 12px; }
  .step-card { display: grid; grid-template-columns: 38px 1fr; gap: 12px; padding: 20px; border-radius: 16px; border: 1px solid var(--color-border); background: var(--color-bg-card); box-shadow: var(--shadow-sm); }
  .step-card.ready { border-color: color-mix(in srgb, var(--color-success) 28%, var(--color-border)); }
  .status-icon { width: 32px; height: 32px; display: grid; place-items: center; color: var(--color-fg-3); border: 1px solid var(--color-border); border-radius: 50%; }
  .ready .status-icon { color: var(--color-success); background: var(--color-success-light); border-color: transparent; }
  .step-heading { display: flex; align-items: center; gap: 9px; margin: 3px 0; color: var(--color-fg-0); }
  .step-heading h2 { font-size: 16px; margin: 0; }
  .step-body > p { color: var(--color-fg-2); margin: 2px 0 10px; font-size: 13px; }
  .truth-value { display: inline-flex; padding: 6px 9px; border-radius: 8px; background: var(--color-bg-body); color: var(--color-fg-1); font: 600 12px var(--font-mono); word-break: break-all; }
  .step-actions { display: flex; align-items: center; gap: 14px; margin-top: 12px; flex-wrap: wrap; }
  .btn-primary, .btn-secondary, .text-link { display: inline-flex; align-items: center; justify-content: center; gap: 7px; text-decoration: none; }
  .text-link { margin-top: 11px; color: var(--color-primary); font-size: 13px; font-weight: 650; width: fit-content; }
  .secret-once { display: grid; grid-template-columns: 1fr auto; gap: 9px 14px; align-items: center; padding: 13px; margin-top: 12px; border-radius: 10px; background: var(--color-warning-light); border: 1px solid color-mix(in srgb, var(--color-warning) 25%, var(--color-border)); }
  .secret-once div { display: flex; flex-direction: column; }
  .secret-once span { font-size: 11px; color: var(--color-fg-2); }
  .secret-once code { grid-column: 1 / -1; overflow-wrap: anywhere; }
  .endpoint-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; background: #0f172a; color: #e2e8f0; border-radius: 10px; padding: 10px 12px; margin-top: 12px; }
  .icon-button { display: grid; place-items: center; flex: none; padding: 7px; border: 0; border-radius: 7px; background: rgba(255,255,255,.1); color: white; cursor: pointer; }
  .snippet-box { margin-top: 10px; overflow: hidden; border-radius: 12px; border: 1px solid #26344d; background: #0b1220; color: #dbeafe; }
  .snippet-tabs { display: flex; align-items: center; gap: 2px; padding: 6px; border-bottom: 1px solid #26344d; overflow-x: auto; }
  .snippet-tabs button { border: 0; background: transparent; color: #94a3b8; border-radius: 7px; padding: 7px 10px; cursor: pointer; }
  .snippet-tabs button.active { background: #1e293b; color: white; }
  .snippet-tabs .copy-snippet { margin-left: auto; display: inline-flex; align-items: center; gap: 5px; }
  pre { margin: 0; padding: 15px; overflow-x: auto; font-size: 12px; line-height: 1.6; }
  .safe-note { font-size: 11px !important; margin-top: 8px !important; }
  .step-skeleton { height: 124px; }
  .loading-grid { display: grid; gap: 12px; }
  .notice { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 15px; border-radius: 10px; margin-bottom: 12px; }
  .error-notice { background: var(--color-error-light); color: var(--color-error); }
  @media (max-width: 640px) {
    .hero-card { flex-direction: column; padding: 21px; }
    .progress-card { width: 100%; }
    .step-card { grid-template-columns: 32px minmax(0, 1fr); padding: 16px 13px; }
    .secret-once { grid-template-columns: 1fr; }
    .secret-once code { grid-column: auto; }
  }
</style>

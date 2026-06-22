<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import CodexIcon from '$lib/components/icons/CodexIcon.svelte';
  import ClaudeIcon from '$lib/components/icons/ClaudeIcon.svelte';
  import GeminiIcon from '$lib/components/icons/GeminiIcon.svelte';
  import CopilotIcon from '$lib/components/icons/CopilotIcon.svelte';
  import { FlaskConical, CircleCheck, TriangleAlert, X, ChevronDown, ChevronRight, Key, Lock, LockOpen, Trash2, Wrench, Shield, Cpu, Terminal, ArrowRight, Zap, CircleAlert, Info } from 'lucide-svelte';

  interface Provider {
    name: string;
    track: string;
    state: string;
    risk_badge: string;
    auth_env_var: string;
    credential_set: boolean;
    validation_evidence: string;
    admitted_at: string | null;
    activated_at: string | null;
    capabilities: string[];
    descriptor: any;
  }

  interface GateResult {
    Gate: string;
    Outcome: string;
    Reason: string;
  }

  interface ProviderDetail extends Provider {
    auth_method_id: string;
    deactivated_at: string | null;
    default_path: string;
    args: string[] | null;
    foreign_auth_vars: string[];
    admission_report: { Go?: boolean; Results?: GateResult[] } | null;
  }

  interface CredentialStatus {
    provider: string;
    configured: boolean;
    source: string;
    masked_value: string;
    env_var: string;
    updated_at?: string;
  }

  let providers = $state<Provider[]>([]);
  let credentials = $state<Record<string, CredentialStatus>>({});
  let loading = $state(true);
  let error = $state('');
  let actionLoading = $state('');
  let expanded = $state<string | null>(null);
  let details = $state<Record<string, ProviderDetail>>({});
  let credentialInput = $state<Record<string, string>>({});
  let showCredForm = $state<string | null>(null);

  const summary = $derived({
    total: providers.length,
    active: providers.filter(p => p.state === 'active').length,
    admitted: providers.filter(p => p.state === 'admitted').length,
    proposed: providers.filter(p => p.state === 'proposed').length,
  });

  // Provider brand config
  const providerBrands: Record<string, {
    color: string; bg: string; gradient: string; border: string;
    company: string; tagline: string; icon: string;
    features: string[];
  }> = {
    codex: {
      color: '#10a37f', bg: 'rgba(16,163,127,0.08)',
      gradient: 'linear-gradient(135deg, #10a37f 0%, #0d8c6d 100%)',
      border: 'rgba(16,163,127,0.18)',
      company: 'OpenAI', tagline: 'Code generation & reasoning engine',
      icon: 'codex',
      features: ['Code gen', 'Reasoning', 'Agentic'],
    },
    'claude-code': {
      color: '#d97757', bg: 'rgba(217,119,87,0.08)',
      gradient: 'linear-gradient(135deg, #d97757 0%, #c4603f 100%)',
      border: 'rgba(217,119,87,0.18)',
      company: 'Anthropic', tagline: 'Conversational coding agent',
      icon: 'claude',
      features: ['Chat', 'Analysis', 'Safe'],
    },
    'gemini-cli': {
      color: '#4285f4', bg: 'rgba(66,133,244,0.08)',
      gradient: 'linear-gradient(135deg, #4285f4 0%, #3b7de9 100%)',
      border: 'rgba(66,133,244,0.18)',
      company: 'Google', tagline: 'Multimodal code assistant',
      icon: 'gemini',
      features: ['Multimodal', 'Fast', 'Context'],
    },
    copilot: {
      color: '#6366f1', bg: 'rgba(99,102,241,0.08)',
      gradient: 'linear-gradient(135deg, #6366f1 0%, #5558e6 100%)',
      border: 'rgba(99,102,241,0.18)',
      company: 'GitHub', tagline: 'AI pair programmer',
      icon: 'copilot',
      features: ['Inline', 'Chat', 'Workspace'],
    },
  };

  function getBrand(name: string) {
    return providerBrands[name] || {
      color: 'var(--color-primary)', bg: 'var(--color-primary-light)',
      gradient: 'linear-gradient(135deg, var(--color-primary) 0%, var(--color-primary) 100%)',
      border: 'var(--color-border)',
      company: 'Unknown', tagline: 'Experimental provider',
      icon: 'default', features: [],
    };
  }

  function stateConfig(state: string) {
    switch (state) {
      case 'active': return { label: 'Active', color: '#22c55e', bg: 'rgba(34,197,94,0.12)', icon: 'check' };
      case 'admitted': return { label: 'Ready', color: '#3b82f6', bg: 'rgba(59,130,246,0.12)', icon: 'shield' };
      case 'deprecated': return { label: 'Deprecated', color: '#9ca3af', bg: 'rgba(156,163,175,0.12)', icon: 'x' };
      default: return { label: 'Setup', color: '#f59e0b', bg: 'rgba(245,158,11,0.12)', icon: 'alert' };
    }
  }

  onMount(async () => {
    await fetchProviders();
    await fetchCredentials();
  });

  async function fetchProviders() {
    loading = true; error = '';
    try {
      const res = await api.get<any>('/api/experimental/providers');
      providers = res.data || [];
    } catch (e: any) { error = e.message; }
    finally { loading = false; }
  }

  async function fetchCredentials() {
    try {
      const res = await api.get<any>('/api/experimental/credentials');
      const list: CredentialStatus[] = res.data || [];
      const map: Record<string, CredentialStatus> = {};
      for (const c of list) { map[c.provider] = c; }
      credentials = map;
    } catch (e: any) { /* non-fatal */ }
  }

  async function toggleDetail(name: string) {
    if (expanded === name) { expanded = null; return; }
    expanded = name;
    if (!details[name]) {
      try {
        const res = await api.get<any>(`/api/experimental/providers/${name}`);
        details[name] = res.data;
      } catch (e: any) { error = e.message; }
    }
  }

  async function admit(name: string) {
    actionLoading = name + ':admit';
    try {
      await api.post<any>(`/api/experimental/providers/${name}/admit`, {});
      await fetchProviders(); await fetchCredentials();
      if (expanded === name) { delete details[name]; expanded = null; }
    } catch (e: any) { error = e.message; }
    finally { actionLoading = ''; }
  }

  async function activate(name: string) {
    actionLoading = name + ':activate';
    try {
      await api.post<any>(`/api/experimental/providers/${name}/activate`, {});
      await fetchProviders();
      if (expanded === name) { delete details[name]; expanded = null; }
    } catch (e: any) { error = e.message; }
    finally { actionLoading = ''; }
  }

  async function deactivate(name: string) {
    actionLoading = name + ':deactivate';
    try {
      await api.post<any>(`/api/experimental/providers/${name}/deactivate`, {});
      await fetchProviders();
      if (expanded === name) { delete details[name]; expanded = null; }
    } catch (e: any) { error = e.message; }
    finally { actionLoading = ''; }
  }

  async function setCredential(name: string) {
    const value = credentialInput[name]?.trim();
    if (!value) return;
    actionLoading = name + ':cred';
    try {
      await api.put<any>(`/api/experimental/credentials/${name}`, { credential: value });
      credentialInput[name] = ''; showCredForm = null;
      await fetchCredentials(); await fetchProviders();
    } catch (e: any) { error = e.message; }
    finally { actionLoading = ''; }
  }

  async function deleteCredential(name: string) {
    actionLoading = name + ':cred-del';
    try {
      await api.delete<any>(`/api/experimental/credentials/${name}`);
      await fetchCredentials(); await fetchProviders();
    } catch (e: any) { error = e.message; }
    finally { actionLoading = ''; }
  }

  function fmtDate(d: string | null): string {
    if (!d) return '—';
    return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }
</script>

<svelte:head><title>Experimental Providers — Lintasan</title></svelte:head>

<div style="animation: fadeInUp 0.4s ease-out;">
  <!-- Header with gradient accent -->
  <div class="section-header">
    <div class="header-icon">
      <FlaskConical size={22} />
    </div>
    <div class="header-text">
      <h1 class="header-title">Experimental Providers</h1>
      <p class="header-desc">ACP-based coding agents — securely connect and manage provider accounts</p>
    </div>
  </div>

  <!-- Stats Strip -->
  <div class="stats-strip">
    {#each [
      { label: 'PROVIDERS', value: summary.total, color: 'var(--color-fg-0)', icon: Cpu },
      { label: 'ACTIVE', value: summary.active, color: '#22c55e', icon: CircleCheck },
      { label: 'READY', value: summary.admitted, color: '#3b82f6', icon: Shield },
      { label: 'SETUP', value: summary.proposed, color: '#f59e0b', icon: TriangleAlert },
    ] as stat}
      <div class="stat-cell">
        <stat.icon size={18} style="color: {stat.color}; opacity: 0.5;" />
        <div class="stat-value" style="color: {stat.color};">{stat.value}</div>
        <div class="stat-label">{stat.label}</div>
      </div>
    {/each}
  </div>

  {#if error}
    <div class="error-banner">
      <CircleAlert size={16} />
      <span>{error}</span>
      <button class="error-close" onclick={() => { error = ''; }}><X size={14} /></button>
    </div>
  {/if}

  {#if loading}
    <div class="loading-state">
      <Spinner />
      <p>Loading providers...</p>
    </div>
  {:else if providers.length === 0}
    <EmptyState title="No Experimental Providers" description="Cohort-A providers will appear here once the framework is deployed." />
  {:else}
    <!-- Provider Cards Grid -->
    <div class="provider-grid">
      {#each providers as p (p.name)}
        {@const brand = getBrand(p.name)}
        {@const sc = stateConfig(p.state)}
        {@const cred = credentials[p.name]}
        {@const isOpen = expanded === p.name}
        {@const det = details[p.name]}

        <div class="provider-card" class:expanded={isOpen} style="--brand-color: {brand.color}; --brand-bg: {brand.bg}; --brand-border: {brand.border}; --brand-gradient: {brand.gradient};">
          <!-- Card Header -->
          <div class="card-header">
            <!-- Brand Icon Container -->
            <div class="brand-icon-box" style="background: {brand.bg};">
              {#if brand.icon === 'codex'}
                <CodexIcon size={36} />
              {:else if brand.icon === 'claude'}
                <ClaudeIcon size={36} />
              {:else if brand.icon === 'gemini'}
                <GeminiIcon size={36} />
              {:else if brand.icon === 'copilot'}
                <CopilotIcon size={36} />
              {:else}
                <Terminal size={28} color="white" />
              {/if}
            </div>
            <!-- Title & Badge -->
            <div class="card-titles">
              <div class="provider-name">{p.name}</div>
              <div class="company-tag" style="color: {brand.color}; background: {brand.color}12; border-color: {brand.color}20;">
                {brand.company}
              </div>
            </div>
            <!-- State Badge -->
            <div class="state-pill" style="background: {sc.bg}; color: {sc.color}; border: 1px solid {sc.color}30;">
              {#if sc.icon === 'check'}<CircleCheck size={13} />
              {:else if sc.icon === 'shield'}<Shield size={13} />
              {:else if sc.icon === 'alert'}<TriangleAlert size={13} />
              {:else}<X size={13} />{/if}
              {sc.label}
            </div>
          </div>

          <!-- Tagline & Features -->
          <div class="card-body">
            <p class="tagline">{brand.tagline}</p>
            <div class="feature-pills">
              {#each brand.features as feat}
                <span class="feature-pill" style="color: {brand.color}; background: {brand.color}10; border: 1px solid {brand.color}18;">
                  {feat}
                </span>
              {/each}
            </div>
          </div>

          <!-- Divider -->
          <div class="card-divider"></div>

          <!-- Credential Section -->
          <div class="cred-area">
            <div class="cred-label-row">
              <Key size={14} style="color: var(--color-fg-4);" />
              <span>Credential</span>
            </div>

            {#if cred?.configured}
              <div class="cred-configured">
                <div class="cred-left">
                  <Lock size={13} style="color: #22c55e;" />
                  <span class="cred-source" style="color: #22c55e; background: rgba(34,197,94,0.08); border: 1px solid rgba(34,197,94,0.15);">
                    {cred.source}
                  </span>
                  {#if cred.masked_value}
                    <code class="cred-masked">{cred.masked_value}</code>
                  {/if}
                </div>
                <div class="cred-actions">
                  <button class="btn-ghost xs" onclick={() => { showCredForm = p.name; }}>Update</button>
                  {#if cred.source === 'dashboard'}
                    <button class="btn-ghost xs danger" disabled={actionLoading === p.name + ':cred-del'} onclick={() => deleteCredential(p.name)}>
                      <Trash2 size={12} />
                    </button>
                  {/if}
                </div>
              </div>
            {:else}
              <div class="cred-empty">
                <LockOpen size={14} style="color: var(--color-fg-4);" />
                <span>Not configured</span>
              </div>
            {/if}

            {#if showCredForm === p.name}
              <form class="cred-form" onsubmit={(e) => { e.preventDefault(); setCredential(p.name); }}>
                <input
                  type="password"
                  class="cred-field"
                  placeholder={`Enter ${p.auth_env_var}`}
                  bind:value={credentialInput[p.name]}
                  autocomplete="off"
                />
                <div class="cred-form-actions">
                  <button type="submit" class="btn-brand xs" style="background: {brand.gradient};" disabled={actionLoading === p.name + ':cred' || !credentialInput[p.name]?.trim()}>
                    {actionLoading === p.name + ':cred' ? 'Saving...' : 'Save'}
                  </button>
                  <button type="button" class="btn-ghost xs" onclick={() => { showCredForm = null; }}>Cancel</button>
                </div>
              </form>
            {:else if !cred?.configured}
              <button class="btn-set-cred" style="color: {brand.color}; border-color: {brand.color}30;" onclick={() => { showCredForm = p.name; }}>
                <Key size={14} />
                Set {p.auth_env_var}
              </button>
            {/if}
          </div>

          <!-- Info Row -->
          <div class="card-divider"></div>
          <div class="info-row">
            <div class="info-chip">
              <Wrench size={12} />
              <span>{p.capabilities?.length || 0} capabilities</span>
            </div>
            {#if p.validation_evidence}
              <div class="info-chip evidence">
                {p.validation_evidence}
              </div>
            {/if}
          </div>

          <!-- Action Footer -->
          <div class="card-divider"></div>
          <div class="card-footer">
            {#if p.state === 'proposed'}
              <button
                class="btn-primary"
                style="background: {brand.gradient};"
                disabled={actionLoading === p.name + ':admit' || !p.credential_set}
                onclick={() => admit(p.name)}
              >
                {actionLoading === p.name + ':admit' ? 'Admitting...' : 'Admit Provider'}
                <ArrowRight size={15} />
              </button>
              {#if !p.credential_set}
                <div class="hint-warn">
                  <TriangleAlert size={13} />
                  <span>Credential required</span>
                </div>
              {/if}
            {:else if p.state === 'admitted'}
              <button class="btn-primary accent-green" disabled={actionLoading === p.name + ':activate'} onclick={() => activate(p.name)}>
                {actionLoading === p.name + ':activate' ? 'Activating...' : 'Activate'}
                <Zap size={15} />
              </button>
              <button class="btn-secondary" disabled={actionLoading === p.name + ':deactivate'} onclick={() => deactivate(p.name)}>
                Deactivate
              </button>
            {:else if p.state === 'active'}
              <div class="live-badge">
                <span class="live-dot"></span>
                <span>Live in routing</span>
              </div>
              <button class="btn-secondary" disabled={actionLoading === p.name + ':deactivate'} onclick={() => deactivate(p.name)}>
                Deactivate
              </button>
            {/if}

            <button class="btn-detail" onclick={() => toggleDetail(p.name)} title="View details">
              {#if isOpen}<ChevronDown size={17} />{:else}<ChevronRight size={17} />{/if}
            </button>
          </div>

          <!-- Expanded Details -->
          {#if isOpen && det}
            <div class="detail-panel" style="border-top: 1px solid var(--brand-border, var(--color-border));">
              <div class="detail-grid">
                <div class="detail-field">
                  <span class="detail-label">Executable</span>
                  <code class="detail-code">{det.default_path}{det.args?.length ? ' ' + det.args.join(' ') : ''}</code>
                </div>
                <div class="detail-field">
                  <span class="detail-label">Auth Method</span>
                  <code class="detail-code">{det.auth_method_id || '—'}</code>
                </div>
                <div class="detail-field">
                  <span class="detail-label">Admitted</span>
                  <span class="detail-value">{fmtDate(det.admitted_at)}</span>
                </div>
                <div class="detail-field">
                  <span class="detail-label">Activated</span>
                  <span class="detail-value">{fmtDate(det.activated_at)}</span>
                </div>
                <div class="detail-field wide">
                  <span class="detail-label">Foreign Env Vars</span>
                  <span class="detail-value">{det.foreign_auth_vars?.join(', ') || '—'}</span>
                </div>
                <div class="detail-field wide">
                  <span class="detail-label">Capabilities</span>
                  <div class="cap-list">
                    {#each (det.capabilities || []) as cap}
                      <span class="cap-chip" style="color: {brand.color}; background: {brand.bg}; border: 1px solid {brand.border};">
                        {cap}
                      </span>
                    {/each}
                    {#if !det.capabilities?.length}
                      <span class="detail-empty">—</span>
                    {/if}
                  </div>
                </div>
              </div>

              {#if det.admission_report?.Results}
                <div class="gates-section">
                  <h4 class="gates-title">Admission Gates</h4>
                  {#each det.admission_report.Results as gate}
                    <div class="gate-row">
                      {#if gate.Outcome === 'pass'}
                        <CircleCheck size={15} style="color: #22c55e; flex-shrink: 0;" />
                      {:else}
                        <X size={15} style="color: #ef4444; flex-shrink: 0;" />
                      {/if}
                      <span class="gate-label">{gate.Gate}</span>
                      <span class="gate-reason">{gate.Reason}</span>
                    </div>
                  {/each}
                </div>
              {/if}

              <!-- Admission error notice -->
              {#if det.validation_evidence?.startsWith('admission-error')}
                <div class="admission-notice">
                  <Info size={14} style="color: #f59e0b;" />
                  <span>{det.validation_evidence}</span>
                </div>
              {/if}
            </div>
          {:else if isOpen}
            <div class="detail-loading">
              <Spinner />
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  /* ── Section Header ── */
  .section-header {
    display: flex; align-items: center; gap: 12px;
    margin-bottom: 20px;
  }
  .header-icon {
    width: 44px; height: 44px; border-radius: 12px;
    background: rgba(139,92,246,0.12);
    display: flex; align-items: center; justify-content: center;
    flex-shrink: 0;
  }
  .header-icon :global(svg) { color: #8b5cf6; }
  .header-title {
    font-size: 23px; font-weight: 750; color: var(--color-fg-0);
    margin: 0; line-height: 1.2;
  }
  .header-desc {
    font-size: 13px; color: var(--color-fg-3); margin: 3px 0 0;
  }

  /* ── Stats Strip ── */
  .stats-strip {
    display: grid; grid-template-columns: repeat(4, 1fr);
    gap: 0; background: var(--color-bg-card);
    border-radius: 12px; overflow: hidden;
    margin-bottom: 24px;
    border: 1px solid var(--color-border);
  }
  .stat-cell {
    display: flex; flex-direction: column; align-items: center;
    gap: 5px; padding: 18px 12px;
    background: var(--color-bg-card);
  }
  .stat-value {
    font-size: 26px; font-weight: 800;
    font-family: var(--font-mono); line-height: 1;
  }
  .stat-label {
    font-size: 10px; font-weight: 600; color: var(--color-fg-3);
    text-transform: uppercase; letter-spacing: 0.8px;
  }

  /* ── Error Banner ── */
  .error-banner {
    display: flex; align-items: center; gap: 10px;
    background: rgba(239,68,68,0.08); color: #ef4444;
    padding: 12px 16px; border-radius: 10px; margin-bottom: 18px;
    font-size: 13px; font-weight: 500;
    border: 1px solid rgba(239,68,68,0.15);
  }
  .error-close {
    margin-left: auto; background: none; border: none;
    cursor: pointer; color: inherit; padding: 2px;
    display: flex; align-items: center;
  }

  /* ── Loading ── */
  .loading-state {
    display: flex; flex-direction: column; align-items: center;
    padding: 60px 0; gap: 12px; color: var(--color-fg-3);
    font-size: 14px;
  }

  /* ── Provider Grid ── */
  .provider-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
    gap: 22px;
  }

  /* ── Provider Card ── */
  .provider-card {
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    border-radius: 16px;
    overflow: hidden;
    transition: border-color 0.2s, box-shadow 0.25s, transform 0.2s;
    position: relative;
  }
  .provider-card:hover {
    border-color: var(--brand-border, var(--color-border));
    box-shadow: 0 2px 8px rgba(0,0,0,0.04);
  }
  .provider-card.expanded {
    border-color: var(--brand-border, var(--color-primary));
    box-shadow: 0 0 0 1px var(--brand-color, var(--color-primary))18;
  }

  /* ── Card Header ── */
  .card-header {
    display: flex; align-items: flex-start; gap: 14px;
    padding: 22px 22px 0;
  }
  .brand-icon-box {
    width: 60px; height: 60px; border-radius: 16px;
    display: flex; align-items: center; justify-content: center;
    flex-shrink: 0;
    background: var(--brand-bg, rgba(59,130,246,0.08));
  }
  .card-titles {
    flex: 1; min-width: 0;
  }
  .provider-name {
    font-size: 17px; font-weight: 700; color: var(--color-fg-0);
    margin-bottom: 5px; letter-spacing: -0.3px;
    text-transform: lowercase;
    font-family: var(--font-mono);
  }
  .company-tag {
    display: inline-flex; align-items: center;
    padding: 3px 9px; border-radius: 7px; border: 1px solid;
    font-size: 10px; font-weight: 700; text-transform: uppercase;
    letter-spacing: 0.6px;
    opacity: 0.75;
  }
  .state-pill {
    display: flex; align-items: center; gap: 5px;
    padding: 5px 11px; border-radius: 20px;
    font-size: 11px; font-weight: 700; text-transform: uppercase;
    letter-spacing: 0.4px; flex-shrink: 0;
    white-space: nowrap;
  }

  /* ── Card Body ── */
  .card-body {
    padding: 14px 22px 16px;
  }
  .tagline {
    font-size: 13px; color: var(--color-fg-3);
    margin: 0 0 10px; line-height: 1.4;
  }
  .feature-pills {
    display: flex; flex-wrap: wrap; gap: 6px;
  }
  .feature-pill {
    padding: 4px 9px; border-radius: 7px;
    font-size: 10px; font-weight: 600;
    letter-spacing: 0.2px;
    opacity: 0.7;
  }

  /* ── Divider ── */
  .card-divider {
    height: 1px; background: var(--color-border);
    margin: 0;
  }

  /* ── Credential Area ── */
  .cred-area {
    padding: 16px 22px;
    display: flex; flex-direction: column; gap: 10px;
  }
  .cred-label-row {
    display: flex; align-items: center; gap: 6px;
    font-size: 11px; font-weight: 600; color: var(--color-fg-3);
    text-transform: uppercase; letter-spacing: 0.5px;
  }
  .cred-configured {
    display: flex; align-items: center; justify-content: space-between;
    flex-wrap: wrap; gap: 8px;
  }
  .cred-left {
    display: flex; align-items: center; gap: 8px;
    flex-wrap: wrap;
  }
  .cred-source {
    padding: 3px 9px; border-radius: 7px;
    font-size: 10px; font-weight: 700; text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .cred-masked {
    font-size: 11px; color: var(--color-fg-2);
    background: var(--color-bg-3);
    padding: 4px 9px; border-radius: 5px;
    font-family: var(--font-mono);
    max-width: 160px; overflow: hidden; text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cred-empty {
    display: flex; align-items: center; gap: 7px;
    font-size: 12px; color: var(--color-fg-4); font-style: italic;
  }
  .cred-form {
    display: flex; flex-direction: column; gap: 8px;
  }
  .cred-field {
    width: 100%; padding: 9px 13px; border-radius: 9px;
    border: 1px solid var(--color-border);
    background: var(--color-bg-1); color: var(--color-fg-1);
    font-size: 13px; font-family: var(--font-mono);
    outline: none; box-sizing: border-box;
    transition: border-color 0.2s, box-shadow 0.2s;
  }
  .cred-field:focus {
    border-color: var(--brand-color, var(--color-primary));
    box-shadow: 0 0 0 3px rgba(59,130,246,0.1);
  }
  .cred-form-actions {
    display: flex; gap: 8px;
  }

  /* ── Buttons ── */
  .btn-ghost {
    padding: 5px 11px; border-radius: 7px; border: none;
    background: var(--color-bg-3); color: var(--color-fg-2);
    font-size: 11px; font-weight: 600; cursor: pointer;
    transition: background 0.15s;
  }
  .btn-ghost:hover { background: var(--color-border); }
  .btn-ghost.danger { color: #ef4444; }
  .btn-ghost.danger:hover { background: rgba(239,68,68,0.08); }
  .btn-ghost:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-ghost.xs { padding: 5px 10px; font-size: 11px; }
  .btn-brand {
    padding: 7px 14px; border-radius: 8px; border: none;
    color: white; font-size: 12px; font-weight: 600;
    cursor: pointer; transition: opacity 0.15s;
  }
  .btn-brand:disabled { opacity: 0.4; cursor: not-allowed; }
  .btn-set-cred {
    display: flex; align-items: center; gap: 7px;
    padding: 10px 16px; border-radius: 10px;
    border: 1.5px dashed; background: transparent;
    font-size: 12px; font-weight: 600; cursor: pointer;
    transition: background 0.15s, border-color 0.15s;
    width: 100%; justify-content: center;
  }
  .btn-set-cred:hover {
    background: var(--brand-bg, var(--color-bg-3));
  }

  /* ── Info Row ── */
  .info-row {
    padding: 12px 22px;
    display: flex; flex-wrap: wrap; gap: 7px;
  }
  .info-chip {
    display: flex; align-items: center; gap: 5px;
    padding: 4px 9px; border-radius: 7px;
    background: var(--color-bg-3); color: var(--color-fg-3);
    font-size: 11px; font-weight: 500;
  }
  .info-chip.evidence {
    font-family: var(--font-mono); font-size: 10px;
    color: var(--color-fg-4);
  }

  /* ── Card Footer ── */
  .card-footer {
    padding: 14px 22px;
    display: flex; align-items: center; gap: 10px;
    flex-wrap: wrap;
  }
  .btn-primary {
    flex: 1; padding: 10px 18px; border-radius: 10px;
    border: none; color: white; font-size: 13px;
    font-weight: 650; cursor: pointer;
    display: flex; align-items: center; justify-content: center; gap: 6px;
    transition: opacity 0.15s, transform 0.1s;
    letter-spacing: -0.1px;
  }
  .btn-primary:disabled { opacity: 0.4; cursor: not-allowed; }
  .btn-primary:active:not(:disabled) { transform: scale(0.97); }
  .btn-primary.accent-green { background: #22c55e; }
  .btn-secondary {
    padding: 9px 16px; border-radius: 10px; border: none;
    background: var(--color-bg-3); color: var(--color-fg-2);
    font-size: 12px; font-weight: 600; cursor: pointer;
    transition: background 0.15s;
  }
  .btn-secondary:hover { background: var(--color-border); }
  .btn-secondary:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-detail {
    width: 34px; height: 34px; border-radius: 9px;
    border: none; background: var(--color-bg-3);
    color: var(--color-fg-2); cursor: pointer;
    display: flex; align-items: center; justify-content: center;
    transition: background 0.15s; margin-left: auto;
  }
  .btn-detail:hover { background: var(--color-border); }
  .hint-warn {
    display: flex; align-items: center; gap: 5px;
    font-size: 11px; color: #f59e0b; font-weight: 500;
  }
  .live-badge {
    flex: 1; display: flex; align-items: center; gap: 7px;
    font-size: 13px; font-weight: 650; color: #22c55e;
  }
  .live-dot {
    width: 8px; height: 8px; border-radius: 50%;
    background: #22c55e; box-shadow: 0 0 6px rgba(34,197,94,0.5);
    animation: pulse-live 2s ease-in-out infinite;
  }
  @keyframes pulse-live {
    0%, 100% { opacity: 1; box-shadow: 0 0 6px rgba(34,197,94,0.5); }
    50% { opacity: 0.5; box-shadow: 0 0 2px rgba(34,197,94,0.3); }
  }

  /* ── Expand Detail ── */
  .detail-panel {
    padding: 18px 22px 20px;
    animation: fadeInScale 0.2s ease-out;
  }
  .detail-grid {
    display: grid; grid-template-columns: 1fr 1fr;
    gap: 14px; margin-bottom: 16px;
  }
  .detail-field {
    display: flex; flex-direction: column; gap: 4px;
  }
  .detail-field.wide { grid-column: 1 / -1; }
  .detail-label {
    font-size: 9px; color: var(--color-fg-4);
    text-transform: uppercase; letter-spacing: 0.8px; font-weight: 600;
  }
  .detail-value { font-size: 12px; color: var(--color-fg-1); }
  .detail-code {
    font-size: 11px; background: var(--color-bg-3);
    padding: 5px 9px; border-radius: 5px;
    font-family: var(--font-mono); color: var(--color-fg-2);
    word-break: break-all;
  }
  .detail-empty {
    font-size: 12px; color: var(--color-fg-4);
  }
  .cap-list {
    display: flex; flex-wrap: wrap; gap: 5px;
  }
  .cap-chip {
    padding: 3px 8px; border-radius: 5px;
    font-size: 10px; font-weight: 600;
  }
  .gates-section { margin-top: 16px; }
  .gates-title {
    font-size: 11px; font-weight: 600; color: var(--color-fg-3);
    margin: 0 0 10px; text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .gate-row {
    display: flex; align-items: flex-start; gap: 7px;
    font-size: 12px; padding: 5px 0;
  }
  .gate-label {
    font-weight: 600; color: var(--color-fg-1);
    min-width: 80px; flex-shrink: 0;
  }
  .gate-reason {
    color: var(--color-fg-3); word-break: break-word;
  }
  .admission-notice {
    display: flex; align-items: flex-start; gap: 8px;
    margin-top: 12px; padding: 10px 12px;
    background: rgba(245,158,11,0.06);
    border-radius: 8px; border: 1px solid rgba(245,158,11,0.15);
    font-size: 12px; color: #d97706;
    font-family: var(--font-mono);
  }
  .detail-loading {
    padding: 20px; display: flex; justify-content: center;
  }

  /* ── Animations ── */
  @keyframes fadeInScale {
    from { opacity: 0; transform: scale(0.98); }
    to { opacity: 1; transform: scale(1); }
  }
</style>
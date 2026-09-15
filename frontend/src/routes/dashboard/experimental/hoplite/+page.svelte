<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import TabNav from '$lib/components/TabNav.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import { Cloud, Key, Lock, RefreshCw, Play, GitPullRequest, ExternalLink, Trash2, CircleCheck, CircleAlert, ArrowLeft, Copy, ArrowRight, FolderGit2, ChevronDown } from 'lucide-svelte';

  const tabs = [
    { label: 'Accounts', path: '/dashboard/connections' },
    { label: 'Discover', path: '/dashboard/discover' },
    { label: 'OAuth IDE', path: '/dashboard/oauth-ide', lab: true },
    { label: 'Experimental', path: '/dashboard/experimental', lab: true },
    { label: 'Hoplite Cloud', path: '/dashboard/experimental/hoplite', lab: true }
  ];

  type Status = { configured: boolean; source: string; masked_value: string; env_var: string; mode: string; routing: string };
  type AdapterModel = { id: string; object: string; owned_by: string; display_name?: string; hoplite_project_id?: string; hoplite_project_name?: string; hoplite_account_id?: string; hoplite_model_id?: string; provider?: string; context_window_tokens?: number; catalog_eligibility?: string; catalog_revision?: string };
  type ConnectionTest = { ok: boolean; checked_at: string; latency_ms: number; project_count: number; verified_operations: string[]; thread_create: string; meta?: { rate_limit?: string; rate_limit_policy?: string; request_id?: string } };
  type Project = { id: string; name: string; description?: string; defaultBranch?: string; defaultModel?: string; agentSpeed?: 'standard' | 'fast'; repos?: { repoFullName: string; branch?: string }[] };
  type PullRequest = { url?: string; number?: number; state?: string };
  type Thread = { id: string; projectId: string; title?: string; status: string; modelId?: string; runStatus?: string; updatedAt?: string; pullRequests?: PullRequest[] };
  type Message = { id: string; role: string; content: string; createdAt?: string };

  let status = $state<Status | null>(null);
  let accountID = $state('');
  let connectionTest = $state<ConnectionTest | null>(null);
  let projects = $state<Project[]>([]);
  let adapterModels = $state<AdapterModel[]>([]);
  let threads = $state<Thread[]>([]);
  let selectedProject = $state('');
  let selectedThread = $state<Thread | null>(null);
  let messages = $state<Message[]>([]);
  let credential = $state('');
  let editingCredential = $state(false);
  let prompt = $state('');
  let title = $state('');

  function accountQuery(prefix = '?'): string {
    return accountID ? `${prefix}account_id=${encodeURIComponent(accountID)}` : '';
  }
  let speed = $state<'standard' | 'fast'>('standard');
  let modelMode = $state<'default' | 'catalog' | 'custom'>('default');
  let catalogModel = $state('');
  let customModel = $state('');
  let autoFix = $state(false);
  let autoMerge = $state(false);
  let pendingOperationId = $state('');
  let pendingOperationFingerprint = $state('');
  let loading = $state(true);
  let action = $state('');
  let error = $state('');
  let notice = $state('');
  let connectionError = $state('');
  let agentTestThread = $state<Thread | null>(null);
  let agentTestMessages = $state<Message[]>([]);
  let agentTestRequestedModel = $state('');
  let polling = false;
  let pollTimer: ReturnType<typeof setInterval> | undefined;

  onMount(() => {
    accountID = page.url.searchParams.get('account_id')?.trim() || '';
    void loadStatus();
    pollTimer = setInterval(() => {
      if (agentTestThread && !isTerminal(agentTestThread.status) && !polling) void refreshAgentTest(true);
      else if (selectedThread && !polling) void refreshThread(selectedThread, true);
    }, 5000);
  });

  onDestroy(() => {
    if (pollTimer) clearInterval(pollTimer);
  });

  function messageOf(e: any) {
    return e?.message || 'Request failed';
  }

  function selectedProjectData() {
    return projects.find((project) => project.id === selectedProject);
  }

  function selectedModel() {
    if (modelMode === 'catalog') return catalogModel;
    return modelMode === 'custom' ? customModel.trim() : '';
  }

  function modelLabel() {
    if (modelMode === 'catalog') return adapterModels.find((model) => model.hoplite_model_id === catalogModel)?.display_name || 'Select a catalog model';
    if (modelMode === 'custom') return customModel.trim() || 'Custom model not entered';
    return selectedProjectData()?.defaultModel || 'Hoplite project default';
  }

  function adapterModelLabel() {
    return adapterModels.map((model) => model.id).join(' · ');
  }

  function projectModelID(project: Project) {
    const advertised = adapterModels.find((model) =>
      model.hoplite_project_id === project.id &&
      model.hoplite_account_id === (accountID || 'hoplite-cloud-agent') &&
      model.catalog_eligibility === 'project-default'
    );
    if (advertised) return advertised.id;
    if (accountID) return '';
    return `hoplite-agent/${project.id}`;
  }

  function projectModels(project?: Project) {
    if (!project) return [];
    return adapterModels.filter((model) => model.hoplite_project_id === project.id && !!model.hoplite_model_id);
  }

  function formatContext(tokens?: number) {
    if (!tokens) return 'Unknown';
    return tokens >= 1_000_000 ? `${tokens / 1_000_000}M` : `${tokens / 1_000}k`;
  }

  function eligibilityLabel(value?: string) {
    if (value === 'free+pro') return 'Free + Pro';
    if (value === 'free-only') return 'Free only';
    if (value === 'free-via-contributor+pro') return 'Free via Contributor · Pro regular';
    return value === 'pro' ? 'Pro only' : 'Unknown';
  }

  async function copyAdapterModelID(modelID: string) {
    try {
      await navigator.clipboard.writeText(modelID);
      notice = `Copied ${modelID}`;
    } catch {
      error = 'Could not copy the model ID. Select and copy it manually.';
    }
  }

  async function copyModelID(project: Project) {
    const modelID = projectModelID(project);
    try {
      await navigator.clipboard.writeText(modelID);
      notice = `Copied ${modelID}`;
    } catch {
      error = 'Could not copy the model ID. Select and copy it manually.';
    }
  }

  function isTerminal(value: string) {
    return ['ready', 'failed', 'blocked', 'archived'].includes(value);
  }

  function testVerdict(value: string) {
    if (value === 'ready') return 'PASS';
    if (['failed', 'blocked'].includes(value)) return 'FAIL';
    if (value === 'archived') return 'STOPPED';
    return 'RUNNING';
  }

  async function loadStatus() {
    loading = true; error = '';
    try {
      const res = await api.get<{ data: Status }>(`/api/experimental/cloud-agents/hoplite/status${accountQuery()}`);
      status = res.data;
      loading = false;
      if (status.configured) await testConnection();
    } catch (e: any) { error = messageOf(e); }
    finally { loading = false; }
  }

  async function saveCredential() {
    if (!credential.trim()) return;
    action = 'credential'; error = ''; notice = '';
    try {
      if (accountID) {
        await api.patch(`/api/experimental/cloud-agents/hoplite/accounts/${accountID}`, { credential: credential.trim() });
      } else {
        await api.put('/api/experimental/credentials/hoplite', { credential: credential.trim() });
      }
      credential = '';
      editingCredential = false;
      notice = 'Credential encrypted and saved.';
      await loadStatus();
    } catch (e: any) { error = messageOf(e); }
    finally { action = ''; }
  }

  async function deleteCredential() {
    action = 'credential'; error = ''; notice = '';
    try {
      if (accountID) {
        throw new Error('Delete this account from Connections to confirm removal of its encrypted credential.');
      }
      await api.delete('/api/experimental/credentials/hoplite');
      projects = []; threads = []; selectedProject = '';
      notice = 'Dashboard credential removed.';
      await loadStatus();
    } catch (e: any) { error = messageOf(e); }
    finally { action = ''; }
  }

  async function testConnection() {
    action = 'test'; error = ''; notice = ''; connectionError = ''; connectionTest = null;
    try {
      const res = await api.post<{ data: ConnectionTest }>(`/api/experimental/cloud-agents/hoplite/test${accountQuery()}`, {});
      connectionTest = res.data;
      notice = `Connection verified — ${res.data.project_count} project(s), ${res.data.latency_ms} ms.`;
      await loadProjects();
    } catch (e: any) {
      connectionError = messageOf(e);
      projects = [];
      threads = [];
      selectedProject = '';
    } finally { action = ''; }
  }

  async function loadProjects() {
    const res = await api.get<{ data: { projects: Project[] } }>(`/api/experimental/cloud-agents/hoplite/projects${accountQuery()}`);
    projects = res.data.projects || [];
    const modelsRes = await api.get<{ data: AdapterModel[] }>('/v1/models');
    adapterModels = (modelsRes.data || []).filter((model) => {
      if (accountID) return model.hoplite_account_id === accountID;
      return !model.hoplite_account_id || model.hoplite_account_id === 'hoplite-cloud-agent';
    });
    if (!selectedProject && projects.length) selectedProject = projects[0].id;
    const project = selectedProjectData();
    if (project?.agentSpeed) speed = project.agentSpeed;
    if (selectedProject) await loadThreads();
  }

  async function loadThreads() {
    selectedThread = null;
    messages = [];
    if (!selectedProject) {
      threads = [];
      return;
    }
    const project = selectedProjectData();
    if (project?.agentSpeed) speed = project.agentSpeed;
    action = 'threads'; error = '';
    try {
      const res = await api.get<{ data: { threads: Thread[] } }>(`/api/experimental/cloud-agents/hoplite/threads?projectId=${encodeURIComponent(selectedProject)}${accountQuery('&')}`);
      threads = res.data.threads || [];
    } catch (e: any) { error = messageOf(e); }
    finally { action = ''; }
  }

  async function createThread() {
    if (!selectedProject || !prompt.trim()) return;
    action = 'create'; error = ''; notice = '';
    const operationPayload = {
      projectId: selectedProject,
      prompt: prompt.trim(),
      title: title.trim(),
      speed,
      model: selectedModel(),
      autoFix,
      autoMerge
    };
    const fingerprint = JSON.stringify(operationPayload);
    if (!pendingOperationId || pendingOperationFingerprint !== fingerprint) {
      pendingOperationId = `lintasan-${crypto.randomUUID()}`;
      pendingOperationFingerprint = fingerprint;
    }
    try {
      const res = await api.post<{ data: { result: { thread: Thread } } }>(`/api/experimental/cloud-agents/hoplite/threads${accountQuery()}`, {
        ...operationPayload,
        clientOperationId: pendingOperationId
      });
      prompt = ''; title = '';
      pendingOperationId = '';
      pendingOperationFingerprint = '';
      notice = `Thread ${res.data.result.thread.id} queued.`;
      await loadThreads();
    } catch (e: any) { error = messageOf(e); }
    finally { action = ''; }
  }

  async function runAgentTest() {
    if (!selectedProject || (modelMode === 'catalog' && !catalogModel) || (modelMode === 'custom' && !customModel.trim())) return;
    action = 'agent-test'; error = ''; notice = '';
    try {
      const res = await api.post<{ data: { result: { thread: Thread } } }>(`/api/experimental/cloud-agents/hoplite/threads${accountQuery()}`, {
        projectId: selectedProject,
        title: `Lintasan model test · ${modelLabel()}`,
        prompt: 'Read the repository context and report a concise readiness summary. Do not modify files, do not create commits, and do not open or merge a pull request.',
        speed,
        model: selectedModel(),
        autoFix: false,
        autoMerge: false,
        clientOperationId: `lintasan-test-${crypto.randomUUID()}`
      });
      agentTestThread = res.data.result.thread;
      agentTestMessages = [];
      agentTestRequestedModel = modelLabel();
      notice = `Real agent test queued as ${agentTestThread.id}.`;
      await refreshAgentTest(false);
    } catch (e: any) { error = messageOf(e); }
    finally { action = ''; }
  }

  async function refreshAgentTest(silent: boolean) {
    if (!agentTestThread || polling) return;
    polling = true;
    try {
      const id = encodeURIComponent(agentTestThread.id);
      const [threadRes, messageRes] = await Promise.all([
        api.get<{ data: { thread: Thread } }>(`/api/experimental/cloud-agents/hoplite/threads/${id}${accountQuery()}`),
        api.get<{ data: { messages: Message[] } }>(`/api/experimental/cloud-agents/hoplite/threads/${id}/messages${accountQuery()}`)
      ]);
      agentTestThread = threadRes.data.thread;
      agentTestMessages = messageRes.data.messages || [];
    } catch (e: any) {
      if (!silent) error = messageOf(e);
    } finally { polling = false; }
  }

  async function inspectThread(item: Thread) {
    selectedThread = item;
    await refreshThread(item, false);
  }

  async function refreshThread(item: Thread, silent: boolean) {
    if (polling) return;
    polling = true;
    action = `thread:${item.id}`; error = '';
    try {
      const [threadRes, messageRes] = await Promise.all([
        api.get<{ data: { thread: Thread } }>(`/api/experimental/cloud-agents/hoplite/threads/${encodeURIComponent(item.id)}${accountQuery()}`),
        api.get<{ data: { messages: Message[] } }>(`/api/experimental/cloud-agents/hoplite/threads/${encodeURIComponent(item.id)}/messages${accountQuery()}`)
      ]);
      if (selectedThread?.id === item.id) {
        selectedThread = threadRes.data.thread;
        messages = messageRes.data.messages || [];
      }
    } catch (e: any) {
      if (!silent) error = messageOf(e);
    } finally {
      polling = false;
      if (action === `thread:${item.id}`) action = '';
    }
  }

  function statusTone(value: string) {
    if (['succeeded', 'completed', 'ready', 'merged'].includes(value)) return 'good';
    if (['failed', 'blocked', 'cancelled'].includes(value)) return 'bad';
    return 'active';
  }
</script>

<svelte:head><title>Hoplite Cloud Agent — Lintasan</title></svelte:head>
<TabNav {tabs} />

<div class="page">
  <header>
    <a class="back" href="/dashboard/experimental"><ArrowLeft size={14} /> Experimental</a>
    <div class="headline">
      <div class="logo"><Cloud size={25} /></div>
      <div><h1>Hoplite Cloud Agent</h1><p>Hoplite runs coding-agent tasks for your repositories. Responses may take several minutes.</p></div>
    </div>
  </header>

  {#if error}<div class="banner error"><CircleAlert size={16} /> {error}</div>{/if}
  {#if notice}<div class="banner success"><CircleCheck size={16} /> {notice}</div>{/if}

  {#if loading}
    <div class="loading"><Spinner /> Loading Hoplite...</div>
  {:else}
    <div class="steps" aria-label="Hoplite setup progress">
      <div class="step" class:complete={status?.configured}><span>{status?.configured ? '✓' : '1'}</span><div><small>Step 1</small><strong>Connect Hoplite</strong></div></div>
      <div class="step-line" class:complete={status?.configured}></div>
      <div class="step" class:active={status?.configured && !connectionTest} class:complete={!!connectionTest}><span>{connectionTest ? '✓' : '2'}</span><div><small>Step 2</small><strong>Detect projects</strong></div></div>
      <div class="step-line" class:complete={projects.length > 0}></div>
      <div class="step" class:complete={projects.length > 0}><span>{projects.length ? '✓' : '3'}</span><div><small>Step 3</small><strong>Ready to use</strong></div></div>
    </div>

    <section class="panel credential-panel">
      <div class="step-heading"><div class="step-icon"><Key size={19} /></div><div><div class="eyebrow">Step 1</div><h2>Connect Hoplite</h2><p>Enter your Hoplite API key to discover project-backed models.</p></div></div>
      {#if status?.configured}
        <div class="credential-state"><Lock size={15} /><code>{status.masked_value}</code><span>{status.source}</span></div>
        <div class="actions">
          <button class="secondary" disabled={!!action} onclick={testConnection}><Play size={14} /> {action === 'test' ? 'Testing...' : 'Test connection'}</button>
          <button class="secondary" disabled={!!action} onclick={() => { editingCredential = !editingCredential; credential = ''; }}><Key size={14} /> Update key</button>
          {#if !accountID && status.source === 'dashboard'}<button class="danger" disabled={!!action} onclick={deleteCredential}><Trash2 size={14} /> Remove</button>{/if}
        </div>
        {#if editingCredential}
          <form class="credential-form update-form" onsubmit={(e) => { e.preventDefault(); saveCredential(); }}>
            <label class="sr-only" for="hoplite-update-key">New Hoplite API key</label>
            <input id="hoplite-update-key" type="password" bind:value={credential} placeholder="New hop_... key" autocomplete="off" />
            <button class="primary" disabled={!credential.trim() || !!action}>{action === 'credential' ? 'Connecting...' : 'Connect'}</button>
          </form>
        {/if}
      {:else}
        <form class="credential-form" onsubmit={(e) => { e.preventDefault(); saveCredential(); }}>
          <label class="sr-only" for="hoplite-key">Hoplite API key</label>
          <Key size={15} /><input id="hoplite-key" type="password" bind:value={credential} placeholder="hop_..." autocomplete="off" />
          <button class="primary" disabled={!credential.trim() || !!action}>{action === 'credential' ? 'Connecting...' : 'Connect'}</button>
        </form>
      {/if}
      <p class="security-note"><Lock size={13} /> Your key is encrypted at rest and never returned to this browser after saving.</p>
    </section>

    <section class="panel detection-panel" class:muted={!status?.configured}>
      <div class="step-heading"><div class="step-icon blue"><RefreshCw size={19} /></div><div><div class="eyebrow">Step 2</div><h2>Detect projects</h2><p>Lintasan verifies project access, then combines each project with the exact model IDs from Hoplite's versioned app contract.</p></div></div>
      {#if !status?.configured}
        <div class="inline-state"><span class="state-dot"></span>Waiting for a Hoplite connection</div>
      {:else if action === 'test'}
        <div class="inline-state active"><Spinner /><div><strong>Detecting projects...</strong><span>Testing the connection and fetching your projects.</span></div></div>
      {:else if connectionError}
        <div class="inline-state failed"><CircleAlert size={18} /><div><strong>We couldn't detect projects</strong><span>{connectionError}</span></div><button class="secondary" onclick={testConnection}><RefreshCw size={14} /> Retry detection</button></div>
      {:else if connectionTest}
        <div class="inline-state success"><CircleCheck size={18} /><div><strong>{connectionTest.project_count} project{connectionTest.project_count === 1 ? '' : 's'} detected</strong><span>Connection verified in {connectionTest.latency_ms} ms.</span></div><button class="secondary" onclick={testConnection}><RefreshCw size={14} /> Refresh</button></div>
      {:else}
        <button class="secondary" onclick={testConnection}><RefreshCw size={14} /> Detect projects</button>
      {/if}
    </section>

    <section class="ready-section">
      <div class="section-title ready-title"><div><div class="eyebrow">Step 3</div><h2>Ready to use</h2><p>Choose a real Hoplite model grouped by project. Catalog eligibility is informational. Available to your account is confirmed only when Hoplite accepts a thread.</p></div>{#if projects.length}<span class="ready-count"><CircleCheck size={14} /> {projects.length} project{projects.length === 1 ? '' : 's'}</span>{/if}</div>
      {#if projects.length}
        <div class="project-grid">
          {#each projects as project}
            <article class="project-card">
              <div class="project-head"><div class="project-icon"><FolderGit2 size={19} /></div><span class="ready-badge"><span></span>Ready</span></div>
              <div><h3>{project.name}</h3>{#if project.description}<p>{project.description}</p>{/if}</div>
              <div class="model-id"><small>Project-default alias</small><code>{projectModelID(project)}</code></div>
              <div class="project-actions"><button class="secondary" onclick={() => copyModelID(project)}><Copy size={14} /> Copy model ID</button><a class="primary link-button" href={`/dashboard/playground?model=${encodeURIComponent(projectModelID(project))}`}><Play size={14} /> Test in Chat</a></div>
              <div class="model-list">
                {#each projectModels(project) as model}
                  <div class="model-choice">
                    <div><strong>{model.display_name}</strong><span>{model.provider} · Context window {formatContext(model.context_window_tokens)} · Catalog eligibility: {eligibilityLabel(model.catalog_eligibility)}</span></div>
                    <code>{model.id}</code>
                    <div class="project-actions"><button class="secondary" onclick={() => copyAdapterModelID(model.id)}><Copy size={14} /> Copy Model ID</button><a class="primary link-button" href={`/dashboard/playground?model=${encodeURIComponent(model.id)}`}><Play size={14} /> Test in Chat</a></div>
                  </div>
                {/each}
              </div>
            </article>
          {/each}
        </div>
      {:else}
        <div class="empty-ready"><Cloud size={24} /><strong>Connect Hoplite to activate models in Lintasan Proxy.</strong><span>Project model cards appear here after a successful connection.</span></div>
      {/if}
    </section>

    {#if status?.configured}
      <details class="advanced">
        <summary><span><span class="advanced-icon"><ChevronDown size={16} /></span><span><strong>Advanced tools</strong><small>Connection diagnostics, quota-aware tests, and thread controls</small></span></span></summary>
        <div class="advanced-content">
      <section class="panel diagnostics-panel">
        <div class="section-title">
          <div><div class="eyebrow">Connection diagnostics</div><h2>{connectionTest?.ok ? 'Connected to Hoplite' : connectionError ? 'Connection failed' : 'Not verified yet'}</h2></div>
          <button class="secondary" disabled={!!action} onclick={testConnection}><RefreshCw size={14} /> {action === 'test' ? 'Testing...' : 'Run connection test'}</button>
        </div>
        <div class="adapter-note">
          <strong>OpenAI adapter uses agent semantics</strong>
          <span>Each non-streaming request creates one coding-agent thread, waits up to four minutes, and returns the terminal assistant chat plus thread/PR metadata. Streaming is explicitly rejected. Auto-fix and auto-merge are always disabled.</span>
          {#if adapterModels.length}<code>{adapterModelLabel()}</code>{/if}
        </div>
        {#if connectionTest}
          <div class="diagnostic-grid">
            <div><span>Status</span><strong class="good-text">Connected</strong></div>
            <div><span>Latency</span><strong>{connectionTest.latency_ms} ms</strong></div>
            <div><span>Projects</span><strong>{connectionTest.project_count}</strong></div>
            <div><span>Last checked</span><strong>{new Date(connectionTest.checked_at).toLocaleString()}</strong></div>
            <div><span>Read capability</span><strong>{connectionTest.verified_operations.join(', ')}</strong></div>
            <div><span>Write capability</span><strong>thread-create not tested</strong></div>
          </div>
          <div class="rate-row"><span>Rate limit</span><code>{connectionTest.meta?.rate_limit || 'Not returned by Hoplite'}</code></div>
        {:else if connectionError}
          <div class="diagnostic-failure"><CircleAlert size={16} /><div><strong>Failed</strong><span>{connectionError}</span></div></div>
        {:else}
          <p class="diagnostic-empty">Run the read-only connection test to verify authentication, project access, latency, and rate-limit metadata.</p>
        {/if}
      </section>

      <section class="panel agent-test-panel">
        <div class="section-title"><div><div class="eyebrow">Agent test</div><h2>Run real agent test</h2></div><span class="quota-warning">Uses Hoplite quota</span></div>
        <p>This creates a real Hoplite thread and may use quota. The fixed prompt requests a read-only report, but Hoplite exposes no dry-run guarantee; autoFix: false and autoMerge: false.</p>
        <div class="test-controls">
          <label>Project<select bind:value={selectedProject} onchange={loadThreads}><option value="">Select project</option>{#each projects as p}<option value={p.id}>{p.name}</option>{/each}</select></label>
          <label>Model source<select bind:value={modelMode}><option value="default">Project default</option><option value="catalog">Hoplite catalog</option><option value="custom">Advanced custom ID</option></select></label>
          {#if modelMode === 'catalog'}<label>Hoplite model<select bind:value={catalogModel}><option value="">Select model</option>{#each projectModels(selectedProjectData()) as m}<option value={m.hoplite_model_id}>{m.display_name} · {eligibilityLabel(m.catalog_eligibility)}</option>{/each}</select></label>{/if}
          {#if modelMode === 'custom'}<label>Custom model ID<input bind:value={customModel} placeholder="Exact Hoplite model ID" /></label>{/if}
          <label>Resolved model<input value={modelLabel()} readonly /></label>
          <button class="primary" disabled={!selectedProject || (modelMode === 'catalog' && !catalogModel) || (modelMode === 'custom' && !customModel.trim()) || !!action} onclick={runAgentTest}><Play size={14} /> {action === 'agent-test' ? 'Starting...' : 'Run real agent test'}</button>
        </div>
        {#if agentTestThread}
          <div class="test-result">
            <div class="test-result-head">
              <div><span>Agent test result</span><strong>{testVerdict(agentTestThread.status)}</strong></div>
              <code>{agentTestThread.id}</code>
              <span class:good={testVerdict(agentTestThread.status) === 'PASS'} class:bad={testVerdict(agentTestThread.status) === 'FAIL'}>{agentTestThread.status}</span>
            </div>
            <div class="test-meta"><span>Requested: {agentTestRequestedModel}</span><span>Actual: {agentTestThread.modelId || 'pending/not returned'}</span><span>Run: {agentTestThread.runStatus || agentTestThread.status}</span></div>
            <div class="messages compact">{#each agentTestMessages as msg}<article><span>{msg.role}</span><p>{msg.content}</p></article>{/each}</div>
          </div>
        {/if}
      </section>

      <div class="workspace">
        <section class="panel composer">
          <div class="section-title"><div><div class="eyebrow">New task</div><h2>Queue coding thread</h2></div><span class="safe">Explicit controls</span></div>
          <label>Project<select bind:value={selectedProject} onchange={loadThreads}><option value="">Select project</option>{#each projects as p}<option value={p.id}>{p.name}</option>{/each}</select></label>
          <label>Title <span>optional</span><input bind:value={title} maxlength="500" placeholder="Fix failing checkout test" /></label>
          <label>Prompt<textarea bind:value={prompt} rows="6" placeholder="Describe the goal, constraints, and verification required..."></textarea></label>
          <label>Speed<select bind:value={speed}><option value="standard">Standard</option><option value="fast">Fast</option></select></label>
          <label>Model source<select bind:value={modelMode}><option value="default">Project default</option><option value="catalog">Hoplite catalog</option><option value="custom">Advanced custom ID</option></select></label>
          {#if modelMode === 'catalog'}<label>Hoplite model<select bind:value={catalogModel}><option value="">Select model</option>{#each projectModels(selectedProjectData()) as m}<option value={m.hoplite_model_id}>{m.display_name} · {eligibilityLabel(m.catalog_eligibility)}</option>{/each}</select></label>{/if}
          {#if modelMode === 'custom'}<label>Custom model ID<input bind:value={customModel} placeholder="Exact Hoplite model ID" /></label>{/if}
          <small class="model-hint">Resolved model: {modelLabel()}. Catalog eligibility does not guarantee account access; advanced custom IDs are validated only when a real thread starts.</small>
          <div class="toggles">
            <label class="toggle"><input type="checkbox" bind:checked={autoFix} /><span><strong>Automatic PR fixes</strong><small>Allow Hoplite to address review and check feedback.</small></span></label>
            <label class="toggle"><input type="checkbox" bind:checked={autoMerge} /><span><strong>Automatic PR merge</strong><small>Merge only after Hoplite's required checks succeed.</small></span></label>
          </div>
          <button class="primary wide" disabled={!selectedProject || !prompt.trim() || !!action} onclick={createThread}><Play size={15} /> {action === 'create' ? 'Queueing...' : 'Create thread'}</button>
        </section>

        <section class="panel threads-panel">
          <div class="section-title"><div><div class="eyebrow">Activity</div><h2>Project threads</h2></div><button class="icon-btn" disabled={!selectedProject || !!action} onclick={loadThreads} title="Refresh"><RefreshCw size={15} /></button></div>
          {#if threads.length === 0}<div class="empty">No threads in this project.</div>{/if}
          <div class="thread-list">
            {#each threads as item}
              <button class="thread" class:selected={selectedThread?.id === item.id} onclick={() => inspectThread(item)}>
                <div><strong>{item.title || item.id}</strong><small>{item.id}</small></div><span class:good={statusTone(item.status) === 'good'} class:bad={statusTone(item.status) === 'bad'}>{item.status}</span>
              </button>
            {/each}
          </div>
          {#if selectedThread}
            <div class="detail">
              <div class="detail-head"><strong>{selectedThread.title || selectedThread.id}</strong><span>{selectedThread.status}</span></div>
              {#each selectedThread.pullRequests || [] as pr}
                {#if pr.url}<a class="pr" href={pr.url} target="_blank" rel="noreferrer"><GitPullRequest size={14} /> PR #{pr.number || ''}<ExternalLink size={12} /></a>{/if}
              {/each}
              <div class="messages">{#each messages as msg}<article><span>{msg.role}</span><p>{msg.content}</p></article>{/each}</div>
            </div>
          {/if}
        </section>
      </div>
        </div>
      </details>
    {/if}
  {/if}
</div>

<style>
  .page{animation:fadeInUp .35s ease}.back{display:inline-flex;align-items:center;gap:5px;color:var(--color-fg-3);font-size:12px;text-decoration:none;margin-bottom:14px}.headline{display:flex;align-items:center;gap:13px;margin-bottom:22px}.logo{width:48px;height:48px;border-radius:14px;background:linear-gradient(135deg,#111827,#334155);color:white;display:grid;place-items:center}.headline h1,.panel h2{margin:0;color:var(--color-fg-0)}.headline h1{font-size:23px}.headline p,.panel p{margin:4px 0 0;color:var(--color-fg-3);font-size:13px}.safe{margin-left:auto;display:flex;align-items:center;gap:5px;padding:6px 9px;border-radius:8px;background:rgba(34,197,94,.09);color:#16a34a;font-size:11px;font-weight:650}.panel{background:var(--color-bg-card);border:1px solid var(--color-border);border-radius:15px;padding:20px}.credential-panel{display:flex;align-items:center;gap:16px;margin-bottom:18px;flex-wrap:wrap}.credential-panel>div:first-child{flex:1}.eyebrow{text-transform:uppercase;letter-spacing:.08em;color:var(--color-fg-4);font-size:10px;font-weight:700;margin-bottom:4px}.credential-state{display:flex;align-items:center;gap:8px;color:#16a34a}.credential-state code{color:var(--color-fg-2);background:var(--color-bg-3);padding:6px 9px;border-radius:7px}.credential-state span{font-size:10px;text-transform:uppercase}.actions,.credential-form{display:flex;align-items:center;gap:8px}.credential-form{min-width:390px}.update-form{flex-basis:100%;margin-left:auto;max-width:520px}.diagnostics-panel,.agent-test-panel{margin-bottom:18px}.adapter-note{display:flex;flex-direction:column;gap:5px;padding:12px;margin:0 0 14px;border:1px solid rgba(59,130,246,.2);border-radius:10px;background:rgba(59,130,246,.07);color:var(--color-fg-2)}.adapter-note span{font-size:12px;line-height:1.5;color:var(--color-fg-3)}.adapter-note code{font-size:11px;overflow-wrap:anywhere}.diagnostic-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.diagnostic-grid>div{display:flex;flex-direction:column;gap:5px;padding:12px;border:1px solid var(--color-border);border-radius:10px;background:var(--color-bg-2)}.diagnostic-grid span,.rate-row span,.test-result-head span,.test-meta span{font-size:10px;color:var(--color-fg-4);text-transform:uppercase;letter-spacing:.05em}.diagnostic-grid strong{font-size:12px;color:var(--color-fg-1);word-break:break-word}.good-text{color:#16a34a!important}.rate-row{display:flex;align-items:center;gap:10px;margin-top:10px}.rate-row code{font-size:11px;color:var(--color-fg-2);background:var(--color-bg-3);padding:6px 8px;border-radius:7px}.diagnostic-empty{padding:10px 0}.diagnostic-failure{display:flex;gap:9px;align-items:flex-start;color:#ef4444;background:rgba(239,68,68,.08);padding:12px;border-radius:9px}.diagnostic-failure div{display:flex;flex-direction:column;gap:3px}.diagnostic-failure span{font-size:11px}.quota-warning{margin-left:auto;padding:5px 8px;border-radius:7px;background:rgba(245,158,11,.12);color:#d97706;font-size:10px;font-weight:700}.test-controls{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;align-items:end;margin-top:14px}.test-controls label{display:flex;flex-direction:column;gap:6px;font-size:10px;font-weight:700;color:var(--color-fg-3);text-transform:uppercase}.test-controls input,.test-controls select{border:1px solid var(--color-border);background:var(--color-bg-1);color:var(--color-fg-1);border-radius:9px;padding:10px;font:inherit;text-transform:none}.test-result{margin-top:14px;padding:14px;border:1px solid var(--color-border);border-radius:11px;background:var(--color-bg-2)}.test-result-head{display:flex;align-items:center;gap:12px}.test-result-head div{display:flex;flex-direction:column;gap:2px}.test-result-head code{margin-left:auto;font-size:10px}.test-result-head>span{padding:4px 7px;border-radius:7px;background:rgba(59,130,246,.1);color:#3b82f6}.test-result-head>span.good{background:rgba(34,197,94,.1);color:#16a34a}.test-result-head>span.bad{background:rgba(239,68,68,.1);color:#ef4444}.test-meta{display:flex;gap:14px;flex-wrap:wrap;margin-top:9px}.messages.compact{max-height:220px}.model-hint{color:var(--color-fg-4);font-size:10px;line-height:1.5}.workspace{display:grid;grid-template-columns:minmax(320px,.85fr) minmax(360px,1.15fr);gap:18px}.section-title{display:flex;align-items:center;justify-content:space-between;margin-bottom:16px}.section-title h2,.panel h2{font-size:16px}.composer{display:flex;flex-direction:column;gap:13px}.composer .section-title{margin-bottom:0}.composer label{display:flex;flex-direction:column;gap:6px;font-size:11px;font-weight:650;color:var(--color-fg-3);text-transform:uppercase;letter-spacing:.04em}.composer label span{font-weight:400;text-transform:none}.composer input,.composer select,.composer textarea,.credential-form input{border:1px solid var(--color-border);background:var(--color-bg-1);color:var(--color-fg-1);border-radius:9px;padding:10px 11px;font:inherit;text-transform:none;letter-spacing:normal;outline:none}.composer textarea{resize:vertical;min-height:110px}.toggles{display:grid;grid-template-columns:1fr 1fr;gap:8px}.composer .toggle{display:flex;flex-direction:row;align-items:flex-start;gap:8px;padding:10px;border:1px solid var(--color-border);border-radius:9px;background:var(--color-bg-2);text-transform:none;letter-spacing:normal}.toggle input{width:auto;margin-top:3px}.toggle span{display:flex;flex-direction:column;gap:2px}.toggle strong{font-size:11px;color:var(--color-fg-1)}.toggle small{font-size:10px;color:var(--color-fg-4);font-weight:400}.composer input:focus,.composer select:focus,.composer textarea:focus,.credential-form input:focus{border-color:var(--color-primary);box-shadow:0 0 0 3px color-mix(in srgb,var(--color-primary) 12%,transparent)}button{font:inherit}.primary,.secondary,.danger,.icon-btn{border:0;border-radius:9px;padding:9px 13px;display:inline-flex;align-items:center;justify-content:center;gap:6px;font-size:12px;font-weight:650;cursor:pointer}.primary{background:var(--color-primary);color:white}.secondary,.icon-btn{background:var(--color-bg-3);color:var(--color-fg-1)}.danger{background:rgba(239,68,68,.08);color:#ef4444}.wide{width:100%}button:disabled{opacity:.45;cursor:not-allowed}.thread-list{display:flex;flex-direction:column;gap:7px}.thread{width:100%;border:1px solid var(--color-border);background:var(--color-bg-1);border-radius:10px;padding:11px;display:flex;align-items:center;text-align:left;cursor:pointer;color:var(--color-fg-1)}.thread.selected{border-color:var(--color-primary)}.thread div{display:flex;flex-direction:column;gap:3px;min-width:0}.thread strong{font-size:12px}.thread small{font-family:var(--font-mono);color:var(--color-fg-4)}.thread>span{margin-left:auto;font-size:10px;padding:4px 7px;border-radius:7px;background:rgba(59,130,246,.1);color:#3b82f6}.thread>span.good{background:rgba(34,197,94,.1);color:#16a34a}.thread>span.bad{background:rgba(239,68,68,.1);color:#ef4444}.detail{border-top:1px solid var(--color-border);margin-top:16px;padding-top:16px}.detail-head{display:flex;justify-content:space-between;font-size:12px}.pr{display:flex;align-items:center;gap:5px;color:var(--color-primary);font-size:12px;margin-top:10px;text-decoration:none}.messages{display:flex;flex-direction:column;gap:8px;margin-top:12px;max-height:330px;overflow:auto}.messages article{background:var(--color-bg-2);border-radius:9px;padding:10px}.messages span{font-size:9px;text-transform:uppercase;color:var(--color-fg-4);font-weight:700}.messages p{white-space:pre-wrap;word-break:break-word}.banner{display:flex;align-items:center;gap:8px;padding:10px 13px;border-radius:9px;margin-bottom:12px;font-size:12px}.banner.error{background:rgba(239,68,68,.08);color:#ef4444}.banner.success{background:rgba(34,197,94,.08);color:#16a34a}.loading,.empty{padding:35px;text-align:center;color:var(--color-fg-4);font-size:12px}.loading{display:flex;justify-content:center;gap:8px}.icon-btn{padding:8px}@media(max-width:900px){.workspace{grid-template-columns:1fr}.diagnostic-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.test-controls{grid-template-columns:repeat(2,minmax(0,1fr))}.credential-panel{align-items:stretch;flex-direction:column}.credential-form{min-width:0;width:100%}.headline{align-items:flex-start;flex-wrap:wrap}.safe{margin-left:8px}}@media(max-width:560px){.credential-form,.actions{flex-direction:column;align-items:stretch}.panel{padding:15px}}
  .steps{display:flex;align-items:center;max-width:760px;margin:0 auto 22px;padding:0 10px}.step{display:flex;align-items:center;gap:9px;min-width:145px;color:var(--color-fg-4)}.step>span{display:grid;place-items:center;width:30px;height:30px;border-radius:50%;border:1px solid var(--color-border);background:var(--color-bg-card);font-size:12px;font-weight:750}.step div{display:flex;flex-direction:column}.step small{font-size:9px;text-transform:uppercase;letter-spacing:.08em}.step strong{font-size:12px;color:var(--color-fg-2)}.step.complete>span{border-color:#22c55e;background:#22c55e;color:#fff}.step.complete strong{color:var(--color-fg-0)}.step.active>span{border-color:#3b82f6;color:#2563eb;box-shadow:0 0 0 3px rgba(59,130,246,.1)}.step-line{height:1px;flex:1;background:var(--color-border);margin:0 12px}.step-line.complete{background:#86efac}
  .step-heading{display:flex;align-items:center;gap:12px;flex:1}.step-icon{display:grid;place-items:center;min-width:38px;height:38px;border-radius:11px;color:#7c3aed;background:rgba(124,58,237,.1)}.step-icon.blue{color:#2563eb;background:rgba(37,99,235,.1)}.security-note{display:flex;align-items:center;gap:5px;flex-basis:100%;font-size:11px!important;margin:0!important;padding-left:50px}.detection-panel{margin-bottom:22px;display:flex;align-items:center;gap:18px;flex-wrap:wrap}.detection-panel .step-heading{min-width:280px}.detection-panel.muted{opacity:.72}.inline-state{display:flex;align-items:center;gap:10px;margin-left:auto;padding:10px 12px;border-radius:10px;background:var(--color-bg-3);color:var(--color-fg-3);font-size:12px}.inline-state div{display:flex;flex-direction:column;gap:2px}.inline-state span{font-size:11px}.inline-state.success{color:#15803d;background:rgba(34,197,94,.09)}.inline-state.failed{color:#dc2626;background:rgba(239,68,68,.08)}.inline-state.active{color:#2563eb;background:rgba(59,130,246,.08)}.state-dot{width:8px;height:8px;border-radius:50%;background:var(--color-fg-4)}
  .ready-section{margin-bottom:22px;padding:4px}.ready-title{margin:0 0 13px}.ready-title p{margin:4px 0 0;color:var(--color-fg-3);font-size:12px}.ready-count,.ready-badge{display:flex;align-items:center;gap:5px;color:#15803d;background:rgba(34,197,94,.09);border-radius:999px;padding:6px 9px;font-size:11px;font-weight:700}.project-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:12px}.project-card{display:flex;flex-direction:column;gap:14px;min-width:0;padding:17px;border:1px solid var(--color-border);border-radius:14px;background:var(--color-bg-card);box-shadow:0 5px 18px rgba(15,23,42,.035)}.project-head{display:flex;justify-content:space-between;align-items:center}.project-icon{display:grid;place-items:center;width:38px;height:38px;border-radius:10px;color:#2563eb;background:rgba(59,130,246,.09)}.ready-badge span{width:6px;height:6px;border-radius:50%;background:#22c55e}.project-card h3{margin:0;color:var(--color-fg-0);font-size:15px}.project-card p{margin:4px 0 0;color:var(--color-fg-3);font-size:12px}.model-id{padding:10px;border-radius:9px;background:var(--color-bg-3);overflow:hidden}.model-id small{display:block;margin-bottom:4px;color:var(--color-fg-4);text-transform:uppercase;letter-spacing:.06em;font-size:9px}.model-id code{font-size:11px;color:var(--color-fg-1);overflow-wrap:anywhere}.project-actions{display:flex;gap:8px;margin-top:auto}.project-actions>*{flex:1;justify-content:center}.link-button{text-decoration:none}.model-list{display:flex;flex-direction:column;gap:10px;padding-top:4px;border-top:1px solid var(--color-border)}.model-choice{display:flex;flex-direction:column;gap:7px;padding:11px;border:1px solid var(--color-border);border-radius:10px;background:var(--color-bg-2)}.model-choice>div:first-child{display:flex;flex-direction:column;gap:2px}.model-choice strong{font-size:12px;color:var(--color-fg-1)}.model-choice span{font-size:10px;color:var(--color-fg-3)}.model-choice code{font-size:10px;color:var(--color-fg-2);overflow-wrap:anywhere}.model-choice .project-actions>*{font-size:10px;padding:6px 8px}.empty-ready{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:7px;min-height:145px;padding:24px;border:1px dashed var(--color-border);border-radius:14px;background:var(--color-bg-card);color:var(--color-fg-3);text-align:center}.empty-ready strong{color:var(--color-fg-1);font-size:13px}.empty-ready span{font-size:11px}.advanced{margin-top:10px;border:1px solid var(--color-border);border-radius:14px;background:var(--color-bg-card);overflow:hidden}.advanced>summary{cursor:pointer;list-style:none;padding:15px 18px}.advanced>summary::-webkit-details-marker{display:none}.advanced>summary>span{display:flex;align-items:center;gap:10px}.advanced>summary strong,.advanced>summary small{display:block}.advanced>summary strong{font-size:13px;color:var(--color-fg-1)}.advanced>summary small{margin-top:2px;font-size:11px;color:var(--color-fg-3)}.advanced-icon{display:grid;place-items:center;width:30px;height:30px;border-radius:8px;background:var(--color-bg-3);transition:transform .2s}.advanced[open] .advanced-icon{transform:rotate(180deg)}.advanced-content{padding:0 16px 16px}.advanced-content>.panel{border-color:var(--color-border);box-shadow:none}.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
  @media(max-width:700px){.steps{align-items:flex-start;padding:0}.step{min-width:0;flex:1;justify-content:center}.step div{display:none}.step-line{margin:15px 6px}.credential-panel,.detection-panel{align-items:stretch}.step-heading{min-width:100%!important}.security-note{padding-left:0}.inline-state{margin-left:0;width:100%;flex-wrap:wrap}.inline-state button{width:100%;justify-content:center}.project-grid{grid-template-columns:1fr}.project-actions{flex-direction:column}.project-actions>*{min-height:42px}.advanced-content{padding:0 10px 10px}}
</style>

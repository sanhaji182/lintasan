<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { api } from '$lib/api';
  import TabNav from '$lib/components/TabNav.svelte';
  import Spinner from '$lib/components/Spinner.svelte';
  import { Cloud, Key, Lock, RefreshCw, Play, GitPullRequest, ExternalLink, ShieldCheck, Trash2, CircleCheck, CircleAlert, ArrowLeft } from 'lucide-svelte';

  const tabs = [
    { label: 'Accounts', path: '/dashboard/connections' },
    { label: 'Discover', path: '/dashboard/discover' },
    { label: 'OAuth IDE', path: '/dashboard/oauth-ide', lab: true },
    { label: 'Experimental', path: '/dashboard/experimental', lab: true },
    { label: 'Hoplite Cloud', path: '/dashboard/experimental/hoplite', lab: true }
  ];

  type Status = { configured: boolean; source: string; masked_value: string; env_var: string; mode: string; routing: string };
  type Project = { id: string; name: string; description?: string; defaultBranch?: string; repos?: { repoFullName: string; branch?: string }[] };
  type PullRequest = { url?: string; number?: number; state?: string };
  type Thread = { id: string; projectId: string; title?: string; status: string; updatedAt?: string; pullRequests?: PullRequest[] };
  type Message = { id: string; role: string; content: string; createdAt?: string };

  let status = $state<Status | null>(null);
  let projects = $state<Project[]>([]);
  let threads = $state<Thread[]>([]);
  let selectedProject = $state('');
  let selectedThread = $state<Thread | null>(null);
  let messages = $state<Message[]>([]);
  let credential = $state('');
  let editingCredential = $state(false);
  let prompt = $state('');
  let title = $state('');
  let speed = $state<'standard' | 'fast'>('standard');
  let autoFix = $state(false);
  let autoMerge = $state(false);
  let pendingOperationId = $state('');
  let pendingOperationFingerprint = $state('');
  let loading = $state(true);
  let action = $state('');
  let error = $state('');
  let notice = $state('');
  let polling = false;
  let pollTimer: ReturnType<typeof setInterval> | undefined;

  onMount(() => {
    void loadStatus();
    pollTimer = setInterval(() => {
      if (selectedThread && !polling) void refreshThread(selectedThread, true);
    }, 5000);
  });

  onDestroy(() => {
    if (pollTimer) clearInterval(pollTimer);
  });

  function messageOf(e: any) {
    return e?.message || 'Request failed';
  }

  async function loadStatus() {
    loading = true; error = '';
    try {
      const res = await api.get<{ data: Status }>('/api/experimental/cloud-agents/hoplite/status');
      status = res.data;
      if (status.configured) await loadProjects();
    } catch (e: any) { error = messageOf(e); }
    finally { loading = false; }
  }

  async function saveCredential() {
    if (!credential.trim()) return;
    action = 'credential'; error = ''; notice = '';
    try {
      await api.put('/api/experimental/credentials/hoplite', { credential: credential.trim() });
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
      await api.delete('/api/experimental/credentials/hoplite');
      projects = []; threads = []; selectedProject = '';
      notice = 'Dashboard credential removed.';
      await loadStatus();
    } catch (e: any) { error = messageOf(e); }
    finally { action = ''; }
  }

  async function testConnection() {
    action = 'test'; error = ''; notice = '';
    try {
      const res = await api.post<{ data: { project_count: number } }>('/api/experimental/cloud-agents/hoplite/test', {});
      notice = `Connected — ${res.data.project_count} project(s) available.`;
      await loadProjects();
    } catch (e: any) { error = messageOf(e); }
    finally { action = ''; }
  }

  async function loadProjects() {
    const res = await api.get<{ data: { projects: Project[] } }>('/api/experimental/cloud-agents/hoplite/projects');
    projects = res.data.projects || [];
    if (!selectedProject && projects.length) selectedProject = projects[0].id;
    if (selectedProject) await loadThreads();
  }

  async function loadThreads() {
    selectedThread = null;
    messages = [];
    if (!selectedProject) {
      threads = [];
      return;
    }
    action = 'threads'; error = '';
    try {
      const res = await api.get<{ data: { threads: Thread[] } }>(`/api/experimental/cloud-agents/hoplite/threads?projectId=${encodeURIComponent(selectedProject)}`);
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
      autoFix,
      autoMerge
    };
    const fingerprint = JSON.stringify(operationPayload);
    if (!pendingOperationId || pendingOperationFingerprint !== fingerprint) {
      pendingOperationId = `lintasan-${crypto.randomUUID()}`;
      pendingOperationFingerprint = fingerprint;
    }
    try {
      const res = await api.post<{ data: { result: { thread: Thread } } }>('/api/experimental/cloud-agents/hoplite/threads', {
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
        api.get<{ data: { thread: Thread } }>(`/api/experimental/cloud-agents/hoplite/threads/${encodeURIComponent(item.id)}`),
        api.get<{ data: { messages: Message[] } }>(`/api/experimental/cloud-agents/hoplite/threads/${encodeURIComponent(item.id)}/messages`)
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
      <div><h1>Hoplite Cloud Agent</h1><p>Repository coding jobs in isolated cloud sandboxes.</p></div>
      <span class="isolation"><ShieldCheck size={13} /> Isolated from LLM routing</span>
    </div>
  </header>

  {#if error}<div class="banner error"><CircleAlert size={16} /> {error}</div>{/if}
  {#if notice}<div class="banner success"><CircleCheck size={16} /> {notice}</div>{/if}

  {#if loading}
    <div class="loading"><Spinner /> Loading Hoplite...</div>
  {:else}
    <section class="panel credential-panel">
      <div><div class="eyebrow">Credential</div><h2>Server-side API access</h2><p>The key is encrypted at rest and never returned to this browser after saving.</p></div>
      {#if status?.configured}
        <div class="credential-state"><Lock size={15} /><code>{status.masked_value}</code><span>{status.source}</span></div>
        <div class="actions">
          <button class="secondary" disabled={!!action} onclick={testConnection}><Play size={14} /> {action === 'test' ? 'Testing...' : 'Test connection'}</button>
          <button class="secondary" disabled={!!action} onclick={() => { editingCredential = !editingCredential; credential = ''; }}><Key size={14} /> Update key</button>
          {#if status.source === 'dashboard'}<button class="danger" disabled={!!action} onclick={deleteCredential}><Trash2 size={14} /> Remove</button>{/if}
        </div>
        {#if editingCredential}
          <form class="credential-form update-form" onsubmit={(e) => { e.preventDefault(); saveCredential(); }}>
            <input type="password" bind:value={credential} placeholder="New hop_... key" autocomplete="off" />
            <button class="primary" disabled={!credential.trim() || !!action}>{action === 'credential' ? 'Saving...' : 'Replace key'}</button>
          </form>
        {/if}
      {:else}
        <form class="credential-form" onsubmit={(e) => { e.preventDefault(); saveCredential(); }}>
          <Key size={15} /><input type="password" bind:value={credential} placeholder="hop_..." autocomplete="off" />
          <button class="primary" disabled={!credential.trim() || !!action}>{action === 'credential' ? 'Saving...' : 'Save encrypted key'}</button>
        </form>
      {/if}
    </section>

    {#if status?.configured}
      <div class="workspace">
        <section class="panel composer">
          <div class="section-title"><div><div class="eyebrow">New task</div><h2>Queue coding thread</h2></div><span class="safe">Explicit controls</span></div>
          <label>Project<select bind:value={selectedProject} onchange={loadThreads}><option value="">Select project</option>{#each projects as p}<option value={p.id}>{p.name}</option>{/each}</select></label>
          <label>Title <span>optional</span><input bind:value={title} maxlength="500" placeholder="Fix failing checkout test" /></label>
          <label>Prompt<textarea bind:value={prompt} rows="6" placeholder="Describe the goal, constraints, and verification required..."></textarea></label>
          <label>Speed<select bind:value={speed}><option value="standard">Standard</option><option value="fast">Fast</option></select></label>
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
    {/if}
  {/if}
</div>

<style>
  .page{animation:fadeInUp .35s ease}.back{display:inline-flex;align-items:center;gap:5px;color:var(--color-fg-3);font-size:12px;text-decoration:none;margin-bottom:14px}.headline{display:flex;align-items:center;gap:13px;margin-bottom:22px}.logo{width:48px;height:48px;border-radius:14px;background:linear-gradient(135deg,#111827,#334155);color:white;display:grid;place-items:center}.headline h1,.panel h2{margin:0;color:var(--color-fg-0)}.headline h1{font-size:23px}.headline p,.panel p{margin:4px 0 0;color:var(--color-fg-3);font-size:13px}.isolation,.safe{margin-left:auto;display:flex;align-items:center;gap:5px;padding:6px 9px;border-radius:8px;background:rgba(34,197,94,.09);color:#16a34a;font-size:11px;font-weight:650}.panel{background:var(--color-bg-card);border:1px solid var(--color-border);border-radius:15px;padding:20px}.credential-panel{display:flex;align-items:center;gap:16px;margin-bottom:18px;flex-wrap:wrap}.credential-panel>div:first-child{flex:1}.eyebrow{text-transform:uppercase;letter-spacing:.08em;color:var(--color-fg-4);font-size:10px;font-weight:700;margin-bottom:4px}.credential-state{display:flex;align-items:center;gap:8px;color:#16a34a}.credential-state code{color:var(--color-fg-2);background:var(--color-bg-3);padding:6px 9px;border-radius:7px}.credential-state span{font-size:10px;text-transform:uppercase}.actions,.credential-form{display:flex;align-items:center;gap:8px}.credential-form{min-width:390px}.update-form{flex-basis:100%;margin-left:auto;max-width:520px}.workspace{display:grid;grid-template-columns:minmax(320px,.85fr) minmax(360px,1.15fr);gap:18px}.section-title{display:flex;align-items:center;justify-content:space-between;margin-bottom:16px}.section-title h2,.panel h2{font-size:16px}.composer{display:flex;flex-direction:column;gap:13px}.composer .section-title{margin-bottom:0}.composer label{display:flex;flex-direction:column;gap:6px;font-size:11px;font-weight:650;color:var(--color-fg-3);text-transform:uppercase;letter-spacing:.04em}.composer label span{font-weight:400;text-transform:none}.composer input,.composer select,.composer textarea,.credential-form input{border:1px solid var(--color-border);background:var(--color-bg-1);color:var(--color-fg-1);border-radius:9px;padding:10px 11px;font:inherit;text-transform:none;letter-spacing:normal;outline:none}.composer textarea{resize:vertical;min-height:110px}.toggles{display:grid;grid-template-columns:1fr 1fr;gap:8px}.composer .toggle{display:flex;flex-direction:row;align-items:flex-start;gap:8px;padding:10px;border:1px solid var(--color-border);border-radius:9px;background:var(--color-bg-2);text-transform:none;letter-spacing:normal}.toggle input{width:auto;margin-top:3px}.toggle span{display:flex;flex-direction:column;gap:2px}.toggle strong{font-size:11px;color:var(--color-fg-1)}.toggle small{font-size:10px;color:var(--color-fg-4);font-weight:400}.composer input:focus,.composer select:focus,.composer textarea:focus,.credential-form input:focus{border-color:var(--color-primary);box-shadow:0 0 0 3px color-mix(in srgb,var(--color-primary) 12%,transparent)}button{font:inherit}.primary,.secondary,.danger,.icon-btn{border:0;border-radius:9px;padding:9px 13px;display:inline-flex;align-items:center;justify-content:center;gap:6px;font-size:12px;font-weight:650;cursor:pointer}.primary{background:var(--color-primary);color:white}.secondary,.icon-btn{background:var(--color-bg-3);color:var(--color-fg-1)}.danger{background:rgba(239,68,68,.08);color:#ef4444}.wide{width:100%}button:disabled{opacity:.45;cursor:not-allowed}.thread-list{display:flex;flex-direction:column;gap:7px}.thread{width:100%;border:1px solid var(--color-border);background:var(--color-bg-1);border-radius:10px;padding:11px;display:flex;align-items:center;text-align:left;cursor:pointer;color:var(--color-fg-1)}.thread.selected{border-color:var(--color-primary)}.thread div{display:flex;flex-direction:column;gap:3px;min-width:0}.thread strong{font-size:12px}.thread small{font-family:var(--font-mono);color:var(--color-fg-4)}.thread>span{margin-left:auto;font-size:10px;padding:4px 7px;border-radius:7px;background:rgba(59,130,246,.1);color:#3b82f6}.thread>span.good{background:rgba(34,197,94,.1);color:#16a34a}.thread>span.bad{background:rgba(239,68,68,.1);color:#ef4444}.detail{border-top:1px solid var(--color-border);margin-top:16px;padding-top:16px}.detail-head{display:flex;justify-content:space-between;font-size:12px}.pr{display:flex;align-items:center;gap:5px;color:var(--color-primary);font-size:12px;margin-top:10px;text-decoration:none}.messages{display:flex;flex-direction:column;gap:8px;margin-top:12px;max-height:330px;overflow:auto}.messages article{background:var(--color-bg-2);border-radius:9px;padding:10px}.messages span{font-size:9px;text-transform:uppercase;color:var(--color-fg-4);font-weight:700}.messages p{white-space:pre-wrap;word-break:break-word}.banner{display:flex;align-items:center;gap:8px;padding:10px 13px;border-radius:9px;margin-bottom:12px;font-size:12px}.banner.error{background:rgba(239,68,68,.08);color:#ef4444}.banner.success{background:rgba(34,197,94,.08);color:#16a34a}.loading,.empty{padding:35px;text-align:center;color:var(--color-fg-4);font-size:12px}.loading{display:flex;justify-content:center;gap:8px}.icon-btn{padding:8px}@media(max-width:900px){.workspace{grid-template-columns:1fr}.credential-panel{align-items:stretch;flex-direction:column}.credential-form{min-width:0;width:100%}.headline{align-items:flex-start;flex-wrap:wrap}.isolation{margin-left:61px}.safe{margin-left:8px}}@media(max-width:560px){.credential-form,.actions{flex-direction:column;align-items:stretch}.isolation{margin-left:0}.panel{padding:15px}}
</style>

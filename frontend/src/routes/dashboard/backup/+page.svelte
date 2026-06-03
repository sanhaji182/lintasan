<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import StatusBadge from '$lib/components/StatusBadge.svelte';
  import {
    Database, Download, Upload, RotateCcw, Trash2, Clock,
    FileJson, HardDrive, Shield, CheckCircle2, AlertCircle
  } from 'lucide-svelte/icons';

  interface BackupRecord {
    id: string;
    filename: string;
    size: number;
    type: string;
    status: string;
    createdAt: string;
  }

  let backups = $state<BackupRecord[]>([]);
  let loading = $state(true);
  let error = $state('');
  let exporting = $state(false);
  let importing = $state(false);
  let restoringId = $state<string | null>(null);

  async function loadBackups() {
    try {
      const data = await api.get<{ backups: BackupRecord[] }>('/api/backup');
      backups = data.backups || [];
    } catch (e: any) {
      error = e.message || 'Failed to load backups';
    }
  }

  onMount(async () => {
    loading = true;
    await loadBackups();
    loading = false;
  });

  async function exportConfig() {
    exporting = true;
    try {
      const data = await api.post<{ backup: BackupRecord }>('/api/backup/export');
      backups = [data.backup, ...backups];
    } catch (e: any) {
      error = e.message || 'Failed to export configuration';
    }
    exporting = false;
  }

  async function downloadExport(backupId: string) {
    try {
      const res = await api.raw(`/api/backup/${backupId}/download`);
      if (!res.ok) throw new Error('Download failed');
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `lintasan-backup-${backupId}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (e: any) {
      error = e.message || 'Failed to download backup';
    }
  }

  async function importConfig(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    importing = true;
    try {
      const text = await file.text();
      const config = JSON.parse(text);
      await api.post('/api/backup/import', config);
      await loadBackups();
    } catch (e: any) {
      error = e.message || 'Failed to import configuration';
    }
    importing = false;
    input.value = '';
  }

  async function restoreBackup(backupId: string) {
    if (!confirm('This will overwrite your current configuration. Are you sure?')) return;
    restoringId = backupId;
    try {
      await api.post(`/api/backup/${backupId}/restore`);
      backups = backups.map(b =>
        b.id === backupId ? { ...b, status: 'restored' } : b
      );
    } catch (e: any) {
      error = e.message || 'Failed to restore backup';
    }
    restoringId = null;
  }

  async function deleteBackup(backupId: string) {
    if (!confirm('Are you sure you want to delete this backup?')) return;
    try {
      await api.delete(`/api/backup/${backupId}`);
      backups = backups.filter(b => b.id !== backupId);
    } catch (e: any) {
      error = e.message || 'Failed to delete backup';
    }
  }

  function formatSize(bytes: number): string {
    if (bytes >= 1_048_576) return (bytes / 1_048_576).toFixed(1) + ' MB';
    if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return bytes + ' B';
  }

  function formatDate(ts: string): string {
    try {
      return new Date(ts).toLocaleString(undefined, {
        month: 'short', day: 'numeric', year: 'numeric',
        hour: '2-digit', minute: '2-digit'
      });
    } catch {
      return ts;
    }
  }

  function getTypeIcon(type: string) {
    switch (type) {
      case 'full': return HardDrive;
      case 'config': return FileJson;
      case 'security': return Shield;
      default: return Database;
    }
  }
</script>

<svelte:head>
  <title>Backup — Lintasan</title>
</svelte:head>

<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2.5">
      <div
        class="flex items-center justify-center rounded-xl"
        style="width: 40px; height: 40px; background: var(--color-success-light);"
      >
        <Database size={20} style="color: var(--color-success);" stroke-width={1.8} />
      </div>
      <div>
        <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Backup & Restore</div>
        <div style="font-size: 12px; color: var(--color-fg-3);">Export, import and manage configuration backups</div>
      </div>
    </div>
  </div>

  <!-- Export/Import Actions -->
  <div class="grid gap-5" style="grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));">
    <!-- Export Card -->
    <div class="card">
      <div class="flex items-center gap-3" style="margin-bottom: 16px;">
        <div
          class="flex items-center justify-center rounded-lg"
          style="width: 36px; height: 36px; background: var(--color-primary-light);"
        >
          <Download size={18} style="color: var(--color-primary);" />
        </div>
        <div>
          <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Export Configuration</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">Create a snapshot of current settings</div>
        </div>
      </div>
      <button
        class="btn-primary flex items-center gap-1.5"
        onclick={exportConfig}
        disabled={exporting}
        style="width: 100%; justify-content: center;"
      >
        <Download size={14} />
        {exporting ? 'Exporting...' : 'Export Now'}
      </button>
    </div>

    <!-- Import Card -->
    <div class="card">
      <div class="flex items-center gap-3" style="margin-bottom: 16px;">
        <div
          class="flex items-center justify-center rounded-lg"
          style="width: 36px; height: 36px; background: var(--color-warning-light);"
        >
          <Upload size={18} style="color: var(--color-warning);" />
        </div>
        <div>
          <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Import Configuration</div>
          <div style="font-size: 12px; color: var(--color-fg-3);">Restore from a JSON backup file</div>
        </div>
      </div>
      <label class="btn-secondary import-btn" style="width: 100%; justify-content: center; display: flex; align-items: center; gap: 6px; cursor: pointer;">
        <Upload size={14} />
        {importing ? 'Importing...' : 'Choose File'}
        <input
          type="file"
          accept=".json"
          onchange={importConfig}
          disabled={importing}
          style="display: none;"
        />
      </label>
    </div>
  </div>

  <!-- Backup History -->
  <div class="card" style="padding: 0; overflow: hidden;">
    <div
      class="flex items-center justify-between"
      style="padding: 18px 20px; border-bottom: 1px solid var(--color-border);"
    >
      <div class="flex items-center gap-2">
        <Clock size={16} style="color: var(--color-primary);" />
        <span style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Backup History</span>
      </div>
      <button class="btn-secondary" onclick={loadBackups} style="padding: 5px 12px; font-size: 12px;">
        <RotateCcw size={12} style="display: inline; vertical-align: -1px;" />
        Refresh
      </button>
    </div>

    {#if loading}
      <Spinner />
    {:else if backups.length === 0}
      <EmptyState
        icon={Database}
        title="No backups yet"
        description="Create an export to save your current configuration."
      />
    {:else}
      <div style="overflow-x: auto;">
        <table class="backups-table">
          <thead>
            <tr>
              <th style="width: 220px;">Name</th>
              <th style="width: 100px;">Type</th>
              <th style="width: 90px;">Size</th>
              <th style="width: 100px;">Status</th>
              <th style="width: 160px;">Created</th>
              <th style="width: 140px;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each backups as backup, i (backup.id)}
              <tr style="animation: fadeInUp {0.3 + i * 0.03}s ease-out;">
                <td>
                  <div class="flex items-center gap-2.5">
                    <div
                      class="flex items-center justify-center rounded-lg"
                      style="width: 30px; height: 30px; background: var(--color-bg-body); flex-shrink: 0;"
                    >
                      {#if backup.type === 'full'}
                        <HardDrive size={14} style="color: var(--color-fg-2);" />
                      {:else if backup.type === 'config'}
                        <FileJson size={14} style="color: var(--color-fg-2);" />
                      {:else if backup.type === 'security'}
                        <Shield size={14} style="color: var(--color-fg-2);" />
                      {:else}
                        <Database size={14} style="color: var(--color-fg-2);" />
                      {/if}
                    </div>
                    <span class="font-mono" style="font-size: 12px; font-weight: 500; color: var(--color-fg-0);">
                      {backup.filename}
                    </span>
                  </div>
                </td>
                <td>
                  <span class="badge" style="font-size: 10px; padding: 2px 8px; background: var(--color-info-light); color: var(--color-info);">
                    {backup.type}
                  </span>
                </td>
                <td>
                  <span class="font-mono" style="font-size: 12px; color: var(--color-fg-2);">
                    {formatSize(backup.size)}
                  </span>
                </td>
                <td>
                  <StatusBadge status={backup.status} />
                </td>
                <td>
                  <span style="font-size: 12px; color: var(--color-fg-3);">
                    {formatDate(backup.createdAt)}
                  </span>
                </td>
                <td>
                  <div class="flex items-center gap-1">
                    <button
                      class="btn-icon"
                      style="color: var(--color-primary);"
                      onclick={() => downloadExport(backup.id)}
                      title="Download backup"
                    >
                      <Download size={14} />
                    </button>
                    <button
                      class="btn-icon"
                      style="color: var(--color-success);"
                      onclick={() => restoreBackup(backup.id)}
                      disabled={restoringId === backup.id}
                      title="Restore from this backup"
                    >
                      <RotateCcw size={14} class={restoringId === backup.id ? 'spin-icon' : ''} />
                    </button>
                    <button
                      class="btn-icon"
                      style="color: var(--color-error);"
                      onclick={() => deleteBackup(backup.id)}
                      title="Delete backup"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>

      <!-- Table Footer -->
      <div
        class="flex items-center justify-between"
        style="padding: 12px 16px; border-top: 1px solid var(--color-border); background: var(--color-bg-body);"
      >
        <span style="font-size: 12px; color: var(--color-fg-3);">
          {backups.length} backup{backups.length !== 1 ? 's' : ''}
        </span>
      </div>
    {/if}
  </div>

  {#if error}
    <div
      class="flex items-center gap-2"
      style="
        padding: 12px 16px; border-radius: var(--radius-sm);
        background: var(--color-error-light); color: var(--color-error);
        font-size: 13px; font-weight: 500;
      "
    >
      {error}
      <button style="margin-left: auto; cursor: pointer; color: var(--color-error); background: none; border: none;" onclick={() => error = ''}>&times;</button>
    </div>
  {/if}
</div>

<style>
  .backups-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  .backups-table th {
    padding: 10px 16px;
    text-align: left;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--color-fg-3);
    background: var(--color-bg-body);
    border-bottom: 1px solid var(--color-border);
    white-space: nowrap;
  }
  .backups-table td {
    padding: 12px 16px;
    border-bottom: 1px solid var(--color-border-light);
    vertical-align: middle;
  }
  .backups-table tbody tr {
    transition: var(--transition);
  }
  .backups-table tbody tr:hover {
    background: var(--color-primary-light);
  }
  .backups-table tbody tr:last-child td {
    border-bottom: none;
  }
  .btn-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: var(--radius-sm);
    border: none;
    background: transparent;
    cursor: pointer;
    transition: var(--transition);
  }
  .btn-icon:hover {
    background: var(--color-bg-sidebar-hover);
  }
  .btn-icon:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  :global(.spin-icon) {
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  .import-btn {
    font-size: 13px;
  }
</style>

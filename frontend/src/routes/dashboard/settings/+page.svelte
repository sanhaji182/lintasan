<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import {
    Settings as SettingsIcon, Save, Shield, Zap, Globe, Server,
    Key, RotateCcw, Gauge, Wifi, Lock, Link, KeyRound
  } from 'lucide-svelte';
  import { showToast } from '$lib/toast';

  interface SettingsData {
    [key: string]: any;
  }

  let settings = $state<Record<string, any>>({
    master_key: '',
    api_keys: '',
    aliases: ''
  });

  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let success = $state('');

  // --- Danger zone: system reset -------------------------------------------
  // Two modes because they mean very different things. Soft keeps the operator
  // logged in; factory returns the install to first-run and invalidates every
  // API key and session. The typed phrase is the last line of defence, so it
  // differs per mode — a soft confirmation can never trigger a factory wipe.
  type ResetMode = 'soft' | 'factory';

  const RESET_PHRASE: Record<ResetMode, string> = {
    soft: 'RESET',
    factory: 'FACTORY RESET'
  };

  let resetMode = $state<ResetMode | null>(null);
  let resetConfirm = $state('');
  let resetting = $state(false);
  let resetResult = $state<any>(null);

  function openReset(mode: ResetMode) {
    resetMode = mode;
    resetConfirm = '';
    resetResult = null;
  }

  function closeReset() {
    resetMode = null;
    resetConfirm = '';
  }

  async function runReset() {
    if (!resetMode) return;
    if (resetConfirm.trim() !== RESET_PHRASE[resetMode]) return;

    resetting = true;
    try {
      const res = await api.post<any>('/api/system/reset', {
        mode: resetMode,
        confirm: resetConfirm.trim()
      });
      resetResult = res;
      showToast(
        resetMode === 'factory'
          ? 'Factory reset complete — you will be signed out'
          : 'Reset complete',
        'success'
      );
      // A factory reset destroys this session's user and the JWT secret, so
      // staying on the page would just produce 401s. Give the operator a
      // moment to copy the new admin password, then send them to login.
      if (resetMode === 'factory') {
        setTimeout(() => { window.location.href = '/login'; }, 30000);
      } else {
        setTimeout(() => loadSettings(), 500);
      }
    } catch (e: any) {
      showToast(e?.message || 'Reset failed', 'error');
      resetting = false;
      return;
    }
    resetting = false;
    resetMode = null;
  }

  async function loadSettings() {
    try {
      const res = await api.get<{ data: Record<string, any> }>('/api/settings');
      const raw = res.data || res;
      const parsed: Record<string, any> = {};
      for (const [key, value] of Object.entries(raw)) {
        if (typeof value === 'string') {
          try {
            const parsedVal = JSON.parse(value);
            parsed[key] = parsedVal;
          } catch {
            if (key === 'oauth_ide_enabled' || key === 'responses_api_enabled') {
              parsed[key] = value === 'true' || value === '1';
            } else {
              parsed[key] = value;
            }
          }
        } else {
          parsed[key] = value;
        }
      }
      settings = {
        master_key: '',
        api_keys: '',
        aliases: '',
        port: 20180,
        base_url: 'https://lintasan.sans.biz.id',
        log_level: 'info',
        max_retries: 3,
        request_timeout: 30000,
        cache_enabled: true,
        rate_limit_enabled: false,
        cors_enabled: true,
        oauth_ide_enabled: false,
        responses_api_enabled: false,
        ...parsed
      };
    } catch (e: any) {
      error = e.message || 'Failed to load settings';
    }
  }

  onMount(async () => {
    loading = true;
    await loadSettings();
    loading = false;
  });

  async function saveSettings() {
    saving = true;
    error = '';
    success = '';
    try {
      await api.put('/api/settings', settings);
      success = 'Settings saved successfully';
      showToast('Settings saved successfully', 'success');
      setTimeout(() => success = '', 3000);
    } catch (e: any) {
      error = e.message || 'Failed to save settings';
      showToast('Failed to save settings', 'error');
    }
    saving = false;
  }

  async function resetDefaults() {
    if (!confirm('Reset all settings to defaults?')) return;
    settings = {
      master_key: '',
      api_keys: '',
      aliases: ''
    };
  }
</script>

<svelte:head>
  <title>Settings — Lintasan</title>
</svelte:head>

<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2.5">
      <div
        class="flex items-center justify-center rounded-xl"
        style="width: 40px; height: 40px; background: var(--color-primary-light);"
      >
        <SettingsIcon size={20} style="color: var(--color-primary);" stroke-width={1.8} />
      </div>
      <div>
        <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Settings</div>
        <div style="font-size: 12px; color: var(--color-fg-3);">Configure gateway behavior and defaults</div>
      </div>
    </div>
    <div class="flex items-center gap-2">
      <button class="btn-secondary flex items-center gap-1.5" onclick={resetDefaults}>
        <RotateCcw size={14} />
        Reset
      </button>
      <button
        class="btn-primary flex items-center gap-1.5"
        onclick={saveSettings}
        disabled={saving}
      >
        <Save size={14} />
        {saving ? 'Saving...' : 'Save Changes'}
      </button>
    </div>
  </div>

  {#if loading}
    <Spinner />
  {:else}
    <!-- Success/Error messages -->
    {#if success}
      <div
        class="flex items-center gap-2"
        style="
          padding: 12px 16px; border-radius: var(--radius-sm);
          background: var(--color-success-light); color: var(--color-success);
          font-size: 13px; font-weight: 500;
        "
      >
        <Zap size={14} />
        {success}
      </div>
    {/if}

    <!-- Performance Section -->
    <div class="card" style="animation: fadeInUp 0.3s ease-out;">
      <div class="flex items-center gap-2.5" style="margin-bottom: 24px;">
        <div
          class="flex items-center justify-center rounded-lg"
          style="width: 32px; height: 32px; background: var(--color-primary-light);"
        >
          <Gauge size={16} style="color: var(--color-primary);" />
        </div>
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Performance</div>
      </div>

      <div class="settings-group">
        <!-- Cache Toggle -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="flex items-center gap-2">
              <Zap size={14} style="color: var(--color-warning);" />
              <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">Response Caching</span>
            </div>
            <span style="font-size: 12px; color: var(--color-fg-3);">Cache API responses to reduce latency and costs</span>
          </div>
          <button
            class="toggle-btn"
            class:active={settings.cache_enabled ?? true}
            aria-label={`Toggle response caching: ${(settings.cache_enabled ?? true) ? 'enabled' : 'disabled'}`}
            title={`Response caching ${(settings.cache_enabled ?? true) ? 'enabled' : 'disabled'}`}
            onclick={() => settings.cache_enabled = !(settings.cache_enabled ?? true)}
          >
            <div class="toggle-track">
              <div class="toggle-thumb"></div>
            </div>
          </button>
        </div>

        <!-- Rate Limiting Toggle -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="flex items-center gap-2">
              <Shield size={14} style="color: var(--color-error);" />
              <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">Rate Limiting</span>
            </div>
            <span style="font-size: 12px; color: var(--color-fg-3);">Throttle requests to prevent abuse</span>
          </div>
          <button
            class="toggle-btn"
            class:active={settings.rate_limit_enabled ?? true}
            aria-label={`Toggle rate limiting: ${(settings.rate_limit_enabled ?? true) ? 'enabled' : 'disabled'}`}
            title={`Rate limiting ${(settings.rate_limit_enabled ?? true) ? 'enabled' : 'disabled'}`}
            onclick={() => settings.rate_limit_enabled = !(settings.rate_limit_enabled ?? true)}
          >
            <div class="toggle-track">
              <div class="toggle-thumb"></div>
            </div>
          </button>
        </div>

        <!-- CORS Toggle -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="flex items-center gap-2">
              <Globe size={14} style="color: var(--color-info);" />
              <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">CORS Headers</span>
            </div>
            <span style="font-size: 12px; color: var(--color-fg-3);">Allow cross-origin requests from browsers</span>
          </div>
          <button
            class="toggle-btn"
            class:active={settings.cors_enabled ?? true}
            aria-label={`Toggle CORS headers: ${(settings.cors_enabled ?? true) ? 'enabled' : 'disabled'}`}
            title={`CORS headers ${(settings.cors_enabled ?? true) ? 'enabled' : 'disabled'}`}
            onclick={() => settings.cors_enabled = !(settings.cors_enabled ?? true)}
          >
            <div class="toggle-track">
              <div class="toggle-thumb"></div>
            </div>
          </button>
        </div>
      </div>
    </div>

    <!-- Experimental -->
    <div class="card" style="animation: fadeInUp 0.45s ease-out;">
      <div class="flex items-center gap-2.5" style="margin-bottom: 24px;">
        <div
          class="flex items-center justify-center rounded-lg"
          style="width: 32px; height: 32px; background: rgba(139, 92, 246, 0.12);"
        >
          <KeyRound size={16} style="color: #7c3aed;" />
        </div>
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Experimental</div>
      </div>

      <div class="settings-group">
        <div class="setting-row">
          <div class="setting-info">
            <div class="flex items-center gap-2">
              <KeyRound size={14} style="color: #7c3aed;" />
              <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">OAuth IDE (lab)</span>
            </div>
            <span style="font-size: 12px; color: var(--color-fg-3);">
              IDE OAuth flows (Accounts / OAuth IDE). Admin-only; no server restart. Client IDs still via env.
            </span>
          </div>
          <button
            class="toggle-btn"
            class:active={settings.oauth_ide_enabled ?? false}
            aria-label={`OAuth IDE lab: ${(settings.oauth_ide_enabled ?? false) ? 'enabled' : 'disabled'}`}
            title={`OAuth IDE ${(settings.oauth_ide_enabled ?? false) ? 'enabled' : 'disabled'}`}
            onclick={() => settings.oauth_ide_enabled = !(settings.oauth_ide_enabled ?? false)}
          >
            <div class="toggle-track">
              <div class="toggle-thumb"></div>
            </div>
          </button>
        </div>

        <!-- Codex Responses API Toggle -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="flex items-center gap-2">
              <KeyRound size={14} style="color: #7c3aed;" />
              <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">Codex Responses API (lab)</span>
            </div>
            <span style="font-size: 12px; color: var(--color-fg-3);">
              POST /v1/responses — Codex-compatible ingress (streaming + tool calls). Admin-only; no server restart.
            </span>
          </div>
          <button
            class="toggle-btn"
            class:active={settings.responses_api_enabled ?? false}
            aria-label={`Codex Responses API: ${(settings.responses_api_enabled ?? false) ? 'enabled' : 'disabled'}`}
            title={`Codex Responses API ${(settings.responses_api_enabled ?? false) ? 'enabled' : 'disabled'}`}
            onclick={() => settings.responses_api_enabled = !(settings.responses_api_enabled ?? false)}
          >
            <div class="toggle-track">
              <div class="toggle-thumb"></div>
            </div>
          </button>
        </div>
      </div>
    </div>

    <!-- Danger Zone -->
    <div class="card" style="animation: fadeInUp 0.5s ease-out; border-color: rgba(239, 68, 68, 0.35);">
      <div class="flex items-center gap-2.5" style="margin-bottom: 8px;">
        <div
          class="flex items-center justify-center rounded-lg"
          style="width: 32px; height: 32px; background: rgba(239, 68, 68, 0.12);"
        >
          <RotateCcw size={16} style="color: #ef4444;" />
        </div>
        <div style="font-size: 14px; font-weight: 600; color: #ef4444;">Danger Zone</div>
      </div>
      <p style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 20px;">
        A full database backup is written before anything is deleted, and the path is
        returned to you. Recovery is a file copy.
      </p>

      <div class="settings-group">
        <div class="setting-row">
          <div class="setting-info">
            <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">Reset data</span>
            <span style="font-size: 12px; color: var(--color-fg-3);">
              Clears connections, discovered models, request logs, caches and routing config
              (combos, aliases, fallback chains). <strong>Keeps</strong> your login, API keys
              and master key — existing clients keep working.
            </span>
          </div>
          <button class="danger-btn" onclick={() => openReset('soft')}>Reset data</button>
        </div>

        <div class="setting-row">
          <div class="setting-info">
            <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">Factory reset</span>
            <span style="font-size: 12px; color: var(--color-fg-3);">
              Everything above, <strong>plus</strong> users, sessions, master key and JWT secret.
              The install returns to first-run setup: every existing API key stops working and
              you are signed out. A new admin password is generated and shown once.
            </span>
          </div>
          <button class="danger-btn solid" onclick={() => openReset('factory')}>Factory reset</button>
        </div>
      </div>
    </div>

    <!-- Gateway Section -->
    <div class="card" style="animation: fadeInUp 0.4s ease-out;">
      <div class="flex items-center gap-2.5" style="margin-bottom: 24px;">
        <div
          class="flex items-center justify-center rounded-lg"
          style="width: 32px; height: 32px; background: var(--color-success-light);"
        >
          <Server size={16} style="color: var(--color-success);" />
        </div>
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Gateway</div>
      </div>

      <div class="settings-group">
        <!-- Master Key -->
        <div class="setting-row column">
          <label for="setting-master-key" style="font-size: 13px; font-weight: 500; color: var(--color-fg-0); margin-bottom: 4px;">
            <div class="flex items-center gap-2">
              <Key size={14} style="color: var(--color-warning);" />
              Master API Key
            </div>
          </label>
          <span style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 8px;">
            Used to authenticate admin API requests
          </span>
          <input
            id="setting-master-key"
            class="input-field"
            type="password"
            placeholder="Enter master key"
            bind:value={settings.master_key}
          />
        </div>

        <!-- Port -->
        <div class="setting-row column">
          <label for="setting-port" style="font-size: 13px; font-weight: 500; color: var(--color-fg-0); margin-bottom: 4px;">
            <div class="flex items-center gap-2">
              <Wifi size={14} style="color: var(--color-purple);" />
              Port
            </div>
          </label>
          <span style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 8px;">
            The port the gateway listens on
          </span>
          <input
            id="setting-port"
            class="input-field"
            type="number"
            placeholder="3000"
            bind:value={settings.port}
            style="max-width: 200px;"
          />
        </div>

        <!-- Base URL -->
        <div class="setting-row column">
          <label for="setting-base-url" style="font-size: 13px; font-weight: 500; color: var(--color-fg-0); margin-bottom: 4px;">
            <div class="flex items-center gap-2">
              <Link size={14} style="color: var(--color-primary);" />
              Base URL
            </div>
          </label>
          <span style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 8px;">
            The public URL for this gateway instance
          </span>
          <input
            id="setting-base-url"
            class="input-field"
            placeholder="http://localhost:3000"
            bind:value={settings.base_url}
          />
        </div>
      </div>
    </div>

    <!-- Advanced Section -->
    <div class="card" style="animation: fadeInUp 0.5s ease-out;">
      <div class="flex items-center gap-2.5" style="margin-bottom: 24px;">
        <div
          class="flex items-center justify-center rounded-lg"
          style="width: 32px; height: 32px; background: var(--color-warning-light);"
        >
          <Lock size={16} style="color: var(--color-warning);" />
        </div>
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">Advanced</div>
      </div>

      <div class="settings-group">
        <!-- Log Level -->
        <div class="setting-row column">
          <label for="setting-log-level" style="font-size: 13px; font-weight: 500; color: var(--color-fg-0); margin-bottom: 4px;">
            Log Level
          </label>
          <span style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 8px;">
            Controls the verbosity of gateway logs
          </span>
          <select
            id="setting-log-level"
            class="input-field"
            bind:value={settings.log_level}
            style="max-width: 200px;"
          >
            <option value="debug">Debug</option>
            <option value="info">Info</option>
            <option value="warn">Warning</option>
            <option value="error">Error</option>
          </select>
        </div>

        <!-- Max Retries -->
        <div class="setting-row column">
          <label for="setting-max-retries" style="font-size: 13px; font-weight: 500; color: var(--color-fg-0); margin-bottom: 4px;">
            Max Retries
          </label>
          <span style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 8px;">
            Number of retry attempts for failed requests
          </span>
          <input
            id="setting-max-retries"
            class="input-field"
            type="number"
            min="0"
            max="10"
            bind:value={settings.max_retries}
            style="max-width: 200px;"
          />
        </div>

        <!-- Request Timeout -->
        <div class="setting-row column">
          <label for="setting-timeout" style="font-size: 13px; font-weight: 500; color: var(--color-fg-0); margin-bottom: 4px;">
            Request Timeout (ms)
          </label>
          <span style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 8px;">
            Maximum time to wait for an upstream response
          </span>
          <input
            id="setting-timeout"
            class="input-field"
            type="number"
            min="1000"
            max="120000"
            bind:value={settings.request_timeout}
            style="max-width: 200px;"
          />
        </div>
      </div>
    </div>
  {/if}

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

<!-- Reset confirmation dialog. The action button stays disabled until the exact
     phrase is typed, so a mis-click can never destroy an install. -->
{#if resetMode}
  <div
    class="reset-overlay"
    role="button"
    tabindex="-1"
    onclick={(e) => { if (e.target === e.currentTarget && !resetting) closeReset(); }}
    onkeydown={(e) => { if (e.key === 'Escape' && !resetting) closeReset(); }}
  >
    <div class="reset-dialog">
      <h3 style="font-size: 15px; font-weight: 600; color: #ef4444; margin-bottom: 8px;">
        {resetMode === 'factory' ? 'Factory reset' : 'Reset data'}
      </h3>

      <p style="font-size: 13px; color: var(--color-fg-2); line-height: 1.55; margin-bottom: 6px;">
        {#if resetMode === 'factory'}
          This deletes <strong>all providers, models, logs, users and secrets</strong>.
          Every existing API key stops working, all sessions end, and the install
          returns to first-run setup.
        {:else}
          This deletes <strong>all providers, discovered models, request logs, caches
          and routing config</strong>. Your login and API keys are kept.
        {/if}
      </p>
      <p style="font-size: 12px; color: var(--color-fg-3); margin-bottom: 16px;">
        A backup is written first — you can restore by copying that file back.
      </p>

      <label for="reset-confirm-input" style="font-size: 12px; color: var(--color-fg-2);">
        Type <code style="color: #ef4444; font-weight: 600;">{RESET_PHRASE[resetMode]}</code> to confirm
      </label>
      <input
        id="reset-confirm-input"
        class="input-field"
        style="margin-top: 8px;"
        bind:value={resetConfirm}
        disabled={resetting}
        autocomplete="off"
        placeholder={RESET_PHRASE[resetMode]}
      />

      <div class="flex items-center gap-2" style="margin-top: 20px; justify-content: flex-end;">
        <button class="cancel-btn" onclick={closeReset} disabled={resetting}>Cancel</button>
        <button
          class="danger-btn solid"
          disabled={resetting || resetConfirm.trim() !== RESET_PHRASE[resetMode]}
          onclick={runReset}
        >
          {resetting ? 'Resetting…' : (resetMode === 'factory' ? 'Factory reset' : 'Reset data')}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Post-reset receipt. For a factory reset this is the ONLY time the new admin
     password is shown, so it stays until dismissed. -->
{#if resetResult}
  <div class="reset-overlay">
    <div class="reset-dialog">
      <h3 style="font-size: 15px; font-weight: 600; color: var(--color-success); margin-bottom: 12px;">
        Reset complete
      </h3>

      {#if resetResult.admin_password}
        <div style="padding: 12px; border-radius: 8px; background: rgba(239,68,68,0.08); border: 1px solid rgba(239,68,68,0.3); margin-bottom: 16px;">
          <div style="font-size: 12px; color: #ef4444; font-weight: 600; margin-bottom: 6px;">
            Save this now — it is not shown again
          </div>
          <div style="font-size: 13px; color: var(--color-fg-0);">
            Username: <code>{resetResult.admin_username}</code>
          </div>
          <div style="font-size: 13px; color: var(--color-fg-0); word-break: break-all;">
            Password: <code>{resetResult.admin_password}</code>
          </div>
          <div style="font-size: 11px; color: var(--color-fg-3); margin-top: 6px;">
            You must change it at first login. Signing out in 30 seconds.
          </div>
        </div>
      {/if}

      <div style="font-size: 12px; color: var(--color-fg-2); margin-bottom: 4px;">Backup written to</div>
      <code style="font-size: 12px; color: var(--color-fg-0); word-break: break-all;">{resetResult.backup_path}</code>

      <div class="flex items-center" style="margin-top: 20px; justify-content: flex-end;">
        <button
          class="cancel-btn"
          onclick={() => {
            const wasFactory = !!resetResult?.admin_password;
            resetResult = null;
            if (wasFactory) window.location.href = '/login';
          }}
        >Done</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .danger-btn {
    padding: 8px 14px;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    white-space: nowrap;
    border: 1px solid rgba(239, 68, 68, 0.5);
    background: transparent;
    color: #ef4444;
    transition: background 0.15s ease, opacity 0.15s ease;
  }
  .danger-btn:hover:not(:disabled) { background: rgba(239, 68, 68, 0.1); }
  .danger-btn.solid {
    background: #ef4444;
    color: #fff;
    border-color: #ef4444;
  }
  .danger-btn.solid:hover:not(:disabled) { background: #dc2626; }
  .danger-btn:disabled { opacity: 0.45; cursor: not-allowed; }

  .cancel-btn {
    padding: 8px 14px;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid var(--color-border);
    background: transparent;
    color: var(--color-fg-2);
  }
  .cancel-btn:hover:not(:disabled) { background: var(--color-bg-hover); }
  .cancel-btn:disabled { opacity: 0.45; cursor: not-allowed; }

  .reset-overlay {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
    background: rgba(0, 0, 0, 0.55);
    backdrop-filter: blur(2px);
  }
  .reset-dialog {
    width: 100%;
    max-width: 460px;
    padding: 24px;
    border-radius: 14px;
    background: var(--color-bg-card);
    border: 1px solid var(--color-border);
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.35);
  }

  .settings-group {
    display: flex;
    flex-direction: column;
    gap: 20px;
  }
  .setting-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding-bottom: 20px;
    border-bottom: 1px solid var(--color-border-light);
  }
  .setting-row:last-child {
    padding-bottom: 0;
    border-bottom: none;
  }
  .setting-row.column {
    flex-direction: column;
    align-items: flex-start;
  }
  .setting-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .toggle-btn {
    background: none;
    border: none;
    cursor: pointer;
    padding: 0;
    flex-shrink: 0;
  }
  .toggle-track {
    width: 44px;
    height: 24px;
    background: var(--color-border);
    border-radius: 12px;
    position: relative;
    transition: var(--transition);
  }
  .toggle-btn.active .toggle-track {
    background: var(--color-primary);
  }
  .toggle-thumb {
    width: 18px;
    height: 18px;
    background: white;
    border-radius: 50%;
    position: absolute;
    top: 3px;
    left: 3px;
    transition: var(--transition);
    box-shadow: var(--shadow-sm);
  }
  .toggle-btn.active .toggle-thumb {
    left: 23px;
  }
</style>

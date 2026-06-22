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
            if (key === 'oauth_ide_enabled') {
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

<style>
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

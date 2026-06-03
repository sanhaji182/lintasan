<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import {
    UserCog, Plus, Trash2, X, Save, Search, Shield, KeyRound, User, Crown
  } from 'lucide-svelte/icons';

  // Real backend schema (internal/auth/users.go → User).
  interface UserRecord {
    id: string;
    username: string;
    role: string; // "admin" | "user"
    must_change_password?: boolean;
    created_at?: string;
    updated_at?: string;
  }

  let users = $state<UserRecord[]>([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let searchQuery = $state('');
  let saving = $state(false);

  // Current logged-in user (from localStorage, set at login).
  let me = $state<{ id?: string; username?: string; role?: string }>({});

  // Create form
  let showCreateForm = $state(false);
  let formUsername = $state('');
  let formPassword = $state('');
  let formRole = $state('user');

  // Reset-password modal (admin → any user)
  let resetTarget = $state<UserRecord | null>(null);
  let resetPassword = $state('');

  // Self-service change-password modal
  let showChangeMine = $state(false);
  let curPassword = $state('');
  let newPassword = $state('');

  const roleColors: Record<string, string> = {
    admin: 'var(--color-purple)',
    user: 'var(--color-fg-2)',
  };

  async function loadUsers() {
    try {
      // Backend returns a RAW array (not {data:[...]}). Older code read res.data
      // and silently showed an empty table — that was the "admin missing" bug.
      const res = await api.get<UserRecord[]>('/api/users');
      const arr = Array.isArray(res) ? res : [];
      const q = searchQuery.trim().toLowerCase();
      users = q ? arr.filter(u => u.username?.toLowerCase().includes(q)) : arr;
      error = '';
    } catch (e: any) {
      error = e.message || 'Failed to load users';
    }
  }

  onMount(async () => {
    try {
      const raw = localStorage.getItem('lintasan_user');
      if (raw) me = JSON.parse(raw);
    } catch {}
    loading = true;
    await loadUsers();
    loading = false;
  });

  function resetCreateForm() {
    formUsername = '';
    formPassword = '';
    formRole = 'user';
    showCreateForm = false;
  }

  async function createUser() {
    if (!formUsername.trim() || !formPassword.trim()) return;
    if (formPassword.length < 8) { error = 'Password must be at least 8 characters'; return; }
    saving = true;
    try {
      await api.post('/api/users', {
        username: formUsername.trim(),
        password: formPassword,
        role: formRole
      });
      notice = `User "${formUsername.trim()}" created`;
      resetCreateForm();
      await loadUsers();
    } catch (e: any) {
      error = e.message || 'Failed to create user';
    }
    saving = false;
  }

  async function changeRole(user: UserRecord, newRole: string) {
    if (user.role === newRole) return;
    try {
      await api.put(`/api/users/${user.id}`, { role: newRole });
      notice = `${user.username} is now ${newRole}`;
      await loadUsers();
    } catch (e: any) {
      error = e.message || 'Failed to update role';
    }
  }

  async function deleteUser(user: UserRecord) {
    if (!confirm(`Delete user "${user.username}"? This cannot be undone.`)) return;
    try {
      await api.delete(`/api/users/${user.id}`);
      notice = `User "${user.username}" deleted`;
      await loadUsers();
    } catch (e: any) {
      error = e.message || 'Failed to delete user';
    }
  }

  async function submitReset() {
    if (!resetTarget || resetPassword.length < 8) { error = 'Password must be at least 8 characters'; return; }
    saving = true;
    try {
      await api.post(`/api/users/${resetTarget.id}/reset-password`, { new_password: resetPassword });
      notice = `Password reset for "${resetTarget.username}". They must change it on next login.`;
      resetTarget = null;
      resetPassword = '';
      await loadUsers();
    } catch (e: any) {
      error = e.message || 'Failed to reset password';
    }
    saving = false;
  }

  async function submitChangeMine() {
    if (newPassword.length < 8) { error = 'New password must be at least 8 characters'; return; }
    saving = true;
    try {
      await api.post('/api/auth/change-password', {
        current_password: curPassword,
        new_password: newPassword
      });
      notice = 'Your password has been changed.';
      showChangeMine = false;
      curPassword = '';
      newPassword = '';
    } catch (e: any) {
      error = e.message || 'Failed to change password';
    }
    saving = false;
  }

  function handleSearch() {
    loading = true;
    loadUsers().then(() => loading = false);
  }

  function formatDate(ts?: string): string {
    if (!ts) return '—';
    try { return new Date(ts).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' }); }
    catch { return ts; }
  }
</script>

<svelte:head>
  <title>Users — Lintasan</title>
</svelte:head>

<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2.5">
      <div class="flex items-center justify-center rounded-xl" style="width: 40px; height: 40px; background: var(--color-info-light);">
        <UserCog size={20} style="color: var(--color-info);" stroke-width={1.8} />
      </div>
      <div>
        <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">User Management</div>
        <div style="font-size: 12px; color: var(--color-fg-3);">Dashboard accounts and access</div>
      </div>
    </div>
    <div class="flex items-center gap-2">
      <button class="btn-secondary flex items-center gap-1.5" onclick={() => { showChangeMine = true; error=''; }}>
        <KeyRound size={14} stroke-width={2} />
        Change my password
      </button>
      <button class="btn-primary flex items-center gap-1.5" onclick={() => { resetCreateForm(); showCreateForm = !showCreateForm; error=''; }}>
        <Plus size={14} stroke-width={2} />
        Add User
      </button>
    </div>
  </div>

  <!-- Search -->
  <div class="card" style="padding: 16px 20px;">
    <div class="flex items-center gap-3">
      <div class="search-wrapper">
        <Search size={14} style="color: var(--color-fg-3); position: absolute; left: 10px; top: 50%; transform: translateY(-50%); pointer-events: none;" />
        <input class="input-field search-input" placeholder="Search by username..." bind:value={searchQuery}
          onkeydown={(e) => e.key === 'Enter' && handleSearch()} />
      </div>
      <button class="btn-primary" onclick={handleSearch} style="padding: 7px 14px;">Search</button>
    </div>
  </div>

  <!-- Create form -->
  {#if showCreateForm}
    <div class="card" style="animation: fadeInUp 0.3s ease-out;">
      <div class="flex items-center justify-between" style="margin-bottom: 16px;">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">New User</div>
        <button class="btn-icon" onclick={resetCreateForm}><X size={16} style="color: var(--color-fg-3);" /></button>
      </div>
      <div class="grid gap-3" style="grid-template-columns: 1fr 1fr 150px;">
        <input class="input-field" placeholder="Username" bind:value={formUsername} autocomplete="off" />
        <input class="input-field" type="password" placeholder="Password (min 8 chars)" bind:value={formPassword} autocomplete="new-password" />
        <select class="input-field" bind:value={formRole}>
          <option value="user">User</option>
          <option value="admin">Admin</option>
        </select>
      </div>
      <div class="flex items-center gap-2" style="margin-top: 16px;">
        <button class="btn-primary flex items-center gap-1.5" onclick={createUser}
          disabled={saving || !formUsername.trim() || formPassword.length < 8}>
          <Save size={14} />
          {saving ? 'Creating...' : 'Create User'}
        </button>
        <button class="btn-secondary" onclick={resetCreateForm}>Cancel</button>
      </div>
    </div>
  {/if}

  <!-- Users table -->
  <div class="card" style="padding: 0; overflow: hidden;">
    {#if loading}
      <Spinner />
    {:else if users.length === 0}
      <EmptyState icon={UserCog} title="No users found"
        description={searchQuery.trim() ? 'Try a different search.' : 'Add users to grant dashboard access.'} />
    {:else}
      <div style="overflow-x: auto;">
        <table class="users-table">
          <thead>
            <tr>
              <th style="width: 260px;">Username</th>
              <th style="width: 140px;">Role</th>
              <th style="width: 160px;">Password</th>
              <th style="width: 140px;">Created</th>
              <th style="width: 200px;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each users as user, i (user.id)}
              <tr style="animation: fadeInUp {0.3 + i * 0.03}s ease-out;">
                <td>
                  <div class="flex items-center gap-2.5">
                    <div class="user-avatar">{user.username.charAt(0).toUpperCase()}</div>
                    <span style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">
                      {user.username}
                      {#if user.id === me.id}<span style="color: var(--color-fg-3); font-weight: 400;"> (you)</span>{/if}
                    </span>
                  </div>
                </td>
                <td>
                  <span class="badge flex items-center gap-1" style="font-size: 10px; display: inline-flex; background: {roleColors[user.role] || 'var(--color-fg-2)'}15; color: {roleColors[user.role] || 'var(--color-fg-2)'};">
                    {#if user.role === 'admin'}<Crown size={10} />{:else}<User size={10} />{/if}
                    {user.role}
                  </span>
                </td>
                <td>
                  {#if user.must_change_password}
                    <span class="badge" style="font-size: 10px; background: var(--color-warning)15; color: var(--color-warning);">must change</span>
                  {:else}
                    <span style="font-size: 12px; color: var(--color-fg-3);">set</span>
                  {/if}
                </td>
                <td><span style="font-size: 12px; color: var(--color-fg-3);">{formatDate(user.created_at)}</span></td>
                <td>
                  <div class="flex items-center gap-1">
                    <!-- role toggle -->
                    <button class="btn-icon" style="color: var(--color-purple);" title={user.role === 'admin' ? 'Demote to user' : 'Promote to admin'}
                      onclick={() => changeRole(user, user.role === 'admin' ? 'user' : 'admin')}>
                      <Shield size={14} />
                    </button>
                    <!-- reset password -->
                    <button class="btn-icon" style="color: var(--color-info);" title="Reset password"
                      onclick={() => { resetTarget = user; resetPassword=''; error=''; }}>
                      <KeyRound size={14} />
                    </button>
                    <!-- delete (not self) -->
                    {#if user.id !== me.id}
                      <button class="btn-icon" style="color: var(--color-error);" title="Delete user" onclick={() => deleteUser(user)}>
                        <Trash2 size={14} />
                      </button>
                    {/if}
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="flex items-center justify-between" style="padding: 12px 16px; border-top: 1px solid var(--color-border); background: var(--color-bg-body);">
        <span style="font-size: 12px; color: var(--color-fg-3);">{users.length} user{users.length !== 1 ? 's' : ''}</span>
      </div>
    {/if}
  </div>

  {#if notice}
    <div class="flex items-center gap-2" style="padding: 12px 16px; border-radius: var(--radius-sm); background: var(--color-success-light); color: var(--color-success); font-size: 13px; font-weight: 500;">
      {notice}
      <button style="margin-left: auto; cursor: pointer; color: var(--color-success); background: none; border: none;" onclick={() => notice = ''}>&times;</button>
    </div>
  {/if}
  {#if error}
    <div class="flex items-center gap-2" style="padding: 12px 16px; border-radius: var(--radius-sm); background: var(--color-error-light); color: var(--color-error); font-size: 13px; font-weight: 500;">
      {error}
      <button style="margin-left: auto; cursor: pointer; color: var(--color-error); background: none; border: none;" onclick={() => error = ''}>&times;</button>
    </div>
  {/if}
</div>

<!-- Reset-password modal -->
{#if resetTarget}
  <div class="modal-backdrop" onclick={() => resetTarget = null} onkeydown={(e) => e.key === 'Escape' && (resetTarget = null)} role="presentation">
    <div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Reset password" tabindex="-1">
      <div class="flex items-center justify-between" style="margin-bottom: 16px;">
        <div style="font-size: 14px; font-weight: 600;">Reset password — {resetTarget.username}</div>
        <button class="btn-icon" onclick={() => resetTarget = null}><X size={16} /></button>
      </div>
      <p style="font-size: 12px; color: var(--color-fg-3); margin: 0 0 12px;">
        The user will be required to change this password on their next login.
      </p>
      <input class="input-field" type="password" placeholder="New password (min 8 chars)" bind:value={resetPassword} autocomplete="new-password" style="width:100%;" />
      <div class="flex items-center gap-2" style="margin-top: 16px;">
        <button class="btn-primary flex items-center gap-1.5" onclick={submitReset} disabled={saving || resetPassword.length < 8}>
          <KeyRound size={14} /> {saving ? 'Resetting...' : 'Reset Password'}
        </button>
        <button class="btn-secondary" onclick={() => resetTarget = null}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

<!-- Self-service change-password modal -->
{#if showChangeMine}
  <div class="modal-backdrop" onclick={() => showChangeMine = false} onkeydown={(e) => e.key === 'Escape' && (showChangeMine = false)} role="presentation">
    <div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Change my password" tabindex="-1">
      <div class="flex items-center justify-between" style="margin-bottom: 16px;">
        <div style="font-size: 14px; font-weight: 600;">Change my password</div>
        <button class="btn-icon" onclick={() => showChangeMine = false}><X size={16} /></button>
      </div>
      <input class="input-field" type="password" placeholder="Current password" bind:value={curPassword} autocomplete="current-password" style="width:100%; margin-bottom: 10px;" />
      <input class="input-field" type="password" placeholder="New password (min 8 chars)" bind:value={newPassword} autocomplete="new-password" style="width:100%;" />
      <div class="flex items-center gap-2" style="margin-top: 16px;">
        <button class="btn-primary flex items-center gap-1.5" onclick={submitChangeMine} disabled={saving || !curPassword || newPassword.length < 8}>
          <Save size={14} /> {saving ? 'Saving...' : 'Change Password'}
        </button>
        <button class="btn-secondary" onclick={() => showChangeMine = false}>Cancel</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .search-wrapper { position: relative; flex: 1; min-width: 200px; max-width: 360px; }
  .search-input { padding-left: 32px !important; }
  .users-table { width: 100%; border-collapse: collapse; font-size: 13px; }
  .users-table th {
    padding: 10px 16px; text-align: left; font-size: 11px; font-weight: 600;
    text-transform: uppercase; letter-spacing: 0.5px; color: var(--color-fg-3);
    background: var(--color-bg-body); border-bottom: 1px solid var(--color-border); white-space: nowrap;
  }
  .users-table td { padding: 12px 16px; border-bottom: 1px solid var(--color-border-light); vertical-align: middle; }
  .users-table tbody tr { transition: var(--transition); }
  .users-table tbody tr:hover { background: var(--color-primary-light); }
  .users-table tbody tr:last-child td { border-bottom: none; }
  .user-avatar {
    width: 30px; height: 30px; border-radius: 50%; background: var(--color-primary-light);
    color: var(--color-primary); display: flex; align-items: center; justify-content: center;
    font-size: 12px; font-weight: 600; flex-shrink: 0;
  }
  .btn-icon {
    display: flex; align-items: center; justify-content: center; width: 32px; height: 32px;
    border-radius: var(--radius-sm); border: none; background: transparent; cursor: pointer; transition: var(--transition);
  }
  .btn-icon:hover { background: var(--color-bg-sidebar-hover); }
  .modal-backdrop {
    position: fixed; inset: 0; background: rgba(15, 23, 42, 0.45);
    display: flex; align-items: center; justify-content: center; z-index: 50; padding: 20px;
  }
  .modal {
    background: var(--color-bg-card, #fff); border: 1px solid var(--color-border);
    border-radius: 14px; padding: 24px; width: 100%; max-width: 420px;
    box-shadow: 0 12px 48px rgba(15, 23, 42, 0.18);
  }
  @media (max-width: 768px) { .search-wrapper { max-width: 100%; } }
</style>

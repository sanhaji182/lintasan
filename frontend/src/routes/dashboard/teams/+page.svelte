<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Spinner from '$lib/components/Spinner.svelte';
  import EmptyState from '$lib/components/EmptyState.svelte';
  import {
    Users, Plus, Trash2, UserPlus, UserMinus,
    Crown, Shield, User, X, Settings
  } from 'lucide-svelte/icons';

  interface Team {
    id: string;
    name: string;
    description?: string;
    members: string[];
    created_at: string;
  }

  let teams = $state<Team[]>([]);
  let loading = $state(true);
  let error = $state('');
  let showCreateForm = $state(false);
  let showMemberModal = $state(false);
  let selectedTeam = $state<Team | null>(null);
  let saving = $state(false);

  // Create form
  let newTeamName = $state('');
  let newTeamDescription = $state('');

  // Member form
  let newMemberEmail = $state('');
  let newMemberRole = $state('member');

  const roleIcons: Record<string, typeof Crown> = {
    owner: Crown,
    admin: Shield,
    member: User,
  };

  const roleColors: Record<string, string> = {
    owner: 'var(--color-warning)',
    admin: 'var(--color-purple)',
    member: 'var(--color-fg-2)',
  };

  async function loadTeams() {
    try {
      const res = await api.get<{ data: Team[] }>('/api/teams');
      teams = res.data || [];
    } catch (e: any) {
      error = e.message || 'Failed to load teams';
    }
  }

  onMount(async () => {
    loading = true;
    await loadTeams();
    loading = false;
  });

  async function createTeam() {
    if (!newTeamName.trim()) return;
    saving = true;
    try {
      const res = await api.post<{ data: Team }>('/api/teams', {
        name: newTeamName.trim(),
        description: newTeamDescription.trim()
      });
      const team = res.data || res as any;
      teams = [...teams, { ...team, members: team.members || [] }];
      newTeamName = '';
      newTeamDescription = '';
      showCreateForm = false;
    } catch (e: any) {
      error = e.message || 'Failed to create team';
    }
    saving = false;
  }

  async function deleteTeam(teamId: string) {
    if (!confirm('Are you sure you want to delete this team?')) return;
    try {
      await api.delete(`/api/teams/${teamId}`);
      teams = teams.filter(t => t.id !== teamId);
      if (selectedTeam?.id === teamId) {
        selectedTeam = null;
        showMemberModal = false;
      }
    } catch (e: any) {
      error = e.message || 'Failed to delete team';
    }
  }

  function openMembers(team: Team) {
    selectedTeam = team;
    showMemberModal = true;
    newMemberEmail = '';
    newMemberRole = 'member';
  }

  async function addMember() {
    if (!selectedTeam || !newMemberEmail.trim()) return;
    saving = true;
    try {
      await api.post(
        `/api/teams/${selectedTeam.id}/members`,
        { username: newMemberEmail.trim() }
      );
      teams = teams.map(t => {
        if (t.id === selectedTeam!.id) {
          return { ...t, members: [...t.members, newMemberEmail.trim()] };
        }
        return t;
      });
      selectedTeam = teams.find(t => t.id === selectedTeam!.id) || null;
      newMemberEmail = '';
    } catch (e: any) {
      error = e.message || 'Failed to add member';
    }
    saving = false;
  }

  async function removeMember(teamId: string, memberName: string) {
    try {
      await api.delete(`/api/teams/${teamId}/members/${memberName}`);
      teams = teams.map(t => {
        if (t.id === teamId) {
          return { ...t, members: t.members.filter(m => m !== memberName) };
        }
        return t;
      });
      selectedTeam = teams.find(t => t.id === teamId) || null;
    } catch (e: any) {
      error = e.message || 'Failed to remove member';
    }
  }

  function closeMemberModal() {
    showMemberModal = false;
    selectedTeam = null;
  }

  function handleOverlayKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      closeMemberModal();
    }
  }

  function stopModalClose(event: MouseEvent) {
    event.stopPropagation();
  }

  function stopModalCloseKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.stopPropagation();
      closeMemberModal();
    }
  }

  function getMemberCount(team: Team): number {
    return team.members.length;
  }

  function formatDate(ts: string): string {
    try {
      return new Date(ts).toLocaleDateString(undefined, {
        month: 'short', day: 'numeric', year: 'numeric'
      });
    } catch {
      return ts;
    }
  }
</script>

<svelte:head>
  <title>Teams — Lintasan</title>
</svelte:head>

<div style="display: flex; flex-direction: column; gap: 24px;">
  <!-- Header -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2.5">
      <div
        class="flex items-center justify-center rounded-xl"
        style="width: 40px; height: 40px; background: var(--color-purple-light);"
      >
        <Users size={20} style="color: var(--color-purple);" stroke-width={1.8} />
      </div>
      <div>
        <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">Teams</div>
        <div style="font-size: 12px; color: var(--color-fg-3);">Manage teams and their members</div>
      </div>
    </div>
    <button
      class="btn-primary flex items-center gap-1.5"
      onclick={() => showCreateForm = !showCreateForm}
    >
      <Plus size={14} stroke-width={2} />
      Create Team
    </button>
  </div>

  <!-- Create Team Form -->
  {#if showCreateForm}
    <div class="card" style="animation: fadeInUp 0.3s ease-out;">
      <div class="flex items-center justify-between" style="margin-bottom: 16px;">
        <div style="font-size: 14px; font-weight: 600; color: var(--color-fg-0);">New Team</div>
        <button
          class="btn-icon"
          onclick={() => { showCreateForm = false; newTeamName = ''; newTeamDescription = ''; }}
        >
          <X size={16} style="color: var(--color-fg-3);" />
        </button>
      </div>
      <div class="flex flex-col gap-3">
        <input
          class="input-field"
          placeholder="Team name"
          bind:value={newTeamName}
          onkeydown={(e) => e.key === 'Enter' && createTeam()}
        />
        <input
          class="input-field"
          placeholder="Description (optional)"
          bind:value={newTeamDescription}
        />
        <div class="flex items-center gap-2">
          <button class="btn-primary" onclick={createTeam} disabled={saving || !newTeamName.trim()}>
            {saving ? 'Creating...' : 'Create Team'}
          </button>
          <button class="btn-secondary" onclick={() => { showCreateForm = false; newTeamName = ''; newTeamDescription = ''; }}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- Teams Grid -->
  {#if loading}
    <Spinner />
  {:else if teams.length === 0}
    <div class="card">
      <EmptyState
        icon={Users}
        title="No teams yet"
        description="Create a team to collaborate with your organization."
      />
    </div>
  {:else}
    <div class="grid gap-5" style="grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));">
      {#each teams as team, i (team.id)}
        <div class="card team-card" style="animation: fadeInUp {0.3 + i * 0.05}s ease-out;">
          <div class="flex items-start justify-between" style="margin-bottom: 16px;">
            <div style="flex: 1; min-width: 0;">
              <div class="flex items-center gap-2" style="margin-bottom: 4px;">
                <div
                  class="flex items-center justify-center rounded-lg"
                  style="width: 32px; height: 32px; background: var(--color-primary-light); flex-shrink: 0;"
                >
                  <Users size={16} style="color: var(--color-primary);" />
                </div>
                <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0); white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                  {team.name}
                </div>
              </div>
              {#if team.description}
                <div style="font-size: 12px; color: var(--color-fg-3); margin-left: 40px; line-height: 1.4;">
                  {team.description}
                </div>
              {/if}
            </div>
            <button
              class="btn-icon"
              style="color: var(--color-error);"
              onclick={() => deleteTeam(team.id)}
              title="Delete team"
            >
              <Trash2 size={14} />
            </button>
          </div>

          <!-- Member count and role badges -->
          <div
            class="flex items-center justify-between"
            style="padding: 12px 0; border-top: 1px solid var(--color-border);"
          >
            <div class="flex items-center gap-2">
              <span style="font-size: 12px; color: var(--color-fg-2);">
                {team.members.length} member{team.members.length !== 1 ? 's' : ''}
              </span>
            </div>
            <button
              class="btn-secondary"
              style="padding: 5px 12px; font-size: 12px;"
              onclick={() => openMembers(team)}
            >
              <Settings size={12} style="display: inline; vertical-align: -1px;" />
              Manage
            </button>
          </div>

          <!-- Member avatars (first 5) -->
          <div class="flex items-center" style="margin-top: 8px;">
            {#each team.members.slice(0, 5) as member, mi}
              <div
                class="avatar-circle"
                style="margin-left: {mi > 0 ? '-8px' : '0'}; z-index: {5 - mi};"
                title={member}
              >
                {member.charAt(0).toUpperCase()}
              </div>
            {/each}
            {#if team.members.length > 5}
              <div
                class="avatar-circle avatar-overflow"
                style="margin-left: -8px; z-index: 0;"
              >
                +{team.members.length - 5}
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- Member Management Modal -->
  {#if showMemberModal && selectedTeam}
    <div
      class="modal-overlay"
      role="button"
      tabindex="0"
      aria-label="Close team member modal"
      onclick={closeMemberModal}
      onkeydown={handleOverlayKeydown}
    >
      <div
        class="modal-card card"
        role="dialog"
        aria-modal="true"
        aria-label="Team member management"
        tabindex="-1"
        onclick={stopModalClose}
        onkeydown={stopModalCloseKeydown}
      >
        <div class="flex items-center justify-between" style="margin-bottom: 20px;">
          <div>
            <div style="font-size: 15px; font-weight: 600; color: var(--color-fg-0);">
              {selectedTeam.name} — Members
            </div>
            <div style="font-size: 12px; color: var(--color-fg-3);">
              {selectedTeam.members.length} member{selectedTeam.members.length !== 1 ? 's' : ''}
            </div>
          </div>
          <button class="btn-icon" onclick={closeMemberModal}>
            <X size={16} style="color: var(--color-fg-3);" />
          </button>
        </div>

        <!-- Add Member -->
        <div class="flex items-center gap-2" style="margin-bottom: 20px;">
          <input
            class="input-field"
            style="flex: 1;"
            placeholder="Username"
            bind:value={newMemberEmail}
            onkeydown={(e) => e.key === 'Enter' && addMember()}
          />
          <button class="btn-primary" onclick={addMember} disabled={saving || !newMemberEmail.trim()}>
            <UserPlus size={14} />
          </button>
        </div>

        <!-- Members List -->
        {#if selectedTeam.members.length === 0}
          <EmptyState
            icon={Users}
            title="No members"
            description="Add a username to this team."
          />
        {:else}
          <div style="display: flex; flex-direction: column; gap: 8px; max-height: 400px; overflow-y: auto;">
            {#each selectedTeam.members as member (member)}
              <div class="member-row flex items-center justify-between">
                <div class="flex items-center gap-3">
                  <div class="avatar-circle" style="width: 32px; height: 32px; font-size: 12px;">
                    {member.charAt(0).toUpperCase()}
                  </div>
                  <div>
                    <div style="font-size: 13px; font-weight: 500; color: var(--color-fg-0);">{member}</div>
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <button
                    class="btn-icon"
                    style="color: var(--color-error); width: 28px; height: 28px;"
                    onclick={() => removeMember(selectedTeam!.id, member)}
                    title="Remove member"
                  >
                    <UserMinus size={14} />
                  </button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
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
  .team-card {
    transition: var(--transition);
  }
  .team-card:hover {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 3px var(--color-primary-glow);
  }
  .avatar-circle {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--color-primary-light);
    color: var(--color-primary);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    font-weight: 600;
    border: 2px solid var(--color-bg-card);
  }
  .avatar-overflow {
    background: var(--color-border);
    color: var(--color-fg-2);
  }
  .member-row {
    padding: 10px 12px;
    background: var(--color-bg-body);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    transition: var(--transition);
  }
  .member-row:hover {
    border-color: var(--color-border);
    box-shadow: var(--shadow-sm);
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
    background: var(--color-error-light);
  }
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    animation: fadeInUp 0.2s ease-out;
  }
  .modal-card {
    width: 100%;
    max-width: 520px;
    max-height: 80vh;
    overflow-y: auto;
    animation: fadeInScale 0.3s ease-out;
  }
  @media (max-width: 768px) {
    .modal-card {
      margin: 16px;
    }
  }
</style>

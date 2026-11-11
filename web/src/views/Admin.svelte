<script>
  import { onMount } from 'svelte';
  import { api } from '../api.svelte.js';
  import { refreshPending } from '../admin.svelte.js';
  import { t } from '../i18n.svelte.js';

  let filter = $state('pending');
  let list = $state(null);
  let error = $state('');

  async function load() {
    const q = filter === 'all' ? '' : `?status=${filter}`;
    const r = await api.get(`/api/admin/users${q}`);
    if (r.kind === 'ok') list = r.data;
    else if (r.kind !== 'session') error = t('errorText');
    refreshPending();
  }

  async function setStatus(u, status) {
    error = '';
    const r = await api.put(`/api/admin/users/${u.id}/status`, { status });
    if (r.kind === 'http') error = r.status === 409 ? t('adminProtected') : t('errorText');
    await load();
  }

  async function remove(u) {
    if (!confirm(t('confirmDeleteUser', { email: u.email }))) return;
    error = '';
    const r = await api.del(`/api/admin/users/${u.id}`);
    if (r.kind === 'http') error = r.status === 409 ? t('adminProtected') : t('errorText');
    await load();
  }

  function pick(f) {
    filter = f;
    load();
  }

  onMount(load);
</script>

<h1>{t('adminTitle')}</h1>

<div class="row" style="margin-bottom: 1rem">
  {#each ['pending', 'approved', 'blocked', 'all'] as f}
    <button class={filter === f ? '' : 'secondary'} onclick={() => pick(f)}>{t(`status_${f}`)}</button>
  {/each}
</div>
{#if error}<p class="error">{error}</p>{/if}

{#if list === null}
  <p class="muted">{t('loading')}</p>
{:else if list.length === 0}
  <p class="muted">{t('noUsers')}</p>
{:else}
  <div class="card">
    <ul class="plain">
      {#each list as u (u.id)}
        <li class="row between">
          <span>
            <strong>{u.name}</strong>
            <span class="muted"> {u.email} · {t(`status_${u.status}`)}{u.is_admin ? ` · ${t('admin')}` : ''}</span>
          </span>
          {#if !u.is_admin}
            <span class="row">
              {#if u.status !== 'approved'}<button class="small" onclick={() => setStatus(u, 'approved')}>{t('approve')}</button>{/if}
              {#if u.status !== 'blocked'}<button class="secondary small" onclick={() => setStatus(u, 'blocked')}>{t('block')}</button>{/if}
              <button class="danger small" onclick={() => remove(u)}>{t('delete')}</button>
            </span>
          {/if}
        </li>
      {/each}
    </ul>
  </div>
{/if}

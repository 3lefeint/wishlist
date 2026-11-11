<script>
  import { onMount } from 'svelte';
  import { api } from '../api.svelte.js';
  import { t } from '../i18n.svelte.js';
  import AdminNav from './AdminNav.svelte';

  let list = $state(null);
  let error = $state('');
  let users = $state([]);
  let name = $state('');
  let owner = $state('');

  async function load() {
    const r = await api.get('/api/admin/circles');
    if (r.kind === 'ok') list = r.data;
    else if (r.kind !== 'session') error = t('errorText');
    const u = await api.get('/api/admin/users?status=approved');
    if (u.kind === 'ok') users = u.data.filter((x) => !x.is_admin);
  }

  async function create(e) {
    e.preventDefault();
    error = '';
    const r = await api.post('/api/admin/circles', { name, user_id: Number(owner) });
    if (r.kind === 'ok') location.hash = `/admin/circles/${r.data.id}`;
    else if (r.kind !== 'session') error = t('invalidName');
  }

  async function remove(c) {
    if (!confirm(t('confirmDeleteCircle', { name: c.name }))) return;
    error = '';
    const r = await api.del(`/api/admin/circles/${c.id}`);
    if (r.kind === 'http' || r.kind === 'error') error = t('errorText');
    await load();
  }

  onMount(load);
</script>

<AdminNav current="circles" />
<h1>{t('adminCircles')}</h1>
{#if error}<p class="error">{error}</p>{/if}
<form class="card row" onsubmit={create}>
  <input type="text" bind:value={name} maxlength="80" placeholder={t('newCircle')} />
  <select bind:value={owner}>
    <option value="">{t('chooseUser')}</option>
    {#each users as u (u.id)}<option value={u.id}>{u.name || u.email}</option>{/each}
  </select>
  <button disabled={!name.trim() || !owner}>{t('create')}</button>
</form>
{#if list === null}
  <p class="muted">{t('loading')}</p>
{:else if list.length === 0}
  <p class="muted">{t('noCirclesAdmin')}</p>
{:else}
  <div class="card">
    <ul class="plain">
      {#each list as c (c.id)}
        <li class="row between">
          <span>
            <a href={`#/admin/circles/${c.id}`}><strong>{c.name}</strong></a>
            <span class="muted"> {t('circleCounts', { m: c.member_count, g: c.gift_count })}</span>
          </span>
          <button class="danger small" onclick={() => remove(c)}>{t('delete')}</button>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<script>
  import { onMount } from 'svelte';
  import { api } from '../api.svelte.js';
  import { formatDate, t } from '../i18n.svelte.js';

  let { id } = $props();
  let circle = $state(null);
  let gifts = $state([]);
  let candidates = $state([]);
  let missing = $state(false);
  let error = $state('');
  let newUser = $state('');

  const refusals = { 409: t('lastAdmin'), 404: t('notFound') };

  async function load() {
    const r = await api.get(`/api/admin/circles/${id}`);
    if (r.kind === 'ok') circle = r.data;
    else if (r.kind === 'http') {
      missing = true;
      return;
    }
    const g = await api.get(`/api/admin/circles/${id}/gifts`);
    if (g.kind === 'ok') gifts = g.data;
    const u = await api.get('/api/admin/users?status=approved');
    if (u.kind === 'ok') candidates = u.data;
  }

  // Runs a change and reloads; shows the server's refusal if any.
  async function act(promise) {
    error = '';
    const r = await promise;
    if (r.kind === 'http') error = (r.status === 409 && r.message.includes('already')) ? t('alreadyMember') : (refusals[r.status] ?? t('errorText'));
    else if (r.kind === 'error') error = t('errorText');
    await load();
    return r.kind === 'ok';
  }

  const free = $derived(candidates.filter((u) => !circle?.members.some((m) => m.user_id === u.id)));

  async function addMember(e) {
    e.preventDefault();
    if (await act(api.post(`/api/admin/circles/${id}/members`, { user_id: Number(newUser), role: 'member' }))) newUser = '';
  }
  const setRole = (m, role) => act(api.put(`/api/admin/circles/${id}/members/${m.user_id}`, { role }));
  const removeMember = (m) => confirm(t('confirmRemove', { name: m.name })) && act(api.del(`/api/admin/circles/${id}/members/${m.user_id}`));
  const toggle = (g) => act(api.put(`/api/admin/gifts/${g.id}/active`, { active: !g.active }));
  const removeGift = (g) => confirm(t('confirmDeleteGift')) && act(api.del(`/api/admin/gifts/${g.id}`));

  async function removeCircle() {
    if (!confirm(t('confirmDeleteCircle', { name: circle.name }))) return;
    const r = await api.del(`/api/admin/circles/${id}`);
    if (r.kind === 'ok') location.hash = '/admin/circles';
    else error = t('errorText');
  }

  onMount(load);

  const byPerson = $derived.by(() => {
    const groups = [];
    for (const g of gifts) {
      let grp = groups.find((x) => x.person_id === g.person_id);
      if (!grp) groups.push((grp = { person_id: g.person_id, name: g.person_name, items: [] }));
      grp.items.push(g);
    }
    return groups;
  });
</script>

{#if missing}
  <div class="card"><h1>{t('notFound')}</h1><a href="#/admin/circles">{t('backToCircles')}</a></div>
{:else if !circle}
  <p class="muted">{t('loading')}</p>
{:else}
  <p><a href="#/admin/circles">{t('backToCircles')}</a></p>
  <div class="row between">
    <h1>{circle.name}</h1>
    <button class="danger small" onclick={removeCircle}>{t('deleteCircle')}</button>
  </div>
  {#if error}<p class="error">{error}</p>{/if}

  <h2>{t('membersTitle')}</h2>
  <div class="card">
    <ul class="plain">
      {#each circle.members as m (m.user_id)}
        <li class="row between">
          <span>
            {m.name} <span class="muted">{m.email}{m.birthday ? ` · ${formatDate(m.birthday)}` : ''}</span>
          </span>
          <span class="row">
            <select value={m.role} onchange={(e) => setRole(m, e.currentTarget.value)}>
              <option value="member">{t('roleMember')}</option>
              <option value="admin">{t('circleAdmin')}</option>
            </select>
            <button class="danger small" onclick={() => removeMember(m)}>{t('remove')}</button>
          </span>
        </li>
      {/each}
    </ul>
    <form class="row" onsubmit={addMember}>
      <select bind:value={newUser}>
        <option value="">{t('chooseUser')}</option>
        {#each free as u (u.id)}<option value={u.id}>{u.name} ({u.email})</option>{/each}
      </select>
      <button disabled={!newUser}>{t('addMember')}</button>
    </form>
  </div>

  <h2>{t('giftsTitle')}</h2>
  {#if gifts.length === 0}
    <p class="muted">{t('noGifts')}</p>
  {/if}
  {#each byPerson as grp (grp.person_id)}
    <h3>{grp.name}</h3>
    <div class="card">
      <ul class="plain">
        {#each grp.items as g (g.id)}
          <li class="row between">
            <span class="row">
              {#if g.has_image}<img src={`/api/admin/gifts/${g.id}/thumb`} alt="" width="48" height="48" style="object-fit: cover; border-radius: 6px" />{/if}
              <span>
                <strong>{g.title}</strong>{#if !g.active} <span class="muted">({t('deactivated')})</span>{/if}
                {#if g.comment}<div class="muted">{g.comment}</div>{/if}
                {#if g.url}<div><a href={g.url} target="_blank" rel="noopener noreferrer">{g.url}</a></div>{/if}
              </span>
            </span>
            <span class="row">
              <button class="secondary small" onclick={() => toggle(g)}>{g.active ? t('deactivate') : t('activate')}</button>
              <button class="danger small" onclick={() => removeGift(g)}>{t('delete')}</button>
            </span>
          </li>
        {/each}
      </ul>
    </div>
  {/each}
{/if}

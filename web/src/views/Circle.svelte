<script>
  import { onMount } from 'svelte';
  import { api } from '../api.svelte.js';
  import { t } from '../i18n.svelte.js';

  let { id, me } = $props();
  let circle = $state(null);
  let missing = $state(false);
  let error = $state('');
  let personName = $state('');
  let copied = $state(false);

  const isAdmin = $derived(circle?.role === 'admin');
  const memberName = (uid) => circle.members.find((m) => m.user_id === uid)?.name ?? '?';
  const canManage = (p) => isAdmin || p.guardians.includes(me.id);
  const link = $derived(circle?.invite ? `${location.origin}/#/join/${circle.invite.token}` : '');

  async function load() {
    const r = await api.get(`/api/circles/${id}`);
    if (r.kind === 'ok') circle = r.data;
    else if (r.kind === 'http') missing = true;
  }

  // Runs a change, reloads the circle and shows the server's refusal if any.
  async function act(promise, refusal = {}) {
    error = '';
    const r = await promise;
    if (r.kind === 'ok') {
      await load();
      return r;
    }
    if (r.kind === 'http') error = refusal[r.status] ?? t('errorText');
    else if (r.kind === 'error') error = t('errorText');
    return null;
  }

  async function addPerson(e) {
    e.preventDefault();
    if (await act(api.post(`/api/circles/${id}/persons`, { name: personName }))) personName = '';
  }

  const rename = async (p) => {
    const name = prompt(t('renamePerson'), p.name);
    if (name?.trim()) await act(api.put(`/api/persons/${p.id}`, { name }));
  };
  const removePerson = async (p) => {
    if (confirm(t('confirmDeletePerson', { name: p.name }))) await act(api.del(`/api/persons/${p.id}`));
  };
  const addGuardian = (p, uid) => act(api.post(`/api/persons/${p.id}/guardians`, { user_id: Number(uid) }));
  const removeGuardian = (p, uid) => act(api.del(`/api/persons/${p.id}/guardians/${uid}`));

  async function removeMember(m) {
    const self = m.user_id === me.id;
    if (!confirm(self ? t('confirmLeave') : t('confirmRemove', { name: m.name }))) return;
    const r = await act(api.del(`/api/circles/${id}/members/${m.user_id}`), { 409: t('lastAdmin') });
    if (r && self) location.hash = '/';
  }

  const renewInvite = () => act(api.post(`/api/circles/${id}/invite`));
  const revokeInvite = () => act(api.del(`/api/circles/${id}/invite`));

  async function copy() {
    try {
      await navigator.clipboard.writeText(link);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {}
  }

  async function removeCircle() {
    if (!confirm(t('confirmDeleteCircle', { name: circle.name }))) return;
    const r = await act(api.del(`/api/circles/${id}`));
    if (r) location.hash = '/';
  }

  onMount(load);
</script>

{#if missing}
  <div class="card"><h1>{t('notFound')}</h1><a href="#/">{t('home')}</a></div>
{:else if !circle}
  <p class="muted">{t('loading')}</p>
{:else}
  <div class="row between">
    <h1>{circle.name}</h1>
    {#if isAdmin}<button class="danger small" onclick={removeCircle}>{t('deleteCircle')}</button>{/if}
  </div>
  {#if error}<p class="error">{error}</p>{/if}

  <h2>{t('persons')}</h2>
  <div class="card">
    <ul class="plain">
      {#each circle.persons as p (p.id)}
        <li>
          <div class="row between">
            <a href={`#/persons/${p.id}`}><strong>{p.name}</strong></a>
            {#if p.user_id === null && canManage(p)}
              <span class="row">
                <button class="secondary small" onclick={() => rename(p)}>{t('rename')}</button>
                <button class="danger small" onclick={() => removePerson(p)}>{t('delete')}</button>
              </span>
            {/if}
          </div>
          {#if p.user_id === null}
            <div class="muted">
              {t('guardians')}:
              {#each p.guardians as g, i}
                {memberName(g)}{#if canManage(p)}
                  <button class="secondary small" title={t('removeGuardian')} onclick={() => removeGuardian(p, g)}>×</button>{/if}{i < p.guardians.length - 1 ? ', ' : ''}
              {/each}
              {#if canManage(p)}
                <select onchange={(e) => { if (e.currentTarget.value) addGuardian(p, e.currentTarget.value); e.currentTarget.value = ''; }}>
                  <option value="">{t('addGuardian')}</option>
                  {#each circle.members.filter((m) => !p.guardians.includes(m.user_id)) as m (m.user_id)}
                    <option value={m.user_id}>{m.name}</option>
                  {/each}
                </select>
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
    <form class="row" onsubmit={addPerson}>
      <input type="text" bind:value={personName} maxlength="80" placeholder={t('personWithoutAccount')} />
      <button disabled={!personName.trim()}>{t('add')}</button>
    </form>
  </div>

  <h2>{t('membersTitle')}</h2>
  <div class="card">
    <ul class="plain">
      {#each circle.members as m (m.user_id)}
        <li class="row between">
          <span>{m.name} {#if m.role === 'admin'}<span class="muted">({t('circleAdmin')})</span>{/if}</span>
          {#if m.user_id === me.id}
            <button class="secondary small" onclick={() => removeMember(m)}>{t('leave')}</button>
          {:else if isAdmin}
            <button class="danger small" onclick={() => removeMember(m)}>{t('remove')}</button>
          {/if}
        </li>
      {/each}
    </ul>
  </div>

  {#if isAdmin}
    <h2>{t('inviteTitle')}</h2>
    <div class="card">
      {#if circle.invite}
        <p class="muted">{t('inviteValid', { date: new Date(circle.invite.expires_at).toLocaleDateString() })}</p>
        <input type="text" readonly value={link} onclick={(e) => e.currentTarget.select()} />
        <div class="row" style="margin-top: 0.5rem">
          <button onclick={copy}>{copied ? t('copied') : t('copy')}</button>
          <button class="secondary" onclick={renewInvite}>{t('renew')}</button>
          <button class="danger" onclick={revokeInvite}>{t('revoke')}</button>
        </div>
      {:else}
        <p class="muted">{t('noInvite')}</p>
        <button onclick={renewInvite}>{t('createInvite')}</button>
      {/if}
    </div>
  {/if}
{/if}

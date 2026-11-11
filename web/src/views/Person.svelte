<script>
  import { onMount } from 'svelte';
  import { api, uploadImage } from '../api.svelte.js';
  import { t } from '../i18n.svelte.js';

  let { id, me } = $props();
  let person = $state(null);
  let gifts = $state([]);
  let missing = $state(false);
  let error = $state('');
  let editing = $state(null); // null, 'new' or a gift
  let form = $state({ title: '', comment: '', url: '', tags: '' });
  let file = $state(null);
  let version = $state(0); // bumps image URLs after an upload

  async function load() {
    const [p, g] = await Promise.all([api.get(`/api/persons/${id}`), api.get(`/api/persons/${id}/gifts`)]);
    if (p.kind === 'ok' && g.kind === 'ok') {
      person = p.data;
      gifts = g.data;
    } else if (p.kind === 'http') missing = true;
    else if (p.kind === 'error' || g.kind === 'error') error = t('errorText');
  }

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

  function startNew() {
    form = { title: '', comment: '', url: '', tags: '' };
    file = null;
    editing = 'new';
  }
  function startEdit(g) {
    form = { title: g.title, comment: g.comment, url: g.url, tags: g.tags.map((x) => x.name).join(', ') };
    file = null;
    editing = g;
  }

  async function submit(e) {
    e.preventDefault();
    error = '';
    const body = {
      title: form.title,
      comment: form.comment,
      url: form.url,
      tags: form.tags.split(',').map((x) => x.trim()).filter(Boolean),
    };
    const isNew = editing === 'new';
    const r = isNew ? await api.post(`/api/persons/${id}/gifts`, body) : await api.put(`/api/gifts/${editing.id}`, body);
    if (r.kind !== 'ok') {
      if (r.kind === 'http') error = t('invalidGift');
      else if (r.kind === 'error') error = t('errorText');
      return;
    }
    const giftId = isNew ? r.data.id : editing.id;
    if (file) {
      const u = await uploadImage(giftId, file);
      if (u.kind === 'http') {
        error = t('invalidImage');
        await load();
        return;
      }
      version++;
    }
    editing = null;
    await load();
  }

  const claim = (g) => act(api.put(`/api/gifts/${g.id}/claim`), { 409: t('alreadyClaimed') });
  const unclaim = (g) => act(api.del(`/api/gifts/${g.id}/claim`));
  const setActive = (g, active) => act(api.put(`/api/gifts/${g.id}/active`, { active }));
  async function remove(g) {
    if (confirm(t('confirmDeleteGift'))) await act(api.del(`/api/gifts/${g.id}`));
  }
  async function removeImage(g) {
    await act(api.del(`/api/gifts/${g.id}/image`));
    version++;
  }

  onMount(load);
</script>

{#if missing}
  <div class="card"><h1>{t('notFound')}</h1><a href="#/">{t('home')}</a></div>
{:else if !person}
  <p class="muted">{t('loading')}</p>
{:else}
  <p><a href={`#/circles/${person.circle_id}`}>{t('back', { name: person.circle_name })}</a></p>
  <h1>{person.name}</h1>
  {#if person.is_self}<p class="muted">{t('ownList')}</p>{/if}
  {#if error}<p class="error">{error}</p>{/if}

  {#if editing}
    <form class="card" onsubmit={submit}>
      <h2 style="margin-top: 0">{editing === 'new' ? t('addGift') : t('editGift')}</h2>
      <label class="field"><span>{t('giftTitle')}</span><input type="text" bind:value={form.title} maxlength="120" required /></label>
      <label class="field"><span>{t('giftComment')}</span><textarea bind:value={form.comment} rows="3" maxlength="2000"></textarea></label>
      <label class="field"><span>{t('giftUrl')}</span><input type="url" bind:value={form.url} maxlength="2000" placeholder="https://" /></label>
      <label class="field"><span>{t('giftTags')}</span><input type="text" bind:value={form.tags} /></label>
      <label class="field">
        <span>{t('giftImage')}</span>
        <input type="file" accept="image/jpeg,image/png,image/gif" onchange={(e) => (file = e.currentTarget.files[0] ?? null)} />
      </label>
      <div class="row">
        <button disabled={!form.title.trim()}>{t('save')}</button>
        <button type="button" class="secondary" onclick={() => (editing = null)}>{t('cancel')}</button>
      </div>
    </form>
  {:else}
    <p><button onclick={startNew}>{t('addGift')}</button></p>
  {/if}

  {#if gifts.length === 0}
    <p class="muted">{t('noGifts')}</p>
  {/if}
  {#each gifts as g (g.id)}
    <article class="card" class:inactive={!g.active}>
      <div class="gift">
        {#if g.has_image}
          <a href={`/api/gifts/${g.id}/image?v=${version}`} target="_blank" rel="noopener">
            <img src={`/api/gifts/${g.id}/thumb?v=${version}`} alt="" loading="lazy" />
          </a>
        {/if}
        <div class="body">
          <strong>{g.title}</strong>
          {#if !g.active}<span class="muted"> · {t('deactivated')}</span>{/if}
          {#if g.comment}<p style="margin: 0.25rem 0; white-space: pre-wrap">{g.comment}</p>{/if}
          {#if g.url}<a href={g.url} target="_blank" rel="noopener noreferrer">{t('openLink')}</a>{/if}
          <div>
            {#each g.tags as tag (tag.id)}
              <span class="chip" style={`background: ${tag.color}; color: ${tag.text_color}`}>{tag.name}</span>
            {/each}
          </div>
          {#if g.claim}
            <p class="muted" style="margin: 0.25rem 0">
              {g.claim.user_id === me.id ? t('claimedByYou') : t('claimedBy', { name: g.claim.name })}
            </p>
            {#if !g.active && g.claim.user_id === me.id}<p class="error">{t('deactivatedClaim')}</p>{/if}
          {/if}
        </div>
      </div>
      <div class="row" style="margin-top: 0.5rem">
        {#if !person.is_self && g.active && !g.claim}
          <button onclick={() => claim(g)}>{t('claim')}</button>
        {/if}
        {#if g.claim?.user_id === me.id}
          <button class="secondary" onclick={() => unclaim(g)}>{t('unclaim')}</button>
        {/if}
        {#if g.can_edit}
          <button class="secondary small" onclick={() => startEdit(g)}>{t('edit')}</button>
          <button class="secondary small" onclick={() => setActive(g, !g.active)}>{g.active ? t('deactivate') : t('activate')}</button>
          {#if g.has_image}<button class="secondary small" onclick={() => removeImage(g)}>{t('removeImage')}</button>{/if}
        {/if}
        {#if g.can_delete}<button class="danger small" onclick={() => remove(g)}>{t('delete')}</button>{/if}
      </div>
    </article>
  {/each}
{/if}

<style>
  .gift { display: flex; gap: 0.75rem; align-items: flex-start; }
  .gift img { width: 80px; height: 80px; object-fit: cover; border-radius: 8px; }
  .body { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  .inactive { opacity: 0.7; }
</style>

<script>
  import { api } from '../api.svelte.js';
  import { t } from '../i18n.svelte.js';

  let { me, onchange, first = false } = $props();
  // svelte-ignore state_referenced_locally
  let form = $state({
    first_name: me.first_name,
    last_name: me.last_name,
    birthday: me.birthday,
    favorite_color: me.favorite_color,
  });
  // svelte-ignore state_referenced_locally
  let hasColor = $state(me.favorite_color !== '');
  let saved = $state(false);
  let error = $state('');
  const today = new Date().toISOString().slice(0, 10);

  async function save(e) {
    e.preventDefault();
    error = '';
    saved = false;
    const r = await api.put('/api/me', { ...form, favorite_color: hasColor ? form.favorite_color : '' });
    if (r.kind === 'ok') {
      onchange(r.data);
      saved = true;
    } else if (r.kind !== 'session') error = t('invalidProfile');
  }
</script>

<h1>{first ? t('profileTitle') : t('profile')}</h1>
{#if first}<p>{t('profileIntro')}</p>{/if}
<form class="card" onsubmit={save}>
  <p class="muted">{me.email}</p>
  <label class="field">
    <span>{t('firstName')} *</span>
    <input type="text" bind:value={form.first_name} maxlength="80" required />
  </label>
  <label class="field">
    <span>{t('lastName')} *</span>
    <input type="text" bind:value={form.last_name} maxlength="80" required />
  </label>
  <label class="field">
    <span>{t('birthday')} *</span>
    <input type="date" bind:value={form.birthday} min="1900-01-01" max={today} required />
  </label>
  <label class="field">
    <span>{t('favoriteColor')}</span>
    <input
      type="color"
      value={form.favorite_color || '#3a7bd5'}
      oninput={(e) => { form.favorite_color = e.currentTarget.value; hasColor = true; }}
    />
    {#if hasColor}
      <button type="button" class="secondary small" onclick={() => { hasColor = false; form.favorite_color = ''; }}>×</button>
    {/if}
  </label>
  <button>{t('save')}</button>
  {#if saved}<span class="muted"> {t('saved')}</span>{/if}
  {#if error}<p class="error">{error}</p>{/if}
</form>

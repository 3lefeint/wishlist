<script>
  import { api } from '../api.svelte.js';
  import { t } from '../i18n.svelte.js';

  let { me, onchange } = $props();
  // svelte-ignore state_referenced_locally
  let name = $state(me.name);
  let saved = $state(false);
  let error = $state('');

  async function save(e) {
    e.preventDefault();
    error = '';
    saved = false;
    const r = await api.put('/api/me', { name });
    if (r.kind === 'ok') {
      onchange(r.data);
      name = r.data.name;
      saved = true;
    } else if (r.kind !== 'session') error = t('invalidName');
  }
</script>

<h1>{t('profile')}</h1>
<form class="card" onsubmit={save}>
  <p class="muted">{me.email}</p>
  <label class="field">
    <span>{t('displayName')}</span>
    <input type="text" bind:value={name} maxlength="80" />
  </label>
  <button>{t('save')}</button>
  {#if saved}<span class="muted"> {t('saved')}</span>{/if}
  {#if error}<p class="error">{error}</p>{/if}
</form>

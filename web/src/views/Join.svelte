<script>
  import { onMount } from 'svelte';
  import { api } from '../api.svelte.js';
  import { t } from '../i18n.svelte.js';

  let { token } = $props();
  let info = $state(null);
  let invalid = $state(false);
  let error = $state('');

  onMount(async () => {
    const r = await api.get(`/api/invites/${encodeURIComponent(token)}`);
    if (r.kind === 'ok') info = r.data;
    else if (r.kind === 'http') invalid = true;
  });

  async function join() {
    error = '';
    const r = await api.post(`/api/invites/${encodeURIComponent(token)}/join`);
    if (r.kind === 'ok') location.hash = `/circles/${r.data.circle_id}`;
    else if (r.kind !== 'session') error = t('inviteInvalid');
  }
</script>

<div class="card">
  {#if invalid}
    <h1>{t('inviteInvalidTitle')}</h1>
    <p>{t('inviteInvalid')}</p>
    <a href="#/">{t('home')}</a>
  {:else if info}
    <h1>{t('joinTitle', { name: info.name })}</h1>
    <p>{t('joinText')}</p>
    <button onclick={join}>{t('join')}</button>
    {#if error}<p class="error">{error}</p>{/if}
  {:else}
    <p class="muted">{t('loading')}</p>
  {/if}
</div>

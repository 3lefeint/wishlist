<script>
  import { onMount } from 'svelte';
  import { getMe } from './api.js';
  import { i18n, languages, setLang, t } from './i18n.svelte.js';

  let state = $state({ kind: 'loading' });

  async function load() {
    state = { kind: 'loading' };
    state = await getMe();
  }

  onMount(() => {
    document.documentElement.lang = i18n.lang;
    load();
  });
</script>

<header>
  <strong>{t('appName')}</strong>
  <label>
    <span class="sr">{t('language')}</span>
    <select value={i18n.lang} onchange={(e) => setLang(e.currentTarget.value)}>
      {#each languages as l}
        <option value={l}>{l.toUpperCase()}</option>
      {/each}
    </select>
  </label>
</header>

<main>
  {#if state.kind === 'loading'}
    <p class="muted">{t('loading')}</p>
  {:else if state.kind === 'session'}
    <section class="card">
      <h1>{t('sessionTitle')}</h1>
      <p>{t('sessionText')}</p>
      <button onclick={() => location.reload()}>{t('reload')}</button>
    </section>
  {:else if state.kind === 'closed'}
    <section class="card">
      <h1>{t('closedTitle')}</h1>
      <p>{t('closedText')}</p>
    </section>
  {:else if state.kind === 'error'}
    <section class="card">
      <h1>{t('errorTitle')}</h1>
      <p>{t('errorText')}</p>
      <button onclick={load}>{t('retry')}</button>
    </section>
  {:else if state.user.status === 'pending'}
    <section class="card">
      <h1>{t('pendingTitle')}</h1>
      <p>{t('pendingText')}</p>
      <p class="muted">{t('signedInAs', { email: state.user.email })}</p>
    </section>
  {:else if state.user.status === 'blocked'}
    <section class="card">
      <h1>{t('blockedTitle')}</h1>
      <p>{t('blockedText')}</p>
    </section>
  {:else}
    <section class="card">
      <h1>{t('welcome')}</h1>
      <p class="muted">{t('signedInAs', { email: state.user.email })}</p>
      {#if state.user.is_admin}<p>{t('admin')}</p>{/if}
    </section>
  {/if}
</main>

<style>
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border);
  }
  main { max-width: 36rem; margin: 2rem auto; padding: 0 1rem; }
  .card {
    background: var(--card);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 1.25rem;
  }
  h1 { margin-top: 0; font-size: 1.4rem; }
  .muted { color: var(--muted); }
  button {
    background: var(--accent);
    color: #fff;
    border: 0;
    border-radius: 8px;
    padding: 0.5rem 1rem;
    font: inherit;
    cursor: pointer;
  }
  select { font: inherit; padding: 0.25rem; }
  .sr { position: absolute; left: -9999px; }
</style>

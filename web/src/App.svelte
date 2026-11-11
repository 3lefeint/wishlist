<script>
  import { onMount } from 'svelte';
  import { api, net } from './api.svelte.js';
  import { i18n, languages, setLang, t } from './i18n.svelte.js';
  import { admin, refreshPending } from './admin.svelte.js';
  import { router, match } from './router.svelte.js';
  import Home from './views/Home.svelte';
  import Circle from './views/Circle.svelte';
  import Join from './views/Join.svelte';
  import Admin from './views/Admin.svelte';
  import AdminCircles from './views/AdminCircles.svelte';
  import AdminCircle from './views/AdminCircle.svelte';
  import Person from './views/Person.svelte';
  import Profile from './views/Profile.svelte';

  let state = $state({ kind: 'loading' });

  async function load() {
    state = { kind: 'loading' };
    const r = await api.get('/api/me');
    if (r.kind === 'ok') {
      state = { kind: 'ok', user: r.data };
      if (r.data.is_admin && r.data.status === 'approved') refreshPending();
    }
    else if (r.kind === 'session') state = { kind: 'session' };
    else if (r.kind === 'http' && r.status === 503) state = { kind: 'closed' };
    else state = { kind: 'error' };
  }

  onMount(() => {
    document.documentElement.lang = i18n.lang;
    load();
  });

  const route = $derived.by(() => {
    let p;
    if (match('/', router.path)) return { view: 'home' };
    if ((p = match('/circles/:id', router.path))) return { view: 'circle', ...p };
    if ((p = match('/persons/:id', router.path))) return { view: 'person', ...p };
    if ((p = match('/join/:token', router.path))) return { view: 'join', ...p };
    if (match('/profile', router.path)) return { view: 'profile' };
    const isAdmin = state.kind === 'ok' && state.user.is_admin;
    if (isAdmin && match('/admin', router.path)) return { view: 'admin' };
    if (isAdmin && match('/admin/circles', router.path)) return { view: 'adminCircles' };
    if (isAdmin && (p = match('/admin/circles/:id', router.path))) return { view: 'adminCircle', ...p };
    return { view: 'notfound' };
  });
</script>

<header>
  <a class="brand" href="#/"><strong>{t('appName')}</strong></a>
  <span class="row">
    {#if state.kind === 'ok' && state.user.status === 'approved'}
      {#if state.user.is_admin}
        <a href="#/admin">{t('adminLink')}{#if admin.pending > 0} <span class="badge">{admin.pending}</span>{/if}</a>
      {/if}
      <a href="#/profile">{state.user.name}</a>
    {/if}
    <label>
      <span class="sr">{t('language')}</span>
      <select value={i18n.lang} onchange={(e) => setLang(e.currentTarget.value)}>
        {#each languages as l}
          <option value={l}>{l.toUpperCase()}</option>
        {/each}
      </select>
    </label>
  </span>
</header>

{#if net.sessionExpired && state.kind === 'ok'}
  <div class="banner">
    {t('sessionText')} <button class="small" onclick={() => location.reload()}>{t('reload')}</button>
  </div>
{/if}

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
  {:else if !state.user.profile_complete}
    <Profile first me={state.user} onchange={(u) => (state = { kind: 'ok', user: u })} />
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
  {:else if route.view === 'home'}
    <Home />
  {:else if route.view === 'circle'}
    {#key route.id}<Circle id={route.id} me={state.user} />{/key}
  {:else if route.view === 'person'}
    {#key route.id}<Person id={route.id} me={state.user} />{/key}
  {:else if route.view === 'join'}
    <Join token={route.token} />
  {:else if route.view === 'admin'}
    <Admin />
  {:else if route.view === 'adminCircles'}
    <AdminCircles />
  {:else if route.view === 'adminCircle'}
    {#key route.id}<AdminCircle id={route.id} />{/key}
  {:else if route.view === 'profile'}
    <Profile me={state.user} onchange={(u) => (state = { kind: 'ok', user: u })} />
  {:else}
    <section class="card"><h1>{t('notFound')}</h1><a href="#/">{t('home')}</a></section>
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
  .brand { color: inherit; text-decoration: none; }
  main { max-width: 40rem; margin: 1.5rem auto; padding: 0 1rem; }
  .banner {
    background: var(--accent);
    color: #fff;
    padding: 0.5rem 1rem;
    text-align: center;
  }
  .banner button { background: #fff; color: var(--accent); }
  .sr { position: absolute; left: -9999px; }
</style>

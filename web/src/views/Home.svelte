<script>
  import { onMount } from 'svelte';
  import { api } from '../api.svelte.js';
  import { t } from '../i18n.svelte.js';

  let { me } = $props();
  let circles = $state(null);
  let name = $state('');
  let error = $state('');

  async function load() {
    const r = await api.get('/api/circles');
    if (r.kind === 'ok') circles = r.data;
    else error = t('errorText');
  }

  async function create(e) {
    e.preventDefault();
    error = '';
    const r = await api.post('/api/circles', { name });
    if (r.kind === 'ok') {
      name = '';
      location.hash = `/circles/${r.data.id}`;
    } else if (r.kind !== 'session') error = t('invalidName');
  }

  onMount(load);
</script>

<h1>{t('circles')}</h1>

{#if circles === null}
  <p class="muted">{t('loading')}</p>
{:else}
  {#if circles.length === 0}
    <p class="muted">{t('noCircles')}</p>
  {:else}
    <div class="card">
      <ul class="plain">
        {#each circles as c (c.id)}
          <li class="row between">
            <a href={`#/circles/${c.id}`}>{c.name}</a>
            <span class="muted">{t('members', { n: c.member_count })}</span>
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  {#if !me.is_admin}
  <form class="card" onsubmit={create}>
    <label class="field">
      <span>{t('newCircle')}</span>
      <input type="text" bind:value={name} maxlength="80" required />
    </label>
    <button disabled={!name.trim()}>{t('create')}</button>
  </form>
  {/if}
{/if}
{#if error}<p class="error">{error}</p>{/if}

<script>
  import { t } from '../i18n.svelte.js';

  let { current } = $props();
</script>

<nav class="row" style="margin-bottom: 1rem">
  <a href="#/admin"><button class={current === 'users' ? '' : 'secondary'}>{t('adminTitle')}</button></a>
  <a href="#/admin/circles"><button class={current === 'circles' ? '' : 'secondary'}>{t('adminCircles')}</button></a>
</nav>

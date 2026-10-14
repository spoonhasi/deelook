<script lang="ts">
  // Two pages, chosen by the URL hash: the HMI screens (#/) and the
  // time-series collection (#/sampling). ?machine= stays for the HMI.
  import HmiView from './HmiView.svelte'
  import SamplingView from './sampling/SamplingView.svelte'

  let hash = $state(location.hash)
  $effect(() => {
    const on = () => (hash = location.hash)
    addEventListener('hashchange', on)
    return () => removeEventListener('hashchange', on)
  })
  const page = $derived(hash.startsWith('#/sampling') ? 'sampling' : 'hmi')
</script>

<nav class="tabs" aria-label="페이지">
  <a href="#/" class:on={page === 'hmi'} aria-current={page === 'hmi' ? 'page' : undefined}>HMI 화면</a>
  <a href="#/sampling" class:on={page === 'sampling'} aria-current={page === 'sampling' ? 'page' : undefined}>시계열 수집</a>
</nav>
<main>
  {#if page === 'hmi'}
    <HmiView />
  {:else}
    <SamplingView />
  {/if}
</main>

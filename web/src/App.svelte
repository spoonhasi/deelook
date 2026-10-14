<script lang="ts">
  // Two pages, chosen by the URL path: the HMI screens (/) and the
  // time-series collection (/sampling). ?machine= stays for the HMI. The
  // server answers every page path with this app; switching pages here
  // changes the address without loading the page again.
  import HmiView from './HmiView.svelte'
  import SamplingView from './sampling/SamplingView.svelte'

  // addresses of earlier builds, #/ and #/sampling, still lead there
  if (location.hash.startsWith('#/')) {
    history.replaceState(null, '', location.hash.slice(1) + location.search)
  }

  const search = location.search // kept when switching pages
  let path = $state(location.pathname)
  $effect(() => {
    const on = () => (path = location.pathname)
    addEventListener('popstate', on) // back and forward buttons
    return () => removeEventListener('popstate', on)
  })
  const page = $derived(path.replace(/\/+$/, '') === '/sampling' ? 'sampling' : 'hmi')

  function go(e: MouseEvent, to: string) {
    if (e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return // new tab or window: as the browser does
    e.preventDefault()
    if (location.pathname !== to) history.pushState(null, '', to + search)
    path = to
  }
</script>

<nav class="tabs" aria-label="페이지">
  <a href={'/' + search} onclick={(e) => go(e, '/')} class:on={page === 'hmi'} aria-current={page === 'hmi' ? 'page' : undefined}>HMI 화면</a>
  <a href={'/sampling' + search} onclick={(e) => go(e, '/sampling')} class:on={page === 'sampling'}
    aria-current={page === 'sampling' ? 'page' : undefined}>시계열 수집</a>
</nav>
<main>
  {#if page === 'hmi'}
    <HmiView />
  {:else}
    <SamplingView />
  {/if}
</main>

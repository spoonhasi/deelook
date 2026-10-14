<script lang="ts">
  // Two pages, chosen by the URL path: the HMI screens (/hmi) and the
  // time-series collection (/sampling). ?machine= stays for the HMI. The
  // server answers both paths with this app; switching between them here
  // changes the address without loading the page again. The manual (/ and
  // /help) is a page of the server's own.
  import HmiView from './HmiView.svelte'
  import { address } from './lib/address.svelte'
  import SamplingView from './sampling/SamplingView.svelte'

  const page = $derived(address.path.replace(/\/+$/, '') === '/sampling' ? 'sampling' : 'hmi')

  function go(e: MouseEvent, to: string) {
    if (e.button !== 0 || e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return // new tab or window: as the browser does
    e.preventDefault()
    address.go(to)
  }
</script>

<nav class="tabs" aria-label="페이지">
  <a href="/help">도움말</a>
  <a href={'/hmi' + address.search} onclick={(e) => go(e, '/hmi')} class:on={page === 'hmi'}
    aria-current={page === 'hmi' ? 'page' : undefined}>HMI 화면</a>
  <a href={'/sampling' + address.search} onclick={(e) => go(e, '/sampling')} class:on={page === 'sampling'}
    aria-current={page === 'sampling' ? 'page' : undefined}>시계열 수집</a>
</nav>
<main>
  {#if page === 'hmi'}
    <HmiView />
  {:else}
    <SamplingView />
  {/if}
</main>

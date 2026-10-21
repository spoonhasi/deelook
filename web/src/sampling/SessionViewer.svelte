<script lang="ts">
  // A recorded session drawn from the database: all of it, or a zoomed
  // range. Trigger marks are listed; clicking one zooms to it.
  import { api, type ChannelConfig, type SessionInfo, type View } from './api'
  import BitChart from './BitChart.svelte'
  import LiveChart from './LiveChart.svelte'
  import { trgText } from './trigger'

  let {
    session,
    unitOf,
    onclose,
  }: { session: SessionInfo; unitOf: (c: ChannelConfig) => string; onclose: () => void } = $props()

  let view = $state<View | null>(null)
  let error = $state('')
  let loading = $state(false)

  async function load(fromUs = 0, toUs = 0) {
    loading = true
    try {
      view = await api.view(session.id, fromUs, toUs)
      error = ''
    } catch (e) {
      error = String((e as Error).message)
    } finally {
      loading = false
    }
  }

  // a new session: all of it
  $effect(() => {
    void session.id
    void load()
  })

  const span = $derived(view ? view.toUs - view.fromUs : 0)
  function zoom(factor: number) {
    if (!view) return
    const mid = (view.fromUs + view.toUs) / 2
    const half = Math.max(5000, (span * factor) / 2)
    const from = Math.max(0, mid - half)
    void load(from, Math.min(view.totalUs, from + 2 * half))
  }
  function pan(dir: number) {
    if (!view) return
    const from = Math.min(Math.max(0, view.fromUs + (dir * span) / 2), Math.max(0, view.totalUs - span))
    void load(from, from + span)
  }
  const s = (us: number) => (us / 1e6).toFixed(us < 10e6 ? 3 : 1)
</script>

<section class="card viewer">
  <header>
    <h2>세션 #{session.id} 보기</h2>
    <button class="icon" onclick={onclose} aria-label="닫기">✕</button>
  </header>
  {#if view}
    <div class="bar">
      <span class="hint">{s(view.fromUs)} – {s(view.toUs)} s / 전체 {s(view.totalUs)} s</span>
      <button onclick={() => load()} disabled={loading}>전체</button>
      <button onclick={() => pan(-1)} disabled={loading || view.fromUs <= 0} aria-label="앞으로">◀</button>
      <button onclick={() => pan(1)} disabled={loading || view.toUs >= view.totalUs} aria-label="뒤로">▶</button>
      <button onclick={() => zoom(0.5)} disabled={loading || span <= 10000}>확대</button>
      <button onclick={() => zoom(2)} disabled={loading || span >= view.totalUs}>축소</button>
    </div>
    <div class="charts">
      {#each view.channels as ch, i (i)}
        {#if ch.kind === 'pmcbit'}
          <BitChart {ch} events={view.events} />
        {:else}
          <LiveChart {ch} unit={unitOf(session.config.channels[i])} events={view.events} />
        {/if}
      {/each}
    </div>
    {#if view.events.length}
      <h3>책갈피 <small>이 구간</small></h3>
      <ul class="events">
        {#each view.events as e (e.timeUs + ':' + e.bits)}
          <li>
            <button class="link" onclick={() => load(Math.max(0, e.timeUs - 250000), e.timeUs + 250000)}>{(e.timeUs / 1000).toFixed(0)} ms</button>
            <span class="hint">{trgText(e.bits)}</span>
          </li>
        {/each}
      </ul>
    {/if}
  {:else if loading}
    <p class="hint">불러오는 중…</p>
  {/if}
  {#if error}<p class="error">{error}</p>{/if}
</section>

<style>
  .card {
    background: #252525;
    border: 1px solid #333;
    border-radius: 6px;
    padding: 12px 14px;
    min-width: 0;
  }
  header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  h2 {
    margin: 0;
    font-size: 15px;
    color: var(--text-primary);
  }
  h3 {
    margin: 12px 0 4px;
    font-size: 14px;
    color: var(--text-primary);
  }
  h3 small {
    font-weight: 400;
    color: var(--text-secondary);
    font-size: 11px;
  }
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin: 8px 0;
  }
  .bar .hint {
    margin-right: auto;
  }
  button {
    font: inherit;
    font-size: 13px;
    border-radius: 4px;
    padding: 3px 10px;
    cursor: pointer;
    border: 1px solid #555;
    background: #2d2d2d;
    color: var(--text-primary);
  }
  button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  button.link {
    border: none;
    background: none;
    color: #79b8ff;
    padding: 0;
    font-variant-numeric: tabular-nums;
  }
  .charts {
    display: grid;
    gap: 10px;
  }
  .events {
    list-style: none;
    margin: 0;
    padding: 0;
    font-size: 13px;
    max-height: 180px;
    overflow-y: auto;
  }
  .events li {
    display: flex;
    gap: 10px;
    padding: 2px 0;
  }
  .hint {
    color: var(--text-secondary);
    font-size: 12px;
  }
  .error {
    color: #ff8a80;
    font-size: 13px;
  }
</style>

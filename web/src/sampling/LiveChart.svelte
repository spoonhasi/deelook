<script lang="ts">
  // The newest seconds of one channel. Each pixel column is a bucket's
  // min..max band (the server reduces 0.25-1 ms samples that way, so peaks
  // survive); where a bucket holds one sample the band is a 2 px line.
  // Times count from the session's first sample, as in the CSV; dashed
  // marks are the moments the CNC flagged (cnc_sdtreadsmpl2 triggers).
  // Hover shows the bucket's time and values.
  import type { Event, LiveChannel } from './api'
  import { trgText } from './trigger'

  let { ch, unit, events = [] }: { ch: LiveChannel; unit: string; events?: Event[] } = $props()

  const W = 600
  const H = 120
  const PAD = { l: 52, r: 8, t: 8, b: 18 }

  let hover = $state<number | null>(null) // bucket index under the pointer

  const n = $derived(ch.min.length)
  const lo = $derived(n ? Math.min(...ch.min) : 0)
  const hi = $derived(n ? Math.max(...ch.max) : 0)
  // flat data (a stopped axis) still gets a visible range around the value
  const range = $derived.by(() => {
    const pad = Math.max(1, (hi - lo) * 0.08)
    return { lo: lo - pad, hi: hi + pad }
  })
  const x = (i: number) => PAD.l + (n <= 1 ? 0 : (i / (n - 1)) * (W - PAD.l - PAD.r))
  const y = (v: number) => PAD.t + (1 - (v - range.lo) / (range.hi - range.lo)) * (H - PAD.t - PAD.b)

  const band = $derived.by(() => {
    if (!n) return ''
    let d = `M${x(0)},${y(ch.max[0])}`
    for (let i = 1; i < n; i++) d += `L${x(i)},${y(ch.max[i])}`
    for (let i = n - 1; i >= 0; i--) d += `L${x(i)},${y(ch.min[i])}`
    return d + 'Z'
  })
  const t0 = $derived(ch.endUs - n * ch.bucketUs) // time of the first bucket's start
  const at = (i: number) => (t0 + (i + 0.5) * ch.bucketUs) / 1000 // ms, bucket centre
  const marks = $derived(
    events
      .map((e) => ({ e, i: (e.timeUs - t0) / ch.bucketUs - 0.5 }))
      .filter((m) => n > 1 && m.i >= 0 && m.i <= n - 1),
  )

  function move(e: PointerEvent) {
    const svg = e.currentTarget as SVGSVGElement
    const r = svg.getBoundingClientRect()
    const px = ((e.clientX - r.left) / r.width) * W
    if (px < PAD.l || !n) return (hover = null)
    hover = Math.min(n - 1, Math.max(0, Math.round(((px - PAD.l) / (W - PAD.l - PAD.r)) * (n - 1))))
  }
</script>

<figure>
  <figcaption>
    <span class="label">{ch.label}</span>
    <span class="unit">{unit} · {ch.periodUs / 1000} ms</span>
  </figcaption>
  <svg viewBox="0 0 {W} {H}" role="img" aria-label="{ch.label} {(t0 / 1e6).toFixed(2)}–{(ch.endUs / 1e6).toFixed(2)}초"
    onpointermove={move} onpointerleave={() => (hover = null)}>
    <line class="grid" x1={PAD.l} x2={W - PAD.r} y1={y(hi)} y2={y(hi)} />
    <line class="grid" x1={PAD.l} x2={W - PAD.r} y1={y(lo)} y2={y(lo)} />
    <text class="axis" x={PAD.l - 6} y={y(hi) + 4} text-anchor="end">{hi}</text>
    <text class="axis" x={PAD.l - 6} y={y(lo) + 4} text-anchor="end">{lo}</text>
    {#if n}
      <text class="axis" x={PAD.l} y={H - 4}>{(t0 / 1e6).toFixed(2)} s</text>
      <text class="axis" x={W - PAD.r} y={H - 4} text-anchor="end">{(ch.endUs / 1e6).toFixed(2)} s</text>
      <path class="band" d={band} />
    {/if}
    {#each marks as m (m.e.timeUs)}
      <line class="mark" x1={x(m.i)} x2={x(m.i)} y1={PAD.t} y2={H - PAD.b}><title>{trgText(m.e.bits)}</title></line>
    {/each}
    {#if hover !== null}
      <line class="cross" x1={x(hover)} x2={x(hover)} y1={PAD.t} y2={H - PAD.b} />
    {/if}
  </svg>
  {#if hover !== null}
    <div class="tip" style="left: {(x(hover) / W) * 100}%">
      <span class="muted">{at(hover).toFixed(1)} ms</span>
      {#if ch.min[hover] === ch.max[hover]}
        <b>{ch.min[hover]}</b>
      {:else}
        <b>{ch.min[hover]} ~ {ch.max[hover]}</b>
      {/if}
    </div>
  {/if}
</figure>

<style>
  figure {
    position: relative;
    margin: 0;
  }
  figcaption {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
    margin-bottom: 2px;
  }
  .label {
    color: var(--text-primary);
    font-weight: 600;
  }
  .unit,
  .muted {
    color: var(--text-secondary);
  }
  svg {
    display: block;
    width: 100%;
    height: auto;
    background: var(--surface-1);
    border-radius: 4px;
    touch-action: none;
  }
  .grid {
    stroke: var(--grid);
    stroke-width: 1;
  }
  .axis {
    fill: var(--text-secondary);
    font-size: 11px;
    font-family: system-ui, sans-serif;
  }
  .band {
    fill: var(--series-1);
    fill-opacity: 0.45;
    stroke: var(--series-1);
    stroke-width: 2;
    stroke-linejoin: round;
  }
  .mark {
    stroke: var(--mark);
    stroke-width: 2;
    stroke-dasharray: 4 3;
  }
  .cross {
    stroke: var(--text-secondary);
    stroke-width: 1;
    stroke-dasharray: 3 3;
  }
  .tip {
    position: absolute;
    top: 22px;
    transform: translateX(-50%);
    display: flex;
    gap: 6px;
    padding: 2px 8px;
    font-size: 12px;
    white-space: nowrap;
    color: var(--text-primary);
    background: var(--surface-2);
    border: 1px solid var(--grid);
    border-radius: 4px;
    pointer-events: none;
  }
</style>

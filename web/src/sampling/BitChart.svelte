<script lang="ts">
  // A PMC bit channel: one lane per signal, high where the signal was 1.
  // The server gives each bucket the AND and OR of its words, so a pulse
  // shorter than a bucket still shows (as one bucket wide); hover tells
  // apart a bucket that stayed 1 from one that changed.
  import type { Event, LiveChannel } from './api'
  import { trgText } from './trigger'

  let { ch, events = [] }: { ch: LiveChannel; events?: Event[] } = $props()

  const W = 600
  const LANE = 22
  const PAD = { l: 52, r: 8, t: 6, b: 18 }

  const signals = $derived(ch.signals ?? [])
  const H = $derived(PAD.t + PAD.b + LANE * Math.max(1, signals.length))
  const n = $derived(ch.min.length)
  const x = (i: number) => PAD.l + (n <= 1 ? 0 : (i / (n - 1)) * (W - PAD.l - PAD.r))
  const step = $derived(n <= 1 ? 0 : (W - PAD.l - PAD.r) / (n - 1))

  /** The lane's waveform: high where OR has the bit. */
  function wave(b: number): string {
    const top = PAD.t + b * LANE + 4
    const bottom = PAD.t + (b + 1) * LANE - 4
    let d = ''
    for (let i = 0; i < n; i++) {
      const yv = (ch.max[i] >> b) & 1 ? top : bottom
      const x0 = x(i) - step / 2
      d += i === 0 ? `M${Math.max(PAD.l, x0)},${yv}` : `L${x0},${yv}`
      d += `L${Math.min(W - PAD.r, x(i) + step / 2)},${yv}`
    }
    return d
  }

  let hover = $state<number | null>(null)
  const t0 = $derived(ch.endUs - n * ch.bucketUs)
  const marks = $derived(
    events
      .map((e) => ({ e, i: (e.timeUs - t0) / ch.bucketUs - 0.5 }))
      .filter((m) => n > 1 && m.i >= 0 && m.i <= n - 1),
  )
  function move(e: PointerEvent) {
    const r = (e.currentTarget as SVGSVGElement).getBoundingClientRect()
    const px = ((e.clientX - r.left) / r.width) * W
    if (px < PAD.l || !n) return (hover = null)
    hover = Math.min(n - 1, Math.max(0, Math.round(((px - PAD.l) / (W - PAD.l - PAD.r)) * (n - 1))))
  }
  function bit(i: number, b: number): string {
    const and = (ch.min[i] >> b) & 1
    const or = (ch.max[i] >> b) & 1
    return and ? '1' : or ? '0↔1' : '0'
  }
</script>

<figure>
  <figcaption>
    <span class="label">{ch.label}</span>
    <span class="unit">PMC 비트 · {ch.periodUs / 1000} ms</span>
  </figcaption>
  <svg viewBox="0 0 {W} {H}" role="img" aria-label="{ch.label} PMC 신호" onpointermove={move} onpointerleave={() => (hover = null)}>
    {#each signals as s, b (b)}
      <text class="axis" x={PAD.l - 6} y={PAD.t + b * LANE + LANE / 2 + 4} text-anchor="end">{s}</text>
      <line class="grid" x1={PAD.l} x2={W - PAD.r} y1={PAD.t + (b + 1) * LANE - 4} y2={PAD.t + (b + 1) * LANE - 4} />
      {#if n}<path class="wave" d={wave(b)} />{/if}
    {/each}
    {#if n}
      <text class="axis" x={PAD.l} y={H - 4}>{(t0 / 1e6).toFixed(2)} s</text>
      <text class="axis" x={W - PAD.r} y={H - 4} text-anchor="end">{(ch.endUs / 1e6).toFixed(2)} s</text>
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
      <span class="muted">{((t0 + (hover + 0.5) * ch.bucketUs) / 1000).toFixed(1)} ms</span>
      {#each signals as s, b (b)}<span>{s} <b>{bit(hover, b)}</b></span>{/each}
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
  .wave {
    fill: none;
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
    gap: 8px;
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

<script lang="ts">
  // "Sub Panel" of NC Guide: emergency stop, feed and spindle override dials,
  // program protect key and power buttons. The E-stop and the dial pointers
  // follow the machine (emergencyStatus, feedOverride, spindleOverride).
  import type { Envelope } from '../../../lib/types'
  import * as f from '../format'

  let { values }: { values: Record<string, Envelope> } = $props()

  // dial scales as printed on the panel, with the angle of each mark
  // (degrees, counter-clockwise from 3 o'clock; measured on NC Guide)
  const FEED = [0, 1, 2, 4, 6, 8, 10, 15, 20, 30, 40, 50, 60, 70, 80, 90, 95, 100, 105, 110, 120]
  const SPINDLE = [50, 60, 70, 80, 90, 100, 110, 120]
  const feedAngle = (i: number) => 220 - (i * 270) / (FEED.length - 1)
  const spindleAngle = (i: number) => 239 - i * 29.7

  /** Angle of `v` on a scale, interpolated between its marks. */
  function angleOf(v: number | null, scale: number[], angle: (i: number) => number): number {
    if (v === null) return angle(0)
    if (v <= scale[0]) return angle(0)
    for (let i = 1; i < scale.length; i++) {
      if (v <= scale[i]) return angle(i - 1) + ((v - scale[i - 1]) / (scale[i] - scale[i - 1])) * (angle(i) - angle(i - 1))
    }
    return angle(scale.length - 1)
  }
  const pt = (cx: number, cy: number, r: number, deg: number) => {
    const a = (deg * Math.PI) / 180
    return { x: cx + r * Math.cos(a), y: cy - r * Math.sin(a) }
  }

  const emergency = $derived((f.num(values.emergency) ?? 0) === 1)
  const feed = $derived(angleOf(f.num(values.feedOverride), FEED, feedAngle))
  const spindle = $derived(angleOf(f.num(values.spindleOverride), SPINDLE, spindleAngle))
</script>

{#snippet knob(cx: number, cy: number, deg: number)}
  <circle {cx} {cy} r="33" fill="url(#knobBody)" />
  <g transform={`rotate(${-deg} ${cx} ${cy})`}>
    <rect x={cx - 38} y={cy - 12} width="76" height="24" rx="12" fill="url(#knobBar)" stroke="#2a2b2e" />
    <path d={`M${cx + 30} ${cy}l-12 -7v14z`} fill="#e8e8e8" />
  </g>
{/snippet}

<svg viewBox="8 32 680 242" xmlns="http://www.w3.org/2000/svg" aria-label="Sub Panel">
  <defs>
    <radialGradient id="knobBody" cx="0.4" cy="0.35">
      <stop offset="0" stop-color="#8a8c90" />
      <stop offset="1" stop-color="#3a3b3e" />
    </radialGradient>
    <linearGradient id="knobBar" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#77797d" />
      <stop offset="1" stop-color="#2f3033" />
    </linearGradient>
    <radialGradient id="estop" cx="0.4" cy="0.35">
      <stop offset="0" stop-color="#ff7a52" />
      <stop offset="1" stop-color="#c8341a" />
    </radialGradient>
  </defs>
  <rect x="8" y="32" width="680" height="242" rx="4" fill="#63676a" />

  <!-- emergency stop -->
  <circle cx="76" cy="152" r="57" fill="#e2dd45" />
  <circle cx="72" cy="149" r="45" fill="url(#estop)" stroke="#9c2a14" stroke-width="2" class:pressed={emergency} />
  <circle cx="72" cy="149" r="30" fill="none" stroke="#b33a20" stroke-width="2" />
  <circle cx="72" cy="149" r="18" fill="none" stroke="#b33a20" stroke-width="2" />
  {#if emergency}
    <text x="72" y="161" class="emg">EMG</text>
  {/if}

  <!-- feed override -->
  {#each FEED as mark, i (mark)}
    {@const p = pt(240, 157, 64, feedAngle(i))}
    <text x={p.x} y={p.y + 3.5} class="scale">{mark}</text>
  {/each}
  {@render knob(240, 157, feed)}
  <text x="240" y="228" class="cap">∿∿∿ (%)</text>

  <!-- spindle override -->
  {#each SPINDLE as mark, i (mark)}
    {@const p = pt(372, 160, 60, spindleAngle(i))}
    <text x={p.x} y={p.y + 3.5} class="scale">{mark}</text>
  {/each}
  {@render knob(372, 160, spindle)}
  <text x="378" y="228" class="cap">⊐ (%)</text>

  <!-- program protect key -->
  <text x="497" y="112" class="cap">⚿</text>
  <text x="478" y="141" class="sym">I</text>
  <text x="522" y="141" class="sym">O</text>
  <rect x="465" y="155" width="66" height="54" rx="3" fill="#18181a" stroke="#000" />
  <circle cx="498" cy="182" r="18" fill="#c9c2b6" />
  <rect x="486" y="179" width="24" height="6" rx="2" fill="#6e6a62" transform="rotate(-35 498 182)" />

  <!-- power -->
  <text x="610" y="92" class="cap">⏻</text>
  <rect x="582" y="113" width="58" height="40" rx="3" fill="#2c2b30" stroke="#0d0d0f" />
  <rect x="608" y="121" width="6" height="24" rx="2" fill="#f0f0f0" />
  <rect x="582" y="176" width="58" height="40" rx="3" fill="#2c2b30" stroke="#0d0d0f" />
  <circle cx="611" cy="196" r="9" fill="none" stroke="#f0f0f0" stroke-width="4" />
</svg>

<style>
  svg {
    display: block;
    width: 100%;
    height: auto;
    user-select: none;
  }
  text {
    font-family: Arial, Helvetica, sans-serif;
    text-anchor: middle;
    fill: #f2f2f2;
  }
  .scale {
    font-size: 9.5px;
  }
  .cap {
    font-size: 13px;
  }
  .sym {
    font-size: 18px;
  }
  .emg {
    font-size: 30px;
    font-weight: 700;
    fill: #ffe100;
    stroke: #000;
    stroke-width: 1.5px;
    paint-order: stroke;
  }
  .pressed {
    filter: brightness(0.85);
  }
</style>

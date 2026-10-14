<script lang="ts">
  // The Fanuc 0i-F display (640x480 as on NC Guide): header, the page the
  // navigation selects, key-in line, S/T line, status line and soft keys.
  // Coordinates are pixels of the CNC display (640×480, as NC Guide shows it).
  import type { Envelope } from '../../lib/types'
  import Bevel from './Bevel.svelte'
  import Txt from './Txt.svelte'
  import { BLACK, BLUE, GRAY, GREEN, PURPLE, RED, WHITE } from './colors'
  import * as f from './format'
  import type { FanucNav, SoftKey } from './nav.svelte'
  import AlarmPage from './pages/AlarmPage.svelte'
  import MessageRight from './pages/MessageRight.svelte'
  import OffsetPage from './pages/OffsetPage.svelte'
  import PosPage from './pages/PosPage.svelte'
  import ProgRight from './pages/ProgRight.svelte'
  import SplitLeft from './pages/SplitLeft.svelte'
  import Unsupported from './pages/Unsupported.svelte'
  import WorkRight from './pages/WorkRight.svelte'

  let { values: v, nav }: { values: Record<string, Envelope>; nav: FanucNav } = $props()

  const page = $derived(nav.current)
  // split screens keep the position title; full screens carry their own
  const title = $derived(nav.full ? page.title : 'ACTUAL POSITION')

  const time = $derived(f.clock(f.str(v.clock)))
  const emergency = $derived(f.num(v.emergency) ?? 0)
  const alarm = $derived(f.alarmText(f.num(v.alarm), v.alarm?.desc))

  const keyX: [number, number][] = [
    [6, 27],
    [30, 83],
    [86, 139],
    [142, 195],
    [198, 251],
    [254, 307],
    [332, 385],
    [388, 441],
    [444, 497],
    [500, 553],
    [556, 609],
    [612, 633],
  ]
</script>

{#snippet softKey(k: SoftKey, i: number)}
  {@const [x0, x1] = keyX[i]}
  <rect x={x0} y={433} width={x1 - x0 + 1} height={46} fill={GRAY} />
  <Bevel {x0} y0={433} {x1} y1={478} light={2} dark={3} pressed={k.pressed} />
  {#each k.label as line, n (n)}
    <Txt x={x0 + (i === 11 ? 6 : 2)} y={448 + 16 * n} text={line} fill={k.color ?? BLACK} />
  {/each}
{/snippet}

<svg viewBox="0 0 640 480" xmlns="http://www.w3.org/2000/svg" shape-rendering="crispEdges">
  <rect width="640" height="480" fill={GRAY} />

  <!-- header: page title, program comment, O/N number -->
  <rect width="640" height="34" fill={WHITE} />
  <Txt x={0} y={12} text={title} fill={PURPLE} />
  <!-- the comment gets the 27 cells left of the O/N number -->
  <Txt x={135} y={28} text={f.str(v.programComment).slice(0, 27)} fill={PURPLE} />
  <Txt x={360} y={28} big pitch={23} fill={PURPLE}
    text={`${f.programNumber(f.str(v.programName))} ${f.sequenceNumber(f.num(v.sequenceNumber))}`} />

  {#if nav.fn === 'pos'}
    <PosPage values={v} page={nav.posPage} />
  {:else if nav.fn === 'ofs' && page.id === 'offset'}
    <OffsetPage values={v} page={nav.offsetPage} />
  {:else if nav.fn === 'message' && page.id === 'alarm'}
    <AlarmPage values={v} />
  {:else if nav.full}
    <Unsupported title={page.title} full />
  {:else}
    <SplitLeft values={v} page={nav.posPage} />
    {#if nav.fn === 'prog' && page.id === 'program'}
      <ProgRight values={v} />
    {:else if nav.fn === 'ofs' && page.id === 'work'}
      <WorkRight values={v} page={nav.workPage} />
    {:else if nav.fn === 'message' && page.id === 'message'}
      <MessageRight values={v} />
    {:else}
      <Unsupported title={page.title} />
    {/if}
  {/if}

  <!-- key-in buffer (always empty: deelook does not type) -->
  <rect x="320" y="376" width="320" height="17" fill={WHITE} />
  <Txt x={321} y={388} text="A" fill={BLUE} />
  <Txt x={329} y={388} text=">" fill={PURPLE} />
  <Txt x={337} y={388} text="_" fill={PURPLE} blink />

  <!-- actual spindle speed and T code -->
  <Txt x={484} y={406} text="S" fill={GREEN} />
  <Txt x={556} y={406} anchor="end" text={f.integer(f.num(v.spindleSpeed))} />
  <Txt x={557} y={406} text="T" fill={GREEN} />
  <Txt x={564} y={406} text={String(f.num(v.tool) ?? 0).padStart(8, '0')} />

  <!-- status line: mode, automatic operation, motion, M/S/T fin, alarm, clock -->
  <Bevel x0={320} y0={412} x1={463} y1={430} />
  <Txt x={327} y={425} text={f.modeText(f.num(v.mode))} fill={PURPLE} />
  <Txt x={365} y={425} text={f.runText(f.num(v.run))} fill={PURPLE} />
  {#if emergency === 1}
    <!-- emergency stop: white on red, blinking (NC Guide) -->
    <g class="blink">
      <rect x="402" y="414" width="56" height="14" fill={RED} />
      <Txt x={402} y={425} text="--EMG--" fill="#ffffff" />
    </g>
  {:else if emergency === 2}
    <!-- shown for a moment when the emergency stop is released -->
    <Txt x={403} y={425} text="-RESET-" fill={PURPLE} />
  {:else}
    <Txt x={403} y={425} text={f.motionText(f.num(v.motion))} fill={PURPLE} />
    <Txt x={435} y={425} text="***" fill={PURPLE} />
  {/if}
  <Bevel x0={464} y0={412} x1={490} y1={430} />
  {#if alarm}
    <rect x="466" y="414" width="23" height="14" fill={RED} />
    <Txt x={465} y={425} text={alarm} fill="#ffffff" />
  {/if}
  <Bevel x0={491} y0={412} x1={564} y1={430} />
  {#if time.length === 3}
    <Txt x={495} y={425} segs={[[time[0], BLACK], [':', BLUE], [time[1], BLACK], [':', BLUE], [time[2], BLACK]]} />
  {/if}
  <Bevel x0={565} y0={412} x1={631} y1={430} />

  <!-- soft keys -->
  {#each nav.softKeys as k, i (i)}
    {#if k.onpress}
      <g class="active" role="button" tabindex="0" aria-label={k.label.join('').trim()} onclick={k.onpress}
        onkeydown={(e) => e.key === 'Enter' && k.onpress?.()}>
        {@render softKey(k, i)}
      </g>
    {:else}
      <g>{@render softKey(k, i)}</g>
    {/if}
  {/each}
</svg>

<style>
  svg {
    display: block;
    width: 100%;
    height: 100%;
    text-rendering: geometricPrecision;
    user-select: none;
  }
  .active {
    cursor: pointer;
  }
  .active:focus {
    outline: none;
  }
  .blink {
    animation: blink 1s steps(1) infinite;
  }
  @keyframes blink {
    50% {
      opacity: 0;
    }
  }
</style>

<script lang="ts">
  // Left half of the split screens (PROG, WORK, MESSAGE...): position as
  // chosen with the left soft keys, MODAL, and the actual spindle speed.
  // The top of the right half (F, counters) belongs here too.
  import type { Envelope } from '../../../lib/types'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import { GREEN } from '../colors'
  import * as f from '../format'
  import type { PosPage } from '../nav.svelte'
  import Counters from '../parts/Counters.svelte'
  import Modal from '../parts/Modal.svelte'
  import PosBig from '../parts/PosBig.svelte'
  import PosSmall from '../parts/PosSmall.svelte'

  let { values: v, page }: { values: Record<string, Envelope>; page: PosPage } = $props()

  const axes = $derived(f.axesOf(v))
  const unit = $derived(f.unitOf(v))
  const feedUnit = $derived(typeof v.feed?.unit === 'string' ? v.feed.unit.toUpperCase() : 'MM/MIN')
  const speed = $derived(f.integer(f.num(v.spindleSpeed)))
</script>

{#if page === 'all'}
  <PosSmall x0={0} y0={34} x1={158} y1={133} title="RELATIVE" titleX={48} {axes} values={v.relative} dtg={v.distanceToGo} {unit} />
  <PosSmall x0={160} y0={34} x1={318} y1={133} title="ABSOLUTE" titleX={208} {axes} values={v.absolute} dtg={v.distanceToGo} {unit} />
  <PosSmall x0={0} y0={134} x1={158} y1={233} title="MACHINE" titleX={48} {axes} values={v.machine} dtg={v.distanceToGo} {unit} />
  <PosSmall x0={160} y0={134} x1={318} y1={233} title="DISTANCE TO GO" titleX={184} {axes} values={v.distanceToGo} dtg={v.distanceToGo} {unit} />
{:else}
  <PosBig title={page === 'rel' ? 'RELATIVE' : 'ABSOLUTE'} {axes} values={page === 'rel' ? v.relative : v.absolute} dtg={v.distanceToGo} {unit} />
{/if}

<Modal values={v} />
<Txt x={9} y={395} text="S" fill={GREEN} />
{#if speed}<Txt x={191} y={395} anchor="end" text={`${speed}/MIN`} />{/if}

<!-- right half, above the function box -->
<Bevel x0={320} y0={34} x1={639} y1={68} />
<Txt x={322} y={63} big text="F" fill={GREEN} />
<Txt x={588} y={63} big anchor="end" text={f.integer(f.num(v.feed))} />
<Txt x={588} y={63} text={feedUnit} />
<Counters values={v} x={325} y={81} />

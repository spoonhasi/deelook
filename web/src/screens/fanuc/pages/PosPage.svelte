<script lang="ts">
  // POS screen over the whole display: ABSOLUTE / RELATIVE (large values on
  // the left, MACHINE and DISTANCE TO GO on the right) or ALL (four boxes).
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
</script>

{#if page === 'all'}
  <PosSmall x0={0} y0={34} x1={158} y1={147} title="RELATIVE" titleX={48} {axes} values={v.relative} dtg={v.distanceToGo} {unit} />
  <PosSmall x0={160} y0={34} x1={318} y1={147} title="ABSOLUTE" titleX={208} {axes} values={v.absolute} dtg={v.distanceToGo} {unit} />
{:else}
  <PosBig title={page === 'rel' ? 'RELATIVE' : 'ABSOLUTE'} {axes} values={page === 'rel' ? v.relative : v.absolute} dtg={v.distanceToGo} {unit} />
{/if}
<PosSmall x0={320} y0={34} x1={478} y1={147} title="MACHINE" titleX={368} {axes} values={v.machine} dtg={v.distanceToGo} {unit} />
<PosSmall x0={480} y0={34} x1={638} y1={147} title="DISTANCE TO GO" titleX={504} {axes} values={v.distanceToGo} dtg={v.distanceToGo} {unit} />

<Modal values={v} />

<Bevel x0={320} y0={224} x1={639} y1={258} />
<Txt x={322} y={253} big text="F" fill={GREEN} />
<Txt x={588} y={253} big anchor="end" text={f.integer(f.num(v.feed))} />
<Txt x={588} y={253} text={feedUnit} />
<Bevel x0={320} y0={259} x1={639} y1={293} />
<Txt x={322} y={288} big text="S" fill={GREEN} />
<Txt x={588} y={288} big anchor="end" text={f.integer(f.num(v.spindleSpeed))} />
<Txt x={588} y={288} text="/MIN" />

<Counters values={v} x={319} y={323} />

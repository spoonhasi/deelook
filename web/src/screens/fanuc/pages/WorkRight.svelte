<script lang="ts">
  // OFS/SET > WORK: four work coordinate systems per page (00 EXT, 01 G54 ...),
  // two columns of two, and the active one in brackets.
  import type { Envelope } from '../../../lib/types'
  import Txt from '../Txt.svelte'
  import { BLUE, GREEN } from '../colors'
  import * as f from '../format'
  import Field from '../parts/Field.svelte'
  import FuncBox from '../parts/FuncBox.svelte'

  let { values: v, page }: { values: Record<string, Envelope>; page: number } = $props()

  const axes = $derived(f.axesOf(v).slice(0, 3))
  const unit = $derived(f.unitOf(v))
  const active = $derived(f.str(v.activeWork))
  // slot order on a page: left top, left bottom, right top, right bottom
  const slots = [
    { x: 321, axisX: 361, f0: 387, f1: 476, top: 176 },
    { x: 321, axisX: 361, f0: 387, f1: 476, top: 263 },
    { x: 479, axisX: 519, f0: 544, f1: 633, top: 176 },
    { x: 479, axisX: 519, f0: 544, f1: 633, top: 263 },
  ]
</script>

<FuncBox title="WORK COORDINATES">
  {#if active}<Txt x={327} y={152} text={`(${active})`} fill={BLUE} />{/if}
  <Txt x={327} y={172} text="NO." fill={BLUE} />
  <Txt x={415} y={172} text="DATA" fill={BLUE} />
  <Txt x={485} y={172} text="NO." fill={BLUE} />
  <Txt x={573} y={172} text="DATA" fill={BLUE} />
  {#each slots as s, i (i)}
    {@const n = (page - 1) * 4 + i}
    {#if n < f.WORK_SYSTEMS.length}
      {@const sys = f.WORK_SYSTEMS[n]}
      <Txt x={s.x} y={s.top + 13} text={String(n).padStart(3, '0')} fill={BLUE} />
      <Txt x={s.x} y={s.top + 32} text={sys} fill={BLUE} />
      {#each axes as name, a (a)}
        <Txt x={s.axisX} y={s.top + 13 + 19 * a} text={name} fill={GREEN} />
        <Field x0={s.f0} x1={s.f1} top={s.top + 19 * a} cursor={n === 0 && a === 0} text={f.position(f.numAt(v['work' + sys], a), unit)} />
      {/each}
    {/if}
  {/each}
</FuncBox>

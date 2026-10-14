<script lang="ts">
  // OFS/SET > OFFSET over the whole display: 16 rows of the tool offset table
  // (length and radius, geometry and wear) and the positions on the right.
  import type { Envelope } from '../../../lib/types'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import { BLUE } from '../colors'
  import * as f from '../format'
  import { OFFSET_ROWS } from '../nav.svelte'
  import Field from '../parts/Field.svelte'
  import PosSmall from '../parts/PosSmall.svelte'

  let { values: v, page }: { values: Record<string, Envelope>; page: number } = $props()

  const axes = $derived(f.axesOf(v))
  const unit = $derived(f.unitOf(v))
  const count = $derived(f.num(v.offsetCount) ?? 0)
  const cols: [number, number, string][] = [
    [42, 131, 'lengthGeometry'],
    [137, 226, 'lengthWear'],
    [232, 321, 'radiusGeometry'],
    [327, 416, 'radiusWear'],
  ]
</script>

<Bevel x0={0} y0={34} x1={479} y1={374} />
<Txt x={107} y={52} text="(LENGTH)" fill={BLUE} />
<Txt x={291} y={52} text="(RADIUS)" fill={BLUE} />
<Txt x={13} y={68} text="NO." fill={BLUE} />
<Txt x={73} y={68} text="GEOM" fill={BLUE} />
<Txt x={167} y={68} text="WEAR" fill={BLUE} />
<Txt x={265} y={68} text="GEOM" fill={BLUE} />
<Txt x={357} y={68} text="WEAR" fill={BLUE} />
{#each Array.from({ length: OFFSET_ROWS }, (_, i) => i) as i (i)}
  {@const no = (page - 1) * OFFSET_ROWS + i + 1}
  {#if no <= count}
    {@const top = 71 + 18 * i}
    <Txt x={13} y={top + 13} text={String(no).padStart(3, '0')} fill={BLUE} />
    {#each cols as [x0, x1, key], c (key)}
      <Field {x0} {x1} {top} cursor={i === 0 && c === 0} text={f.position(f.numAt(v[key], i), unit)} />
    {/each}
  {/if}
{/each}

<PosSmall x0={480} y0={34} x1={638} y1={147} title="RELATIVE" titleX={528} {axes} values={v.relative} dtg={v.distanceToGo} {unit} />
<PosSmall x0={480} y0={148} x1={638} y1={261} title="ABSOLUTE" titleX={528} {axes} values={v.absolute} dtg={v.distanceToGo} {unit} />
<PosSmall x0={480} y0={262} x1={638} y1={374} title="MACHINE" titleX={528} {axes} values={v.machine} dtg={v.distanceToGo} {unit} />

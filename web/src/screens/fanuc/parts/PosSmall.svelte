<script lang="ts">
  // A small position box (MACHINE, DISTANCE TO GO, and the boxes of the ALL
  // pages): title, then one row per axis in the small font.
  import type { Envelope } from '../../../lib/types'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import { GREEN } from '../colors'
  import * as f from '../format'

  interface Props {
    x0: number
    y0: number
    x1: number
    y1: number
    title: string
    titleX: number
    axes: string[]
    values: Envelope | undefined
    dtg: Envelope | undefined
    unit: string
  }
  let { x0, y0, x1, y1, title, titleX, axes, values, dtg, unit }: Props = $props()
</script>

<Bevel {x0} {y0} {x1} {y1} />
<Txt x={titleX} y={y0 + 13} text={title} />
{#each axes as name, i (i)}
  {@const y = y0 + 30 + 16 * i}
  {#if f.moving(dtg, i)}<Txt x={x0 + 1} {y} text="*" />{/if}
  <Txt x={x0 + 9} {y} text={name} fill={GREEN} />
  <Txt x={x0 + 121} {y} anchor="end" text={f.position(f.numAt(values, i), unit)} />
{/each}

<script lang="ts">
  // The large ABSOLUTE / RELATIVE box (POS screen and left half of the
  // split screens): axis names and values in the large font.
  import type { Envelope } from '../../../lib/types'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import { GREEN } from '../colors'
  import * as f from '../format'

  interface Props {
    title: string
    axes: string[]
    values: Envelope | undefined
    dtg: Envelope | undefined
    unit: string
  }
  let { title, axes, values, dtg, unit }: Props = $props()
</script>

<Bevel x0={0} y0={34} x1={318} y1={229} />
<Txt x={128} y={47} text={title} />
{#each axes as name, i (i)}
  {#if f.moving(dtg, i)}<Txt x={0} y={64 + 32 * i} text="*" />{/if}
  <Txt x={8} y={80 + 32 * i} big text={name} fill={GREEN} />
  <Txt x={312} y={80 + 32 * i} big anchor="end" text={f.position(f.numAt(values, i), unit)} />
{/each}

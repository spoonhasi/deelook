<script lang="ts">
  // PARTS COUNT, RUN TIME and CYCLE TIME (H, M and S drawn in blue).
  import type { Envelope } from '../../../lib/types'
  import Txt from '../Txt.svelte'
  import { BLACK, BLUE } from '../colors'
  import * as f from '../format'

  interface Props {
    values: Record<string, Envelope>
    x: number // label column
    y: number // baseline of PARTS COUNT; the next rows follow 17 px apart
  }
  let { values, x, y }: Props = $props()

  const runTime = $derived(f.duration(f.num(values.runTime)))
  const cycleTime = $derived(f.duration(f.num(values.cycleTime)))
  const hms = (d: { h: string; m: string; s: string }): [string, string][] => [
    [d.h, BLACK],
    ['H', BLUE],
    [d.m, BLACK],
    ['M', BLUE],
    [d.s, BLACK],
    ['S', BLUE],
  ]
</script>

<Txt {x} {y} text="PARTS COUNT" fill={BLUE} />
<Txt x={638} {y} anchor="end" text={f.integer(f.num(values.partsCount))} />
<Txt {x} y={y + 17} text="RUN TIME" fill={BLUE} />
{#if runTime}<Txt x={638} y={y + 17} anchor="end" segs={hms(runTime)} />{/if}
<Txt {x} y={y + 34} text="CYCLE TIME" fill={BLUE} />
{#if cycleTime}<Txt x={638} y={y + 34} anchor="end" segs={hms(cycleTime)} />{/if}

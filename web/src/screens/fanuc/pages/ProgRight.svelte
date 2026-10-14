<script lang="ts">
  // PROG > PROGRAM: the look-ahead buffer as the control shows it. The first
  // line is the executing block (cyan); the lines already read into the
  // buffer come in the control's spaced form and are drawn blue.
  import type { Envelope } from '../../../lib/types'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import { BLUE, CYAN } from '../colors'
  import * as f from '../format'
  import FuncBox from '../parts/FuncBox.svelte'

  let { values: v }: { values: Record<string, Envelope> } = $props()

  const lines = $derived(f.programLines(v.lookAhead))
  const bc = $derived(String(f.num(v.blockCounter) ?? 0).padStart(9, '0'))
  // buffered blocks end with a space before the EOB; plain program text does not
  const buffered = (line: string, i: number) => i > 0 && line.endsWith(' ;')
</script>

<FuncBox title="PROGRAM">
  <Txt x={321} y={150} segs={[['BC:', BLUE], [bc, '#000000']]} />
  <Bevel x0={326} y0={167} x1={632} y1={363} />
  {#if lines.length}<rect x="328" y="168" width="296" height="16" fill={CYAN} />{/if}
  {#each lines.slice(0, 12) as line, i (i)}
    <Txt x={327} y={180 + 16 * i} text={line.slice(0, 37)} fill={buffered(line, i) ? BLUE : '#000000'} />
  {/each}
</FuncBox>

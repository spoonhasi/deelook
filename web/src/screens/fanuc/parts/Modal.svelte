<script lang="ts">
  // MODAL: G codes in FOCAS group order, 8 per column, and the F M H D T S
  // aux modals. Blank aux fields are "not commanded" (deemesh gives 0).
  import type { Envelope } from '../../../lib/types'
  import Txt from '../Txt.svelte'
  import { BLUE } from '../colors'
  import * as f from '../format'

  let { values }: { values: Record<string, Envelope> } = $props()

  const gModal = $derived(f.strArray(values.gModal))
  const aux = (i: number) => f.auxValue(f.numAt(values.aux, i)) // F,M,M2,M3,H,D,T,S
  const cols = [1, 43, 85]
  const auxRows: [string, number][] = [
    ['F', 0],
    ['H', 4],
    ['D', 5],
    ['T', 6],
    ['S', 7],
  ]
</script>

<Txt x={136} y={250} text="MODAL" />
{#each cols as x, col (col)}
  {#each [0, 1, 2, 3, 4, 5, 6, 7] as row (row)}
    <Txt {x} y={267 + 16 * row} text={gModal[col * 8 + row] ?? ''} />
  {/each}
{/each}
{#each auxRows as [letter, idx], row (letter)}
  <Txt x={128} y={267 + 16 * row} text={letter} fill={BLUE} />
  <Txt x={200} y={267 + 16 * row} anchor="end" text={aux(idx)} />
{/each}
<Txt x={205} y={267} text="M" fill={BLUE} />
{#each [1, 2, 3] as idx, row (idx)}
  <Txt x={277} y={267 + 16 * row} anchor="end" text={aux(idx)} />
{/each}

<script lang="ts">
  // MESSAGE > MESSAGE: operator and macro messages (the alarm list entries
  // the control grades as warnings), with the page's scroll bar.
  import type { Envelope } from '../../../lib/types'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import * as f from '../format'
  import FuncBox from '../parts/FuncBox.svelte'

  let { values: v }: { values: Record<string, Envelope> } = $props()

  const messages = $derived(f.alarmItems(v.alarmList).filter((a) => a.severity !== 'alarm'))
</script>

<FuncBox title="OPERATOR MESSAGE">
  {#each messages.slice(0, 12) as m, i (i)}
    <Txt x={323} y={157 + 16 * i} text={`${m.code} ${m.message}`.slice(0, 36)} />
  {/each}
  <Bevel x0={617} y0={145} x1={632} y1={352} />
  <path d="M621 155h8l-4-6z" fill="none" stroke="#000000" />
  <path d="M621 359h8l-4 6z" fill="none" stroke="#000000" />
</FuncBox>

<script lang="ts">
  // MESSAGE > ALARM over the whole display: one line per alarm in red
  // ("PS0010 IMPROPER G-CODE"), or "NO ALARM.".
  import type { Envelope } from '../../../lib/types'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import { RED } from '../colors'
  import * as f from '../format'

  let { values: v }: { values: Record<string, Envelope> } = $props()

  const alarms = $derived(f.alarmItems(v.alarmList).filter((a) => a.severity === 'alarm'))
</script>

<Bevel x0={0} y0={34} x1={639} y1={374} />
{#if f.ok(v.alarmList) && alarms.length === 0}
  <Txt x={1} y={47} text="NO ALARM." />
{/if}
{#each alarms.slice(0, 20) as a, i (i)}
  <Txt x={1} y={47 + 16 * i} text={`${a.code} ${a.message}`.slice(0, 79)} fill={RED} />
{/each}

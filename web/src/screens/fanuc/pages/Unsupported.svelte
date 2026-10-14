<script lang="ts">
  // A screen the control has but deelook does not draw (no deemesh data for
  // it yet): keeps the frame and says so.
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import FuncBox from '../parts/FuncBox.svelte'

  let { title, full = false }: { title: string; full?: boolean } = $props()
</script>

{#if full}
  <Bevel x0={0} y0={34} x1={639} y1={374} />
  <Txt x={320} y={200} anchor="middle" text="NOT AVAILABLE IN DEELOOK" />
{:else}
  <FuncBox {title}>
    <Txt x={480} y={250} anchor="middle" text="NOT AVAILABLE IN DEELOOK" />
  </FuncBox>
{/if}

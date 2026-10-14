<script lang="ts">
  // The right half of a split screen below the counters: a box with a blue
  // title bar, holding the function's own content.
  import type { Snippet } from 'svelte'
  import Bevel from '../Bevel.svelte'
  import Txt from '../Txt.svelte'
  import { BLUE } from '../colors'

  let { title, children }: { title: string; children?: Snippet } = $props()
</script>

<Bevel x0={320} y0={121} x1={639} y1={375} />
<rect x="322" y="122" width="318" height="16" fill={BLUE} />
<Txt x={480} y={135} anchor="middle" text={title} fill="#ffffff" />
{@render children?.()}

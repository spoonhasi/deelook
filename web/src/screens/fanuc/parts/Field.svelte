<script lang="ts">
  // A data cell of the OFFSET and WORK tables: a black top edge over a white
  // field (yellow where the cursor is), the value right-aligned.
  import Txt from '../Txt.svelte'
  import { WHITE, YELLOW } from '../colors'

  interface Props {
    x0: number
    x1: number
    top: number
    text: string
    cursor?: boolean
  }
  let { x0, x1, top, text, cursor = false }: Props = $props()
</script>

<rect x={x0} y={top} width={x1 - x0 + 1} height="1" fill="#000000" />
<rect x={x0} y={top + 1} width={x1 - x0 + 1} height="17" fill={WHITE} />
{#if cursor}<rect x={x0 + 1} y={top + 2} width={x1 - x0} height="14" fill={YELLOW} />{/if}
<Txt x={x1 - 1} y={top + 13} anchor="end" {text} />

<script lang="ts">
  // A box with the HMI's 3D edge: light top-left, dark bottom-right
  // (the other way round when pressed). Coordinates are inclusive pixels.
  interface Props {
    x0: number
    y0: number
    x1: number
    y1: number
    light?: number // width of the light edge
    dark?: number // width of the dark edge
    pressed?: boolean
    fill?: string
  }
  let { x0, y0, x1, y1, light = 1, dark = 2, pressed = false, fill }: Props = $props()

  const w = $derived(x1 - x0 + 1)
  const h = $derived(y1 - y0 + 1)
  const tl = $derived(pressed ? { c: '#000000', n: dark } : { c: '#ffffff', n: light })
  const br = $derived(pressed ? { c: '#ffffff', n: light } : { c: '#000000', n: dark })
</script>

{#if fill}<rect x={x0} y={y0} width={w} height={h} {fill} />{/if}
<rect x={x0} y={y0} width={w} height={tl.n} fill={tl.c} />
<rect x={x0} y={y0} width={tl.n} height={h} fill={tl.c} />
<rect x={x0} y={y1 - br.n + 1} width={w} height={br.n} fill={br.c} />
<rect x={x1 - br.n + 1} y={y0} width={br.n} height={h} fill={br.c} />
